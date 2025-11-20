//
// Copyright (c) 2015-2024 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.
//

package madmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime/metrics"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/procfs"
	"github.com/tinylib/msgp/msgp"
)

//go:generate msgp -unexported -d clearomitted -d "tag json" -d "timezone utc" -d "maps binkeys" -file $GOFILE

// MetricType is a bitfield representation of different metric types.
type MetricType uint32

// MetricsNone indicates no metrics.
const MetricsNone MetricType = 0

const (
	MetricsScanner MetricType = 1 << (iota)
	MetricsDisk
	MetricsOS
	MetricsBatchJobs
	MetricsSiteResync
	MetricNet
	MetricsMem
	MetricsCPU
	MetricsRPC
	MetricsRuntime
	MetricsAPI
	MetricsReplication
	MetricsProcess

	// MetricsAll must be last.
	// Enables all metrics.
	MetricsAll = 1<<(iota) - 1
)

// Contains returns whether m contains all of x.
func (m MetricType) Contains(x MetricType) bool {
	return m&x == x
}

// MetricFlags is a bitfield representation of different metric flags.
type MetricFlags uint64

const (
	MetricsDayStats     MetricFlags = 1 << (iota) // Include daily statistics
	MetricsByHost                                 // Aggregate metrics by host/node.
	MetricsByDisk                                 // Aggregate metrics by disk.
	MetricsLegacyDiskIO                           // Add legacy disk IO metrics.
	MetricsByDiskSet                              // Aggregate metrics by disk pool+set index.
)

// Contains returns whether m contains all of x.
func (m MetricFlags) Contains(x MetricFlags) bool {
	return m&x == x
}

// Add one or more flags to m.
func (m *MetricFlags) Add(x ...MetricFlags) {
	for _, v := range x {
		*m = *m | v
	}
}

// MetricsOptions are options provided to Metrics call.
type MetricsOptions struct {
	Type         MetricType    // Return only these metric types. Several types can be combined using |. Leave at 0 to return all.
	Flags        MetricFlags   // Flags to control returned metrics.
	N            int           // Maximum number of samples to return. 0 will return endless stream.
	Interval     time.Duration // Interval between samples. Will be rounded up to 1s.
	PoolIdx      []int         // Only include metrics for these pools. Leave empty for all.
	Hosts        []string      // Only include specified hosts. Leave empty for all.
	DrivePoolIdx []int         // Only include metrics for these drive pools. Leave empty for all.
	DriveSetIdx  []int         // Only include metrics for these drive sets (combine with PoolIdx if needed).
	Disks        []string      // Include only specific disks. Leave empty for all.
	ByJobID      string
	ByDepID      string

	// Alternative output merging.
	// Populates maps of the same name in the result.
	ByHost bool // Return individual metrics by host. Deprecated: use MetricsByHost instead.
	ByDisk bool // Return individual metrics by disk. Deprecated: use MetricsByDisk instead.
}

// DriveSetPrefix will be used to select drives from specific sets.
const (
	DriveSetPrefix  = "::drive-set::"
	DrivePoolPrefix = "::drive-pool::"
)

// Metrics makes an admin call to retrieve metrics.
// The provided function is called for each received entry.
func (adm *AdminClient) Metrics(ctx context.Context, o MetricsOptions, out func(RealtimeMetrics)) (err error) {
	path := adminAPIPrefix + "/metrics"
	q := make(url.Values)
	q.Set("types", strconv.FormatUint(uint64(o.Type), 10))
	q.Set("n", strconv.Itoa(o.N))
	q.Set("interval", o.Interval.String())
	q.Set("hosts", strings.Join(o.Hosts, ","))
	if o.ByHost {
		q.Set("by-host", "true") // Legacy flag
		o.Flags.Add(MetricsByDisk)
	}
	for _, v := range o.DriveSetIdx {
		o.Disks = append(o.Disks, fmt.Sprintf(DriveSetPrefix+"%d", v))
	}
	for _, v := range o.DrivePoolIdx {
		o.Disks = append(o.Disks, fmt.Sprintf(DrivePoolPrefix+"%d", v))
	}

	q.Set("disks", strings.Join(o.Disks, ","))
	if o.ByDisk {
		q.Set("by-disk", "true") // Legacy flag
		o.Flags.Add(MetricsByDisk)
	}
	if o.ByJobID != "" {
		q.Set("by-jobID", o.ByJobID)
	}
	if o.ByDepID != "" {
		q.Set("by-depID", o.ByDepID)
	}
	if len(o.PoolIdx) > 0 {
		str := make([]string, len(o.PoolIdx))
		for i, id := range o.PoolIdx {
			str[i] = strconv.Itoa(id)
		}
		q.Set("pool-idx", strings.Join(str, ","))
	}
	q.Set("flags", strconv.FormatUint(uint64(o.Flags), 10))

	resp, err := adm.executeMethod(ctx,
		http.MethodGet, requestData{
			customHeaders: map[string][]string{
				"Accept": {"application/vnd.msgpack"},
			},
			relPath:     path,
			queryValues: q,
		},
	)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		closeResponse(resp)
		return httpRespToErrorResponse(resp)
	}
	defer closeResponse(resp)

	// Choose decoder based on content type
	var decodeOne func(m *RealtimeMetrics) error
	switch resp.Header.Get("Content-Type") {
	case "application/vnd.msgpack":
		dec := msgp.NewReader(resp.Body)
		decodeOne = func(m *RealtimeMetrics) error {
			return m.DecodeMsg(dec)
		}
	default:
		dec := json.NewDecoder(resp.Body)
		decodeOne = func(m *RealtimeMetrics) error {
			return dec.Decode(m)
		}
	}
	for {
		var m RealtimeMetrics
		err := decodeOne(&m)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return err
		}
		out(m)
		if m.Final {
			break
		}
	}
	return nil
}

