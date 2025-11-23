package madmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

	// Show error summary (limited to first 3 errors)
	for i, err := range node.metrics.Errors {
		if i < 3 {
			data[fmt.Sprintf("Error %d", i+1)] = err
		}
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
	// Individual metric types - route directly from root
	case "scanner":
		return NewScannerMetricsNode(node.metrics.Aggregated.Scanner, node, "scanner"), nil
	case "disk":
		return NewDiskMetricsNavigator(node.metrics.Aggregated.Disk, node, "disk"), nil
	case "os":
		return NewOSMetricsNavigator(node.metrics.Aggregated.OS, node, "os"), nil
	case "batch_jobs":
		return &BatchJobMetricsNode{batch: node.metrics.Aggregated.BatchJobs, parent: node, path: "batch_jobs"}, nil
	case "site_resync":
		return &SiteResyncMetricsNode{resync: node.metrics.Aggregated.SiteResync, parent: node, path: "site_resync"}, nil
	case "net":
		return NewNetMetricsNavigator(node.metrics.Aggregated.Net, node, "net"), nil
	case "mem":
		return NewMemMetricsNavigator(node.metrics.Aggregated.Mem, node, "mem"), nil
	case "cpu":
		return NewCPUMetricsNavigator(node.metrics.Aggregated.CPU, node, "cpu"), nil
	case "rpc":
		return &RPCMetricsNode{rpc: node.metrics.Aggregated.RPC, parent: node, path: "rpc"}, nil
	case "go":
		return NewRuntimeMetricsNavigator(node.metrics.Aggregated.Go, node, "go"), nil
	case "api":
		return &APIMetricsNode{api: node.metrics.Aggregated.API, parent: node, path: "api"}, nil
	case "replication":
		return NewReplicationMetricsNode(node.metrics.Aggregated.Replication, node, "replication"), nil
	case "process":
		return NewProcessMetricsNode(node.metrics.Aggregated.Process, node, "process"), nil

	// Grouping nodes - preserved as-is
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
					return NewDiskMetricsNavigator(&diskMetric, node, fmt.Sprintf("by_disk/%s", key))
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
		return NewMemMetricsNavigator(node.metrics.Mem, node, fmt.Sprintf("%s/mem", node.path)), nil
	case "cpu":
		return NewCPUMetricsNavigator(node.metrics.CPU, node, fmt.Sprintf("%s/cpu", node.path)), nil
	case "rpc":
		return &RPCMetricsNode{rpc: node.metrics.RPC, parent: node, path: fmt.Sprintf("%s/rpc", node.path)}, nil
	case "go":
		return NewRuntimeMetricsNavigator(node.metrics.Go, node, fmt.Sprintf("%s/go", node.path)), nil
	case "api":
		return &APIMetricsNode{api: node.metrics.API, parent: node, path: fmt.Sprintf("%s/api", node.path)}, nil
	case "replication":
		return NewReplicationMetricsNode(node.metrics.Replication, node, fmt.Sprintf("%s/replication", node.path)), nil
	case "process":
		return NewProcessMetricsNode(node.metrics.Process, node, fmt.Sprintf("%s/process", node.path)), nil
	default:
		return nil, fmt.Errorf("child not found: %s", name)
	}
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
		var keys []string
		// Extract and sort keys
		for k := range data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// Create children in sorted order
		for _, k := range keys {
			children = append(children, MetricChild{Name: k, Description: fmt.Sprintf("Metrics for %s", k)})
		}
		return children
	case map[string]DiskMetric:
		var children []MetricChild
		var keys []string
		// Extract and sort keys
		for k := range data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// Create children in sorted order
		for _, k := range keys {
			children = append(children, MetricChild{Name: k, Description: fmt.Sprintf("Disk metrics for %s", k)})
		}
		return children
	case map[int]map[int]DiskMetric:
		var children []MetricChild
		var keys []int
		// Extract and sort keys
		for k := range data {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		// Create children in sorted order
		for _, k := range keys {
			children = append(children, MetricChild{Name: fmt.Sprintf("%d", k), Description: fmt.Sprintf("Disk set %d", k)})
		}
		return children
	default:
		return []MetricChild{}
	}
}

