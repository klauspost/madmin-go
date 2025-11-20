package madmin

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// RPCMetrics contains metrics for RPC operations.
// Metrics are collected on the sender side of RPC calls.
type RPCMetrics struct {
	Nodes int `json:"nodes,omitempty"`

	CollectedAt time.Time `json:"collected"`

	// Connection stats accumulated for grid systems running on nodes.
	//nolint:staticcheck // SA5008
	ConnectionStats `json:",flatten"`

	// Last minute operation statistics by handler.
	LastMinute map[string]RPCStats `json:"lastMinute,omitempty"`

	// Last day operation statistics by handler, segmented.
	LastDay map[string]SegmentedRPCMetrics `json:"lastDay,omitempty"`

	ByDestination map[string]ConnectionStats `json:"byDestination,omitempty"`
	ByCaller      map[string]ConnectionStats `json:"byCaller,omitempty"`
}

// Merge other into 'm'.
func (m *RPCMetrics) Merge(other *RPCMetrics) {
	if m == nil || other == nil {
		return
	}
	m.Nodes += other.Nodes
	if m.CollectedAt.Before(other.CollectedAt) {
		// Use latest timestamp
		m.CollectedAt = other.CollectedAt
	}

	m.ConnectionStats.Merge(&other.ConnectionStats)

	for k, v := range other.ByDestination {
		if m.ByDestination == nil {
			m.ByDestination = make(map[string]ConnectionStats, len(other.ByDestination))
		}
		existing := m.ByDestination[k]
		existing.Merge(&v)
		m.ByDestination[k] = existing
	}

	for k, v := range other.ByCaller {
		if m.ByCaller == nil {
			m.ByCaller = make(map[string]ConnectionStats, len(other.ByCaller))
		}
		existing := m.ByCaller[k]
		existing.Merge(&v)
		m.ByCaller[k] = existing
	}

	for k, v := range other.LastMinute {
		if m.LastMinute == nil {
			m.LastMinute = make(map[string]RPCStats, len(other.LastMinute))
		}
		existing := m.LastMinute[k]
		existing.Merge(v)
		m.LastMinute[k] = existing
	}
	for k, v := range other.LastDay {
		if m.LastDay == nil {
			m.LastDay = make(map[string]SegmentedRPCMetrics, len(other.LastDay))
		}
		existing, ok := m.LastDay[k]
		if !ok {
			// Deep copy to avoid sharing slice references
			vCopy := v
			if len(v.Segments) > 0 {
				vCopy.Segments = append([]RPCStats{}, v.Segments...)
			}
			m.LastDay[k] = existing
			continue
		}
		existing.Add(&v)
		m.LastDay[k] = existing
	}
}

// LastMinuteTotal returns the total RPCStats for the last minute.
func (m *RPCMetrics) LastMinuteTotal() RPCStats {
	var res RPCStats
	for _, stats := range m.LastMinute {
		res.Merge(stats)
	}
	// Since we are merging across APIs must reset track node count.
	return res
}

// LastDayTotalSegmented returns the total SegmentedRPCMetrics for the last day.
func (m *RPCMetrics) LastDayTotalSegmented() SegmentedRPCMetrics {
	var res SegmentedRPCMetrics
	for _, stats := range m.LastDay {
		res.Add(&stats)
	}
	return res
}

// LastDayTotal returns the accumulated RPCStats for the last day.
func (m *RPCMetrics) LastDayTotal() RPCStats {
	var res RPCStats
	for _, stats := range m.LastDay {
		for _, s := range stats.Segments {
			res.Merge(s)
		}
	}
	return res
}

// ConnectionStats are the overall connection stats.
type ConnectionStats struct {
	Connected        int       `json:"connected,omitempty"`
	Disconnected     int       `json:"disconnected,omitempty"`
	ReconnectCount   int       `json:"reconnectCount,omitempty"` // Total reconnects.
	OutgoingStreams  int       `json:"outgoingStreams,omitempty"`
	IncomingStreams  int       `json:"incomingStreams,omitempty"`
	OutgoingMessages int64     `json:"outgoingMessages,omitempty"`
	IncomingMessages int64     `json:"incomingMessages,omitempty"`
	OutgoingBytes    int64     `json:"outgoingBytes,omitempty"` // Total number of bytes sent.
	IncomingBytes    int64     `json:"incomingBytes,omitempty"` // Total number of bytes received.
	OutQueue         int       `json:"outQueue,omitempty"`
	LastPongTime     time.Time `json:"lastPongTime,omitempty"`
	LastConnectTime  time.Time `json:"lastConnectTime,omitempty"`
	LastPingMS       float64   `json:"lastPingMS,omitempty"`
	MaxPingDurMS     float64   `json:"maxPingDurMS,omitempty"` // Maximum across all merged entries.
}

// Merge other into c.
func (c *ConnectionStats) Merge(other *ConnectionStats) {
	if other == nil {
		return
	}
	c.Connected += other.Connected
	c.Disconnected += other.Disconnected
	c.ReconnectCount += other.ReconnectCount
	c.OutgoingStreams += other.OutgoingStreams
	c.IncomingStreams += other.IncomingStreams
	c.OutgoingMessages += other.OutgoingMessages
	c.IncomingMessages += other.IncomingMessages
	c.OutgoingBytes += other.OutgoingBytes
	c.IncomingBytes += other.IncomingBytes
	c.OutQueue += other.OutQueue
	if c.LastPongTime.Before(other.LastPongTime) {
		c.LastPongTime = other.LastPongTime
		c.LastPingMS = other.LastPingMS
	}
	if c.LastConnectTime.Before(other.LastConnectTime) {
		c.LastConnectTime = other.LastConnectTime
	}
	if c.MaxPingDurMS < other.MaxPingDurMS {
		c.MaxPingDurMS = other.MaxPingDurMS
	}
}

// SegmentedRPCMetrics are segmented RPC metrics.
type SegmentedRPCMetrics = Segmented[RPCStats, *RPCStats]

// RPCStats contains RPC statistics for RPC requests through grid.
type RPCStats struct {
	StartTime       *time.Time `json:"startTime,omitempty"`       // Time range this data covers unless merged from sources with different start times..
	EndTime         *time.Time `json:"endTime,omitempty"`         // Time range this data covers unless merged from sources with different end times.
	WallTimeSecs    float64    `json:"wallTimeSecs,omitempty"`    // Wall time this data covers, accumulated from all nodes.
	Requests        int64      `json:"requests,omitempty"`        // Total number of requests.
	RequestTimeSecs float64    `json:"requestTimeSecs,omitempty"` // Total request time.
	IncomingBytes   int64      `json:"incomingBytes,omitempty"`   // Total number of bytes received.
	OutgoingBytes   int64      `json:"outgoingBytes,omitempty"`   // Total number of bytes sent.
}

