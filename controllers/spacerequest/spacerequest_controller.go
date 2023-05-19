package spacerequest

import (
	"context"
	"fmt"
	"reflect"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	spaceutil "github.com/codeready-toolchain/host-operator/pkg/space"
	restclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	"github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// TokenRequestExpirationSeconds is just a long duration so that the token will not expire for the lifespan of the namespace.
// This token will provide access only at the namespace scope, so once the namespace is deleted this token will just be invalid.
const TokenRequestExpirationSeconds = 3650 * 24 * 60 * 60 // 10 years

// Reconciler reconciles a SpaceRequest object
type Reconciler struct {
	Client         runtimeclient.Client
	Scheme         *runtime.Scheme
	Namespace      string
	MemberClusters map[string]cluster.Cluster
}

// SetupWithManager sets up the controller reconciler with the Manager and the given member clusters.
// Watches SpaceRequests on the member clusters as its primary resources.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, memberClusters map[string]cluster.Cluster) error {
	// since it's mandatory to add a primary resource when creating a new controller,
	// we add the SpaceRequest CR even if there should be no reconciles triggered from the host cluster,
	// only from member clusters (see watches below)
	// SpaceRequest owns subSpaces so events will be triggered for those from the host cluster.
	b := ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SpaceRequest{}).
		Watches(&source.Kind{Type: &toolchainv1alpha1.Space{}},
			handler.EnqueueRequestsFromMapFunc(MapSubSpaceToSpaceRequest()),
		)

	// Watch SpaceRequests in all member clusters and all namespaces.
	for _, memberCluster := range memberClusters {
		b = b.Watches(
			source.NewKindWithCache(&toolchainv1alpha1.SpaceRequest{}, memberCluster.Cache),
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	}
	return b.Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequests/finalizers,verbs=update

// Reconcile ensures that there is a SpaceRequest resource defined in the target member cluster
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling SpaceRequest")

	// Fetch the SpaceRequest
	// search on all member clusters
	spaceRequest := &toolchainv1alpha1.SpaceRequest{}
	var memberClusterWithSpaceRequest cluster.Cluster
	var err error
	for _, memberCluster := range r.MemberClusters {
		err = memberCluster.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: request.Namespace,
			Name:      request.Name,
		}, spaceRequest)
		if err != nil {
			if !errors.IsNotFound(err) {
				// Error reading the object - requeue the request.
				return reconcile.Result{}, errs.Wrap(err, "unable to get the current SpaceRequest")
			}

			//  spacerequest CR not found on current membercluster
			continue
		}

		// save the member cluster on which the SpaceRequest CR was found
		memberClusterWithSpaceRequest = memberCluster
		break // exit once found
	}
	// if we exited with a notFound error
	// it means that we couldn't find the spacerequest object on any of the given member clusters
	if err != nil && errors.IsNotFound(err) {
		// let's just log the info
		logger.Info("unable to find SpaceRequest")
		return reconcile.Result{}, nil
	}

	if util.IsBeingDeleted(spaceRequest) {
		logger.Info("spaceRequest is being deleted")
		return reconcile.Result{}, r.ensureSpaceDeletion(logger, memberClusterWithSpaceRequest, spaceRequest)
	}
	// Add the finalizer if it is not present
	if err := r.addFinalizer(logger, memberClusterWithSpaceRequest, spaceRequest); err != nil {
		return reconcile.Result{}, err
	}

	subSpace, createdOrUpdated, err := r.ensureSpace(logger, memberClusterWithSpaceRequest, spaceRequest)
	// if there was an error or if subSpace was just created or updated,
	// let's just return.
	if err != nil || createdOrUpdated {
		return ctrl.Result{}, err
	}

	// ensure there is a secret that provides admin access to each provisioned namespaces of the subSpace
	if err := r.ensureSecretForProvisionedNamespaces(logger, memberClusterWithSpaceRequest, spaceRequest, subSpace); err != nil {
		return reconcile.Result{}, err
	}

	// update spaceRequest conditions and target cluster url
	err = r.updateSpaceRequest(memberClusterWithSpaceRequest, spaceRequest, subSpace)

	return ctrl.Result{}, err
}

