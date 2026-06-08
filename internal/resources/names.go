// Package resources builds the Kubernetes objects that back a WordPressSite.
package resources

import (
	"fmt"
	"regexp"
	"strings"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
)

var invalid = regexp.MustCompile(`[^a-z0-9_]`)

// DefaultSharedPVCName is the operator-wide ReadWriteMany claim every site
// mounts unless it overrides spec.storage.sharedPVCName. Set at startup.
var DefaultSharedPVCName = "wordpress-shared"

// SharedPVCName returns the shared volume claim a site mounts.
func SharedPVCName(site *wpv1.WordPressSite) string {
	if site.Spec.Storage.SharedPVCName != "" {
		return site.Spec.Storage.SharedPVCName
	}
	return DefaultSharedPVCName
}

// sanitize turns an arbitrary site name into a MySQL-identifier-safe token.
func sanitize(name string) string {
	s := invalid.ReplaceAllString(strings.ToLower(name), "_")
	return strings.Trim(s, "_")
}

// DatabaseName returns the per-site database name.
func DatabaseName(site *wpv1.WordPressSite) string {
	if site.Spec.Database.Name != "" {
		return site.Spec.Database.Name
	}
	n := "wp_" + sanitize(site.Name)
	return truncate(n, 64)
}

// DatabaseUser returns the per-site, least-privilege database user.
// MySQL caps user names at 32 characters.
func DatabaseUser(site *wpv1.WordPressSite) string {
	if site.Spec.Database.User != "" {
		return site.Spec.Database.User
	}
	return truncate("wpu_"+sanitize(site.Name), 32)
}

// DatabaseHost returns the host pattern the user may connect from.
func DatabaseHost(site *wpv1.WordPressSite) string {
	if site.Spec.Database.Host != "" {
		return site.Spec.Database.Host
	}
	return "%"
}

// SecretName is the per-site Secret holding DB credentials and WP salts.
func SecretName(site *wpv1.WordPressSite) string {
	return site.Name + "-wp"
}

// SubPath is the folder inside the shared volume used for this site.
func SubPath(site *wpv1.WordPressSite) string {
	if site.Spec.Storage.SubPath != "" {
		return site.Spec.Storage.SubPath
	}
	return site.Name
}

// TLSSecretName is the Ingress TLS secret.
func TLSSecretName(site *wpv1.WordPressSite) string {
	if site.Spec.TLS.SecretName != "" {
		return site.Spec.TLS.SecretName
	}
	return site.Name + "-tls"
}

// Hosts returns the primary domain plus any aliases.
func Hosts(site *wpv1.WordPressSite) []string {
	hosts := []string{site.Spec.Domain}
	return append(hosts, site.Spec.Aliases...)
}

// Labels are applied to every object owned by a site, enabling selection.
func Labels(site *wpv1.WordPressSite) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "wordpress-manager-operator",
		"app.kubernetes.io/name":       "wordpress",
		"app.kubernetes.io/instance":   site.Name,
		"wp.benji.dev/site":            site.Name,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// URL returns the externally reachable address of the site.
func URL(site *wpv1.WordPressSite) string {
	scheme := "http"
	if site.Spec.TLS.Enabled {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, site.Spec.Domain)
}
