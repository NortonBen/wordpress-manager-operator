package resources

import (
	"crypto/sha256"
	"encoding/hex"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PHPIniFileName is the file the operator drops into PHP's conf.d scan dir.
const PHPIniFileName = "zz-wpmgr.ini"

// PHPMountPath is where the php.ini override is mounted in the official
// php-apache WordPress image (its additional-ini scan directory).
const PHPMountPath = "/usr/local/etc/php/conf.d/" + PHPIniFileName

// DefaultPHPIni is applied to every host unless spec.phpIni overrides it.
const DefaultPHPIni = `file_uploads = On
memory_limit = 256M
upload_max_filesize = 500M
post_max_size = 500M
max_execution_time = 300

; Ensure mysqli is enabled
extension=mysqli
`

// PHPConfigMapName is the per-site ConfigMap holding the php.ini.
func PHPConfigMapName(site *wpv1.WordPressSite) string {
	return site.Name + "-php"
}

// EffectivePHPIni returns the site's custom php.ini, or the default when unset.
func EffectivePHPIni(site *wpv1.WordPressSite) string {
	if site.Spec.PHPIni != "" {
		return site.Spec.PHPIni
	}
	return DefaultPHPIni
}

// PHPIniHash is a short content hash used as a pod annotation so the Deployment
// rolls out (and PHP reloads) whenever the effective php.ini changes.
func PHPIniHash(site *wpv1.WordPressSite) string {
	sum := sha256.Sum256([]byte(EffectivePHPIni(site)))
	return hex.EncodeToString(sum[:])[:16]
}

// DesiredPHPConfigMap builds the ConfigMap carrying the site's effective php.ini.
func DesiredPHPConfigMap(site *wpv1.WordPressSite) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PHPConfigMapName(site),
			Namespace: site.Namespace,
			Labels:    Labels(site),
		},
		Data: map[string]string{
			PHPIniFileName: EffectivePHPIni(site),
			// Read by the forceHTTPS postStart hook (inserted after <?php). The
			// trailing marker comment makes the insert idempotent without keying
			// on "HTTP_X_FORWARDED_PROTO" (which the image's wp-config already has).
			ForwardedProtoFile: ForceHTTPSSnippet + " // " + forwardedProtoMarker + "\n",
		},
	}
}
