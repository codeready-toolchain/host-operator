package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/changetierrequest"
	"github.com/codeready-toolchain/host-operator/controllers/deactivation"
	"github.com/codeready-toolchain/host-operator/controllers/masteruserrecord"
	"github.com/codeready-toolchain/host-operator/controllers/notification"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/controllers/registrationservice"
	"github.com/codeready-toolchain/host-operator/controllers/templateupdaterequest"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainstatus"
	"github.com/codeready-toolchain/host-operator/controllers/usersignup"
	"github.com/codeready-toolchain/host-operator/controllers/usersignupcleanup"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/controllers/toolchaincluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	kubemetrics "github.com/operator-framework/operator-sdk/pkg/kube-metrics"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkmetrics "github.com/operator-framework/operator-sdk/pkg/metrics"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	//+kubebuilder:scaffold:imports
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost               = "0.0.0.0"
	metricsPort         int32 = 8383
	operatorMetricsPort int32 = 8686

	setupLog = ctrl.Log.WithName("setup")
	scheme   = k8sscheme.Scheme
)

func init() {
	utilruntime.Must(apis.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func printVersion() {
	setupLog.Info(fmt.Sprintf("Operator Version: %s", version.Version))
	setupLog.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	setupLog.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
	setupLog.Info(fmt.Sprintf("Commit: %s", version.Commit))
	setupLog.Info(fmt.Sprintf("BuildTime: %s", version.BuildTime))
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=tiertemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=tiertemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=tiertemplates/finalizers,verbs=update

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=toolchainclusters/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=secrets;configmaps;services;services/finalizers;serviceaccounts;pods,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=apps,resources=deployments;deployments/finalizers;replicasets,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;update;patch;create;delete

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.Logger())

	printVersion()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	crtConfig, err := getCRTConfiguration(cfg)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	crtConfig.PrintConfig()

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		setupLog.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "host-operator-lock")
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	// Set default manager options
	options := manager.Options{
		Namespace:          namespace,
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	// Note that this is not intended to be used for excluding namespaces, this is better done via a Predicate
	// Also note that you may face performance issues when using this with a high number of namespaces.
	// More Info: https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg/cache#MultiNamespacedCacheBuilder
	if strings.Contains(namespace, ",") {
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(namespace, ","))
	}

	// Create a new manager to provide shared dependencies and start components
	mgr, err := manager.New(cfg, options)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	setupLog.Info("Registering Components.")

	// Setup all Controllers
	if err = toolchaincluster.NewReconciler(
		mgr,
		ctrl.Log.WithName("controllers").WithName("ToolchainCluster"),
		namespace,
		3*time.Second,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainCluster")
		os.Exit(1)
	}
	if err := (&changetierrequest.Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("ChangeTierRequest"),
		Scheme: mgr.GetScheme(),
		Config: crtConfig,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ChangeTierRequest")
		os.Exit(1)
	}
	if err := (&deactivation.Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Deactivation"),
		Scheme: mgr.GetScheme(),
		Config: crtConfig,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Deactivation")
		os.Exit(1)
	}
	if err := (&masteruserrecord.Reconciler{
		Client:                mgr.GetClient(),
		Log:                   ctrl.Log.WithName("controllers").WithName("MasterUserRecord"),
		Scheme:                mgr.GetScheme(),
		Config:                crtConfig,
		RetrieveMemberCluster: cluster.GetCachedToolchainCluster,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MasterUserRecord")
		os.Exit(1)
	}
	if err := (&notification.Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Notification"),
		Scheme: mgr.GetScheme(),
		Config: crtConfig,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Notification")
	}
	if err := (&nstemplatetier.Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("NSTemplateTier"),
		Scheme: mgr.GetScheme(),
		Config: crtConfig,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NSTemplateTier")
	}
	if err := (&registrationservice.Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("RegistrationService"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RegistrationService")
	}
	if err := (&templateupdaterequest.Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("TemplateUpdateRequest"),
		Scheme: mgr.GetScheme(),
		Config: crtConfig,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TemplateUpdateRequest")
	}
	if err := (&toolchainconfig.Reconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("ToolchainConfig"),
		GetMembersFunc: cluster.GetMemberClusters,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainConfig")
	}
	if err := (&toolchainstatus.Reconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("ToolchainStatus"),
		Scheme:         mgr.GetScheme(),
		Config:         crtConfig,
		HTTPClientImpl: &http.Client{},
		GetMembersFunc: cluster.GetMemberClusters,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainStatus")
	}
	if err := (&usersignup.Reconciler{
		StatusUpdater: &usersignup.StatusUpdater{
			Client: mgr.GetClient(),
		},
		Scheme:            mgr.GetScheme(),
		Log:               ctrl.Log.WithName("controllers").WithName("UserSignup"),
		CrtConfig:         crtConfig,
		GetMemberClusters: cluster.GetMemberClusters,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserSignup")
	}
	if err := (&usersignupcleanup.Reconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("UserSignupCleanup"),
		Scheme: mgr.GetScheme(),
		Config: crtConfig,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserSignupCleanup")
	}
	//+kubebuilder:scaffold:builder

	// Add the Metrics Service
	if err := addMetrics(ctx, cfg); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	metrics.RegisterCustomMetrics()

	stopChannel := signals.SetupSignalHandler()

	go func() {
		setupLog.Info("Starting cluster health checker & creating/updating the NSTemplateTier resources once cache is sync'd")
		if !mgr.GetCache().WaitForCacheSync(stopChannel) {
			setupLog.Error(errors.New("timed out waiting for caches to sync"), "")
			os.Exit(1)
		}

		setupLog.Info("Starting ToolchainCluster health checks.")
		toolchaincluster.StartHealthChecks(mgr, namespace, stopChannel, 10*time.Second)

		// create or update Toolchain status during the operator deployment
		setupLog.Info("Creating/updating the ToolchainStatus resource")
		toolchainStatusName := configuration.ToolchainStatusName
		if err := toolchainstatus.CreateOrUpdateResources(mgr.GetClient(), mgr.GetScheme(), namespace, toolchainStatusName); err != nil {
			setupLog.Error(err, "cannot create/update ToolchainStatus resource")
			os.Exit(1)
		}
		setupLog.Info("Created/updated the ToolchainStatus resource")

		// create or update Registration service during the operator deployment
		setupLog.Info("Creating/updating the RegistrationService resource")
		if err := registrationservice.CreateOrUpdateResources(mgr.GetClient(), mgr.GetScheme(), namespace, crtConfig); err != nil {
			setupLog.Error(err, "cannot create/update RegistrationService resource")
			os.Exit(1)
		}
		setupLog.Info("Created/updated the RegistrationService resources")

		// create or update all NSTemplateTiers on the cluster at startup
		setupLog.Info("Creating/updating the NSTemplateTier resources")
		assets := assets.NewAssets(nstemplatetiers.AssetNames, nstemplatetiers.Asset)
		if err := nstemplatetiers.CreateOrUpdateResources(mgr.GetScheme(), mgr.GetClient(), namespace, assets); err != nil {
			setupLog.Error(err, "")
			os.Exit(1)
		}
		setupLog.Info("Created/updated the NSTemplateTier resources")
	}()

	setupLog.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(stopChannel); err != nil {
		setupLog.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}

// addMetrics will create the Services and Service Monitors to allow the operator export the metrics by using
// the Prometheus operator
func addMetrics(ctx context.Context, cfg *rest.Config) error {
	// Get the namespace the operator is currently deployed in.
	operatorNs, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		if errors.Is(err, k8sutil.ErrRunLocal) {
			setupLog.Info("Skipping CR metrics server creation; not running in a cluster.")
			return nil
		}
	}

	// Add to the below struct any other metrics ports you want to expose.
	servicePorts := []v1.ServicePort{
		{Port: metricsPort, Name: sdkmetrics.OperatorPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: metricsPort}},
		{Port: operatorMetricsPort, Name: sdkmetrics.CRPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: operatorMetricsPort}},
	}

	// Create Service object to expose the metrics port(s).
	service, err := sdkmetrics.CreateMetricsService(ctx, cfg, servicePorts)
	if err != nil {
		return errors.Wrapf(err, "Could not create metrics Service")
	}

	// CreateServiceMonitors will automatically create the prometheus-operator ServiceMonitor resources
	// necessary to configure Prometheus to scrape metrics from this operator.
	services := []*v1.Service{service}

	// The ServiceMonitor is created in the same namespace where the operator is deployed
	_, err = sdkmetrics.CreateServiceMonitors(cfg, operatorNs, services)
	if err != nil {
		setupLog.Info("Could not create ServiceMonitor object", "error", err.Error())
		// If this operator is deployed to a cluster without the prometheus-operator running, it will return
		// ErrServiceMonitorNotPresent, which can be used to safely skip ServiceMonitor creation.
		if err == sdkmetrics.ErrServiceMonitorNotPresent {
			setupLog.Info("Install prometheus-operator in your cluster to create ServiceMonitor objects", "error", err.Error())
		}
	}
	return nil
}

// serveCRMetrics gets the Operator/CustomResource GVKs and generates metrics based on those types.
// It serves those metrics on "http://metricsHost:operatorMetricsPort".
//
// Note: not used for now: by default, this function wants to use all out CRDs, but when we have a "host-only"
// cluster, this metrics server fails to load the CRDs from etcd, which causes CrashLoops.
// If we really need this metrics server, then we should taylor the list of CRDs to expose.
func serveCRMetrics(cfg *rest.Config, operatorNs string) error { //nolint:unused,deadcode
	// The function below returns a list of filtered operator/CR specific GVKs. For more control, override the GVK list below
	// with your own custom logic. Note that if you are adding third party API schemas, probably you will need to
	// customize this implementation to avoid permissions issues.
	filteredGVK, err := k8sutil.GetGVKsFromAddToScheme(toolchainv1alpha1.AddToScheme)
	if err != nil {
		return err
	}

	// The metrics will be generated from the namespaces which are returned here.
	// NOTE that passing nil or an empty list of namespaces in GenerateAndServeCRMetrics will result in an error.
	ns, err := kubemetrics.GetNamespacesForMetrics(operatorNs)
	if err != nil {
		return err
	}

	// Generate and serve custom resource specific metrics.
	err = kubemetrics.GenerateAndServeCRMetrics(cfg, ns, filteredGVK, metricsHost, operatorMetricsPort)
	if err != nil {
		return err
	}
	return nil
}

// getCRTConfiguration creates the client used for configuration and
// returns the loaded crt configuration
func getCRTConfiguration(config *rest.Config) (*configuration.Config, error) {
	// create client that will be used for retrieving the host operator secret
	cl, err := client.New(config, client.Options{})
	if err != nil {
		return nil, err
	}

	return configuration.LoadConfig(cl)
}