// RealtimeMetrics provides realtime metrics.
// This is intended to be expanded over time to cover more types.
type RealtimeMetrics struct {
	// Error indicates an error occurred.
	Errors []string `json:"errors,omitempty"`

	// Hosts indicates the scanned hosts
	Hosts []string `json:"hosts"`

	// Aggregated contains aggregated metrics for all hosts
	Aggregated Metrics `json:"aggregated"`

	// ByHost contains metrics for each host if requested.
	ByHost map[string]Metrics `json:"by_host,omitempty"`

	// ByDisk contains metrics for each disk if requested.
	ByDisk map[string]DiskMetric `json:"by_disk,omitempty"`

	// ByDiskSet contains disk metrics aggregated by pool+set index.
	ByDiskSet map[int]map[int]DiskMetric `json:"by_disk_set,omitempty"`

	// Final indicates whether this is the final packet and the receiver can exit.
	Final bool `json:"final"`
}

// Merge functionality:
//
// Overall rules: a.Merge(b)
//
// 1. All metrics must be accumulated and must be independent of order of merges.
// 2. If a field is not set in the other, it is not modified.
// 3. If a field is set in both, the value is merged.
// 4. Only a may be mutated.
// 5. 'a' can be the zero value.

// Merge will merge other into r.
func (r *RealtimeMetrics) Merge(other *RealtimeMetrics) {
	if other == nil {
		return
	}

	if len(other.Errors) > 0 {
		r.Errors = append(r.Errors, other.Errors...)
	}

	if r.ByHost == nil && len(other.ByHost) > 0 {
		r.ByHost = make(map[string]Metrics, len(other.ByHost))
	}
	for host, metrics := range other.ByHost {
		r.ByHost[host] = metrics
	}

	r.Hosts = append(r.Hosts, other.Hosts...)
	r.Aggregated.Merge(&other.Aggregated)
	sort.Strings(r.Hosts)

	// Gather per disk metrics
	if r.ByDisk == nil && len(other.ByDisk) > 0 {
		r.ByDisk = make(map[string]DiskMetric, len(other.ByDisk))
	}
	for disk, metrics := range other.ByDisk {
		r.ByDisk[disk] = metrics
	}
	if r.ByDiskSet == nil && len(other.ByDiskSet) > 0 {
		r.ByDiskSet = make(map[int]map[int]DiskMetric, len(other.ByDisk))
	}
	for pIdx, pool := range other.ByDiskSet {
		dstp := r.ByDiskSet[pIdx]
		if dstp == nil {
			dstp = make(map[int]DiskMetric, len(pool))
			r.ByDiskSet[pIdx] = dstp
		}
		for sIdx, disks := range pool {
			dsts := dstp[sIdx]
			dsts.Merge(&disks)
			dstp[sIdx] = dsts
		}
	}
}

// Metrics contains all metric types.
type Metrics struct {
	Scanner     *ScannerMetrics     `json:"scanner,omitempty"`
	Disk        *DiskMetric         `json:"disk,omitempty"`
	OS          *OSMetrics          `json:"os,omitempty"`
	BatchJobs   *BatchJobMetrics    `json:"batchJobs,omitempty"`
	SiteResync  *SiteResyncMetrics  `json:"siteResync,omitempty"`
	Net         *NetMetrics         `json:"net,omitempty"`
	Mem         *MemMetrics         `json:"mem,omitempty"`
	CPU         *CPUMetrics         `json:"cpu,omitempty"`
	RPC         *RPCMetrics         `json:"rpc,omitempty"`
	Go          *RuntimeMetrics     `json:"go,omitempty"`
	API         *APIMetrics         `json:"api,omitempty"`
	Replication *ReplicationMetrics `json:"replication,omitempty"`
	Process     *ProcessMetrics     `json:"process,omitempty"`
}

