package madmin

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// ScannerMetrics contains scanner-related metrics
type ScannerMetrics struct {
	// Time these metrics were collected
	CollectedAt time.Time `json:"collected"`

	// Number of buckets currently scanning
	OngoingBuckets int `json:"ongoing_buckets"`

	// Stats per bucket, a map between bucket name and scan stats in all erasure sets
	PerBucketStats map[string][]BucketScanInfo `json:"per_bucket_stats,omitempty"`

	// Number of accumulated operations by type since server restart.
	LifeTimeOps map[string]uint64 `json:"life_time_ops,omitempty"`

	// Number of accumulated ILM operations by type since server restart.
	LifeTimeILM map[string]uint64 `json:"ilm_ops,omitempty"`

	// Last minute operation statistics.
	LastMinute struct {
		// Scanner actions.
		Actions map[string]TimedAction `json:"actions,omitempty"`
		// ILM actions.
		ILM map[string]TimedAction `json:"ilm,omitempty"`
	} `json:"last_minute"`

	// Currently active path(s) being scanned.
	ActivePaths []string `json:"active,omitempty"`

	// Excessive prefixes.
	// Paths that have been marked as having excessive number of entries within the last 24 hours.
	ExcessivePrefixes []string `json:"excessive,omitempty"`
}

// Merge other into 's'.
func (s *ScannerMetrics) Merge(other *ScannerMetrics) {
	if other == nil {
		return
	}

	if s.CollectedAt.Before(other.CollectedAt) {
		// Use latest timestamp
		s.CollectedAt = other.CollectedAt
	}

	if s.OngoingBuckets < other.OngoingBuckets {
		s.OngoingBuckets = other.OngoingBuckets
	}

	if s.PerBucketStats == nil {
		s.PerBucketStats = make(map[string][]BucketScanInfo)
	}
	for bucket, otherSt := range other.PerBucketStats {
		if len(otherSt) == 0 {
			continue
		}
		_, ok := s.PerBucketStats[bucket]
		if !ok {
			s.PerBucketStats[bucket] = otherSt
		}
	}

	// Regular ops
	if len(other.LifeTimeOps) > 0 && s.LifeTimeOps == nil {
		s.LifeTimeOps = make(map[string]uint64, len(other.LifeTimeOps))
	}
	for k, v := range other.LifeTimeOps {
		total := s.LifeTimeOps[k] + v
		s.LifeTimeOps[k] = total
	}
	if s.LastMinute.Actions == nil && len(other.LastMinute.Actions) > 0 {
		s.LastMinute.Actions = make(map[string]TimedAction, len(other.LastMinute.Actions))
	}
	for k, v := range other.LastMinute.Actions {
		total := s.LastMinute.Actions[k]
		total.Merge(v)
		s.LastMinute.Actions[k] = total
	}

	// ILM
	if len(other.LifeTimeILM) > 0 && s.LifeTimeILM == nil {
		s.LifeTimeILM = make(map[string]uint64, len(other.LifeTimeILM))
	}
	for k, v := range other.LifeTimeILM {
		total := s.LifeTimeILM[k] + v
		s.LifeTimeILM[k] = total
	}
	if s.LastMinute.ILM == nil && len(other.LastMinute.ILM) > 0 {
		s.LastMinute.ILM = make(map[string]TimedAction, len(other.LastMinute.ILM))
	}
	for k, v := range other.LastMinute.ILM {
		total := s.LastMinute.ILM[k]
		total.Merge(v)
		s.LastMinute.ILM[k] = total
	}
	s.ActivePaths = append(s.ActivePaths, other.ActivePaths...)
	sort.Strings(s.ActivePaths)

	if len(other.ExcessivePrefixes) > 0 {
		// Merge and remove duplicates
		merged := make(map[string]struct{}, len(s.ExcessivePrefixes)+len(other.ExcessivePrefixes))
		for _, prefix := range s.ExcessivePrefixes {
			merged[prefix] = struct{}{}
		}
		// Add other excessive prefixes
		for _, prefix := range other.ExcessivePrefixes {
			merged[prefix] = struct{}{}
		}
		s.ExcessivePrefixes = make([]string, 0, len(merged))
		for prefix := range merged {
			s.ExcessivePrefixes = append(s.ExcessivePrefixes, prefix)
		}
		sort.Strings(s.ExcessivePrefixes)
	}
}

// ScannerMetricsNode handles navigation for ScannerMetrics
type ScannerMetricsNode struct {
	scanner *ScannerMetrics
	parent  MetricNode
	path    string
}

