package madmin

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
)

// formatNumberCPU function removed - use humanize.Comma instead

// formatFrequency formats frequency values
func formatFrequency(freq uint64) string {
	if freq == 0 {
		return "0 Hz"
	}

	if freq >= 1000000000 {
		return fmt.Sprintf("%.2f GHz", float64(freq)/1000000000)
	} else if freq >= 1000000 {
		return fmt.Sprintf("%.2f MHz", float64(freq)/1000000)
	} else if freq >= 1000 {
		return fmt.Sprintf("%.2f KHz", float64(freq)/1000)
	}
	return fmt.Sprintf("%d Hz", freq)
}

// CPUMetricsNavigator provides navigation for CPU metrics
type CPUMetricsNavigator struct {
	cpu    *CPUMetrics
	parent MetricNode
	path   string
}

// NewCPUMetricsNavigator creates a new CPU metrics navigator
func NewCPUMetricsNavigator(cpu *CPUMetrics, parent MetricNode, path string) *CPUMetricsNavigator {
	return &CPUMetricsNavigator{cpu: cpu, parent: parent, path: path}
}

func (node *CPUMetricsNavigator) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "times", Description: "CPU time breakdown (user/system/idle/etc)"},
		{Name: "load", Description: "System load averages (1min/5min/15min)"},
		{Name: "frequency", Description: "CPU frequency and scaling information"},
		{Name: "models", Description: "CPU model distribution across cluster"},
		{Name: "governors", Description: "CPU frequency governor distribution"},
	}
}

func (node *CPUMetricsNavigator) GetLeafData() map[string]string {
	if node.cpu == nil {
		return map[string]string{"Error": "CPU metrics not available"}
	}

	data := map[string]string{}

	// CPU Overview
	data["CPU OVERVIEW"] = fmt.Sprintf("Collected at %s",
		node.cpu.CollectedAt.Format("2006-01-02 15:04:05"))

	// Cluster Architecture
	if node.cpu.Nodes > 0 {
		data["Cluster Architecture"] = fmt.Sprintf("%s nodes, %s total CPUs (%s CPUs/node avg)",
			humanize.Comma(int64(node.cpu.Nodes)),
			humanize.Comma(int64(node.cpu.CPUCount)),
			fmt.Sprintf("%.1f", float64(node.cpu.CPUCount)/float64(node.cpu.Nodes)))

		if node.cpu.TotalCores > 0 {
			data["Processing Cores"] = fmt.Sprintf("%s total cores (%s cores/node avg, %.1f cores/CPU avg)",
				humanize.Comma(int64(node.cpu.TotalCores)),
				fmt.Sprintf("%.1f", float64(node.cpu.TotalCores)/float64(node.cpu.Nodes)),
				float64(node.cpu.TotalCores)/float64(node.cpu.CPUCount))
		}
	}

	// Performance Summary
	if node.cpu.TotalMhz > 0 {
		totalGhz := node.cpu.TotalMhz / 1000
		data["Processing Power"] = fmt.Sprintf("%.2f GHz total cluster capacity",
			totalGhz)
		if node.cpu.Nodes > 0 {
			avgGhzPerNode := totalGhz / float64(node.cpu.Nodes)
			data["Power per Node"] = fmt.Sprintf("%.2f GHz average per node",
				avgGhzPerNode)
		}
	}

	// Frequency Analysis
	if node.cpu.FreqStatsCount > 0 {
		currentFreq := node.cpu.TotalCurrentFreq / uint64(node.cpu.FreqStatsCount)
		maxFreq := node.cpu.MaxCPUInfoFreq

		data["FREQUENCY ANALYSIS"] = fmt.Sprintf("%d CPUs monitored for frequency",
			node.cpu.FreqStatsCount)

		data["Current Performance"] = fmt.Sprintf("%s average frequency",
			formatFrequency(currentFreq))

		if maxFreq > 0 {
			utilizationPercent := float64(currentFreq) / float64(maxFreq) * 100
			data["Frequency Utilization"] = fmt.Sprintf("%.1f%% of maximum capability (%s max)",
				utilizationPercent, formatFrequency(maxFreq))
		}

		if node.cpu.MinCPUInfoFreq > 0 && node.cpu.MaxCPUInfoFreq > 0 {
			data["Frequency Range"] = fmt.Sprintf("%s - %s available range",
				formatFrequency(node.cpu.MinCPUInfoFreq),
				formatFrequency(node.cpu.MaxCPUInfoFreq))
		}
	}

	// Cache Architecture
	if node.cpu.TotalCacheSize > 0 {
		totalCacheGB := float64(node.cpu.TotalCacheSize) / (1024 * 1024 * 1024)
		data["Cache Architecture"] = fmt.Sprintf("%.2f GB total cache across cluster",
			totalCacheGB)
		if node.cpu.Nodes > 0 {
			avgCacheMB := float64(node.cpu.TotalCacheSize) / (1024 * 1024 * float64(node.cpu.Nodes))
			data["Cache per Node"] = fmt.Sprintf("%.1f MB average per node",
				avgCacheMB)
		}
	}

	// Hardware Diversity
	if len(node.cpu.CPUByModel) > 0 {
		data["HARDWARE DIVERSITY"] = fmt.Sprintf("%d distinct CPU models deployed",
			len(node.cpu.CPUByModel))

		// Find most common CPU model
		var mostCommonModel string
		var maxCount int
		for model, count := range node.cpu.CPUByModel {
			if count > maxCount {
				maxCount = count
				mostCommonModel = model
			}
		}
		if mostCommonModel != "" {
			percentage := float64(maxCount) / float64(node.cpu.CPUCount) * 100
			modelDisplay := mostCommonModel
			if len(modelDisplay) > 50 {
				modelDisplay = modelDisplay[:47] + "..."
			}
			data["Primary CPU Model"] = fmt.Sprintf("%s (%d CPUs, %.1f%%)",
				modelDisplay, maxCount, percentage)
		}
	}

	// Governor Configuration
	if len(node.cpu.GovernorFreq) > 0 {
		data["Power Management"] = fmt.Sprintf("%d frequency governors active",
			len(node.cpu.GovernorFreq))

		// Find most common governor
		var primaryGovernor string
		var maxCount int
		for governor, count := range node.cpu.GovernorFreq {
			if count > maxCount {
				maxCount = count
				primaryGovernor = governor
			}
		}
		if primaryGovernor != "" {
			data["Primary Governor"] = fmt.Sprintf("%s (%d CPUs)",
				primaryGovernor, maxCount)
		}
	}

	// System Health Indicators
	var healthStatus []string
	if node.cpu.TimesStat != nil {
		healthStatus = append(healthStatus, "CPU timing metrics")
	}
	if node.cpu.LoadStat != nil {
		healthStatus = append(healthStatus, "load averages")
	}
	if node.cpu.FreqStatsCount > 0 {
		healthStatus = append(healthStatus, "frequency monitoring")
	}
	if len(healthStatus) > 0 {
		data["Monitoring Health"] = strings.Join(healthStatus, ", ")
	}

	return data
}