// Merge other into r.
func (r *Metrics) Merge(other *Metrics) {
	if other == nil {
		return
	}
	if r.Scanner == nil && other.Scanner != nil {
		r.Scanner = &ScannerMetrics{}
	}
	r.Scanner.Merge(other.Scanner)

	if r.Disk == nil && other.Disk != nil {
		r.Disk = &DiskMetric{}
	}
	r.Disk.Merge(other.Disk)

	if r.OS == nil && other.OS != nil {
		r.OS = &OSMetrics{}
	}
	r.OS.Merge(other.OS)
	if r.BatchJobs == nil && other.BatchJobs != nil {
		r.BatchJobs = &BatchJobMetrics{}
	}
	r.BatchJobs.Merge(other.BatchJobs)

	if r.SiteResync == nil && other.SiteResync != nil {
		r.SiteResync = &SiteResyncMetrics{}
	}
	r.SiteResync.Merge(other.SiteResync)

	if r.Net == nil && other.Net != nil {
		r.Net = &NetMetrics{}
	}
	r.Net.Merge(other.Net)
	if r.RPC == nil && other.RPC != nil {
		r.RPC = &RPCMetrics{}
	}
	r.RPC.Merge(other.RPC)
	if r.Go == nil && other.Go != nil {
		r.Go = &RuntimeMetrics{}
	}
	r.Go.Merge(other.Go)
	if r.API == nil && other.API != nil {
		r.API = &APIMetrics{}
	}
	r.API.Merge(other.API)
	if r.Replication == nil && other.Replication != nil {
		r.Replication = &ReplicationMetrics{}
	}
	r.Replication.Merge(other.Replication)
	if r.Mem == nil && other.Mem != nil {
		r.Mem = &MemMetrics{}
	}
	r.Mem.Merge(other.Mem)
	if r.CPU == nil && other.CPU != nil {
		r.CPU = &CPUMetrics{}
	}
	r.CPU.Merge(other.CPU)
	if r.Process == nil && other.Process != nil {
		r.Process = &ProcessMetrics{}
	}
	r.Process.Merge(other.Process)
}

// ScannerMetrics contains scanner information.

// DiskIOStats contains IO stats of a single drive
type DiskIOStats struct {
	N              int    `json:"n,omitempty"`
	ReadIOs        uint64 `json:"read_ios,omitempty"`
	ReadMerges     uint64 `json:"read_merges,omitempty"`
	ReadSectors    uint64 `json:"read_sectors,omitempty"`
	ReadTicks      uint64 `json:"read_ticks,omitempty"`
	WriteIOs       uint64 `json:"write_ios,omitempty"`
	WriteMerges    uint64 `json:"write_merges,omitempty"`
	WriteSectors   uint64 `json:"write_sectors,omitempty"`
	WriteTicks     uint64 `json:"write_ticks,omitempty"`
	CurrentIOs     uint64 `json:"current_ios,omitempty"`
	TotalTicks     uint64 `json:"total_ticks,omitempty"`
	ReqTicks       uint64 `json:"req_ticks,omitempty"`
	DiscardIOs     uint64 `json:"discard_ios,omitempty"`
	DiscardMerges  uint64 `json:"discard_merges,omitempty"`
	DiscardSectors uint64 `json:"discard_sectors,omitempty"`
	DiscardTicks   uint64 `json:"discard_ticks,omitempty"`
	FlushIOs       uint64 `json:"flush_ios,omitempty"`
	FlushTicks     uint64 `json:"flush_ticks,omitempty"`
}

type DiskIOStatsLegacy struct {
	N              int    `json:"n,omitempty"`
	ReadIOs        uint64 `json:"read_ios,omitempty"`
	ReadMerges     uint64 `json:"read_merges,omitempty"`
	ReadSectors    uint64 `json:"read_sectors,omitempty"`
	ReadTicks      uint64 `json:"read_ticks,omitempty"`
	WriteIOs       uint64 `json:"write_ios,omitempty"`
	WriteMerges    uint64 `json:"write_merges,omitempty"`
	WriteSectors   uint64 `json:"wrte_sectors,omitempty"` // note "spelling"
	WriteTicks     uint64 `json:"write_ticks,omitempty"`
	CurrentIOs     uint64 `json:"current_ios,omitempty"`
	TotalTicks     uint64 `json:"total_ticks,omitempty"`
	ReqTicks       uint64 `json:"req_ticks,omitempty"`
	DiscardIOs     uint64 `json:"discard_ios,omitempty"`
	DiscardMerges  uint64 `json:"discard_merges,omitempty"`
	DiscardSectors uint64 `json:"discard_secotrs,omitempty"` // note "spelling"
	DiscardTicks   uint64 `json:"discard_ticks,omitempty"`
	FlushIOs       uint64 `json:"flush_ios,omitempty"`
	FlushTicks     uint64 `json:"flush_ticks,omitempty"`
}

// Add 'other' to 'd'.
func (d *DiskIOStats) Add(other *DiskIOStats) {
	if other == nil {
		return
	}
	d.N += other.N
	d.ReadIOs += other.ReadIOs
	d.ReadMerges += other.ReadMerges
	d.ReadSectors += other.ReadSectors
	d.ReadTicks += other.ReadTicks
	d.WriteIOs += other.WriteIOs
	d.WriteMerges += other.WriteMerges
	d.WriteSectors += other.WriteSectors
	d.WriteTicks += other.WriteTicks
	d.CurrentIOs += other.CurrentIOs
	d.TotalTicks += other.TotalTicks
	d.ReqTicks += other.ReqTicks
	d.DiscardIOs += other.DiscardIOs
	d.DiscardMerges += other.DiscardMerges
	d.DiscardSectors += other.DiscardSectors
	d.DiscardTicks += other.DiscardTicks
	d.FlushIOs += other.FlushIOs
	d.FlushTicks += other.FlushTicks
}

type (
	SegmentedDiskActions = Segmented[DiskAction, *DiskAction]
	SegmentedDiskIO      = Segmented[DiskIOStats, *DiskIOStats]
)

