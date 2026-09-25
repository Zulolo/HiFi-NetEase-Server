//go:build linux

// Package sysinfo samples the board's health from /proc and /sys (NFR-7):
// CPU, memory, temperature, network throughput and Wi-Fi signal, uptime, and
// the memory of hifid and MPD. Rates are deltas between consecutive samples.
package sysinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Stats struct {
	SampledAt   time.Time `json:"sampled_at"`
	UptimeSec   int64     `json:"uptime_sec"`
	CPUPercent  float64   `json:"cpu_percent"`
	CPUCores    int       `json:"cpu_cores"`
	CPUMHz      int       `json:"cpu_mhz,omitempty"`
	Load1       float64   `json:"load1"`
	Load5       float64   `json:"load5"`
	MemTotal    int64     `json:"mem_total"`
	MemUsed     int64     `json:"mem_used"`
	TempC       float64   `json:"temp_c,omitempty"`
	TempZone    string    `json:"temp_zone,omitempty"`
	NetIface    string    `json:"net_iface,omitempty"`
	NetRxBps    float64   `json:"net_rx_bps"`
	NetTxBps    float64   `json:"net_tx_bps"`
	WifiDBm     int       `json:"wifi_dbm,omitempty"`
	WifiQuality int       `json:"wifi_quality,omitempty"`
	HifidRSS    int64     `json:"hifid_rss"`
	MPDRSS      int64     `json:"mpd_rss"`
}

type sample struct {
	at                time.Time
	cpuIdle, cpuTotal uint64
	rx, tx            uint64
}

var (
	mu   sync.Mutex
	prev *sample
	last *Stats
)

// Sample returns fresh stats. Calls closer than 500 ms apart reuse the previous
// result, so a busy UI cannot make the rates jitter or the reads expensive.
func Sample(iface string) Stats {
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()
	if last != nil && now.Sub(last.SampledAt) < 500*time.Millisecond {
		return *last
	}
	s := Stats{SampledAt: now}
	cur := sample{at: now}

	if f := fields(readFile("/proc/uptime")); len(f) > 0 {
		up, _ := strconv.ParseFloat(f[0], 64)
		s.UptimeSec = int64(up)
	}
	if f := fields(readFile("/proc/loadavg")); len(f) >= 2 {
		s.Load1, _ = strconv.ParseFloat(f[0], 64)
		s.Load5, _ = strconv.ParseFloat(f[1], 64)
	}

	// CPU: "cpu user nice system idle iowait irq softirq steal ..."
	for _, line := range strings.Split(readFile("/proc/stat"), "\n") {
		if strings.HasPrefix(line, "cpu ") {
			f := fields(line)
			var total uint64
			for i := 1; i < len(f) && i <= 8; i++ {
				v, _ := strconv.ParseUint(f[i], 10, 64)
				total += v
				if i == 4 || i == 5 { // idle + iowait
					cur.cpuIdle += v
				}
			}
			cur.cpuTotal = total
		} else if strings.HasPrefix(line, "cpu") {
			s.CPUCores++
		}
	}
	if khz, err := strconv.Atoi(strings.TrimSpace(readFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq"))); err == nil {
		s.CPUMHz = khz / 1000
	}

	// Memory
	var total, avail int64
	for _, line := range strings.Split(readFile("/proc/meminfo"), "\n") {
		f := fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseInt(f[1], 10, 64)
		switch f[0] {
		case "MemTotal:":
			total = v * 1024
		case "MemAvailable:":
			avail = v * 1024
		}
	}
	s.MemTotal, s.MemUsed = total, total-avail

	// Temperature: the CPU zone when the kernel names one (this board has
	// gpu/ve/cpu/ddr zones), otherwise the hottest zone.
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	cpuZone := false
	for _, z := range zones {
		mc, err := strconv.Atoi(strings.TrimSpace(readFile(z)))
		if err != nil {
			continue
		}
		c := float64(mc) / 1000
		zt := strings.TrimSpace(readFile(filepath.Join(filepath.Dir(z), "type")))
		isCPU := strings.Contains(strings.ToLower(zt), "cpu")
		if (isCPU && !cpuZone) || (!cpuZone && c > s.TempC) {
			s.TempC, s.TempZone = c, zt
			cpuZone = cpuZone || isCPU
		}
	}

	// Network counters for the given interface
	s.NetIface = iface
	for _, line := range strings.Split(readFile("/proc/net/dev"), "\n") {
		if name, rest, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && name == iface {
			f := fields(rest)
			if len(f) >= 9 {
				cur.rx, _ = strconv.ParseUint(f[0], 10, 64)
				cur.tx, _ = strconv.ParseUint(f[8], 10, 64)
			}
		}
	}
	// Wi-Fi: "wlan0: 0000   53.  -57.  -256 ..." (quality, level dBm, noise)
	for _, line := range strings.Split(readFile("/proc/net/wireless"), "\n") {
		if name, rest, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && name == iface {
			f := fields(rest)
			if len(f) >= 3 {
				q, _ := strconv.ParseFloat(strings.TrimSuffix(f[1], "."), 64)
				l, _ := strconv.ParseFloat(strings.TrimSuffix(f[2], "."), 64)
				s.WifiQuality, s.WifiDBm = int(q), int(l)
			}
		}
	}

	// Rates against the previous sample
	if prev != nil {
		dt := now.Sub(prev.at).Seconds()
		if dt > 0 {
			if dTotal := cur.cpuTotal - prev.cpuTotal; dTotal > 0 {
				s.CPUPercent = 100 * (1 - float64(cur.cpuIdle-prev.cpuIdle)/float64(dTotal))
			}
			if cur.rx >= prev.rx {
				s.NetRxBps = float64(cur.rx-prev.rx) / dt
			}
			if cur.tx >= prev.tx {
				s.NetTxBps = float64(cur.tx-prev.tx) / dt
			}
		}
	}
	prev = &cur

	s.HifidRSS = rssOf("/proc/self/status")
	s.MPDRSS = rssOfComm("mpd")

	last = &s
	return s
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func fields(s string) []string { return strings.Fields(s) }

func rssOf(statusPath string) int64 {
	for _, line := range strings.Split(readFile(statusPath), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			f := fields(line)
			if len(f) >= 2 {
				kb, _ := strconv.ParseInt(f[1], 10, 64)
				return kb * 1024
			}
		}
	}
	return 0
}

// rssOfComm finds a process by its comm name without exec'ing anything.
func rssOfComm(comm string) int64 {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if strings.TrimSpace(readFile("/proc/"+e.Name()+"/comm")) == comm {
			return rssOf("/proc/" + e.Name() + "/status")
		}
	}
	return 0
}