func (node *CPUMetricsNavigator) GetMetricType() MetricType {
	return MetricsCPU
}

func (node *CPUMetricsNavigator) GetMetricFlags() MetricFlags {
	return 0
}

func (node *CPUMetricsNavigator) GetParent() MetricNode {
	return node.parent
}

func (node *CPUMetricsNavigator) GetPath() string {
	return node.path
}

func (node *CPUMetricsNavigator) RequiredMetricTypes() MetricType {
	return MetricsCPU
}

func (node *CPUMetricsNavigator) GetChild(name string) (MetricNode, error) {
	switch name {
	case "times":
		return NewCPUTimesNode(node.cpu.TimesStat, node, fmt.Sprintf("%s/times", node.path)), nil
	case "load":
		return NewCPULoadNode(node.cpu.LoadStat, node, fmt.Sprintf("%s/load", node.path)), nil
	case "frequency":
		return NewCPUFrequencyNode(node.cpu, node, fmt.Sprintf("%s/frequency", node.path)), nil
	case "models":
		return NewCPUModelsNode(node.cpu.CPUByModel, node, fmt.Sprintf("%s/models", node.path)), nil
	case "governors":
		return NewCPUGovernorsNode(node.cpu.GovernorFreq, node, fmt.Sprintf("%s/governors", node.path)), nil
	default:
		return nil, fmt.Errorf("child not found: %s", name)
	}
}

// CPUTimesNode handles navigation for CPU time statistics
type CPUTimesNode struct {
	times  interface{} // cpu.TimesStat from gopsutil
	parent MetricNode
	path   string
}

func NewCPUTimesNode(times interface{}, parent MetricNode, path string) *CPUTimesNode {
	return &CPUTimesNode{times: times, parent: parent, path: path}
}

