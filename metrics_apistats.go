package madmin

import (
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

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
			a.LastDayAPI[k] = v
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

// GetHealthScore calculates a health score (0-10) based on API performance
func (a APIStats) GetHealthScore() float64 {
	if a.Requests == 0 {
		return 8.0 // Neutral score for no activity
	}

	score := 10.0

	// Error rate penalty
	totalErrors := a.Errors4xx + a.Errors5xx
	errorRate := float64(totalErrors) / float64(a.Requests) * 100
	if errorRate > 5.0 {
		score -= 3.0
	} else if errorRate > 1.0 {
		score -= 1.0
	}

	// Latency penalty
	if a.Requests > 0 {
		avgLatency := (a.RequestTimeSecs / float64(a.Requests)) * 1000
		if avgLatency > 5000 {
			score -= 2.0
		} else if avgLatency > 1000 {
			score -= 1.0
		}
	}

	// Rejection penalty
	totalRejected := a.Rejected.Auth + a.Rejected.Header + a.Rejected.Invalid +
		a.Rejected.NotImplemented + a.Rejected.RequestsTime
	if totalRejected > 0 {
		rejectionRate := float64(totalRejected) / float64(a.Requests) * 100
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

// GetHealthStatus returns a descriptive health status
func (a APIStats) GetHealthStatus() string {
	score := a.GetHealthScore()
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

// GetRecommendations returns actionable performance recommendations
func (a APIStats) GetRecommendations() []string {
	if a.Requests == 0 {
		return []string{"No recent API activity to analyze"}
	}

	var recommendations []string

	// Error rate recommendations
	totalErrors := a.Errors4xx + a.Errors5xx
	errorRate := float64(totalErrors) / float64(a.Requests) * 100
	if errorRate > 5.0 {
		recommendations = append(recommendations, "High error rate detected - investigate failing endpoints")
	}

	if a.Errors5xx > a.Errors4xx && a.Errors5xx > 0 {
		recommendations = append(recommendations, "Server errors exceed client errors - check system health")
	}

	// Latency recommendations
	if a.Requests > 0 {
		avgLatency := (a.RequestTimeSecs / float64(a.Requests)) * 1000
		if avgLatency > 2000 {
			recommendations = append(recommendations, "High average latency - consider performance optimization")
		}

		if a.RequestTimeSecsMax > 0 && a.RequestTimeSecsMax*1000 > avgLatency*5 {
			recommendations = append(recommendations, "High latency variance detected - investigate slow endpoints")
		}
	}

	// TTFB recommendations
	if a.RespTTFBSecs > 0 && a.Requests > 0 {
		avgTTFB := (a.RespTTFBSecs / float64(a.Requests)) * 1000
		if avgTTFB > 500 {
			recommendations = append(recommendations, "Slow time-to-first-byte - optimize request processing")
		}
	}

	// Rejection recommendations
	if a.Rejected.Auth > 0 {
		recommendations = append(recommendations, "Authentication failures detected - verify client credentials")
	}

	if a.Rejected.Invalid > 0 {
		recommendations = append(recommendations, "Invalid request signatures - check client request formatting")
	}

	return recommendations
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

	// Health score
	score := lastMinute.GetHealthScore()
	parts = append(parts, fmt.Sprintf("Health: %.1f/10 %s", score, lastMinute.GetHealthStatus()))

	return strings.Join(parts, " | ")
}

// GetDashboard returns a comprehensive executive dashboard for API metrics
func (a APIMetrics) GetDashboard() map[string]string {
	data := make(map[string]string)

	// === API HEALTH SUMMARY ===
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""
	data["                API HEALTH SUMMARY"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"] = ""

	lastMinute := a.LastMinuteTotal()
	healthScore := lastMinute.GetHealthScore()
	data["Overall API Health Score"] = fmt.Sprintf("%.1f/10 %s", healthScore, lastMinute.GetHealthStatus())
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
				lastMinute.RequestTimeSecsMin*1000, lastMinute.RequestTimeSecsMax*1000)
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

	// === LIFETIME INSIGHTS ===
	data["   "] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   "] = ""
	data["            LIFETIME INSIGHTS"] = ""
	data["━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   "] = ""

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

	// === RECOMMENDATIONS ===
	recommendations := lastMinute.GetRecommendations()
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