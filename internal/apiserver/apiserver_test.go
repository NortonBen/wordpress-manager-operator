package apiserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	"github.com/benji/wordpress-manager-operator/internal/metrics"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newTestServer wires the API server against a MOCK cluster (controller-runtime
// fake client) so the whole admin flow runs with no real Kubernetes / MySQL.
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := wpv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()

	auth, err := NewAuthenticator("admin", "", "s3cret", "test-jwt-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{
		K8s: k8s, Auth: auth, Namespace: "wordpress-sites",
		DBHost: "mysql", DBPort: "3306",
		Metrics: metrics.NewDevProvider(k8s, "wordpress-sites"),
	}
	return srv.Router([]string{"*"})
}

func req(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func loginToken(t *testing.T, h http.Handler) string {
	t.Helper()
	w := req(t, h, "POST", "/api/v1/login", "", `{"username":"admin","password":"s3cret"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("login: got %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("login: no token in response: %s", w.Body.String())
	}
	return out.Token
}

func TestLogin(t *testing.T) {
	h := newTestServer(t)

	if w := req(t, h, "POST", "/api/v1/login", "", `{"username":"admin","password":"wrong"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("bad password: got %d, want 401", w.Code)
	}
	if tok := loginToken(t, h); tok == "" {
		t.Error("expected a JWT")
	}
}

func TestSitesRequireAuth(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/api/v1/sites", "/api/v1/me"} {
		if w := req(t, h, "GET", path, "", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without token: got %d, want 401", path, w.Code)
		}
	}
	if w := req(t, h, "GET", "/api/v1/sites", "Bearer garbage", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: got %d, want 401", w.Code)
	}
}

// TestAdminFlow exercises the full happy path through the mocked cluster.
func TestAdminFlow(t *testing.T) {
	h := newTestServer(t)
	tok := loginToken(t, h)

	// Initially empty.
	w := req(t, h, "GET", "/api/v1/sites", tok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: got %d", w.Code)
	}
	var list []SiteDTO
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Fatalf("expected 0 sites, got %d", len(list))
	}

	// Create.
	create := `{"name":"blog-acme","domain":"blog.acme.example","replicas":2,"tlsEnabled":true,"tlsIssuer":"letsencrypt-prod"}`
	w = req(t, h, "POST", "/api/v1/sites", tok, create)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d, want 201 (body=%s)", w.Code, w.Body.String())
	}

	// Duplicate create -> 409.
	if w := req(t, h, "POST", "/api/v1/sites", tok, create); w.Code != http.StatusConflict {
		t.Errorf("duplicate create: got %d, want 409", w.Code)
	}

	// Validation: missing domain -> 400.
	if w := req(t, h, "POST", "/api/v1/sites", tok, `{"name":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("missing domain: got %d, want 400", w.Code)
	}

	// List now has 1.
	w = req(t, h, "GET", "/api/v1/sites", tok, "")
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Domain != "blog.acme.example" || list[0].Replicas != 2 {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Get one.
	w = req(t, h, "GET", "/api/v1/sites/blog-acme", tok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get: got %d", w.Code)
	}
	var got SiteDTO
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if !got.TLSEnabled || got.TLSIssuer != "letsencrypt-prod" {
		t.Errorf("get: TLS not round-tripped: %+v", got)
	}

	// Get missing -> 404.
	if w := req(t, h, "GET", "/api/v1/sites/nope", tok, ""); w.Code != http.StatusNotFound {
		t.Errorf("get missing: got %d, want 404", w.Code)
	}

	// Delete -> 204, then list empty.
	if w := req(t, h, "DELETE", "/api/v1/sites/blog-acme", tok, ""); w.Code != http.StatusNoContent {
		t.Errorf("delete: got %d, want 204", w.Code)
	}
	w = req(t, h, "GET", "/api/v1/sites", tok, "")
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("after delete expected 0 sites, got %d", len(list))
	}
}

func TestMetrics(t *testing.T) {
	h := newTestServer(t)
	tok := loginToken(t, h)

	// Create a site so per-site usage shows up.
	if w := req(t, h, "POST", "/api/v1/sites", tok,
		`{"name":"blog-acme","domain":"blog.acme.example","replicas":2}`); w.Code != http.StatusCreated {
		t.Fatalf("create: got %d", w.Code)
	}

	w := req(t, h, "GET", "/api/v1/metrics", tok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("metrics: got %d", w.Code)
	}
	var out struct {
		Cluster struct {
			CPU    metrics.Metric `json:"cpu"`
			Memory metrics.Metric `json:"memory"`
		} `json:"cluster"`
		Sites []metrics.SiteUsage `json:"sites"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if out.Cluster.CPU.Capacity <= 0 || out.Cluster.Memory.Capacity <= 0 {
		t.Errorf("expected positive capacity, got %+v", out.Cluster)
	}
	// Available = allocatable - used, must be consistent and non-negative.
	if out.Cluster.CPU.Available != out.Cluster.CPU.Allocatable-out.Cluster.CPU.Used {
		t.Errorf("cpu available mismatch: %+v", out.Cluster.CPU)
	}
	if len(out.Sites) != 1 || out.Sites[0].Name != "blog-acme" || out.Sites[0].CPUMilli <= 0 {
		t.Errorf("expected blog-acme usage, got %+v", out.Sites)
	}
}

func TestPreviewYAML(t *testing.T) {
	h := newTestServer(t)
	tok := loginToken(t, h)

	w := req(t, h, "POST", "/api/v1/sites/preview", tok,
		`{"name":"shop-foo","domain":"shop.foo.example","tlsEnabled":true,"tlsIssuer":"le"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("preview: got %d", w.Code)
	}
	out := w.Body.String()
	for _, want := range []string{"kind: WordPressSite", "kind: Deployment", "kind: Service", "kind: Ingress", "shop.foo.example"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q in:\n%s", want, out)
		}
	}
}
