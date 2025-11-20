package madmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// MetricNavigator provides navigation functionality
type MetricNavigator interface {
	Navigate(path string) (MetricNode, error)
	Root() MetricNode
}

// MetricNode interface for navigation
type MetricNode interface {
	GetChildren() []MetricChild
	GetLeafData() map[string]string
	GetMetricType() MetricType
	GetMetricFlags() MetricFlags
	GetParent() MetricNode
	GetPath() string
	RequiredMetricTypes() MetricType
	GetChild(name string) (MetricNode, error)
	ShouldPauseRefresh() bool
}

// MetricChild represents a navigable child
type MetricChild struct {
	Name        string
	Description string
}

// ClusterAPIStats is a simplified version of madmin.APIStats that is used to
// report cluster-wide API metrics.
type ClusterAPIStats struct {
	// Time these metrics were collected
	CollectedAt time.Time `json:"collected"`

	// Nodes responded to the request.
	Nodes int `json:"nodes"`

	// Errors will contain any errors encountered while collecting the metrics.
	Errors []string `json:"errors,omitempty"`

	// Number of active requests.
	ActiveRequests int64 `json:"activeRequests,omitempty"`

	// Number of queued requests.
	QueuedRequests int64 `json:"queuedRequests,omitempty"`

	// lastMinute is the combined stats for the last minute.
	LastMinute APIStats `json:"lastMinute"`

	// LastDay is the combined stats for the last day.
	LastDay APIStats `json:"lastDay"`

	// LastDaySegmented are the stats for the last day, accumulated in time segments.
	LastDaySegmented SegmentedAPIMetrics `json:"lastDaySegmented"`
}

// ClusterAPIStats makes an admin call to retrieve general API metrics.
func (adm *AdminClient) ClusterAPIStats(ctx context.Context) (res *ClusterAPIStats, err error) {
	path := adminAPIPrefix + "/api/stats"

	resp, err := adm.executeMethod(ctx,
		http.MethodGet, requestData{
			relPath: path,
		},
	)
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}

	res = &ClusterAPIStats{}
	err = json.NewDecoder(resp.Body).Decode(res)
	return res, err
}

// RealtimeMetricsNavigator implements MetricNavigator for RealtimeMetrics
type RealtimeMetricsNavigator struct {
	metrics *RealtimeMetrics
}

// NewRealtimeMetricsNavigator creates a new navigator for RealtimeMetrics
func NewRealtimeMetricsNavigator(metrics *RealtimeMetrics) MetricNavigator {
	return &RealtimeMetricsNavigator{metrics: metrics}
}

// Navigate to a path and return the node at that location
func (nav *RealtimeMetricsNavigator) Navigate(path string) (MetricNode, error) {
	if path == "" || path == "/" {
		return nav.Root(), nil
	}

	// Split path and navigate
	parts := strings.Split(strings.Trim(path, "/"), "/")
	node := nav.Root()

	for _, part := range parts {
		if part == "" {
			continue
		}
		var err error
		node, err = node.GetChild(part)
		if err != nil {
			return nil, fmt.Errorf("path not found: %s", path)
		}
	}

	return node, nil
}

// Root returns the root node
func (nav *RealtimeMetricsNavigator) Root() MetricNode {
	return &RealtimeMetricsNode{metrics: nav.metrics}
}

// RealtimeMetricsNode represents the root node of RealtimeMetrics
type RealtimeMetricsNode struct {
	metrics *RealtimeMetrics
}

func (node *RealtimeMetricsNode) ShouldPauseUpdates() bool {
	//TODO implement me
	panic("implement me")
}

func (node *RealtimeMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "aggregated", Description: "Aggregated metrics across all nodes"},
		{Name: "by_host", Description: "Metrics broken down by individual host"},
		{Name: "by_disk", Description: "Metrics broken down by individual disk"},
		{Name: "by_disk_set", Description: "Metrics broken down by disk set"},
	}
}

