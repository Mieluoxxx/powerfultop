// Package system samples CPU load, memory and the busiest processes.
package system

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type CoreKind int

const (
	Efficiency CoreKind = iota + 1
	Performance
)

// CoreTicks are cumulative per-core tick counters from host_processor_info(PROCESSOR_CPU_LOAD_INFO).
type CoreTicks struct {
	User, System, Idle, Nice uint64
}

// CPULoad is the share of all core time over the sampling interval, each 0…1.
type CPULoad struct {
	User, System, Idle float64
	PerCore            []float64
	// Efficiency and Performance are the cluster shares; HasEfficiency/HasPerformance say whether the Mac has that cluster.
	Efficiency, Performance       float64
	HasEfficiency, HasPerformance bool
}

func (l CPULoad) Usage() float64 { return l.User + l.System }

var IdleLoad = CPULoad{Idle: 1}

// LoadBetween is the busy share between two tick snapshots. Counters can wrap or reset across
// sleep, so a core whose counters went backwards contributes nothing instead of a huge bogus delta.
func LoadBetween(previous, current []CoreTicks, kinds []CoreKind) (CPULoad, bool) {
	if len(previous) == 0 || len(previous) != len(current) {
		return CPULoad{}, false
	}
	var user, system, idle float64
	perCore := make([]float64, 0, len(current))
	clusterBusy := map[CoreKind]float64{}
	clusterTotal := map[CoreKind]float64{}

	for i := range current {
		old, now := previous[i], current[i]
		if now.User < old.User || now.System < old.System || now.Idle < old.Idle || now.Nice < old.Nice {
			perCore = append(perCore, 0)
			continue
		}
		dUser := float64(now.User - old.User + now.Nice - old.Nice)
		dSystem := float64(now.System - old.System)
		dIdle := float64(now.Idle - old.Idle)
		dTotal := dUser + dSystem + dIdle
		user += dUser
		system += dSystem
		idle += dIdle
		share := 0.0
		if dTotal > 0 {
			share = (dUser + dSystem) / dTotal
		}
		perCore = append(perCore, share)
		if i < len(kinds) {
			clusterBusy[kinds[i]] += dUser + dSystem
			clusterTotal[kinds[i]] += dTotal
		}
	}

	total := user + system + idle
	if total <= 0 {
		return CPULoad{}, false
	}
	load := CPULoad{User: user / total, System: system / total, Idle: idle / total, PerCore: perCore}
	if t := clusterTotal[Efficiency]; t > 0 {
		load.Efficiency, load.HasEfficiency = clusterBusy[Efficiency]/t, true
	}
	if t := clusterTotal[Performance]; t > 0 {
		load.Performance, load.HasPerformance = clusterBusy[Performance]/t, true
	}
	return load, true
}

type MemoryPressure int

const (
	PressureNormal   MemoryPressure = 1
	PressureWarning  MemoryPressure = 2
	PressureCritical MemoryPressure = 4
)

// MemoryStats is Activity Monitor's breakdown: app = anonymous minus purgeable, wired, compressed;
// the rest is cached or free and reclaimable, so it does not count as used.
type MemoryStats struct {
	TotalBytes      uint64
	AppBytes        uint64
	WiredBytes      uint64
	CompressedBytes uint64
	SwapUsedBytes   uint64
	Pressure        MemoryPressure
}

func NewMemoryStats(totalBytes, internalPages, purgeablePages, wiredPages, compressorPages, pageSize, swapUsedBytes uint64, pressure MemoryPressure) MemoryStats {
	app := uint64(0)
	if internalPages > purgeablePages {
		app = internalPages - purgeablePages
	}
	return MemoryStats{
		TotalBytes:      totalBytes,
		AppBytes:        app * pageSize,
		WiredBytes:      wiredPages * pageSize,
		CompressedBytes: compressorPages * pageSize,
		SwapUsedBytes:   swapUsedBytes,
		Pressure:        pressure,
	}
}

func (m MemoryStats) UsedBytes() uint64 { return m.AppBytes + m.WiredBytes + m.CompressedBytes }

func (m MemoryStats) AvailableBytes() uint64 {
	if used := m.UsedBytes(); m.TotalBytes > used {
		return m.TotalBytes - used
	}
	return 0
}

