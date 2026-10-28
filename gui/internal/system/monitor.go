//go:build darwin

package system

import (
	"bytes"
	"os/exec"
	"sort"
	"sync"
	"time"

	"powerfultop/internal/notify"
	"powerfultop/internal/system/sysapi"
)

const (
	refreshInterval    = 2 * time.Second
	cpuListInterval    = 5 * time.Second
	memoryListInterval = 10 * time.Second
	processListLimit   = 8
)

// Snapshot is everything the system page and the popover show.
type Snapshot struct {
	CPU         CPULoad
	Memory      *MemoryStats
	LoadAverage []float64
	BootTime    time.Time
	TopCPU      []ProcessUsage
	TopMemory   []ProcessUsage
	CoreKinds   []CoreKind
}

// Monitor samples every refreshInterval while started.
type Monitor struct {
	mu      sync.Mutex
	snap    Snapshot
	changes notify.Listeners

	previous       []CoreTicks
	stop           chan struct{}
	processActive  bool
	lastCPUList    time.Time
	lastMemoryList time.Time
	listing        sync.Mutex
}

func NewMonitor() *Monitor {
	return &Monitor{snap: Snapshot{CPU: IdleLoad, CoreKinds: readCoreKinds()}}
}

func (m *Monitor) Start() {
	m.mu.Lock()
	if m.stop != nil {
		m.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	m.stop = stop
	m.snap.BootTime = readBootTime()
	m.previous = readTicks()
	m.mu.Unlock()

	m.refresh()
	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				m.refresh()
			}
		}
	}()
}

func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stop != nil {
		close(m.stop)
		m.stop = nil
	}
}

func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.snap
	s.CPU.PerCore = append([]float64(nil), s.CPU.PerCore...)
	s.LoadAverage = append([]float64(nil), s.LoadAverage...)
	s.TopCPU = append([]ProcessUsage(nil), s.TopCPU...)
	s.TopMemory = append([]ProcessUsage(nil), s.TopMemory...)
	s.CoreKinds = append([]CoreKind(nil), s.CoreKinds...)
	if s.Memory != nil {
		mem := *s.Memory
		s.Memory = &mem
	}
	return s
}

func (m *Monitor) OnChange(fn func()) {
	m.changes.Add(fn)
}

// SetProcessSamplingActive turns the process lists on while they are on screen: they cost a
// ps/top spawn (top uses ~0.6 s of CPU), so they are not sampled otherwise.
func (m *Monitor) SetProcessSamplingActive(active bool) {
	m.mu.Lock()
	was := m.processActive
	m.processActive = active
	m.mu.Unlock()
	if active && !was {
		go m.sampleProcesses(true)
	}
}

func (m *Monitor) refresh() {
	ticks := readTicks()
	memory, hasMemory := readMemory()
	loadAverage, hasLoad := sysapi.LoadAverage()

	m.mu.Lock()
	if load, ok := LoadBetween(m.previous, ticks, m.snap.CoreKinds); ok {
		m.snap.CPU = load
	}
	m.previous = ticks
	if hasMemory {
		m.snap.Memory = &memory
	}
	if hasLoad {
		m.snap.LoadAverage = loadAverage
	}
	active := m.processActive
	m.mu.Unlock()

	if active {
		go m.sampleProcesses(false)
	}
	m.changes.Notify()
}

func (m *Monitor) sampleProcesses(force bool) {
	// One spawn at a time: a slow top must not pile up behind the next tick.
	if !m.listing.TryLock() {
		return
	}
	defer m.listing.Unlock()
	now := time.Now()
	m.mu.Lock()
	wantCPU := force || now.Sub(m.lastCPUList) >= cpuListInterval
	wantMemory := force || now.Sub(m.lastMemoryList) >= memoryListInterval
	if wantCPU {
		m.lastCPUList = now
	}
	if wantMemory {
		m.lastMemoryList = now
	}
	m.mu.Unlock()

	if wantCPU {
		// ps is setuid root, so it sees WindowServer and other system processes we cannot query.
		list := ParseProcessCPU(run("/bin/ps", "-Aceo", "pid=,pcpu=,comm=", "-r"), processListLimit)
		m.mu.Lock()
		m.snap.TopCPU = list
		m.mu.Unlock()
	}
	if wantMemory {
		list := ParseProcessMemory(run("/usr/bin/top", "-l", "1", "-o", "mem", "-n", "8", "-stats", "pid,command,mem"), processListLimit)
		// top truncates COMMAND to 16 characters; ps reports the full executable name.
		names := ParseProcessNames(run("/bin/ps", "-Aceo", "pid=,comm="))
		for i := range list {
			if name, ok := names[list[i].PID]; ok {
				list[i].Name = name
			}
		}
		m.mu.Lock()
		m.snap.TopMemory = list
		m.mu.Unlock()
	}
	if wantCPU || wantMemory {
		m.changes.Notify()
	}
}

