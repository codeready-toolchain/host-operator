package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"
	"time"

	"github.com/codeready-toolchain/host-operator/controllers/changetierrequest"
	"github.com/codeready-toolchain/host-operator/controllers/deactivation"
	"github.com/codeready-toolchain/host-operator/controllers/masteruserrecord"
	"github.com/codeready-toolchain/host-operator/controllers/notification"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/controllers/templateupdaterequest"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainstatus"
	"github.com/codeready-toolchain/host-operator/controllers/usersignup"
	"github.com/codeready-toolchain/host-operator/controllers/usersignupcleanup"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/controllers/toolchaincluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(apis.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	metrics.RegisterCustomMetrics()
}

func printVersion() {
	setupLog.Info(fmt.Sprintf("Operator Version: %s", version.Version))
	setupLog.Info(fmt.Sprintf("Go Version: %s", goruntime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", goruntime.GOOS, goruntime.GOARCH))
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
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	printVersion()

	namespace, err := commonconfig.GetWatchNamespace()
	if err != nil {
		setupLog.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

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

	crtConfig.Print()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "dc07038f.toolchain.host.operator",
		Namespace:              namespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup all Controllers
	if err = toolchaincluster.NewReconciler(
		mgr,
		namespace,
		3*time.Second,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainCluster")
		os.Exit(1)
	}
	if err := (&changetierrequest.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ChangeTierRequest")
		os.Exit(1)
	}
	if err := (&deactivation.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Deactivation")
		os.Exit(1)
	}
	if err := (&masteruserrecord.Reconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		RetrieveMemberCluster: cluster.GetCachedToolchainCluster,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MasterUserRecord")
		os.Exit(1)
	}
	if err := (&notification.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, crtConfig); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Notification")
	}
	if err := (&nstemplatetier.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NSTemplateTier")
	}
	if err := (&templateupdaterequest.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TemplateUpdateRequest")
	}
	if err := (&toolchainconfig.Reconciler{
		Client:         mgr.GetClient(),
		GetMembersFunc: cluster.GetMemberClusters,
		Scheme:         mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainConfig")
	}
	if err := (&toolchainstatus.Reconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
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
		GetMemberClusters: cluster.GetMemberClusters,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserSignup")
	}
	if err := (&usersignupcleanup.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserSignupCleanup")
	}
	//+kubebuilder:scaffold:builder

	stopChannel := ctrl.SetupSignalHandler()

	go func() {
		setupLog.Info("Starting cluster health checker & creating/updating the NSTemplateTier resources once cache is sync'd")
		if !mgr.GetCache().WaitForCacheSync(stopChannel) {
			setupLog.Error(errors.New("timed out waiting for caches to sync"), "")
			os.Exit(1)
		}

		setupLog.Info("Starting ToolchainCluster health checks.")
		toolchaincluster.StartHealthChecks(stopChannel, mgr, namespace, 10*time.Second)

		// create or update Toolchain status during the operator deployment
		setupLog.Info("Creating/updating the ToolchainStatus resource")
		if err := toolchainstatus.CreateOrUpdateResources(mgr.GetClient(), mgr.GetScheme(), namespace, toolchainconfig.ToolchainStatusName); err != nil {
			setupLog.Error(err, "cannot create/update ToolchainStatus resource")
			os.Exit(1)
		}
		setupLog.Info("Created/updated the ToolchainStatus resource")

		// create or update all NSTemplateTiers on the cluster at startup
		setupLog.Info("Creating/updating the NSTemplateTier resources")
		tierAssets := assets.NewAssets(nstemplatetiers.AssetNames, nstemplatetiers.Asset)
		if err := nstemplatetiers.CreateOrUpdateResources(mgr.GetScheme(), mgr.GetClient(), namespace, tierAssets); err != nil {
			setupLog.Error(err, "")
			os.Exit(1)
		}
		setupLog.Info("Created/updated the NSTemplateTier resources")
	}()

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(stopChannel); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// getCRTConfiguration creates the client used for configuration and
// returns the loaded crt configuration
func getCRTConfiguration(config *rest.Config) (toolchainconfig.ToolchainConfig, error) {
	// create client that will be used for retrieving the host operator secret
	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return toolchainconfig.ToolchainConfig{}, err
	}

	return toolchainconfig.GetToolchainConfig(cl)
}
