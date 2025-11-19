package madmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
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
		"Active Hosts": strconv.Itoa(len(node.metrics.Hosts)),
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

func (node *MetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "scanner", Description: "Scanner-related metrics"},
		{Name: "disk", Description: "Disk usage and performance metrics"},
		{Name: "os", Description: "Operating system metrics"},
		{Name: "batch_jobs", Description: "Batch job execution metrics"},
		{Name: "site_resync", Description: "Site replication resync metrics"},
		{Name: "net", Description: "Network interface metrics"},
		{Name: "mem", Description: "Memory usage metrics"},
		{Name: "cpu", Description: "CPU usage and performance metrics"},
		{Name: "rpc", Description: "RPC call statistics"},
		{Name: "go", Description: "Go runtime metrics"},
		{Name: "api", Description: "API operation metrics"},
		{Name: "replication", Description: "Replication metrics"},
		{Name: "process", Description: "Process-level system metrics"},
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
		return &ReplicationMetricsNode{repl: node.metrics.Replication, parent: node, path: fmt.Sprintf("%s/replication", node.path)}, nil
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
		"path":     node.path,
		"map_size": strconv.Itoa(node.getMapSize()),
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
		"path":      node.path,
		"pools":     strconv.Itoa(len(node.data)),
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

func (node *OSMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "lifetime_ops", Description: "Accumulated operations since server start"},
		{Name: "last_minute", Description: "Last minute operation statistics"},
		{Name: "sensors", Description: "Temperature sensor metrics"},
	}
}
func (node *OSMetricsNode) GetLeafData() map[string]string { return nil }
func (node *OSMetricsNode) GetMetricType() MetricType   { return MetricsOS }
func (node *OSMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *OSMetricsNode) GetParent() MetricNode { return node.parent }
func (node *OSMetricsNode) GetPath() string { return node.path }
func (node *OSMetricsNode) RequiredMetricTypes() MetricType { return MetricsOS }
func (node *OSMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("os metric sub-navigation not yet implemented for: %s", name)
}

type BatchJobMetricsNode struct {
	batch  *BatchJobMetrics
	parent MetricNode
	path   string
}

func (node *BatchJobMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "jobs", Description: "Individual batch jobs by ID"},
	}
}
func (node *BatchJobMetricsNode) GetLeafData() map[string]string { return nil }
func (node *BatchJobMetricsNode) GetMetricType() MetricType   { return MetricsBatchJobs }
func (node *BatchJobMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *BatchJobMetricsNode) GetParent() MetricNode { return node.parent }
func (node *BatchJobMetricsNode) GetPath() string { return node.path }
func (node *BatchJobMetricsNode) RequiredMetricTypes() MetricType { return MetricsBatchJobs }
func (node *BatchJobMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("batch job metric sub-navigation not yet implemented for: %s", name)
}

type SiteResyncMetricsNode struct {
	resync *SiteResyncMetrics
	parent MetricNode
	path   string
}

func (node *SiteResyncMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "status", Description: "Resync operation status"},
		{Name: "progress", Description: "Replication progress metrics"},
		{Name: "failed_buckets", Description: "Buckets that failed to sync"},
	}
}
func (node *SiteResyncMetricsNode) GetLeafData() map[string]string { return nil }
func (node *SiteResyncMetricsNode) GetMetricType() MetricType   { return MetricsSiteResync }
func (node *SiteResyncMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *SiteResyncMetricsNode) GetParent() MetricNode { return node.parent }
func (node *SiteResyncMetricsNode) GetPath() string { return node.path }
func (node *SiteResyncMetricsNode) RequiredMetricTypes() MetricType { return MetricsSiteResync }
func (node *SiteResyncMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("site resync metric sub-navigation not yet implemented for: %s", name)
}


type MemMetricsNode struct {
	mem    *MemMetrics
	parent MetricNode
	path   string
}

func (node *MemMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "info", Description: "Memory usage information"},
		{Name: "swap", Description: "Swap space information"},
		{Name: "cgroup", Description: "Cgroup memory limits"},
	}
}
func (node *MemMetricsNode) GetLeafData() map[string]string { return nil }
func (node *MemMetricsNode) GetMetricType() MetricType   { return MetricsMem }
func (node *MemMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *MemMetricsNode) GetParent() MetricNode { return node.parent }
func (node *MemMetricsNode) GetPath() string { return node.path }
func (node *MemMetricsNode) RequiredMetricTypes() MetricType { return MetricsMem }
func (node *MemMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("mem metric sub-navigation not yet implemented for: %s", name)
}

type CPUMetricsNode struct {
	cpu    *CPUMetrics
	parent MetricNode
	path   string
}

func (node *CPUMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "times", Description: "CPU time statistics"},
		{Name: "load", Description: "System load averages"},
		{Name: "frequency", Description: "CPU frequency information"},
		{Name: "models", Description: "CPU model information"},
	}
}
func (node *CPUMetricsNode) GetLeafData() map[string]string { return nil }
func (node *CPUMetricsNode) GetMetricType() MetricType   { return MetricsCPU }
func (node *CPUMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *CPUMetricsNode) GetParent() MetricNode { return node.parent }
func (node *CPUMetricsNode) GetPath() string { return node.path }
func (node *CPUMetricsNode) RequiredMetricTypes() MetricType { return MetricsCPU }
func (node *CPUMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("cpu metric sub-navigation not yet implemented for: %s", name)
}

type RPCMetricsNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "connections", Description: "RPC connection statistics"},
		{Name: "last_minute", Description: "Last minute RPC statistics by handler"},
		{Name: "last_day", Description: "Last day RPC statistics segmented"},
		{Name: "by_destination", Description: "RPC statistics by destination"},
		{Name: "by_caller", Description: "RPC statistics by caller"},
	}
}
func (node *RPCMetricsNode) GetLeafData() map[string]string { return nil }
func (node *RPCMetricsNode) GetMetricType() MetricType   { return MetricsRPC }
func (node *RPCMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *RPCMetricsNode) GetParent() MetricNode { return node.parent }
func (node *RPCMetricsNode) GetPath() string { return node.path }
func (node *RPCMetricsNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("rpc metric sub-navigation not yet implemented for: %s", name)
}

type RuntimeMetricsNode struct {
	runtime *RuntimeMetrics
	parent  MetricNode
	path    string
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
func (node *RuntimeMetricsNode) GetLeafData() map[string]string { return nil }
func (node *RuntimeMetricsNode) GetMetricType() MetricType   { return MetricsRuntime }
func (node *RuntimeMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *RuntimeMetricsNode) GetParent() MetricNode { return node.parent }
func (node *RuntimeMetricsNode) GetPath() string { return node.path }
func (node *RuntimeMetricsNode) RequiredMetricTypes() MetricType { return MetricsRuntime }
func (node *RuntimeMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("runtime metric sub-navigation not yet implemented for: %s", name)
}

type APIMetricsNode struct {
	api    *APIMetrics
	parent MetricNode
	path   string
}

func (node *APIMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "last_minute", Description: "Last minute API statistics by endpoint"},
		{Name: "last_day", Description: "Last day API statistics segmented"},
		{Name: "since_start", Description: "API statistics since server start"},
	}
}
func (node *APIMetricsNode) GetLeafData() map[string]string {
	// Create comprehensive executive-level API performance dashboard
	return node.generateAPIOverviewDashboard()
}
func (node *APIMetricsNode) GetMetricType() MetricType   { return MetricsAPI }
func (node *APIMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *APIMetricsNode) GetParent() MetricNode { return node.parent }
func (node *APIMetricsNode) GetPath() string { return node.path }
func (node *APIMetricsNode) RequiredMetricTypes() MetricType { return MetricsAPI }
func (node *APIMetricsNode) GetChild(name string) (MetricNode, error) {
	switch name {
	case "last_minute":
		return &APILastMinuteNode{
			api:    node.api,
			parent: node,
			path:   node.path + "/last_minute",
		}, nil
	case "last_day":
		return &APILastDayNode{
			api:    node.api,
			parent: node,
			path:   node.path + "/last_day",
		}, nil
	case "since_start":
		return &APISinceStartNode{
			api:    node.api,
			parent: node,
			path:   node.path + "/since_start",
		}, nil
	default:
		return nil, fmt.Errorf("unknown API metric child: %s", name)
	}
}

// === API CHILD NODES ===

// APILastMinuteNode shows last minute API statistics by endpoint
type APILastMinuteNode struct {
	api    *APIMetrics
	parent MetricNode
	path   string
}

func (node *APILastMinuteNode) GetChildren() []MetricChild {
	if node.api.LastMinuteAPI == nil || len(node.api.LastMinuteAPI) == 0 {
		return []MetricChild{}
	}

	var children []MetricChild
	for endpoint := range node.api.LastMinuteAPI {
		children = append(children, MetricChild{
			Name:        endpoint,
			Description: fmt.Sprintf("Last minute statistics for %s", endpoint),
		})
	}
	return children
}

