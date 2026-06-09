// Command apiserver runs the admin REST API consumed by the React UI.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	"github.com/benji/wordpress-manager-operator/internal/apiserver"
	"github.com/benji/wordpress-manager-operator/internal/controller"
	"github.com/benji/wordpress-manager-operator/internal/metrics"
	"github.com/benji/wordpress-manager-operator/internal/sqlite"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = wpv1.AddToScheme(scheme)
	_ = metricsv1beta1.AddToScheme(scheme) // metrics.k8s.io for resource usage

	namespace := env("SITES_NAMESPACE", "wordpress-sites")
	dbHost := env("MYSQL_HOST", "mysql")
	dbPort := env("MYSQL_PORT", "3306")

	var k8s client.Client
	var reconcile apiserver.ReconcileFunc
	var metricsProvider metrics.Provider

	if env("DEV_MODE", "") == "true" {
		// Dev mock: in-memory Kubernetes + SQLite, no real cluster / MySQL.
		sqliteDir := env("SQLITE_DIR", "./.dev/sqlite")
		prov, perr := sqlite.New(sqliteDir)
		if perr != nil {
			log.Fatalf("sqlite provisioner: %v", perr)
		}
		fc := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&wpv1.WordPressSite{}).
			Build()
		k8s = fc
		rec := &controller.WordPressSiteReconciler{
			Client: fc, Scheme: scheme, DB: prov,
			DBHost: "127.0.0.1", DBPort: dbPort,
			NoServerSideApply: true, // fake client doesn't support SSA
		}
		reconcile = func(ctx context.Context, ns, name string) error {
			_, rerr := rec.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: ns, Name: name},
			})
			return rerr
		}
		metricsProvider = metrics.NewDevProvider(fc, namespace)
		log.Printf("DEV MODE: mock Kubernetes (in-memory) + SQLite at %s", sqliteDir)
	} else {
		cfg, cerr := ctrl.GetConfig()
		if cerr != nil {
			log.Fatalf("load kubeconfig: %v", cerr)
		}
		c, cerr := client.New(cfg, client.Options{Scheme: scheme})
		if cerr != nil {
			log.Fatalf("create k8s client: %v", cerr)
		}
		k8s = c
		metricsProvider = metrics.NewKubeProvider(c, namespace)
	}

	auth, err := apiserver.NewAuthenticator(
		env("ADMIN_USERNAME", "admin"),
		os.Getenv("ADMIN_PASSWORD_HASH"),
		os.Getenv("ADMIN_PASSWORD"),
		os.Getenv("JWT_SECRET"),
		24*time.Hour,
	)
	if err != nil {
		log.Fatalf("auth setup: %v", err)
	}

	srv := &apiserver.Server{
		K8s:       k8s,
		Auth:      auth,
		Namespace: namespace,
		DBHost:    dbHost,
		DBPort:    dbPort,
		Reconcile: reconcile,
		Metrics:   metricsProvider,
		TwoFA: &apiserver.KubeTwoFA{
			K8s:        k8s,
			Namespace:  env("TWOFA_NAMESPACE", "wordpress-system"),
			SecretName: env("TWOFA_SECRET_NAME", "wordpress-admin-2fa"),
			Issuer:     env("TWOFA_ISSUER", "WordPress Manager"),
			Account:    env("ADMIN_USERNAME", "admin"),
		},
	}

	origins := strings.Split(env("CORS_ORIGINS", "*"), ",")
	addr := env("API_ADDR", ":8090")

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Router(origins),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("admin API listening on %s (namespace=%s)", addr, srv.Namespace)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := ctrl.SetupSignalHandler()
	<-stop.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}
