package resources

import (
	"strings"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DefaultImage is used when a site does not pin its own WordPress image.
const DefaultImage = "wordpress:latest"

// ForceHTTPSSnippet is injected into wp-config.php (via WORDPRESS_CONFIG_EXTRA)
// so WordPress behind a TLS-terminating proxy/ingress treats the request as
// HTTPS — avoiding redirect loops and http:// URLs.
const ForceHTTPSSnippet = "$_SERVER['HTTP_X_FORWARDED_PROTO'] = 'https';"

// ForceHTTPSEnabled reports whether the reverse-proxy HTTPS line is injected.
// It is ON by default (when spec.forceHTTPS is unset).
func ForceHTTPSEnabled(site *wpv1.WordPressSite) bool {
	return site.Spec.ForceHTTPS == nil || *site.Spec.ForceHTTPS
}

// DesiredDeployment builds the WordPress Deployment for a site. Every replica
// mounts the SAME ReadWriteMany PVC, but at a per-site subPath, so hosts share
// one volume yet stay isolated in their own folder.
func DesiredDeployment(site *wpv1.WordPressSite, dbHost, dbPort string) *appsv1.Deployment {
	image := site.Spec.Image
	if image == "" {
		image = DefaultImage
	}
	tablePrefix := site.Spec.TablePrefix
	if tablePrefix == "" {
		tablePrefix = "wp_"
	}

	replicas := int32(1)
	if site.Spec.Replicas != nil {
		replicas = *site.Spec.Replicas
	}
	if site.Spec.Suspend {
		replicas = 0
	}

	secretName := SecretName(site)

	env := []corev1.EnvVar{
		{Name: "WORDPRESS_DB_HOST", Value: dbHost + ":" + dbPort},
		{Name: "WORDPRESS_DB_NAME", Value: DatabaseName(site)},
		{Name: "WORDPRESS_DB_USER", Value: DatabaseUser(site)},
		{Name: "WORDPRESS_TABLE_PREFIX", Value: tablePrefix},
		secretEnv("WORDPRESS_DB_PASSWORD", secretName, SecretKeyDBPassword),
	}
	for _, k := range SaltKeys() {
		env = append(env, secretEnv(k, secretName, k))
	}
	// WORDPRESS_CONFIG_EXTRA = optional reverse-proxy HTTPS line (default on)
	// + any user-supplied wp-config snippet.
	var extra []string
	if ForceHTTPSEnabled(site) {
		extra = append(extra, ForceHTTPSSnippet)
	}
	if site.Spec.PHPConfig != "" {
		extra = append(extra, site.Spec.PHPConfig)
	}
	if len(extra) > 0 {
		env = append(env, corev1.EnvVar{Name: "WORDPRESS_CONFIG_EXTRA", Value: strings.Join(extra, "\n")})
	}
	// User-supplied env wins / extends.
	env = append(env, site.Spec.Env...)

	probe := func(initial int32) *corev1.Probe {
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/wp-login.php",
					Port: intstr.FromInt32(80),
				},
			},
			InitialDelaySeconds: initial,
			PeriodSeconds:       15,
			TimeoutSeconds:      5,
			FailureThreshold:    6,
		}
	}

	volumeMounts := []corev1.VolumeMount{{
		Name:      "site-data",
		MountPath: "/var/www/html",
		SubPath:   SubPath(site),
	}}
	volumes := []corev1.Volume{{
		Name: "site-data",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: SharedPVCName(site),
			},
		},
	}}

	// Mount php.ini (default or custom) into PHP's conf.d scan dir. The hash
	// annotation forces a rollout (so PHP reloads) whenever it is edited.
	podAnnotations := map[string]string{
		"wp.benji.dev/php-ini-hash": PHPIniHash(site),
	}
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "php-config",
		MountPath: PHPMountPath,
		SubPath:   PHPIniFileName,
		ReadOnly:  true,
	})
	volumes = append(volumes, corev1.Volume{
		Name: "php-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: PHPConfigMapName(site)},
			},
		},
	})

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      site.Name,
			Namespace: site.Namespace,
			Labels:    Labels(site),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector(site)},
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: Labels(site), Annotations: podAnnotations},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:           "wordpress",
						Image:          image,
						Ports:          []corev1.ContainerPort{{Name: "http", ContainerPort: 80}},
						Env:            env,
						VolumeMounts:   volumeMounts,
						Resources:      site.Spec.Resources,
						ReadinessProbe: probe(20),
						LivenessProbe:  probe(60),
					}},
					Volumes: volumes,
				},
			},
		},
	}
}

func secretEnv(name, secret, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secret},
				Key:                  key,
			},
		},
	}
}

func selector(site *wpv1.WordPressSite) map[string]string {
	return map[string]string{"wp.benji.dev/site": site.Name}
}
