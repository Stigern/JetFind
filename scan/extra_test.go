package scan

import (
	"strings"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"0.5.9", "0.5.9", false},
		{"0.5.10", "0.5.9", true},
		{"0.6.0", "0.5.9", true},
		{"0.5.9", "0.5.10", false},
		{"0.5.9", "0.5.9-dev202609082040", false},
		{"1.0", "0.9.9", true},
		{"", "0.5.9", false},
		{"0.5.9", "", false},
	}
	for _, c := range cases {
		if got := NewerVersion(c.latest, c.current); got != c.want {
			t.Errorf("NewerVersion(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestParseARP(t *testing.T) {
	cases := map[string]string{
		"? (192.168.1.18) at 30:52:53:9:a2:fe on en0 ifscope [ethernet]": "30:52:53:09:a2:fe",
		"  192.168.1.18          30-52-53-09-A2-FE     dynamic":          "30:52:53:09:a2:fe",
		"? (192.168.1.50) at (incomplete) on en0 ifscope [ethernet]":     "",
		"192.168.1.18 0x1 0x0 00:00:00:00:00:00 * eth0":                  "",
	}
	for in, want := range cases {
		ip := "192.168.1.18"
		if strings.Contains(in, "192.168.1.50") {
			ip = "192.168.1.50"
		}
		if got := parseARP(ip, in); got != want {
			t.Errorf("parseARP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseARPMatchesQueriedIP(t *testing.T) {
	windows := "Interface: 192.168.1.10 --- 0x3\n" +
		"  Internet Address      Physical Address      Type\n" +
		"  192.168.1.21          30-52-53-00-1a-2b     dynamic\n" +
		"  192.168.1.50          aa-bb-cc-dd-ee-ff     dynamic\n"
	cases := []struct{ ip, out, want string }{
		{"192.168.1.50", windows, "aa:bb:cc:dd:ee:ff"},
		{"192.168.1.5", windows, ""}, // .5 is not .50
		{"192.168.1.60", windows, ""},
		{"192.168.1.1", "? (192.168.1.1) at 74:ac:b9:4a:a:ee on en0 ifscope [ethernet]", "74:ac:b9:4a:0a:ee"},
		{"192.168.1.50", "192.168.1.50 ether ff-ff-ff-ff-ff-ff C eth0", ""},
	}
	for _, c := range cases {
		if got := parseARP(c.ip, c.out); got != c.want {
			t.Errorf("parseARP(%q, %q) = %q, want %q", c.ip, c.out, got, c.want)
		}
	}
}
