package metrics

import (
	"context"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DevProvider synthesises metrics from the WordPressSites in the (mock) cluster,
// so the dashboard is fully populated in local dev with no metrics-server.
type DevProvider struct {
	c         client.Client
	namespace string
	capCPU    int64 // millicores
	capMem    int64 // bytes
}

// NewDevProvider builds a dev metrics provider with a fixed virtual node.
func NewDevProvider(c client.Client, namespace string) *DevProvider {
	return &DevProvider{
		c:         c,
		namespace: namespace,
		capCPU:    8000,     // 8 vCPU
		capMem:    16 << 30, // 16 GiB
	}
}

func (d *DevProvider) siteUsage(s *wpv1.WordPressSite) SiteUsage {
	replicas := int64(1)
	if s.Spec.Replicas != nil && *s.Spec.Replicas > 0 {
		replicas = int64(*s.Spec.Replicas)
	}
	reqCPU := s.Spec.Resources.Requests.Cpu().MilliValue()
	if reqCPU == 0 {
		reqCPU = 150
	}
	reqMem := s.Spec.Resources.Requests.Memory().Value()
	if reqMem == 0 {
		reqMem = 256 << 20 // 256 MiB
	}
	// Deterministic "actual" usage ~ 70% of request + small per-name jitter.
	jitter := int64(len(s.Name) % 30)
	return SiteUsage{
		Name:        s.Name,
		CPUMilli:    replicas * (reqCPU*70/100 + jitter),
		MemoryBytes: replicas * (reqMem * 80 / 100),
	}
}

// Sites returns synthesised per-site usage.
func (d *DevProvider) Sites(ctx context.Context) ([]SiteUsage, error) {
	list := &wpv1.WordPressSiteList{}
	if err := d.c.List(ctx, list, client.InNamespace(d.namespace)); err != nil {
		return nil, err
	}
	out := make([]SiteUsage, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, d.siteUsage(&list.Items[i]))
	}
	return out, nil
}

// Cluster sums a baseline overhead plus all site usage against a fixed capacity.
func (d *DevProvider) Cluster(ctx context.Context) (ClusterMetrics, error) {
	sites, err := d.Sites(ctx)
	if err != nil {
		return ClusterMetrics{}, err
	}
	usedCPU := int64(500)     // system overhead
	usedMem := int64(2) << 30 // 2 GiB
	for _, s := range sites {
		usedCPU += s.CPUMilli
		usedMem += s.MemoryBytes
	}
	allocCPU := d.capCPU * 95 / 100
	allocMem := d.capMem * 95 / 100
	if usedCPU > allocCPU {
		usedCPU = allocCPU
	}
	if usedMem > allocMem {
		usedMem = allocMem
	}
	cm := ClusterMetrics{
		CPU:              Metric{Used: usedCPU, Capacity: d.capCPU, Allocatable: allocCPU, Available: avail(allocCPU, usedCPU)},
		Memory:           Metric{Used: usedMem, Capacity: d.capMem, Allocatable: allocMem, Available: avail(allocMem, usedMem)},
		MetricsAvailable: true,
		Nodes: []NodeMetrics{{
			Name:   "dev-node",
			CPU:    Metric{Used: usedCPU, Capacity: d.capCPU, Allocatable: allocCPU, Available: avail(allocCPU, usedCPU)},
			Memory: Metric{Used: usedMem, Capacity: d.capMem, Allocatable: allocMem, Available: avail(allocMem, usedMem)},
		}},
	}
	return cm, nil
}

// compile-time assertions.
var _ Provider = (*DevProvider)(nil)
var _ = corev1.ResourceList{}