func (node *RealtimeMetricsNode) GetLeafData() map[string]string {
	data := map[string]string{
		"Collection Status": map[bool]string{true: "Complete", false: "Partial"}[node.metrics.Final],
		"Active Hosts":      strconv.Itoa(len(node.metrics.Hosts)),
	}

	if len(node.metrics.Errors) > 0 {
		data["Collection Errors"] = strconv.Itoa(len(node.metrics.Errors))
	}

	// Add host information
	for i, host := range node.metrics.Hosts {
		if i < 10 { // Limit to first 10 hosts to avoid clutter
			data[fmt.Sprintf("Host %d", i+1)] = host
		}
	}

	if len(node.metrics.Hosts) > 10 {
		data["Additional Hosts"] = fmt.Sprintf("%d more...", len(node.metrics.Hosts)-10)
	}

	// Show error summary
	for i, err := range node.metrics.Errors {
		if i < 3 { // Limit to first 3 errors
			data[fmt.Sprintf("Error %d", i+1)] = err
		}
	}

	if len(node.metrics.Errors) > 3 {
		data["Additional Errors"] = fmt.Sprintf("%d more...", len(node.metrics.Errors)-3)
	}

	return data
}

func (node *RealtimeMetricsNode) GetMetricType() MetricType {
	return MetricsNone // All types available at root
}

func (node *RealtimeMetricsNode) GetMetricFlags() MetricFlags {
	var flags MetricFlags
	if len(node.metrics.ByHost) > 0 {
		flags |= MetricsByHost
	}
	if len(node.metrics.ByDisk) > 0 {
		flags |= MetricsByDisk
	}
	if len(node.metrics.ByDiskSet) > 0 {
		flags |= MetricsByDiskSet
	}
	return flags
}

func (node *RealtimeMetricsNode) GetParent() MetricNode {
	return nil // Root node has no parent
}

func (node *RealtimeMetricsNode) GetPath() string {
	return "/"
}

func (node *RealtimeMetricsNode) RequiredMetricTypes() MetricType {
	return MetricsNone // All types available at root
}

func (node *RealtimeMetricsNode) ShouldPauseRefresh() bool {
	return false // Default behavior - don't pause refresh
}

func (node *RealtimeMetricsNode) GetChild(name string) (MetricNode, error) {
	switch name {
	case "aggregated":
		return &MetricsNode{metrics: &node.metrics.Aggregated, parent: node, path: "aggregated"}, nil
	case "by_host":
		return &MapNode{
			data:        node.metrics.ByHost,
			metricType:  MetricsNone,
			metricFlags: MetricsByHost,
			parent:      node,
			path:        "by_host",
			nodeFactory: func(key string, value interface{}) MetricNode {
				if metrics, ok := value.(Metrics); ok {
					return &MetricsNode{metrics: &metrics, parent: node, path: fmt.Sprintf("by_host/%s", key)}
				}
				return nil
			},
		}, nil
	case "by_disk":
		return &MapNode{
			data:        node.metrics.ByDisk,
			metricType:  MetricsDisk,
			metricFlags: MetricsByDisk,
			parent:      node,
			path:        "by_disk",
			nodeFactory: func(key string, value interface{}) MetricNode {
				if diskMetric, ok := value.(DiskMetric); ok {
					return &DiskMetricNode{diskMetric: &diskMetric, parent: node, path: fmt.Sprintf("by_disk/%s", key)}
				}
				return nil
			},
		}, nil
	case "by_disk_set":
		return &DiskSetMapNode{
			data:        node.metrics.ByDiskSet,
			metricType:  MetricsDisk,
			metricFlags: MetricsByDiskSet,
			parent:      node,
			path:        "by_disk_set",
		}, nil
	default:
		return nil, fmt.Errorf("child not found: %s", name)
	}
}

// MetricsNode handles navigation within a Metrics struct
type MetricsNode struct {
	metrics *Metrics
	parent  MetricNode
	path    string
}

func (node *MetricsNode) ShouldPauseUpdates() bool {
	return false
}

