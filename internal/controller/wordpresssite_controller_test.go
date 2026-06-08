package controller

import (
	"context"
	"testing"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// stubDB records provisioning calls so the reconcile can be tested without a
// real database server — the same mock the dev mode relies on.
type stubDB struct {
	ensured        bool
	dropped        bool
	db, user, pass string
}

func (s *stubDB) EnsureDatabase(_ context.Context, db, user, _ /*host*/, pass string) error {
	s.ensured, s.db, s.user, s.pass = true, db, user, pass
	return nil
}
func (s *stubDB) DropDatabase(_ context.Context, _, _, _ string) error {
	s.dropped = true
	return nil
}

func newReconciler(t *testing.T, objs ...client.Object) (*WordPressSiteReconciler, *stubDB, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := wpv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&wpv1.WordPressSite{}).
		WithObjects(objs...).
		Build()
	db := &stubDB{}
	r := &WordPressSiteReconciler{
		Client: fc, Scheme: scheme, DB: db,
		DBHost: "mysql", DBPort: "3306",
		NoServerSideApply: true, // exercise the fake-client path (dev mode)
	}
	return r, db, fc
}

func reconcile(t *testing.T, r *WordPressSiteReconciler, name string) {
	t.Helper()
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "wordpress-sites", Name: name},
	})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

func sample() *wpv1.WordPressSite {
	return &wpv1.WordPressSite{
		ObjectMeta: metav1.ObjectMeta{Name: "test-site", Namespace: "wordpress-sites"},
		Spec:       wpv1.WordPressSiteSpec{Domain: "test.example", TLS: wpv1.TLSSpec{Enabled: true, Issuer: "le"}},
	}
}

func TestReconcileCreatesEverything(t *testing.T) {
	r, db, c := newReconciler(t, sample())
	reconcile(t, r, "test-site")

	ctx := context.Background()
	get := func(o client.Object) error {
		return c.Get(ctx, client.ObjectKey{Namespace: "wordpress-sites", Name: "test-site"}, o)
	}
	if err := get(&appsv1.Deployment{}); err != nil {
		t.Errorf("Deployment not created: %v", err)
	}
	if err := get(&corev1.Service{}); err != nil {
		t.Errorf("Service not created: %v", err)
	}
	if err := get(&netv1.Ingress{}); err != nil {
		t.Errorf("Ingress not created: %v", err)
	}
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "wordpress-sites", Name: "test-site-wp"}, secret); err != nil {
		t.Errorf("Secret not created: %v", err)
	}

	// Database provisioned with the generated name/user and the secret's password.
	if !db.ensured || db.db != "wp_test_site" || db.user != "wpu_test_site" {
		t.Errorf("unexpected DB provisioning: %+v", db)
	}
	if got := string(secret.Data["WORDPRESS_DB_PASSWORD"]); got == "" || got != db.pass {
		t.Errorf("secret password not passed to provisioner")
	}

	// Status reflects Ready.
	site := &wpv1.WordPressSite{}
	_ = get(site)
	if site.Status.Phase != wpv1.PhaseReady || site.Status.URL != "https://test.example" {
		t.Errorf("status not Ready: %+v", site.Status)
	}
}

func TestReconcileIsIdempotent(t *testing.T) {
	r, _, c := newReconciler(t, sample())
	reconcile(t, r, "test-site")
	dep1 := &appsv1.Deployment{}
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "wordpress-sites", Name: "test-site"}, dep1)

	// Second reconcile must not error and must not duplicate/drift resources.
	reconcile(t, r, "test-site")
	dep2 := &appsv1.Deployment{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "wordpress-sites", Name: "test-site"}, dep2); err != nil {
		t.Fatalf("deployment missing after 2nd reconcile: %v", err)
	}
}

func TestReconcileDeleteDropsData(t *testing.T) {
	site := sample()
	now := metav1.Now()
	site.DeletionTimestamp = &now
	site.Finalizers = []string{wpv1.Finalizer}

	r, db, c := newReconciler(t, site)
	r.DropDataOnDelete = true
	reconcile(t, r, "test-site")

	if !db.dropped {
		t.Error("expected DropDatabase to be called")
	}
	// Finalizer removed → object gone.
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "wordpress-sites", Name: "test-site"}, &wpv1.WordPressSite{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected site removed after finalizer cleared, got %v", err)
	}
}
