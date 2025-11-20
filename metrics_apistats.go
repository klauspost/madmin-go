package madmin

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

//go:generate msgp -unexported -d clearomitted -d "tag json" -d "timezone utc" -d "maps binkeys" -file $GOFILE

// APIStats contains accumulated statistics for the API on a number of nodes.
type APIStats struct {
	Nodes         int        `json:"nodes,omitempty"`         // Number of nodes that have reported data.
	StartTime     *time.Time `json:"startTime,omitempty"`     // Time range this data covers unless merged from sources with different start times..
	EndTime       *time.Time `json:"endTime,omitempty"`       // Time range this data covers unless merged from sources with different end times.
	WallTimeSecs  float64    `json:"wallTimeSecs,omitempty"`  // Wall time this data covers, accumulated from all nodes.
	Requests      int64      `json:"requests,omitempty"`      // Total number of requests.
	IncomingBytes int64      `json:"incomingBytes,omitempty"` // Total number of bytes received.
	OutgoingBytes int64      `json:"outgoingBytes,omitempty"` // Total number of bytes sent.
	Errors4xx     int        `json:"errors_4xx,omitempty"`    // Total number of 4xx (client request) errors.
	Errors5xx     int        `json:"errors_5xx,omitempty"`    // Total number of 5xx (serverside) errors.
	Canceled      int64      `json:"canceled,omitempty"`      // Requests that were canceled before they finished processing.

	// Request times
	RequestTimeSecs  float64 `json:"requestTimeSecs,omitempty"` // Total request time.
	ReqReadSecs      float64 `json:"reqReadSecs,omitempty"`     // Total time spent on request reads in seconds.
	RespSecs         float64 `json:"respSecs,omitempty"`        // Total time spent on responses in seconds.
	RespTTFBSecs     float64 `json:"respTtfbSecs,omitempty"`    // Total time spent on TTFB (req read -> response first byte) in seconds.
	ReadBlockedSecs  float64 `json:"readBlocked,omitempty"`     // Time spent waiting for reads from client.
	WriteBlockedSecs float64 `json:"writeBlocked,omitempty"`    // Time spent waiting for writes to client.

	// Request times min/max
	RequestTimeSecsMin float64 `json:"requestTimeSecsMin,omitempty"` // Min request time.
	RequestTimeSecsMax float64 `json:"requestTimeSecsMax,omitempty"` // Max request time.
	ReqReadSecsMin     float64 `json:"reqReadSecsMin,omitempty"`     // Min time spent on request reads in seconds.
	ReqReadSecsMax     float64 `json:"reqReadSecsMax,omitempty"`     // Max time spent on request reads in seconds.
	RespSecsMin        float64 `json:"respSecsMin,omitempty"`        // Min time spent on responses in seconds.
	RespSecsMax        float64 `json:"respSecsMax,omitempty"`        // Max time spent on responses in seconds.
	RespTTFBSecsMin    float64 `json:"respTtfbSecsMin,omitempty"`    // Min time spent on TTFB (req read -> response first byte) in seconds.
	RespTTFBSecsMax    float64 `json:"respTtfbSecsMax,omitempty"`    // Max time spent on TTFB (req read -> response first byte) in seconds.

	Rejected RejectedAPIStats `json:"rejected,omitempty"`
}

// RejectedAPIStats contains statistics for rejected requests.
type RejectedAPIStats struct {
	Auth           int64 `json:"auth,omitempty"`           // Total number of rejected authentication requests.
	RequestsTime   int64 `json:"requestsTime,omitempty"`   // Requests that were rejected due to outdated request signature.
	Header         int64 `json:"header,omitempty"`         // Requests that were rejected due to header signature.
	Invalid        int64 `json:"invalid,omitempty"`        // Requests that were rejected due to invalid request signature.
	NotImplemented int64 `json:"notImplemented,omitempty"` // Requests that were rejected due to not implemented API.
}

// Helper functions for min/max operations
func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// Add 'other' to a.
func (a *APIStats) Add(other *APIStats) {
	if other == nil {
		return
	}
	a.Merge(*other)
}

// Merge other into 'a'.
func (a *APIStats) Merge(other APIStats) {
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

	a.Nodes += other.Nodes
	a.WallTimeSecs += other.WallTimeSecs
	a.Requests += other.Requests
	a.IncomingBytes += other.IncomingBytes
	a.OutgoingBytes += other.OutgoingBytes
	a.RequestTimeSecs += other.RequestTimeSecs
	a.ReqReadSecs += other.ReqReadSecs
	a.RespSecs += other.RespSecs
	a.RespTTFBSecs += other.RespTTFBSecs
	a.Errors4xx += other.Errors4xx
	a.Errors5xx += other.Errors5xx
	a.Canceled += other.Canceled
	a.Rejected.Auth += other.Rejected.Auth
	a.Rejected.RequestsTime += other.Rejected.RequestsTime
	a.Rejected.Header += other.Rejected.Header
	a.Rejected.Invalid += other.Rejected.Invalid
	a.Rejected.NotImplemented += other.Rejected.NotImplemented
	a.ReadBlockedSecs += other.ReadBlockedSecs
	a.WriteBlockedSecs += other.WriteBlockedSecs

	if a.Requests == 0 && other.Requests == 0 {
		return
	}

	// Find 2 to min/max. If we have 1, just use that twice
	at := *a
	bt := other
	if a.Requests == other.Requests {
		at = bt
	}
	if other.Requests == 0 {
		bt = at
	}
	a.RequestTimeSecsMin = minFloat64(at.RequestTimeSecsMin, bt.RequestTimeSecsMin)
	a.RequestTimeSecsMax = maxFloat64(at.RequestTimeSecsMax, bt.RequestTimeSecsMax)
	a.ReqReadSecsMin = minFloat64(at.ReqReadSecsMin, bt.ReqReadSecsMin)
	a.ReqReadSecsMax = maxFloat64(at.ReqReadSecsMax, bt.ReqReadSecsMax)
	a.RespSecsMin = minFloat64(at.RespSecsMin, bt.RespSecsMin)
	a.RespSecsMax = maxFloat64(at.RespSecsMax, bt.RespSecsMax)
	a.RespTTFBSecsMin = minFloat64(at.RespTTFBSecsMin, bt.RespTTFBSecsMin)
	a.RespTTFBSecsMax = maxFloat64(at.RespTTFBSecsMax, bt.RespTTFBSecsMax)
}

// SegmentedAPIMetrics are segmented API metrics.
type SegmentedAPIMetrics = Segmented[APIStats, *APIStats]

// APIMetrics contains metrics for API operations.
type APIMetrics struct {
	// Time these metrics were collected
	CollectedAt time.Time `json:"collected"`

	// Nodes responded to the request.
	Nodes int `json:"nodes"`

	// Number of active requests.
	ActiveRequests int64 `json:"activeRequests,omitempty"`

	// Number of queued requests.
	QueuedRequests int64 `json:"queuedRequests,omitempty"`

	// Last minute operation statistics by API.
	LastMinuteAPI map[string]APIStats `json:"lastMinuteApi,omitempty"`

	// Last day operation statistics by API, segmented.
	LastDayAPI map[string]SegmentedAPIMetrics `json:"lastDayApi,omitempty"`

	// SinceStart contains operation statistics since server(s) started.
	SinceStart APIStats `json:"since_start"`
}

