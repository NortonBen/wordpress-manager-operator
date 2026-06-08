// Package metrics exposes cluster CPU/RAM usage (used, capacity, allocatable and
// remaining) plus per-site usage, for the admin dashboard. It has a production
// provider (reads the metrics.k8s.io API + Node objects) and a dev provider that
// synthesises plausible numbers from the WordPressSites in the mock cluster.
package metrics

import "context"

// Metric reports usage for one resource dimension. CPU values are in millicores,
// memory values are in bytes.
type Metric struct {
	Used        int64 `json:"used"`
	Capacity    int64 `json:"capacity"`
	Allocatable int64 `json:"allocatable"`
	Available   int64 `json:"available"` // allocatable - used (never negative)
}

// NodeMetrics is per-node usage.
type NodeMetrics struct {
	Name   string `json:"name"`
	CPU    Metric `json:"cpu"`
	Memory Metric `json:"memory"`
}

// ClusterMetrics aggregates the whole cluster.
type ClusterMetrics struct {
	CPU              Metric        `json:"cpu"`    // millicores
	Memory           Metric        `json:"memory"` // bytes
	Nodes            []NodeMetrics `json:"nodes"`
	MetricsAvailable bool          `json:"metricsAvailable"` // false if metrics-server is absent
}

// SiteUsage is the live usage attributed to a single WordPressSite.
type SiteUsage struct {
	Name        string `json:"name"`
	CPUMilli    int64  `json:"cpuMillicores"`
	MemoryBytes int64  `json:"memoryBytes"`
}

// Provider returns cluster and per-site metrics.
type Provider interface {
	Cluster(ctx context.Context) (ClusterMetrics, error)
	Sites(ctx context.Context) ([]SiteUsage, error)
}

// avail computes allocatable-used, clamped at zero.
func avail(allocatable, used int64) int64 {
	if used > allocatable {
		return 0
	}
	return allocatable - used
}