func (m MemoryStats) UsedFraction() float64 {
	if m.TotalBytes == 0 {
		return 0
	}
	return float64(m.UsedBytes()) / float64(m.TotalBytes)
}

// ProcessUsage is a percent of one core in CPU lists, bytes in memory lists.
type ProcessUsage struct {
	PID   int32
	Name  string
	Value float64
}

// ParseProcessCPU parses `ps -Aceo pid=,pcpu=,comm= -r`; comm may contain spaces.
func ParseProcessCPU(output string, limit int) []ProcessUsage {
	var list []ProcessUsage
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		pid, err1 := strconv.ParseInt(parts[0], 10, 32)
		cpu, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		// The name is everything after the second field, spaces included.
		rest := strings.TrimLeft(line, " \t")
		rest = strings.TrimLeft(rest[len(parts[0]):], " \t")
		rest = strings.TrimLeft(rest[len(parts[1]):], " \t")
		list = append(list, ProcessUsage{PID: int32(pid), Name: strings.TrimRight(rest, " \t\r"), Value: cpu})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Value > list[j].Value })
	if len(list) > limit {
		list = list[:limit]
	}
	return list
}

// ParseProcessMemory parses `top -l 1 -o mem -stats pid,command,mem`; MEM is the physical footprint
// Activity Monitor shows (e.g. "1461M", "583K", "1.2G", with an optional trailing "+"/"-" trend marker).
func ParseProcessMemory(output string, limit int) []ProcessUsage {
	rows := strings.Split(output, "\n")
	header := -1
	for i, row := range rows {
		if strings.HasPrefix(row, "PID") {
			header = i
			break
		}
	}
	if header < 0 {
		return nil
	}
	var list []ProcessUsage
	for _, line := range rows[header+1:] {
		trimmed := strings.Trim(line, " \t\r")
		first, last := strings.IndexByte(trimmed, ' '), strings.LastIndexByte(trimmed, ' ')
		if first < 0 || last < 0 {
			continue
		}
		pid, err := strconv.ParseInt(trimmed[:first], 10, 32)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(trimmed[first:last])
		bytes, ok := ParseTopMemory(trimmed[last+1:])
		if !ok || name == "" {
			continue
		}
		list = append(list, ProcessUsage{PID: int32(pid), Name: name, Value: bytes})
		if len(list) == limit {
			break
		}
	}
	return list
}

// ParseProcessNames parses `ps -Aceo pid=,comm=` into a pid → full executable name map.
func ParseProcessNames(output string) map[int32]string {
	names := map[int32]string{}
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		space := strings.IndexByte(trimmed, ' ')
		if space < 0 {
			continue
		}
		pid, err := strconv.ParseInt(trimmed[:space], 10, 32)
		if err != nil {
			continue
		}
		if name := strings.TrimSpace(trimmed[space:]); name != "" {
			names[int32(pid)] = name
		}
	}
	return names
}

func ParseTopMemory(token string) (float64, bool) {
	text := strings.Trim(token, "+- ")
	if text == "" {
		return 0, false
	}
	multipliers := map[byte]float64{'B': 1, 'K': 1024, 'M': 1 << 20, 'G': 1 << 30, 'T': 1 << 40}
	if m, ok := multipliers[text[len(text)-1]]; ok {
		v, err := strconv.ParseFloat(text[:len(text)-1], 64)
		return v * m, err == nil
	}
	v, err := strconv.ParseFloat(text, 64)
	return v, err == nil
}

// FormatBytes follows ByteCountFormatter's memory style: binary units, more decimals for larger
// units, trailing zeros dropped ("583 KB", "1.4 MB", "42.46 GB", "128 GB").
func FormatBytes(bytes float64) string {
	units := []struct {
		name     string
		size     float64
		decimals int
	}{{"TB", 1 << 40, 2}, {"GB", 1 << 30, 2}, {"MB", 1 << 20, 1}, {"KB", 1 << 10, 0}}
	for _, u := range units {
		if math.Abs(bytes) >= u.size {
			s := strconv.FormatFloat(bytes/u.size, 'f', u.decimals, 64)
			if strings.Contains(s, ".") {
				s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
			}
			return s + " " + u.name
		}
	}
	if bytes == 0 {
		return "Zero KB"
	}
	return fmt.Sprintf("%.0f bytes", bytes)
}