func (node *MapNode) GetLeafData() map[string]string {
	// Return empty data - no information displayed for by_host/by_disk navigation
	return map[string]string{}
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
	for poolID, pool := range node.data {
		// Calculate pool-level statistics for better description
		var poolDisks int
		var poolSets int = len(pool)
		var poolHealthyDisks int
		var poolCurrentIOs uint64
		for _, diskSet := range pool {
			poolDisks += diskSet.NDisks
			poolHealthyDisks += (diskSet.NDisks - diskSet.Offline - diskSet.Hanging - diskSet.Healing)
			poolCurrentIOs += diskSet.IOStatsMinute.CurrentIOs
		}

		description := fmt.Sprintf("Pool %d with %d sets, %d disks (%d healthy), %d current IOs",
			poolID, poolSets, poolDisks, poolHealthyDisks, poolCurrentIOs)

		children = append(children, MetricChild{
			Name:        fmt.Sprintf("pool_%d", poolID),
			Description: description,
		})
	}
	return children
}

func (node *DiskSetMapNode) GetLeafData() map[string]string {
	data := map[string]string{}

	// Calculate aggregated statistics across all pools and sets
	var totalSets int
	var totalDisks int
	var totalHealthyDisks int
	var totalOfflineDisks int
	var totalHealingDisks int
	var totalHangingDisks int
	var totalCapacity, totalUsed uint64
	var totalOps uint64
	var totalBytes uint64

	// First pass: calculate pool-level aggregated metrics
	poolMetrics := make(map[int]struct {
		sets    int
		ops     uint64
		accTime float64
		ioOps   uint64
		ioBytes uint64
	})

	for poolID, pool := range node.data {
		poolStat := poolMetrics[poolID]
		for setID, diskSet := range pool {
			totalSets++
			totalDisks += diskSet.NDisks
			totalHealthyDisks += (diskSet.NDisks - diskSet.Offline - diskSet.Hanging - diskSet.Healing)
			totalOfflineDisks += diskSet.Offline
			totalHealingDisks += diskSet.Healing
			totalHangingDisks += diskSet.Hanging

			// Aggregate storage space
			totalCapacity += diskSet.Space.Free.Total + diskSet.Space.Used.Total
			totalUsed += diskSet.Space.Used.Total

			// Aggregate operations from last minute for cluster totals
			for _, action := range diskSet.LastMinute {
				totalOps += action.Count
				totalBytes += action.Bytes
				poolStat.ops += action.Count
				poolStat.accTime += action.AccTime
			}

			// Aggregate pool-level metrics
			poolStat.sets++

			// Aggregate current IO statistics for this pool
			ioStat := diskSet.IOStatsMinute
			poolStat.ioOps += ioStat.ReadIOs + ioStat.WriteIOs + ioStat.DiscardIOs + ioStat.FlushIOs
			poolStat.ioBytes += ioStat.ReadSectors + ioStat.WriteSectors + ioStat.DiscardSectors // Sectors represent data transferred

			_ = setID // Mark as used
		}
		poolMetrics[poolID] = poolStat
	}

	// Second pass: create pool-level display entries
	for poolID, poolStat := range poolMetrics {
		var opsDisplay string
		var ioDisplay string

		// Calculate performance metrics for this pool
		if poolStat.ops > 0 && poolStat.accTime > 0 {
			opsPerSec := float64(poolStat.ops) / poolStat.accTime
			avgTimeMs := (poolStat.accTime / float64(poolStat.ops)) * 1000
			opsDisplay = fmt.Sprintf("%.1f ops/s, %.2fms avg", opsPerSec, avgTimeMs)
		} else {
			opsDisplay = "No recent activity"
		}

		// Calculate current IO metrics for this pool
		if poolStat.ioOps > 0 || poolStat.ioBytes > 0 {
			ioDisplay = fmt.Sprintf(", %s IO/s", humanize.Bytes(poolStat.ioBytes))
		} else {
			ioDisplay = ", No current IO"
		}

		poolLabel := fmt.Sprintf("Pool %d", poolID)
		poolValue := fmt.Sprintf("%s%s (%d sets)", opsDisplay, ioDisplay, poolStat.sets)
		data[poolLabel] = poolValue
	}

	// Summary statistics
	data["00:Cluster Summary"] = fmt.Sprintf("%d pools, %d sets, %d total disks",
		len(node.data), totalSets, totalDisks)

	if totalDisks > 0 {
		healthPercent := float64(totalHealthyDisks) / float64(totalDisks) * 100.0
		var healthStatus string
		switch {
		case healthPercent >= 95:
			healthStatus = "Excellent"
		case healthPercent >= 85:
			healthStatus = "Good"
		case healthPercent >= 70:
			healthStatus = "Warning"
		default:
			healthStatus = "Critical"
		}

		data["01:Disk Health"] = fmt.Sprintf("%s - %d of %d disks healthy (%.1f%%)",
			healthStatus, totalHealthyDisks, totalDisks, healthPercent)

		if totalOfflineDisks > 0 || totalHangingDisks > 0 || totalHealingDisks > 0 {
			var issues []string
			if totalOfflineDisks > 0 {
				issues = append(issues, fmt.Sprintf("%d offline", totalOfflineDisks))
			}
			if totalHangingDisks > 0 {
				issues = append(issues, fmt.Sprintf("%d hanging", totalHangingDisks))
			}
			if totalHealingDisks > 0 {
				issues = append(issues, fmt.Sprintf("%d healing", totalHealingDisks))
			}
			data["02:Issues"] = strings.Join(issues, ", ")
		}
	}

	// Storage capacity summary
	if totalCapacity > 0 {
		usagePercent := float64(totalUsed) / float64(totalCapacity) * 100.0
		data["03:Storage Capacity"] = fmt.Sprintf("%s used of %s total (%.1f%% used)",
			humanize.Bytes(totalUsed), humanize.Bytes(totalCapacity), usagePercent)
	}

	// Activity summary
	if totalOps > 0 {
		data["03:Recent Activity"] = fmt.Sprintf("%s operations, %s transferred (last minute)",
			humanize.Comma(int64(totalOps)), humanize.Bytes(totalBytes))
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
		return NewDiskSetPoolNavigator(poolID, sets, node.metricType, node.metricFlags, node, fmt.Sprintf("%s/pool_%d", node.path, poolID)), nil
	}

	return nil, fmt.Errorf("pool not found: %d", poolID)
}