func (a *APIMetrics) Merge(b *APIMetrics) {
	if b == nil {
		return
	}
	if a.CollectedAt.Before(b.CollectedAt) {
		a.CollectedAt = b.CollectedAt
	}
	a.Nodes += b.Nodes
	a.ActiveRequests += b.ActiveRequests
	a.QueuedRequests += b.QueuedRequests

	for k, v := range b.LastMinuteAPI {
		if a.LastMinuteAPI == nil {
			a.LastMinuteAPI = make(map[string]APIStats, len(b.LastMinuteAPI))
		}
		existing := a.LastMinuteAPI[k]
		existing.Merge(v)
		a.LastMinuteAPI[k] = existing
	}
	for k, v := range b.LastDayAPI {
		if a.LastDayAPI == nil {
			a.LastDayAPI = make(map[string]SegmentedAPIMetrics, len(b.LastDayAPI))
		}
		existing, ok := a.LastDayAPI[k]
		if !ok {
			// Deep copy to avoid sharing slice references
			vCopy := v
			if len(v.Segments) > 0 {
				vCopy.Segments = append([]APIStats{}, v.Segments...)
			}
			a.LastDayAPI[k] = vCopy
			continue
		}
		existing.Add(&v)
		a.LastDayAPI[k] = existing
	}
	a.SinceStart.Merge(b.SinceStart)
}

// LastMinuteTotal returns the total APIStats for the last minute.
func (a APIMetrics) LastMinuteTotal() APIStats {
	var res APIStats
	for _, stats := range a.LastMinuteAPI {
		res.Merge(stats)
	}
	// Since we are merging across APIs must reset track node count.
	res.Nodes = a.Nodes
	return res
}

// LastDayTotalSegmented returns the total SegmentedAPIMetrics for the last day.
// There will be no node-count for values.
func (a APIMetrics) LastDayTotalSegmented() SegmentedAPIMetrics {
	var res SegmentedAPIMetrics
	for _, stats := range a.LastDayAPI {
		res.Add(&stats)
	}
	// Since we are merging across APIs must reset track node count.
	for i := range res.Segments {
		res.Segments[i].Nodes = a.Nodes
	}
	return res
}

// LastDayTotal returns the accumulated APIStats for the last day.
func (a APIMetrics) LastDayTotal() APIStats {
	var res APIStats
	for _, stats := range a.LastDayAPI {
		for _, s := range stats.Segments {
			res.Merge(s)
		}
	}
	// Since we are merging across APIs must reset track node count.
	res.Nodes = a.Nodes

	return res
}

// === ENHANCED API METRICS FORMATTING ===

// String returns a human-readable representation of APIStats
func (a APIStats) String() string {
	if a.Requests == 0 {
		return "No API requests recorded"
	}

	var parts []string

	// Request summary
	parts = append(parts, fmt.Sprintf("Requests: %s", humanize.Comma(a.Requests)))

	// Timing information
	if a.Requests > 0 {
		avgLatency := (a.RequestTimeSecs / float64(a.Requests)) * 1000
		parts = append(parts, fmt.Sprintf("Avg Latency: %.2fms", avgLatency))

		if a.RequestTimeSecsMin > 0 && a.RequestTimeSecsMax > 0 {
			parts = append(parts, fmt.Sprintf("Latency Range: %.1f-%.1fms",
				a.RequestTimeSecsMin*1000, a.RequestTimeSecsMax*1000))
		}
	}

	// Throughput
	totalBytes := a.IncomingBytes + a.OutgoingBytes
	if totalBytes > 0 {
		parts = append(parts, fmt.Sprintf("Throughput: %s", humanize.Bytes(uint64(totalBytes))))
		if a.Requests > 0 {
			avgBytesPerReq := totalBytes / a.Requests
			parts = append(parts, fmt.Sprintf("Avg/Request: %s", humanize.Bytes(uint64(avgBytesPerReq))))
		}
	}

	// Error rates
	totalErrors := a.Errors4xx + a.Errors5xx
	if totalErrors > 0 {
		errorRate := float64(totalErrors) / float64(a.Requests) * 100
		parts = append(parts, fmt.Sprintf("Error Rate: %.2f%% (%d)", errorRate, totalErrors))
	}

	// Rejections
	totalRejected := a.Rejected.Auth + a.Rejected.Header + a.Rejected.Invalid +
		a.Rejected.NotImplemented + a.Rejected.RequestsTime
	if totalRejected > 0 {
		rejectionRate := float64(totalRejected) / float64(a.Requests) * 100
		parts = append(parts, fmt.Sprintf("Rejection Rate: %.2f%% (%d)", rejectionRate, totalRejected))
	}

	if a.Nodes > 0 {
		parts = append(parts, fmt.Sprintf("Nodes: %d", a.Nodes))
	}

	return strings.Join(parts, ", ")
}

// === ENHANCED API METRICS FORMATTING ===

// String returns a human-readable representation of APIMetrics
func (a APIMetrics) String() string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Collected: %s", a.CollectedAt.Format("15:04:05")))
	parts = append(parts, fmt.Sprintf("Nodes: %d", a.Nodes))

	// Queue status
	totalQueue := a.ActiveRequests + a.QueuedRequests
	if totalQueue > 0 {
		parts = append(parts, fmt.Sprintf("Queue: %s active, %s queued",
			humanize.Comma(a.ActiveRequests), humanize.Comma(a.QueuedRequests)))
	}

	// Last minute summary
	lastMinute := a.LastMinuteTotal()
	if lastMinute.Requests > 0 {
		avgLatency := (lastMinute.RequestTimeSecs / float64(lastMinute.Requests)) * 1000
		parts = append(parts, fmt.Sprintf("Last Minute: %s req (%.1fms avg)",
			humanize.Comma(lastMinute.Requests), avgLatency))
	}

	// Endpoints
	if len(a.LastMinuteAPI) > 0 {
		parts = append(parts, fmt.Sprintf("Active Endpoints: %d", len(a.LastMinuteAPI)))
	}

	return strings.Join(parts, " | ")
}

