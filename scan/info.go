package scan

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"time"
)

// Info is live status for a JetKVM, read from its unauthenticated
// /device/status and Prometheus /metrics endpoints.
type Info struct {
	Latency        time.Duration // TCP connect time; 0 if unreachable
	Version        string        // firmware version, e.g. "0.5.9"
	Uptime         time.Duration // 0 when unknown
	IsSetup        bool
	CloudConnected bool
	ActiveClient   string // IP of a browser with a live session, if any
	ClockKnown     bool   // whether the time-sync metric was present
	ClockSynced    bool
}

// metricsLimit caps the /metrics download. The page grows by ~2.5 KB for
// every browser IP that has ever connected, and the values we need sit near
// the end, so it must be far larger than a typical 25 KB response.
const metricsLimit = 4 << 20

// activeWindow is how recent a session ping must be to count as "in use".
// The JetKVM UI pings every few seconds while a session is open.
const activeWindow = 15 * time.Second

// FetchInfo measures latency to d and reads its status and metrics.
// It returns ok=false when the device does not answer at all.
func FetchInfo(ctx context.Context, d Device) (Info, bool) {
	var info Info
	addr := strings.TrimPrefix(d.URL, "http://")
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "80")
	}
	info.Latency = latency(ctx, addr)
	if info.Latency == 0 {
		return info, false
	}

	if body, ok := get(ctx, d.URL+"/device/status", "application/json"); ok {
		var s struct {
			IsSetup bool `json:"isSetup"`
		}
		if json.Unmarshal(body, &s) == nil {
			info.IsSetup = s.IsSetup
		}
	}
	if body, ok := getLimit(ctx, d.URL+"/metrics", "text/plain", metricsLimit); ok {
		applyMetrics(&info, parseMetrics(body), time.Now())
	}
	return info, true
}

// latency returns the fastest of three TCP connects, or 0 if all fail.
func latency(ctx context.Context, addr string) time.Duration {
	var best time.Duration
	d := net.Dialer{Timeout: time.Second}
	for range 3 {
		start := time.Now()
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			continue
		}
		rtt := time.Since(start)
		conn.Close()
		if best == 0 || rtt < best {
			best = rtt
		}
	}
	return best
}

type sample struct {
	name   string
	labels map[string]string
	value  float64
}

// parseMetrics reads the Prometheus text format, ignoring comments.
func parseMetrics(body []byte) []sample {
	var out []sample
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		v, err := strconv.ParseFloat(line[sp+1:], 64)
		if err != nil {
			continue
		}
		s := sample{name: line[:sp], value: v, labels: map[string]string{}}
		if i := strings.IndexByte(s.name, '{'); i >= 0 && strings.HasSuffix(s.name, "}") {
			s.labels = parseLabels(s.name[i+1 : len(s.name)-1])
			s.name = s.name[:i]
		}
		out = append(out, s)
	}
	return out
}

// parseLabels reads `k="v",k2="v2"`. Values may contain commas and
// backslash-escaped quotes (build_info has tags="netgo,timetzdata,…").
func parseLabels(s string) map[string]string {
	labels := map[string]string{}
	for len(s) > 0 {
		eq := strings.IndexByte(s, '=')
		if eq < 0 || eq+1 >= len(s) || s[eq+1] != '"' {
			break
		}
		key := strings.TrimSpace(s[:eq])
		var val strings.Builder
		i := eq + 2
		for ; i < len(s) && s[i] != '"'; i++ {
			if s[i] == '\\' && i+1 < len(s) {
				i++
				if s[i] == 'n' {
					val.WriteByte('\n')
					continue
				}
			}
			val.WriteByte(s[i])
		}
		labels[key] = val.String()
		s = strings.TrimPrefix(s[min(i+1, len(s)):], ",")
	}
	return labels
}

func applyMetrics(info *Info, samples []sample, now time.Time) {
	var lastPing, started float64
	for _, s := range samples {
		switch s.name {
		case "jetkvm_build_info":
			info.Version = s.labels["version"]
		case "jetkvm_cloud_connection_status":
			info.CloudConnected = s.value == 1
		case "jetkvm_timesync_status":
			info.ClockKnown, info.ClockSynced = true, s.value == 1
		case "process_start_time_seconds":
			started = s.value
		case "jetkvm_connection_last_ping_received_timestamp_seconds":
			if s.value > lastPing {
				lastPing = s.value
				if now.Sub(time.Unix(int64(s.value), 0)).Abs() < activeWindow {
					info.ActiveClient = s.labels["source"]
				} else {
					info.ActiveClient = ""
				}
			}
		}
	}
	// The start time is derived from the device's wall clock, so it is only
	// meaningful once that clock is synced. Old firmware lacks the sync
	// metric; fall back to a plausibility check there.
	if started > 0 {
		up := now.Sub(time.Unix(int64(started), 0))
		trusted := info.ClockSynced || (!info.ClockKnown && up < 5*365*24*time.Hour)
		if up > 0 && trusted {
			info.Uptime = up
		}
	}
}