// DiskSetPoolNavigator provides enhanced navigation for disk set pools
type DiskSetPoolNavigator struct {
	poolID      int
	poolSets    map[int]DiskMetric
	metricType  MetricType
	metricFlags MetricFlags
	parent      MetricNode
	path        string
}

func NewDiskSetPoolNavigator(poolID int, poolSets map[int]DiskMetric, metricType MetricType, metricFlags MetricFlags, parent MetricNode, path string) *DiskSetPoolNavigator {
	return &DiskSetPoolNavigator{
		poolID:      poolID,
		poolSets:    poolSets,
		metricType:  metricType,
		metricFlags: metricFlags,
		parent:      parent,
		path:        path,
	}
}

func (node *DiskSetPoolNavigator) ShouldPauseUpdates() bool {
	return false
}

func (node *DiskSetPoolNavigator) GetChildren() []MetricChild {
	var children []MetricChild
	for setID, diskSet := range node.poolSets {
		healthyDisks := diskSet.NDisks - diskSet.Offline - diskSet.Hanging - diskSet.Healing
		currentIOs := diskSet.IOStatsMinute.CurrentIOs
		description := fmt.Sprintf("Set %d with %d disks (%d healthy), %d current IOs",
			setID, diskSet.NDisks, healthyDisks, currentIOs)

		children = append(children, MetricChild{
			Name:        fmt.Sprintf("set_%d", setID),
			Description: description,
		})
	}

	// Sort children by set ID for consistent ordering
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})

	return children
}

