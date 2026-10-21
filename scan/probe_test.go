package scan

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
)

func serve(status, index string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device/status":
			if status == "" {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(status))
		default:
			w.Write([]byte(index))
		}
	}))
}

func TestProbe(t *testing.T) {
	const jetIndex = `<html><head><title>JetKVM</title></head></html>`
	cases := []struct {
		name, status, index string
		want                bool
	}{
		{"new firmware with sku", `{"isSetup":true,"sku":"jetkvm-v2"}`, "", true},
		{"old firmware, title fallback", `{"isSetup":false}`, jetIndex, true},
		{"isSetup but other page", `{"isSetup":true}`, `<title>Router</title>`, false},
		{"generic web server", "", `<title>NAS</title>`, false},
		{"json without isSetup", `{"ok":true}`, jetIndex, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := serve(c.status, c.index)
			defer srv.Close()
			host := strings.TrimPrefix(srv.URL, "http://")
			d, ok := Probe(context.Background(), host)
			if ok != c.want {
				t.Fatalf("Probe = %v, want %v", ok, c.want)
			}
			if ok && (d.URL != srv.URL || d.IP != "127.0.0.1" || d.Name == "") {
				t.Fatalf("unexpected device %+v", d)
			}
		})
	}
}

func TestClassifyDial(t *testing.T) {
	wrap := func(e error) error {
		return &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connect", Err: e}}
	}
	cases := []struct {
		err  error
		want dialResult
	}{
		{nil, answered},
		{wrap(syscall.ECONNREFUSED), answered},
		{wrap(syscall.EHOSTUNREACH), unreachable},
		{context.DeadlineExceeded, silent},
	}
	for _, c := range cases {
		if got := classifyDial(c.err); got != c.want {
			t.Errorf("classifyDial(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestProbeClosedPort(t *testing.T) {
	srv := serve("", "")
	host := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()
	if _, ok := Probe(context.Background(), host); ok {
		t.Fatal("closed port reported as JetKVM")
	}
}