// updateSpaceRequest updates conditions and target cluster url of the spaceRequest with the ones on the Space resource
func (r *Reconciler) updateSpaceRequest(memberClusterWithSpaceRequest cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest, subSpace *toolchainv1alpha1.Space) error {
	// set target cluster URL to space request status
	targetClusterURL, err := r.getTargetClusterURL(subSpace.Status.TargetCluster)
	if err != nil {
		return err
	}
	spaceRequest.Status.TargetClusterURL = targetClusterURL
	// reflect Ready condition from subSpace to spaceRequest object
	if condition.IsTrue(subSpace.Status.Conditions, toolchainv1alpha1.ConditionReady) {
		// subSpace was provisioned so let's set Ready type condition to true on the space request
		return r.updateStatus(spaceRequest, memberClusterWithSpaceRequest, toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.SpaceProvisionedReason,
		})
	} else if condition.IsFalse(subSpace.Status.Conditions, toolchainv1alpha1.ConditionReady) {
		// subSpace is not ready let's copy the condition together with reason/message fields
		conditionReadyFromSubSpace, _ := condition.FindConditionByType(subSpace.Status.Conditions, toolchainv1alpha1.ConditionReady)
		return r.updateStatus(spaceRequest, memberClusterWithSpaceRequest, conditionReadyFromSubSpace)
	}
	return nil
}

// setFinalizers sets the finalizers for Space
func (r *Reconciler) addFinalizer(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) error {
	// Add the finalizer if it is not present
	if !util.HasFinalizer(spaceRequest, toolchainv1alpha1.FinalizerName) {
		logger.Info("adding finalizer on SpaceRequest")
		util.AddFinalizer(spaceRequest, toolchainv1alpha1.FinalizerName)
		if err := memberCluster.Client.Update(context.TODO(), spaceRequest); err != nil {
			return errs.Wrap(err, "error while adding finalizer")
		}
	}
	return nil
}

func (r *Reconciler) ensureSpace(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) (*toolchainv1alpha1.Space, bool, error) {
	logger.Info("ensuring subSpace")

	// find parent space from namespace labels
	parentSpace, err := r.getParentSpace(memberCluster, spaceRequest)
	if err != nil {
		return nil, false, err
	}
	// parentSpace is being deleted
	if util.IsBeingDeleted(parentSpace) {
		return nil, false, errs.New("parentSpace is being deleted")
	}

	// validate tierName
	if err := r.validateNSTemplateTier(spaceRequest.Spec.TierName); err != nil {
		return nil, false, err
	}

	// create if not found on the expected target cluster
	subSpace := &toolchainv1alpha1.Space{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      spaceutil.SubSpaceName(parentSpace),
	}, subSpace); err != nil {
		if errors.IsNotFound(err) {
			// no spaces found, let's create it
			logger.Info("creating subSpace")
			if err := r.setStatusProvisioning(memberCluster, spaceRequest); err != nil {
				return nil, false, errs.Wrap(err, "error updating status")
			}
			subSpace, err = r.createNewSubSpace(logger, spaceRequest, parentSpace)
			if err != nil {
				// failed to create subSpace
				return nil, false, r.setStatusFailedToCreateSubSpace(logger, memberCluster, spaceRequest, err)
			}
			return subSpace, true, nil // a subSpace was created
		}
		// failed to create subSpace
		return nil, false, r.setStatusFailedToCreateSubSpace(logger, memberCluster, spaceRequest, err)
	}
	logger.Info("subSpace already exists")
	updated, err := r.updateExistingSubSpace(logger, spaceRequest, subSpace)
	return subSpace, updated, err
}

func (r *Reconciler) createNewSubSpace(logger logr.Logger, spaceRequest *toolchainv1alpha1.SpaceRequest, parentSpace *toolchainv1alpha1.Space) (*toolchainv1alpha1.Space, error) {
	subSpace := spaceutil.NewSubSpace(spaceRequest, parentSpace)
	err := r.Client.Create(context.TODO(), subSpace)
	if err != nil && !errors.IsAlreadyExists(err) {
		return subSpace, errs.Wrap(err, "unable to create subSpace")
	}

	logger.Info("created subSpace", "subSpace.Name", subSpace.Name, "spaceRequest.Spec.TargetClusterRoles", spaceRequest.Spec.TargetClusterRoles, "spaceRequest.Spec.TierName", spaceRequest.Spec.TierName, "subSpace.Spec.TargetCluster", subSpace.Spec.TargetCluster)
	return subSpace, nil
}

// updateExistingSubSpace updates the subSpace with the config from the spaceRequest.
// returns true/nil if the subSpace was updated
// returns false/nil if the subSpace was already up to date
// returns false/err if something went wrong or the subSpace is being deleted
func (r *Reconciler) updateExistingSubSpace(logger logr.Logger, spaceRequest *toolchainv1alpha1.SpaceRequest, subSpace *toolchainv1alpha1.Space) (bool, error) {
	// subSpace found let's check if it's up to date
	logger.Info("subSpace already exists")
	// check if subSpace is being deleted
	if util.IsBeingDeleted(subSpace) {
		return false, fmt.Errorf("cannot update subSpace because it is currently being deleted")
	}
	// update tier and clusterroles in the subspace
	// if they do not match the ones on the spaceRequest
	return r.updateSubSpace(logger, subSpace, spaceRequest)
}

