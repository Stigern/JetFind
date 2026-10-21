//go:build !darwin

package scan

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func lookupARP(ip string) string {
	if runtime.GOOS == "linux" {
		if b, err := os.ReadFile("/proc/net/arp"); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				f := strings.Fields(line)
				if len(f) >= 4 && f[0] == ip {
					return normalizeMAC(f[3])
				}
			}
		}
	}
	args := []string{"-n", ip}
	if runtime.GOOS == "windows" {
		args = []string{"-a", ip}
	}
	out, err := exec.Command("arp", args...).Output()
	if err != nil {
		return ""
	}
	return parseARP(string(out))
}
