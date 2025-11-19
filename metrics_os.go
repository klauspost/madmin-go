package madmin

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// formatNumber formats large numbers with thousand separators
func formatNumber(n uint64) string {
	s := strconv.FormatUint(n, 10)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(r)
	}
	return result.String()
}

// formatBytes formats bytes in human readable format
func formatBytes(bytes uint64) string {
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
		return fmt.Sprintf("%.0f %s", size, units[unitIndex])
	}
	return fmt.Sprintf("%.2f %s", size, units[unitIndex])
}

// OSMetricsNavigator provides navigation for OS metrics
type OSMetricsNavigator struct {
	os     *OSMetrics
	parent MetricNode
	path   string
}

// NewOSMetricsNavigator creates a new OS metrics navigator
func NewOSMetricsNavigator(os *OSMetrics, parent MetricNode, path string) *OSMetricsNavigator {
	return &OSMetricsNavigator{os: os, parent: parent, path: path}
}

func (node *OSMetricsNavigator) GetChildren() []MetricChild {
	return []MetricChild{
		{Name: "lifetime_ops", Description: "Accumulated operations since server start"},
		{Name: "last_minute", Description: "Last minute operation statistics"},
		{Name: "sensors", Description: "Temperature sensor metrics"},
	}
}

func (node *OSMetricsNavigator) GetLeafData() map[string]string {
	if node.os == nil {
		return map[string]string{"Status": "OS metrics not available"}
	}

	data := map[string]string{
		"Collected At":           node.os.CollectedAt.Format("2006-01-02 15:04:05"),
		"Operation Types":        strconv.Itoa(len(node.os.LifeTimeOps)),
		"Last Minute Operations": strconv.Itoa(len(node.os.LastMinute.Operations)),
		"Temperature Sensors":    strconv.Itoa(len(node.os.Sensors)),
	}

	// Add totals for lifetime ops
	var totalLifetimeOps uint64
	for _, count := range node.os.LifeTimeOps {
		totalLifetimeOps += count
	}
	data["Total Lifetime Operations"] = fmt.Sprintf("%s", formatNumber(totalLifetimeOps))

	return data
}

func (node *OSMetricsNavigator) GetMetricType() MetricType {
	return MetricsOS
}

func (node *OSMetricsNavigator) GetMetricFlags() MetricFlags {
	return 0
}

func (node *OSMetricsNavigator) GetParent() MetricNode {
	return node.parent
}

func (node *OSMetricsNavigator) GetPath() string {
	return node.path
}

func (node *OSMetricsNavigator) RequiredMetricTypes() MetricType {
	return MetricsOS
}

func (node *OSMetricsNavigator) ShouldPauseRefresh() bool {
	return false
}

func (node *OSMetricsNavigator) GetChild(name string) (MetricNode, error) {
	switch name {
	case "lifetime_ops":
		return NewOSLifetimeOpsNode(node.os.LifeTimeOps, node, fmt.Sprintf("%s/lifetime_ops", node.path)), nil
	case "last_minute":
		return NewOSLastMinuteNode(node.os.LastMinute.Operations, node, fmt.Sprintf("%s/last_minute", node.path)), nil
	case "sensors":
		return NewOSSensorsNode(node.os.Sensors, node, fmt.Sprintf("%s/sensors", node.path)), nil
	default:
		return nil, fmt.Errorf("child not found: %s", name)
	}
}

// OSLifetimeOpsNode handles navigation for OS lifetime operations
type OSLifetimeOpsNode struct {
	ops    map[string]uint64
	parent MetricNode
	path   string
}

func NewOSLifetimeOpsNode(ops map[string]uint64, parent MetricNode, path string) *OSLifetimeOpsNode {
	return &OSLifetimeOpsNode{ops: ops, parent: parent, path: path}
}

