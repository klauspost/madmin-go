package madmin

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// DiskMetricsNavigator provides enhanced navigation for disk metrics
type DiskMetricsNavigator struct {
	disk   *DiskMetric
	parent MetricNode
	path   string
}

// NewDiskMetricsNavigator creates a new enhanced disk metrics navigator
func NewDiskMetricsNavigator(disk *DiskMetric, parent MetricNode, path string) *DiskMetricsNavigator {
	return &DiskMetricsNavigator{disk: disk, parent: parent, path: path}
}

func (node *DiskMetricsNavigator) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "ops_last_minute", Description: "Last minute disk operations by type"},
		{Name: "ops_last_day", Description: "Last day segmented disk operations"},
		{Name: "ops_lifetime", Description: "Lifetime disk operations by type"},
		{Name: "io_last_minute", Description: "Last minute IO statistics (real-time)"},
		{Name: "io_last_day", Description: "Daily IO statistics (historical)"},
		{Name: "space", Description: "Disk space information"},
		{Name: "healing", Description: "Disk healing information"},
		{Name: "cache", Description: "Disk cache statistics"},
	}
}

func (node *DiskMetricsNavigator) GetLeafData() map[string]string {
	if node.disk == nil {
		return map[string]string{"Error": "disk metrics not available"}
	}

	data := map[string]string{}

	// Disk Overview
	data["DISK OVERVIEW"] = fmt.Sprintf("Collected at %s",
		node.disk.CollectedAt.Format("2006-01-02 15:04:05"))

	// Disk Health Status
	totalDisks := node.disk.NDisks
	healthyDisks := totalDisks - node.disk.Offline - node.disk.Hanging - node.disk.Healing

	if totalDisks > 0 {
		healthPercent := float64(healthyDisks) / float64(totalDisks) * 100.0
		data["Drive Health"] = fmt.Sprintf("%d healthy, %d total (%.1f%% healthy)",
			healthyDisks, totalDisks, healthPercent)

		if node.disk.Offline > 0 {
			data["Offline Drives"] = fmt.Sprintf("%d drives offline", node.disk.Offline)
		}
		if node.disk.Hanging > 0 {
			data["Hanging Drives"] = fmt.Sprintf("%d drives hanging", node.disk.Hanging)
		}
		if node.disk.Healing > 0 {
			data["Healing Drives"] = fmt.Sprintf("%d drives healing", node.disk.Healing)
		}
	}

	// Location Information
	var locationParts []string
	if node.disk.PoolIdx != nil {
		locationParts = append(locationParts, fmt.Sprintf("Pool %d", *node.disk.PoolIdx))
	}
	if node.disk.SetIdx != nil {
		locationParts = append(locationParts, fmt.Sprintf("Set %d", *node.disk.SetIdx))
	}
	if node.disk.DiskIdx != nil {
		locationParts = append(locationParts, fmt.Sprintf("Disk %d", *node.disk.DiskIdx))
	}
	if len(locationParts) > 0 {
		data["Location"] = strings.Join(locationParts, ", ")
	}

	// Storage Space Summary
	if node.disk.Space.N > 0 {
		totalSpace := node.disk.Space.Free.Total + node.disk.Space.Used.Total
		if totalSpace > 0 {
			usagePercent := float64(node.disk.Space.Used.Total) / float64(totalSpace) * 100.0
			data["Storage Usage"] = fmt.Sprintf("%s used, %s free (%.1f%% used)",
				humanize.Bytes(node.disk.Space.Used.Total),
				humanize.Bytes(node.disk.Space.Free.Total),
				usagePercent)
		}

		if node.disk.Space.N > 1 {
			data["Space Distribution"] = fmt.Sprintf("Min: %s free, Max: %s free across %d drives",
				humanize.Bytes(node.disk.Space.Free.Min),
				humanize.Bytes(node.disk.Space.Free.Max),
				node.disk.Space.N)
		}

		// Inode usage
		totalInodes := node.disk.Space.UsedInodes.Total + node.disk.Space.FreeInodes.Total
		if totalInodes > 0 {
			inodePercent := float64(node.disk.Space.UsedInodes.Total) / float64(totalInodes) * 100.0
			data["Inode Usage"] = fmt.Sprintf("%s used, %s free (%.1f%% used)",
				humanize.Comma(int64(node.disk.Space.UsedInodes.Total)),
				humanize.Comma(int64(node.disk.Space.FreeInodes.Total)),
				inodePercent)
		}
	}

	// Disk State Issues
	if len(node.disk.State) > 0 {
		var totalIssues int
		for _, count := range node.disk.State {
			totalIssues += count
		}
		if totalIssues > 0 {
			data["State Issues"] = fmt.Sprintf("%d issues across %d state types",
				totalIssues, len(node.disk.State))
		}
	}

	// OPERATIONS SUMMARY - Last minute totals
	if len(node.disk.LastMinute) > 0 {
		var totalOps uint64
		var totalBytes uint64
		var totalTime float64

		for _, action := range node.disk.LastMinute {
			totalOps += action.Count
			totalBytes += action.Bytes
			totalTime += action.AccTime
		}

		if totalOps > 0 && totalTime > 0 {
			opsPerSec := float64(totalOps) / totalTime
			mbPerSec := float64(totalBytes) / totalTime / (1024 * 1024)

			data["OPERATIONS SUMMARY"] = fmt.Sprintf("%.1f OPS/sec, %.2f MB/s (last minute totals)", opsPerSec, mbPerSec)
		} else if totalOps > 0 {
			data["OPERATIONS SUMMARY"] = fmt.Sprintf("%s operations (last minute)", humanize.Comma(int64(totalOps)))
		}
	}

	// IO SUMMARY - From last minute IO stats
	minute := &node.disk.IOStatsMinute
	if minute.N > 0 {
		timeframeSeconds := 60.0

		// Calculate total IOs and bytes
		totalIOs := minute.ReadIOs + minute.WriteIOs + minute.DiscardIOs + minute.FlushIOs
		totalSectors := minute.ReadSectors + minute.WriteSectors + minute.DiscardSectors
		totalBytes := totalSectors * 512 // 512 bytes per sector

		if totalIOs > 0 {
			iosPerSec := float64(totalIOs) / timeframeSeconds
			mbPerSec := float64(totalBytes) / timeframeSeconds / (1024 * 1024)

			data["IO SUMMARY"] = fmt.Sprintf("%.1f IO/sec, %.2f MB/s (%d drives)", iosPerSec, mbPerSec, minute.N)
		}
	}

	// Additional Features
	var features []string
	if node.disk.Cache != nil {
		features = append(features, "cache stats")
	}
	if node.disk.HealingInfo != nil {
		features = append(features, "healing info")
	}
	if node.disk.IOStats != nil {
		features = append(features, "legacy IO stats")
	}
	if len(features) > 0 {
		data["Available Features"] = strings.Join(features, ", ")
	}

	return data
}

