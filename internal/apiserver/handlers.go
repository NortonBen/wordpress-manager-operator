package apiserver

import (
	"encoding/json"
	"net/http"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	"github.com/benji/wordpress-manager-operator/internal/resources"

	"github.com/go-chi/chi/v5"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// SiteDTO is the JSON shape exchanged with the UI. It is intentionally smaller
// and flatter than the full CRD.
type SiteDTO struct {
	Name         string   `json:"name"`
	Domain       string   `json:"domain"`
	Aliases      []string `json:"aliases,omitempty"`
	Image        string   `json:"image,omitempty"`
	Replicas     int32    `json:"replicas"`
	TLSEnabled   bool     `json:"tlsEnabled"`
	TLSIssuer    string   `json:"tlsIssuer,omitempty"`
	IngressClass string   `json:"ingressClass,omitempty"`
	TablePrefix  string   `json:"tablePrefix,omitempty"`
	PHPConfig    string   `json:"phpConfig,omitempty"`

	// Read-only status, populated on GET/list.
	Phase        string `json:"phase,omitempty"`
	URL          string `json:"url,omitempty"`
	DatabaseName string `json:"databaseName,omitempty"`
	DatabaseUser string `json:"databaseUser,omitempty"`
}

func toDTO(s *wpv1.WordPressSite) SiteDTO {
	replicas := int32(1)
	if s.Spec.Replicas != nil {
		replicas = *s.Spec.Replicas
	}
	return SiteDTO{
		Name:         s.Name,
		Domain:       s.Spec.Domain,
		Aliases:      s.Spec.Aliases,
		Image:        s.Spec.Image,
		Replicas:     replicas,
		TLSEnabled:   s.Spec.TLS.Enabled,
		TLSIssuer:    s.Spec.TLS.Issuer,
		IngressClass: s.Spec.IngressClassName,
		TablePrefix:  s.Spec.TablePrefix,
		PHPConfig:    s.Spec.PHPConfig,
		Phase:        s.Status.Phase,
		URL:          s.Status.URL,
		DatabaseName: s.Status.DatabaseName,
		DatabaseUser: s.Status.DatabaseUser,
	}
}

func (d SiteDTO) toSite(namespace string) *wpv1.WordPressSite {
	replicas := d.Replicas
	if replicas == 0 {
		replicas = 1
	}
	return &wpv1.WordPressSite{
		ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: namespace},
		Spec: wpv1.WordPressSiteSpec{
			Domain:           d.Domain,
			Aliases:          d.Aliases,
			Image:            d.Image,
			Replicas:         &replicas,
			IngressClassName: d.IngressClass,
			TablePrefix:      d.TablePrefix,
			PHPConfig:        d.PHPConfig,
			TLS:              wpv1.TLSSpec{Enabled: d.TLSEnabled, Issuer: d.TLSIssuer},
		},
	}
}

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	list := &wpv1.WordPressSiteList{}
	if err := s.K8s.List(r.Context(), list, client.InNamespace(s.Namespace)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]SiteDTO, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, toDTO(&list.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request) {
	site, err := s.fetch(r)
	if err != nil {
		notFoundOr500(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDTO(site))
}

func (s *Server) createSite(w http.ResponseWriter, r *http.Request) {
	var dto SiteDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if dto.Name == "" || dto.Domain == "" {
		writeError(w, http.StatusBadRequest, "name and domain are required")
		return
	}
	site := dto.toSite(s.Namespace)
	if err := s.K8s.Create(r.Context(), site); err != nil {
		if apierrors.IsAlreadyExists(err) {
			writeError(w, http.StatusConflict, "site already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Dev mode: reconcile in-process so the site immediately gains status.
	s.maybeReconcile(r.Context(), site.Name)
	if s.Reconcile != nil {
		_ = s.K8s.Get(r.Context(), client.ObjectKeyFromObject(site), site)
	}
	writeJSON(w, http.StatusCreated, toDTO(site))
}

func (s *Server) deleteSite(w http.ResponseWriter, r *http.Request) {
	site, err := s.fetch(r)
	if err != nil {
		notFoundOr500(w, err)
		return
	}
	if err := s.K8s.Delete(r.Context(), site); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Dev mode: process the finalizer in-process so the delete completes.
	s.maybeReconcile(r.Context(), site.Name)
	w.WriteHeader(http.StatusNoContent)
}

// previewYAML renders the exact manifests the operator would generate for a
// (possibly not-yet-created) site — powering the "customise via YAML" feature.
func (s *Server) previewYAML(w http.ResponseWriter, r *http.Request) {
	var dto SiteDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	site := dto.toSite(s.Namespace)
	// Stamp apiVersion/kind so the preview is valid, directly-applyable YAML
	// (typed objects carry no TypeMeta until they pass through the API server).
	site.TypeMeta = metav1.TypeMeta{APIVersion: wpv1.GroupVersion.String(), Kind: "WordPressSite"}
	dep := resources.DesiredDeployment(site, s.DBHost, s.DBPort)
	dep.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}
	svc := resources.DesiredService(site)
	svc.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Service"}
	ing := resources.DesiredIngress(site)
	ing.TypeMeta = metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"}

	docs := [][]byte{}
	for _, obj := range []any{site, dep, svc, ing} {
		b, err := yaml.Marshal(obj)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		docs = append(docs, b)
	}
	w.Header().Set("Content-Type", "application/x-yaml")
	for i, d := range docs {
		if i > 0 {
			_, _ = w.Write([]byte("---\n"))
		}
		_, _ = w.Write(d)
	}
}

func (s *Server) fetch(r *http.Request) (*wpv1.WordPressSite, error) {
	name := chi.URLParam(r, "name")
	site := &wpv1.WordPressSite{}
	err := s.K8s.Get(r.Context(), client.ObjectKey{Namespace: s.Namespace, Name: name}, site)
	return site, err
}

// Ensure these k8s types are linked for scheme registration in server.go.
var _ = appsv1.Deployment{}
var _ = corev1.Service{}
var _ = netv1.Ingress{}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func notFoundOr500(w http.ResponseWriter, err error) {
	if apierrors.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
