package scan

import (
	"fmt"
	"net"
	"syscall"

	"golang.org/x/net/route"
)

// lookupARP reads the kernel's link-layer routes directly. Shelling out to
// arp(8) is not enough on macOS: the child process lacks the app's Local
// Network permission and sees an empty table.
func lookupARP(ip string) string {
	want := net.ParseIP(ip).To4()
	if want == nil {
		return ""
	}
	rib, err := route.FetchRIB(syscall.AF_INET, syscall.NET_RT_FLAGS, syscall.RTF_LLINFO)
	if err != nil {
		return ""
	}
	msgs, err := route.ParseRIB(syscall.NET_RT_FLAGS, rib)
	if err != nil {
		return ""
	}
	for _, m := range msgs {
		rm, ok := m.(*route.RouteMessage)
		if !ok || len(rm.Addrs) <= syscall.RTAX_GATEWAY {
			continue
		}
		dst, ok := rm.Addrs[syscall.RTAX_DST].(*route.Inet4Addr)
		if !ok || !net.IP(dst.IP[:]).Equal(want) {
			continue
		}
		if ll, ok := rm.Addrs[syscall.RTAX_GATEWAY].(*route.LinkAddr); ok && len(ll.Addr) == 6 {
			a := ll.Addr
			return normalizeMAC(fmt.Sprintf("%x:%x:%x:%x:%x:%x", a[0], a[1], a[2], a[3], a[4], a[5]))
		}
	}
	return ""
}