// validateNSTemplateTier checks if the provided tierName in the spaceRequest exists and is valid
func (r *Reconciler) validateNSTemplateTier(tierName string) error {
	if tierName == "" {
		return fmt.Errorf("tierName cannot be blank")
	}
	// check if requested tier exists
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      tierName,
	}, tier); err != nil {
		if errors.IsNotFound(err) {
			return err
		}
		// Error reading the object - requeue the request.
		return errs.Wrap(err, "unable to get the current NSTemplateTier")
	}
	return nil
}

// updateSubSpace updates the tierName and targetClusterRoles from the spaceRequest to the subSpace object
// if they are not up to date.
// returns false/nil if everything is up to date
// returns true/nil if subSpace was updated
// returns false/err if something went wrong
func (r *Reconciler) updateSubSpace(logger logr.Logger, subSpace *toolchainv1alpha1.Space, spaceRequest *toolchainv1alpha1.SpaceRequest) (bool, error) {
	logger.Info("update subSpace")
	if spaceRequest.Spec.TierName == subSpace.Spec.TierName &&
		reflect.DeepEqual(spaceRequest.Spec.TargetClusterRoles, subSpace.Spec.TargetClusterRoles) {
		// everything is up to date let's return
		return false, nil
	}

	// update tier name and target cluster roles
	logger.Info("updating subSpace", "name", subSpace.Name)
	subSpace.Spec.TierName = spaceRequest.Spec.TierName
	subSpace.Spec.TargetClusterRoles = spaceRequest.Spec.TargetClusterRoles
	err := r.Client.Update(context.TODO(), subSpace)
	if err != nil {
		return false, errs.Wrap(err, "unable to update tiername and targetclusterroles")
	}

	logger.Info("subSpace updated", "subSpace.name", subSpace.Name, "subSpace.Spec.TargetClusterRoles", subSpace.Spec.TargetClusterRoles, "subSpace.Spec.TierName", subSpace.Spec.TierName)
	return true, nil
}

// getParentSpace retrieves the name of the space that provisioned the namespace in which the spacerequest was issued.
// we need to find the parentSpace so we can `link` the 2 spaces and inherit the spacebindings from the parent.
func (r *Reconciler) getParentSpace(memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) (*toolchainv1alpha1.Space, error) {
	parentSpace := &toolchainv1alpha1.Space{}
	namespace := &corev1.Namespace{}
	err := memberCluster.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: "",
		Name:      spaceRequest.Namespace,
	}, namespace)
	if err != nil {
		return nil, errs.Wrap(err, "unable to get namespace")
	}
	// get owner name which is equal to NSTemplateSet name and Space name
	parentSpaceName, found := namespace.Labels[toolchainv1alpha1.OwnerLabelKey]
	if !found || parentSpaceName == "" {
		return nil, errs.Errorf("unable to find owner label %s on namespace %s", toolchainv1alpha1.OwnerLabelKey, namespace.GetName())
	}

	// check that parentSpace object still exists
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      parentSpaceName,
	}, parentSpace)
	if err != nil {
		return nil, errs.Wrap(err, "unable to get parentSpace")
	}

	return parentSpace, nil // all good
}

func (r *Reconciler) ensureSpaceDeletion(logger logr.Logger, memberClusterWithSpaceRequest cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) error {
	logger.Info("ensure subSpace deletion")
	if !util.HasFinalizer(spaceRequest, toolchainv1alpha1.FinalizerName) {
		// finalizer was already removed, nothing to delete anymore...
		return nil
	}
	// find parent space from namespace labels
	parentSpace, err := r.getParentSpace(memberClusterWithSpaceRequest, spaceRequest)
	if err != nil {
		return err
	}
	if isBeingDeleted, err := r.deleteSubSpace(logger, parentSpace); err != nil {
		return r.setStatusTerminatingFailed(logger, memberClusterWithSpaceRequest, spaceRequest, err)
	} else if isBeingDeleted {
		if err := r.setStatusTerminating(memberClusterWithSpaceRequest, spaceRequest); err != nil {
			return errs.Wrap(err, "error updating status")
		}
		return nil
	}

	// Remove finalizer from SpaceRequest
	util.RemoveFinalizer(spaceRequest, toolchainv1alpha1.FinalizerName)
	if err := memberClusterWithSpaceRequest.Client.Update(context.TODO(), spaceRequest); err != nil {
		return r.setStatusTerminatingFailed(logger, memberClusterWithSpaceRequest, spaceRequest, errs.Wrap(err, "failed to remove finalizer"))
	}
	logger.Info("removed finalizer")
	return nil
}

