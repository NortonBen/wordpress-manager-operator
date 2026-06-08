package resources

import (
	"crypto/rand"
	"encoding/base64"
	"maps"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WordPress security keys/salts injected into the container. The official
// image substitutes these into wp-config.php.
var saltKeys = []string{
	"WORDPRESS_AUTH_KEY",
	"WORDPRESS_SECURE_AUTH_KEY",
	"WORDPRESS_LOGGED_IN_KEY",
	"WORDPRESS_NONCE_KEY",
	"WORDPRESS_AUTH_SALT",
	"WORDPRESS_SECURE_AUTH_SALT",
	"WORDPRESS_LOGGED_IN_SALT",
	"WORDPRESS_NONCE_SALT",
}

// SecretKeyDBPassword is the data key holding the generated DB password.
const SecretKeyDBPassword = "WORDPRESS_DB_PASSWORD"

// RandomPassword returns a URL-safe random secret of the given byte length.
func RandomPassword(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DesiredSecret builds the per-site Secret. When an existing secret is passed,
// its password and salts are preserved so reconciliation is idempotent and does
// not rotate live credentials on every loop.
func DesiredSecret(site *wpv1.WordPressSite, existing *corev1.Secret) *corev1.Secret {
	data := map[string][]byte{}
	if existing != nil {
		maps.Copy(data, existing.Data)
	}

	if len(data[SecretKeyDBPassword]) == 0 {
		data[SecretKeyDBPassword] = []byte(RandomPassword(24))
	}
	for _, k := range saltKeys {
		if len(data[k]) == 0 {
			data[k] = []byte(RandomPassword(48))
		}
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(site),
			Namespace: site.Namespace,
			Labels:    Labels(site),
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

// SaltKeys exposes the salt env-var names for the Deployment builder.
func SaltKeys() []string { return saltKeys }