// NewScannerMetricsNode creates a new ScannerMetricsNode
func NewScannerMetricsNode(scanner *ScannerMetrics, parent MetricNode, path string) *ScannerMetricsNode {
	return &ScannerMetricsNode{
		scanner: scanner,
		parent:  parent,
		path:    path,
	}
}

func (node *ScannerMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "buckets", Description: "Per-bucket scanning statistics"},
		{Name: "lifetime_ops", Description: "Accumulated operations since server start"},
		{Name: "lifetime_ilm", Description: "Accumulated ILM operations since server start"},
		{Name: "last_minute", Description: "Last minute operation statistics"},
		{Name: "active_paths", Description: "Currently active scan paths"},
		{Name: "excessive_paths", Description: "Paths marked as having excessive entries"},
	}
}

func (node *ScannerMetricsNode) GetLeafData() map[string]string {
	data := map[string]string{
		"collected_at":        node.scanner.CollectedAt.Format(time.RFC3339),
		"ongoing_buckets":     strconv.Itoa(node.scanner.OngoingBuckets),
		"bucket_count":        strconv.Itoa(len(node.scanner.PerBucketStats)),
		"lifetime_op_types":   strconv.Itoa(len(node.scanner.LifeTimeOps)),
		"lifetime_ilm_types":  strconv.Itoa(len(node.scanner.LifeTimeILM)),
		"active_paths":        strconv.Itoa(len(node.scanner.ActivePaths)),
		"excessive_paths":     strconv.Itoa(len(node.scanner.ExcessivePrefixes)),
		"last_minute_actions": strconv.Itoa(len(node.scanner.LastMinute.Actions)),
		"last_minute_ilm":     strconv.Itoa(len(node.scanner.LastMinute.ILM)),
	}

	// Add totals for lifetime ops
	var totalLifetimeOps uint64
	for _, count := range node.scanner.LifeTimeOps {
		totalLifetimeOps += count
	}
	data["total_lifetime_ops"] = strconv.FormatUint(totalLifetimeOps, 10)

	var totalLifetimeILM uint64
	for _, count := range node.scanner.LifeTimeILM {
		totalLifetimeILM += count
	}
	data["total_lifetime_ilm"] = strconv.FormatUint(totalLifetimeILM, 10)

	return data
}

func (node *ScannerMetricsNode) GetMetricType() MetricType {
	return MetricsScanner
}

func (node *ScannerMetricsNode) GetMetricFlags() MetricFlags {
	return 0
}

func (node *ScannerMetricsNode) GetParent() MetricNode {
	return node.parent
}

func (node *ScannerMetricsNode) GetPath() string {
	return node.path
}

func (node *ScannerMetricsNode) RequiredMetricTypes() MetricType {
	return MetricsScanner
}

func (node *ScannerMetricsNode) GetChild(name string) (MetricNode, error) {
	switch name {
	case "buckets":
		return NewScannerBucketsNode(node.scanner.PerBucketStats, node, fmt.Sprintf("%s/buckets", node.path)), nil
	case "lifetime_ops":
		return NewScannerLifetimeOpsNode(node.scanner.LifeTimeOps, node, fmt.Sprintf("%s/lifetime_ops", node.path)), nil
	case "lifetime_ilm":
		return NewScannerLifetimeILMNode(node.scanner.LifeTimeILM, node, fmt.Sprintf("%s/lifetime_ilm", node.path)), nil
	case "last_minute":
		return NewScannerLastMinuteNode(&node.scanner.LastMinute, node, fmt.Sprintf("%s/last_minute", node.path)), nil
	case "active_paths":
		return NewScannerPathsNode(node.scanner.ActivePaths, "active", node, fmt.Sprintf("%s/active_paths", node.path)), nil
	case "excessive_paths":
		return NewScannerPathsNode(node.scanner.ExcessivePrefixes, "excessive", node, fmt.Sprintf("%s/excessive_paths", node.path)), nil
	default:
		return nil, fmt.Errorf("child not found: %s", name)
	}
}

// Helper nodes for scanner sub-components

type ScannerBucketsNode struct {
	buckets map[string][]BucketScanInfo
	parent  MetricNode
	path    string
}

func NewScannerBucketsNode(buckets map[string][]BucketScanInfo, parent MetricNode, path string) *ScannerBucketsNode {
	return &ScannerBucketsNode{buckets: buckets, parent: parent, path: path}
}