// DiskMetric contains metrics for one or more disks.
type DiskMetric struct {
	// Time these metrics were collected
	CollectedAt time.Time `json:"collected"`

	// Number of disks
	NDisks int `json:"n_disks"`

	// DiskIdx will be populated if all disks in the metrics have the same drive index.
	DiskIdx *int `json:"disk_idx,omitempty"`

	// SetIdx will be populated if all disks in the metrics are part of the same set.
	SetIdx *int `json:"set_idx,omitempty"`

	// PoolIdx will be populated if all disks in the metrics are part of the same pool.
	PoolIdx *int `json:"pool_idx,omitempty"`

	// Disk states for non-ok disks.
	// See madmin.DriveState for possible values.
	State map[string]int `json:"state,omitempty"`

	// Offline disks
	Offline int `json:"offline,omitempty"`

	// Hanging - drives hanging.
	Hanging int `json:"waiting,omitempty"`

	// Healing disks
	// Deprecated, will be removed in later releases
	Healing int `json:"healing,omitempty"`

	// HealingInfo gives us a high level overview of the drives healing state
	HealingInfo *DriveHealInfo `json:"healingInfo,omitempty"`

	// Cache stats if enabled.
	Cache *CacheStats `json:"cache,omitempty"`

	// Space info.
	Space DriveSpaceInfo `json:"space"`

	// Number of accumulated operations by type.
	LifetimeOps map[string]DiskAction `json:"lifetime_ops,omitempty"`

	// Last minute statistics.
	LastMinute map[string]DiskAction `json:"last_minute,omitempty"`

	// LastDaySegmented contains the segmented metrics for the last day.
	LastDaySegmented map[string]SegmentedDiskActions `json:"last_day,omitempty"`

	// IO stats.
	// Deprecated: use io_min, io_day instead.
	IOStats *DiskIOStatsLegacy `json:"iostats,omitempty"`

	// Rolling window last minute IO stats.
	IOStatsMinute DiskIOStats `json:"io_min"`

	// Rolling window daily IO stats.
	IOStatsDay SegmentedDiskIO `json:"io_day"`
}

type DriveHealInfo struct {
	ItemsHealed uint64    `json:"itemsHealed"`
	ItemsFailed uint64    `json:"itemsFailed"`
	HealID      string    `json:"healID"`
	Finished    bool      `json:"finished"`
	Started     time.Time `json:"started"`
	Updated     time.Time `json:"updated"`
}

// DriveSpaceInfo is the space info of one or more drives.
type DriveSpaceInfo struct {
	N          int               `json:"n"`
	Free       TotalMinMaxUint64 `json:"free"`
	Used       TotalMinMaxUint64 `json:"used"`
	UsedInodes TotalMinMaxUint64 `json:"used_inodes"`
	FreeInodes TotalMinMaxUint64 `json:"free_inodes"`
}

func (d *DriveSpaceInfo) Merge(other DriveSpaceInfo) {
	d.N += other.N
	d.Free.Merge(other.Free, d.N)
	d.Used.Merge(other.Used, d.N)
	d.UsedInodes.Merge(other.UsedInodes, d.N)
	d.FreeInodes.Merge(other.FreeInodes, d.N)
}

//msgp:tuple TotalMinMaxUint64
type TotalMinMaxUint64 struct {
	Total uint64 `json:"total"`
	Min   uint64 `json:"min"`
	Max   uint64 `json:"max"`
}

func (t *TotalMinMaxUint64) SetAll(v uint64) {
	t.Total = v
	t.Min = v
	t.Max = v
}

// Merge 'other' into 't', assuming both are set.
func (t *TotalMinMaxUint64) Merge(other TotalMinMaxUint64, tCnt int) {
	t.Total += other.Total
	if tCnt == 0 || t.Min > other.Min {
		t.Min = other.Min
	}
	t.Max = max(t.Max, other.Max)
}