func (node *DiskMetricsNavigator) GetMetricType() MetricType {
	return MetricsDisk
}

func (node *DiskMetricsNavigator) GetMetricFlags() MetricFlags {
	return 0
}

func (node *DiskMetricsNavigator) GetParent() MetricNode {
	return node.parent
}

func (node *DiskMetricsNavigator) GetPath() string {
	return node.path
}

func (node *DiskMetricsNavigator) RequiredMetricTypes() MetricType {
	return MetricsDisk
}

func (node *DiskMetricsNavigator) ShouldPauseRefresh() bool {
	return false
}

func (node *DiskMetricsNavigator) GetChild(name string) (MetricNode, error) {
	switch name {
	case "ops_last_minute":
		return NewDiskLastMinuteNode(node.disk.LastMinute, node, fmt.Sprintf("%s/ops_last_minute", node.path)), nil
	case "ops_last_day":
		return NewDiskLastDayNode(node.disk.LastDaySegmented, node, fmt.Sprintf("%s/ops_last_day", node.path)), nil
	case "ops_lifetime":
		return NewDiskLifetimeOpsNode(node.disk.LifetimeOps, node, fmt.Sprintf("%s/ops_lifetime", node.path)), nil
	case "io_last_minute":
		return NewDiskIOMinuteStatsNode(node.disk, node, fmt.Sprintf("%s/io_last_minute", node.path)), nil
	case "io_last_day":
		return NewDiskIODailyStatsNode(node.disk, node, fmt.Sprintf("%s/io_last_day", node.path)), nil
	case "space":
		return NewDiskSpaceNode(&node.disk.Space, node, fmt.Sprintf("%s/space", node.path)), nil
	case "healing":
		return NewDiskHealingNode(node.disk.HealingInfo, node, fmt.Sprintf("%s/healing", node.path)), nil
	case "cache":
		return NewDiskCacheNode(node.disk.Cache, node, fmt.Sprintf("%s/cache", node.path)), nil
	default:
		return nil, fmt.Errorf("child not found: %s", name)
	}
}

// DiskSpaceNode handles navigation for disk space information
type DiskSpaceNode struct {
	space  *DriveSpaceInfo
	parent MetricNode
	path   string
}

func NewDiskSpaceNode(space *DriveSpaceInfo, parent MetricNode, path string) *DiskSpaceNode {
	return &DiskSpaceNode{space: space, parent: parent, path: path}
}

func (node *DiskSpaceNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "free", Description: "Free space statistics"},
		{Name: "used", Description: "Used space statistics"},
		{Name: "inodes", Description: "Inode usage statistics"},
	}
}

func (node *DiskSpaceNode) GetLeafData() map[string]string {
	if node.space == nil {
		return map[string]string{"error": "space info not available"}
	}

	return map[string]string{
		"n":                 strconv.Itoa(node.space.N),
		"free_total":        strconv.FormatUint(node.space.Free.Total, 10),
		"free_min":          strconv.FormatUint(node.space.Free.Min, 10),
		"free_max":          strconv.FormatUint(node.space.Free.Max, 10),
		"used_total":        strconv.FormatUint(node.space.Used.Total, 10),
		"used_min":          strconv.FormatUint(node.space.Used.Min, 10),
		"used_max":          strconv.FormatUint(node.space.Used.Max, 10),
		"used_inodes_total": strconv.FormatUint(node.space.UsedInodes.Total, 10),
		"used_inodes_min":   strconv.FormatUint(node.space.UsedInodes.Min, 10),
		"used_inodes_max":   strconv.FormatUint(node.space.UsedInodes.Max, 10),
		"free_inodes_total": strconv.FormatUint(node.space.FreeInodes.Total, 10),
		"free_inodes_min":   strconv.FormatUint(node.space.FreeInodes.Min, 10),
		"free_inodes_max":   strconv.FormatUint(node.space.FreeInodes.Max, 10),
	}
}

func (node *DiskSpaceNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskSpaceNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *DiskSpaceNode) GetParent() MetricNode           { return node.parent }
func (node *DiskSpaceNode) GetPath() string                 { return node.path }
func (node *DiskSpaceNode) RequiredMetricTypes() MetricType { return MetricsDisk }

func (node *DiskSpaceNode) ShouldPauseRefresh() bool {
	return false
}
func (node *DiskSpaceNode) GetChild(name string) (MetricNode, error) {
	// Could implement sub-navigation for free/used/inodes
	return nil, fmt.Errorf("disk space component navigation not yet implemented for: %s", name)
}