func (node *APILastMinuteNode) GetLeafData() map[string]string {
	data := make(map[string]string)

	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""
	data["            LAST MINUTE API SUMMARY"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""

	total := node.api.LastMinuteTotal()

	if total.Requests == 0 {
		data["Status"] = "No API requests in the last minute"
		return data
	}

	// Summary statistics
	avgLatency := (total.RequestTimeSecs / float64(total.Requests)) * 1000
	data["Total Requests"] = humanize.Comma(total.Requests)
	data["Average Latency"] = fmt.Sprintf("%.1f ms", avgLatency)

	if total.RequestTimeSecsMax > 0 {
		data["Latency Range"] = fmt.Sprintf("%.1f - %.1f ms",
			total.RequestTimeSecsMin*1000, total.RequestTimeSecsMax*1000)
	}

	// Throughput
	totalBytes := total.IncomingBytes + total.OutgoingBytes
	if totalBytes > 0 {
		data["Total Throughput"] = humanize.Bytes(uint64(totalBytes))
		data["↳ Incoming"] = humanize.Bytes(uint64(total.IncomingBytes))
		data["↳ Outgoing"] = humanize.Bytes(uint64(total.OutgoingBytes))
	}

	// Error analysis
	totalErrors := total.Errors4xx + total.Errors5xx
	if totalErrors > 0 {
		errorRate := float64(totalErrors) / float64(total.Requests) * 100
		data["Error Rate"] = fmt.Sprintf("%.2f%% (%d errors)", errorRate, totalErrors)
		if total.Errors4xx > 0 {
			data["↳ Client Errors (4xx)"] = fmt.Sprintf("%d", total.Errors4xx)
		}
		if total.Errors5xx > 0 {
			data["↳ Server Errors (5xx)"] = fmt.Sprintf("%d", total.Errors5xx)
		}
	}

	// Endpoint breakdown
	data[" "] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ "] = ""
	data["            ENDPOINT BREAKDOWN"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ "] = ""

	data["Active Endpoints"] = fmt.Sprintf("%d endpoints", len(node.api.LastMinuteAPI))

	// Show top 5 busiest endpoints
	type endpointStat struct {
		name string
		stats APIStats
	}

	var endpoints []endpointStat
	for name, stats := range node.api.LastMinuteAPI {
		endpoints = append(endpoints, endpointStat{name, stats})
	}

	// Sort by request count
	for i := 0; i < len(endpoints)-1; i++ {
		for j := i + 1; j < len(endpoints); j++ {
			if endpoints[i].stats.Requests < endpoints[j].stats.Requests {
				endpoints[i], endpoints[j] = endpoints[j], endpoints[i]
			}
		}
	}

	maxShow := 5
	if len(endpoints) < maxShow {
		maxShow = len(endpoints)
	}

	for i := 0; i < maxShow; i++ {
		ep := endpoints[i]
		if ep.stats.Requests > 0 {
			avgLatency := (ep.stats.RequestTimeSecs / float64(ep.stats.Requests)) * 1000
			errors := ep.stats.Errors4xx + ep.stats.Errors5xx
			data[fmt.Sprintf("↳ %s", ep.name)] = fmt.Sprintf("%s req, %.1fms avg, %d err",
				humanize.Comma(ep.stats.Requests), avgLatency, errors)
		}
	}

	return data
}

func (node *APILastMinuteNode) GetMetricType() MetricType   { return MetricsAPI }
func (node *APILastMinuteNode) GetMetricFlags() MetricFlags { return 0 }
func (node *APILastMinuteNode) GetParent() MetricNode       { return node.parent }
func (node *APILastMinuteNode) GetPath() string             { return node.path }
func (node *APILastMinuteNode) RequiredMetricTypes() MetricType { return MetricsAPI }
func (node *APILastMinuteNode) GetChild(name string) (MetricNode, error) {
	if node.api.LastMinuteAPI == nil {
		return nil, fmt.Errorf("no last minute API data available")
	}

	if stats, exists := node.api.LastMinuteAPI[name]; exists {
		return &APIEndpointNode{
			endpoint: name,
			stats:    stats,
			parent:   node,
			path:     node.path + "/" + name,
		}, nil
	}

	return nil, fmt.Errorf("endpoint not found: %s", name)
}

// APILastDayNode shows last day API statistics segmented
type APILastDayNode struct {
	api    *APIMetrics
	parent MetricNode
	path   string
}

func (node *APILastDayNode) GetChildren() []MetricChild {
	if node.api.LastDayAPI == nil || len(node.api.LastDayAPI) == 0 {
		return []MetricChild{}
	}

	var children []MetricChild
	for endpoint := range node.api.LastDayAPI {
		children = append(children, MetricChild{
			Name:        endpoint,
			Description: fmt.Sprintf("Last day segmented statistics for %s", endpoint),
		})
	}
	return children
}

func (node *APILastDayNode) GetLeafData() map[string]string {
	data := make(map[string]string)

	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""
	data["            LAST DAY API SUMMARY"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""

	total := node.api.LastDayTotal()

	if total.Requests == 0 {
		data["Status"] = "No API requests in the last day"
		return data
	}

	// Summary statistics
	avgLatency := (total.RequestTimeSecs / float64(total.Requests)) * 1000
	data["Total Requests"] = humanize.Comma(total.Requests)
	data["Average Latency"] = fmt.Sprintf("%.1f ms", avgLatency)

	if total.RequestTimeSecsMax > 0 {
		data["Latency Range"] = fmt.Sprintf("%.1f - %.1f ms",
			total.RequestTimeSecsMin*1000, total.RequestTimeSecsMax*1000)
	}

	// Throughput
	totalBytes := total.IncomingBytes + total.OutgoingBytes
	if totalBytes > 0 {
		data["Total Throughput"] = humanize.Bytes(uint64(totalBytes))
		data["↳ Incoming"] = humanize.Bytes(uint64(total.IncomingBytes))
		data["↳ Outgoing"] = humanize.Bytes(uint64(total.OutgoingBytes))

		if total.WallTimeSecs > 0 {
			avgThroughputPerSec := float64(totalBytes) / total.WallTimeSecs
			data["Avg Throughput/sec"] = fmt.Sprintf("%s/sec", humanize.Bytes(uint64(avgThroughputPerSec)))
		}
	}

	// Error analysis
	totalErrors := total.Errors4xx + total.Errors5xx
	if totalErrors > 0 {
		errorRate := float64(totalErrors) / float64(total.Requests) * 100
		data["Error Rate"] = fmt.Sprintf("%.2f%% (%d errors)", errorRate, totalErrors)
		if total.Errors4xx > 0 {
			data["↳ Client Errors (4xx)"] = fmt.Sprintf("%d", total.Errors4xx)
		}
		if total.Errors5xx > 0 {
			data["↳ Server Errors (5xx)"] = fmt.Sprintf("%d", total.Errors5xx)
		}
	}

	// Request rate analysis
	if total.WallTimeSecs > 0 {
		avgRPS := float64(total.Requests) / total.WallTimeSecs
		data["Average RPS"] = fmt.Sprintf("%.1f req/sec", avgRPS)
	}

	data["Active Endpoints"] = fmt.Sprintf("%d endpoints", len(node.api.LastDayAPI))
	data["Responding Nodes"] = fmt.Sprintf("%d nodes", node.api.Nodes)

	return data
}

func (node *APILastDayNode) GetMetricType() MetricType   { return MetricsAPI }
func (node *APILastDayNode) GetMetricFlags() MetricFlags { return 0 }
func (node *APILastDayNode) GetParent() MetricNode       { return node.parent }
func (node *APILastDayNode) GetPath() string             { return node.path }
func (node *APILastDayNode) RequiredMetricTypes() MetricType { return MetricsAPI }
func (node *APILastDayNode) GetChild(name string) (MetricNode, error) {
	if node.api.LastDayAPI == nil {
		return nil, fmt.Errorf("no last day API data available")
	}

	if segmented, exists := node.api.LastDayAPI[name]; exists {
		return &APISegmentedNode{
			endpoint:  name,
			segmented: segmented,
			parent:    node,
			path:      node.path + "/" + name,
		}, nil
	}

	return nil, fmt.Errorf("endpoint not found: %s", name)
}

// APISinceStartNode shows API statistics since server start
type APISinceStartNode struct {
	api    *APIMetrics
	parent MetricNode
	path   string
}

func (node *APISinceStartNode) GetChildren() []MetricChild {
	return []MetricChild{}
}

func (node *APISinceStartNode) GetLeafData() map[string]string {
	data := make(map[string]string)

	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""
	data["          API LIFETIME STATISTICS"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""

	since := node.api.SinceStart

	if since.Requests == 0 {
		data["Status"] = "No API requests recorded since server start"
		return data
	}

	// Basic statistics
	data["Total Requests"] = humanize.Comma(since.Requests)

	if since.WallTimeSecs > 0 {
		avgRPS := float64(since.Requests) / since.WallTimeSecs
		data["Average RPS"] = fmt.Sprintf("%.1f req/sec", avgRPS)

		uptimeHours := since.WallTimeSecs / 3600
		data["Uptime Coverage"] = fmt.Sprintf("%.1f hours", uptimeHours)
	}

	// Latency statistics
	if since.Requests > 0 {
		avgLatency := (since.RequestTimeSecs / float64(since.Requests)) * 1000
		data["Average Latency"] = fmt.Sprintf("%.1f ms", avgLatency)

		if since.RequestTimeSecsMax > 0 {
			data["Latency Range"] = fmt.Sprintf("%.1f - %.1f ms",
				since.RequestTimeSecsMin*1000, since.RequestTimeSecsMax*1000)
		}

		// TTFB Analysis
		if since.RespTTFBSecs > 0 {
			avgTTFB := (since.RespTTFBSecs / float64(since.Requests)) * 1000
			data["Avg Time to First Byte"] = fmt.Sprintf("%.1f ms", avgTTFB)
		}
	}

	// Throughput analysis
	totalBytes := since.IncomingBytes + since.OutgoingBytes
	if totalBytes > 0 {
		data["Total Data Processed"] = humanize.Bytes(uint64(totalBytes))
		data["↳ Incoming"] = humanize.Bytes(uint64(since.IncomingBytes))
		data["↳ Outgoing"] = humanize.Bytes(uint64(since.OutgoingBytes))

		if since.Requests > 0 {
			avgBytesPerReq := totalBytes / since.Requests
			data["Avg Bytes per Request"] = humanize.Bytes(uint64(avgBytesPerReq))
		}

		if since.WallTimeSecs > 0 {
			avgThroughputPerSec := float64(totalBytes) / since.WallTimeSecs
			data["Avg Throughput/sec"] = fmt.Sprintf("%s/sec", humanize.Bytes(uint64(avgThroughputPerSec)))
		}
	}

	// Error analysis
	totalErrors := since.Errors4xx + since.Errors5xx
	if totalErrors > 0 {
		errorRate := float64(totalErrors) / float64(since.Requests) * 100
		data["Lifetime Error Rate"] = fmt.Sprintf("%.3f%% (%d errors)", errorRate, totalErrors)
		if since.Errors4xx > 0 {
			data["↳ Client Errors (4xx)"] = humanize.Comma(int64(since.Errors4xx))
		}
		if since.Errors5xx > 0 {
			data["↳ Server Errors (5xx)"] = humanize.Comma(int64(since.Errors5xx))
		}
	} else {
		data["Lifetime Error Rate"] = "0% - No errors recorded"
	}

	// Cancellation analysis
	if since.Canceled > 0 {
		cancelRate := float64(since.Canceled) / float64(since.Requests) * 100
		data["Canceled Requests"] = fmt.Sprintf("%.2f%% (%s)", cancelRate, humanize.Comma(since.Canceled))
	}

	// Blocking analysis
	if since.ReadBlockedSecs > 0 || since.WriteBlockedSecs > 0 {
		data[" "] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ "] = ""
		data["            BLOCKING ANALYSIS"] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ "] = ""

		if since.ReadBlockedSecs > 0 {
			avgReadBlocked := (since.ReadBlockedSecs / float64(since.Requests)) * 1000
			data["Avg Read Blocking"] = fmt.Sprintf("%.1f ms/req", avgReadBlocked)
		}
		if since.WriteBlockedSecs > 0 {
			avgWriteBlocked := (since.WriteBlockedSecs / float64(since.Requests)) * 1000
			data["Avg Write Blocking"] = fmt.Sprintf("%.1f ms/req", avgWriteBlocked)
		}
	}

	data["Responding Nodes"] = fmt.Sprintf("%d nodes", since.Nodes)

	return data
}

func (node *APISinceStartNode) GetMetricType() MetricType   { return MetricsAPI }
func (node *APISinceStartNode) GetMetricFlags() MetricFlags { return 0 }
func (node *APISinceStartNode) GetParent() MetricNode       { return node.parent }
func (node *APISinceStartNode) GetPath() string             { return node.path }
func (node *APISinceStartNode) RequiredMetricTypes() MetricType { return MetricsAPI }
func (node *APISinceStartNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for since_start node")
}

// APIEndpointNode shows detailed statistics for a specific endpoint
type APIEndpointNode struct {
	endpoint string
	stats    APIStats
	parent   MetricNode
	path     string
}

func (node *APIEndpointNode) GetChildren() []MetricChild {
	return []MetricChild{}
}

func (node *APIEndpointNode) GetLeafData() map[string]string {
	data := make(map[string]string)

	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""
	data[fmt.Sprintf("          ENDPOINT: %s", strings.ToUpper(node.endpoint))] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""

	if node.stats.Requests == 0 {
		data["Status"] = "No requests recorded for this endpoint"
		return data
	}

	// Basic statistics
	data["Total Requests"] = humanize.Comma(node.stats.Requests)

	// Timing analysis
	avgLatency := (node.stats.RequestTimeSecs / float64(node.stats.Requests)) * 1000
	data["Average Latency"] = fmt.Sprintf("%.1f ms", avgLatency)

	if node.stats.RequestTimeSecsMax > 0 {
		data["Latency Range"] = fmt.Sprintf("%.1f - %.1f ms",
			node.stats.RequestTimeSecsMin*1000, node.stats.RequestTimeSecsMax*1000)
	}

	// TTFB Analysis
	if node.stats.RespTTFBSecs > 0 {
		avgTTFB := (node.stats.RespTTFBSecs / float64(node.stats.Requests)) * 1000
		data["Avg Time to First Byte"] = fmt.Sprintf("%.1f ms", avgTTFB)
	}

	// Throughput analysis
	totalBytes := node.stats.IncomingBytes + node.stats.OutgoingBytes
	if totalBytes > 0 {
		data["Total Throughput"] = humanize.Bytes(uint64(totalBytes))
		data["↳ Incoming"] = humanize.Bytes(uint64(node.stats.IncomingBytes))
		data["↳ Outgoing"] = humanize.Bytes(uint64(node.stats.OutgoingBytes))

		avgBytesPerReq := totalBytes / node.stats.Requests
		data["Avg Bytes per Request"] = humanize.Bytes(uint64(avgBytesPerReq))
	}

	// Error analysis
	totalErrors := node.stats.Errors4xx + node.stats.Errors5xx
	if totalErrors > 0 {
		errorRate := float64(totalErrors) / float64(node.stats.Requests) * 100
		data["Error Rate"] = fmt.Sprintf("%.2f%% (%d errors)", errorRate, totalErrors)
		if node.stats.Errors4xx > 0 {
			data["↳ Client Errors (4xx)"] = fmt.Sprintf("%d", node.stats.Errors4xx)
		}
		if node.stats.Errors5xx > 0 {
			data["↳ Server Errors (5xx)"] = fmt.Sprintf("%d", node.stats.Errors5xx)
		}
	} else {
		data["Error Rate"] = "0% - No errors"
	}

	// Cancellation analysis
	if node.stats.Canceled > 0 {
		cancelRate := float64(node.stats.Canceled) / float64(node.stats.Requests) * 100
		data["Canceled Requests"] = fmt.Sprintf("%.2f%% (%s)", cancelRate, humanize.Comma(node.stats.Canceled))
	}

	// Rejection analysis
	totalRejected := node.stats.Rejected.Auth + node.stats.Rejected.Header +
					node.stats.Rejected.Invalid + node.stats.Rejected.NotImplemented +
					node.stats.Rejected.RequestsTime
	if totalRejected > 0 {
		data[" "] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ "] = ""
		data["            REJECTION ANALYSIS"] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ "] = ""

		rejectionRate := float64(totalRejected) / float64(node.stats.Requests) * 100
		data["Rejection Rate"] = fmt.Sprintf("%.2f%% (%d)", rejectionRate, totalRejected)

		if node.stats.Rejected.Auth > 0 {
			data["↳ Authentication"] = fmt.Sprintf("%d", node.stats.Rejected.Auth)
		}
		if node.stats.Rejected.Header > 0 {
			data["↳ Header Issues"] = fmt.Sprintf("%d", node.stats.Rejected.Header)
		}
		if node.stats.Rejected.Invalid > 0 {
			data["↳ Invalid Requests"] = fmt.Sprintf("%d", node.stats.Rejected.Invalid)
		}
		if node.stats.Rejected.NotImplemented > 0 {
			data["↳ Not Implemented"] = fmt.Sprintf("%d", node.stats.Rejected.NotImplemented)
		}
		if node.stats.Rejected.RequestsTime > 0 {
			data["↳ Outdated Signatures"] = fmt.Sprintf("%d", node.stats.Rejected.RequestsTime)
		}
	}

	// Health assessment
	score := node.stats.GetHealthScore()
	data["  "] = ""
	data["Health Score"] = fmt.Sprintf("%.1f/10 %s", score, node.stats.GetHealthStatus())

	// Performance recommendations
	recommendations := node.stats.GetRecommendations()
	if len(recommendations) > 0 {
		data["   "] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  "] = ""
		data["              RECOMMENDATIONS"] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  "] = ""

		for i, rec := range recommendations {
			data[fmt.Sprintf("• Recommendation %d", i+1)] = rec
		}
	}

	data["Responding Nodes"] = fmt.Sprintf("%d nodes", node.stats.Nodes)

	return data
}

func (node *APIEndpointNode) GetMetricType() MetricType   { return MetricsAPI }
func (node *APIEndpointNode) GetMetricFlags() MetricFlags { return 0 }
func (node *APIEndpointNode) GetParent() MetricNode       { return node.parent }
func (node *APIEndpointNode) GetPath() string             { return node.path }
func (node *APIEndpointNode) RequiredMetricTypes() MetricType { return MetricsAPI }
func (node *APIEndpointNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for endpoint node")
}

// APISegmentedNode shows segmented statistics for a specific endpoint over the last day
type APISegmentedNode struct {
	endpoint  string
	segmented SegmentedAPIMetrics
	parent    MetricNode
	path      string
}

func (node *APISegmentedNode) GetChildren() []MetricChild {
	var children []MetricChild
	for i := range node.segmented.Segments {
		children = append(children, MetricChild{
			Name:        fmt.Sprintf("segment_%d", i),
			Description: fmt.Sprintf("Time segment %d statistics", i),
		})
	}
	return children
}

func (node *APISegmentedNode) GetLeafData() map[string]string {
	data := make(map[string]string)

	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""
	data[fmt.Sprintf("      SEGMENTED: %s", strings.ToUpper(node.endpoint))] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""

	data["Segment Count"] = fmt.Sprintf("%d segments", len(node.segmented.Segments))
	data["Interval"] = fmt.Sprintf("%d seconds", node.segmented.Interval)

	// Aggregate statistics
	var totalRequests int64
	var totalErrors int
	var totalBytes int64
	var totalLatency float64

	for _, segment := range node.segmented.Segments {
		totalRequests += segment.Requests
		totalErrors += segment.Errors4xx + segment.Errors5xx
		totalBytes += segment.IncomingBytes + segment.OutgoingBytes
		totalLatency += segment.RequestTimeSecs
	}

	if totalRequests > 0 {
		avgLatency := (totalLatency / float64(totalRequests)) * 1000
		data["Total Requests"] = humanize.Comma(totalRequests)
		data["Average Latency"] = fmt.Sprintf("%.1f ms", avgLatency)

		if totalErrors > 0 {
			errorRate := float64(totalErrors) / float64(totalRequests) * 100
			data["Error Rate"] = fmt.Sprintf("%.2f%% (%d errors)", errorRate, totalErrors)
		}

		if totalBytes > 0 {
			data["Total Throughput"] = humanize.Bytes(uint64(totalBytes))
		}

		// Calculate request rate per segment
		avgReqPerSegment := float64(totalRequests) / float64(len(node.segmented.Segments))
		data["Avg Requests/Segment"] = fmt.Sprintf("%.1f", avgReqPerSegment)
	}

	return data
}

func (node *APISegmentedNode) GetMetricType() MetricType   { return MetricsAPI }
func (node *APISegmentedNode) GetMetricFlags() MetricFlags { return 0 }
func (node *APISegmentedNode) GetParent() MetricNode       { return node.parent }
func (node *APISegmentedNode) GetPath() string             { return node.path }
func (node *APISegmentedNode) RequiredMetricTypes() MetricType { return MetricsAPI }
func (node *APISegmentedNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("segmented endpoint children not yet implemented: %s", name)
}

type ReplicationMetricsNode struct {
	repl   *ReplicationMetrics
	parent MetricNode
	path   string
}

func (node *ReplicationMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "active", Description: "Active replication events"},
		{Name: "queued", Description: "Queued replication events"},
		{Name: "targets", Description: "Replication statistics by target"},
	}
}
func (node *ReplicationMetricsNode) GetLeafData() map[string]string { return nil }
func (node *ReplicationMetricsNode) GetMetricType() MetricType   { return MetricsReplication }
func (node *ReplicationMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *ReplicationMetricsNode) GetParent() MetricNode { return node.parent }
func (node *ReplicationMetricsNode) GetPath() string { return node.path }
func (node *ReplicationMetricsNode) RequiredMetricTypes() MetricType { return MetricsReplication }
func (node *ReplicationMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("replication metric sub-navigation not yet implemented for: %s", name)
}

type ProcessMetricsNode struct {
	process *ProcessMetrics
	parent  MetricNode
	path    string
}

func (node *ProcessMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "cpu", Description: "Process CPU usage"},
		{Name: "memory", Description: "Process memory usage"},
		{Name: "io", Description: "Process IO statistics"},
		{Name: "context_switches", Description: "Process context switch statistics"},
		{Name: "page_faults", Description: "Process page fault statistics"},
	}
}
func (node *ProcessMetricsNode) GetLeafData() map[string]string { return nil }
func (node *ProcessMetricsNode) GetMetricType() MetricType   { return MetricsProcess }
func (node *ProcessMetricsNode) GetMetricFlags() MetricFlags { return 0 }
func (node *ProcessMetricsNode) GetParent() MetricNode { return node.parent }
func (node *ProcessMetricsNode) GetPath() string { return node.path }
func (node *ProcessMetricsNode) RequiredMetricTypes() MetricType { return MetricsProcess }
func (node *ProcessMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("process metric sub-navigation not yet implemented for: %s", name)
}


