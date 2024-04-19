package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"
	"time"

	"github.com/codeready-toolchain/host-operator/controllers/deactivation"
	"github.com/codeready-toolchain/host-operator/controllers/masteruserrecord"
	"github.com/codeready-toolchain/host-operator/controllers/notification"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/controllers/socialevent"
	"github.com/codeready-toolchain/host-operator/controllers/space"
	"github.com/codeready-toolchain/host-operator/controllers/spacebindingcleanup"
	"github.com/codeready-toolchain/host-operator/controllers/spacebindingrequest"
	"github.com/codeready-toolchain/host-operator/controllers/spacecleanup"
	"github.com/codeready-toolchain/host-operator/controllers/spacecompletion"
	"github.com/codeready-toolchain/host-operator/controllers/spaceprovisionerconfig"
	"github.com/codeready-toolchain/host-operator/controllers/spacerequest"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainstatus"
	"github.com/codeready-toolchain/host-operator/controllers/usersignup"
	"github.com/codeready-toolchain/host-operator/controllers/usersignupcleanup"
	"github.com/codeready-toolchain/host-operator/deploy"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/host-operator/pkg/capacity"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/segment"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	"github.com/codeready-toolchain/host-operator/pkg/templates/nstemplatetiers"
	"github.com/codeready-toolchain/host-operator/pkg/templates/usertiers"
	"github.com/codeready-toolchain/host-operator/version"
	"github.com/codeready-toolchain/toolchain-common/controllers/toolchaincluster"
	"github.com/codeready-toolchain/toolchain-common/controllers/toolchainclustercache"
	"github.com/codeready-toolchain/toolchain-common/controllers/toolchainclusterresources"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	commoncluster "github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/status"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	klogv1 "k8s.io/klog"
	klogv2 "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
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

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=*,verbs=*
//+kubebuilder:rbac:groups="",resources=secrets;configmaps;services;services/finalizers;serviceaccounts;pods,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=apps,resources=deployments;deployments/finalizers;replicasets,verbs=get;list;watch;update;patch;create;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io;authorization.openshift.io,resources=rolebindings;roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;update;patch;create;delete

