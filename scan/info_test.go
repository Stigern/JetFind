package scan

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const sampleMetrics = `# HELP jetkvm_build_info A metric with a constant '1' value.
# TYPE jetkvm_build_info gauge
jetkvm_build_info{branch="dev",goarch="arm",tags="netgo,timetzdata,nomsgpack",version="0.5.9"} 1
jetkvm_cloud_connection_status 0
jetkvm_timesync_status 1
process_start_time_seconds 1.79232668915e+09
jetkvm_connection_last_ping_received_timestamp_seconds{source="192.168.1.96",type="local"} 1.792248763e+09
jetkvm_connection_last_ping_received_timestamp_seconds{source="192.168.1.176",type="local"} 1.7925993954e+09
`

func TestApplyMetrics(t *testing.T) {
	now := time.Unix(1792599400, 0)
	var info Info
	applyMetrics(&info, parseMetrics([]byte(sampleMetrics)), now)

	if info.Version != "0.5.9" {
		t.Errorf("Version = %q", info.Version)
	}
	if info.CloudConnected {
		t.Error("CloudConnected = true")
	}
	if want := now.Sub(time.Unix(1792326689, 0)); info.Uptime != want {
		t.Errorf("Uptime = %v, want %v", info.Uptime, want)
	}
	if info.ActiveClient != "192.168.1.176" {
		t.Errorf("ActiveClient = %q", info.ActiveClient)
	}
	if !info.ClockKnown || !info.ClockSynced {
		t.Errorf("clock known=%v synced=%v, want both true", info.ClockKnown, info.ClockSynced)
	}

	// Same data a minute later: the session is no longer live.
	info = Info{}
	applyMetrics(&info, parseMetrics([]byte(sampleMetrics)), now.Add(time.Minute))
	if info.ActiveClient != "" {
		t.Errorf("stale session reported active: %q", info.ActiveClient)
	}
}

func TestApplyMetricsUnsyncedClock(t *testing.T) {
	now := time.Unix(1792599400, 0)
	cases := map[string]string{
		"ancient start, no sync metric":   "process_start_time_seconds 3.27\n",
		"plausible start, clock unsynced": "jetkvm_timesync_status 0\nprocess_start_time_seconds 1.7925e+09\n",
	}
	for name, metrics := range cases {
		var info Info
		applyMetrics(&info, parseMetrics([]byte(metrics)), now)
		if info.Uptime != 0 {
			t.Errorf("%s: Uptime = %v, want 0", name, info.Uptime)
		}
	}
}

func TestParseLabels(t *testing.T) {
	got := parseLabels(`tags="a,b=c",quote="say \"hi\"",empty="",version="0.5.9"`)
	want := map[string]string{"tags": "a,b=c", "quote": `say "hi"`, "empty": "", "version": "0.5.9"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestFetchInfoLargeMetrics(t *testing.T) {
	// Many past browser sessions push the useful lines far down the page.
	var b strings.Builder
	for i := 0; b.Len() < 200<<10; i++ {
		fmt.Fprintf(&b, "jetkvm_connection_ping_duration_seconds_bucket{source=\"10.0.%d.%d\",le=\"0.1\"} 1\n", i/250, i%250)
	}
	b.WriteString("jetkvm_timesync_status 1\nprocess_start_time_seconds 1\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			w.Write([]byte(b.String()))
		}
	}))
	defer srv.Close()
	info, ok := FetchInfo(context.Background(), Device{URL: srv.URL})
	if !ok || !info.ClockKnown || info.Uptime == 0 {
		t.Fatalf("ok=%v clockKnown=%v uptime=%v; metrics past 64 KB were dropped", ok, info.ClockKnown, info.Uptime)
	}
}

func TestFetchInfoReachableWhenConnectMeasuresZero(t *testing.T) {
	since = func(time.Time) time.Duration { return 0 }
	defer func() { since = time.Since }()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, ok := FetchInfo(context.Background(), Device{URL: srv.URL}); !ok {
		t.Fatal("a device that accepted the connection was reported as not answering")
	}
}

func TestFetchInfoUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if _, ok := FetchInfo(context.Background(), Device{URL: url}); ok {
		t.Fatal("a closed port was reported as answering")
	}
}
