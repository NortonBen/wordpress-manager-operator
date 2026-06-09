package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WordPressSiteSpec defines the desired state of a single WordPress host.
type WordPressSiteSpec struct {
	// Domain is the primary FQDN bound to this site. An Ingress rule and TLS
	// certificate are generated for it automatically.
	// +kubebuilder:validation:MinLength=1
	Domain string `json:"domain"`

	// Aliases are additional hostnames that also route to this site.
	// +optional
	Aliases []string `json:"aliases,omitempty"`

	// Image is the WordPress container image. Defaults to a sane php-apache build.
	// +optional
	Image string `json:"image,omitempty"`

	// Replicas is the number of WordPress pods. Because all replicas share the
	// same sub-folder on a ReadWriteMany volume, >1 is supported.
	// +optional
	// +kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources are the compute resources for the WordPress container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage controls how this site's files are placed on the shared volume.
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Database controls per-site database provisioning. A dedicated database and
	// a dedicated, least-privilege user are created for every site.
	// +optional
	Database DatabaseSpec `json:"database,omitempty"`

	// TLS controls certificate issuance for the site's Ingress.
	// +optional
	TLS TLSSpec `json:"tls,omitempty"`

	// IngressClassName overrides the operator-wide default ingress class.
	// +optional
	IngressClassName string `json:"ingressClassName,omitempty"`

	// IngressAnnotations are merged onto the generated Ingress, enabling
	// per-site customisation (rate limits, body size, auth, etc.).
	// +optional
	IngressAnnotations map[string]string `json:"ingressAnnotations,omitempty"`

	// Env are extra environment variables injected into the WordPress container,
	// on top of the auto-generated DB credentials and security keys.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// PHPConfig is appended verbatim to wp-config.php via WORDPRESS_CONFIG_EXTRA.
	// +optional
	PHPConfig string `json:"phpConfig,omitempty"`

	// PHPIni is raw php.ini content mounted into the PHP conf.d scan directory,
	// letting you tune PHP runtime settings (memory_limit, upload_max_filesize,
	// max_execution_time, ...). The WordPress pod is rolled out automatically
	// whenever this value changes, so edits re-apply on the fly.
	// +optional
	PHPIni string `json:"phpIni,omitempty"`

	// TablePrefix sets the WordPress table prefix. Defaults to "wp_".
	// +optional
	// +kubebuilder:default=wp_
	TablePrefix string `json:"tablePrefix,omitempty"`

	// ForceHTTPS, when enabled (the DEFAULT when unset), injects
	// `$_SERVER['HTTP_X_FORWARDED_PROTO'] = 'https';` into wp-config.php so
	// WordPress behind a TLS-terminating proxy/ingress builds https URLs and
	// avoids redirect loops. Set to false to turn it off.
	// +optional
	ForceHTTPS *bool `json:"forceHTTPS,omitempty"`

	// Suspend stops reconciliation and scales the deployment to zero when true.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// StorageSpec describes the shared-volume sub-folder layout for a site.
type StorageSpec struct {
	// SharedPVCName is the ReadWriteMany PVC every site mounts. Defaults to the
	// operator-wide shared PVC.
	// +optional
	SharedPVCName string `json:"sharedPVCName,omitempty"`

	// SubPath is the folder inside the shared volume used for this site. Defaults
	// to the site's name, guaranteeing isolation between hosts.
	// +optional
	SubPath string `json:"subPath,omitempty"`
}

// DatabaseSpec describes per-site database provisioning.
type DatabaseSpec struct {
	// Name overrides the generated database name (default: wp_<site>).
	// +optional
	Name string `json:"name,omitempty"`

	// User overrides the generated database user (default: wpu_<site>).
	// +optional
	User string `json:"user,omitempty"`

	// Host restricts where the user may connect from. Defaults to "%".
	// +optional
	// +kubebuilder:default="%"
	Host string `json:"host,omitempty"`
}

// TLSSpec controls certificate behaviour for the generated Ingress.
type TLSSpec struct {
	// Enabled turns on HTTPS for the site.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Issuer is the cert-manager ClusterIssuer used to obtain the certificate.
	// +optional
	Issuer string `json:"issuer,omitempty"`

	// SecretName overrides the TLS secret name (default: <site>-tls).
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// WordPressSiteStatus is the observed state of a WordPress host.
type WordPressSiteStatus struct {
	// Phase is a coarse, human-friendly lifecycle state.
	// +optional
	Phase string `json:"phase,omitempty"`

	// URL is the externally reachable address of the site.
	// +optional
	URL string `json:"url,omitempty"`

	// DatabaseName / DatabaseUser report what was provisioned in MySQL.
	// +optional
	DatabaseName string `json:"databaseName,omitempty"`
	// +optional
	DatabaseUser string `json:"databaseUser,omitempty"`

	// SecretName is the per-site Secret holding DB credentials and WP salts.
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// ObservedGeneration is the .metadata.generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions follow the standard Kubernetes condition convention.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Phase constants.
const (
	PhasePending      = "Pending"
	PhaseProvisioning = "Provisioning"
	PhaseReady        = "Ready"
	PhaseSuspended    = "Suspended"
	PhaseError        = "Error"
)

// Finalizer ensures the per-site database is cleaned up on deletion.
const Finalizer = "wp.benji.dev/finalizer"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wp;wpsite
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WordPressSite is a single managed WordPress host.
type WordPressSite struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WordPressSiteSpec   `json:"spec,omitempty"`
	Status WordPressSiteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WordPressSiteList contains a list of WordPressSite.
type WordPressSiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WordPressSite `json:"items"`
}