// Add 'other' to a.
func (a *RPCStats) Add(other *RPCStats) {
	if other == nil {
		return
	}
	a.Merge(*other)
}

// Merge other into 'a'.
func (a *RPCStats) Merge(other RPCStats) {
	if a.StartTime == nil && a.Requests == 0 {
		a.StartTime = other.StartTime
	}
	if a.EndTime == nil && a.Requests == 0 {
		a.EndTime = other.EndTime
	}
	if a.StartTime != nil && other.StartTime != nil && !a.StartTime.Equal(*other.StartTime) {
		a.StartTime = nil
	}
	if a.EndTime != nil && other.EndTime != nil && !a.EndTime.Equal(*other.EndTime) {
		a.EndTime = nil
	}
	a.WallTimeSecs += other.WallTimeSecs
	a.Requests += other.Requests
	a.IncomingBytes += other.IncomingBytes
	a.OutgoingBytes += other.OutgoingBytes
	a.RequestTimeSecs += other.RequestTimeSecs
}

// String returns a human-readable representation of RPCStats
func (r RPCStats) String() string {
	if r.Requests == 0 {
		return "No RPC requests recorded"
	}

	var parts []string

	// Request summary
	parts = append(parts, fmt.Sprintf("Requests: %s", humanize.Comma(r.Requests)))

	// Timing information
	if r.Requests > 0 {
		avgLatency := (r.RequestTimeSecs / float64(r.Requests)) * 1000
		parts = append(parts, fmt.Sprintf("Avg Latency: %.2fms", avgLatency))
	}

	// Throughput
	totalBytes := r.IncomingBytes + r.OutgoingBytes
	if totalBytes > 0 {
		parts = append(parts, fmt.Sprintf("Throughput: %s", humanize.Bytes(uint64(totalBytes))))
		if r.Requests > 0 {
			avgBytesPerReq := totalBytes / r.Requests
			parts = append(parts, fmt.Sprintf("Avg/Request: %s", humanize.Bytes(uint64(avgBytesPerReq))))
		}
	}

	return strings.Join(parts, ", ")
}

// String returns a human-readable representation of RPCMetrics
func (r RPCMetrics) String() string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Collected: %s", r.CollectedAt.Format("15:04:05")))
	parts = append(parts, fmt.Sprintf("Nodes: %d", r.Nodes))

	// Connection status
	parts = append(parts, fmt.Sprintf("Connected: %d", r.Connected))
	if r.Disconnected > 0 {
		parts = append(parts, fmt.Sprintf("Disconnected: %d", r.Disconnected))
	}

	// Last minute summary
	var totalRequests int64
	for _, stats := range r.LastMinute {
		totalRequests += stats.Requests
	}
	if totalRequests > 0 {
		parts = append(parts, fmt.Sprintf("Last Minute: %s req", humanize.Comma(totalRequests)))
	}

	// Handlers
	if len(r.LastMinute) > 0 {
		parts = append(parts, fmt.Sprintf("Active Handlers: %d", len(r.LastMinute)))
	}

	return strings.Join(parts, " | ")
}

// RPCMetricsNode represents the root RPC metrics node
type RPCMetricsNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCMetricsNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "last_minute", Description: "Last minute RPC statistics by handler"},
		{Name: "last_day", Description: "Last day RPC statistics segmented"},
		{Name: "by_destination", Description: "RPC statistics grouped by destination"},
		{Name: "by_caller", Description: "RPC statistics grouped by caller"},
	}
}

func (node *RPCMetricsNode) GetLeafData() map[string]string {
	return node.generateRPCOverviewDashboard()
}

func (node *RPCMetricsNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCMetricsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RPCMetricsNode) GetParent() MetricNode           { return node.parent }
func (node *RPCMetricsNode) GetPath() string                 { return node.path }
func (node *RPCMetricsNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCMetricsNode) ShouldPauseRefresh() bool        { return false }

func (node *RPCMetricsNode) GetChild(name string) (MetricNode, error) {
	switch name {
	case "last_minute":
		return &RPCLastMinuteNode{
			rpc:    node.rpc,
			parent: node,
			path:   node.path + "/last_minute",
		}, nil
	case "last_day":
		return &RPCLastDayNode{
			rpc:    node.rpc,
			parent: node,
			path:   node.path + "/last_day",
		}, nil
	case "by_destination":
		return &RPCByDestinationNode{
			rpc:    node.rpc,
			parent: node,
			path:   node.path + "/by_destination",
		}, nil
	case "by_caller":
		return &RPCByCallerNode{
			rpc:    node.rpc,
			parent: node,
			path:   node.path + "/by_caller",
		}, nil
	default:
		return nil, fmt.Errorf("unknown RPC metric child: %s", name)
	}
}

// RPCLastMinuteNode shows last minute RPC statistics by handler
type RPCLastMinuteNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCLastMinuteNode) ShouldPauseRefresh() bool { return false }

func (node *RPCLastMinuteNode) GetChildren() []MetricChild {
	// No children - all data shown as leaf data
	return []MetricChild{}
}

func (node *RPCLastMinuteNode) GetLeafData() map[string]string {
	if node.rpc == nil || len(node.rpc.LastMinute) == 0 {
		return map[string]string{"Status": "No RPC requests recorded"}
	}

	data := make(map[string]string)

	// Get sorted handler names
	var handlers []string
	for handler := range node.rpc.LastMinute {
		handlers = append(handlers, handler)
	}
	sort.Strings(handlers)

	// Add individual handler statistics
	for _, handler := range handlers {
		stats := node.rpc.LastMinute[handler]
		if stats.Requests == 0 {
			continue // Skip handlers with no requests
		}

		var parts []string

		// Average time
		if stats.Requests > 0 && stats.RequestTimeSecs > 0 {
			avgLatency := (stats.RequestTimeSecs / float64(stats.Requests)) * 1000
			parts = append(parts, fmt.Sprintf("avg time: %.2fms", avgLatency))
		} else {
			parts = append(parts, "avg time: 0ms")
		}

		// RPS (requests per second)
		rps := float64(stats.Requests) / 60.0 // over the minute
		parts = append(parts, fmt.Sprintf("rps: %.2f", rps))

		// Incoming bytes per second
		if stats.IncomingBytes > 0 {
			inBps := float64(stats.IncomingBytes) / 60.0
			parts = append(parts, fmt.Sprintf("in: %s/s", humanize.Bytes(uint64(inBps))))
		} else {
			parts = append(parts, "in: 0B/s")
		}

		// Outgoing bytes per second
		if stats.OutgoingBytes > 0 {
			outBps := float64(stats.OutgoingBytes) / 60.0
			parts = append(parts, fmt.Sprintf("out: %s/s", humanize.Bytes(uint64(outBps))))
		} else {
			parts = append(parts, "out: 0B/s")
		}

		// Total number of requests
		parts = append(parts, fmt.Sprintf("n: %s", humanize.Comma(stats.Requests)))

		data[handler] = strings.Join(parts, ", ")
	}

	return data
}