// Merge other into 's'.
func (d *DiskMetric) Merge(other *DiskMetric) {
	if other == nil {
		return
	}
	if d.NDisks == 0 {
		*d = *other
		return
	}
	if d.CollectedAt.Before(other.CollectedAt) {
		// Use latest timestamp
		d.CollectedAt = other.CollectedAt
	}
	// PoolIdx and SetIdx must match for all disks in the metrics.
	if d.PoolIdx == nil && d.NDisks == 0 && other.PoolIdx != nil {
		d.PoolIdx = other.PoolIdx
	} else if other.PoolIdx == nil || d.PoolIdx != nil && other.PoolIdx != nil && *d.PoolIdx != *other.PoolIdx {
		d.PoolIdx = nil
	}
	if d.SetIdx == nil && d.NDisks == 0 && other.SetIdx != nil {
		d.SetIdx = other.SetIdx
	} else if other.SetIdx == nil || d.SetIdx != nil && other.SetIdx != nil && *d.SetIdx != *other.SetIdx || d.PoolIdx == nil {
		d.SetIdx = nil
	}
	if d.DiskIdx == nil && d.NDisks == 0 && other.DiskIdx != nil {
		d.DiskIdx = other.DiskIdx
	} else if other.DiskIdx == nil || d.DiskIdx != nil && other.DiskIdx != nil && *d.DiskIdx != *other.DiskIdx || d.SetIdx == nil {
		d.DiskIdx = nil
	}
	if len(other.State) > 0 {
		if d.State == nil {
			d.State = make(map[string]int, len(other.State))
		}
		for k, v := range other.State {
			d.State[k] = d.State[k] + v
		}
	}
	d.NDisks += other.NDisks
	d.Offline += other.Offline
	d.Healing += other.Healing
	d.Hanging += other.Hanging
	if other.Cache != nil {
		if d.Cache == nil {
			d.Cache = other.Cache
		}
		d.Cache.Merge(other.Cache)
	}
	d.Space.Merge(other.Space)

	if len(other.LifetimeOps) > 0 && d.LifetimeOps == nil {
		d.LifetimeOps = make(map[string]DiskAction, len(other.LifetimeOps))
	}
	for k, v := range other.LifetimeOps {
		t := d.LifetimeOps[k]
		t.Add(&v)
		d.LifetimeOps[k] = t
	}

	if d.LastMinute == nil && len(other.LastMinute) > 0 {
		d.LastMinute = make(map[string]DiskAction, len(other.LastMinute))
	}
	for k, v := range other.LastMinute {
		t := d.LastMinute[k]
		t.Add(&v)
		d.LastMinute[k] = t
	}

	if len(other.LastDaySegmented) > 0 && d.LastDaySegmented == nil {
		d.LastDaySegmented = make(map[string]SegmentedDiskActions, len(other.LastDaySegmented))
	}
	for k, v := range other.LastDaySegmented {
		t := d.LastDaySegmented[k]
		t.Add(&v)
		d.LastDaySegmented[k] = t
	}
	if other.IOStats != nil {
		if d.IOStats == nil {
			d.IOStats = new(DiskIOStatsLegacy)
		}
		a, b := DiskIOStats(*d.IOStats), DiskIOStats(*other.IOStats)
		a.Add(&b)
		c := DiskIOStatsLegacy(a)
		d.IOStats = &c
	}
	d.IOStatsMinute.Add(&other.IOStatsMinute)
	d.IOStatsDay.Add(&other.IOStatsDay)
}

// LifetimeTotal returns the accumulated Disk metrics for all operations
func (d DiskMetric) LifetimeTotal() DiskAction {
	var res DiskAction
	for _, s := range d.LifetimeOps {
		res.Add(&s)
	}
	return res
}

// SensorMetrics aggregated sensor metrics for a single sensor key
type SensorMetrics struct {
	MinTemp         float64 `json:"min_temp"`                   // Minimum temperature seen
	MaxTemp         float64 `json:"max_temp"`                   // Maximum temperature seen
	TotalTemp       float64 `json:"total_temp"`                 // Total temperature for averaging
	Count           int     `json:"count"`                      // Number of readings
	ExceedsCritical int     `json:"exceeds_critical,omitempty"` // Count of readings exceeding critical threshold
}

// OSMetrics contains metrics for OS operations.
type OSMetrics struct {
	// Time these metrics were collected
	CollectedAt time.Time `json:"collected"`

	// Number of accumulated operations by type since server restart.
	LifeTimeOps map[string]uint64 `json:"life_time_ops,omitempty"`

	// Last minute statistics.
	LastMinute struct {
		Operations map[string]TimedAction `json:"operations,omitempty"`
	} `json:"last_minute"`

	// Aggregated temperature sensor metrics by sensor key
	Sensors map[string]SensorMetrics `json:"sensors,omitempty"`
}

// Merge other into 'o'.
func (o *OSMetrics) Merge(other *OSMetrics) {
	if other == nil {
		return
	}
	if o.CollectedAt.Before(other.CollectedAt) {
		// Use latest timestamp
		o.CollectedAt = other.CollectedAt
	}

	if len(other.LifeTimeOps) > 0 && o.LifeTimeOps == nil {
		o.LifeTimeOps = make(map[string]uint64, len(other.LifeTimeOps))
	}
	for k, v := range other.LifeTimeOps {
		total := o.LifeTimeOps[k] + v
		o.LifeTimeOps[k] = total
	}

	if o.LastMinute.Operations == nil && len(other.LastMinute.Operations) > 0 {
		o.LastMinute.Operations = make(map[string]TimedAction, len(other.LastMinute.Operations))
	}
	for k, v := range other.LastMinute.Operations {
		total := o.LastMinute.Operations[k]
		total.Merge(v)
		o.LastMinute.Operations[k] = total
	}

	// Merge sensor metrics
	if len(other.Sensors) > 0 {
		if o.Sensors == nil {
			o.Sensors = make(map[string]SensorMetrics)
		}
		for key, otherSensor := range other.Sensors {
			existing := o.Sensors[key]
			// Handle min/max
			if existing.Count == 0 {
				// First data for this sensor
				existing.MinTemp = otherSensor.MinTemp
				existing.MaxTemp = otherSensor.MaxTemp
			} else {
				if otherSensor.MinTemp < existing.MinTemp {
					existing.MinTemp = otherSensor.MinTemp
				}
				if otherSensor.MaxTemp > existing.MaxTemp {
					existing.MaxTemp = otherSensor.MaxTemp
				}
			}
			// Accumulate totals
			existing.TotalTemp += otherSensor.TotalTemp
			existing.Count += otherSensor.Count
			existing.ExceedsCritical += otherSensor.ExceedsCritical
			o.Sensors[key] = existing
		}
	}
}