func (node *CPUTimesNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "user", Description: "User CPU time"},
		{Name: "system", Description: "System CPU time"},
		{Name: "idle", Description: "Idle CPU time"},
		{Name: "nice", Description: "Nice CPU time"},
		{Name: "iowait", Description: "IO wait CPU time"},
		{Name: "irq", Description: "IRQ CPU time"},
		{Name: "softirq", Description: "Soft IRQ CPU time"},
		{Name: "steal", Description: "Steal CPU time"},
		{Name: "guest", Description: "Guest CPU time"},
		{Name: "guest_nice", Description: "Guest nice CPU time"},
	}
}

func (node *CPUTimesNode) GetLeafData() map[string]string {
	// CPU times statistics would be extracted from gopsutil TimesStat
	// For now, return basic structure
	return map[string]string{
		"available": strconv.FormatBool(node.times != nil),
	}
}

func (node *CPUTimesNode) GetMetricType() MetricType       { return MetricsCPU }
func (node *CPUTimesNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *CPUTimesNode) GetParent() MetricNode           { return node.parent }
func (node *CPUTimesNode) GetPath() string                 { return node.path }
func (node *CPUTimesNode) RequiredMetricTypes() MetricType { return MetricsCPU }
func (node *CPUTimesNode) GetChild(name string) (MetricNode, error) {
	// Individual time component nodes would be implemented here
	return nil, fmt.Errorf("cpu time component navigation not yet implemented for: %s", name)
}

// CPULoadNode handles navigation for system load averages
type CPULoadNode struct {
	load   interface{} // load.AvgStat from gopsutil
	parent MetricNode
	path   string
}

func NewCPULoadNode(load interface{}, parent MetricNode, path string) *CPULoadNode {
	return &CPULoadNode{load: load, parent: parent, path: path}
}

func (node *CPULoadNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "load1", Description: "1-minute load average"},
		{Name: "load5", Description: "5-minute load average"},
		{Name: "load15", Description: "15-minute load average"},
	}
}

func (node *CPULoadNode) GetLeafData() map[string]string {
	// Load average statistics would be extracted from gopsutil AvgStat
	// For now, return basic structure
	return map[string]string{
		"available": strconv.FormatBool(node.load != nil),
	}
}

func (node *CPULoadNode) GetMetricType() MetricType       { return MetricsCPU }
func (node *CPULoadNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *CPULoadNode) GetParent() MetricNode           { return node.parent }
func (node *CPULoadNode) GetPath() string                 { return node.path }
func (node *CPULoadNode) RequiredMetricTypes() MetricType { return MetricsCPU }
func (node *CPULoadNode) GetChild(name string) (MetricNode, error) {
	// Individual load component nodes would be implemented here
	return nil, fmt.Errorf("cpu load component navigation not yet implemented for: %s", name)
}

// CPUFrequencyNode handles navigation for CPU frequency information
type CPUFrequencyNode struct {
	cpu    *CPUMetrics
	parent MetricNode
	path   string
}

func NewCPUFrequencyNode(cpu *CPUMetrics, parent MetricNode, path string) *CPUFrequencyNode {
	return &CPUFrequencyNode{cpu: cpu, parent: parent, path: path}
}

func (node *CPUFrequencyNode) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "current", Description: "Current CPU frequency statistics"},
		{Name: "scaling", Description: "CPU scaling frequency statistics"},
		{Name: "limits", Description: "CPU frequency limits"},
		{Name: "governors", Description: "CPU frequency governors"},
	}
}