func main() { // nolint:gocyclo
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
	ctx := ctrl.SetupSignalHandler()

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

	// create client that will be used for retrieving the host operator secret & ToolchainCluster CRs
	cl, err := runtimeclient.New(cfg, runtimeclient.Options{
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

	// initialize the Segment client
	segmentClient, err := segment.DefaultClient(crtConfig.RegistrationService().Analytics().SegmentWriteKey())
	if err != nil {
		setupLog.Error(err, "unable to init the Segment client")
		os.Exit(1)
	}
	if segmentClient != nil {
		defer func() {
			if err := segmentClient.Close(); err != nil {
				setupLog.Error(err, "error while closing the Segment client")
			}
		}()
	}

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
	memberClusters, err := addMemberClusters(mgr, cl, namespace, true)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err = (&toolchainclusterresources.Reconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Templates: &deploy.ToolchainClusterTemplateFS,
	}).SetupWithManager(mgr, namespace); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainClusterResources")
		os.Exit(1)
	}
	if err = toolchainclustercache.NewReconciler(
		mgr,
		namespace,
		memberClientTimeout,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainClusterCache")
		os.Exit(1)
	}

	if err := (&toolchaincluster.Reconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		RequeAfter: 10 * time.Second,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainCluster")
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
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		Namespace:      namespace,
		MemberClusters: memberClusters,
	}).SetupWithManager(mgr, memberClusters); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MasterUserRecord")
		os.Exit(1)
	}
	if err := (&notification.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, crtConfig); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Notification")
		os.Exit(1)
	}
	if err := (&nstemplatetier.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NSTemplateTier")
		os.Exit(1)
	}
	if err := (&toolchainconfig.Reconciler{
		Client:         mgr.GetClient(),
		GetMembersFunc: commoncluster.GetMemberClusters,
		Scheme:         mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainConfig")
		os.Exit(1)
	}

	if err := (&toolchainstatus.Reconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		HTTPClientImpl:      &http.Client{},
		VersionCheckManager: status.VersionCheckManager{GetGithubClientFunc: commonclient.NewGitHubClient},
		GetMembersFunc:      commoncluster.GetMemberClusters,
		Namespace:           namespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ToolchainStatus")
		os.Exit(1)
	}
	if err := (&usersignup.Reconciler{
		StatusUpdater: &usersignup.StatusUpdater{
			Client: mgr.GetClient(),
		},
		Namespace:      namespace,
		Scheme:         mgr.GetScheme(),
		SegmentClient:  segmentClient,
		ClusterManager: capacity.NewClusterManager(namespace, mgr.GetClient()),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserSignup")
		os.Exit(1)
	}
	if err := (&usersignupcleanup.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserSignupCleanup")
		os.Exit(1)
	}
	// init cluster scoped member cluster clients
	clusterScopedMemberClusters, err := addMemberClusters(mgr, cl, namespace, false)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	// enable space request
	if crtConfig.SpaceConfig().SpaceRequestIsEnabled() {
		if err = (&spacerequest.Reconciler{
			Client:         mgr.GetClient(),
			Namespace:      namespace,
			MemberClusters: clusterScopedMemberClusters,
			Scheme:         mgr.GetScheme(),
		}).SetupWithManager(mgr, clusterScopedMemberClusters); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "SpaceRequest")
			os.Exit(1)
		}
	}

	// enable space binding request
	if crtConfig.SpaceConfig().SpaceBindingRequestIsEnabled() {
		if err = (&spacebindingrequest.Reconciler{
			Client:         mgr.GetClient(),
			Namespace:      namespace,
			MemberClusters: clusterScopedMemberClusters,
			Scheme:         mgr.GetScheme(),
		}).SetupWithManager(mgr, clusterScopedMemberClusters); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "SpaceBindingRequest")
			os.Exit(1)
		}
	}
	if err = (&space.Reconciler{
		Client:         mgr.GetClient(),
		Namespace:      namespace,
		MemberClusters: memberClusters,
	}).SetupWithManager(mgr, memberClusters); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Space")
		os.Exit(1)
	}
	if err = (&spacecompletion.Reconciler{
		Client:         mgr.GetClient(),
		Namespace:      namespace,
		ClusterManager: capacity.NewClusterManager(namespace, mgr.GetClient()),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SpaceCompletion")
		os.Exit(1)
	}
	if err = (&spacebindingcleanup.Reconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		Namespace:      namespace,
		MemberClusters: clusterScopedMemberClusters,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SpaceBindingCleanup")
		os.Exit(1)
	}
	if err = (&spacecleanup.Reconciler{
		Client:    mgr.GetClient(),
		Namespace: namespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SpaceCleanup")
		os.Exit(1)
	}
	if err = (&socialevent.Reconciler{
		Client:    mgr.GetClient(),
		Namespace: namespace,
		StatusUpdater: &socialevent.StatusUpdater{
			Client: mgr.GetClient(),
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SocialEvent")
		os.Exit(1)
	}
	if err = (&spaceprovisionerconfig.Reconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SpaceProvisionerConfig")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	go func() {
		setupLog.Info("Starting cluster health checker & creating/updating the NSTemplateTier resources once cache is sync'd")
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			setupLog.Error(errors.New("timed out waiting for caches to sync"), "")
			os.Exit(1)
		}

		// create or update Toolchain status during the operator deployment
		setupLog.Info("Creating/updating the ToolchainStatus resource")
		if err := toolchainstatus.CreateOrUpdateResources(ctx, mgr.GetClient(), namespace, toolchainconfig.ToolchainStatusName); err != nil {
			setupLog.Error(err, "cannot create/update ToolchainStatus resource")
			os.Exit(1)
		}
		setupLog.Info("Created/updated the ToolchainStatus resource")

		// create or update all NSTemplateTiers on the cluster at startup
		setupLog.Info("Creating/updating the NSTemplateTier resources")
		nstemplatetierAssets := assets.NewAssets(nstemplatetiers.AssetNames, nstemplatetiers.Asset)
		if err := nstemplatetiers.CreateOrUpdateResources(ctx, mgr.GetScheme(), mgr.GetClient(), namespace, nstemplatetierAssets); err != nil {
			setupLog.Error(err, "")
			os.Exit(1)
		}
		setupLog.Info("Created/updated the NSTemplateTier resources")

		// create or update all UserTiers on the cluster at startup
		setupLog.Info("Creating/updating the UserTier resources")
		usertierAssets := assets.NewAssets(usertiers.AssetNames, usertiers.Asset)
		if err := usertiers.CreateOrUpdateResources(ctx, mgr.GetScheme(), mgr.GetClient(), namespace, usertierAssets); err != nil {
			setupLog.Error(err, "")
			os.Exit(1)
		}
		setupLog.Info("Created/updated the UserTier resources")
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
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func addMemberClusters(mgr ctrl.Manager, cl runtimeclient.Client, namespace string, namespacedCache bool) (map[string]cluster.Cluster, error) {
	memberConfigs, err := commoncluster.ListToolchainClusterConfigs(cl, namespace, memberClientTimeout)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get ToolchainCluster configs for members")
	}
	memberClusters := map[string]cluster.Cluster{}
	for _, memberConfig := range memberConfigs {
		setupLog.Info("adding cluster for a member", "name", memberConfig.Name, "apiEndpoint", memberConfig.APIEndpoint)

		memberCluster, err := runtimecluster.New(memberConfig.RestConfig, func(options *runtimecluster.Options) {
			options.Scheme = scheme
			// for some resources like SpaceRequest/SpaceBindingRequest we need the cache to be clustered scoped
			// because those resources are in user namespaces and not member operator namespace.
			if namespacedCache {
				options.Namespace = memberConfig.OperatorNamespace
			}
		})
		if err != nil {
			return nil, errors.Wrapf(err, "unable to create member cluster definition for "+memberConfig.Name)
		}
		if err := mgr.Add(memberCluster); err != nil {
			return nil, errors.Wrapf(err, "unable to add member cluster to the manager for "+memberConfig.Name)
		}
		// These fields need to be set when using the REST client
		memberConfig.RestConfig.ContentConfig = rest.ContentConfig{
			GroupVersion:         &authv1.SchemeGroupVersion,
			NegotiatedSerializer: clientgoscheme.Codecs,
		}
		restClient, err := rest.RESTClientFor(memberConfig.RestConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to create member cluster rest client "+memberConfig.Name)
		}
		memberClusters[memberConfig.Name] = cluster.Cluster{
			Config:     memberConfig,
			Client:     memberCluster.GetClient(),
			RESTClient: restClient,
			Cache:      memberCluster.GetCache(),
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
