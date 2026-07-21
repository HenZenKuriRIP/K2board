package services

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PanelVersion is shown on the admin dashboard.
const PanelVersion = "v1.4.26"

var processStartedAt = time.Now()

// HostHealth is panel host / process health for the admin dashboard.
type HostHealth struct {
	Hostname   string  `json:"hostname"`
	OS         string  `json:"os"`
	Arch       string  `json:"arch"`
	GoVersion  string  `json:"go_version"`
	NumCPU     int     `json:"num_cpu"`
	Goroutines int     `json:"goroutines"`
	UptimeSec  int64   `json:"uptime_sec"`
	// Process memory (Go runtime)
	AllocBytes uint64 `json:"alloc_bytes"`
	SysBytes   uint64 `json:"sys_bytes"`
	// System load (1/5/15 min); 0 if unavailable
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
	// System memory
	MemTotalBytes uint64  `json:"mem_total_bytes"`
	MemUsedBytes  uint64  `json:"mem_used_bytes"`
	MemUsedPct    float64 `json:"mem_used_pct"`
	// Root disk
	DiskTotalBytes uint64  `json:"disk_total_bytes"`
	DiskUsedBytes  uint64  `json:"disk_used_bytes"`
	DiskUsedPct    float64 `json:"disk_used_pct"`
	// CPU % (0–100); -1 if not yet sampled
	CPUPercent float64 `json:"cpu_percent"`
	// Status: healthy | warn | critical
	Status  string `json:"status"`
	Message string `json:"message"`
}

var (
	cpuMu         sync.Mutex
	cpuPrevIdle   uint64
	cpuPrevTotal  uint64
	cpuPrevOK     bool
	cpuLastPct    = -1.0
)

// CollectHostHealth gathers process + host metrics (best-effort per platform).
func CollectHostHealth() HostHealth {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	host, _ := os.Hostname()
	h := HostHealth{
		Hostname:   host,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		GoVersion:  runtime.Version(),
		NumCPU:     runtime.NumCPU(),
		Goroutines: runtime.NumGoroutine(),
		UptimeSec:  int64(time.Since(processStartedAt).Seconds()),
		AllocBytes: ms.Alloc,
		SysBytes:   ms.Sys,
		CPUPercent: -1,
	}

	fillLoadAvg(&h)
	fillMemInfo(&h)
	fillDiskRoot(&h)
	fillCPUPercent(&h)
	deriveHealthStatus(&h)
	return h
}

func deriveHealthStatus(h *HostHealth) {
	// Critical thresholds
	if h.MemUsedPct >= 92 || h.DiskUsedPct >= 95 {
		h.Status = "critical"
		if h.DiskUsedPct >= 95 {
			h.Message = "磁盘空间即将耗尽"
		} else {
			h.Message = "系统内存压力较高"
		}
		return
	}
	if h.CPUPercent >= 90 {
		h.Status = "critical"
		h.Message = "CPU 使用率过高"
		return
	}
	// Warn
	if h.MemUsedPct >= 80 || h.DiskUsedPct >= 85 || h.CPUPercent >= 75 ||
		(h.NumCPU > 0 && h.Load1 >= float64(h.NumCPU)*1.5) {
		h.Status = "warn"
		h.Message = "资源使用偏高，建议关注"
		return
	}
	h.Status = "healthy"
	h.Message = "运行正常"
}

func fillLoadAvg(h *HostHealth) {
	// Linux: /proc/loadavg  — Darwin: not always present; leave 0
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}
	parts := strings.Fields(string(data))
	if len(parts) < 3 {
		return
	}
	h.Load1, _ = strconv.ParseFloat(parts[0], 64)
	h.Load5, _ = strconv.ParseFloat(parts[1], 64)
	h.Load15, _ = strconv.ParseFloat(parts[2], 64)
}

func fillMemInfo(h *HostHealth) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	var total, available uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// values are kB
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		v *= 1024
		switch fields[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			available = v
		}
	}
	if total == 0 {
		return
	}
	used := total - available
	if available > total {
		used = 0
	}
	h.MemTotalBytes = total
	h.MemUsedBytes = used
	h.MemUsedPct = float64(used) / float64(total) * 100
}

func fillCPUPercent(h *HostHealth) {
	idle, total, ok := readProcStatCPU()
	if !ok {
		return
	}
	cpuMu.Lock()
	defer cpuMu.Unlock()
	if cpuPrevOK && total > cpuPrevTotal {
		dTotal := total - cpuPrevTotal
		dIdle := idle - cpuPrevIdle
		if dTotal > 0 {
			busy := float64(dTotal-dIdle) / float64(dTotal) * 100
			if busy < 0 {
				busy = 0
			}
			if busy > 100 {
				busy = 100
			}
			cpuLastPct = busy
		}
	}
	cpuPrevIdle = idle
	cpuPrevTotal = total
	cpuPrevOK = true
	h.CPUPercent = cpuLastPct
}

func readProcStatCPU() (idle, total uint64, ok bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, false
	}
	// cpu  user nice system idle iowait irq softirq steal ...
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	var vals []uint64
	for i := 1; i < len(fields); i++ {
		v, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			break
		}
		vals = append(vals, v)
	}
	if len(vals) < 4 {
		return 0, 0, false
	}
	for _, v := range vals {
		total += v
	}
	idle = vals[3]
	if len(vals) > 4 {
		idle += vals[4] // iowait
	}
	return idle, total, true
}