// BatchJobMetrics contains metrics for batch operations
type BatchJobMetrics struct {
	// Time these metrics were collected
	CollectedAt time.Time `json:"collected"`

	// Jobs by ID.
	Jobs map[string]JobMetric
}

type JobMetric struct {
	JobID         string    `json:"jobID"`
	JobType       string    `json:"jobType"`
	StartTime     time.Time `json:"startTime"`
	LastUpdate    time.Time `json:"lastUpdate"`
	RetryAttempts int       `json:"retryAttempts"`

	Complete bool   `json:"complete"`
	Failed   bool   `json:"failed"`
	Status   string `json:"status"`

	// Specific job type data:
	Replicate *ReplicateInfo   `json:"replicate,omitempty"`
	KeyRotate *KeyRotationInfo `json:"rotation,omitempty"`
	Expired   *ExpirationInfo  `json:"expired,omitempty"`
	Catalog   *CatalogInfo     `json:"catalog,omitempty"`
}

type ReplicateInfo struct {
	// Last bucket/object batch replicated
	Bucket string `json:"lastBucket"`
	Object string `json:"lastObject"`

	// Verbose information
	Objects             int64 `json:"objects"`
	ObjectsFailed       int64 `json:"objectsFailed"`
	DeleteMarkers       int64 `json:"deleteMarkers"`
	DeleteMarkersFailed int64 `json:"deleteMarkersFailed"`
	BytesTransferred    int64 `json:"bytesTransferred"`
	BytesFailed         int64 `json:"bytesFailed"`
}

type ExpirationInfo struct {
	// Last bucket/object key rotated
	Bucket string `json:"lastBucket"`
	Object string `json:"lastObject"`

	// Verbose information
	Objects             int64 `json:"objects"`
	ObjectsFailed       int64 `json:"objectsFailed"`
	DeleteMarkers       int64 `json:"deleteMarkers"`
	DeleteMarkersFailed int64 `json:"deleteMarkersFailed"`
}

type KeyRotationInfo struct {
	// Last bucket/object key rotated
	Bucket string `json:"lastBucket"`
	Object string `json:"lastObject"`

	// Verbose information
	Objects       int64 `json:"objects"`
	ObjectsFailed int64 `json:"objectsFailed"`
}

type CatalogInfo struct {
	Bucket            string `json:"bucket"`
	LastBucketScanned string `json:"lastBucketScanned,omitempty"` // Deprecated 07/01/2025; Replaced by `bucket`
	LastObjectScanned string `json:"lastObjectScanned"`
	LastBucketMatched string `json:"lastBucketMatched,omitempty"` // Deprecated 07/01/2025; Replaced by `bucket`
	LastObjectMatched string `json:"lastObjectMatched"`

	ObjectsScannedCount uint64 `json:"objectsScannedCount"`
	ObjectsMatchedCount uint64 `json:"objectsMatchedCount"`

	// Represents the number of objects' metadata that were written to output
	// objects.
	RecordsWrittenCount uint64 `json:"recordsWrittenCount"`
	// Represents the number of output objects created.
	OutputObjectsCount uint64 `json:"outputObjectsCount"`
	// Manifest file path (part of the output of a catalog job)
	ManifestPathBucket string `json:"manifestPathBucket"`
	ManifestPathObject string `json:"manifestPathObject"`

	// Error message
	ErrorMsg string `json:"errorMsg"`

	// Used to resume catalog jobs
	LastObjectWritten string            `json:"lastObjectWritten,omitempty"`
	OutputFiles       []CatalogDataFile `json:"outputFiles,omitempty"`
}

// Merge other into 'o'.
func (o *BatchJobMetrics) Merge(other *BatchJobMetrics) {
	if other == nil || len(other.Jobs) == 0 {
		return
	}
	if o.CollectedAt.Before(other.CollectedAt) {
		// Use latest timestamp
		o.CollectedAt = other.CollectedAt
	}
	// Use latest metrics
	if o.Jobs == nil {
		o.Jobs = make(map[string]JobMetric, len(other.Jobs))
	}
	for k, v := range other.Jobs {
		if exists, ok := o.Jobs[k]; !ok || exists.LastUpdate.Before(v.LastUpdate) {
			o.Jobs[k] = v
		}
	}
}

// SiteResyncMetrics contains metrics for site resync operation
type SiteResyncMetrics struct {
	// Time these metrics were collected
	CollectedAt time.Time `json:"collected"`
	// Status of resync operation
	ResyncStatus string    `json:"resyncStatus,omitempty"`
	StartTime    time.Time `json:"startTime"`
	LastUpdate   time.Time `json:"lastUpdate"`
	NumBuckets   int64     `json:"numBuckets"`
	ResyncID     string    `json:"resyncID"`
	DeplID       string    `json:"deplID"`

	// Completed size in bytes
	ReplicatedSize int64 `json:"completedReplicationSize"`
	// Total number of objects replicated
	ReplicatedCount int64 `json:"replicationCount"`
	// Failed size in bytes
	FailedSize int64 `json:"failedReplicationSize"`
	// Total number of failed operations
	FailedCount int64 `json:"failedReplicationCount"`
	// Buckets that could not be synced
	FailedBuckets []string `json:"failedBuckets"`
	// Last bucket/object replicated.
	Bucket string `json:"bucket,omitempty"`
	Object string `json:"object,omitempty"`
}