func (node *RPCLastMinuteNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCLastMinuteNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RPCLastMinuteNode) GetParent() MetricNode           { return node.parent }
func (node *RPCLastMinuteNode) GetPath() string                 { return node.path }
func (node *RPCLastMinuteNode) RequiredMetricTypes() MetricType { return MetricsRPC }

func (node *RPCLastMinuteNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for last minute RPC stats")
}

// RPCLastDayNode shows last day RPC statistics segmented
type RPCLastDayNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCLastDayNode) ShouldPauseRefresh() bool { return true }

func (node *RPCLastDayNode) GetChildren() []MetricChild {
	if len(node.rpc.LastDay) == 0 {
		return []MetricChild{}
	}

	var children []MetricChild

	// Add "All" entry first
	children = append(children, MetricChild{
		Name:        "All",
		Description: "Aggregated statistics for all RPC handlers",
	})

	// Add individual handlers, sorted alphabetically
	var handlerNames []string
	for handlerName := range node.rpc.LastDay {
		handlerNames = append(handlerNames, handlerName)
	}
	sort.Strings(handlerNames)

	for _, handlerName := range handlerNames {
		segmented := node.rpc.LastDay[handlerName]
		totalRequests := int64(0)
		for _, segment := range segmented.Segments {
			totalRequests += segment.Requests
		}

		children = append(children, MetricChild{
			Name:        handlerName,
			Description: fmt.Sprintf("Last day statistics for %s (%d total requests)", handlerName, totalRequests),
		})
	}

	return children
}

func (node *RPCLastDayNode) GetLeafData() map[string]string {
	if len(node.rpc.LastDay) == 0 {
		return map[string]string{"Status": "No last day RPC data available"}
	}

	// Calculate total across all handlers
	var totalStats RPCStats
	for _, segmented := range node.rpc.LastDay {
		for _, segment := range segmented.Segments {
			totalStats.Merge(segment)
		}
	}

	return generateRPCStatsDisplay(totalStats, len(node.rpc.LastDay), false, nil)
}

func (node *RPCLastDayNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCLastDayNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *RPCLastDayNode) GetParent() MetricNode           { return node.parent }
func (node *RPCLastDayNode) GetPath() string                 { return node.path }
func (node *RPCLastDayNode) RequiredMetricTypes() MetricType { return MetricsRPC }

func (node *RPCLastDayNode) GetChild(name string) (MetricNode, error) {
	// Handle "All" entry
	if name == "All" {
		return &RPCLastDayAllNode{
			rpc:    node.rpc,
			parent: node,
			path:   node.path + "/" + name,
		}, nil
	}

	// Handle individual handlers
	if segmented, exists := node.rpc.LastDay[name]; exists {
		return &RPCLastDayHandlerNode{
			rpc:         node.rpc,
			handlerName: name,
			segmented:   segmented,
			parent:      node,
			path:        node.path + "/" + name,
		}, nil
	}

	return nil, fmt.Errorf("RPC handler not found: %s", name)
}

// RPCLastDayAllNode shows aggregated time segments for all handlers
type RPCLastDayAllNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCLastDayAllNode) ShouldPauseRefresh() bool { return true }

func (node *RPCLastDayAllNode) GetChildren() []MetricChild {
	if len(node.rpc.LastDay) == 0 {
		return []MetricChild{}
	}

	// Get any segmented data to determine time segments
	var firstSegmented *SegmentedRPCMetrics
	for _, segmented := range node.rpc.LastDay {
		firstSegmented = &segmented
		break
	}

	if firstSegmented == nil || len(firstSegmented.Segments) == 0 {
		return []MetricChild{}
	}

	var children []MetricChild

	// Add "Total" entry first
	children = append(children, MetricChild{
		Name:        "Total",
		Description: "Last day total statistics across all time segments",
	})

	// Add time segments, most recent first (filter out empty segments)
	for i := len(firstSegmented.Segments) - 1; i >= 0; i-- {
		segmentTime := firstSegmented.FirstTime.Add(time.Duration(i*firstSegmented.Interval) * time.Second)
		endTime := segmentTime.Add(time.Duration(firstSegmented.Interval) * time.Second)
		segmentName := segmentTime.UTC().Format("15:04Z")

		// Calculate total requests for this time segment across all handlers
		totalRequests := int64(0)
		for _, segmented := range node.rpc.LastDay {
			if i < len(segmented.Segments) {
				totalRequests += segmented.Segments[i].Requests
			}
		}

		// Filter out time segments with no requests
		if totalRequests == 0 {
			continue
		}

		children = append(children, MetricChild{
			Name: segmentName,
			Description: fmt.Sprintf("RPC %s -> %s (%d requests)",
				segmentTime.Local().Format("15:04"),
				endTime.Local().Format("15:04"),
				totalRequests),
		})
	}

	return children
}

func (node *RPCLastDayAllNode) GetLeafData() map[string]string {
	// Calculate total across all handlers and segments
	var totalStats RPCStats
	for _, segmented := range node.rpc.LastDay {
		for _, segment := range segmented.Segments {
			totalStats.Merge(segment)
		}
	}
	return generateRPCStatsDisplay(totalStats, len(node.rpc.LastDay), false, nil)
}

func (node *RPCLastDayAllNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCLastDayAllNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *RPCLastDayAllNode) GetParent() MetricNode           { return node.parent }
func (node *RPCLastDayAllNode) GetPath() string                 { return node.path }
func (node *RPCLastDayAllNode) RequiredMetricTypes() MetricType { return MetricsRPC }

