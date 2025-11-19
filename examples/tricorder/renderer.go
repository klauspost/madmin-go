package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/minio/madmin-go/v4"
)

// Styles for the UI
var (
	// Colors
	primaryColor   = lipgloss.Color("#6bcf7f") // Green
	secondaryColor = lipgloss.Color("#7c7c7c") // Gray
	errorColor     = lipgloss.Color("#ff6b6b") // Red
	warningColor   = lipgloss.Color("#ffd93d") // Yellow
	successColor   = lipgloss.Color("#6bcf7f") // Green

	// Main styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			PaddingLeft(2)

	pathStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Italic(true).
			PaddingLeft(2)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			Background(primaryColor).
			PaddingLeft(1).
			PaddingRight(1)

	itemStyle = lipgloss.NewStyle().
			PaddingLeft(3)

	descriptionStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Italic(true)

	keyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Width(20)

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff"))

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			PaddingLeft(2)

	statusStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Italic(true).
			PaddingLeft(2)

	helpStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Border(lipgloss.RoundedBorder(), true, false, false, false).
			BorderForeground(secondaryColor).
			Padding(0, 1)
)

// Renderer handles formatting and display of metrics data
type Renderer struct {
	width  int
	height int
}

// NewRenderer creates a new renderer
func NewRenderer(width, height int) *Renderer {
	return &Renderer{
		width:  width,
		height: height,
	}
}

// SetSize updates the renderer dimensions
func (r *Renderer) SetSize(width, height int) {
	r.width = width
	r.height = height
}

// RenderHeader renders the main header with title and path
func (r *Renderer) RenderHeader(nav *NavigationState) string {
	// Build combined header line
	breadcrumbs := nav.GetBreadcrumbs()
	pathDisplay := strings.Join(breadcrumbs, " › ")

	// Get timing info with fixed width
	var timeDisplay string
	if nav.IsRefreshing() {
		timeDisplay = "refreshing"
	} else {
		lastRefresh := nav.GetLastRefresh()
		if !lastRefresh.IsZero() {
			timeAgo := time.Since(lastRefresh)
			if timeAgo < 10*time.Second {
				timeDisplay = fmt.Sprintf("%.1fs ago", timeAgo.Seconds())
			} else {
				timeDisplay = fmt.Sprintf("%vs ago", int(timeAgo.Seconds()))
			}
		} else {
			timeDisplay = "no refresh"
		}
	}

	// Get metric type
	var typeDisplay string
	metricType, _ := nav.GetMetricInfo()
	if metricType != madmin.MetricsNone {
		typeDisplay = r.formatMetricType(metricType)
	}

	// Combine everything in one line
	var combined string
	if typeDisplay != "" {
		combined = fmt.Sprintf("📡 %s [%s | %s]", pathDisplay, timeDisplay, typeDisplay)
	} else {
		combined = fmt.Sprintf("📡 %s [%s]", pathDisplay, timeDisplay)
	}

	return titleStyle.Render(combined) + "\n"
}

// renderStatus renders the status line with refresh time and errors
func (r *Renderer) renderStatus(nav *NavigationState) string {
	var statusParts []string

	// Last refresh time
	if nav.IsRefreshing() {
		statusParts = append(statusParts, "🔄 Refreshing...")
	} else {
		lastRefresh := nav.GetLastRefresh()
		if !lastRefresh.IsZero() {
			timeAgo := time.Since(lastRefresh).Truncate(time.Second)
			statusParts = append(statusParts, fmt.Sprintf("Last refresh: %v ago", timeAgo))
		}
	}

	// Metric type info
	metricType, _ := nav.GetMetricInfo()
	if metricType != madmin.MetricsNone {
		typeStr := r.formatMetricType(metricType)
		if typeStr != "" {
			statusParts = append(statusParts, fmt.Sprintf("Type: %s", typeStr))
		}
	}

	if len(statusParts) > 0 {
		return statusStyle.Render(strings.Join(statusParts, " | "))
	}

	return ""
}

// RenderError renders error messages
func (r *Renderer) RenderError(nav *NavigationState) string {
	errorMsg := nav.GetErrorMessage()
	if errorMsg == "" {
		return ""
	}

	return errorStyle.Render(fmt.Sprintf("⚠️  %s", errorMsg)) + "\n"
}