// DiskLifetimeOpsNode handles navigation for lifetime disk operations
type DiskLifetimeOpsNode struct {
	ops    map[string]DiskAction
	parent MetricNode
	path   string
}

func (node *DiskLifetimeOpsNode) ShouldPauseRefresh() bool {
	return false
}

func NewDiskLifetimeOpsNode(ops map[string]DiskAction, parent MetricNode, path string) *DiskLifetimeOpsNode {
	return &DiskLifetimeOpsNode{ops: ops, parent: parent, path: path}
}

func (node *DiskLifetimeOpsNode) GetChildren() []MetricChild {
	// Return no children - display operations as leaf data instead
	return []MetricChild{}
}

func (node *DiskLifetimeOpsNode) GetLeafData() map[string]string {
	if node.ops == nil {
		return map[string]string{"Operation Types": "0"}
	}

	data := map[string]string{}

	// Calculate totals across all operations first
	var totalCount, totalBytes uint64
	var totalTime float64

	for _, action := range node.ops {
		totalCount += action.Count
		totalBytes += action.Bytes
		totalTime += action.AccTime
	}

	// Add overall summary
	if totalCount > 0 && totalTime > 0 {
		avgTime := totalTime / float64(totalCount) * 1000 // Convert to milliseconds
		rps := float64(totalCount) / totalTime
		if totalBytes > 0 {
			avgSize := float64(totalBytes) / float64(totalCount)
			data["TOTAL"] = fmt.Sprintf("avg time: %.2fms, avg size: %s, rps: %.2f, n: %s",
				avgTime, humanize.Bytes(uint64(avgSize)), rps, humanize.Comma(int64(totalCount)))
		} else {
			data["TOTAL"] = fmt.Sprintf("avg time: %.2fms, rps: %.2f, n: %s",
				avgTime, rps, humanize.Comma(int64(totalCount)))
		}
	}

	// Format each operation with statistics
	var operations []string
	for opType := range node.ops {
		operations = append(operations, opType)
	}
	sort.Strings(operations)

	for _, opType := range operations {
		action := node.ops[opType]
		if action.Count > 0 && action.AccTime > 0 {
			avgTime := action.AccTime / float64(action.Count) * 1000 // Convert to milliseconds
			rps := float64(action.Count) / action.AccTime

			if action.Bytes > 0 {
				avgSize := float64(action.Bytes) / float64(action.Count)
				minTime := action.MinTime * 1000 // Convert to milliseconds
				maxTime := action.MaxTime * 1000 // Convert to milliseconds
				data[opType] = fmt.Sprintf("avg time: %.2fms, avg size: %s, rps: %.2f, min: %.1fms max: %.1fs, n: %s",
					avgTime, humanize.Bytes(uint64(avgSize)), rps, minTime, maxTime, humanize.Comma(int64(action.Count)))
			} else {
				minTime := action.MinTime * 1000 // Convert to milliseconds
				maxTime := action.MaxTime * 1000 // Convert to milliseconds
				data[opType] = fmt.Sprintf("avg time: %.2fms, rps: %.2f, min: %.1fms max: %.1fs, n: %s",
					avgTime, rps, minTime, maxTime, humanize.Comma(int64(action.Count)))
			}
		} else {
			data[opType] = fmt.Sprintf("Count: %s", humanize.Comma(int64(action.Count)))
		}
	}

	return data
}