func (node *CPUFrequencyNode) GetLeafData() map[string]string {
	if node.cpu == nil || node.cpu.FreqStatsCount == 0 {
		return map[string]string{
			"Status": "No frequency monitoring data available",
			"Note":   "Frequency stats require CPU frequency monitoring capability",
		}
	}

	data := map[string]string{}

	data["FREQUENCY MONITORING"] = fmt.Sprintf("%s CPUs monitored for frequency scaling",
		humanize.Comma(int64(node.cpu.FreqStatsCount)))

	// Current performance
	currentFreq := node.cpu.TotalCurrentFreq / uint64(node.cpu.FreqStatsCount)
	scalingFreq := node.cpu.TotalScalingCurrentFreq / uint64(node.cpu.FreqStatsCount)

	data["Current Performance"] = fmt.Sprintf("%s average actual frequency",
		formatFrequency(currentFreq))
	data["Scaling Frequency"] = fmt.Sprintf("%s average scaling frequency",
		formatFrequency(scalingFreq))

	// Performance analysis
	if node.cpu.MaxCPUInfoFreq > 0 {
		utilizationPercent := float64(currentFreq) / float64(node.cpu.MaxCPUInfoFreq) * 100
		data["Performance Utilization"] = fmt.Sprintf("%.1f%% of maximum capability",
			utilizationPercent)
	}

	// Hardware capabilities
	if node.cpu.MinCPUInfoFreq > 0 && node.cpu.MaxCPUInfoFreq > 0 {
		data["Hardware Range"] = fmt.Sprintf("%s - %s (CPU info limits)",
			formatFrequency(node.cpu.MinCPUInfoFreq),
			formatFrequency(node.cpu.MaxCPUInfoFreq))
	}

	if node.cpu.MinScalingFreq > 0 && node.cpu.MaxScalingFreq > 0 {
		data["Scaling Range"] = fmt.Sprintf("%s - %s (governor-controlled)",
			formatFrequency(node.cpu.MinScalingFreq),
			formatFrequency(node.cpu.MaxScalingFreq))
	}

	// Efficiency insights
	if currentFreq > 0 && scalingFreq > 0 {
		if currentFreq == scalingFreq {
			data["Scaling Status"] = "CPU running at target scaling frequency"
		} else {
			diff := float64(currentFreq) / float64(scalingFreq) * 100
			data["Scaling Status"] = fmt.Sprintf("CPU at %.1f%% of scaling target", diff)
		}
	}

	return data
}

func (node *CPUFrequencyNode) GetMetricType() MetricType       { return MetricsCPU }
func (node *CPUFrequencyNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *CPUFrequencyNode) GetParent() MetricNode           { return node.parent }
func (node *CPUFrequencyNode) GetPath() string                 { return node.path }
func (node *CPUFrequencyNode) RequiredMetricTypes() MetricType { return MetricsCPU }
func (node *CPUFrequencyNode) GetChild(name string) (MetricNode, error) {
	switch name {
	case "governors":
		return NewCPUGovernorsNode(node.cpu.GovernorFreq, node, fmt.Sprintf("%s/governors", node.path)), nil
	default:
		return nil, fmt.Errorf("cpu frequency component navigation not yet implemented for: %s", name)
	}
}

// CPUModelsNode handles navigation for CPU model information
type CPUModelsNode struct {
	models map[string]int
	parent MetricNode
	path   string
}

func NewCPUModelsNode(models map[string]int, parent MetricNode, path string) *CPUModelsNode {
	return &CPUModelsNode{models: models, parent: parent, path: path}
}

func (node *CPUModelsNode) GetChildren() []MetricChild {
	var children []MetricChild
	for modelName := range node.models {
		children = append(children, MetricChild{
			Name:        modelName,
			Description: fmt.Sprintf("CPU model %s information", modelName),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})
	return children
}

func (node *CPUModelsNode) GetLeafData() map[string]string {
	if node.models == nil {
		return map[string]string{"CPU Models": "0", "Total CPUs": "0"}
	}

	data := map[string]string{}
	var totalCPUs int

	// Sort models by count (descending)
	type modelCount struct {
		name  string
		count int
	}
	var models []modelCount
	for modelName, count := range node.models {
		models = append(models, modelCount{modelName, count})
		totalCPUs += count
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].count > models[j].count
	})

	data["CPU MODEL DISTRIBUTION"] = fmt.Sprintf("%d distinct models, %s total CPUs",
		len(node.models), humanize.Comma(int64(totalCPUs)))

	// Show each model with percentage
	for _, model := range models {
		percentage := float64(model.count) / float64(totalCPUs) * 100
		displayName := strings.TrimSpace(model.name)
		if len(displayName) > 50 {
			displayName = displayName[:47] + "..."
		}
		data[displayName] = fmt.Sprintf("%s CPUs (%.1f%%)",
			humanize.Comma(int64(model.count)), percentage)
	}

	return data
}