// deleteSubSpace checks if the subSpace is still provisioned and in case deletes it.
// returns true/nil if the deletion of the subSpace was triggered
// returns false/nil if the subSpace was already deleted
// return false/err if something went wrong
func (r *Reconciler) deleteSubSpace(logger logr.Logger, parentSpace *toolchainv1alpha1.Space) (bool, error) {
	subSpace := &toolchainv1alpha1.Space{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Namespace,
		Name:      spaceutil.SubSpaceName(parentSpace),
	}, subSpace); err != nil {
		if errors.IsNotFound(err) {
			// no spaces found, already deleted
			logger.Info("subSpace was already deleted", "subSpace.name", subSpace.Name)
			return false, nil
		}
		// failed to get subSpace
		return false, err
	}
	// deleting subSpace
	return r.deleteExistingSubSpace(logger, subSpace)
}

// deleteExistingSubSpace deletes a given space object in case deletion was not issued already.
// returns true/nil if the deletion of the space was triggered
// returns false/nil if the space was already deleted
// return false/err if something went wrong
func (r *Reconciler) deleteExistingSubSpace(logger logr.Logger, subSpace *toolchainv1alpha1.Space) (bool, error) {
	if util.IsBeingDeleted(subSpace) {
		logger.Info("the subSpace resource is already being deleted")
		deletionTimestamp := subSpace.GetDeletionTimestamp()
		if time.Since(deletionTimestamp.Time) > 120*time.Second {
			return false, fmt.Errorf("subSpace deletion has not completed in over 2 minutes")
		}
		return true, nil // subspace is still being deleted
	}

	logger.Info("deleting the subSpace resource", "subSpace name", subSpace.Name)
	// Delete subSpace associated with SpaceRequest
	if err := r.Client.Delete(context.TODO(), subSpace); err != nil {
		if errors.IsNotFound(err) {
			return false, nil // was already deleted
		}
		return false, errs.Wrap(err, "unable to delete subspace") // something wrong happened
	}
	logger.Info("deleted the subSpace resource", "subSpace name", subSpace.Name)
	return true, nil
}

func (r *Reconciler) ensureSecretForProvisionedNamespaces(logger logr.Logger, memberClusterWithSpaceRequest cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest, subSpace *toolchainv1alpha1.Space) error {
	if len(subSpace.Status.ProvisionedNamespaces) == 0 {
		logger.Info("provisioned namespaces not available yet...")
		return nil
	}
	logger.Info("ensure secret for provisioned namespaces")

	var namespaceAccess []toolchainv1alpha1.NamespaceAccess
	for _, namespace := range subSpace.Status.ProvisionedNamespaces {
		// check if kubeconfig secret exists,
		// if it doesn't exist it will be created
		secretList := &corev1.SecretList{}
		secretLabels := runtimeclient.MatchingLabels{
			toolchainv1alpha1.SpaceRequestLabelKey:                     spaceRequest.GetName(),
			toolchainv1alpha1.SpaceRequestProvisionedNamespaceLabelKey: namespace.Name,
		}
		if err := memberClusterWithSpaceRequest.Client.List(context.TODO(), secretList, secretLabels, runtimeclient.InNamespace(spaceRequest.GetNamespace())); err != nil {
			return errs.Wrap(err, fmt.Sprintf(`attempt to list Secrets associated with spaceRequest %s in namespace %s failed`, spaceRequest.GetName(), spaceRequest.GetNamespace()))
		}

		kubeConfigSecret := &corev1.Secret{}
		switch {
		case len(secretList.Items) == 0:
			// create the secret for this namespace
			clientConfig, err := r.generateKubeConfig(memberClusterWithSpaceRequest, namespace.Name)
			if err != nil {
				return err
			}
			clientConfigFormatted, err := clientcmd.Write(*clientConfig)
			if err != nil {
				return err
			}
			kubeConfigSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: spaceRequest.Name + "-",
					Namespace:    spaceRequest.Namespace,
					Labels:       secretLabels,
				},
				Type: corev1.SecretTypeOpaque,
				StringData: map[string]string{
					"kubeconfig": string(clientConfigFormatted),
				},
			}
			if err := controllerutil.SetControllerReference(spaceRequest, kubeConfigSecret, r.Scheme); err != nil {
				return errs.Wrap(err, "error setting controller reference for secret "+kubeConfigSecret.Name)
			}
			if err := memberClusterWithSpaceRequest.Client.Create(context.TODO(), kubeConfigSecret); err != nil {
				return err
			}
			logger.Info("Created Secret", "Name", kubeConfigSecret.Name)

			// add provisioned namespace and name of the secret that provides access to the namespace
			namespaceAccess = append(namespaceAccess, toolchainv1alpha1.NamespaceAccess{
				Name:      namespace.Name,
				SecretRef: kubeConfigSecret.Name,
			})

		case len(secretList.Items) == 1:
			// a secret is already present for this namespace
			namespaceAccess = append(namespaceAccess, toolchainv1alpha1.NamespaceAccess{
				Name:      namespace.Name,
				SecretRef: secretList.Items[0].Name,
			})

		case len(secretList.Items) > 1:
			// some unexpected issue causing to many secrets
			return fmt.Errorf("invalid number of secrets found. actual %d, expected %d", len(secretList.Items), 1)
		}

	}

	// update space request status in case secrets for provisioned namespace were created.
	if len(namespaceAccess) > 0 {
		spaceRequest.Status.NamespaceAccess = namespaceAccess
		return memberClusterWithSpaceRequest.Client.Status().Update(context.TODO(), spaceRequest)
	}

	return nil
}