func (node *RPCLastDayAllNode) GetChild(name string) (MetricNode, error) {
	if len(node.rpc.LastDay) == 0 {
		return nil, fmt.Errorf("no last day segmented data available")
	}

	// Handle "Total" entry
	if name == "Total" {
		return &RPCLastDayTotalNode{
			rpc:    node.rpc,
			parent: node,
			path:   node.path + "/" + name,
		}, nil
	}

	// Get segment information from first handler
	var firstSegmented *SegmentedRPCMetrics
	for _, segmented := range node.rpc.LastDay {
		firstSegmented = &segmented
		break
	}

	if firstSegmented == nil {
		return nil, fmt.Errorf("no segmented data available")
	}

	// Handle time segments
	for i := len(firstSegmented.Segments) - 1; i >= 0; i-- {
		segmentTime := firstSegmented.FirstTime.Add(time.Duration(i*firstSegmented.Interval) * time.Second)
		if segmentTime.UTC().Format("15:04Z") == name {
			// Aggregate this time segment across all handlers
			var aggregatedStats RPCStats
			for _, segmented := range node.rpc.LastDay {
				if i < len(segmented.Segments) {
					aggregatedStats.Merge(segmented.Segments[i])
				}
			}

			return &RPCTimeSegmentAllNode{
				segment:     aggregatedStats,
				segmentTime: segmentTime,
				parent:      node,
				path:        node.path + "/" + name,
			}, nil
		}
	}

	return nil, fmt.Errorf("time segment not found: %s", name)
}

// RPCLastDayTotalNode shows total last day statistics
type RPCLastDayTotalNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCLastDayTotalNode) ShouldPauseRefresh() bool   { return true }
func (node *RPCLastDayTotalNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *RPCLastDayTotalNode) GetLeafData() map[string]string {
	var totalStats RPCStats
	for _, segmented := range node.rpc.LastDay {
		for _, segment := range segmented.Segments {
			totalStats.Merge(segment)
		}
	}
	return generateRPCStatsDisplay(totalStats, len(node.rpc.LastDay), false, nil)
}

func (node *RPCLastDayTotalNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCLastDayTotalNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *RPCLastDayTotalNode) GetParent() MetricNode           { return node.parent }
func (node *RPCLastDayTotalNode) GetPath() string                 { return node.path }
func (node *RPCLastDayTotalNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCLastDayTotalNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for last day total node")
}

// RPCTimeSegmentAllNode shows aggregated RPC statistics for a specific time segment
type RPCTimeSegmentAllNode struct {
	segment     RPCStats
	segmentTime time.Time
	parent      MetricNode
	path        string
}

func (node *RPCTimeSegmentAllNode) ShouldPauseRefresh() bool   { return true }
func (node *RPCTimeSegmentAllNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *RPCTimeSegmentAllNode) GetLeafData() map[string]string {
	return generateRPCStatsDisplay(node.segment, 1, false, nil)
}

func (node *RPCTimeSegmentAllNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCTimeSegmentAllNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *RPCTimeSegmentAllNode) GetParent() MetricNode           { return node.parent }
func (node *RPCTimeSegmentAllNode) GetPath() string                 { return node.path }
func (node *RPCTimeSegmentAllNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCTimeSegmentAllNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for time segment")
}

// RPCLastDayHandlerNode shows segmented statistics for a specific RPC handler
type RPCLastDayHandlerNode struct {
	rpc         *RPCMetrics
	handlerName string
	segmented   SegmentedRPCMetrics
	parent      MetricNode
	path        string
}

func (node *RPCLastDayHandlerNode) ShouldPauseRefresh() bool { return true }

func (node *RPCLastDayHandlerNode) GetChildren() []MetricChild {
	if len(node.segmented.Segments) == 0 {
		return []MetricChild{}
	}

	var children []MetricChild

	// Add "Total" entry first
	children = append(children, MetricChild{
		Name:        "Total",
		Description: fmt.Sprintf("Total last day statistics for %s", node.handlerName),
	})

	// Add time segments, most recent first (filter out empty segments)
	for i := len(node.segmented.Segments) - 1; i >= 0; i-- {
		segment := node.segmented.Segments[i]

		// Filter out time segments with no requests
		if segment.Requests == 0 {
			continue
		}

		segmentTime := node.segmented.FirstTime.Add(time.Duration(i*node.segmented.Interval) * time.Second)
		endTime := segmentTime.Add(time.Duration(node.segmented.Interval) * time.Second)
		segmentName := segmentTime.UTC().Format("15:04Z")

		children = append(children, MetricChild{
			Name: segmentName,
			Description: fmt.Sprintf("%s %s -> %s (%d requests)",
				node.handlerName,
				segmentTime.Local().Format("15:04"),
				endTime.Local().Format("15:04"),
				segment.Requests),
		})
	}

	return children
}

func (node *RPCLastDayHandlerNode) GetLeafData() map[string]string {
	// Calculate total for this handler
	var totalStats RPCStats
	for _, segment := range node.segmented.Segments {
		totalStats.Merge(segment)
	}
	return generateRPCStatsDisplay(totalStats, 1, false, nil)
}

func (node *RPCLastDayHandlerNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCLastDayHandlerNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *RPCLastDayHandlerNode) GetParent() MetricNode           { return node.parent }
func (node *RPCLastDayHandlerNode) GetPath() string                 { return node.path }
func (node *RPCLastDayHandlerNode) RequiredMetricTypes() MetricType { return MetricsRPC }

func (node *RPCLastDayHandlerNode) GetChild(name string) (MetricNode, error) {
	// Handle "Total" entry
	if name == "Total" {
		var totalStats RPCStats
		for _, segment := range node.segmented.Segments {
			totalStats.Merge(segment)
		}

		return &RPCHandlerTotalNode{
			handler:   node.handlerName,
			stats:     totalStats,
			parent:    node,
			path:      node.path + "/" + name,
			timeRange: "last day",
		}, nil
	}

	// Handle time segments
	for i := len(node.segmented.Segments) - 1; i >= 0; i-- {
		segmentTime := node.segmented.FirstTime.Add(time.Duration(i*node.segmented.Interval) * time.Second)
		if segmentTime.UTC().Format("15:04Z") == name {
			return &RPCHandlerSegmentNode{
				handler:     node.handlerName,
				stats:       node.segmented.Segments[i],
				segmentTime: segmentTime,
				parent:      node,
				path:        node.path + "/" + name,
			}, nil
		}
	}

	return nil, fmt.Errorf("time segment not found: %s", name)
}

