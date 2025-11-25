package madmin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
)

//go:generate msgp  -d clearomitted -d "tag json" -d "timezone utc" -d "maps binkeys" -file $GOFILE

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

func (node *CPUMetricsNavigator) ShouldPauseRefresh() bool {
	return false
}

// NewCPUMetricsNavigator creates a new CPU metrics navigator
func NewCPUMetricsNavigator(cpu *CPUMetrics, parent MetricNode, path string) *CPUMetricsNavigator {
	return &CPUMetricsNavigator{cpu: cpu, parent: parent, path: path}
}

func (node *CPUMetricsNavigator) GetChildren() []MetricChild {
	return []MetricChild{}
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

	// CPU Times Breakdown
	if node.cpu.TimesStat != nil {
		times := node.cpu.TimesStat

		// Calculate total time for percentages
		totalTime := times.User + times.System + times.Idle + times.Nice +
			times.Iowait + times.Irq + times.Softirq + times.Steal +
			times.Guest + times.GuestNice

		if totalTime > 0 {
			data["User Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.User/totalTime)*100, times.User)
			data["System Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.System/totalTime)*100, times.System)
			data["Idle Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.Idle/totalTime)*100, times.Idle)

			// Only show non-zero times to keep display clean
			if times.Nice > 0 {
				data["Nice Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.Nice/totalTime)*100, times.Nice)
			}
			if times.Iowait > 0 {
				data["IO Wait Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.Iowait/totalTime)*100, times.Iowait)
			}
			if times.Irq > 0 {
				data["IRQ Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.Irq/totalTime)*100, times.Irq)
			}
			if times.Softirq > 0 {
				data["Soft IRQ Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.Softirq/totalTime)*100, times.Softirq)
			}
			if times.Steal > 0 {
				data["Steal Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.Steal/totalTime)*100, times.Steal)
			}
			if times.Guest > 0 {
				data["Guest Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.Guest/totalTime)*100, times.Guest)
			}
			if times.GuestNice > 0 {
				data["Guest Nice Time"] = fmt.Sprintf("%.1f%% (%.2fs)", (times.GuestNice/totalTime)*100, times.GuestNice)
			}
		}
	}

	// Load Averages
	if node.cpu.LoadStat != nil {
		load := node.cpu.LoadStat
		data["Load 1min"] = fmt.Sprintf("%.2f", load.Load1)
		data["Load 5min"] = fmt.Sprintf("%.2f", load.Load5)
		data["Load 15min"] = fmt.Sprintf("%.2f", load.Load15)
	}

	// Frequency Information
	if node.cpu.FreqStatsCount > 0 {
		currentFreq := node.cpu.TotalCurrentFreq / uint64(node.cpu.FreqStatsCount)
		data["Current Frequency"] = formatFrequency(currentFreq)

		if node.cpu.MaxCPUInfoFreq > 0 {
			utilization := float64(currentFreq) / float64(node.cpu.MaxCPUInfoFreq) * 100
			data["Frequency Utilization"] = fmt.Sprintf("%.1f%%", utilization)
		}

		if node.cpu.TotalScalingCurrentFreq > 0 {
			scalingFreq := node.cpu.TotalScalingCurrentFreq / uint64(node.cpu.FreqStatsCount)
			data["Scaling Frequency"] = formatFrequency(scalingFreq)
		}
	}

	// CPU Models Distribution
	if len(node.cpu.CPUByModel) > 0 {
		totalCPUs := 0
		for _, count := range node.cpu.CPUByModel {
			totalCPUs += count
		}

		// Sort models by count (descending)
		type modelStat struct {
			name  string
			count int
		}
		var models []modelStat
		for name, count := range node.cpu.CPUByModel {
			models = append(models, modelStat{name, count})
		}
		sort.Slice(models, func(i, j int) bool {
			return models[i].count > models[j].count
		})

		// Show top 3 models
		for i, model := range models {
			if i >= 3 {
				break
			}
			percentage := float64(model.count) / float64(totalCPUs) * 100
			key := fmt.Sprintf("CPU Model %d", i+1)
			// Truncate long model names
			name := model.name
			if len(name) > 40 {
				name = name[:37] + "..."
			}
			data[key] = fmt.Sprintf("%s (%d CPUs, %.1f%%)", name, model.count, percentage)
		}

		if len(models) > 3 {
			data["Other Models"] = fmt.Sprintf("%d additional models", len(models)-3)
		}
	}

	// Governor Distribution
	if len(node.cpu.GovernorFreq) > 0 {
		totalCPUs := 0
		for _, count := range node.cpu.GovernorFreq {
			totalCPUs += count
		}

		// Sort governors by count (descending)
		type govStat struct {
			name  string
			count int
		}
		var governors []govStat
		for name, count := range node.cpu.GovernorFreq {
			governors = append(governors, govStat{name, count})
		}
		sort.Slice(governors, func(i, j int) bool {
			return governors[i].count > governors[j].count
		})

		// Show all governors since there are usually only a few
		for i, gov := range governors {
			percentage := float64(gov.count) / float64(totalCPUs) * 100
			key := fmt.Sprintf("Governor %s", gov.name)
			data[key] = fmt.Sprintf("%d CPUs (%.1f%%)", gov.count, percentage)

			if i >= 3 { // Limit to avoid clutter
				break
			}
		}
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
	return nil, fmt.Errorf("no children available - all CPU data shown in main display")
}