func (node *MetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "api", Description: "API operation metrics"},
		{Name: "disk", Description: "Disk usage and performance metrics"},
		{Name: "rpc", Description: "RPC call statistics"},
		{Name: "net", Description: "Network interface metrics"},
		{Name: "os", Description: "Operating system metrics"},
		{Name: "cpu", Description: "CPU usage and performance metrics"},
		{Name: "mem", Description: "Memory usage metrics"},
		{Name: "go", Description: "Go runtime metrics"},
		{Name: "process", Description: "Process-level system metrics"},
		{Name: "replication", Description: "Replication metrics"},
		{Name: "scanner", Description: "Scanner-related metrics"},
		{Name: "batch_jobs", Description: "Batch job execution metrics"},
		{Name: "site_resync", Description: "Site replication resync metrics"},
	}
}

func (node *MetricsNode) GetLeafData() map[string]string {
	return nil // MetricsNode is a navigation node, not a leaf
}

func (node *MetricsNode) GetMetricType() MetricType {
	return MetricsNone // All types available at Metrics level
}

func (node *MetricsNode) GetMetricFlags() MetricFlags {
	return 0 // No specific flags at this level
}

func (node *MetricsNode) GetParent() MetricNode {
	return node.parent
}

func (node *MetricsNode) GetPath() string {
	return node.path
}

func (node *MetricsNode) RequiredMetricTypes() MetricType {
	return MetricsNone // All types available at Metrics level
}

func (node *MetricsNode) ShouldPauseRefresh() bool {
	return false
}

func (node *MetricsNode) GetChild(name string) (MetricNode, error) {
	switch name {
	case "scanner":
		return NewScannerMetricsNode(node.metrics.Scanner, node, fmt.Sprintf("%s/scanner", node.path)), nil
	case "disk":
		return NewDiskMetricsNavigator(node.metrics.Disk, node, fmt.Sprintf("%s/disk", node.path)), nil
	case "os":
		return NewOSMetricsNavigator(node.metrics.OS, node, fmt.Sprintf("%s/os", node.path)), nil
	case "batch_jobs":
		return &BatchJobMetricsNode{batch: node.metrics.BatchJobs, parent: node, path: fmt.Sprintf("%s/batch_jobs", node.path)}, nil
	case "site_resync":
		return &SiteResyncMetricsNode{resync: node.metrics.SiteResync, parent: node, path: fmt.Sprintf("%s/site_resync", node.path)}, nil
	case "net":
		return NewNetMetricsNavigator(node.metrics.Net, node, fmt.Sprintf("%s/net", node.GetPath())), nil
	case "mem":
		return &MemMetricsNode{mem: node.metrics.Mem, parent: node, path: fmt.Sprintf("%s/mem", node.path)}, nil
	case "cpu":
		return NewCPUMetricsNavigator(node.metrics.CPU, node, fmt.Sprintf("%s/cpu", node.path)), nil
	case "rpc":
		return &RPCMetricsNode{rpc: node.metrics.RPC, parent: node, path: fmt.Sprintf("%s/rpc", node.path)}, nil
	case "go":
		return &RuntimeMetricsNode{runtime: node.metrics.Go, parent: node, path: fmt.Sprintf("%s/go", node.path)}, nil
	case "api":
		return &APIMetricsNode{api: node.metrics.API, parent: node, path: fmt.Sprintf("%s/api", node.path)}, nil
	case "replication":
		return NewReplicationMetricsNode(node.metrics.Replication, node, fmt.Sprintf("%s/replication", node.path)), nil
	case "process":
		return &ProcessMetricsNode{process: node.metrics.Process, parent: node, path: fmt.Sprintf("%s/process", node.path)}, nil
	default:
		return nil, fmt.Errorf("child not found: %s", name)
	}
}

// DiskMetricNode handles DiskMetric navigation
type DiskMetricNode struct {
	diskMetric *DiskMetric
	parent     MetricNode
	path       string
}

func (node *DiskMetricNode) ShouldPauseUpdates() bool {
	//TODO implement me
	panic("implement me")
}

func (node *DiskMetricNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "space", Description: "Disk space information"},
		{Name: "lifetime_ops", Description: "Lifetime disk operations"},
		{Name: "last_minute", Description: "Last minute disk operations"},
		{Name: "last_day", Description: "Last day segmented disk operations"},
		{Name: "io_stats", Description: "Disk IO statistics"},
		{Name: "healing", Description: "Disk healing information"},
		{Name: "cache", Description: "Disk cache statistics"},
	}
}