// RPCConnectionsNode shows RPC connection statistics and health
type RPCConnectionsNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCConnectionsNode) ShouldPauseRefresh() bool { return false }

func (node *RPCConnectionsNode) GetChildren() []MetricChild {
	var children []MetricChild

	// Connection summary
	children = append(children, MetricChild{
		Name:        "summary",
		Description: fmt.Sprintf("Connection health overview (Connected: %d, Disconnected: %d)", node.rpc.Connected, node.rpc.Disconnected),
	})

	return children
}

func (node *RPCConnectionsNode) GetLeafData() map[string]string {
	data := make(map[string]string)

	data["Total Nodes"] = fmt.Sprintf("%d", node.rpc.Nodes)
	data["Connected"] = fmt.Sprintf("%d", node.rpc.Connected)
	data["Disconnected"] = fmt.Sprintf("%d", node.rpc.Disconnected)

	if node.rpc.Nodes > 0 {
		connectionRate := float64(node.rpc.Connected) / float64(node.rpc.Nodes) * 100
		data["Connection Rate"] = fmt.Sprintf("%.1f%%", connectionRate)

		// Health assessment
		if connectionRate >= 95.0 {
			data["Health Status"] = "Excellent"
		} else if connectionRate >= 80.0 {
			data["Health Status"] = "Good"
		} else if connectionRate >= 60.0 {
			data["Health Status"] = "Fair"
		} else {
			data["Health Status"] = "Poor"
		}
	}

	data["Last Updated"] = node.rpc.CollectedAt.Format("2006-01-02 15:04:05")

	return data
}

func (node *RPCConnectionsNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCConnectionsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RPCConnectionsNode) GetParent() MetricNode           { return node.parent }
func (node *RPCConnectionsNode) GetPath() string                 { return node.path }
func (node *RPCConnectionsNode) RequiredMetricTypes() MetricType { return MetricsRPC }

func (node *RPCConnectionsNode) GetChild(name string) (MetricNode, error) {
	switch name {
	case "summary":
		return &RPCConnectionSummaryNode{
			rpc:    node.rpc,
			parent: node,
			path:   node.path + "/" + name,
		}, nil
	default:
		return nil, fmt.Errorf("connection child not found: %s", name)
	}
}

// RPCConnectionSummaryNode shows connection summary details
type RPCConnectionSummaryNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCConnectionSummaryNode) ShouldPauseRefresh() bool   { return false }
func (node *RPCConnectionSummaryNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *RPCConnectionSummaryNode) GetLeafData() map[string]string {
	data := make(map[string]string)

	data["Cluster Nodes"] = fmt.Sprintf("%d nodes configured", node.rpc.Nodes)
	data["Connected Nodes"] = fmt.Sprintf("%d online", node.rpc.Connected)
	if node.rpc.Disconnected > 0 {
		data["Disconnected Nodes"] = fmt.Sprintf("%d offline", node.rpc.Disconnected)
	}

	if node.rpc.Nodes > 0 {
		uptime := float64(node.rpc.Connected) / float64(node.rpc.Nodes) * 100
		data["Cluster Availability"] = fmt.Sprintf("%.2f%%", uptime)
	}

	// Add activity summary
	totalActivity := int64(0)
	for _, stats := range node.rpc.LastMinute {
		totalActivity += stats.Requests
	}
	if totalActivity > 0 {
		data["Recent Activity"] = fmt.Sprintf("%s requests (last minute)", humanize.Comma(totalActivity))
	} else {
		data["Recent Activity"] = "No recent RPC activity"
	}

	return data
}

func (node *RPCConnectionSummaryNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCConnectionSummaryNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RPCConnectionSummaryNode) GetParent() MetricNode           { return node.parent }
func (node *RPCConnectionSummaryNode) GetPath() string                 { return node.path }
func (node *RPCConnectionSummaryNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCConnectionSummaryNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for connection summary")
}

// RPCByDestinationNode groups RPC statistics by destination
type RPCByDestinationNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCByDestinationNode) ShouldPauseRefresh() bool { return false }

func (node *RPCByDestinationNode) GetChildren() []MetricChild {
	if len(node.rpc.ByDestination) == 0 {
		return []MetricChild{}
	}

	var destinations []string
	for dest := range node.rpc.ByDestination {
		destinations = append(destinations, dest)
	}
	sort.Strings(destinations)

	var children []MetricChild
	for _, dest := range destinations {
		stats := node.rpc.ByDestination[dest]

		var parts []string

		// Connection status
		if stats.Connected > 0 {
			parts = append(parts, fmt.Sprintf("%d connected", stats.Connected))
		}
		if stats.Disconnected > 0 {
			parts = append(parts, fmt.Sprintf("%d disconnected", stats.Disconnected))
		}

		// Message counts
		if stats.OutgoingMessages > 0 {
			parts = append(parts, fmt.Sprintf("%s out msgs", humanize.Comma(stats.OutgoingMessages)))
		}
		if stats.IncomingMessages > 0 {
			parts = append(parts, fmt.Sprintf("%s in msgs", humanize.Comma(stats.IncomingMessages)))
		}

		// Ping info
		if stats.LastPingMS > 0 {
			parts = append(parts, fmt.Sprintf("%.1fms ping", stats.LastPingMS))
		}

		description := strings.Join(parts, ", ")
		if description == "" {
			description = "No activity"
		}

		children = append(children, MetricChild{
			Name:        dest,
			Description: description,
		})
	}
	return children
}

func (node *RPCByDestinationNode) GetLeafData() map[string]string {
	if len(node.rpc.ByDestination) == 0 {
		return map[string]string{"Status": "No destination data available"}
	}

	data := make(map[string]string)

	var totalConnected, totalDisconnected int
	var totalOutgoing, totalIncoming int64
	var totalOutBytes, totalInBytes int64

	for _, stats := range node.rpc.ByDestination {
		totalConnected += stats.Connected
		totalDisconnected += stats.Disconnected
		totalOutgoing += stats.OutgoingMessages
		totalIncoming += stats.IncomingMessages
		totalOutBytes += stats.OutgoingBytes
		totalInBytes += stats.IncomingBytes
	}

	data["Active Destinations"] = fmt.Sprintf("%d", len(node.rpc.ByDestination))
	data["Total Connected"] = fmt.Sprintf("%d", totalConnected)
	if totalDisconnected > 0 {
		data["Total Disconnected"] = fmt.Sprintf("%d", totalDisconnected)
	}
	data["Total Outgoing Messages"] = humanize.Comma(totalOutgoing)
	data["Total Incoming Messages"] = humanize.Comma(totalIncoming)

	if totalOutBytes > 0 {
		data["Total Outgoing Bytes"] = humanize.Bytes(uint64(totalOutBytes))
	}
	if totalInBytes > 0 {
		data["Total Incoming Bytes"] = humanize.Bytes(uint64(totalInBytes))
	}

	return data
}

