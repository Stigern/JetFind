package scan

import (
	"net"
	"testing"
)

func TestNarrow(t *testing.T) {
	cases := map[string]string{
		"192.168.1.41/24": "192.168.1.0/24",
		"10.0.5.7/22":     "10.0.4.0/22",
		"10.20.30.40/16":  "10.20.30.0/24",
		"172.16.9.9/8":    "172.16.9.0/24",
	}
	for in, want := range cases {
		ip, n, _ := net.ParseCIDR(in)
		n.IP = ip
		if got := narrow(n).String(); got != want {
			t.Errorf("narrow(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestHosts(t *testing.T) {
	_, n, _ := net.ParseCIDR("192.168.1.0/24")
	h := Hosts(n)
	if len(h) != 254 || h[0].String() != "192.168.1.1" || h[253].String() != "192.168.1.254" {
		t.Fatalf("got %d hosts, first %v last %v", len(h), h[0], h[len(h)-1])
	}
	_, n, _ = net.ParseCIDR("10.0.0.4/31")
	if h := Hosts(n); len(h) != 2 {
		t.Fatalf("/31: got %d hosts, want 2", len(h))
	}
}
