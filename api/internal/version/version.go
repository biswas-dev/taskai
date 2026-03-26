package version

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
	startedAt = time.Now()
)

// BackendInfo matches BuildMe's backend section
type BackendInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version,omitempty"`
	Platform  string `json:"platform,omitempty"`
}

// RuntimeInfo matches BuildMe's runtime section
type RuntimeInfo struct {
	Hostname string `json:"hostname"`
	PID      int    `json:"pid"`
	Port     int    `json:"port,omitempty"`
	Uptime   int64  `json:"uptime_seconds"`
	Started  string `json:"started_at"`
}

// ResourceMetrics matches BuildMe's resources section
type ResourceMetrics struct {
	MemoryAllocMB  float64 `json:"memory_alloc_mb"`
	HeapInuseMB    float64 `json:"heap_inuse_mb"`
	StackInuseMB   float64 `json:"stack_inuse_mb"`
	Goroutines     int     `json:"goroutines"`
	NumGC          uint32  `json:"num_gc"`
	GCPauseTotalMS float64 `json:"gc_pause_total_ms"`
	GCLastPauseMS  float64 `json:"gc_last_pause_ms"`
}

// DatabaseInfo matches BuildMe's database section
type DatabaseInfo struct {
	Type             string `json:"type"`
	MigrationVersion int    `json:"migration_version,omitempty"`
	Environment      string `json:"environment,omitempty"`
}

// ContainerMetrics matches BuildMe's container section
type ContainerMetrics struct {
	MemoryUsageMB float64 `json:"memory_usage_mb,omitempty"`
	MemoryLimitMB float64 `json:"memory_limit_mb,omitempty"`
	CPUUsageNS    int64   `json:"cpu_usage_ns,omitempty"`
}

// VersionResponse matches the BuildMe dashboard expected format exactly
type VersionResponse struct {
	Backend   BackendInfo       `json:"backend"`
	Runtime   RuntimeInfo       `json:"runtime"`
	Resources ResourceMetrics   `json:"resources"`
	Database  DatabaseInfo      `json:"database"`
	Container *ContainerMetrics `json:"container,omitempty"`
}

// Get returns version info in BuildMe-compatible format
func Get(env string, dbVersion int, dbDriver string) VersionResponse {
	hostname, _ := os.Hostname()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	var lastPauseNs uint64
	if mem.NumGC > 0 {
		lastPauseNs = mem.PauseNs[(mem.NumGC+255)%256]
	}

	resp := VersionResponse{
		Backend: BackendInfo{
			Version:   Version,
			GitCommit: GitCommit,
			BuildTime: BuildTime,
			GoVersion: runtime.Version(),
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		},
		Runtime: RuntimeInfo{
			Hostname: hostname,
			PID:      os.Getpid(),
			Port:     8080,
			Uptime:   int64(time.Since(startedAt).Seconds()),
			Started:  startedAt.UTC().Format(time.RFC3339),
		},
		Resources: ResourceMetrics{
			MemoryAllocMB:  float64(mem.Alloc) / 1024 / 1024,
			HeapInuseMB:    float64(mem.HeapInuse) / 1024 / 1024,
			StackInuseMB:   float64(mem.StackInuse) / 1024 / 1024,
			Goroutines:     runtime.NumGoroutine(),
			NumGC:          mem.NumGC,
			GCPauseTotalMS: float64(mem.PauseTotalNs) / 1e6,
			GCLastPauseMS:  float64(lastPauseNs) / 1e6,
		},
		Database: DatabaseInfo{
			Type:             dbDriver,
			MigrationVersion: dbVersion,
			Environment:      env,
		},
	}

	if cm := readContainerMetrics(); cm != nil {
		resp.Container = cm
	}

	return resp
}

func readContainerMetrics() *ContainerMetrics {
	cm := &ContainerMetrics{}
	hasData := false

	// cgroup v2
	if val := readCgroupInt("/sys/fs/cgroup/memory.current"); val > 0 {
		cm.MemoryUsageMB = float64(val) / 1024 / 1024
		hasData = true
	} else if val := readCgroupInt("/sys/fs/cgroup/memory/memory.usage_in_bytes"); val > 0 {
		cm.MemoryUsageMB = float64(val) / 1024 / 1024
		hasData = true
	}

	if val := readCgroupInt("/sys/fs/cgroup/memory.max"); val > 0 && val < 1<<62 {
		cm.MemoryLimitMB = float64(val) / 1024 / 1024
		hasData = true
	} else if val := readCgroupInt("/sys/fs/cgroup/memory/memory.limit_in_bytes"); val > 0 && val < 1<<62 {
		cm.MemoryLimitMB = float64(val) / 1024 / 1024
		hasData = true
	}

	if content, err := os.ReadFile("/sys/fs/cgroup/cpu.stat"); err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "usage_usec ") {
				if usec, e := strconv.ParseInt(strings.TrimPrefix(line, "usage_usec "), 10, 64); e == nil {
					cm.CPUUsageNS = usec * 1000
					hasData = true
				}
				break
			}
		}
	} else if val := readCgroupInt("/sys/fs/cgroup/cpuacct/cpuacct.usage"); val > 0 {
		cm.CPUUsageNS = val
		hasData = true
	}

	if !hasData {
		return nil
	}
	return cm
}

func readCgroupInt(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(data))
	if s == "max" {
		return 1 << 62
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}
