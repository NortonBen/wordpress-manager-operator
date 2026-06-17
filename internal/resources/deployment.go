package resources

import (
	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DefaultImage is used when a site does not pin its own WordPress image.
const DefaultImage = "wordpress:latest"

// ForceHTTPSSnippet is inserted at the TOP of wp-config.php (right after the
// opening <?php) so WordPress behind a TLS-terminating proxy/ingress treats the
// request as HTTPS — avoiding redirect loops and http:// URLs. It runs before
// anything else in wp-config.
const ForceHTTPSSnippet = "$_SERVER['HTTP_X_FORWARDED_PROTO'] = 'https';"

const (
	// ForwardedProtoFile is the ConfigMap key / mounted file holding the snippet.
	ForwardedProtoFile = "forwarded-proto.php"
	// ForwardedProtoMount is where that file is mounted in the container.
	ForwardedProtoMount = "/etc/wpmgr/" + ForwardedProtoFile
)

// forwardedProtoMarker uniquely tags our inserted line. The official WordPress
// image's wp-config already contains the string "HTTP_X_FORWARDED_PROTO" (its
// built-in reverse-proxy block), so the idempotency guard must NOT key on that —
// it would always match and skip the insert. We key on this marker instead.
const forwardedProtoMarker = "wpmgr-forwarded-proto"

// forwardedProtoPostStart waits for the WordPress entrypoint to generate
// wp-config.php, then (idempotently, guarded by the marker) inserts the snippet
// right after <?php. sed reads the line from the mounted file (no shell/PHP
// escaping) and writes via a temp file + mv so it works with both GNU and BSD
// sed (no `-i`). Always exits 0 so a transient issue never kills the pod; it
// edits the on-disk wp-config so existing sites are fixed on the next restart.
// Paths are overridable (WPMGR_WPCONFIG / WPMGR_SNIPPET) for testing.
const forwardedProtoPostStart = `f="${WPMGR_WPCONFIG:-/var/www/html/wp-config.php}"; ` +
	`snip="${WPMGR_SNIPPET:-` + ForwardedProtoMount + `}"; ` +
	`for i in $(seq 1 60); do [ -f "$f" ] && break; sleep 1; done; ` +
	`if [ -f "$f" ] && ! grep -q ` + forwardedProtoMarker + ` "$f"; then ` +
	`sed "/^<?php/r $snip" "$f" > "$f.wpmgr" && mv "$f.wpmgr" "$f"; fi; exit 0`

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
	// WORDPRESS_CONFIG_EXTRA carries the user-supplied wp-config snippet. The
	// forceHTTPS line is NOT placed here (it would land lower in wp-config);
	// instead a postStart hook inserts it right after <?php (see below).
	if site.Spec.PHPConfig != "" {
		env = append(env, corev1.EnvVar{Name: "WORDPRESS_CONFIG_EXTRA", Value: site.Spec.PHPConfig})
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

	// forceHTTPS: mount the snippet and insert it right after <?php at startup.
	var lifecycle *corev1.Lifecycle
	if ForceHTTPSEnabled(site) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "php-config",
			MountPath: ForwardedProtoMount,
			SubPath:   ForwardedProtoFile,
			ReadOnly:  true,
		})
		lifecycle = &corev1.Lifecycle{
			PostStart: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{Command: []string{"sh", "-c", forwardedProtoPostStart}},
			},
		}
	}

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
						Lifecycle:      lifecycle,
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
