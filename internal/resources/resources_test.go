package resources

import (
	"testing"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func site(name string) *wpv1.WordPressSite {
	return &wpv1.WordPressSite{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "wordpress-sites"},
		Spec:       wpv1.WordPressSiteSpec{Domain: name + ".example"},
	}
}

func TestNaming(t *testing.T) {
	s := site("blog-acme")
	if got := DatabaseName(s); got != "wp_blog_acme" {
		t.Errorf("DatabaseName = %q, want wp_blog_acme", got)
	}
	if got := DatabaseUser(s); got != "wpu_blog_acme" {
		t.Errorf("DatabaseUser = %q, want wpu_blog_acme", got)
	}
	if got := SubPath(s); got != "blog-acme" {
		t.Errorf("SubPath = %q, want blog-acme", got)
	}
}

func TestDatabaseUserTruncation(t *testing.T) {
	// MySQL user names cap at 32 chars.
	s := site("a-very-long-tenant-site-name-that-exceeds-limits")
	if got := DatabaseUser(s); len(got) > 32 {
		t.Errorf("DatabaseUser len = %d (%q), want <= 32", len(got), got)
	}
}

func TestDeploymentSharedVolumeSubPath(t *testing.T) {
	s := site("shop-foo")
	dep := DesiredDeployment(s, "mysql", "3306")

	c := dep.Spec.Template.Spec.Containers[0]
	if len(c.VolumeMounts) != 1 || c.VolumeMounts[0].SubPath != "shop-foo" {
		t.Fatalf("expected single mount with subPath=shop-foo, got %+v", c.VolumeMounts)
	}
	vol := dep.Spec.Template.Spec.Volumes[0]
	if vol.PersistentVolumeClaim == nil || vol.PersistentVolumeClaim.ClaimName != DefaultSharedPVCName {
		t.Fatalf("expected shared PVC %q, got %+v", DefaultSharedPVCName, vol.VolumeSource)
	}

	// DB env wired from the per-site secret.
	var hasPassFromSecret bool
	for _, e := range c.Env {
		if e.Name == "WORDPRESS_DB_PASSWORD" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			hasPassFromSecret = true
		}
	}
	if !hasPassFromSecret {
		t.Error("WORDPRESS_DB_PASSWORD should come from the per-site Secret")
	}
}

func TestSecretPreservesExistingPassword(t *testing.T) {
	s := site("blog-acme")
	first := DesiredSecret(s, nil)
	pass := first.Data[SecretKeyDBPassword]
	if len(pass) == 0 {
		t.Fatal("expected a generated password")
	}
	// Second reconcile with the existing secret must keep the same password.
	second := DesiredSecret(s, first)
	if string(second.Data[SecretKeyDBPassword]) != string(pass) {
		t.Error("password rotated across reconciles; should be stable")
	}
	for _, k := range SaltKeys() {
		if len(second.Data[k]) == 0 {
			t.Errorf("salt %s missing", k)
		}
	}
}

func TestIngressBindsDomainAndAliasesWithTLS(t *testing.T) {
	s := site("blog-acme")
	s.Spec.Aliases = []string{"www.blog-acme.example"}
	s.Spec.TLS = wpv1.TLSSpec{Enabled: true, Issuer: "letsencrypt-prod"}

	ing := DesiredIngress(s)
	if len(ing.Spec.Rules) != 2 {
		t.Fatalf("expected 2 ingress rules (domain + alias), got %d", len(ing.Spec.Rules))
	}
	if len(ing.Spec.TLS) != 1 || ing.Spec.TLS[0].SecretName != "blog-acme-tls" {
		t.Fatalf("expected TLS secret blog-acme-tls, got %+v", ing.Spec.TLS)
	}
	if ing.Annotations["cert-manager.io/cluster-issuer"] != "letsencrypt-prod" {
		t.Error("expected cert-manager issuer annotation")
	}
}
