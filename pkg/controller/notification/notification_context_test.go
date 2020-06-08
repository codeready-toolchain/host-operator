package notification

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"testing"
)

const (
	operatorNamespace = "toolchain-host-operator"
)

func newNsTemplateTier(tierName, clusterRevision string, nsTypes ...string) *toolchainv1alpha1.NSTemplateTier {
	namespaces := make([]toolchainv1alpha1.NSTemplateTierNamespace, len(nsTypes))
	for i, nsType := range nsTypes {
		revision := fmt.Sprintf("123abc%d", i+1)
		namespaces[i] = toolchainv1alpha1.NSTemplateTierNamespace{
			TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, nsType, revision),
		}
	}

	return &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNamespace,
			Name:      tierName,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{
			Namespaces: namespaces,
			ClusterResources: &toolchainv1alpha1.NSTemplateTierClusterResources{
				TemplateRef: nstemplatetiers.NewTierTemplateName(tierName, "clusterresources", clusterRevision),
			},
		},
	}
}

var basicNSTemplateTier = newNsTemplateTier("basic", "654321b", "code", "dev", "stage")

func TestNotificationContextExtractedFromUserSignupOk(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: newObjectMeta("john", "jsmith@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			Username:      "jsmith@redhat.com",
			Approved:      true,
			TargetCluster: "east",
			FamilyName:    "Smith",
			GivenName:     "John",
		},
	}

	r, _, _ := prepareReconcile(t, userSignup.Name, userSignup, basicNSTemplateTier)

	// when
	notificationCtx, err := NewNotificationContext(context.Background(), r.client, userSignup.Name, operatorNamespace)

	// then
	require.NoError(t, err)

	require.Equal(t, userSignup.Name, notificationCtx.UserID)
	require.Equal(t, userSignup.Spec.GivenName, notificationCtx.FirstName)
	require.Equal(t, userSignup.Spec.FamilyName, notificationCtx.LastName)
	require.Equal(t, userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey], notificationCtx.Email)
	require.Equal(t, userSignup.Spec.Company, notificationCtx.CompanyName)
}

func newObjectMeta(name, email string) metav1.ObjectMeta {
	if name == "" {
		name = uuid.NewV4().String()
	}

	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write([]byte(email))
	emailHash := hex.EncodeToString(md5hash.Sum(nil))

	return metav1.ObjectMeta{
		Name:      name,
		Namespace: operatorNamespace,
		Annotations: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailAnnotationKey: email,
		},
		Labels: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailHashLabelKey: emailHash,
		},
	}
}

// ReconcileUserSignup reconciles a UserSignup object
type ReconcileUserSignup struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) (*ReconcileUserSignup, reconcile.Request, *test.FakeClient) {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "test-namespace",
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte("mycooltoken"),
		},
	}

	initObjs = append(initObjs, secret)

	client := test.NewFakeClient(t, initObjs...)

	r := &ReconcileUserSignup{
		client: client,
		scheme: s,
	}
	return r, newReconcileRequest(name), client
}

func newReconcileRequest(name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: operatorNamespace,
		},
	}
}