func (node *RPCByDestinationNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCByDestinationNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RPCByDestinationNode) GetParent() MetricNode           { return node.parent }
func (node *RPCByDestinationNode) GetPath() string                 { return node.path }
func (node *RPCByDestinationNode) RequiredMetricTypes() MetricType { return MetricsRPC }

func (node *RPCByDestinationNode) GetChild(name string) (MetricNode, error) {
	if stats, exists := node.rpc.ByDestination[name]; exists {
		return &RPCDestinationNode{
			destination: name,
			stats:       stats,
			parent:      node,
			path:        node.path + "/" + name,
		}, nil
	}
	return nil, fmt.Errorf("destination not found: %s", name)
}

// RPCByCallerNode groups RPC statistics by caller (similar to destination for now)
type RPCByCallerNode struct {
	rpc    *RPCMetrics
	parent MetricNode
	path   string
}

func (node *RPCByCallerNode) ShouldPauseRefresh() bool { return false }

func (node *RPCByCallerNode) GetChildren() []MetricChild {
	if len(node.rpc.ByCaller) == 0 {
		return []MetricChild{}
	}

	var callers []string
	for caller := range node.rpc.ByCaller {
		callers = append(callers, caller)
	}
	sort.Strings(callers)

	var children []MetricChild
	for _, caller := range callers {
		stats := node.rpc.ByCaller[caller]

		var parts []string

		// Connection status
		if stats.Connected > 0 {
			parts = append(parts, fmt.Sprintf("%d connected", stats.Connected))
		}
		if stats.Disconnected > 0 {
			parts = append(parts, fmt.Sprintf("%d disconnected", stats.Disconnected))
		}

		// Message counts
		if stats.IncomingMessages > 0 {
			parts = append(parts, fmt.Sprintf("%s in msgs", humanize.Comma(stats.IncomingMessages)))
		}
		if stats.OutgoingMessages > 0 {
			parts = append(parts, fmt.Sprintf("%s out msgs", humanize.Comma(stats.OutgoingMessages)))
		}

		// Ping info
		if stats.LastPingMS > 0 {
			parts = append(parts, fmt.Sprintf("%.1fms ping", stats.LastPingMS))
		}

		description := strings.Join(parts, ", ")
		if description == "" {
			description = "No activity"
		}

		children = append(children, MetricChild{
			Name:        caller,
			Description: description,
		})
	}
	return children
}

func (node *RPCByCallerNode) GetLeafData() map[string]string {
	if len(node.rpc.ByCaller) == 0 {
		return map[string]string{"Status": "No caller data available"}
	}

	data := make(map[string]string)

	var totalConnected, totalDisconnected int
	var totalOutgoing, totalIncoming int64
	var totalOutBytes, totalInBytes int64

	for _, stats := range node.rpc.ByCaller {
		totalConnected += stats.Connected
		totalDisconnected += stats.Disconnected
		totalOutgoing += stats.OutgoingMessages
		totalIncoming += stats.IncomingMessages
		totalOutBytes += stats.OutgoingBytes
		totalInBytes += stats.IncomingBytes
	}

	data["Active Callers"] = fmt.Sprintf("%d", len(node.rpc.ByCaller))
	data["Total Connected"] = fmt.Sprintf("%d", totalConnected)
	if totalDisconnected > 0 {
		data["Total Disconnected"] = fmt.Sprintf("%d", totalDisconnected)
	}
	data["Total Outgoing Messages"] = humanize.Comma(totalOutgoing)
	data["Total Incoming Messages"] = humanize.Comma(totalIncoming)

	if totalOutBytes > 0 {
		data["Total Outgoing Bytes"] = humanize.Bytes(uint64(totalOutBytes))
	}
	if totalInBytes > 0 {
		data["Total Incoming Bytes"] = humanize.Bytes(uint64(totalInBytes))
	}

	return data
}

func (node *RPCByCallerNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCByCallerNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RPCByCallerNode) GetParent() MetricNode           { return node.parent }
func (node *RPCByCallerNode) GetPath() string                 { return node.path }
func (node *RPCByCallerNode) RequiredMetricTypes() MetricType { return MetricsRPC }

func (node *RPCByCallerNode) GetChild(name string) (MetricNode, error) {
	if stats, exists := node.rpc.ByCaller[name]; exists {
		return &RPCCallerNode{
			caller: name,
			stats:  stats,
			parent: node,
			path:   node.path + "/" + name,
		}, nil
	}
	return nil, fmt.Errorf("caller not found: %s", name)
}

// RPCDestinationNode shows detailed connection statistics for a specific destination
type RPCDestinationNode struct {
	destination string
	stats       ConnectionStats
	parent      MetricNode
	path        string
}

func (node *RPCDestinationNode) ShouldPauseRefresh() bool   { return true }
func (node *RPCDestinationNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *RPCDestinationNode) GetLeafData() map[string]string {
	data := make(map[string]string)

	data["Destination"] = node.destination
	data["Connected"] = fmt.Sprintf("%d", node.stats.Connected)
	data["Disconnected"] = fmt.Sprintf("%d", node.stats.Disconnected)
	data["Reconnect Count"] = fmt.Sprintf("%d", node.stats.ReconnectCount)

	// Stream information
	if node.stats.OutgoingStreams > 0 {
		data["Outgoing Streams"] = fmt.Sprintf("%d", node.stats.OutgoingStreams)
	}
	if node.stats.IncomingStreams > 0 {
		data["Incoming Streams"] = fmt.Sprintf("%d", node.stats.IncomingStreams)
	}

	// Message counts
	data["Outgoing Messages"] = humanize.Comma(node.stats.OutgoingMessages)
	data["Incoming Messages"] = humanize.Comma(node.stats.IncomingMessages)

	// Bytes
	if node.stats.OutgoingBytes > 0 {
		data["Outgoing Bytes"] = humanize.Bytes(uint64(node.stats.OutgoingBytes))
	}
	if node.stats.IncomingBytes > 0 {
		data["Incoming Bytes"] = humanize.Bytes(uint64(node.stats.IncomingBytes))
	}

	// Queue and timing
	if node.stats.OutQueue > 0 {
		data["Outgoing Queue"] = fmt.Sprintf("%d", node.stats.OutQueue)
	}
	if node.stats.LastPingMS > 0 {
		data["Last Ping"] = fmt.Sprintf("%.2f ms", node.stats.LastPingMS)
	}
	if node.stats.MaxPingDurMS > 0 {
		data["Max Ping"] = fmt.Sprintf("%.2f ms", node.stats.MaxPingDurMS)
	}

	// Connection timing
	if !node.stats.LastConnectTime.IsZero() {
		data["Last Connect"] = node.stats.LastConnectTime.Format("2006-01-02 15:04:05")
	}
	if !node.stats.LastPongTime.IsZero() {
		data["Last Pong"] = node.stats.LastPongTime.Format("2006-01-02 15:04:05")
	}

	return data
}