// generateAPIOverviewDashboard creates an executive-level API performance dashboard
func (node *APIMetricsNode) generateAPIOverviewDashboard() map[string]string {
	data := make(map[string]string)

	// === API HEALTH SUMMARY ===
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""
	data["                API HEALTH SUMMARY"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""

	lastMinute := node.api.LastMinuteTotal()
	healthScore := node.calculateAPIHealthScore(&lastMinute)
	data["Overall API Health Score"] = fmt.Sprintf("%.1f/10 %s", healthScore, node.getHealthStatus(healthScore))
	data["Active Nodes"] = fmt.Sprintf("%d nodes responding", node.api.Nodes)
	data["Collection Time"] = node.api.CollectedAt.Format("15:04:05")

	// Request queue information
	data["Active Requests"] = humanize.Comma(node.api.ActiveRequests)
	data["Queued Requests"] = humanize.Comma(node.api.QueuedRequests)

	totalQueue := node.api.ActiveRequests + node.api.QueuedRequests
	data["Total Queue Depth"] = humanize.Comma(totalQueue)

	// === PERFORMANCE OVERVIEW ===
	data[""] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ "] = ""
	data["            PERFORMANCE OVERVIEW"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ "] = ""

	// Last minute performance
	if lastMinute.Requests > 0 {
		avgLatency := (lastMinute.RequestTimeSecs / float64(lastMinute.Requests)) * 1000
		data["Request Rate (Last Minute)"] = fmt.Sprintf("%s req/min", humanize.Comma(lastMinute.Requests))
		data["Average Latency"] = fmt.Sprintf("%.1f ms", avgLatency)

		if lastMinute.RequestTimeSecsMax > 0 {
			data["Latency Range"] = fmt.Sprintf("%.1f - %.1f ms",
				lastMinute.RequestTimeSecsMin*1000,
				lastMinute.RequestTimeSecsMax*1000)
		}

		// TTFB Analysis
		if lastMinute.RespTTFBSecs > 0 {
			avgTTFB := (lastMinute.RespTTFBSecs / float64(lastMinute.Requests)) * 1000
			data["Avg Time to First Byte"] = fmt.Sprintf("%.1f ms", avgTTFB)
		}
	} else {
		data["Request Rate (Last Minute)"] = "No requests"
	}

	// Throughput analysis
	totalBytes := lastMinute.IncomingBytes + lastMinute.OutgoingBytes
	if totalBytes > 0 {
		data["Throughput (Last Minute)"] = fmt.Sprintf("%s/min", humanize.Bytes(uint64(totalBytes)))
		data["↳ Incoming"] = fmt.Sprintf("%s/min", humanize.Bytes(uint64(lastMinute.IncomingBytes)))
		data["↳ Outgoing"] = fmt.Sprintf("%s/min", humanize.Bytes(uint64(lastMinute.OutgoingBytes)))
	}

	// === ERROR ANALYSIS ===
	data["  "] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  "] = ""
	data["              ERROR ANALYSIS"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  "] = ""

	totalErrors := lastMinute.Errors4xx + lastMinute.Errors5xx
	if totalErrors > 0 || lastMinute.Requests > 0 {
		errorRate := float64(totalErrors) / float64(lastMinute.Requests) * 100
		if lastMinute.Requests == 0 {
			errorRate = 0
		}
		data["Error Rate (Last Minute)"] = fmt.Sprintf("%.2f%% (%d errors)", errorRate, totalErrors)

		if lastMinute.Errors4xx > 0 {
			data["↳ 4xx Client Errors"] = fmt.Sprintf("%d (%.1f%%)",
				lastMinute.Errors4xx,
				float64(lastMinute.Errors4xx)/float64(apiMaxInt(totalErrors, 1))*100)
		}
		if lastMinute.Errors5xx > 0 {
			data["↳ 5xx Server Errors"] = fmt.Sprintf("%d (%.1f%%)",
				lastMinute.Errors5xx,
				float64(lastMinute.Errors5xx)/float64(apiMaxInt(totalErrors, 1))*100)
		}
		if lastMinute.Canceled > 0 {
			data["↳ Canceled Requests"] = fmt.Sprintf("%d", lastMinute.Canceled)
		}
	} else {
		data["Error Rate (Last Minute)"] = "No errors detected"
	}

	// Rejection analysis
	rejections := lastMinute.Rejected
	totalRejected := rejections.Auth + rejections.Header + rejections.Invalid + rejections.NotImplemented + rejections.RequestsTime
	if totalRejected > 0 {
		data["Rejected Requests"] = fmt.Sprintf("%d rejections", totalRejected)
		if rejections.Auth > 0 {
			data["↳ Authentication"] = fmt.Sprintf("%d", rejections.Auth)
		}
		if rejections.Header > 0 {
			data["↳ Header Issues"] = fmt.Sprintf("%d", rejections.Header)
		}
		if rejections.Invalid > 0 {
			data["↳ Invalid Requests"] = fmt.Sprintf("%d", rejections.Invalid)
		}
		if rejections.NotImplemented > 0 {
			data["↳ Not Implemented"] = fmt.Sprintf("%d", rejections.NotImplemented)
		}
	}

	// === LIFETIME INSIGHTS ===
	data["   "] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   "] = ""
	data["            LIFETIME INSIGHTS"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   "] = ""

	since := node.api.SinceStart
	if since.Requests > 0 {
		data["Total Requests"] = humanize.Comma(since.Requests)
		data["Total Data Processed"] = humanize.Bytes(uint64(since.IncomingBytes + since.OutgoingBytes))

		lifetimeErrorRate := float64(since.Errors4xx + since.Errors5xx) / float64(since.Requests) * 100
		data["Lifetime Error Rate"] = fmt.Sprintf("%.3f%%", lifetimeErrorRate)

		if since.WallTimeSecs > 0 {
			avgRPS := float64(since.Requests) / since.WallTimeSecs
			data["Average RPS"] = fmt.Sprintf("%.1f req/sec", avgRPS)
		}
	}

	// === ENDPOINT ANALYSIS ===
	endpointCount := len(node.api.LastMinuteAPI)
	if endpointCount > 0 {
		data["    "] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━    "] = ""
		data["            ENDPOINT ANALYSIS"] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━    "] = ""

		data["Active Endpoints"] = fmt.Sprintf("%d endpoints receiving traffic", endpointCount)

		// Find top endpoints by request count
		type endpointStat struct {
			name string
			stats APIStats
		}

		var endpoints []endpointStat
		for name, stats := range node.api.LastMinuteAPI {
			endpoints = append(endpoints, endpointStat{name, stats})
		}

		// Sort by request count
		for i := 0; i < len(endpoints)-1; i++ {
			for j := i + 1; j < len(endpoints); j++ {
				if endpoints[i].stats.Requests < endpoints[j].stats.Requests {
					endpoints[i], endpoints[j] = endpoints[j], endpoints[i]
				}
			}
		}

		// Show top 5 busiest endpoints
		maxShow := min(5, len(endpoints))
		for i := 0; i < maxShow; i++ {
			ep := endpoints[i]
			if ep.stats.Requests > 0 {
				avgLatency := (ep.stats.RequestTimeSecs / float64(ep.stats.Requests)) * 1000
				errors := ep.stats.Errors4xx + ep.stats.Errors5xx
				data[fmt.Sprintf("↳ %s", ep.name)] = fmt.Sprintf("%s req, %.1fms avg, %d err",
					humanize.Comma(ep.stats.Requests), avgLatency, errors)
			}
		}
	}

	// === RECOMMENDATIONS ===
	recommendations := node.generateAPIRecommendations(&lastMinute, &since)
	if len(recommendations) > 0 {
		data["     "] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━     "] = ""
		data["              RECOMMENDATIONS"] = ""
		data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━     "] = ""

		for i, rec := range recommendations {
			data[fmt.Sprintf("• Recommendation %d", i+1)] = rec
		}
	}

	return data
}

// calculateAPIHealthScore computes health score based on error rates, latency, and queue status
func (node *APIMetricsNode) calculateAPIHealthScore(lastMinute *APIStats) float64 {
	score := 10.0

	if lastMinute.Requests == 0 {
		return 8.0 // Neutral score for no activity
	}

	// Error rate penalty
	errorRate := float64(lastMinute.Errors4xx + lastMinute.Errors5xx) / float64(lastMinute.Requests) * 100
	if errorRate > 5.0 {
		score -= 3.0
	} else if errorRate > 1.0 {
		score -= 1.0
	}

	// Latency penalty
	avgLatency := (lastMinute.RequestTimeSecs / float64(lastMinute.Requests)) * 1000
	if avgLatency > 5000 {
		score -= 2.0
	} else if avgLatency > 1000 {
		score -= 1.0
	}

	// Queue buildup penalty
	if node.api.QueuedRequests > 100 {
		score -= 2.0
	} else if node.api.QueuedRequests > 10 {
		score -= 1.0
	}

	// High rejection penalty
	totalRejected := lastMinute.Rejected.Auth + lastMinute.Rejected.Header +
		lastMinute.Rejected.Invalid + lastMinute.Rejected.NotImplemented + lastMinute.Rejected.RequestsTime
	if totalRejected > 0 {
		rejectionRate := float64(totalRejected) / float64(lastMinute.Requests) * 100
		if rejectionRate > 10.0 {
			score -= 2.0
		} else if rejectionRate > 5.0 {
			score -= 1.0
		}
	}

	if score < 0.0 {
		return 0.0
	}
	return score
}

// getHealthStatus returns health status string based on score
func (node *APIMetricsNode) getHealthStatus(score float64) string {
	switch {
	case score >= 9.0:
		return "🟢 EXCELLENT"
	case score >= 8.0:
		return "🟢 GOOD"
	case score >= 6.0:
		return "🟡 FAIR"
	case score >= 4.0:
		return "🟠 POOR"
	default:
		return "🔴 CRITICAL"
	}
}

// generateAPIRecommendations provides actionable insights
func (node *APIMetricsNode) generateAPIRecommendations(lastMinute, sinceStart *APIStats) []string {
	var recommendations []string

	if lastMinute.Requests == 0 {
		return []string{"No recent API activity to analyze"}
	}

	// Error rate recommendations
	errorRate := float64(lastMinute.Errors4xx + lastMinute.Errors5xx) / float64(lastMinute.Requests) * 100
	if errorRate > 5.0 {
		recommendations = append(recommendations, "High error rate detected - investigate failing endpoints")
	}

	if lastMinute.Errors5xx > lastMinute.Errors4xx && lastMinute.Errors5xx > 0 {
		recommendations = append(recommendations, "Server errors exceed client errors - check system health")
	}

	// Latency recommendations
	avgLatency := (lastMinute.RequestTimeSecs / float64(lastMinute.Requests)) * 1000
	if avgLatency > 2000 {
		recommendations = append(recommendations, "High average latency - consider performance optimization")
	}

	if lastMinute.RequestTimeSecsMax > 0 && lastMinute.RequestTimeSecsMax*1000 > avgLatency*5 {
		recommendations = append(recommendations, "High latency variance detected - investigate slow endpoints")
	}

	// Queue recommendations
	if node.api.QueuedRequests > 50 {
		recommendations = append(recommendations, "Request queue buildup - consider scaling or load balancing")
	}

	// TTFB recommendations
	if lastMinute.RespTTFBSecs > 0 && lastMinute.Requests > 0 {
		avgTTFB := (lastMinute.RespTTFBSecs / float64(lastMinute.Requests)) * 1000
		if avgTTFB > 500 {
			recommendations = append(recommendations, "Slow time-to-first-byte - optimize request processing")
		}
	}

	// Rejection recommendations
	rejections := lastMinute.Rejected
	if rejections.Auth > 0 {
		recommendations = append(recommendations, "Authentication failures detected - verify client credentials")
	}

	if rejections.Invalid > 0 {
		recommendations = append(recommendations, "Invalid request signatures - check client request formatting")
	}

	// Throughput recommendations
	if len(node.api.LastMinuteAPI) > 10 {
		recommendations = append(recommendations, "High endpoint diversity - monitor for unused or deprecated APIs")
	}

	return recommendations
}

// Helper function for API metrics
func apiMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
