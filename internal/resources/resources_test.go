package resources

import (
	"strings"
	"testing"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	var siteMount *corev1.VolumeMount
	for i := range c.VolumeMounts {
		if c.VolumeMounts[i].Name == "site-data" {
			siteMount = &c.VolumeMounts[i]
		}
	}
	if siteMount == nil || siteMount.SubPath != "shop-foo" {
		t.Fatalf("expected site-data mount with subPath=shop-foo, got %+v", c.VolumeMounts)
	}
	var siteVol *corev1.Volume
	for i := range dep.Spec.Template.Spec.Volumes {
		if dep.Spec.Template.Spec.Volumes[i].Name == "site-data" {
			siteVol = &dep.Spec.Template.Spec.Volumes[i]
		}
	}
	if siteVol == nil || siteVol.PersistentVolumeClaim == nil || siteVol.PersistentVolumeClaim.ClaimName != DefaultSharedPVCName {
		t.Fatalf("expected shared PVC %q for site-data", DefaultSharedPVCName)
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

func TestPHPIniMountedAndHashed(t *testing.T) {
	s := site("blog-acme")

	// No custom php.ini → the DEFAULT php.ini is applied (always mounted).
	if EffectivePHPIni(s) != DefaultPHPIni {
		t.Fatal("empty phpIni should fall back to DefaultPHPIni")
	}
	cm := DesiredPHPConfigMap(s)
	if cm.Name != "blog-acme-php" || cm.Data[PHPIniFileName] != DefaultPHPIni {
		t.Fatalf("configmap should carry the default php.ini: %+v", cm.Data)
	}

	dep := DesiredDeployment(s, "mysql", "3306")
	c := dep.Spec.Template.Spec.Containers[0]
	var mounted bool
	for _, vm := range c.VolumeMounts {
		if vm.Name == "php-config" && vm.MountPath == PHPMountPath && vm.SubPath == PHPIniFileName {
			mounted = true
		}
	}
	if !mounted {
		t.Errorf("php.ini not mounted into conf.d: %+v", c.VolumeMounts)
	}
	defaultHash := dep.Spec.Template.ObjectMeta.Annotations["wp.benji.dev/php-ini-hash"]
	if defaultHash == "" {
		t.Error("expected php-ini-hash pod annotation")
	}

	// Custom php.ini overrides the default and changes the hash → rollout.
	s.Spec.PHPIni = "memory_limit = 1024M\n"
	if EffectivePHPIni(s) != s.Spec.PHPIni {
		t.Error("custom phpIni should override the default")
	}
	dep2 := DesiredDeployment(s, "mysql", "3306")
	if dep2.Spec.Template.ObjectMeta.Annotations["wp.benji.dev/php-ini-hash"] == defaultHash {
		t.Error("hash should change when php.ini changes")
	}
}

func configExtra(dep *appsv1.Deployment) (string, bool) {
	for _, e := range dep.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "WORDPRESS_CONFIG_EXTRA" {
			return e.Value, true
		}
	}
	return "", false
}

func hasForwardedMount(dep *appsv1.Deployment) bool {
	for _, vm := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
		if vm.MountPath == ForwardedProtoMount && vm.SubPath == ForwardedProtoFile {
			return true
		}
	}
	return false
}

func TestForceHTTPSInsertsAfterPhpTag(t *testing.T) {
	s := site("blog-acme")
	if !ForceHTTPSEnabled(s) {
		t.Error("ForceHTTPS should default to true")
	}

	dep := DesiredDeployment(s, "mysql", "3306")
	c := dep.Spec.Template.Spec.Containers[0]

	// A postStart hook seds the line into wp-config right after <?php.
	if c.Lifecycle == nil || c.Lifecycle.PostStart == nil || c.Lifecycle.PostStart.Exec == nil {
		t.Fatal("expected a postStart hook when forceHTTPS is on")
	}
	cmd := strings.Join(c.Lifecycle.PostStart.Exec.Command, " ")
	if !strings.Contains(cmd, "wp-config.php") || !strings.Contains(cmd, "sed") || !strings.Contains(cmd, "<?php") {
		t.Errorf("postStart should sed the snippet after <?php: %q", cmd)
	}
	if !hasForwardedMount(dep) {
		t.Error("forwarded-proto.php file not mounted")
	}
	// The snippet lives in the per-site ConfigMap.
	cm := DesiredPHPConfigMap(s)
	if !strings.Contains(cm.Data[ForwardedProtoFile], "HTTP_X_FORWARDED_PROTO") {
		t.Errorf("ConfigMap missing forwarded snippet: %q", cm.Data[ForwardedProtoFile])
	}
	// It must NOT be in WORDPRESS_CONFIG_EXTRA anymore.
	if v, ok := configExtra(dep); ok && strings.Contains(v, "HTTP_X_FORWARDED_PROTO") {
		t.Errorf("forceHTTPS should not be in CONFIG_EXTRA: %q", v)
	}

	// phpConfig still goes to CONFIG_EXTRA (without the forceHTTPS line).
	s.Spec.PHPConfig = "define('WP_DEBUG', true);"
	v, ok := configExtra(DesiredDeployment(s, "mysql", "3306"))
	if !ok || !strings.Contains(v, "WP_DEBUG") || strings.Contains(v, "HTTP_X_FORWARDED_PROTO") {
		t.Errorf("CONFIG_EXTRA should be phpConfig only, got %q", v)
	}

	// Explicitly OFF → no hook, no mount.
	off := false
	s2 := site("shop-foo")
	s2.Spec.ForceHTTPS = &off
	dep2 := DesiredDeployment(s2, "mysql", "3306")
	if dep2.Spec.Template.Spec.Containers[0].Lifecycle != nil {
		t.Error("no lifecycle hook expected when forceHTTPS off")
	}
	if hasForwardedMount(dep2) {
		t.Error("no forwarded-proto mount expected when forceHTTPS off")
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
