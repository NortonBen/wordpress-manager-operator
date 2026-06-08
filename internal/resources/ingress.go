package resources

import (
	"maps"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultIngressClass is used when neither the site nor the operator overrides it.
var DefaultIngressClass = "nginx"

// DesiredIngress builds the Ingress that binds the site's domain (and aliases)
// to its Service, wiring TLS via cert-manager when enabled.
func DesiredIngress(site *wpv1.WordPressSite) *netv1.Ingress {
	pathType := netv1.PathTypePrefix
	className := site.Spec.IngressClassName
	if className == "" {
		className = DefaultIngressClass
	}

	annotations := map[string]string{
		// WordPress needs large uploads; lift the default body size.
		"nginx.ingress.kubernetes.io/proxy-body-size": "256m",
	}
	if site.Spec.TLS.Enabled && site.Spec.TLS.Issuer != "" {
		annotations["cert-manager.io/cluster-issuer"] = site.Spec.TLS.Issuer
	}
	maps.Copy(annotations, site.Spec.IngressAnnotations)

	rules := make([]netv1.IngressRule, 0, len(Hosts(site)))
	for _, host := range Hosts(site) {
		rules = append(rules, netv1.IngressRule{
			Host: host,
			IngressRuleValue: netv1.IngressRuleValue{
				HTTP: &netv1.HTTPIngressRuleValue{
					Paths: []netv1.HTTPIngressPath{{
						Path:     "/",
						PathType: &pathType,
						Backend: netv1.IngressBackend{
							Service: &netv1.IngressServiceBackend{
								Name: site.Name,
								Port: netv1.ServiceBackendPort{Number: 80},
							},
						},
					}},
				},
			},
		})
	}

	ing := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        site.Name,
			Namespace:   site.Namespace,
			Labels:      Labels(site),
			Annotations: annotations,
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &className,
			Rules:            rules,
		},
	}

	if site.Spec.TLS.Enabled {
		ing.Spec.TLS = []netv1.IngressTLS{{
			Hosts:      Hosts(site),
			SecretName: TLSSecretName(site),
		}}
	}
	return ing
}
