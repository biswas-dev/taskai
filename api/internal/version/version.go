package version

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	// Version is the semantic version (set via ldflags during build)
	Version = "dev"

	// GitCommit is the git commit hash (set via ldflags during build)
	GitCommit = "unknown"

	// BuildTime is the build timestamp (set via ldflags during build)
	BuildTime = "unknown"

	// startTime records when the process started
	startTime = time.Now()
)

// ResourceMetrics matches BuildMe's expected format for dashboard metrics
type ResourceMetrics struct {
	MemoryAllocMB  float64 `json:"memory_alloc_mb"`
	HeapInuseMB    float64 `json:"heap_inuse_mb"`
	StackInuseMB   float64 `json:"stack_inuse_mb"`
	Goroutines     int     `json:"goroutines"`
	NumGC          uint32  `json:"num_gc"`
	GCPauseTotalMS float64 `json:"gc_pause_total_ms"`
	GCLastPauseMS  float64 `json:"gc_last_pause_ms"`
}

// ContainerMetrics holds cgroup-based container stats (Linux only)
type ContainerMetrics struct {
	MemoryUsageMB float64 `json:"memory_usage_mb"`
	MemoryLimitMB float64 `json:"memory_limit_mb"`
	CPUUsageNS    int64   `json:"cpu_usage_ns,omitempty"`
}

// Info holds version and build information
type Info struct {
	Version     string            `json:"version"`
	GitCommit   string            `json:"git_commit"`
	BuildTime   string            `json:"build_time"`
	GoVersion   string            `json:"go_version"`
	Platform    string            `json:"platform"`
	ServerTime  time.Time         `json:"server_time"`
	Uptime      string            `json:"uptime"`
	DBVersion   int               `json:"db_version,omitempty"`
	DBDriver    string            `json:"db_driver"`
	Environment string            `json:"environment"`
	Hostname    string            `json:"hostname,omitempty"`
	Resources   *ResourceMetrics  `json:"resources,omitempty"`
	Container   *ContainerMetrics `json:"container,omitempty"`
}

// Get returns the current version information
func Get(env string, dbVersion int, dbDriver string) Info {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	hostname, _ := os.Hostname()

	info := Info{
		Version:     Version,
		GitCommit:   GitCommit,
		BuildTime:   BuildTime,
		GoVersion:   runtime.Version(),
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
		ServerTime:  time.Now().UTC(),
		Uptime:      time.Since(startTime).Truncate(time.Second).String(),
		DBVersion:   dbVersion,
		DBDriver:    dbDriver,
		Environment: env,
		Hostname:    hostname,
		Resources: &ResourceMetrics{
			MemoryAllocMB:  float64(m.Alloc) / 1024 / 1024,
			HeapInuseMB:    float64(m.HeapInuse) / 1024 / 1024,
			StackInuseMB:   float64(m.StackInuse) / 1024 / 1024,
			Goroutines:     runtime.NumGoroutine(),
			NumGC:          m.NumGC,
			GCPauseTotalMS: float64(m.PauseTotalNs) / 1e6,
			GCLastPauseMS:  float64(m.PauseNs[(m.NumGC+255)%256]) / 1e6,
		},
	}

	// Read container cgroup metrics (Linux only, best-effort)
	if c := readContainerMetrics(); c != nil {
		info.Container = c
	}

	return info
}

// readContainerMetrics reads cgroup v2 memory stats. Returns nil if not in a container.
func readContainerMetrics() *ContainerMetrics {
	usage := readCgroupInt("/sys/fs/cgroup/memory.current")
	limit := readCgroupInt("/sys/fs/cgroup/memory.max")
	if usage <= 0 {
		// Try cgroup v1
		usage = readCgroupInt("/sys/fs/cgroup/memory/memory.usage_in_bytes")
		limit = readCgroupInt("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	}
	if usage <= 0 {
		return nil
	}

	c := &ContainerMetrics{
		MemoryUsageMB: float64(usage) / 1024 / 1024,
	}
	// cgroup "max" or very large limit means no real limit
	if limit > 0 && limit < 1<<50 {
		c.MemoryLimitMB = float64(limit) / 1024 / 1024
	}

	// CPU usage (cgroup v2)
	cpuData, err := os.ReadFile("/sys/fs/cgroup/cpuacct/cpuacct.usage")
	if err != nil {
		cpuData, err = os.ReadFile("/sys/fs/cgroup/cpu.stat")
		if err == nil {
			for _, line := range strings.Split(string(cpuData), "\n") {
				if strings.HasPrefix(line, "usage_usec ") {
					if usec, e := strconv.ParseInt(strings.TrimPrefix(line, "usage_usec "), 10, 64); e == nil {
						c.CPUUsageNS = usec * 1000
					}
					break
				}
			}
		}
	} else {
		if ns, e := strconv.ParseInt(strings.TrimSpace(string(cpuData)), 10, 64); e == nil {
			c.CPUUsageNS = ns
		}
	}

	return c
}

func readCgroupInt(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(data))
	if s == "max" {
		return 1 << 62 // effectively unlimited
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// Sprintf helper for human-readable values (used in logging)
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