func (o SiteResyncMetrics) Complete() bool {
	return strings.ToLower(o.ResyncStatus) == "completed"
}

// Merge other into 'o'.
func (o *SiteResyncMetrics) Merge(other *SiteResyncMetrics) {
	if other == nil {
		return
	}
	if o.CollectedAt.Before(other.CollectedAt) {
		// Use latest
		*o = *other
	}
}

type NetMetrics struct {
	// Time these metrics were collected
	CollectedAt time.Time `json:"collected"`

	// NICs contains interface -> stats map.
	Interfaces map[string]InterfaceStats

	// Deprecated: Does not merge.
	InterfaceName string `json:"interfaceName"`

	// Internode Stats.
	NetStats procfs.NetDevLine `json:"netstats"`
}

//msgp:replace procfs.NetDevLine with:procfsNetDevLine

// Merge other into 'o'.
func (n *NetMetrics) Merge(other *NetMetrics) {
	if other == nil {
		return
	}
	if n.CollectedAt.Before(other.CollectedAt) {
		// Use latest timestamp
		n.CollectedAt = other.CollectedAt
	}
	for k, v := range other.Interfaces {
		if n.Interfaces == nil {
			n.Interfaces = make(map[string]InterfaceStats, len(other.Interfaces))
		}
		n.Interfaces[k] = n.Interfaces[k].add(v)
	}
	n.NetStats = procfs.NetDevLine(procfsNetDevLine(n.NetStats).add(procfsNetDevLine(other.NetStats)))
}

// InterfaceStats contains accumulated stats for a network interface.
type InterfaceStats struct {
	N                 int `json:"n"`
	procfs.NetDevLine `json:"stats"`
}

func (n InterfaceStats) add(other InterfaceStats) InterfaceStats {
	return InterfaceStats{
		N:          n.N,
		NetDevLine: procfs.NetDevLine(procfsNetDevLine(n.NetDevLine).add(procfsNetDevLine(other.NetDevLine))),
	}
}

//msgp:replace NodeCommon with:nodeCommon

// nodeCommon - use as replacement for NodeCommon
// We do not want to give NodeCommon codegen, since it is used for embedding.
type nodeCommon struct {
	Addr  string `json:"addr"`
	Error string `json:"error,omitempty"`
}

type MemMetrics struct {
	// Time these metrics were collected
	CollectedAt time.Time `json:"collected"`

	Nodes int `json:"nodes"` // Note: Will be zero for older servers.

	Info MemInfo `json:"memInfo"`
}

// Merge other into 'm'.
func (m *MemMetrics) Merge(other *MemMetrics) {
	if other == nil {
		return
	}
	m.Nodes += other.Nodes
	if m.CollectedAt.Before(other.CollectedAt) {
		// Use latest timestamp
		m.CollectedAt = other.CollectedAt
	}
	m.Info.Merge(&other.Info)
}

// MemInfo contains system's RAM and swap information.
type MemInfo struct {
	// NodeCommon shouldn't be used since it cannot be merged.
	NodeCommon

	Total          uint64 `json:"total,omitempty"`
	Used           uint64 `json:"used,omitempty"`
	Free           uint64 `json:"free,omitempty"`
	Available      uint64 `json:"available,omitempty"`
	Shared         uint64 `json:"shared,omitempty"`
	Cache          uint64 `json:"cache,omitempty"`
	Buffers        uint64 `json:"buffer,omitempty"`
	SwapSpaceTotal uint64 `json:"swap_space_total,omitempty"`
	SwapSpaceFree  uint64 `json:"swap_space_free,omitempty"`
	// Limit will store cgroup limit if configured and
	// less than Total, otherwise same as Total
	Limit uint64 `json:"limit,omitempty"`
}

func (m *MemInfo) Merge(other *MemInfo) {
	if other == nil {
		return
	}
	if m.Total == 0 && m.Addr == "" {
		m.NodeCommon = other.NodeCommon
	} else if m.NodeCommon != other.NodeCommon {
		m.NodeCommon = NodeCommon{}
	}
	m.Total += other.Total
	m.Used += other.Used
	m.Free += other.Free
	m.Available += other.Available
	m.Shared += other.Shared
	m.Cache += other.Cache
	m.Buffers += other.Buffers
	m.SwapSpaceTotal += other.SwapSpaceTotal
	m.SwapSpaceFree += other.SwapSpaceFree
	m.Limit += other.Limit
}

//msgp:replace metrics.Float64Histogram with:localF64H

// local copy of localF64H, can be casted to/from metrics.Float64Histogram
type localF64H struct {
	Counts  []uint64  `json:"counts,omitempty"`
	Buckets []float64 `json:"buckets,omitempty"`
}

