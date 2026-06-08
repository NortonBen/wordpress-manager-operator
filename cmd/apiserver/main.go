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

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Fatalf("load kubeconfig: %v", err)
	}
	k8s, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("create k8s client: %v", err)
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
		Namespace: env("SITES_NAMESPACE", "wordpress-sites"),
		DBHost:    env("MYSQL_HOST", "mysql"),
		DBPort:    env("MYSQL_PORT", "3306"),
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