func (node *DiskMetricNode) GetLeafData() map[string]string {
	return nil // DiskMetricNode is now a navigation node
}

func (node *DiskMetricNode) GetMetricType() MetricType {
	return MetricsDisk
}

func (node *DiskMetricNode) GetMetricFlags() MetricFlags {
	return 0
}

func (node *DiskMetricNode) GetParent() MetricNode {
	return node.parent
}

func (node *DiskMetricNode) GetPath() string {
	return node.path
}

func (node *DiskMetricNode) RequiredMetricTypes() MetricType {
	return MetricsDisk
}

func (node *DiskMetricNode) ShouldPauseRefresh() bool {
	return false
}

func (node *DiskMetricNode) GetChild(name string) (MetricNode, error) {
	// TODO: Implement proper disk metric sub-navigation
	// For now return an error indicating child navigation is not yet implemented
	return nil, fmt.Errorf("disk metric sub-navigation not yet implemented for: %s", name)
}

// MapNode handles dynamic map-based navigation
type MapNode struct {
	data        interface{}
	metricType  MetricType
	metricFlags MetricFlags
	parent      MetricNode
	path        string
	nodeFactory func(key string, value interface{}) MetricNode
}

func (node *MapNode) ShouldPauseUpdates() bool {
	//TODO implement me
	panic("implement me")
}

func (node *MapNode) GetChildren() []MetricChild {
	switch data := node.data.(type) {
	case map[string]Metrics:
		var children []MetricChild
		for k := range data {
			children = append(children, MetricChild{Name: k, Description: fmt.Sprintf("Metrics for %s", k)})
		}
		return children
	case map[string]DiskMetric:
		var children []MetricChild
		for k := range data {
			children = append(children, MetricChild{Name: k, Description: fmt.Sprintf("Disk metrics for %s", k)})
		}
		return children
	case map[int]map[int]DiskMetric:
		var children []MetricChild
		for k := range data {
			children = append(children, MetricChild{Name: fmt.Sprintf("%d", k), Description: fmt.Sprintf("Disk set %d", k)})
		}
		return children
	default:
		return []MetricChild{}
	}
}

func (node *MapNode) GetLeafData() map[string]string {
	children := node.GetChildren()
	data := map[string]string{
		"path":      node.path,
		"map_size":  strconv.Itoa(node.getMapSize()),
		"key_count": strconv.Itoa(len(children)),
	}
	for i, key := range children {
		data[fmt.Sprintf("key_%d", i)] = key.Name
	}
	return data
}

func (node *MapNode) GetPath() string {
	return node.path
}

func (node *MapNode) RequiredMetricTypes() MetricType {
	return node.metricType
}

func (node *MapNode) ShouldPauseRefresh() bool {
	return false
}

func (node *MapNode) getMapSize() int {
	switch data := node.data.(type) {
	case map[string]Metrics:
		return len(data)
	case map[string]DiskMetric:
		return len(data)
	case map[int]map[int]DiskMetric:
		return len(data)
	default:
		return 0
	}
}

func (node *MapNode) GetMetricType() MetricType {
	return node.metricType
}

func (node *MapNode) GetMetricFlags() MetricFlags {
	return node.metricFlags
}

func (node *MapNode) GetParent() MetricNode {
	return node.parent
}

func (node *MapNode) GetChild(name string) (MetricNode, error) {
	switch data := node.data.(type) {
	case map[string]Metrics:
		if value, exists := data[name]; exists {
			return node.nodeFactory(name, value), nil
		}
	case map[string]DiskMetric:
		if value, exists := data[name]; exists {
			return node.nodeFactory(name, value), nil
		}
	case map[int]map[int]DiskMetric:
		// This is handled by DiskSetMapNode
		return nil, fmt.Errorf("use DiskSetMapNode for nested disk set maps")
	}
	return nil, fmt.Errorf("key not found: %s", name)
}