func (node *OSLifetimeOpsNode) GetChildren() []MetricChild {
	var children []MetricChild
	for opType := range node.ops {
		children = append(children, MetricChild{
			Name:        opType,
			Description: fmt.Sprintf("Count for OS operation type %s", opType),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})
	return children
}

func (node *OSLifetimeOpsNode) GetLeafData() map[string]string {
	if node.ops == nil {
		return map[string]string{"Operation Types": "0", "Total Operations": "0"}
	}

	data := map[string]string{
		"Operation Types": strconv.Itoa(len(node.ops)),
	}
	var total uint64
	for opType, count := range node.ops {
		// Convert technical operation names to display names
		displayName := strings.ReplaceAll(strings.Title(strings.ReplaceAll(opType, "_", " ")), "Api", "API")
		data[displayName] = formatNumber(count)
		total += count
	}
	data["Total Operations"] = formatNumber(total)
	return data
}

func (node *OSLifetimeOpsNode) GetMetricType() MetricType       { return MetricsOS }
func (node *OSLifetimeOpsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *OSLifetimeOpsNode) GetParent() MetricNode           { return node.parent }
func (node *OSLifetimeOpsNode) GetPath() string                 { return node.path }
func (node *OSLifetimeOpsNode) RequiredMetricTypes() MetricType { return MetricsOS }

func (node *OSLifetimeOpsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *OSLifetimeOpsNode) GetChild(name string) (MetricNode, error) {
	if count, exists := node.ops[name]; exists {
		return NewOSOpCountNode(name, count, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("operation type not found: %s", name)
}

// OSLastMinuteNode handles navigation for OS last minute operations
type OSLastMinuteNode struct {
	operations map[string]TimedAction
	parent     MetricNode
	path       string
}

func NewOSLastMinuteNode(operations map[string]TimedAction, parent MetricNode, path string) *OSLastMinuteNode {
	return &OSLastMinuteNode{operations: operations, parent: parent, path: path}
}

func (node *OSLastMinuteNode) GetChildren() []MetricChild {
	var children []MetricChild
	for opType := range node.operations {
		children = append(children, MetricChild{
			Name:        opType,
			Description: fmt.Sprintf("Timing statistics for %s operations", opType),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})
	return children
}

func (node *OSLastMinuteNode) GetLeafData() map[string]string {
	if node.operations == nil {
		return map[string]string{"Operation Types": "0", "Total Operations": "0", "Total Time": "0 ms"}
	}

	data := map[string]string{
		"Operation Types": strconv.Itoa(len(node.operations)),
	}
	var totalCount, totalTime uint64
	for opType, action := range node.operations {
		displayName := strings.ReplaceAll(strings.Title(strings.ReplaceAll(opType, "_", " ")), "Api", "API")
		data[displayName+" Operations"] = formatNumber(action.Count)
		data[displayName+" Time"] = fmt.Sprintf("%.2f ms", float64(action.AccTime)/1000000) // Convert nanoseconds to milliseconds
		totalCount += action.Count
		totalTime += action.AccTime
	}
	data["Total Operations"] = formatNumber(totalCount)
	data["Total Time"] = fmt.Sprintf("%.2f ms", float64(totalTime)/1000000)
	return data
}

func (node *OSLastMinuteNode) GetMetricType() MetricType       { return MetricsOS }
func (node *OSLastMinuteNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *OSLastMinuteNode) GetParent() MetricNode           { return node.parent }
func (node *OSLastMinuteNode) GetPath() string                 { return node.path }
func (node *OSLastMinuteNode) RequiredMetricTypes() MetricType { return MetricsOS }

func (node *OSLastMinuteNode) ShouldPauseRefresh() bool {
	return false
}
func (node *OSLastMinuteNode) GetChild(name string) (MetricNode, error) {
	if action, exists := node.operations[name]; exists {
		return NewOSTimedActionNode(name, &action, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("operation not found: %s", name)
}

// OSSensorsNode handles navigation for OS temperature sensors
type OSSensorsNode struct {
	sensors map[string]SensorMetrics
	parent  MetricNode
	path    string
}

func NewOSSensorsNode(sensors map[string]SensorMetrics, parent MetricNode, path string) *OSSensorsNode {
	return &OSSensorsNode{sensors: sensors, parent: parent, path: path}
}

func (node *OSSensorsNode) GetChildren() []MetricChild {
	var children []MetricChild
	for sensorKey := range node.sensors {
		children = append(children, MetricChild{
			Name:        sensorKey,
			Description: fmt.Sprintf("Temperature metrics for sensor %s", sensorKey),
		})
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})
	return children
}

func (node *OSSensorsNode) GetLeafData() map[string]string {
	if node.sensors == nil {
		return map[string]string{"Temperature Sensors": "0"}
	}

	data := map[string]string{
		"Temperature Sensors": strconv.Itoa(len(node.sensors)),
	}

	// Calculate aggregate sensor statistics
	var totalReadings, totalExceedsCritical int
	var minTempOverall, maxTempOverall, totalTempOverall float64
	first := true

	for sensorKey, sensor := range node.sensors {
		displayKey := strings.Title(strings.ReplaceAll(sensorKey, "_", " "))
		data[displayKey+" Readings"] = formatNumber(uint64(sensor.Count))
		data[displayKey+" Min Temp"] = fmt.Sprintf("%.1f°C", sensor.MinTemp)
		data[displayKey+" Max Temp"] = fmt.Sprintf("%.1f°C", sensor.MaxTemp)
		if sensor.Count > 0 {
			data[displayKey+" Avg Temp"] = fmt.Sprintf("%.1f°C", sensor.TotalTemp/float64(sensor.Count))
		}
		if sensor.ExceedsCritical > 0 {
			data[displayKey+" Critical Events"] = formatNumber(uint64(sensor.ExceedsCritical))
		}

		totalReadings += sensor.Count
		totalExceedsCritical += sensor.ExceedsCritical
		totalTempOverall += sensor.TotalTemp

		if first {
			minTempOverall = sensor.MinTemp
			maxTempOverall = sensor.MaxTemp
			first = false
		} else {
			if sensor.MinTemp < minTempOverall {
				minTempOverall = sensor.MinTemp
			}
			if sensor.MaxTemp > maxTempOverall {
				maxTempOverall = sensor.MaxTemp
			}
		}
	}

	if totalReadings > 0 {
		data["Total Readings"] = formatNumber(uint64(totalReadings))
		data["Overall Min Temp"] = fmt.Sprintf("%.1f°C", minTempOverall)
		data["Overall Max Temp"] = fmt.Sprintf("%.1f°C", maxTempOverall)
		data["Overall Avg Temp"] = fmt.Sprintf("%.1f°C", totalTempOverall/float64(totalReadings))
		if totalExceedsCritical > 0 {
			data["Total Critical Events"] = formatNumber(uint64(totalExceedsCritical))
		}
	}

	return data
}

func (node *OSSensorsNode) GetMetricType() MetricType       { return MetricsOS }
func (node *OSSensorsNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *OSSensorsNode) GetParent() MetricNode           { return node.parent }
func (node *OSSensorsNode) GetPath() string                 { return node.path }
func (node *OSSensorsNode) RequiredMetricTypes() MetricType { return MetricsOS }

func (node *OSSensorsNode) ShouldPauseRefresh() bool {
	return false
}
func (node *OSSensorsNode) GetChild(name string) (MetricNode, error) {
	if sensor, exists := node.sensors[name]; exists {
		return NewOSSensorNode(name, &sensor, node, fmt.Sprintf("%s/%s", node.path, name)), nil
	}
	return nil, fmt.Errorf("sensor not found: %s", name)
}

// OSOpCountNode represents a leaf node with operation count
type OSOpCountNode struct {
	opType string
	count  uint64
	parent MetricNode
	path   string
}

func NewOSOpCountNode(opType string, count uint64, parent MetricNode, path string) *OSOpCountNode {
	return &OSOpCountNode{opType: opType, count: count, parent: parent, path: path}
}

func (node *OSOpCountNode) GetChildren() []MetricChild { return []MetricChild{} }
func (node *OSOpCountNode) GetLeafData() map[string]string {
	displayName := strings.ReplaceAll(strings.Title(strings.ReplaceAll(node.opType, "_", " ")), "Api", "API")
	return map[string]string{
		"Operation Type": displayName,
		"Total Count":    formatNumber(node.count),
	}
}
func (node *OSOpCountNode) GetMetricType() MetricType       { return MetricsOS }
func (node *OSOpCountNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *OSOpCountNode) GetParent() MetricNode           { return node.parent }
func (node *OSOpCountNode) GetPath() string                 { return node.path }
func (node *OSOpCountNode) RequiredMetricTypes() MetricType { return MetricsOS }

func (node *OSOpCountNode) ShouldPauseRefresh() bool {
	return false
}
func (node *OSOpCountNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("operation count is a leaf node")
}

// OSTimedActionNode represents a leaf node with timed action details
type OSTimedActionNode struct {
	actionType string
	action     *TimedAction
	parent     MetricNode
	path       string
}

func NewOSTimedActionNode(actionType string, action *TimedAction, parent MetricNode, path string) *OSTimedActionNode {
	return &OSTimedActionNode{actionType: actionType, action: action, parent: parent, path: path}
}

func (node *OSTimedActionNode) GetChildren() []MetricChild { return []MetricChild{} }
func (node *OSTimedActionNode) GetLeafData() map[string]string {
	displayName := strings.ReplaceAll(strings.Title(strings.ReplaceAll(node.actionType, "_", " ")), "Api", "API")

	if node.action == nil {
		return map[string]string{
			"Action Type":    displayName,
			"Operations":     "0",
			"Total Time":     "0 ms",
			"Min Time":       "0 ms",
			"Max Time":       "0 ms",
			"Data Processed": "0 B",
		}
	}

	return map[string]string{
		"Action Type":    displayName,
		"Operations":     formatNumber(node.action.Count),
		"Total Time":     fmt.Sprintf("%.2f ms", float64(node.action.AccTime)/1000000),
		"Min Time":       fmt.Sprintf("%.2f ms", float64(node.action.MinTime)/1000000),
		"Max Time":       fmt.Sprintf("%.2f ms", float64(node.action.MaxTime)/1000000),
		"Data Processed": formatBytes(node.action.Bytes),
	}
}
func (node *OSTimedActionNode) GetMetricType() MetricType       { return MetricsOS }
func (node *OSTimedActionNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *OSTimedActionNode) GetParent() MetricNode           { return node.parent }
func (node *OSTimedActionNode) GetPath() string                 { return node.path }
func (node *OSTimedActionNode) RequiredMetricTypes() MetricType { return MetricsOS }

func (node *OSTimedActionNode) ShouldPauseRefresh() bool {
	return false
}
func (node *OSTimedActionNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("timed action is a leaf node")
}

// OSSensorNode represents a leaf node with sensor details
type OSSensorNode struct {
	sensorKey string
	sensor    *SensorMetrics
	parent    MetricNode
	path      string
}

func NewOSSensorNode(sensorKey string, sensor *SensorMetrics, parent MetricNode, path string) *OSSensorNode {
	return &OSSensorNode{sensorKey: sensorKey, sensor: sensor, parent: parent, path: path}
}

func (node *OSSensorNode) GetChildren() []MetricChild { return []MetricChild{} }
func (node *OSSensorNode) GetLeafData() map[string]string {
	displayKey := strings.Title(strings.ReplaceAll(node.sensorKey, "_", " "))

	if node.sensor == nil {
		return map[string]string{
			"Sensor Name":     displayKey,
			"Readings":        "0",
			"Min Temperature": "0°C",
			"Max Temperature": "0°C",
			"Avg Temperature": "0°C",
			"Critical Events": "0",
		}
	}

	avgTemp := float64(0)
	if node.sensor.Count > 0 {
		avgTemp = node.sensor.TotalTemp / float64(node.sensor.Count)
	}

	data := map[string]string{
		"Sensor Name":     displayKey,
		"Readings":        formatNumber(uint64(node.sensor.Count)),
		"Min Temperature": fmt.Sprintf("%.1f°C", node.sensor.MinTemp),
		"Max Temperature": fmt.Sprintf("%.1f°C", node.sensor.MaxTemp),
		"Avg Temperature": fmt.Sprintf("%.1f°C", avgTemp),
	}

	if node.sensor.ExceedsCritical > 0 {
		data["Critical Events"] = formatNumber(uint64(node.sensor.ExceedsCritical))
	}

	return data
}
func (node *OSSensorNode) GetMetricType() MetricType       { return MetricsOS }
func (node *OSSensorNode) GetMetricFlags() MetricFlags     { return 0 }
func (node *OSSensorNode) GetParent() MetricNode           { return node.parent }
func (node *OSSensorNode) GetPath() string                 { return node.path }
func (node *OSSensorNode) RequiredMetricTypes() MetricType { return MetricsOS }

func (node *OSSensorNode) ShouldPauseRefresh() bool {
	return false
}
func (node *OSSensorNode) GetChild(name string) (MetricNode, error) {
	return nil, fmt.Errorf("sensor is a leaf node")
}