func (node *DiskLifetimeOpsNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskLifetimeOpsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *DiskLifetimeOpsNode) GetParent() MetricNode           { return node.parent }
func (node *DiskLifetimeOpsNode) GetPath() string                 { return node.path }
func (node *DiskLifetimeOpsNode) RequiredMetricTypes() MetricType { return MetricsDisk }
func (node *DiskLifetimeOpsNode) GetChild(name string) (MetricNode, error) {
	if action, exists := node.ops[name]; exists {
		return NewDiskActionNode(name, &action, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("disk operation not found: %s", name)
}

// DiskLastMinuteNode handles navigation for last minute disk operations
type DiskLastMinuteNode struct {
	ops    map[string]DiskAction
	parent MetricNode
	path   string
}

func NewDiskLastMinuteNode(ops map[string]DiskAction, parent MetricNode, path string) *DiskLastMinuteNode {
	return &DiskLastMinuteNode{ops: ops, parent: parent, path: path}
}

func (node *DiskLastMinuteNode) GetChildren() []MetricChild {
	// Return no children - display operations as leaf data instead
	return []MetricChild{}
}

func (node *DiskLastMinuteNode) GetLeafData() map[string]string {
	if node.ops == nil {
		return map[string]string{"Operation Types": "0"}
	}

	data := map[string]string{}

	// Calculate totals across all operations first
	var totalCount, totalBytes uint64
	var totalTime float64

	for _, action := range node.ops {
		totalCount += action.Count
		totalBytes += action.Bytes
		totalTime += action.AccTime
	}

	// Add overall summary
	if totalCount > 0 && totalTime > 0 {
		avgTime := totalTime / float64(totalCount) * 1000 // Convert to milliseconds
		rps := float64(totalCount) / totalTime
		if totalBytes > 0 {
			avgSize := float64(totalBytes) / float64(totalCount)
			data["TOTAL"] = fmt.Sprintf("avg time: %.2fms, avg size: %s, rps: %.2f, n: %s",
				avgTime, humanize.Bytes(uint64(avgSize)), rps, humanize.Comma(int64(totalCount)))
		} else {
			data["TOTAL"] = fmt.Sprintf("avg time: %.2fms, rps: %.2f, n: %s",
				avgTime, rps, humanize.Comma(int64(totalCount)))
		}
	}

	// Format each operation with statistics
	var operations []string
	for opType := range node.ops {
		operations = append(operations, opType)
	}
	sort.Strings(operations)

	for _, opType := range operations {
		action := node.ops[opType]
		if action.Count > 0 && action.AccTime > 0 {
			avgTime := action.AccTime / float64(action.Count) * 1000 // Convert to milliseconds
			rps := float64(action.Count) / action.AccTime

			if action.Bytes > 0 {
				avgSize := float64(action.Bytes) / float64(action.Count)
				minTime := action.MinTime * 1000 // Convert to milliseconds
				maxTime := action.MaxTime * 1000 // Convert to milliseconds
				data[opType] = fmt.Sprintf("avg time: %.2fms, avg size: %s, rps: %.2f, min: %.1fms max: %.1fs, n: %s",
					avgTime, humanize.Bytes(uint64(avgSize)), rps, minTime, maxTime, humanize.Comma(int64(action.Count)))
			} else {
				minTime := action.MinTime * 1000 // Convert to milliseconds
				maxTime := action.MaxTime * 1000 // Convert to milliseconds
				data[opType] = fmt.Sprintf("avg time: %.2fms, rps: %.2f, min: %.1fms max: %.1fs, n: %s",
					avgTime, rps, minTime, maxTime, humanize.Comma(int64(action.Count)))
			}
		} else {
			data[opType] = fmt.Sprintf("Count: %s", humanize.Comma(int64(action.Count)))
		}
	}

	return data
}

func (node *DiskLastMinuteNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskLastMinuteNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *DiskLastMinuteNode) GetParent() MetricNode           { return node.parent }
func (node *DiskLastMinuteNode) GetPath() string                 { return node.path }
func (node *DiskLastMinuteNode) RequiredMetricTypes() MetricType { return MetricsDisk }

func (node *DiskLastMinuteNode) ShouldPauseRefresh() bool {
	return false
}
func (node *DiskLastMinuteNode) GetChild(name string) (MetricNode, error) {
	if action, exists := node.ops[name]; exists {
		return NewDiskActionNode(name, &action, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("disk operation not found: %s", name)
}

// DiskLastDayNode handles navigation for segmented last day operations
type DiskLastDayNode struct {
	segmented map[string]SegmentedDiskActions
	parent    MetricNode
	path      string
}

func NewDiskLastDayNode(segmented map[string]SegmentedDiskActions, parent MetricNode, path string) *DiskLastDayNode {
	return &DiskLastDayNode{segmented: segmented, parent: parent, path: path}
}

func (node *DiskLastDayNode) GetChildren() []MetricChild {
	// Return no children - display day totals as leaf data instead
	return []MetricChild{}
}

func (node *DiskLastDayNode) GetLeafData() map[string]string {
	if node.segmented == nil {
		return map[string]string{"Operation Types": "0"}
	}

	data := map[string]string{}

	// Calculate day totals across all operations and segments
	var totalCount, totalBytes uint64
	var totalTime float64

	for _, segmented := range node.segmented {
		for _, segment := range segmented.Segments {
			totalCount += segment.Count
			totalBytes += segment.Bytes
			totalTime += segment.AccTime
		}
	}

	// Add overall day summary
	if totalCount > 0 && totalTime > 0 {
		avgTime := totalTime / float64(totalCount) * 1000 // Convert to milliseconds
		rps := float64(totalCount) / totalTime
		if totalBytes > 0 {
			avgSize := float64(totalBytes) / float64(totalCount)
			data["DAY TOTAL"] = fmt.Sprintf("avg time: %.2fms, avg size: %s, rps: %.2f, n: %s",
				avgTime, humanize.Bytes(uint64(avgSize)), rps, humanize.Comma(int64(totalCount)))
		} else {
			data["DAY TOTAL"] = fmt.Sprintf("avg time: %.2fms, rps: %.2f, n: %s",
				avgTime, rps, humanize.Comma(int64(totalCount)))
		}
	}

	// Format each operation type with day totals
	var operations []string
	for opType := range node.segmented {
		operations = append(operations, opType)
	}
	sort.Strings(operations)

	for _, opType := range operations {
		segmented := node.segmented[opType]
		total := segmented.Total()

		if total.Count > 0 && total.AccTime > 0 {
			avgTime := total.AccTime / float64(total.Count) * 1000 // Convert to milliseconds
			rps := float64(total.Count) / total.AccTime
			minTime := total.MinTime * 1000 // Convert to milliseconds
			maxTime := total.MaxTime * 1000 // Convert to milliseconds

			if total.Bytes > 0 {
				avgSize := float64(total.Bytes) / float64(total.Count)
				data[opType] = fmt.Sprintf("avg time: %.2fms, avg size: %s, rps: %.2f, min: %.1fms max: %.1fs, n: %s",
					avgTime, humanize.Bytes(uint64(avgSize)), rps, minTime, maxTime, humanize.Comma(int64(total.Count)))
			} else {
				data[opType] = fmt.Sprintf("avg time: %.2fms, rps: %.2f, min: %.1fms max: %.1fs, n: %s",
					avgTime, rps, minTime, maxTime, humanize.Comma(int64(total.Count)))
			}
		} else {
			data[opType] = fmt.Sprintf("n: %s", humanize.Comma(int64(total.Count)))
		}
	}

	return data
}

func (node *DiskLastDayNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskLastDayNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *DiskLastDayNode) GetParent() MetricNode           { return node.parent }
func (node *DiskLastDayNode) GetPath() string                 { return node.path }
func (node *DiskLastDayNode) RequiredMetricTypes() MetricType { return MetricsDisk }

func (node *DiskLastDayNode) ShouldPauseRefresh() bool {
	return true
}
func (node *DiskLastDayNode) GetChild(name string) (MetricNode, error) {
	// Could implement navigation into individual segmented data
	return nil, fmt.Errorf("disk segmented navigation not yet implemented for: %s", name)
}

// DiskIOStatsNode handles navigation for disk IO statistics
type DiskIOStatsNode struct {
	disk   *DiskMetric
	parent MetricNode
	path   string
}

func NewDiskIOStatsNode(disk *DiskMetric, parent MetricNode, path string) *DiskIOStatsNode {
	return &DiskIOStatsNode{disk: disk, parent: parent, path: path}
}

func (node *DiskIOStatsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "minute", Description: "Last minute IO statistics (real-time)"},
		{Name: "daily", Description: "Daily aggregated IO statistics"},
	}
}

func (node *DiskIOStatsNode) GetLeafData() map[string]string {
	if node.disk == nil {
		return map[string]string{"Error": "disk metrics not available"}
	}

	data := map[string]string{}

	// IO Statistics Overview
	data["IO STATISTICS"] = "Navigate to specific timeframes for detailed IO analysis"

	// Quick overview of minute stats
	minute := &node.disk.IOStatsMinute
	if minute.N > 0 {
		totalIOs := minute.ReadIOs + minute.WriteIOs + minute.DiscardIOs + minute.FlushIOs
		data["Last Minute Overview"] = fmt.Sprintf("%s total IOs from %d drives",
			humanize.Comma(int64(totalIOs)), minute.N)

		if minute.TotalTicks > 0 {
			utilPercent := float64(minute.TotalTicks) / (60.0 * 1000.0) * 100.0
			data["Device Utilization"] = fmt.Sprintf("%.1f%% average utilization",
				utilPercent/float64(minute.N))
		}
	} else {
		data["Last Minute Status"] = "No recent IO activity data available"
	}

	// Navigation guidance
	data["Available Timeframes"] = "minute (real-time), daily (historical)"
	data["Recommendation"] = "Navigate to 'minute' for real-time IO performance monitoring"

	return data
}

func (node *DiskIOStatsNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskIOStatsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *DiskIOStatsNode) GetParent() MetricNode           { return node.parent }
func (node *DiskIOStatsNode) GetPath() string                 { return node.path }
func (node *DiskIOStatsNode) RequiredMetricTypes() MetricType { return MetricsDisk }

func (node *DiskIOStatsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *DiskIOStatsNode) GetChild(name string) (MetricNode, error) {
	switch name {
	case "minute":
		return NewDiskIOMinuteStatsNode(node.disk, node, fmt.Sprintf("%s/minute", node.path)), nil
	case "daily":
		return NewDiskIODailyStatsNode(node.disk, node, fmt.Sprintf("%s/daily", node.path)), nil
	default:
		return nil, fmt.Errorf("disk io component not found: %s", name)
	}
}

// DiskIOMinuteStatsNode handles last minute IO statistics
type DiskIOMinuteStatsNode struct {
	disk   *DiskMetric
	parent MetricNode
	path   string
}

func NewDiskIOMinuteStatsNode(disk *DiskMetric, parent MetricNode, path string) *DiskIOMinuteStatsNode {
	return &DiskIOMinuteStatsNode{disk: disk, parent: parent, path: path}
}

func (node *DiskIOMinuteStatsNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *DiskIOMinuteStatsNode) GetLeafData() map[string]string {
	if node.disk == nil {
		return map[string]string{"Error": "disk metrics not available"}
	}

	data := map[string]string{}

	// Process minute IO stats (60 second timeframe)
	minute := &node.disk.IOStatsMinute
	if minute.N > 0 {
		timeframeSeconds := 60.0 // Last minute stats
		data["LAST MINUTE IO STATS"] = fmt.Sprintf("Aggregated from %d drives over %.0f seconds",
			minute.N, timeframeSeconds)

		// Format IO operation totals and rates
		formatIOStats(data, "Read", minute.ReadIOs, minute.ReadMerges, minute.ReadSectors,
			minute.ReadTicks, minute.N, timeframeSeconds)
		formatIOStats(data, "Write", minute.WriteIOs, minute.WriteMerges, minute.WriteSectors,
			minute.WriteTicks, minute.N, timeframeSeconds)
		formatIOStats(data, "Discard", minute.DiscardIOs, minute.DiscardMerges, minute.DiscardSectors,
			minute.DiscardTicks, minute.N, timeframeSeconds)
		formatIOStats(data, "Flush", minute.FlushIOs, 0, 0,
			minute.FlushTicks, minute.N, timeframeSeconds)

		// Add device utilization metrics
		if minute.TotalTicks > 0 {
			utilPercent := float64(minute.TotalTicks) / (timeframeSeconds * 1000.0) * 100.0
			avgUtilPerDrive := utilPercent / float64(minute.N)
			data["Device Utilization"] = fmt.Sprintf("%.1f%% total, %.1f%% avg per drive",
				utilPercent, avgUtilPerDrive)
		}

		if minute.ReqTicks > 0 {
			avgQueueTime := float64(minute.ReqTicks) / float64(minute.ReadIOs+minute.WriteIOs+minute.DiscardIOs+minute.FlushIOs)
			data["Avg Queue Time"] = fmt.Sprintf("%.2fms per request", avgQueueTime)
		}

		if minute.CurrentIOs > 0 {
			data["Current IOs"] = fmt.Sprintf("%s in flight, %.1f avg per drive",
				humanize.Comma(int64(minute.CurrentIOs)), float64(minute.CurrentIOs)/float64(minute.N))
		}
	}

	if len(data) == 0 {
		return map[string]string{"Status": "No last minute IO statistics available"}
	}

	return data
}

func (node *DiskIOMinuteStatsNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskIOMinuteStatsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *DiskIOMinuteStatsNode) GetParent() MetricNode           { return node.parent }
func (node *DiskIOMinuteStatsNode) GetPath() string                 { return node.path }
func (node *DiskIOMinuteStatsNode) RequiredMetricTypes() MetricType { return MetricsDisk }
func (node *DiskIOMinuteStatsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *DiskIOMinuteStatsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("minute IO stats is a leaf node")
}

// DiskIODailyStatsNode handles daily IO statistics
type DiskIODailyStatsNode struct {
	disk   *DiskMetric
	parent MetricNode
	path   string
}

func NewDiskIODailyStatsNode(disk *DiskMetric, parent MetricNode, path string) *DiskIODailyStatsNode {
	return &DiskIODailyStatsNode{disk: disk, parent: parent, path: path}
}

func (node *DiskIODailyStatsNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *DiskIODailyStatsNode) GetLeafData() map[string]string {
	if node.disk == nil {
		return map[string]string{"Error": "disk metrics not available"}
	}

	data := map[string]string{}

	// Process daily segmented IO stats
	dailyStats := &node.disk.IOStatsDay
	if len(dailyStats.Segments) > 0 {
		// Aggregate all daily segments into totals
		var totalReadIOs, totalWriteIOs, totalDiscardIOs, totalFlushIOs uint64
		var totalReadSectors, totalWriteSectors, totalDiscardSectors uint64
		var totalReadTicks, totalWriteTicks, totalDiscardTicks, totalFlushTicks uint64
		var totalReadMerges, totalWriteMerges, totalDiscardMerges uint64
		var totalTicks, totalReqTicks uint64
		var segmentCount int
		var numDrives int

		for _, seg := range dailyStats.Segments {
			totalReadIOs += seg.ReadIOs
			totalWriteIOs += seg.WriteIOs
			totalDiscardIOs += seg.DiscardIOs
			totalFlushIOs += seg.FlushIOs
			totalReadSectors += seg.ReadSectors
			totalWriteSectors += seg.WriteSectors
			totalDiscardSectors += seg.DiscardSectors
			totalReadTicks += seg.ReadTicks
			totalWriteTicks += seg.WriteTicks
			totalDiscardTicks += seg.DiscardTicks
			totalFlushTicks += seg.FlushTicks
			totalReadMerges += seg.ReadMerges
			totalWriteMerges += seg.WriteMerges
			totalDiscardMerges += seg.DiscardMerges
			totalTicks += seg.TotalTicks
			totalReqTicks += seg.ReqTicks
			if seg.N > numDrives {
				numDrives = seg.N
			}
			segmentCount++
		}

		timeframeSeconds := float64(segmentCount * 3600) // Each segment represents 1 hour, convert to seconds

		data["DAILY IO STATS"] = fmt.Sprintf("Aggregated from %d drives over %d hour segments",
			numDrives, segmentCount)

		// Format IO operation totals and rates (similar to minute stats)
		formatIOStats(data, "Read", totalReadIOs, totalReadMerges, totalReadSectors,
			totalReadTicks, numDrives, timeframeSeconds)
		formatIOStats(data, "Write", totalWriteIOs, totalWriteMerges, totalWriteSectors,
			totalWriteTicks, numDrives, timeframeSeconds)
		formatIOStats(data, "Discard", totalDiscardIOs, totalDiscardMerges, totalDiscardSectors,
			totalDiscardTicks, numDrives, timeframeSeconds)
		formatIOStats(data, "Flush", totalFlushIOs, 0, 0,
			totalFlushTicks, numDrives, timeframeSeconds)

		// Add device utilization metrics
		if totalTicks > 0 && numDrives > 0 {
			utilPercent := float64(totalTicks) / (timeframeSeconds * 1000.0) * 100.0
			avgUtilPerDrive := utilPercent / float64(numDrives)
			data["Device Utilization"] = fmt.Sprintf("%.1f%% total, %.1f%% avg per drive",
				utilPercent, avgUtilPerDrive)
		}

		if totalReqTicks > 0 {
			totalIOs := totalReadIOs + totalWriteIOs + totalDiscardIOs + totalFlushIOs
			if totalIOs > 0 {
				avgQueueTime := float64(totalReqTicks) / float64(totalIOs)
				data["Avg Queue Time"] = fmt.Sprintf("%.2fms per request", avgQueueTime)
			}
		}
	} else {
		data["Status"] = "No daily IO statistics available"
		data["Note"] = "Daily IO segments may not be collected yet or MetricsDayStats flag not enabled"
	}

	return data
}

func (node *DiskIODailyStatsNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskIODailyStatsNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *DiskIODailyStatsNode) GetParent() MetricNode           { return node.parent }
func (node *DiskIODailyStatsNode) GetPath() string                 { return node.path }
func (node *DiskIODailyStatsNode) RequiredMetricTypes() MetricType { return MetricsDisk }
func (node *DiskIODailyStatsNode) ShouldPauseRefresh() bool {
	return true
}
func (node *DiskIODailyStatsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("daily IO stats is a leaf node")
}

// DiskHealingNode handles navigation for disk healing information
type DiskHealingNode struct {
	healing *DriveHealInfo
	parent  MetricNode
	path    string
}

func NewDiskHealingNode(healing *DriveHealInfo, parent MetricNode, path string) *DiskHealingNode {
	return &DiskHealingNode{healing: healing, parent: parent, path: path}
}

func (node *DiskHealingNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *DiskHealingNode) GetLeafData() map[string]string {
	if node.healing == nil {
		return map[string]string{"healing_active": "false"}
	}

	return map[string]string{
		"healing_active": "true",
		"items_healed":   strconv.FormatUint(node.healing.ItemsHealed, 10),
		"items_failed":   strconv.FormatUint(node.healing.ItemsFailed, 10),
		"heal_id":        node.healing.HealID,
		"finished":       strconv.FormatBool(node.healing.Finished),
		"started":        node.healing.Started.Format(time.RFC3339),
		"updated":        node.healing.Updated.Format(time.RFC3339),
	}
}

func (node *DiskHealingNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskHealingNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *DiskHealingNode) GetParent() MetricNode           { return node.parent }
func (node *DiskHealingNode) GetPath() string                 { return node.path }
func (node *DiskHealingNode) RequiredMetricTypes() MetricType { return MetricsDisk }

func (node *DiskHealingNode) ShouldPauseRefresh() bool {
	return false
}
func (node *DiskHealingNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("disk healing is a leaf node")
}

// DiskCacheNode handles navigation for cache statistics
type DiskCacheNode struct {
	cache  interface{} // CacheStats type
	parent MetricNode
	path   string
}

func NewDiskCacheNode(cache interface{}, parent MetricNode, path string) *DiskCacheNode {
	return &DiskCacheNode{cache: cache, parent: parent, path: path}
}

func (node *DiskCacheNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *DiskCacheNode) GetLeafData() map[string]string {
	return map[string]string{
		"cache_available": strconv.FormatBool(node.cache != nil),
	}
}

func (node *DiskCacheNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskCacheNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *DiskCacheNode) GetParent() MetricNode           { return node.parent }
func (node *DiskCacheNode) GetPath() string                 { return node.path }
func (node *DiskCacheNode) RequiredMetricTypes() MetricType { return MetricsDisk }

func (node *DiskCacheNode) ShouldPauseRefresh() bool {
	return false
}
func (node *DiskCacheNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("disk cache is a leaf node")
}

// DiskSummaryNode provides aggregated disk statistics
type DiskSummaryNode struct {
	disk   *DiskMetric
	parent MetricNode
	path   string
}

func NewDiskSummaryNode(disk *DiskMetric, parent MetricNode, path string) *DiskSummaryNode {
	return &DiskSummaryNode{disk: disk, parent: parent, path: path}
}

func (node *DiskSummaryNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *DiskSummaryNode) GetLeafData() map[string]string {
	if node.disk == nil {
		return map[string]string{"Error": "disk metrics not available"}
	}

	data := map[string]string{}

	// Executive Summary Header
	data["DISK SUMMARY"] = fmt.Sprintf("Collected at %s",
		node.disk.CollectedAt.Format("2006-01-02 15:04:05"))

	// Cluster Health Overview
	totalDisks := node.disk.NDisks
	healthyDisks := totalDisks - node.disk.Offline - node.disk.Hanging - node.disk.Healing

	if totalDisks > 0 {
		healthPercent := float64(healthyDisks) / float64(totalDisks) * 100.0
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

		data["Cluster Health"] = fmt.Sprintf("%s - %d of %d drives healthy (%.1f%%)",
			healthStatus, healthyDisks, totalDisks, healthPercent)

		// Problem breakdown
		var issues []string
		if node.disk.Offline > 0 {
			issues = append(issues, fmt.Sprintf("%d offline", node.disk.Offline))
		}
		if node.disk.Hanging > 0 {
			issues = append(issues, fmt.Sprintf("%d hanging", node.disk.Hanging))
		}
		if node.disk.Healing > 0 {
			issues = append(issues, fmt.Sprintf("%d healing", node.disk.Healing))
		}
		if len(issues) > 0 {
			data["Issues"] = strings.Join(issues, ", ")
		}
	}

	// Storage Capacity Analysis
	if node.disk.Space.N > 0 {
		totalCapacity := node.disk.Space.Free.Total + node.disk.Space.Used.Total
		if totalCapacity > 0 {
			usagePercent := float64(node.disk.Space.Used.Total) / float64(totalCapacity) * 100.0

			var capacityStatus string
			switch {
			case usagePercent < 70:
				capacityStatus = "Healthy"
			case usagePercent < 85:
				capacityStatus = "Monitor"
			case usagePercent < 95:
				capacityStatus = "Warning"
			default:
				capacityStatus = "Critical"
			}

			data["Storage Capacity"] = fmt.Sprintf("%s - %s used of %s total (%.1f%%)",
				capacityStatus,
				humanize.Bytes(node.disk.Space.Used.Total),
				humanize.Bytes(totalCapacity),
				usagePercent)

			// Capacity distribution
			if node.disk.Space.N > 1 {
				data["Capacity Distribution"] = fmt.Sprintf("Range: %s to %s free across %d drives",
					humanize.Bytes(node.disk.Space.Free.Min),
					humanize.Bytes(node.disk.Space.Free.Max),
					node.disk.Space.N)
			}

			// Inode status
			totalInodes := node.disk.Space.UsedInodes.Total + node.disk.Space.FreeInodes.Total
			if totalInodes > 0 {
				inodePercent := float64(node.disk.Space.UsedInodes.Total) / float64(totalInodes) * 100.0
				data["File System"] = fmt.Sprintf("%s inodes used of %s total (%.1f%%)",
					humanize.Comma(int64(node.disk.Space.UsedInodes.Total)),
					humanize.Comma(int64(totalInodes)),
					inodePercent)
			}
		}
	}

	// Operational Activity Summary
	totalOpTypes := len(node.disk.LifetimeOps) + len(node.disk.LastMinute)
	if totalOpTypes > 0 {
		data["Operations Tracked"] = fmt.Sprintf("%d operation types tracked", totalOpTypes)
	}

	if len(node.disk.LifetimeOps) > 0 {
		data["Lifetime Operations"] = fmt.Sprintf("%d operation types with historical data",
			len(node.disk.LifetimeOps))
	}

	if len(node.disk.LastMinute) > 0 {
		data["Recent Activity"] = fmt.Sprintf("%d operation types active in last minute",
			len(node.disk.LastMinute))
	}

	if len(node.disk.LastDaySegmented) > 0 {
		data["Daily Trends"] = fmt.Sprintf("%d operation types with daily segmented data",
			len(node.disk.LastDaySegmented))
	}

	// State Issues Detail
	if len(node.disk.State) > 0 {
		var totalStateIssues int
		for _, count := range node.disk.State {
			totalStateIssues += count
		}
		if totalStateIssues > 0 {
			data["State Issues"] = fmt.Sprintf("%d drive state issues across %d categories",
				totalStateIssues, len(node.disk.State))
		}
	}

	// Available Features
	var availableFeatures []string
	if node.disk.HealingInfo != nil {
		availableFeatures = append(availableFeatures, "active healing")
	}
	if node.disk.Cache != nil {
		availableFeatures = append(availableFeatures, "cache statistics")
	}
	if node.disk.IOStats != nil {
		availableFeatures = append(availableFeatures, "legacy IO metrics")
	}
	if len(availableFeatures) > 0 {
		data["Additional Features"] = strings.Join(availableFeatures, ", ")
	}

	// Performance Indicators
	if len(node.disk.LastMinute) > 0 {
		data["Performance"] = "Real-time metrics available - navigate to IO stats for details"
	}

	return data
}

func (node *DiskSummaryNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskSummaryNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *DiskSummaryNode) GetParent() MetricNode           { return node.parent }
func (node *DiskSummaryNode) GetPath() string                 { return node.path }
func (node *DiskSummaryNode) RequiredMetricTypes() MetricType { return MetricsDisk }

func (node *DiskSummaryNode) ShouldPauseRefresh() bool {
	return false
}
func (node *DiskSummaryNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("disk summary is a leaf node")
}

// DiskActionNode represents a leaf node with disk action details
type DiskActionNode struct {
	actionType string
	action     *DiskAction
	parent     MetricNode
	path       string
}

func NewDiskActionNode(actionType string, action *DiskAction, parent MetricNode, path string) *DiskActionNode {
	return &DiskActionNode{actionType: actionType, action: action, parent: parent, path: path}
}

func (node *DiskActionNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *DiskActionNode) GetLeafData() map[string]string {
	if node.action == nil {
		return map[string]string{
			"action_type": node.actionType,
			"count":       "0",
			"bytes":       "0",
			"acc_time":    "0",
			"min_time":    "0",
			"max_time":    "0",
		}
	}

	return map[string]string{
		"action_type": node.actionType,
		"count":       strconv.FormatUint(node.action.Count, 10),
		"bytes":       strconv.FormatUint(node.action.Bytes, 10),
		"acc_time":    fmt.Sprintf("%.2f", node.action.AccTime),
		"min_time":    fmt.Sprintf("%.2f", node.action.MinTime),
		"max_time":    fmt.Sprintf("%.2f", node.action.MaxTime),
	}
}

func (node *DiskActionNode) GetMetricType() MetricType       { return MetricsDisk }
func (node *DiskActionNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *DiskActionNode) GetParent() MetricNode           { return node.parent }
func (node *DiskActionNode) GetPath() string                 { return node.path }
func (node *DiskActionNode) RequiredMetricTypes() MetricType { return MetricsDisk }

func (node *DiskActionNode) ShouldPauseRefresh() bool {
	return false
}
func (node *DiskActionNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("disk action is a leaf node")
}

// formatIOStats formats IO statistics for a specific operation type (Read/Write/Discard/Flush)
func formatIOStats(data map[string]string, opType string, ios, merges, sectors, ticks uint64, numDrives int, timeframeSeconds float64) {
	if ios == 0 {
		return // Skip operations with no activity
	}

	// Calculate rates per second
	iosPerSec := float64(ios) / timeframeSeconds
	avgIOsPerDrivePerSec := iosPerSec / float64(numDrives)

	// Calculate sector sizes (512 bytes per sector)
	totalBytes := sectors * 512
	bytesPerSec := float64(totalBytes) / timeframeSeconds
	avgBytesPerDrivePerSec := bytesPerSec / float64(numDrives)

	// Calculate timing metrics
	var avgServiceTime float64
	if ios > 0 && ticks > 0 {
		avgServiceTime = float64(ticks) / float64(ios) // Average milliseconds per IO
	}

	// Calculate merge efficiency
	mergePercent := float64(merges) / float64(ios+merges) * 100.0

	// Format the comprehensive statistics
	statsLine := fmt.Sprintf("%s/sec", humanize.Comma(int64(iosPerSec)))
	if avgIOsPerDrivePerSec > 0.01 { // Only show per-drive if meaningful
		statsLine += fmt.Sprintf(" (%.1f/drive)", avgIOsPerDrivePerSec)
	}

	if totalBytes > 0 {
		statsLine += fmt.Sprintf(", %s/sec", humanize.Bytes(uint64(bytesPerSec)))
		if avgBytesPerDrivePerSec > 0 {
			statsLine += fmt.Sprintf(" (%s/drive)", humanize.Bytes(uint64(avgBytesPerDrivePerSec)))
		}
	}

	if avgServiceTime > 0 {
		statsLine += fmt.Sprintf(", %.2fms avg service", avgServiceTime)
	}

	if merges > 0 {
		statsLine += fmt.Sprintf(", %.1f%% merged", mergePercent)
	}

	data[opType+" Operations"] = statsLine

	// Add totals for reference
	totalLine := fmt.Sprintf("%s IOs", humanize.Comma(int64(ios)))
	if totalBytes > 0 {
		totalLine += fmt.Sprintf(", %s transferred", humanize.Bytes(totalBytes))
	}
	if merges > 0 {
		totalLine += fmt.Sprintf(", %s merged", humanize.Comma(int64(merges)))
	}
	data[opType+" Totals"] = totalLine
}