func (r *Reconciler) generateKubeConfig(memberClusterWithSpaceRequest cluster.Cluster, namespace string) (*api.Config, error) {
	// create a token request for the admin service account
	token, err := restclient.CreateTokenRequest(memberClusterWithSpaceRequest.RESTClient, types.NamespacedName{
		Namespace: namespace,
		Name:      toolchainv1alpha1.AdminServiceAccountName,
	}, TokenRequestExpirationSeconds)
	if err != nil {
		return nil, err
	}

	// create apiConfig based on the secret content
	clusters := make(map[string]*api.Cluster, 1)
	clusters["default-cluster"] = &api.Cluster{
		Server:                   memberClusterWithSpaceRequest.Config.APIEndpoint,
		CertificateAuthorityData: memberClusterWithSpaceRequest.Config.RestConfig.CAData,
	}
	contexts := make(map[string]*api.Context, 1)
	contexts["default-context"] = &api.Context{
		Cluster:   "default-cluster",
		Namespace: namespace,
		AuthInfo:  toolchainv1alpha1.AdminServiceAccountName,
	}
	authinfos := make(map[string]*api.AuthInfo, 1)
	authinfos[toolchainv1alpha1.AdminServiceAccountName] = &api.AuthInfo{
		Token: token,
	}

	clientConfig := &api.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default-context",
		AuthInfos:      authinfos,
	}
	return clientConfig, nil
}

func (r *Reconciler) setStatusProvisioning(memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) error {
	return r.updateStatus(
		spaceRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceProvisioningReason,
		})
}

func (r *Reconciler) setStatusTerminating(memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest) error {
	return r.updateStatus(
		spaceRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ConditionReady,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.SpaceTerminatingReason,
		})
}

func (r *Reconciler) setStatusTerminatingFailed(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest, cause error) error {
	if err := r.updateStatus(
		spaceRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceTerminatingFailedReason,
			Message: cause.Error(),
		}); err != nil {
		logger.Error(cause, "unable to terminate Space")
		return err
	}
	return cause
}

func (r *Reconciler) setStatusFailedToCreateSubSpace(logger logr.Logger, memberCluster cluster.Cluster, spaceRequest *toolchainv1alpha1.SpaceRequest, cause error) error {
	if err := r.updateStatus(
		spaceRequest,
		memberCluster,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ConditionReady,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.SpaceProvisioningFailedReason,
			Message: cause.Error(),
		}); err != nil {
		logger.Error(err, "unable to create Space")
		return err
	}
	return cause
}

func (r *Reconciler) updateStatus(spaceRequest *toolchainv1alpha1.SpaceRequest, memberCluster cluster.Cluster, conditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	spaceRequest.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(spaceRequest.Status.Conditions, conditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return memberCluster.Client.Status().Update(context.TODO(), spaceRequest)
}

// getTargetClusterURL checks if the targetClusterName from the space exists and returns it's APIEndpoint.
func (r *Reconciler) getTargetClusterURL(targetClusterName string) (string, error) {
	targetCluster, found := r.MemberClusters[targetClusterName]
	if !found {
		return "", fmt.Errorf("unable to find target cluster with name: %s", targetClusterName)
	}
	return targetCluster.APIEndpoint, nil
}
