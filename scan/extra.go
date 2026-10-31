package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const releasesURL = "https://api.github.com/repos/jetkvm/kvm/releases/latest"

// LatestRelease returns the newest stable JetKVM firmware version from
// GitHub, e.g. "0.5.9". Release tags look like "release/0.5.9".
func LatestRelease(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github: %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return strings.TrimPrefix(rel.TagName, "release/"), nil
}

// NewerVersion reports whether latest is a higher version than current.
// Pre-release suffixes ("-dev…") are ignored, so a dev build of the current
// release is not flagged.
func NewerVersion(latest, current string) bool {
	l, c := versionParts(latest), versionParts(current)
	if l == nil || c == nil {
		return false
	}
	for i := 0; i < max(len(l), len(c)); i++ {
		var a, b int
		if i < len(l) {
			a = l[i]
		}
		if i < len(c) {
			b = c[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var parts []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		parts = append(parts, n)
	}
	return parts
}

var macRE = regexp.MustCompile(`(?i)\b([0-9a-f]{1,2}(?:[:-][0-9a-f]{1,2}){5})\b`)

// MACAddress looks up ip in this machine's ARP table. It only works for
// hosts on the same network segment, which a scanned JetKVM always is.
func MACAddress(ip string) string { return lookupARP(ip) }

// parseARP extracts the MAC address of ip from arp output, which may hold
// rows for other hosts.
func parseARP(ip, out string) string {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.FieldsFunc(line, func(r rune) bool { return unicode.IsSpace(r) || r == '(' || r == ')' })
		if !slices.Contains(fields, ip) {
			continue
		}
		if m := macRE.FindStringSubmatch(line); m != nil {
			return normalizeMAC(m[1])
		}
	}
	return ""
}

// normalizeMAC lower-cases a MAC and zero-pads each byte (macOS prints
// "30:52:53:9:a2:fe"); all-zero and broadcast entries are treated as unknown.
func normalizeMAC(s string) string {
	parts := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ':' || r == '-' })
	if len(parts) != 6 {
		return ""
	}
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	mac := strings.Join(parts, ":")
	if mac == "00:00:00:00:00:00" || mac == "ff:ff:ff:ff:ff:ff" {
		return ""
	}
	return mac
}
