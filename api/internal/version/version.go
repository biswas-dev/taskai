package version

import (
	"fmt"
	"os"
	"runtime"
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

// RuntimeStats holds Go runtime statistics
type RuntimeStats struct {
	Goroutines  int     `json:"goroutines"`
	MemAlloc    string  `json:"mem_alloc"`
	MemSys      string  `json:"mem_sys"`
	GCCycles    uint32  `json:"gc_cycles"`
	GCPauseTotal string `json:"gc_pause_total"`
	GCLastPause  string `json:"gc_last_pause"`
}

// Info holds version and build information
type Info struct {
	Version     string        `json:"version"`
	GitCommit   string        `json:"git_commit"`
	BuildTime   string        `json:"build_time"`
	GoVersion   string        `json:"go_version"`
	Platform    string        `json:"platform"`
	ServerTime  time.Time     `json:"server_time"`
	Uptime      string        `json:"uptime"`
	DBVersion   int           `json:"db_version,omitempty"`
	DBDriver    string        `json:"db_driver"`
	Environment string        `json:"environment"`
	Hostname    string        `json:"hostname,omitempty"`
	Runtime     *RuntimeStats `json:"runtime,omitempty"`
}

func formatBytes(b uint64) string {
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

// Get returns the current version information
func Get(env string, dbVersion int, dbDriver string) Info {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	hostname, _ := os.Hostname()

	return Info{
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
		Runtime: &RuntimeStats{
			Goroutines:   runtime.NumGoroutine(),
			MemAlloc:     formatBytes(m.Alloc),
			MemSys:       formatBytes(m.Sys),
			GCCycles:     m.NumGC,
			GCPauseTotal: fmt.Sprintf("%.2f ms", float64(m.PauseTotalNs)/1e6),
			GCLastPause:  fmt.Sprintf("%.2f ms", float64(m.PauseNs[(m.NumGC+255)%256])/1e6),
		},
	}
}
