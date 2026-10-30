package scan

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW, which package syscall doesn't define.
const createNoWindow = 0x08000000

// lookupARP asks arp.exe. Without createNoWindow a GUI app gets a console
// window flashing up for every device found.
func lookupARP(ip string) string {
	cmd := exec.Command("arp", "-a", ip)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseARP(string(out))
}