// RuntimeMetrics contains metrics for the go runtime.
// See more at https://pkg.go.dev/runtime/metrics
type RuntimeMetrics struct {
	// UintMetrics contains KindUint64 values
	UintMetrics map[string]uint64 `json:"uintMetrics,omitempty"`

	// FloatMetrics contains KindFloat64 values
	FloatMetrics map[string]float64 `json:"floatMetrics,omitempty"`

	// HistMetrics contains KindFloat64Histogram values
	HistMetrics map[string]metrics.Float64Histogram `json:"histMetrics,omitempty"`

	// N tracks the number of merged entries.
	N int `json:"n"`
}

// Merge other into 'm'.
func (m *RuntimeMetrics) Merge(other *RuntimeMetrics) {
	if m == nil || other == nil {
		return
	}
	if m.UintMetrics == nil {
		m.UintMetrics = make(map[string]uint64, len(other.UintMetrics))
	}
	if m.FloatMetrics == nil {
		m.FloatMetrics = make(map[string]float64, len(other.FloatMetrics))
	}
	if m.HistMetrics == nil {
		m.HistMetrics = make(map[string]metrics.Float64Histogram, len(other.HistMetrics))
	}
	for k, v := range other.UintMetrics {
		m.UintMetrics[k] += v
	}
	for k, v := range other.FloatMetrics {
		m.FloatMetrics[k] += v
	}
	for k, v := range other.HistMetrics {
		existing := m.HistMetrics[k]
		if len(existing.Buckets) == 0 {
			m.HistMetrics[k] = v
			continue
		}
		// TODO: Technically, I guess we may have differing buckets,
		// but they should be the same for the runtime.
		if len(existing.Buckets) == len(v.Buckets) {
			for i, count := range v.Counts {
				existing.Counts[i] += count
			}
		}
	}
	m.N += other.N
}

// Segmenter implement interface on pointers.
type Segmenter[T any] interface {
	msgp.Encodable
	msgp.Marshaler
	msgp.Decodable
	msgp.Unmarshaler
	msgp.Sizer
	Add(*T)
}

//msgp:ignore Segmented

// Segmented contains f type A metrics segmented by time.
// FirstTime must be aligned to a start time that it a multiple of Interval.
type Segmented[T any, PT interface {
	*T
	Segmenter[T]
}] struct {
	Interval  int       `json:"intervalSecs,omitempty"` // Interval covered by each segment in seconds.
	FirstTime time.Time `json:"firstTime,omitzero"`     // Timestamp of first (ie oldest) segment
	Segments  []T       `json:"segments,omitempty"`     // List of DiskAction for each segment ordered by time (oldest first).
}

// Add 'other' to 'a'.
func (s *Segmented[T, PT]) Add(other *Segmented[T, PT]) {
	if other == nil {
		return
	}
	if len(other.Segments) == 0 {
		return
	}
	if len(s.Segments) == 0 {
		// Copy slice to avoid overriding the original segment
		*s = *other
		s.Segments = append([]T{}, other.Segments...)
		return
	}

	// Intervals must match to merge safely.
	if other.Interval == 0 || s.Interval != other.Interval {
		// Cannot merge different resolutions without resampling.
		// Bail out silently as there's no error mechanism here.
		return
	}

	// Fast-path: same start time and same number of segments -> direct in-place merge.
	if s.FirstTime.Equal(other.FirstTime) && len(s.Segments) == len(other.Segments) {
		for i := range s.Segments {
			t := PT(&s.Segments[i])
			t.Add(&other.Segments[i])
		}
		return
	}
	// More complex merge...
	step := time.Duration(s.Interval) * time.Second

	// Determine the unified timeline.
	start := s.FirstTime
	if other.FirstTime.Before(start) {
		start = other.FirstTime
	}

	// Compute end times (exclusive).
	aEnd := s.FirstTime.Add(time.Duration(len(s.Segments)) * step)
	oEnd := other.FirstTime.Add(time.Duration(len(other.Segments)) * step)

	// Total number of slots to cover both series.
	totalSlots := int(oEnd.Sub(start) / step)
	if aEnd.After(oEnd) {
		totalSlots = int(aEnd.Sub(start) / step)
	}

	// Prepare the result slice with zero-value APIStats (acts as empty).
	newSegments := make([]T, totalSlots)

	// Copy/merge 's' into new slice at the proper offset.
	if s.FirstTime.After(start) {
		offset := int(s.FirstTime.Sub(start) / step)
		copy(newSegments[offset:offset+len(s.Segments)], s.Segments)
	} else {
		// s starts at 'start'
		copy(newSegments[:len(s.Segments)], s.Segments)
	}

	// Merge 'other' into new slice at the proper offset.
	otherOffset := int(other.FirstTime.Sub(start) / step)
	for i, s := range other.Segments {
		idx := otherOffset + i
		if idx < 0 || idx >= len(newSegments) {
			continue
		}
		pt := PT(&newSegments[idx])
		pt.Add(&s)
	}

	// Update receiver with merged result.
	s.FirstTime = start
	s.Segments = newSegments
}

// Total returns the total of all segments.
func (s *Segmented[T, PT]) Total() T {
	var res T
	if s == nil {
		return res
	}
	pt := PT(&res)
	for i := range s.Segments {
		pt.Add(&s.Segments[i])
	}
	// Since we are merging across APIs must reset track node count.
	return res
}
