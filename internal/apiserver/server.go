package apiserver

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Server hosts the admin REST API consumed by the React UI.
type Server struct {
	K8s       client.Client
	Auth      *Authenticator
	Namespace string // namespace WordPressSites live in
	DBHost    string
	DBPort    string
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
		pr.Delete("/api/v1/sites/{name}", s.deleteSite)
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