// DiskSetMapNode handles the nested map structure for ByDiskSet
type DiskSetMapNode struct {
	data        map[int]map[int]DiskMetric
	metricType  MetricType
	metricFlags MetricFlags
	parent      MetricNode
	path        string
}

func (node *DiskSetMapNode) ShouldPauseUpdates() bool {
	//TODO implement me
	panic("implement me")
}

func (node *DiskSetMapNode) GetChildren() []MetricChild {
	var children []MetricChild
	for k := range node.data {
		children = append(children, MetricChild{
			Name:        fmt.Sprintf("pool_%d", k),
			Description: fmt.Sprintf("Disk set pool %d", k),
		})
	}
	return children
}

func (node *DiskSetMapNode) GetLeafData() map[string]string {
	children := node.GetChildren()
	data := map[string]string{
		"path":       node.path,
		"pools":      strconv.Itoa(len(node.data)),
		"pool_count": strconv.Itoa(len(children)),
	}
	for i, child := range children {
		data[fmt.Sprintf("pool_%d", i)] = child.Name
	}
	return data
}

func (node *DiskSetMapNode) GetMetricType() MetricType {
	return node.metricType
}

func (node *DiskSetMapNode) GetMetricFlags() MetricFlags {
	return node.metricFlags
}

func (node *DiskSetMapNode) GetParent() MetricNode {
	return node.parent
}

func (node *DiskSetMapNode) GetPath() string {
	return node.path
}

func (node *DiskSetMapNode) RequiredMetricTypes() MetricType {
	return node.metricType
}

func (node *DiskSetMapNode) ShouldPauseRefresh() bool {
	return false
}

func (node *DiskSetMapNode) GetChild(name string) (MetricNode, error) {
	if !strings.HasPrefix(name, "pool_") {
		return nil, fmt.Errorf("invalid pool name format: %s", name)
	}

	poolIDStr := strings.TrimPrefix(name, "pool_")
	var poolID int
	if _, err := fmt.Sscanf(poolIDStr, "%d", &poolID); err != nil {
		return nil, fmt.Errorf("invalid pool ID: %s", poolIDStr)
	}

	if sets, exists := node.data[poolID]; exists {
		return &MapNode{
			data:        sets,
			metricType:  node.metricType,
			metricFlags: node.metricFlags,
			parent:      node,
			path:        fmt.Sprintf("%s/pool_%d", node.path, poolID),
			nodeFactory: func(key string, value interface{}) MetricNode {
				if diskMetric, ok := value.(DiskMetric); ok {
					return &DiskMetricNode{diskMetric: &diskMetric, parent: node, path: fmt.Sprintf("%s/pool_%d/set_%s", node.path, poolID, key)}
				}
				return nil
			},
		}, nil
	}

	return nil, fmt.Errorf("pool not found: %d", poolID)
}

// Stub implementations for all other metric node types

type OSMetricsNode struct {
	os     *OSMetrics
	parent MetricNode
	path   string
}

func (node *OSMetricsNode) ShouldPauseUpdates() bool {
	//TODO implement me
	panic("implement me")
}

func (node *OSMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "lifetime_ops", Description: "Accumulated operations since server start"},
		{Name: "last_minute", Description: "Last minute operation statistics"},
		{Name: "sensors", Description: "Temperature sensor metrics"},
	}
}
func (node *OSMetricsNode) GetLeafData() map[string]string  { return nil }
func (node *OSMetricsNode) GetMetricType() MetricType       { return MetricsOS }
func (node *OSMetricsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *OSMetricsNode) GetParent() MetricNode           { return node.parent }
func (node *OSMetricsNode) GetPath() string                 { return node.path }
func (node *OSMetricsNode) RequiredMetricTypes() MetricType { return MetricsOS }

func (node *OSMetricsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *OSMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("os metric sub-navigation not yet implemented for: %s", name)
}

type BatchJobMetricsNode struct {
	batch  *BatchJobMetrics
	parent MetricNode
	path   string
}

func (node *BatchJobMetricsNode) ShouldPauseUpdates() bool {
	//TODO implement me
	panic("implement me")
}

