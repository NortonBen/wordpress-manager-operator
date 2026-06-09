package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/benji/wordpress-manager-operator/internal/metrics"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileFunc reconciles a single site by namespace/name. In production the
// operator does this out-of-process and this is nil; in local dev mode the API
// server runs the reconciler in-process against a mock cluster.
type ReconcileFunc func(ctx context.Context, namespace, name string) error

// Server hosts the admin REST API consumed by the React UI.
type Server struct {
	K8s       client.Client
	Auth      *Authenticator
	Namespace string // namespace WordPressSites live in
	DBHost    string
	DBPort    string
	Metrics   metrics.Provider // cluster/site CPU+RAM usage

	// Reconcile, when set (dev mode), is invoked after create/delete so the
	// mock cluster converges immediately without a separate operator process.
	Reconcile ReconcileFunc
}

func (s *Server) maybeReconcile(ctx context.Context, name string) {
	if s.Reconcile != nil {
		_ = s.Reconcile(ctx, s.Namespace, name)
	}
}

// Router builds the HTTP handler with auth, CORS and routes.
func (s *Server) Router(corsOrigins []string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	r.Post("/api/v1/login", s.login)

	// Authenticated routes.
	r.Group(func(pr chi.Router) {
		pr.Use(s.Auth.Middleware)
		pr.Get("/api/v1/sites", s.listSites)
		pr.Post("/api/v1/sites", s.createSite)
		pr.Post("/api/v1/sites/preview", s.previewYAML)
		pr.Get("/api/v1/sites/{name}", s.getSite)
		pr.Put("/api/v1/sites/{name}", s.updateSite)
		pr.Delete("/api/v1/sites/{name}", s.deleteSite)
		pr.Get("/api/v1/sites/{name}/yaml", s.getSiteYAML)
		pr.Put("/api/v1/sites/{name}/yaml", s.updateSiteYAML)
		pr.Post("/api/v1/sites/{name}/suspend", s.setSuspend(true))
		pr.Post("/api/v1/sites/{name}/resume", s.setSuspend(false))
		pr.Get("/api/v1/metrics", s.getMetrics)
		pr.Get("/api/v1/me", s.me)
	})
	return r
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	token, err := s.Auth.Login(body.Username, body.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userCtxKey).(string)
	writeJSON(w, http.StatusOK, map[string]string{"username": user})
}

// getMetrics returns cluster CPU/RAM (used, capacity, allocatable, remaining)
// plus per-site usage, powering the dashboard resource cards.
func (s *Server) getMetrics(w http.ResponseWriter, r *http.Request) {
	if s.Metrics == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics not configured")
		return
	}
	cluster, err := s.Metrics.Cluster(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sites, err := s.Metrics.Sites(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sites == nil {
		sites = []metrics.SiteUsage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cluster": cluster, "sites": sites})
}