func (node *DiskSetPoolNavigator) GetLeafData() map[string]string {
	data := map[string]string{}

	// Pool-level aggregated statistics
	var totalSets int = len(node.poolSets)
	var totalDisks int
	var totalHealthyDisks int
	var totalOfflineDisks int
	var totalHealingDisks int
	var totalHangingDisks int
	var totalCapacity, totalUsed uint64
	var totalOps uint64
	var totalBytes uint64

	for setID, diskSet := range node.poolSets {
		totalDisks += diskSet.NDisks
		totalHealthyDisks += (diskSet.NDisks - diskSet.Offline - diskSet.Hanging - diskSet.Healing)
		totalOfflineDisks += diskSet.Offline
		totalHealingDisks += diskSet.Healing
		totalHangingDisks += diskSet.Hanging

		// Aggregate storage space
		totalCapacity += diskSet.Space.Free.Total + diskSet.Space.Used.Total
		totalUsed += diskSet.Space.Used.Total

		// Aggregate operations from last minute
		for _, action := range diskSet.LastMinute {
			totalOps += action.Count
			totalBytes += action.Bytes
		}

		// Individual set performance metrics
		var setOps uint64
		var setTime float64
		for _, action := range diskSet.LastMinute {
			setOps += action.Count
			setTime += action.AccTime
		}

		var opsDisplay string
		if setOps > 0 && setTime > 0 {
			opsPerSec := float64(setOps) / setTime
			avgTimeMs := (setTime / float64(setOps)) * 1000 // Convert to milliseconds
			opsDisplay = fmt.Sprintf("%.1f ops/s, %.2fms avg time", opsPerSec, avgTimeMs)
		} else {
			opsDisplay = "No recent activity"
		}

		data[fmt.Sprintf("Set %d", setID)] = opsDisplay
	}

	// Pool summary
	data["00:Pool Summary"] = fmt.Sprintf("Pool %d: %d sets, %d total disks",
		node.poolID, totalSets, totalDisks)

	if totalDisks > 0 {
		healthPercent := float64(totalHealthyDisks) / float64(totalDisks) * 100.0
		data["01:Pool Health"] = fmt.Sprintf("%d of %d disks healthy (%.1f%%)",
			totalHealthyDisks, totalDisks, healthPercent)

		if totalOfflineDisks > 0 || totalHangingDisks > 0 || totalHealingDisks > 0 {
			var issues []string
			if totalOfflineDisks > 0 {
				issues = append(issues, fmt.Sprintf("%d offline", totalOfflineDisks))
			}
			if totalHangingDisks > 0 {
				issues = append(issues, fmt.Sprintf("%d hanging", totalHangingDisks))
			}
			if totalHealingDisks > 0 {
				issues = append(issues, fmt.Sprintf("%d healing", totalHealingDisks))
			}
			data["02:Pool Issues"] = strings.Join(issues, ", ")
		}
	}

	// Pool storage capacity
	if totalCapacity > 0 {
		usagePercent := float64(totalUsed) / float64(totalCapacity) * 100.0
		data["03:Pool Storage"] = fmt.Sprintf("%s used of %s total (%.1f%% used)",
			humanize.Bytes(totalUsed), humanize.Bytes(totalCapacity), usagePercent)
	}

	// Pool activity summary
	if totalOps > 0 {
		data["04:Pool Activity"] = fmt.Sprintf("%s operations, %s transferred (last minute)",
			humanize.Comma(int64(totalOps)), humanize.Bytes(totalBytes))
	}

	return data
}

func (node *DiskSetPoolNavigator) GetMetricType() MetricType       { return node.metricType }
func (node *DiskSetPoolNavigator) GetMetricFlags() MetricFlags     { return node.metricFlags }
func (node *DiskSetPoolNavigator) GetParent() MetricNode           { return node.parent }
func (node *DiskSetPoolNavigator) GetPath() string                 { return node.path }
func (node *DiskSetPoolNavigator) RequiredMetricTypes() MetricType { return node.metricType }
func (node *DiskSetPoolNavigator) ShouldPauseRefresh() bool        { return false }

func (node *DiskSetPoolNavigator) GetChild(name string) (MetricNode, error) {
	if !strings.HasPrefix(name, "set_") {
		return nil, fmt.Errorf("invalid set name format: %s", name)
	}

	setIDStr := strings.TrimPrefix(name, "set_")
	var setID int
	if _, err := fmt.Sscanf(setIDStr, "%d", &setID); err != nil {
		return nil, fmt.Errorf("invalid set ID: %s", setIDStr)
	}

	if diskMetric, exists := node.poolSets[setID]; exists {
		return NewDiskMetricsNavigator(&diskMetric, node, fmt.Sprintf("%s/set_%d", node.path, setID)), nil
	}

	return nil, fmt.Errorf("set not found: %d", setID)
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
