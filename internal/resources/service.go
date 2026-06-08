package resources

import (
	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DesiredService builds the ClusterIP Service fronting a site's pods.
func DesiredService(site *wpv1.WordPressSite) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      site.Name,
			Namespace: site.Namespace,
			Labels:    Labels(site),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selector(site),
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt32(80),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}