func (node *ScannerBucketsNode) GetChildren() []MetricChild {
	var children []MetricChild
	for bucket := range node.buckets {
		children = append(children, MetricChild{
			Name:        bucket,
			Description: fmt.Sprintf("Scan info for bucket %s", bucket),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})
	return children
}

func (node *ScannerBucketsNode) GetLeafData() map[string]string {
	return map[string]string{
		"bucket_count": strconv.Itoa(len(node.buckets)),
	}
}

func (node *ScannerBucketsNode) GetMetricType() MetricType       { return MetricsScanner }
func (node *ScannerBucketsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ScannerBucketsNode) GetParent() MetricNode           { return node.parent }
func (node *ScannerBucketsNode) GetPath() string                 { return node.path }
func (node *ScannerBucketsNode) RequiredMetricTypes() MetricType { return MetricsScanner }
func (node *ScannerBucketsNode) GetChild(name string) (MetricNode, error) {
	if stats, exists := node.buckets[name]; exists {
		return NewScannerBucketStatsNode(stats, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("bucket not found: %s", name)
}

type ScannerBucketStatsNode struct {
	stats  []BucketScanInfo
	parent MetricNode
	path   string
}

func NewScannerBucketStatsNode(stats []BucketScanInfo, parent MetricNode, path string) *ScannerBucketStatsNode {
	return &ScannerBucketStatsNode{stats: stats, parent: parent, path: path}
}

func (node *ScannerBucketStatsNode) GetChildren() []MetricChild { return []MetricChild{} }
func (node *ScannerBucketStatsNode) GetLeafData() map[string]string {
	return map[string]string{
		"scan_sets": strconv.Itoa(len(node.stats)),
	}
}
func (node *ScannerBucketStatsNode) GetMetricType() MetricType       { return MetricsScanner }
func (node *ScannerBucketStatsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ScannerBucketStatsNode) GetParent() MetricNode           { return node.parent }
func (node *ScannerBucketStatsNode) GetPath() string                 { return node.path }
func (node *ScannerBucketStatsNode) RequiredMetricTypes() MetricType { return MetricsScanner }
func (node *ScannerBucketStatsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("bucket stats is a leaf node")
}

type ScannerLifetimeOpsNode struct {
	ops    map[string]uint64
	parent MetricNode
	path   string
}

func NewScannerLifetimeOpsNode(ops map[string]uint64, parent MetricNode, path string) *ScannerLifetimeOpsNode {
	return &ScannerLifetimeOpsNode{ops: ops, parent: parent, path: path}
}

func (node *ScannerLifetimeOpsNode) GetChildren() []MetricChild {
	var children []MetricChild
	for opType := range node.ops {
		children = append(children, MetricChild{
			Name:        opType,
			Description: fmt.Sprintf("Count for operation type %s", opType),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})
	return children
}

func (node *ScannerLifetimeOpsNode) GetLeafData() map[string]string {
	data := map[string]string{
		"op_types": strconv.Itoa(len(node.ops)),
	}
	var total uint64
	for opType, count := range node.ops {
		data[opType] = strconv.FormatUint(count, 10)
		total += count
	}
	data["total"] = strconv.FormatUint(total, 10)
	return data
}

func (node *ScannerLifetimeOpsNode) GetMetricType() MetricType       { return MetricsScanner }
func (node *ScannerLifetimeOpsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ScannerLifetimeOpsNode) GetParent() MetricNode           { return node.parent }
func (node *ScannerLifetimeOpsNode) GetPath() string                 { return node.path }
func (node *ScannerLifetimeOpsNode) RequiredMetricTypes() MetricType { return MetricsScanner }
func (node *ScannerLifetimeOpsNode) GetChild(name string) (MetricNode, error) {
	if count, exists := node.ops[name]; exists {
		return NewScannerOpCountNode(name, count, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("operation type not found: %s", name)
}

type ScannerLifetimeILMNode struct {
	ilm    map[string]uint64
	parent MetricNode
	path   string
}

func NewScannerLifetimeILMNode(ilm map[string]uint64, parent MetricNode, path string) *ScannerLifetimeILMNode {
	return &ScannerLifetimeILMNode{ilm: ilm, parent: parent, path: path}
}

func (node *ScannerLifetimeILMNode) GetChildren() []MetricChild {
	var children []MetricChild
	for ilmType := range node.ilm {
		children = append(children, MetricChild{
			Name:        ilmType,
			Description: fmt.Sprintf("Count for ILM operation type %s", ilmType),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})
	return children
}

func (node *ScannerLifetimeILMNode) GetLeafData() map[string]string {
	data := map[string]string{
		"ilm_types": strconv.Itoa(len(node.ilm)),
	}
	var total uint64
	for ilmType, count := range node.ilm {
		data[ilmType] = strconv.FormatUint(count, 10)
		total += count
	}
	data["total"] = strconv.FormatUint(total, 10)
	return data
}

func (node *ScannerLifetimeILMNode) GetMetricType() MetricType       { return MetricsScanner }
func (node *ScannerLifetimeILMNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ScannerLifetimeILMNode) GetParent() MetricNode           { return node.parent }
func (node *ScannerLifetimeILMNode) GetPath() string                 { return node.path }
func (node *ScannerLifetimeILMNode) RequiredMetricTypes() MetricType { return MetricsScanner }
func (node *ScannerLifetimeILMNode) GetChild(name string) (MetricNode, error) {
	if count, exists := node.ilm[name]; exists {
		return NewScannerOpCountNode(name, count, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("ILM operation type not found: %s", name)
}

type ScannerLastMinuteNode struct {
	lastMinute *struct {
		Actions map[string]TimedAction `json:"actions,omitempty"`
		ILM     map[string]TimedAction `json:"ilm,omitempty"`
	}
	parent MetricNode
	path   string
}

func NewScannerLastMinuteNode(lastMinute *struct {
	Actions map[string]TimedAction `json:"actions,omitempty"`
	ILM     map[string]TimedAction `json:"ilm,omitempty"`
}, parent MetricNode, path string) *ScannerLastMinuteNode {
	return &ScannerLastMinuteNode{lastMinute: lastMinute, parent: parent, path: path}
}

func (node *ScannerLastMinuteNode) GetChildren() []MetricChild {
	var children []MetricChild
	if len(node.lastMinute.Actions) > 0 {
		children = append(children, MetricChild{
			Name:        "actions",
			Description: "Scanner actions performed in the last minute",
		})
	}
	if len(node.lastMinute.ILM) > 0 {
		children = append(children, MetricChild{
			Name:        "ilm",
			Description: "ILM actions performed in the last minute",
		})
	}
	return children
}

func (node *ScannerLastMinuteNode) GetLeafData() map[string]string {
	return map[string]string{
		"action_types": strconv.Itoa(len(node.lastMinute.Actions)),
		"ilm_types":    strconv.Itoa(len(node.lastMinute.ILM)),
	}
}

func (node *ScannerLastMinuteNode) GetMetricType() MetricType       { return MetricsScanner }
func (node *ScannerLastMinuteNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ScannerLastMinuteNode) GetParent() MetricNode           { return node.parent }
func (node *ScannerLastMinuteNode) GetPath() string                 { return node.path }
func (node *ScannerLastMinuteNode) RequiredMetricTypes() MetricType { return MetricsScanner }
func (node *ScannerLastMinuteNode) GetChild(name string) (MetricNode, error) {
	switch name {
	case "actions":
		return NewScannerTimedActionsNode(node.lastMinute.Actions, node, fmt.Sprintf("%s/actions", node.path)), nil
	case "ilm":
		return NewScannerTimedActionsNode(node.lastMinute.ILM, node, fmt.Sprintf("%s/ilm", node.path)), nil
	default:
		return nil, fmt.Errorf("child not found: %s", name)
	}
}

type ScannerTimedActionsNode struct {
	actions map[string]TimedAction
	parent  MetricNode
	path    string
}

func NewScannerTimedActionsNode(actions map[string]TimedAction, parent MetricNode, path string) *ScannerTimedActionsNode {
	return &ScannerTimedActionsNode{actions: actions, parent: parent, path: path}
}

func (node *ScannerTimedActionsNode) GetChildren() []MetricChild {
	var children []MetricChild
	for actionType := range node.actions {
		children = append(children, MetricChild{
			Name:        actionType,
			Description: fmt.Sprintf("Timing statistics for %s actions", actionType),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})
	return children
}

func (node *ScannerTimedActionsNode) GetLeafData() map[string]string {
	data := map[string]string{
		"action_types": strconv.Itoa(len(node.actions)),
	}
	var totalCount, totalTime uint64
	for actionType, action := range node.actions {
		data[actionType+"_count"] = strconv.FormatUint(action.Count, 10)
		data[actionType+"_time"] = strconv.FormatUint(action.AccTime, 10)
		totalCount += action.Count
		totalTime += action.AccTime
	}
	data["total_count"] = strconv.FormatUint(totalCount, 10)
	data["total_time"] = strconv.FormatUint(totalTime, 10)
	return data
}

func (node *ScannerTimedActionsNode) GetMetricType() MetricType       { return MetricsScanner }
func (node *ScannerTimedActionsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ScannerTimedActionsNode) GetParent() MetricNode           { return node.parent }
func (node *ScannerTimedActionsNode) GetPath() string                 { return node.path }
func (node *ScannerTimedActionsNode) RequiredMetricTypes() MetricType { return MetricsScanner }
func (node *ScannerTimedActionsNode) GetChild(name string) (MetricNode, error) {
	if action, exists := node.actions[name]; exists {
		return NewScannerTimedActionNode(name, &action, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("action not found: %s", name)
}

type ScannerTimedActionNode struct {
	actionType string
	action     *TimedAction
	parent     MetricNode
	path       string
}

func NewScannerTimedActionNode(actionType string, action *TimedAction, parent MetricNode, path string) *ScannerTimedActionNode {
	return &ScannerTimedActionNode{actionType: actionType, action: action, parent: parent, path: path}
}

func (node *ScannerTimedActionNode) GetChildren() []MetricChild { return []MetricChild{} }
func (node *ScannerTimedActionNode) GetLeafData() map[string]string {
	return map[string]string{
		"action_type": node.actionType,
		"count":       strconv.FormatUint(node.action.Count, 10),
		"acc_time":    strconv.FormatUint(node.action.AccTime, 10),
		"min_time":    strconv.FormatUint(node.action.MinTime, 10),
		"max_time":    strconv.FormatUint(node.action.MaxTime, 10),
		"bytes":       strconv.FormatUint(node.action.Bytes, 10),
	}
}
func (node *ScannerTimedActionNode) GetMetricType() MetricType       { return MetricsScanner }
func (node *ScannerTimedActionNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ScannerTimedActionNode) GetParent() MetricNode           { return node.parent }
func (node *ScannerTimedActionNode) GetPath() string                 { return node.path }
func (node *ScannerTimedActionNode) RequiredMetricTypes() MetricType { return MetricsScanner }
func (node *ScannerTimedActionNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("timed action is a leaf node")
}

type ScannerPathsNode struct {
	paths    []string
	pathType string // "active" or "excessive"
	parent   MetricNode
	path     string
}

func NewScannerPathsNode(paths []string, pathType string, parent MetricNode, path string) *ScannerPathsNode {
	return &ScannerPathsNode{paths: paths, pathType: pathType, parent: parent, path: path}
}

func (node *ScannerPathsNode) GetChildren() []MetricChild { return []MetricChild{} }
func (node *ScannerPathsNode) GetLeafData() map[string]string {
	data := map[string]string{
		"path_type":  node.pathType,
		"path_count": strconv.Itoa(len(node.paths)),
	}
	for i, path := range node.paths {
		data[fmt.Sprintf("path_%d", i)] = path
	}
	return data
}
func (node *ScannerPathsNode) GetMetricType() MetricType       { return MetricsScanner }
func (node *ScannerPathsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ScannerPathsNode) GetParent() MetricNode           { return node.parent }
func (node *ScannerPathsNode) GetPath() string                 { return node.path }
func (node *ScannerPathsNode) RequiredMetricTypes() MetricType { return MetricsScanner }
func (node *ScannerPathsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("paths node is a leaf node")
}

type ScannerOpCountNode struct {
	opType string
	count  uint64
	parent MetricNode
	path   string
}

func NewScannerOpCountNode(opType string, count uint64, parent MetricNode, path string) *ScannerOpCountNode {
	return &ScannerOpCountNode{opType: opType, count: count, parent: parent, path: path}
}

func (node *ScannerOpCountNode) GetChildren() []MetricChild { return []MetricChild{} }
func (node *ScannerOpCountNode) GetLeafData() map[string]string {
	return map[string]string{
		"operation_type": node.opType,
		"count":          strconv.FormatUint(node.count, 10),
	}
}
func (node *ScannerOpCountNode) GetMetricType() MetricType       { return MetricsScanner }
func (node *ScannerOpCountNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ScannerOpCountNode) GetParent() MetricNode           { return node.parent }
func (node *ScannerOpCountNode) GetPath() string                 { return node.path }
func (node *ScannerOpCountNode) RequiredMetricTypes() MetricType { return MetricsScanner }
func (node *ScannerOpCountNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("operation count is a leaf node")
}
