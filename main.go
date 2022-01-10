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
	"github.com/codeready-toolchain/host-operator/controllers/space"
	"github.com/codeready-toolchain/host-operator/controllers/templateupdaterequest"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainstatus"
	"github.com/codeready-toolchain/host-operator/controllers/usersignup"
	"github.com/codeready-toolchain/host-operator/controllers/usersignupcleanup"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/controllers/toolchaincluster"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"

	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	klogv1 "k8s.io/klog"
	klogv2 "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	runtimecluster "sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const memberClientTimeout = 3 * time.Second

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
		Encoder: zapcore.NewJSONEncoder(zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}),
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// also set the client-go logger so we get the same JSON output
	klogv2.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// see https://github.com/kubernetes/klog#coexisting-with-klogv2
	// BEGIN : hack to redirect klogv1 calls to klog v2
	// Tell klog NOT to log into STDERR. Otherwise, we risk
	// certain kinds of API errors getting logged into a directory not
	// available in a `FROM scratch` Docker container, causing us to abort
	var klogv1Flags flag.FlagSet
	klogv1.InitFlags(&klogv1Flags)
	if err := klogv1Flags.Set("logtostderr", "false"); err != nil { // By default klog v1 logs to stderr, switch that off
		setupLog.Error(err, "")
		os.Exit(1)
	}
	if err := klogv1Flags.Set("stderrthreshold", "FATAL"); err != nil { // stderrthreshold defaults to ERROR, so we don't get anything in stderr
		setupLog.Error(err, "")
		os.Exit(1)
	}
	klogv1.SetOutputBySeverity("INFO", klogWriter{}) // tell klog v1 to use the custom writer
	// END : hack to redirect klogv1 calls to klog v2

	printVersion()

	namespace, err := commonconfig.GetWatchNamespace()
	if err != nil {
		setupLog.Error(err, "failed to get watch namespace")
		os.Exit(1)
	}

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	// create client that will be used for retrieving the host operator secret & ToolchainCluster CRs
	cl, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		setupLog.Error(err, "unable to create a client")
		os.Exit(1)
	}

	crtConfig, err := toolchainconfig.GetToolchainConfig(cl)
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
	memberClusters, err := addMemberClusters(mgr, cl, namespace)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err = toolchaincluster.NewReconciler(
		mgr,
		namespace,
		memberClientTimeout,
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
		RetrieveMemberCluster: commoncluster.GetCachedToolchainCluster,
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
		GetMembersFunc: commoncluster.GetMemberClusters,
		Scheme:         mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainConfig")
	}
	if err := (&toolchainstatus.Reconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		HTTPClientImpl: &http.Client{},
		GetMembersFunc: commoncluster.GetMemberClusters,
		Namespace:      namespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainStatus")
	}
	if err := (&usersignup.Reconciler{
		StatusUpdater: &usersignup.StatusUpdater{
			Client: mgr.GetClient(),
		},
		Scheme:            mgr.GetScheme(),
		GetMemberClusters: commoncluster.GetMemberClusters,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserSignup")
	}
	if err := (&usersignupcleanup.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserSignupCleanup")
	}
	if err = (&space.Reconciler{
		Client:         mgr.GetClient(),
		Namespace:      namespace,
		MemberClusters: memberClusters,
	}).SetupWithManager(mgr, memberClusters); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Space")
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

func addMemberClusters(mgr ctrl.Manager, cl client.Client, namespace string) (map[string]cluster.Cluster, error) {
	memberConfigs, err := commoncluster.ListToolchainClusterConfigs(cl, namespace, commoncluster.Member, memberClientTimeout)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get ToolchainCluster configs for members")
	}
	memberClusters := map[string]cluster.Cluster{}
	for _, memberConfig := range memberConfigs {
		setupLog.Info("adding cluster for a member", "name", memberConfig.Name, "apiEndpoint", memberConfig.APIEndpoint)

		memberCluster, err := runtimecluster.New(memberConfig.RestConfig, func(options *runtimecluster.Options) {
			options.Scheme = scheme
			options.Namespace = memberConfig.OperatorNamespace
		})
		if err != nil {
			return nil, errors.Wrapf(err, "unable to create member cluster definition for "+memberConfig.Name)
		}
		if err := mgr.Add(memberCluster); err != nil {
			return nil, errors.Wrapf(err, "unable to add member cluster to the manager for "+memberConfig.Name)
		}
		memberClusters[memberConfig.Name] = cluster.Cluster{
			OperatorNamespace: memberConfig.OperatorNamespace,
			Client:            memberCluster.GetClient(),
			Cache:             memberCluster.GetCache(),
		}
	}
	return memberClusters, nil
}

// OutputCallDepth is the stack depth where we can find the origin of this call
const OutputCallDepth = 6

// DefaultPrefixLength is the length of the log prefix that we have to strip out
const DefaultPrefixLength = 53

// klogWriter is used in SetOutputBySeverity call below to redirect
// any calls to klogv1 to end up in klogv2
type klogWriter struct{}

func (kw klogWriter) Write(p []byte) (n int, err error) {
	if len(p) < DefaultPrefixLength {
		klogv2.InfoDepth(OutputCallDepth, string(p))
		return len(p), nil
	}
	if p[0] == 'I' {
		klogv2.InfoDepth(OutputCallDepth, string(p[DefaultPrefixLength:]))
	} else if p[0] == 'W' {
		klogv2.WarningDepth(OutputCallDepth, string(p[DefaultPrefixLength:]))
	} else if p[0] == 'E' {
		klogv2.ErrorDepth(OutputCallDepth, string(p[DefaultPrefixLength:]))
	} else if p[0] == 'F' {
		klogv2.FatalDepth(OutputCallDepth, string(p[DefaultPrefixLength:]))
	} else {
		klogv2.InfoDepth(OutputCallDepth, string(p[DefaultPrefixLength:]))
	}
	return len(p), nil
}