func (node *RPCDestinationNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCDestinationNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RPCDestinationNode) GetParent() MetricNode           { return node.parent }
func (node *RPCDestinationNode) GetPath() string                 { return node.path }
func (node *RPCDestinationNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCDestinationNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for destination")
}

// RPCCallerNode shows detailed connection statistics for a specific caller
type RPCCallerNode struct {
	caller string
	stats  ConnectionStats
	parent MetricNode
	path   string
}

func (node *RPCCallerNode) ShouldPauseRefresh() bool   { return true }
func (node *RPCCallerNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *RPCCallerNode) GetLeafData() map[string]string {
	data := make(map[string]string)

	data["Caller"] = node.caller
	data["Connected"] = fmt.Sprintf("%d", node.stats.Connected)
	data["Disconnected"] = fmt.Sprintf("%d", node.stats.Disconnected)
	data["Reconnect Count"] = fmt.Sprintf("%d", node.stats.ReconnectCount)

	// Stream information
	if node.stats.IncomingStreams > 0 {
		data["Incoming Streams"] = fmt.Sprintf("%d", node.stats.IncomingStreams)
	}
	if node.stats.OutgoingStreams > 0 {
		data["Outgoing Streams"] = fmt.Sprintf("%d", node.stats.OutgoingStreams)
	}

	// Message counts
	data["Incoming Messages"] = humanize.Comma(node.stats.IncomingMessages)
	data["Outgoing Messages"] = humanize.Comma(node.stats.OutgoingMessages)

	// Bytes
	if node.stats.IncomingBytes > 0 {
		data["Incoming Bytes"] = humanize.Bytes(uint64(node.stats.IncomingBytes))
	}
	if node.stats.OutgoingBytes > 0 {
		data["Outgoing Bytes"] = humanize.Bytes(uint64(node.stats.OutgoingBytes))
	}

	// Queue and timing
	if node.stats.OutQueue > 0 {
		data["Outgoing Queue"] = fmt.Sprintf("%d", node.stats.OutQueue)
	}
	if node.stats.LastPingMS > 0 {
		data["Last Ping"] = fmt.Sprintf("%.2f ms", node.stats.LastPingMS)
	}
	if node.stats.MaxPingDurMS > 0 {
		data["Max Ping"] = fmt.Sprintf("%.2f ms", node.stats.MaxPingDurMS)
	}

	// Connection timing
	if !node.stats.LastConnectTime.IsZero() {
		data["Last Connect"] = node.stats.LastConnectTime.Format("2006-01-02 15:04:05")
	}
	if !node.stats.LastPongTime.IsZero() {
		data["Last Pong"] = node.stats.LastPongTime.Format("2006-01-02 15:04:05")
	}

	return data
}

func (node *RPCCallerNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCCallerNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RPCCallerNode) GetParent() MetricNode           { return node.parent }
func (node *RPCCallerNode) GetPath() string                 { return node.path }
func (node *RPCCallerNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCCallerNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for caller")
}

// RPCHandlerNode shows detailed statistics for a specific RPC handler
type RPCHandlerNode struct {
	handler string
	stats   RPCStats
	parent  MetricNode
	path    string
}

func (node *RPCHandlerNode) ShouldPauseRefresh() bool   { return true }
func (node *RPCHandlerNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *RPCHandlerNode) GetLeafData() map[string]string {
	return generateRPCStatsDisplay(node.stats, 1, false, nil)
}

func (node *RPCHandlerNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCHandlerNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *RPCHandlerNode) GetParent() MetricNode           { return node.parent }
func (node *RPCHandlerNode) GetPath() string                 { return node.path }
func (node *RPCHandlerNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCHandlerNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for RPC handler")
}

// RPCHandlerTotalNode shows total statistics for a handler over a time range
type RPCHandlerTotalNode struct {
	handler   string
	stats     RPCStats
	parent    MetricNode
	path      string
	timeRange string
}

func (node *RPCHandlerTotalNode) ShouldPauseRefresh() bool   { return true }
func (node *RPCHandlerTotalNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *RPCHandlerTotalNode) GetLeafData() map[string]string {
	data := generateRPCStatsDisplay(node.stats, 1, false, nil)
	data["Handler"] = node.handler
	data["Time Range"] = node.timeRange
	return data
}

func (node *RPCHandlerTotalNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCHandlerTotalNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *RPCHandlerTotalNode) GetParent() MetricNode           { return node.parent }
func (node *RPCHandlerTotalNode) GetPath() string                 { return node.path }
func (node *RPCHandlerTotalNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCHandlerTotalNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for handler total")
}

// RPCHandlerSegmentNode shows statistics for a specific handler in a time segment
type RPCHandlerSegmentNode struct {
	handler     string
	stats       RPCStats
	segmentTime time.Time
	parent      MetricNode
	path        string
}

func (node *RPCHandlerSegmentNode) ShouldPauseRefresh() bool   { return true }
func (node *RPCHandlerSegmentNode) GetChildren() []MetricChild { return []MetricChild{} }

func (node *RPCHandlerSegmentNode) GetLeafData() map[string]string {
	data := generateRPCStatsDisplay(node.stats, 1, false, nil)
	data["Handler"] = node.handler
	data["Time Segment"] = node.segmentTime.Format("2006-01-02 15:04:05")
	return data
}

func (node *RPCHandlerSegmentNode) GetMetricType() MetricType       { return MetricsRPC }
func (node *RPCHandlerSegmentNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *RPCHandlerSegmentNode) GetParent() MetricNode           { return node.parent }
func (node *RPCHandlerSegmentNode) GetPath() string                 { return node.path }
func (node *RPCHandlerSegmentNode) RequiredMetricTypes() MetricType { return MetricsRPC }
func (node *RPCHandlerSegmentNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for handler segment")
}