// RenderContent renders the main content area (children first, then properties)
func (r *Renderer) RenderContent(nav *NavigationState) string {
	var content []string

	// Show navigation children first if available, or back navigation for leaf nodes
	if !nav.IsLeaf() {
		content = append(content, r.renderChildren(nav))
	} else if nav.CanNavigateBack() {
		// For leaf nodes, still show back navigation
		content = append(content, r.renderBackNavigation(nav))
	}

	// Then show leaf data/properties at the bottom
	leafData := nav.GetLeafData()
	if len(leafData) > 0 {
		if len(content) > 0 {
			content = append(content, "") // Add spacing between sections
		}
		content = append(content, r.renderLeafData(nav))
	}

	if len(content) == 0 {
		if nav.IsRefreshing() {
			return itemStyle.Render("🔄 Loading metrics...")
		}
		return itemStyle.Render("No data available")
	}

	return strings.Join(content, "\n")
}

// renderBackNavigation renders just the back navigation for leaf nodes
func (r *Renderer) renderBackNavigation(nav *NavigationState) string {
	if !nav.CanNavigateBack() {
		return ""
	}

	var lines []string
	lines = append(lines, itemStyle.Render("Navigation:"))
	lines = append(lines, "")

	selectedIndex := nav.GetSelectedIndex()
	var line string
	if selectedIndex == 0 {
		// Highlighted selection
		line = selectedStyle.Render("► ..")
	} else {
		// Normal item (this shouldn't happen for leaf nodes, but just in case)
		line = itemStyle.Render("  ..")
	}
	lines = append(lines, line)

	return strings.Join(lines, "\n")
}