func (node *BatchJobMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "jobs", Description: "Individual batch jobs by ID"},
	}
}
func (node *BatchJobMetricsNode) GetLeafData() map[string]string  { return nil }
func (node *BatchJobMetricsNode) GetMetricType() MetricType       { return MetricsBatchJobs }
func (node *BatchJobMetricsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *BatchJobMetricsNode) GetParent() MetricNode           { return node.parent }
func (node *BatchJobMetricsNode) GetPath() string                 { return node.path }
func (node *BatchJobMetricsNode) RequiredMetricTypes() MetricType { return MetricsBatchJobs }

func (node *BatchJobMetricsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *BatchJobMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("batch job metric sub-navigation not yet implemented for: %s", name)
}

type SiteResyncMetricsNode struct {
	resync *SiteResyncMetrics
	parent MetricNode
	path   string
}

func (node *SiteResyncMetricsNode) ShouldPauseUpdates() bool {
	//TODO implement me
	panic("implement me")
}

func (node *SiteResyncMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "status", Description: "Resync operation status"},
		{Name: "progress", Description: "Replication progress metrics"},
		{Name: "failed_buckets", Description: "Buckets that failed to sync"},
	}
}
func (node *SiteResyncMetricsNode) GetLeafData() map[string]string  { return nil }
func (node *SiteResyncMetricsNode) GetMetricType() MetricType       { return MetricsSiteResync }
func (node *SiteResyncMetricsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *SiteResyncMetricsNode) GetParent() MetricNode           { return node.parent }
func (node *SiteResyncMetricsNode) GetPath() string                 { return node.path }
func (node *SiteResyncMetricsNode) RequiredMetricTypes() MetricType { return MetricsSiteResync }

func (node *SiteResyncMetricsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *SiteResyncMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("site resync metric sub-navigation not yet implemented for: %s", name)
}

type MemMetricsNode struct {
	mem    *MemMetrics
	parent MetricNode
	path   string
}

func (node *MemMetricsNode) ShouldPauseUpdates() bool {
	//TODO implement me
	panic("implement me")
}

func (node *MemMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "info", Description: "Memory usage information"},
		{Name: "swap", Description: "Swap space information"},
		{Name: "cgroup", Description: "Cgroup memory limits"},
	}
}
func (node *MemMetricsNode) GetLeafData() map[string]string  { return nil }
func (node *MemMetricsNode) GetMetricType() MetricType       { return MetricsMem }
func (node *MemMetricsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *MemMetricsNode) GetParent() MetricNode           { return node.parent }
func (node *MemMetricsNode) GetPath() string                 { return node.path }
func (node *MemMetricsNode) RequiredMetricTypes() MetricType { return MetricsMem }

func (node *MemMetricsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *MemMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("mem metric sub-navigation not yet implemented for: %s", name)
}



type RuntimeMetricsNode struct {
	runtime *RuntimeMetrics
	parent  MetricNode
	path    string
}

func (node *RuntimeMetricsNode) ShouldPauseUpdates() bool {
	//TODO implement me
	panic("implement me")
}

func (node *RuntimeMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "uint_metrics", Description: "Go runtime uint64 metrics"},
		{Name: "float_metrics", Description: "Go runtime float64 metrics"},
		{Name: "histogram_metrics", Description: "Go runtime histogram metrics"},
		{Name: "gc", Description: "Garbage collection metrics"},
		{Name: "memory", Description: "Go memory statistics"},
	}
}
func (node *RuntimeMetricsNode) GetLeafData() map[string]string  { return nil }
func (node *RuntimeMetricsNode) GetMetricType() MetricType       { return MetricsRuntime }
func (node *RuntimeMetricsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RuntimeMetricsNode) GetParent() MetricNode           { return node.parent }
func (node *RuntimeMetricsNode) GetPath() string                 { return node.path }
func (node *RuntimeMetricsNode) RequiredMetricTypes() MetricType { return MetricsRuntime }

func (node *RuntimeMetricsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *RuntimeMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("runtime metric sub-navigation not yet implemented for: %s", name)
}
