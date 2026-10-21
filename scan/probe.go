package scan

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Device is a JetKVM found on the network.
type Device struct {
	Name string // reverse-DNS hostname, or "JetKVM" when there is none
	IP   string
	URL  string
}

const (
	workers        = 64
	connectTimeout = 400 * time.Millisecond
	httpTimeout    = 2 * time.Second
)

var client = &http.Client{
	Timeout: httpTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// dialResult classifies the TCP connect to a host.
type dialResult int

const (
	silent      dialResult = iota // timeout or other error
	answered                      // connected, or actively refused
	unreachable                   // "no route to host"
)

func classifyDial(err error) dialResult {
	switch {
	case err == nil, errors.Is(err, syscall.ECONNREFUSED):
		return answered
	case errors.Is(err, syscall.EHOSTUNREACH):
		return unreachable
	}
	return silent
}

// Run probes every host in nets and calls found for each JetKVM. progress is
// called after each host with the number done and the total. Both callbacks
// are invoked from worker goroutines. Run returns when all hosts are probed
// or ctx is cancelled.
//
// blocked reports that nothing was found and most hosts were "unreachable".
// That is what macOS does while the app waits for Local Network permission;
// on a working network empty addresses time out instead. The gateway may
// still answer while blocked, so answered hosts don't rule it out.
func Run(ctx context.Context, nets []*net.IPNet, progress func(done, total int), found func(Device)) (blocked bool) {
	var hosts []net.IP
	for _, n := range nets {
		hosts = append(hosts, Hosts(n)...)
	}
	total := len(hosts)
	jobs := make(chan net.IP)
	var done, nFound, nUnreachable atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				d, ok, res := probe(ctx, ip.String())
				if ok {
					nFound.Add(1)
					found(d)
				}
				if res == unreachable {
					nUnreachable.Add(1)
				}
				progress(int(done.Add(1)), total)
			}
		}()
	}
	for _, ip := range hosts {
		select {
		case jobs <- ip:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	return ctx.Err() == nil && nFound.Load() == 0 && nUnreachable.Load() > int64(total/2)
}

// Probe checks whether host (an IP, optionally with :port) is a JetKVM.
func Probe(ctx context.Context, host string) (Device, bool) {
	d, ok, _ := probe(ctx, host)
	return d, ok
}

func probe(ctx context.Context, host string) (Device, bool, dialResult) {
	addr := host
	if _, _, err := net.SplitHostPort(host); err != nil {
		addr = net.JoinHostPort(host, "80")
	}
	d := net.Dialer{Timeout: connectTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return Device{}, false, classifyDial(err)
	}
	conn.Close()

	base := "http://" + addr
	if strings.HasSuffix(addr, ":80") {
		base = "http://" + strings.TrimSuffix(addr, ":80")
	}
	if !isJetKVM(ctx, base) {
		return Device{}, false, answered
	}
	ip, _, _ := net.SplitHostPort(addr)
	return Device{Name: lookupName(ctx, ip), IP: ip, URL: base}, true, answered
}

// isJetKVM asks the unauthenticated /device/status endpoint, which answers
// {"isSetup":bool[,"sku":"jetkvm-…"]}. Older firmware has no sku, so the
// page title is checked as a fallback.
func isJetKVM(ctx context.Context, base string) bool {
	var status struct {
		IsSetup *bool  `json:"isSetup"`
		SKU     string `json:"sku"`
	}
	body, ok := get(ctx, base+"/device/status", "application/json")
	if !ok || json.Unmarshal(body, &status) != nil || status.IsSetup == nil {
		return false
	}
	if strings.HasPrefix(status.SKU, "jetkvm") {
		return true
	}
	page, ok := get(ctx, base+"/", "text/html")
	return ok && strings.Contains(string(page), "<title>JetKVM</title>")
}

func get(ctx context.Context, url, accept string) ([]byte, bool) {
	return getLimit(ctx, url, accept, 64<<10)
}

// getLimit fetches url and returns at most limit bytes of a 200 response.
func getLimit(ctx context.Context, url, accept string, limit int64) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Accept", accept)
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return body, err == nil
}

// lookupName returns the short reverse-DNS name of ip (routers usually know
// the DHCP hostname, e.g. "jetkvm-abc123.lan"), or "JetKVM".
func lookupName(ctx context.Context, ip string) string {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return "JetKVM"
	}
	sort.Strings(names)
	name := strings.TrimSuffix(names[0], ".")
	if i := strings.IndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	return name
}
