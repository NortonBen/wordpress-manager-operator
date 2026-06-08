// Command operator runs the WordPressSite controller manager.
package main

import (
	"os"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	"github.com/benji/wordpress-manager-operator/internal/controller"
	"github.com/benji/wordpress-manager-operator/internal/mysql"
	"github.com/benji/wordpress-manager-operator/internal/resources"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = wpv1.AddToScheme(scheme)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(env("LOG_DEV", "false") == "true")))
	setupLog := ctrl.Log.WithName("setup")

	// Operator-wide defaults sourced from the environment / Helm values.
	resources.DefaultSharedPVCName = env("SHARED_PVC_NAME", "wordpress-shared")
	resources.DefaultIngressClass = env("INGRESS_CLASS", "nginx")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: env("METRICS_ADDR", ":8080")},
		HealthProbeBindAddress: env("HEALTH_ADDR", ":8081"),
		LeaderElection:         env("LEADER_ELECT", "true") == "true",
		LeaderElectionID:       "wordpress-manager-operator",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	reconciler := &controller.WordPressSiteReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		DB: mysql.New(mysql.Config{
			Host:     env("MYSQL_HOST", "mysql"),
			Port:     env("MYSQL_PORT", "3306"),
			User:     env("MYSQL_ADMIN_USER", "root"),
			Password: os.Getenv("MYSQL_ADMIN_PASSWORD"),
		}),
		DBHost:           env("MYSQL_HOST", "mysql"),
		DBPort:           env("MYSQL_PORT", "3306"),
		DropDataOnDelete: env("DROP_DATA_ON_DELETE", "false") == "true",
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WordPressSite")
		os.Exit(1)
	}

	_ = mgr.AddHealthzCheck("healthz", healthz.Ping)
	_ = mgr.AddReadyzCheck("readyz", healthz.Ping)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