func (node *CPUModelsNode) GetMetricType() MetricType       { return MetricsCPU }
func (node *CPUModelsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *CPUModelsNode) GetParent() MetricNode           { return node.parent }
func (node *CPUModelsNode) GetPath() string                 { return node.path }
func (node *CPUModelsNode) RequiredMetricTypes() MetricType { return MetricsCPU }
func (node *CPUModelsNode) GetChild(name string) (MetricNode, error) {
	if count, exists := node.models[name]; exists {
		return NewCPUModelNode(name, count, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("cpu model not found: %s", name)
}

// CPUGovernorsNode handles navigation for CPU frequency governors
type CPUGovernorsNode struct {
	governors map[string]int
	parent    MetricNode
	path      string
}

func NewCPUGovernorsNode(governors map[string]int, parent MetricNode, path string) *CPUGovernorsNode {
	return &CPUGovernorsNode{governors: governors, parent: parent, path: path}
}

func (node *CPUGovernorsNode) GetChildren() []MetricChild {
	var children []MetricChild
	for governor := range node.governors {
		children = append(children, MetricChild{
			Name:        governor,
			Description: fmt.Sprintf("CPU frequency governor %s", governor),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})
	return children
}

func (node *CPUGovernorsNode) GetLeafData() map[string]string {
	if node.governors == nil {
		return map[string]string{"Status": "No governor information available"}
	}

	data := map[string]string{}
	var totalCPUs int

	// Sort governors by count (descending)
	type governorCount struct {
		name  string
		count int
	}
	var governors []governorCount
	for governorName, count := range node.governors {
		governors = append(governors, governorCount{governorName, count})
		totalCPUs += count
	}
	sort.Slice(governors, func(i, j int) bool {
		return governors[i].count > governors[j].count
	})

	data["FREQUENCY GOVERNOR DISTRIBUTION"] = fmt.Sprintf("%d governor types, %s total CPUs",
		len(node.governors), humanize.Comma(int64(totalCPUs)))

	// Show each governor with percentage
	for _, gov := range governors {
		percentage := float64(gov.count) / float64(totalCPUs) * 100
		data[gov.name] = fmt.Sprintf("%s CPUs (%.1f%%)",
			humanize.Comma(int64(gov.count)), percentage)
	}

	// Add explanation
	data["About Governors"] = "Frequency governors control CPU scaling behavior (performance/powersave/ondemand/etc)"

	return data
}

func (node *CPUGovernorsNode) GetMetricType() MetricType       { return MetricsCPU }
func (node *CPUGovernorsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *CPUGovernorsNode) GetParent() MetricNode           { return node.parent }
func (node *CPUGovernorsNode) GetPath() string                 { return node.path }
func (node *CPUGovernorsNode) RequiredMetricTypes() MetricType { return MetricsCPU }
func (node *CPUGovernorsNode) GetChild(name string) (MetricNode, error) {
	if count, exists := node.governors[name]; exists {
		return NewCPUGovernorNode(name, count, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("governor not found: %s", name)
}

// CPUSummaryNode removed - summary functionality integrated into main CPUMetricsNavigator overview

// CPUModelNode represents a leaf node with CPU model details
type CPUModelNode struct {
	modelName string
	count     int
	parent    MetricNode
	path      string
}

func NewCPUModelNode(modelName string, count int, parent MetricNode, path string) *CPUModelNode {
	return &CPUModelNode{modelName: modelName, count: count, parent: parent, path: path}
}

func (node *CPUModelNode) GetChildren() []MetricChild { return []MetricChild{} }
func (node *CPUModelNode) GetLeafData() map[string]string {
	return map[string]string{
		"model_name": node.modelName,
		"count":      strconv.Itoa(node.count),
	}
}
func (node *CPUModelNode) GetMetricType() MetricType       { return MetricsCPU }
func (node *CPUModelNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *CPUModelNode) GetParent() MetricNode           { return node.parent }
func (node *CPUModelNode) GetPath() string                 { return node.path }
func (node *CPUModelNode) RequiredMetricTypes() MetricType { return MetricsCPU }
func (node *CPUModelNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("cpu model is a leaf node")
}

// CPUGovernorNode represents a leaf node with governor details
type CPUGovernorNode struct {
	governor string
	count    int
	parent   MetricNode
	path     string
}

func NewCPUGovernorNode(governor string, count int, parent MetricNode, path string) *CPUGovernorNode {
	return &CPUGovernorNode{governor: governor, count: count, parent: parent, path: path}
}

func (node *CPUGovernorNode) GetChildren() []MetricChild { return []MetricChild{} }
func (node *CPUGovernorNode) GetLeafData() map[string]string {
	return map[string]string{
		"governor": node.governor,
		"count":    strconv.Itoa(node.count),
	}
}
func (node *CPUGovernorNode) GetMetricType() MetricType       { return MetricsCPU }
func (node *CPUGovernorNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *CPUGovernorNode) GetParent() MetricNode           { return node.parent }
func (node *CPUGovernorNode) GetPath() string                 { return node.path }
func (node *CPUGovernorNode) RequiredMetricTypes() MetricType { return MetricsCPU }
func (node *CPUGovernorNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("cpu governor is a leaf node")
}
