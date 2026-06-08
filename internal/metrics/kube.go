package metrics

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SiteLabel groups pods belonging to one WordPressSite.
const SiteLabel = "wp.benji.dev/site"

// KubeProvider reads real usage from the metrics.k8s.io API (metrics-server) and
// capacity/allocatable from Node objects. If metrics-server is not installed it
// degrades gracefully: capacities are still reported, usage is zero and
// MetricsAvailable is false.
type KubeProvider struct {
	c         client.Client
	namespace string
}

// NewKubeProvider builds the production metrics provider.
func NewKubeProvider(c client.Client, namespace string) *KubeProvider {
	return &KubeProvider{c: c, namespace: namespace}
}

func (k *KubeProvider) Cluster(ctx context.Context) (ClusterMetrics, error) {
	nodes := &corev1.NodeList{}
	if err := k.c.List(ctx, nodes); err != nil {
		return ClusterMetrics{}, err
	}

	// Per-node usage from metrics-server (best effort).
	used := map[string][2]int64{} // name -> {cpuMilli, memBytes}
	var cm ClusterMetrics
	nm := &metricsv1beta1.NodeMetricsList{}
	if err := k.c.List(ctx, nm); err == nil {
		cm.MetricsAvailable = true
		for i := range nm.Items {
			cpu := nm.Items[i].Usage.Cpu().MilliValue()
			mem := nm.Items[i].Usage.Memory().Value()
			used[nm.Items[i].Name] = [2]int64{cpu, mem}
		}
	}

	var capCPU, capMem, allocCPU, allocMem, usedCPU, usedMem int64
	for i := range nodes.Items {
		n := &nodes.Items[i]
		ncapCPU := n.Status.Capacity.Cpu().MilliValue()
		ncapMem := n.Status.Capacity.Memory().Value()
		nallocCPU := n.Status.Allocatable.Cpu().MilliValue()
		nallocMem := n.Status.Allocatable.Memory().Value()
		u := used[n.Name]

		capCPU += ncapCPU
		capMem += ncapMem
		allocCPU += nallocCPU
		allocMem += nallocMem
		usedCPU += u[0]
		usedMem += u[1]

		cm.Nodes = append(cm.Nodes, NodeMetrics{
			Name:   n.Name,
			CPU:    Metric{Used: u[0], Capacity: ncapCPU, Allocatable: nallocCPU, Available: avail(nallocCPU, u[0])},
			Memory: Metric{Used: u[1], Capacity: ncapMem, Allocatable: nallocMem, Available: avail(nallocMem, u[1])},
		})
	}

	cm.CPU = Metric{Used: usedCPU, Capacity: capCPU, Allocatable: allocCPU, Available: avail(allocCPU, usedCPU)}
	cm.Memory = Metric{Used: usedMem, Capacity: capMem, Allocatable: allocMem, Available: avail(allocMem, usedMem)}
	return cm, nil
}

func (k *KubeProvider) Sites(ctx context.Context) ([]SiteUsage, error) {
	pm := &metricsv1beta1.PodMetricsList{}
	if err := k.c.List(ctx, pm, client.InNamespace(k.namespace)); err != nil {
		// metrics-server absent → no per-site usage, but don't fail the request.
		return nil, nil
	}
	agg := map[string]*SiteUsage{}
	for i := range pm.Items {
		site := pm.Items[i].Labels[SiteLabel]
		if site == "" {
			continue
		}
		u := agg[site]
		if u == nil {
			u = &SiteUsage{Name: site}
			agg[site] = u
		}
		for _, ct := range pm.Items[i].Containers {
			u.CPUMilli += ct.Usage.Cpu().MilliValue()
			u.MemoryBytes += ct.Usage.Memory().Value()
		}
	}
	out := make([]SiteUsage, 0, len(agg))
	for _, v := range agg {
		out = append(out, *v)
	}
	return out, nil
}

var _ Provider = (*KubeProvider)(nil)
