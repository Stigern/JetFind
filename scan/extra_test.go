package scan

import "testing"

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
		if got := parseARP(in); got != want {
			t.Errorf("parseARP(%q) = %q, want %q", in, got, want)
		}
	}
}