func run(path string, args ...string) string {
	var out bytes.Buffer
	cmd := exec.Command(path, args...)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil && out.Len() == 0 {
		return ""
	}
	return out.String()
}

func readTicks() []CoreTicks {
	raw, ok := sysapi.ProcessorLoadTicks()
	if !ok {
		return nil
	}
	// CPU_STATE_USER, SYSTEM, IDLE, NICE
	ticks := make([]CoreTicks, len(raw))
	for i, r := range raw {
		ticks[i] = CoreTicks{User: uint64(r[0]), System: uint64(r[1]), Idle: uint64(r[2]), Nice: uint64(r[3])}
	}
	return ticks
}

// readCoreKinds follows the device tree, whose logical CPU order matches the kernel's; each cpuN
// node carries cluster-type "E" or "P".
func readCoreKinds() []CoreKind {
	cpus := sysapi.EntryFromPath("IODeviceTree:/cpus")
	if cpus == 0 {
		return nil
	}
	defer cpus.Release()
	iterator := cpus.Children("IODeviceTree")
	if iterator == 0 {
		return nil
	}
	defer iterator.Release()
	type core struct {
		id   uint32
		kind CoreKind
	}
	var cores []core
	for child := iterator.Next(); child != 0; child = iterator.Next() {
		clusterType, ok1 := child.Property("cluster-type")
		cpuID, ok2 := child.Property("cpu-id")
		child.Release()
		typeBytes, ok3 := clusterType.([]byte)
		idBytes, ok4 := cpuID.([]byte)
		if !ok1 || !ok2 || !ok3 || !ok4 || len(idBytes) < 4 {
			continue
		}
		kind := Performance
		if string(bytes.TrimRight(typeBytes, "\x00")) == "E" {
			kind = Efficiency
		}
		id := uint32(idBytes[0]) | uint32(idBytes[1])<<8 | uint32(idBytes[2])<<16 | uint32(idBytes[3])<<24
		cores = append(cores, core{id, kind})
	}
	sort.Slice(cores, func(i, j int) bool { return cores[i].id < cores[j].id })
	kinds := make([]CoreKind, len(cores))
	for i, c := range cores {
		kinds[i] = c.kind
	}
	return kinds
}

func readMemory() (MemoryStats, bool) {
	stats, ok := sysapi.HostVMInfo64()
	if !ok {
		return MemoryStats{}, false
	}
	var total uint64
	var pageSize int64
	if sysapi.Sysctl("hw.memsize", &total) != nil || sysapi.Sysctl("hw.pagesize", &pageSize) != nil {
		return MemoryStats{}, false
	}
	// struct xsw_usage
	var swap struct {
		Total, Avail, Used uint64
		PageSize           uint32
		Encrypted          int32
	}
	_ = sysapi.Sysctl("vm.swapusage", &swap)
	level := int32(PressureNormal)
	_ = sysapi.Sysctl("kern.memorystatus_vm_pressure_level", &level)
	pressure := MemoryPressure(level)
	if pressure != PressureWarning && pressure != PressureCritical {
		pressure = PressureNormal
	}
	return NewMemoryStats(total, uint64(stats.InternalPageCount), uint64(stats.PurgeableCount), uint64(stats.WireCount),
		uint64(stats.CompressorPageCount), uint64(pageSize), swap.Used, pressure), true
}

func readBootTime() time.Time {
	// struct timeval
	var boot struct {
		Sec  int64
		Usec int32
		_    int32
	}
	if sysapi.Sysctl("kern.boottime", &boot) != nil {
		return time.Time{}
	}
	return time.Unix(boot.Sec, 0)
}