// Helper function to generate RPC statistics display
func generateRPCStatsDisplay(stats RPCStats, handlerCount int, showHandlerBreakdown bool, lastMinute map[string]RPCStats) map[string]string {
	data := make(map[string]string)

	// Basic request statistics
	data["Total Requests"] = humanize.Comma(stats.Requests)

	// Timing information
	if stats.Requests > 0 {
		avgLatency := (stats.RequestTimeSecs / float64(stats.Requests)) * 1000
		data["Average Latency"] = fmt.Sprintf("%.2f ms", avgLatency)
		data["Total Time"] = fmt.Sprintf("%.2f sec", stats.RequestTimeSecs)
	} else {
		data["Average Latency"] = "0 ms"
		data["Total Time"] = "0 sec"
	}

	// Network throughput
	totalBytes := stats.IncomingBytes + stats.OutgoingBytes
	if totalBytes > 0 {
		data["Total Throughput"] = humanize.Bytes(uint64(totalBytes))
		data["Incoming Bytes"] = humanize.Bytes(uint64(stats.IncomingBytes))
		data["Outgoing Bytes"] = humanize.Bytes(uint64(stats.OutgoingBytes))

		if stats.Requests > 0 {
			avgBytesPerReq := totalBytes / stats.Requests
			data["Avg Bytes/Request"] = humanize.Bytes(uint64(avgBytesPerReq))
		}
	} else {
		data["Total Throughput"] = "0 B"
	}

	// Performance metrics
	if stats.Requests > 0 && stats.RequestTimeSecs > 0 {
		rps := float64(stats.Requests) / stats.RequestTimeSecs
		data["Requests/Second"] = fmt.Sprintf("%.1f", rps)

		if totalBytes > 0 {
			bps := float64(totalBytes) / stats.RequestTimeSecs
			data["Bytes/Second"] = humanize.Bytes(uint64(bps)) + "/s"
		}
	}

	// Handler breakdown for last minute stats
	if showHandlerBreakdown && lastMinute != nil {
		data["Active Handlers"] = fmt.Sprintf("%d", len(lastMinute))

		// Find top 3 handlers by request count
		type handlerStats struct {
			name     string
			requests int64
		}
		var handlers []handlerStats
		for name, hstats := range lastMinute {
			if hstats.Requests > 0 {
				handlers = append(handlers, handlerStats{name, hstats.Requests})
			}
		}

		// Sort by request count descending
		sort.Slice(handlers, func(i, j int) bool {
			return handlers[i].requests > handlers[j].requests
		})

		// Show top 3 handlers
		for i, h := range handlers {
			if i >= 3 {
				break
			}
			key := fmt.Sprintf("Top %d Handler", i+1)
			percentage := float64(h.requests) / float64(stats.Requests) * 100
			data[key] = fmt.Sprintf("%s (%s req, %.1f%%)", h.name, humanize.Comma(h.requests), percentage)
		}
	}

	return data
}

// Helper function to generate RPC overview dashboard
func (node *RPCMetricsNode) generateRPCOverviewDashboard() map[string]string {
	data := make(map[string]string)

	// Collection timestamp
	data["Last Updated"] = node.rpc.CollectedAt.Format("2006-01-02 15:04:05")

	// Connection status
	data["Cluster Nodes"] = fmt.Sprintf("%d nodes configured", node.rpc.Nodes)
	data["Connected Nodes"] = fmt.Sprintf("%d online", node.rpc.Connected)
	if node.rpc.Disconnected > 0 {
		data["Disconnected Nodes"] = fmt.Sprintf("%d offline", node.rpc.Disconnected)
	}

	// Connection health assessment
	if node.rpc.Nodes > 0 {
		connectionRate := float64(node.rpc.Connected) / float64(node.rpc.Nodes) * 100
		data["Connection Rate"] = fmt.Sprintf("%.1f%%", connectionRate)

		// Health status
		var healthStatus string
		if connectionRate >= 95.0 {
			healthStatus = "Excellent"
		} else if connectionRate >= 80.0 {
			healthStatus = "Good"
		} else if connectionRate >= 60.0 {
			healthStatus = "Fair"
		} else {
			healthStatus = "Poor"
		}
		data["Health Status"] = healthStatus
		data["Cluster Availability"] = fmt.Sprintf("%.2f%%", connectionRate)
	}

	// Activity summary (last minute)
	if len(node.rpc.LastMinute) > 0 {
		var totalRequests int64
		var totalLatency float64
		var totalBytes int64
		handlerCount := 0

		for _, stats := range node.rpc.LastMinute {
			if stats.Requests > 0 {
				totalRequests += stats.Requests
				totalLatency += stats.RequestTimeSecs
				totalBytes += stats.IncomingBytes + stats.OutgoingBytes
				handlerCount++
			}
		}

		data["Recent Activity"] = fmt.Sprintf("%s requests (last minute)", humanize.Comma(totalRequests))
		data["Active Handlers"] = fmt.Sprintf("%d handlers", handlerCount)

		if totalRequests > 0 {
			if totalLatency > 0 {
				avgLatency := (totalLatency / float64(totalRequests)) * 1000
				data["Avg Response Time"] = fmt.Sprintf("%.2f ms", avgLatency)
			}

			rps := float64(totalRequests) / 60.0 // per second over the minute
			data["Request Rate"] = fmt.Sprintf("%.1f req/s", rps)

			if totalBytes > 0 {
				data["Total Throughput"] = humanize.Bytes(uint64(totalBytes))
				bps := float64(totalBytes) / 60.0 // per second over the minute
				data["Throughput Rate"] = humanize.Bytes(uint64(bps)) + "/s"
			}
		}
	} else {
		data["Recent Activity"] = "No recent RPC activity"
	}

	// Historical data summary (last day)
	if len(node.rpc.LastDay) > 0 {
		var dayTotal int64
		var dayLatency float64

		for _, segmented := range node.rpc.LastDay {
			for _, segment := range segmented.Segments {
				dayTotal += segment.Requests
				dayLatency += segment.RequestTimeSecs
			}
		}

		data["Daily Total"] = fmt.Sprintf("%s requests", humanize.Comma(dayTotal))
		if dayTotal > 0 && dayLatency > 0 {
			avgDayLatency := (dayLatency / float64(dayTotal)) * 1000
			data["Daily Avg Latency"] = fmt.Sprintf("%.2f ms", avgDayLatency)
		}
	}

	return data
}