// renderChildren renders the list of child nodes
func (r *Renderer) renderChildren(nav *NavigationState) string {
	children := nav.GetChildren()
	if len(children) == 0 {
		if nav.IsRefreshing() {
			return itemStyle.Render("🔄 Loading metrics...")
		}
		// If no children and can navigate back, show back navigation
		if nav.CanNavigateBack() {
			return r.renderBackNavigation(nav)
		}
		return itemStyle.Render("No children available")
	}

	var lines []string

	// Add back navigation option if we can go back
	totalOptions := len(children)
	if nav.CanNavigateBack() {
		totalOptions++
	}

	lines = append(lines, itemStyle.Render(fmt.Sprintf("Available options (%d):", totalOptions)))
	lines = append(lines, "")

	selectedIndex := nav.GetSelectedIndex()
	currentIndex := 0

	// Add .. back navigation as first option if available
	if nav.CanNavigateBack() {
		var line string
		if selectedIndex == 0 {
			// Highlighted selection
			line = selectedStyle.Render("► ..")
		} else {
			// Normal item
			line = itemStyle.Render("  ..")
		}
		lines = append(lines, line)
		currentIndex = 1
	}

	// Add regular children, adjusting for the .. entry
	for i, child := range children {
		var line string
		displayIndex := currentIndex + i
		if displayIndex == selectedIndex {
			// Highlighted selection
			line = selectedStyle.Render(fmt.Sprintf("► %s", child.Name))
			if child.Description != "" {
				line += " " + descriptionStyle.Render(fmt.Sprintf("- %s", child.Description))
			}
		} else {
			// Normal item
			line = itemStyle.Render(fmt.Sprintf("  %s", child.Name))
			if child.Description != "" {
				line += " " + descriptionStyle.Render(fmt.Sprintf("- %s", child.Description))
			}
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderLeafData renders key-value data for leaf nodes
func (r *Renderer) renderLeafData(nav *NavigationState) string {
	data := nav.GetLeafData()
	if len(data) == 0 {
		if nav.IsRefreshing() {
			return itemStyle.Render("🔄 Loading data...")
		}
		return itemStyle.Render("No data available")
	}

	var lines []string
	lines = append(lines, itemStyle.Render(fmt.Sprintf("Metric Data (%d values):", len(data))))
	lines = append(lines, "")

	// Sort keys by numeric prefix if present, otherwise alphabetically
	var keys []string
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys) // This will sort by the numeric prefix first

	for _, key := range keys {
		value := data[key]

		// Strip numeric prefix if present (format: "00:Key Name")
		displayKey := key
		if colonIndex := strings.Index(key, ":"); colonIndex != -1 && colonIndex <= 3 {
			displayKey = key[colonIndex+1:]
		}

		// Skip empty keys (like separators)
		if strings.TrimSpace(displayKey) == "" {
			continue
		}

		formattedValue := r.formatValue(displayKey, value)

		line := fmt.Sprintf("%s %s",
			keyStyle.Render(displayKey),
			valueStyle.Render(formattedValue))

		lines = append(lines, itemStyle.Render(line))
	}

	return strings.Join(lines, "\n")
}

// formatValue formats a value based on its key and content
func (r *Renderer) formatValue(key, value string) string {
	// Handle special formatting for common types
	if strings.Contains(key, "time") || strings.Contains(key, "duration") {
		if num, err := strconv.ParseInt(value, 10, 64); err == nil {
			if strings.Contains(key, "duration") || strings.Contains(key, "time") {
				// Assume nanoseconds for duration-like fields
				duration := time.Duration(num) * time.Nanosecond
				return duration.String()
			}
			if strings.Contains(key, "timestamp") || strings.Contains(key, "collected_at") {
				// Assume Unix timestamp
				if num > 1000000000 { // Looks like Unix timestamp
					timestamp := time.Unix(num, 0)
					return timestamp.Format("2006-01-02 15:04:05")
				}
			}
		}
	}

	// Handle byte counts
	if strings.Contains(key, "byte") || strings.Contains(key, "size") {
		if num, err := strconv.ParseInt(value, 10, 64); err == nil {
			return r.formatBytes(num)
		}
	}

	// Handle counts and numbers
	if num, err := strconv.ParseInt(value, 10, 64); err == nil {
		return r.formatNumber(num)
	}

	// Handle percentages
	if strings.HasSuffix(key, "_percent") || strings.Contains(key, "usage") {
		if num, err := strconv.ParseFloat(value, 64); err == nil {
			return fmt.Sprintf("%.2f%%", num)
		}
	}

	// Return as-is for strings
	return value
}

// formatBytes formats byte counts in human readable format
func (r *Renderer) formatBytes(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	size := float64(bytes)
	unitIndex := 0

	for size >= 1024 && unitIndex < len(units)-1 {
		size /= 1024
		unitIndex++
	}

	if unitIndex == 0 {
		return fmt.Sprintf("%d %s", int64(size), units[unitIndex])
	}
	return fmt.Sprintf("%.2f %s", size, units[unitIndex])
}

// formatNumber formats large numbers with thousand separators
func (r *Renderer) formatNumber(num int64) string {
	str := strconv.FormatInt(num, 10)
	if len(str) <= 3 {
		return str
	}

	var result strings.Builder
	for i, digit := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(digit)
	}

	return result.String()
}

// formatMetricType converts MetricType to readable string
func (r *Renderer) formatMetricType(metricType madmin.MetricType) string {
	var types []string

	if metricType.Contains(madmin.MetricsScanner) {
		types = append(types, "Scanner")
	}
	if metricType.Contains(madmin.MetricsDisk) {
		types = append(types, "Disk")
	}
	if metricType.Contains(madmin.MetricsOS) {
		types = append(types, "OS")
	}
	if metricType.Contains(madmin.MetricsBatchJobs) {
		types = append(types, "Batch")
	}
	if metricType.Contains(madmin.MetricsSiteResync) {
		types = append(types, "Resync")
	}
	if metricType.Contains(madmin.MetricNet) {
		types = append(types, "Network")
	}
	if metricType.Contains(madmin.MetricsMem) {
		types = append(types, "Memory")
	}
	if metricType.Contains(madmin.MetricsCPU) {
		types = append(types, "CPU")
	}
	if metricType.Contains(madmin.MetricsRPC) {
		types = append(types, "RPC")
	}
	if metricType.Contains(madmin.MetricsRuntime) {
		types = append(types, "Runtime")
	}
	if metricType.Contains(madmin.MetricsAPI) {
		types = append(types, "API")
	}
	if metricType.Contains(madmin.MetricsReplication) {
		types = append(types, "Replication")
	}
	if metricType.Contains(madmin.MetricsProcess) {
		types = append(types, "Process")
	}

	return strings.Join(types, ",")
}

// RenderHelp renders the help/instructions
func (r *Renderer) RenderHelp() string {
	helpText := []string{
		"Navigation: ↑/↓ Move/Scroll  Enter Select  Esc Back  PgUp/PgDn Page  r Refresh  q Quit",
	}

	return helpStyle.Render(strings.Join(helpText, " | "))
}