// GetDashboard returns a comprehensive executive dashboard for API metrics
func (a APIMetrics) GetDashboard() map[string]string {
	data := make(map[string]string)

	lastMinute := a.LastMinuteTotal()
	data["Active Nodes"] = fmt.Sprintf("%d nodes responding", a.Nodes)
	data["Collection Time"] = a.CollectedAt.Format("15:04:05")

	// Request queue status
	totalQueue := a.ActiveRequests + a.QueuedRequests
	if totalQueue > 0 {
		data["Request Queue Status"] = fmt.Sprintf("%s active, %s queued",
			humanize.Comma(a.ActiveRequests), humanize.Comma(a.QueuedRequests))
	} else {
		data["Request Queue Status"] = "No queued requests"
	}

	// Last minute performance
	if lastMinute.Requests > 0 {
		avgLatency := (lastMinute.RequestTimeSecs / float64(lastMinute.Requests)) * 1000
		data["Requests"] = fmt.Sprintf("%s req/min", humanize.Comma(lastMinute.Requests))
		data["Average Latency"] = fmt.Sprintf("%.1f ms", avgLatency)

		// Timing range analysis
		if lastMinute.RequestTimeSecsMax > 0 {
			data["Latency Range"] = fmt.Sprintf("%.1f - %.1f ms",
				lastMinute.RespTTFBSecsMin*1000, lastMinute.RespTTFBSecsMax*1000)
		}
		if lastMinute.RespTTFBSecsMax > 0 {
			data["TTFB Range"] = fmt.Sprintf("%.1f - %.1f ms",
				lastMinute.RespTTFBSecsMin*1000, lastMinute.RespTTFBSecsMax*1000)
		}
		if lastMinute.ReqReadSecsMax > 0 {
			data["Req Read Range"] = fmt.Sprintf("%.1f - %.1f ms",
				lastMinute.ReqReadSecsMin*1000, lastMinute.ReqReadSecsMax*1000)
		}
		if lastMinute.RespSecsMax > 0 {
			data["Resp Wr Range"] = fmt.Sprintf("%.1f - %.1f ms",
				lastMinute.RespSecsMin*1000, lastMinute.RespSecsMax*1000)
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
		data["Throughput"] = fmt.Sprintf("%s/min", humanize.Bytes(uint64(totalBytes)))
		data["↳ Incoming"] = fmt.Sprintf("%s/min", humanize.Bytes(uint64(lastMinute.IncomingBytes)))
		data["↳ Outgoing"] = fmt.Sprintf("%s/min", humanize.Bytes(uint64(lastMinute.OutgoingBytes)))
	}

	totalErrors := lastMinute.Errors4xx + lastMinute.Errors5xx
	if totalErrors > 0 || lastMinute.Requests > 0 {
		var errorRate float64
		if lastMinute.Requests > 0 {
			errorRate = float64(totalErrors) / float64(lastMinute.Requests) * 100
		}
		data["Error Rate (Last Minute)"] = fmt.Sprintf("%.2f%% (%d errors)", errorRate, totalErrors)

		if lastMinute.Errors4xx > 0 {
			data["↳ 4xx Client Errors"] = fmt.Sprintf("%d", lastMinute.Errors4xx)
		}
		if lastMinute.Errors5xx > 0 {
			data["↳ 5xx Server Errors"] = fmt.Sprintf("%d", lastMinute.Errors5xx)
		}
		if lastMinute.Canceled > 0 {
			data["↳ Canceled Requests"] = fmt.Sprintf("%d", lastMinute.Canceled)
		}
	} else {
		data["Error Rate (Last Minute)"] = "No errors detected"
	}

	// Rejection analysis
	rejections := lastMinute.Rejected
	totalRejected := rejections.Auth + rejections.Header + rejections.Invalid +
		rejections.NotImplemented + rejections.RequestsTime
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

	since := a.SinceStart
	if since.Requests > 0 {
		data["Total Requests"] = humanize.Comma(since.Requests)
		data["Total Data Processed"] = humanize.Bytes(uint64(since.IncomingBytes + since.OutgoingBytes))

		lifetimeErrors := since.Errors4xx + since.Errors5xx
		lifetimeErrorRate := float64(lifetimeErrors) / float64(since.Requests) * 100
		data["Lifetime Error Rate"] = fmt.Sprintf("%.3f%%", lifetimeErrorRate)

		if since.WallTimeSecs > 0 {
			avgRPS := float64(since.Requests) / since.WallTimeSecs
			data["Average RPS"] = fmt.Sprintf("%.1f req/sec", avgRPS)
		}
	}

	// === ENDPOINT ANALYSIS ===
	endpointCount := len(a.LastMinuteAPI)
	if endpointCount > 0 {
		data["    "] = ""

		data["Active Endpoints"] = fmt.Sprintf("%d endpoints receiving traffic", endpointCount)

		// Find top endpoints by request count
		type endpointStat struct {
			name  string
			stats APIStats
		}

		var endpoints []endpointStat
		for name, stats := range a.LastMinuteAPI {
			endpoints = append(endpoints, endpointStat{name, stats})
		}

		// Simple bubble sort by request count
		for i := 0; i < len(endpoints)-1; i++ {
			for j := i + 1; j < len(endpoints); j++ {
				if endpoints[i].stats.Requests < endpoints[j].stats.Requests {
					endpoints[i], endpoints[j] = endpoints[j], endpoints[i]
				}
			}
		}

		// Show top 5 busiest endpoints
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
	}

	return data
}

type APIMetricsNode struct {
	api    *APIMetrics
	parent MetricNode
	path   string
}

func (n *APIMetricsNode) ShouldPauseRefresh() bool {
	return false
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
func (node *APIMetricsNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APIMetricsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *APIMetricsNode) GetParent() MetricNode           { return node.parent }
func (node *APIMetricsNode) GetPath() string                 { return node.path }
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

func (node *APILastMinuteNode) ShouldPauseRefresh() bool {
	return false
}

func (node *APILastMinuteNode) GetChildren() []MetricChild {
	if node.api.LastMinuteAPI == nil || len(node.api.LastMinuteAPI) == 0 {
		return []MetricChild{}
	}

	// Get sorted endpoint names to ensure consistent ordering
	var endpoints []string
	for endpoint := range node.api.LastMinuteAPI {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)

	var children []MetricChild
	for _, endpoint := range endpoints {
		if node.api.LastMinuteAPI[endpoint].Requests == 0 {
			continue
		}
		lastMinute := node.api.LastMinuteAPI[endpoint]
		avgLatency := float64(0)
		if lastMinute.Requests > 0 {
			avgLatency = (lastMinute.RequestTimeSecs / float64(lastMinute.Requests)) * 1000
		}

		children = append(children, MetricChild{
			Name: endpoint,
			Description: fmt.Sprintf("Last Minute: %s req (%.1fms avg)",
				humanize.Comma(lastMinute.Requests), avgLatency),
		})
	}
	return children
}

// generateAPIStatsDisplay creates a consistent API statistics display
func generateAPIStatsDisplay(stats APIStats, endpointsCount int, showTopEndpoints bool, endpoints map[string]APIStats) map[string]string {
	if stats.Requests == 0 {
		data := make(map[string]string)
		data["Status"] = "No API requests recorded"
		return data
	}

	// Use ordered slice to maintain consistent display order
	var entries []struct{ key, value string }

	// === BASIC METRICS ===
	entries = append(entries, struct{ key, value string }{"Total Requests", humanize.Comma(stats.Requests)})
	if endpointsCount > 0 {
		entries = append(entries, struct{ key, value string }{"Active Endpoints", fmt.Sprintf("%d endpoints", endpointsCount)})
	}
	entries = append(entries, struct{ key, value string }{"Responding Nodes", fmt.Sprintf("%d nodes", stats.Nodes)})

	// Calculate RPS if we have wall time
	if stats.WallTimeSecs > 0 {
		rps := float64(stats.Requests) / stats.WallTimeSecs
		entries = append(entries, struct{ key, value string }{"Avg RPS", fmt.Sprintf("%.1f req/sec", rps)})
	}

	// === TIMING METRICS ===
	avgLatency := (stats.RequestTimeSecs / float64(stats.Requests)) * 1000
	entries = append(entries, struct{ key, value string }{"Avg Latency", fmt.Sprintf("%.1f ms", avgLatency)})

	if stats.RespTTFBSecs > 0 {
		avgTTFB := (stats.RespTTFBSecs / float64(stats.Requests)) * 1000
		entries = append(entries, struct{ key, value string }{"Avg TTFB", fmt.Sprintf("%.1f ms", avgTTFB)})
	}

	// === TIMING RANGES ===
	if stats.RequestTimeSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"Latency Range", fmt.Sprintf("%.1f - %.1f ms",
			stats.RequestTimeSecsMin*1000, stats.RequestTimeSecsMax*1000)})
	}
	if stats.RespTTFBSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"TTFB Range", fmt.Sprintf("%.1f - %.1f ms",
			stats.RespTTFBSecsMin*1000, stats.RespTTFBSecsMax*1000)})
	}
	if stats.ReqReadSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"Req Read Range", fmt.Sprintf("%.1f - %.1f ms",
			stats.ReqReadSecsMin*1000, stats.ReqReadSecsMax*1000)})
	}
	if stats.RespSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"Resp Wr Range", fmt.Sprintf("%.1f - %.1f ms",
			stats.RespSecsMin*1000, stats.RespSecsMax*1000)})
	}

	// === THROUGHPUT ===
	totalBytes := stats.IncomingBytes + stats.OutgoingBytes
	if totalBytes > 0 {
		entries = append(entries, struct{ key, value string }{"Total Throughput", humanize.Bytes(uint64(totalBytes))})
		entries = append(entries, struct{ key, value string }{"-> Incoming", humanize.Bytes(uint64(stats.IncomingBytes))})
		entries = append(entries, struct{ key, value string }{"<- Outgoing", humanize.Bytes(uint64(stats.OutgoingBytes))})

		avgBytesPerReq := totalBytes / stats.Requests
		entries = append(entries, struct{ key, value string }{"Avg Bytes", humanize.Bytes(uint64(avgBytesPerReq)) + "/req"})
	}

	// === ERROR ANALYSIS ===
	totalErrors := stats.Errors4xx + stats.Errors5xx
	if stats.Requests > 0 {
		if stats.Requests > 0 {
			errorRate := float64(totalErrors) / float64(stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"Error Rate", fmt.Sprintf("%.2f%% (%d errors)", errorRate, totalErrors)})
		}
		if totalErrors > 0 {
			if stats.Errors4xx > 0 {
				clientErrorRate := float64(stats.Errors4xx) / float64(stats.Requests) * 100
				entries = append(entries, struct{ key, value string }{"↳ 4xx Client Errors", fmt.Sprintf("%d (%.2f%%)",
					stats.Errors4xx, clientErrorRate)})
			}
			if stats.Errors5xx > 0 {
				serverErrorRate := float64(stats.Errors5xx) / float64(stats.Requests) * 100
				entries = append(entries, struct{ key, value string }{"↳ 5xx Server Errors", fmt.Sprintf("%d (%.2f%%)",
					stats.Errors5xx, serverErrorRate)})
			}
			if stats.Canceled > 0 {
				cancelRate := float64(stats.Canceled) / float64(stats.Requests) * 100
				entries = append(entries, struct{ key, value string }{"↳ Canceled Requests", fmt.Sprintf("%d (%.2f%%)", stats.Canceled, cancelRate)})
			}
		}
	}

	// === REJECTIONS ===
	totalRejected := stats.Rejected.Auth + stats.Rejected.Header +
		stats.Rejected.Invalid + stats.Rejected.NotImplemented +
		stats.Rejected.RequestsTime
	if totalRejected > 0 {
		rejectionRate := float64(totalRejected) / float64(stats.Requests) * 100
		entries = append(entries, struct{ key, value string }{"Rejected Requests", fmt.Sprintf("%d rejections (%.2f%%)", totalRejected, rejectionRate)})

		if stats.Rejected.Auth > 0 {
			authRate := float64(stats.Rejected.Auth) / float64(stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Authentication", fmt.Sprintf("%d (%.2f%%)", stats.Rejected.Auth, authRate)})
		}
		if stats.Rejected.Header > 0 {
			headerRate := float64(stats.Rejected.Header) / float64(stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Header Issues", fmt.Sprintf("%d (%.2f%%)", stats.Rejected.Header, headerRate)})
		}
		if stats.Rejected.Invalid > 0 {
			invalidRate := float64(stats.Rejected.Invalid) / float64(stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Invalid Requests", fmt.Sprintf("%d (%.2f%%)", stats.Rejected.Invalid, invalidRate)})
		}
		if stats.Rejected.NotImplemented > 0 {
			notImplRate := float64(stats.Rejected.NotImplemented) / float64(stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Not Implemented", fmt.Sprintf("%d (%.2f%%)", stats.Rejected.NotImplemented, notImplRate)})
		}
		if stats.Rejected.RequestsTime > 0 {
			timeRate := float64(stats.Rejected.RequestsTime) / float64(stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Outdated Signatures", fmt.Sprintf("%d (%.2f%%)", stats.Rejected.RequestsTime, timeRate)})
		}
	}

	// === BLOCKING ANALYSIS ===
	if stats.Requests > 0 && (stats.ReadBlockedSecs > 0 || stats.WriteBlockedSecs > 0) {
		if stats.ReadBlockedSecs > 0 {
			avgReadBlocked := (stats.ReadBlockedSecs / float64(stats.Requests)) * 1000
			entries = append(entries, struct{ key, value string }{"Avg Read Blocking", fmt.Sprintf("%.1f ms/req", avgReadBlocked)})
		}
		if stats.WriteBlockedSecs > 0 {
			avgWriteBlocked := (stats.WriteBlockedSecs / float64(stats.Requests)) * 1000
			entries = append(entries, struct{ key, value string }{"Avg Write Blocking", fmt.Sprintf("%.1f ms/req", avgWriteBlocked)})
		}
	}

	// === TOP ENDPOINTS (only for last_minute) ===
	if showTopEndpoints && endpoints != nil {
		type endpointStat struct {
			name  string
			stats APIStats
		}

		var endpointList []endpointStat
		for name, stat := range endpoints {
			endpointList = append(endpointList, endpointStat{name, stat})
		}

		// Sort by request count
		for i := 0; i < len(endpointList)-1; i++ {
			for j := i + 1; j < len(endpointList); j++ {
				if endpointList[i].stats.Requests < endpointList[j].stats.Requests {
					endpointList[i], endpointList[j] = endpointList[j], endpointList[i]
				}
			}
		}

		maxShow := 5
		if len(endpointList) < maxShow {
			maxShow = len(endpointList)
		}

		if maxShow > 0 {
			entries = append(entries, struct{ key, value string }{"Top Endpoints", fmt.Sprintf("Showing %d busiest endpoints", maxShow)})
			for i := 0; i < maxShow; i++ {
				ep := endpointList[i]
				if ep.stats.Requests > 0 {
					avgLatency := (ep.stats.RequestTimeSecs / float64(ep.stats.Requests)) * 1000
					errors := ep.stats.Errors4xx + ep.stats.Errors5xx
					entries = append(entries, struct{ key, value string }{fmt.Sprintf("↳ %s", ep.name), fmt.Sprintf("%s req, %.1fms avg, %d err",
						humanize.Comma(ep.stats.Requests), avgLatency, errors)})
				}
			}
		}
	}

	// Convert ordered entries to map with numbered prefixes to preserve order
	data := make(map[string]string)
	for i, entry := range entries {
		key := fmt.Sprintf("%02d:%s", i, entry.key)
		data[key] = entry.value
	}

	return data
}

func (node *APILastMinuteNode) GetLeafData() map[string]string {
	total := node.api.LastMinuteTotal()
	return generateAPIStatsDisplay(total, len(node.api.LastMinuteAPI), true, node.api.LastMinuteAPI)
}

func (node *APILastMinuteNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APILastMinuteNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *APILastMinuteNode) GetParent() MetricNode           { return node.parent }
func (node *APILastMinuteNode) GetPath() string                 { return node.path }
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

func (node *APILastDayNode) ShouldPauseRefresh() bool {
	return true
}

func (node *APILastDayNode) GetChildren() []MetricChild {
	if len(node.api.LastDayAPI) == 0 {
		return []MetricChild{}
	}

	var children []MetricChild

	// Add "All" entry first - shows aggregated time segments
	children = append(children, MetricChild{
		Name:        "All",
		Description: "Aggregated statistics for all API endpoints",
	})

	// Add individual API endpoints, sorted alphabetically
	var apiNames []string
	for apiName := range node.api.LastDayAPI {
		apiNames = append(apiNames, apiName)
	}
	sort.Strings(apiNames)

	for _, apiName := range apiNames {
		segmented := node.api.LastDayAPI[apiName]
		totalRequests := int64(0)
		for _, segment := range segmented.Segments {
			totalRequests += segment.Requests
		}

		children = append(children, MetricChild{
			Name:        apiName,
			Description: fmt.Sprintf("Last day statistics for %s (%d total requests)", apiName, totalRequests),
		})
	}

	return children
}

func (node *APILastDayNode) GetLeafData() map[string]string {
	total := node.api.LastDayTotal()
	return generateAPIStatsDisplay(total, len(node.api.LastDayAPI), false, nil)
}

func (node *APILastDayNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APILastDayNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *APILastDayNode) GetParent() MetricNode           { return node.parent }
func (node *APILastDayNode) GetPath() string                 { return node.path }
func (node *APILastDayNode) RequiredMetricTypes() MetricType { return MetricsAPI }

func (node *APILastDayNode) GetChild(name string) (MetricNode, error) {
	// Handle "All" entry - shows aggregated time segments
	if name == "All" {
		return &APILastDayAllNode{
			api:    node.api,
			parent: node,
			path:   node.path + "/" + name,
		}, nil
	}

	// Handle individual API endpoints
	if segmented, exists := node.api.LastDayAPI[name]; exists {
		return &APILastDayEndpointNode{
			api:       node.api,
			apiName:   name,
			segmented: segmented,
			parent:    node,
			path:      node.path + "/" + name,
		}, nil
	}

	return nil, fmt.Errorf("API endpoint not found: %s", name)
}

// APILastDayAllNode shows aggregated time segments for all API endpoints
type APILastDayAllNode struct {
	api    *APIMetrics
	parent MetricNode
	path   string
}

func (node *APILastDayAllNode) ShouldPauseRefresh() bool {
	return true
}

func (node *APILastDayAllNode) GetChildren() []MetricChild {
	segmented := node.api.LastDayTotalSegmented()
	if len(segmented.Segments) == 0 {
		return []MetricChild{}
	}

	var children []MetricChild

	// Add "Total" entry first
	children = append(children, MetricChild{
		Name:        "Total",
		Description: "Last day total statistics across all time segments",
	})

	// Add time segments, most recent first (filter out empty segments)
	for i := len(segmented.Segments) - 1; i >= 0; i-- {
		segmentTime := segmented.FirstTime.Add(time.Duration(i*segmented.Interval) * time.Second)
		endTime := segmentTime.Add(time.Duration(segmented.Interval) * time.Second)
		segmentName := segmentTime.UTC().Format("15:04Z")

		// Get request count for this segment
		requests := int64(0)
		if i < len(segmented.Segments) {
			requests = segmented.Segments[i].Requests
		}

		// Filter out time segments with no requests
		if requests == 0 {
			continue
		}

		children = append(children, MetricChild{
			Name: segmentName,
			Description: fmt.Sprintf("API %s -> %s (%d requests)",
				segmentTime.Local().Format("15:04"),
				endTime.Local().Format("15:04"),
				requests),
		})
	}

	return children
}

func (node *APILastDayAllNode) GetChild(name string) (MetricNode, error) {
	segmented := node.api.LastDayTotalSegmented()
	if len(segmented.Segments) == 0 {
		return nil, fmt.Errorf("no last day segmented data available")
	}

	// Handle "Total" entry
	if name == "Total" {
		return &APILastDayTotalNode{
			api:    node.api,
			parent: node,
			path:   node.path + "/" + name,
		}, nil
	}

	// Handle time segments - find by time format (with UTC indicator)
	for i := len(segmented.Segments) - 1; i >= 0; i-- {
		segmentTime := segmented.FirstTime.Add(time.Duration(i*segmented.Interval) * time.Second)
		if segmentTime.UTC().Format("15:04Z") == name {
			return &APITimeSegmentAllNode{
				segment:     segmented.Segments[i],
				segmentTime: segmentTime,
				parent:      node,
				path:        node.path + "/" + name,
			}, nil
		}
	}

	return nil, fmt.Errorf("time segment not found: %s", name)
}

func (node *APILastDayAllNode) GetLeafData() map[string]string {
	total := node.api.LastDayTotal()
	return generateAPIStatsDisplay(total, len(node.api.LastDayAPI), false, nil)
}

func (node *APILastDayAllNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APILastDayAllNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *APILastDayAllNode) GetParent() MetricNode           { return node.parent }
func (node *APILastDayAllNode) GetPath() string                 { return node.path }
func (node *APILastDayAllNode) RequiredMetricTypes() MetricType { return MetricsAPI }

// APILastDayEndpointNode shows time segments for a specific API endpoint
type APILastDayEndpointNode struct {
	api       *APIMetrics
	apiName   string
	segmented SegmentedAPIMetrics
	parent    MetricNode
	path      string
}

func (node *APILastDayEndpointNode) GetChildren() []MetricChild {
	if len(node.segmented.Segments) == 0 {
		return []MetricChild{}
	}

	var children []MetricChild

	// Add "Total" entry first
	children = append(children, MetricChild{
		Name:        "Total",
		Description: fmt.Sprintf("Total statistics for %s across all time segments", node.apiName),
	})

	// Add time segments, most recent first (filter out empty segments)
	for i := len(node.segmented.Segments) - 1; i >= 0; i-- {
		segmentTime := node.segmented.FirstTime.Add(time.Duration(i*node.segmented.Interval) * time.Second)
		endTime := segmentTime.Add(time.Duration(node.segmented.Interval) * time.Second)
		segmentName := segmentTime.UTC().Format("15:04Z")

		// Get request count for this segment
		requests := int64(0)
		if i < len(node.segmented.Segments) {
			requests = node.segmented.Segments[i].Requests
		}

		// Filter out time segments with no requests
		if requests == 0 {
			continue
		}

		children = append(children, MetricChild{
			Name: segmentName,
			Description: fmt.Sprintf("%s %s -> %s (%d requests)",
				node.apiName,
				segmentTime.Local().Format("15:04"),
				endTime.Local().Format("15:04"),
				requests),
		})
	}

	return children
}

func (node *APILastDayEndpointNode) GetChild(name string) (MetricNode, error) {
	if len(node.segmented.Segments) == 0 {
		return nil, fmt.Errorf("no segmented data available for API %s", node.apiName)
	}

	// Handle "Total" entry
	if name == "Total" {
		// Calculate total stats for this endpoint
		total := APIStats{}
		for _, segment := range node.segmented.Segments {
			total.Merge(segment)
		}

		return &APIEndpointNode{
			endpoint: node.apiName,
			stats:    total,
			parent:   node,
			path:     node.path + "/" + name,
		}, nil
	}

	// Handle time segments - find by time format (with UTC indicator)
	for i := len(node.segmented.Segments) - 1; i >= 0; i-- {
		segmentTime := node.segmented.FirstTime.Add(time.Duration(i*node.segmented.Interval) * time.Second)
		if segmentTime.UTC().Format("15:04Z") == name {
			return &APIEndpointNode{
				endpoint: node.apiName,
				stats:    node.segmented.Segments[i],
				parent:   node,
				path:     node.path + "/" + name,
			}, nil
		}
	}

	return nil, fmt.Errorf("time segment not found: %s", name)
}

func (node *APILastDayEndpointNode) GetLeafData() map[string]string {
	// Calculate total stats for this endpoint
	total := APIStats{}
	for _, segment := range node.segmented.Segments {
		total.Merge(segment)
	}
	return generateAPIStatsDisplay(total, 1, false, nil)
}

func (node *APILastDayEndpointNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APILastDayEndpointNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *APILastDayEndpointNode) GetParent() MetricNode           { return node.parent }
func (node *APILastDayEndpointNode) GetPath() string                 { return node.path }
func (node *APILastDayEndpointNode) RequiredMetricTypes() MetricType { return MetricsAPI }
func (node *APILastDayEndpointNode) ShouldPauseRefresh() bool {
	return true
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
	return generateAPIStatsDisplay(node.api.SinceStart, 0, false, nil)
}

func (node *APISinceStartNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APISinceStartNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *APISinceStartNode) GetParent() MetricNode           { return node.parent }
func (node *APISinceStartNode) GetPath() string                 { return node.path }
func (node *APISinceStartNode) RequiredMetricTypes() MetricType { return MetricsAPI }

func (node *APISinceStartNode) ShouldPauseRefresh() bool {
	return false
}
func (node *APISinceStartNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for since_start node")
}

// APILastDayTotalNode shows the total last day statistics
type APILastDayTotalNode struct {
	api    *APIMetrics
	parent MetricNode
	path   string
}

func (node *APILastDayTotalNode) GetChildren() []MetricChild {
	return []MetricChild{}
}

func (node *APILastDayTotalNode) GetLeafData() map[string]string {
	total := node.api.LastDayTotal()
	return generateAPIStatsDisplay(total, len(node.api.LastDayAPI), false, nil)
}

func (node *APILastDayTotalNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APILastDayTotalNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *APILastDayTotalNode) GetParent() MetricNode           { return node.parent }
func (node *APILastDayTotalNode) GetPath() string                 { return node.path }
func (node *APILastDayTotalNode) RequiredMetricTypes() MetricType { return MetricsAPI }

func (node *APILastDayTotalNode) ShouldPauseRefresh() bool {
	return true
}
func (node *APILastDayTotalNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for last day total node")
}

// APITimeSegmentNode shows statistics for a specific time segment
// APITimeSegmentAllNode shows aggregated API statistics for a specific time segment
type APITimeSegmentAllNode struct {
	segment     APIStats
	segmentTime time.Time
	parent      MetricNode
	path        string
}

func (node *APITimeSegmentAllNode) GetChildren() []MetricChild {
	return []MetricChild{}
}

func (node *APITimeSegmentAllNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("no children available for time segment")
}

func (node *APITimeSegmentAllNode) GetLeafData() map[string]string {
	return generateAPIStatsDisplay(node.segment, 1, false, nil)
}

func (node *APITimeSegmentAllNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APITimeSegmentAllNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *APITimeSegmentAllNode) GetParent() MetricNode           { return node.parent }
func (node *APITimeSegmentAllNode) GetPath() string                 { return node.path }
func (node *APITimeSegmentAllNode) RequiredMetricTypes() MetricType { return MetricsAPI }

func (node *APITimeSegmentAllNode) ShouldPauseRefresh() bool {
	return true
}

type APITimeSegmentNode struct {
	segment     APIStats
	segmentTime time.Time
	parent      MetricNode
	path        string
}

func (node *APITimeSegmentNode) ShouldPauseRefresh() bool {
	return true
}

func (node *APITimeSegmentNode) GetChildren() []MetricChild {
	// Check if we have individual API data for this time segment
	// For now, we'll show "All" as the only option since we have aggregated data
	return []MetricChild{
		{
			Name:        "All",
			Description: fmt.Sprintf("All API endpoints combined for %s time segment", node.segmentTime.Local().Format("15:04")),
		},
	}
}

func (node *APITimeSegmentNode) GetLeafData() map[string]string {
	// This node now has children, so it should just show navigation info
	data := make(map[string]string)
	endTime := node.segmentTime.Add(time.Duration(15) * time.Minute) // Assume 15-minute intervals for now
	data["Time Range"] = fmt.Sprintf("%s -> %s",
		node.segmentTime.Local().Format("15:04"),
		endTime.Local().Format("15:04"))
	data["Total Requests"] = humanize.Comma(node.segment.Requests)
	data["Available APIs"] = "Select 'All' to view combined statistics"
	return data
}

func (node *APITimeSegmentNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APITimeSegmentNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *APITimeSegmentNode) GetParent() MetricNode           { return node.parent }
func (node *APITimeSegmentNode) GetPath() string                 { return node.path }
func (node *APITimeSegmentNode) RequiredMetricTypes() MetricType { return MetricsAPI }
func (node *APITimeSegmentNode) GetChild(name string) (MetricNode, error) {
	if name == "All" {
		return &APITimeSegmentAllNode{
			segment:     node.segment,
			segmentTime: node.segmentTime,
			parent:      node,
			path:        node.path + "/" + name,
		}, nil
	}
	return nil, fmt.Errorf("API selection not found: %s", name)
}

// APIEndpointNode shows detailed statistics for a specific endpoint
type APIEndpointNode struct {
	endpoint string
	stats    APIStats
	parent   MetricNode
	path     string
}

func (node *APIEndpointNode) ShouldPauseRefresh() bool {
	return false
}

func (node *APIEndpointNode) GetChildren() []MetricChild {
	return []MetricChild{}
}

func (node *APIEndpointNode) GetLeafData() map[string]string {
	if node.stats.Requests == 0 {
		data := make(map[string]string)
		data["Status"] = "No requests recorded for this endpoint"
		return data
	}

	// Use ordered slice to maintain consistent display order
	var entries []struct{ key, value string }

	// === ENDPOINT HEADER ===
	entries = append(entries, struct{ key, value string }{"Endpoint", node.endpoint})
	entries = append(entries, struct{ key, value string }{"Total Requests", humanize.Comma(node.stats.Requests)})

	// === BASIC METRICS ===
	entries = append(entries, struct{ key, value string }{"Responding Nodes", fmt.Sprintf("%d nodes", node.stats.Nodes)})

	// Calculate RPS if we have wall time
	if node.stats.WallTimeSecs > 0 {
		rps := float64(node.stats.Requests) / node.stats.WallTimeSecs
		entries = append(entries, struct{ key, value string }{"Avg RPS", fmt.Sprintf("%.1f req/sec", rps)})
	}

	// === TIMING METRICS ===
	avgLatency := (node.stats.RequestTimeSecs / float64(node.stats.Requests)) * 1000
	entries = append(entries, struct{ key, value string }{"Avg Latency", fmt.Sprintf("%.1f ms", avgLatency)})

	if node.stats.RespTTFBSecs > 0 {
		avgTTFB := (node.stats.RespTTFBSecs / float64(node.stats.Requests)) * 1000
		entries = append(entries, struct{ key, value string }{"Avg TTFB", fmt.Sprintf("%.1f ms", avgTTFB)})
	}

	// === TIMING RANGES (all 4 like main page) ===
	if node.stats.RequestTimeSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"Latency Range", fmt.Sprintf("%.1f - %.1f ms",
			node.stats.RequestTimeSecsMin*1000, node.stats.RequestTimeSecsMax*1000)})
	}
	if node.stats.RespTTFBSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"TTFB Range", fmt.Sprintf("%.1f - %.1f ms",
			node.stats.RespTTFBSecsMin*1000, node.stats.RespTTFBSecsMax*1000)})
	}
	if node.stats.ReqReadSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"Req Read Range", fmt.Sprintf("%.1f - %.1f ms",
			node.stats.ReqReadSecsMin*1000, node.stats.ReqReadSecsMax*1000)})
	}
	if node.stats.RespSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"Resp Wr Range", fmt.Sprintf("%.1f - %.1f ms",
			node.stats.RespSecsMin*1000, node.stats.RespSecsMax*1000)})
	}

	// === THROUGHPUT ===
	totalBytes := node.stats.IncomingBytes + node.stats.OutgoingBytes
	if totalBytes > 0 && node.stats.Requests > 0 {
		entries = append(entries, struct{ key, value string }{"Total Throughput", humanize.Bytes(uint64(totalBytes))})
		entries = append(entries, struct{ key, value string }{"-> Incoming", humanize.Bytes(uint64(node.stats.IncomingBytes))})
		entries = append(entries, struct{ key, value string }{"<- Outgoing", humanize.Bytes(uint64(node.stats.OutgoingBytes))})

		avgBytesPerReq := totalBytes / node.stats.Requests
		entries = append(entries, struct{ key, value string }{"Avg Bytes", humanize.Bytes(uint64(avgBytesPerReq)) + "/req"})
	}

	// === ERROR ANALYSIS ===
	stats := node.stats
	if stats.Requests > 0 {
		totalErrors := stats.Errors5xx + stats.Errors5xx
		if stats.Requests > 0 {
			errorRate := float64(totalErrors) / float64(stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"Error Rate", fmt.Sprintf("%.2f%% (%d errors)", errorRate, totalErrors)})
		}
		if totalErrors > 0 {
			if stats.Errors4xx > 0 {
				clientErrorRate := float64(stats.Errors4xx) / float64(stats.Requests) * 100
				entries = append(entries, struct{ key, value string }{"↳ 4xx Client Errors", fmt.Sprintf("%d (%.2f%%)",
					stats.Errors4xx, clientErrorRate)})
			}
			if stats.Errors5xx > 0 {
				serverErrorRate := float64(stats.Errors5xx) / float64(stats.Requests) * 100
				entries = append(entries, struct{ key, value string }{"↳ 5xx Server Errors", fmt.Sprintf("%d (%.2f%%)",
					stats.Errors5xx, serverErrorRate)})
			}
			if stats.Canceled > 0 {
				cancelRate := float64(stats.Canceled) / float64(stats.Requests) * 100
				entries = append(entries, struct{ key, value string }{"↳ Canceled Requests", fmt.Sprintf("%d (%.2f%%)", stats.Canceled, cancelRate)})
			}
		}
	}

	// === REJECTIONS ===
	totalRejected := node.stats.Rejected.Auth + node.stats.Rejected.Header +
		node.stats.Rejected.Invalid + node.stats.Rejected.NotImplemented +
		node.stats.Rejected.RequestsTime
	if totalRejected > 0 {
		rejectionRate := float64(totalRejected) / float64(node.stats.Requests) * 100
		entries = append(entries, struct{ key, value string }{"Rejected Requests", fmt.Sprintf("%d rejections (%.2f%%)", totalRejected, rejectionRate)})

		if node.stats.Rejected.Auth > 0 {
			authRate := float64(node.stats.Rejected.Auth) / float64(node.stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Authentication", fmt.Sprintf("%d (%.2f%%)", node.stats.Rejected.Auth, authRate)})
		}
		if node.stats.Rejected.Header > 0 {
			headerRate := float64(node.stats.Rejected.Header) / float64(node.stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Header Issues", fmt.Sprintf("%d (%.2f%%)", node.stats.Rejected.Header, headerRate)})
		}
		if node.stats.Rejected.Invalid > 0 {
			invalidRate := float64(node.stats.Rejected.Invalid) / float64(node.stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Invalid Requests", fmt.Sprintf("%d (%.2f%%)", node.stats.Rejected.Invalid, invalidRate)})
		}
		if node.stats.Rejected.NotImplemented > 0 {
			notImplRate := float64(node.stats.Rejected.NotImplemented) / float64(node.stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Not Implemented", fmt.Sprintf("%d (%.2f%%)", node.stats.Rejected.NotImplemented, notImplRate)})
		}
		if node.stats.Rejected.RequestsTime > 0 {
			timeRate := float64(node.stats.Rejected.RequestsTime) / float64(node.stats.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Outdated Signatures", fmt.Sprintf("%d (%.2f%%)", node.stats.Rejected.RequestsTime, timeRate)})
		}
	}

	// === BLOCKING ANALYSIS ===
	if node.stats.Requests > 0 && (node.stats.ReadBlockedSecs > 0 || node.stats.WriteBlockedSecs > 0) {
		if node.stats.ReadBlockedSecs > 0 {
			avgReadBlocked := (node.stats.ReadBlockedSecs / float64(node.stats.Requests)) * 1000
			entries = append(entries, struct{ key, value string }{"Avg Read Blocking", fmt.Sprintf("%.1f ms/req", avgReadBlocked)})
		}
		if node.stats.WriteBlockedSecs > 0 {
			avgWriteBlocked := (node.stats.WriteBlockedSecs / float64(node.stats.Requests)) * 1000
			entries = append(entries, struct{ key, value string }{"Avg Write Blocking", fmt.Sprintf("%.1f ms/req", avgWriteBlocked)})
		}
	}

	// Convert ordered entries to map with numbered prefixes to preserve order
	data := make(map[string]string)
	for i, entry := range entries {
		key := fmt.Sprintf("%02d:%s", i, entry.key)
		data[key] = entry.value
	}

	return data
}

func (node *APIEndpointNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APIEndpointNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *APIEndpointNode) GetParent() MetricNode           { return node.parent }
func (node *APIEndpointNode) GetPath() string                 { return node.path }
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

func (node *APISegmentedNode) GetMetricType() MetricType       { return MetricsAPI }
func (node *APISegmentedNode) GetMetricFlags() MetricFlags     { return MetricsDayStats }
func (node *APISegmentedNode) GetParent() MetricNode           { return node.parent }
func (node *APISegmentedNode) GetPath() string                 { return node.path }
func (node *APISegmentedNode) RequiredMetricTypes() MetricType { return MetricsAPI }

func (node *APISegmentedNode) ShouldPauseRefresh() bool {
	return true
}
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
func (node *ReplicationMetricsNode) GetLeafData() map[string]string  { return nil }
func (node *ReplicationMetricsNode) GetMetricType() MetricType       { return MetricsReplication }
func (node *ReplicationMetricsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ReplicationMetricsNode) GetParent() MetricNode           { return node.parent }
func (node *ReplicationMetricsNode) GetPath() string                 { return node.path }
func (node *ReplicationMetricsNode) RequiredMetricTypes() MetricType { return MetricsReplication }

func (node *ReplicationMetricsNode) ShouldPauseRefresh() bool {
	return false
}
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
func (node *ProcessMetricsNode) GetLeafData() map[string]string  { return nil }
func (node *ProcessMetricsNode) GetMetricType() MetricType       { return MetricsProcess }
func (node *ProcessMetricsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *ProcessMetricsNode) GetParent() MetricNode           { return node.parent }
func (node *ProcessMetricsNode) GetPath() string                 { return node.path }
func (node *ProcessMetricsNode) RequiredMetricTypes() MetricType { return MetricsProcess }

func (node *ProcessMetricsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *ProcessMetricsNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("process metric sub-navigation not yet implemented for: %s", name)
}

// generateAPIOverviewDashboard creates a clean API performance dashboard
func (node *APIMetricsNode) generateAPIOverviewDashboard() map[string]string {
	lastMinute := node.api.LastMinuteTotal()

	// Use ordered slice to maintain consistent display order
	var entries []struct{ key, value string }

	// === SYSTEM INFO ===
	entries = append(entries, struct{ key, value string }{"Active Nodes", fmt.Sprintf("%d nodes responding", node.api.Nodes)})
	entries = append(entries, struct{ key, value string }{"Collection Time", node.api.CollectedAt.Format("15:04:05")})

	// === QUEUE STATUS ===
	entries = append(entries, struct{ key, value string }{"Active Requests", humanize.Comma(node.api.ActiveRequests)})
	entries = append(entries, struct{ key, value string }{"Queued Requests", humanize.Comma(node.api.QueuedRequests)})
	totalQueue := node.api.ActiveRequests + node.api.QueuedRequests
	entries = append(entries, struct{ key, value string }{"Total Queue Depth", humanize.Comma(totalQueue)})

	// === REQUEST METRICS ===
	if lastMinute.Requests > 0 {
		entries = append(entries, struct{ key, value string }{"Request Rate", fmt.Sprintf("%s req/min", humanize.Comma(lastMinute.Requests))})
		rps := float64(lastMinute.Requests) / 60.0
		entries = append(entries, struct{ key, value string }{"Avg RPS", fmt.Sprintf("%.1f req/sec", rps)})

		avgLatency := (lastMinute.RequestTimeSecs / float64(lastMinute.Requests)) * 1000
		entries = append(entries, struct{ key, value string }{"Avg Latency", fmt.Sprintf("%.1f ms", avgLatency)})

		if lastMinute.RespTTFBSecs > 0 {
			avgTTFB := (lastMinute.RespTTFBSecs / float64(lastMinute.Requests)) * 1000
			entries = append(entries, struct{ key, value string }{"Avg TTFB", fmt.Sprintf("%.1f ms", avgTTFB)})
		}
	} else {
		entries = append(entries, struct{ key, value string }{"Request Rate", "No requests"})
		entries = append(entries, struct{ key, value string }{"Avg RPS", "0.0 req/sec"})
	}

	// === TIMING RANGES ===
	if lastMinute.RequestTimeSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"Latency Range", fmt.Sprintf("%.1f - %.1f ms",
			lastMinute.RequestTimeSecsMin*1000, lastMinute.RequestTimeSecsMax*1000)})
	}
	if lastMinute.RespTTFBSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"TTFB Range", fmt.Sprintf("%.1f - %.1f ms",
			lastMinute.RespTTFBSecsMin*1000, lastMinute.RespTTFBSecsMax*1000)})
	}
	if lastMinute.ReqReadSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"Req Read Range", fmt.Sprintf("%.1f - %.1f ms",
			lastMinute.ReqReadSecsMin*1000, lastMinute.ReqReadSecsMax*1000)})
	}
	if lastMinute.RespSecsMax > 0 {
		entries = append(entries, struct{ key, value string }{"Resp Wr Range", fmt.Sprintf("%.1f - %.1f ms",
			lastMinute.RespSecsMin*1000, lastMinute.RespSecsMax*1000)})
	}

	// === THROUGHPUT ===
	totalBytes := lastMinute.IncomingBytes + lastMinute.OutgoingBytes
	if totalBytes > 0 {
		entries = append(entries, struct{ key, value string }{"Throughput", fmt.Sprintf("%s/min", humanize.Bytes(uint64(totalBytes)))})
		entries = append(entries, struct{ key, value string }{"↳ Incoming", fmt.Sprintf("%s/min", humanize.Bytes(uint64(lastMinute.IncomingBytes)))})
		entries = append(entries, struct{ key, value string }{"↳ Outgoing", fmt.Sprintf("%s/min", humanize.Bytes(uint64(lastMinute.OutgoingBytes)))})
	}

	// === ERROR ANALYSIS ===
	totalErrors := lastMinute.Errors4xx + lastMinute.Errors5xx
	if totalErrors > 0 || lastMinute.Requests > 0 {
		errorRate := float64(totalErrors) / float64(lastMinute.Requests) * 100
		if lastMinute.Requests == 0 {
			errorRate = 0
		}
		entries = append(entries, struct{ key, value string }{"Error Rate", fmt.Sprintf("%.2f%% (%d errors)", errorRate, totalErrors)})

		if lastMinute.Errors4xx > 0 {
			clientErrorRate := float64(lastMinute.Errors4xx) / float64(lastMinute.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ 4xx Client Errors", fmt.Sprintf("%d (%.2f%%)",
				lastMinute.Errors4xx, clientErrorRate)})
		}
		if lastMinute.Errors5xx > 0 {
			serverErrorRate := float64(lastMinute.Errors5xx) / float64(lastMinute.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ 5xx Server Errors", fmt.Sprintf("%d (%.2f%%)",
				lastMinute.Errors5xx, serverErrorRate)})
		}
		if lastMinute.Canceled > 0 {
			cancelRate := float64(lastMinute.Canceled) / float64(lastMinute.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Canceled Requests", fmt.Sprintf("%d (%.2f%%)", lastMinute.Canceled, cancelRate)})
		}
	} else {
		entries = append(entries, struct{ key, value string }{"Error Rate", "No errors detected"})
	}

	// === REJECTIONS ===
	rejections := lastMinute.Rejected
	totalRejected := rejections.Auth + rejections.Header + rejections.Invalid + rejections.NotImplemented + rejections.RequestsTime
	if totalRejected > 0 {
		rejectionRate := float64(totalRejected) / float64(lastMinute.Requests) * 100
		entries = append(entries, struct{ key, value string }{"Rejected Requests", fmt.Sprintf("%d rejections (%.2f%%)", totalRejected, rejectionRate)})

		if rejections.Auth > 0 {
			authRate := float64(rejections.Auth) / float64(lastMinute.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Authentication", fmt.Sprintf("%d (%.2f%%)", rejections.Auth, authRate)})
		}
		if rejections.Header > 0 {
			headerRate := float64(rejections.Header) / float64(lastMinute.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Header Issues", fmt.Sprintf("%d (%.2f%%)", rejections.Header, headerRate)})
		}
		if rejections.Invalid > 0 {
			invalidRate := float64(rejections.Invalid) / float64(lastMinute.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Invalid Requests", fmt.Sprintf("%d (%.2f%%)", rejections.Invalid, invalidRate)})
		}
		if rejections.NotImplemented > 0 {
			notImplRate := float64(rejections.NotImplemented) / float64(lastMinute.Requests) * 100
			entries = append(entries, struct{ key, value string }{"↳ Not Implemented", fmt.Sprintf("%d (%.2f%%)", rejections.NotImplemented, notImplRate)})
		}
	}

	// === BLOCKING ANALYSIS ===
	if lastMinute.Requests > 0 && (lastMinute.ReadBlockedSecs > 0 || lastMinute.WriteBlockedSecs > 0) {
		if lastMinute.ReadBlockedSecs > 0 {
			avgReadBlocked := (lastMinute.ReadBlockedSecs / float64(lastMinute.Requests)) * 1000
			entries = append(entries, struct{ key, value string }{"Avg Read Blocking", fmt.Sprintf("%.1f ms/req", avgReadBlocked)})
		}
		if lastMinute.WriteBlockedSecs > 0 {
			avgWriteBlocked := (lastMinute.WriteBlockedSecs / float64(lastMinute.Requests)) * 1000
			entries = append(entries, struct{ key, value string }{"Avg Write Blocking", fmt.Sprintf("%.1f ms/req", avgWriteBlocked)})
		}
	}

	// Convert ordered entries to map with numbered prefixes to preserve order
	data := make(map[string]string)
	for i, entry := range entries {
		key := fmt.Sprintf("%02d:%s", i, entry.key)
		data[key] = entry.value
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
	errorRate := float64(lastMinute.Errors4xx+lastMinute.Errors5xx) / float64(lastMinute.Requests) * 100
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
	errorRate := float64(lastMinute.Errors4xx+lastMinute.Errors5xx) / float64(lastMinute.Requests) * 100
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
