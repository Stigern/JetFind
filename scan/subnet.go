package scan

import (
	"encoding/binary"
	"net"
)

// maxPrefix is the widest network we scan as-is. Anything larger (e.g. a /16)
// is narrowed to the /24 around our own address to keep scans quick.
const maxPrefix = 22

// LocalSubnets returns the IPv4 networks of the machine's active,
// non-loopback, non-point-to-point interfaces (VPN tunnels are skipped).
func LocalSubnets() ([]*net.IPNet, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var nets []*net.IPNet
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagPointToPoint != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil || ipnet.IP.IsLinkLocalUnicast() {
				continue
			}
			n := narrow(ipnet)
			if !seen[n.String()] {
				seen[n.String()] = true
				nets = append(nets, n)
			}
		}
	}
	return nets, nil
}

// narrow masks an interface address to its network, shrinking it to a /24
// when the prefix is shorter than maxPrefix.
func narrow(ipnet *net.IPNet) *net.IPNet {
	ip := ipnet.IP.To4()
	ones, _ := ipnet.Mask.Size()
	if ones < maxPrefix {
		ones = 24
	}
	mask := net.CIDRMask(ones, 32)
	return &net.IPNet{IP: ip.Mask(mask), Mask: mask}
}

// Hosts lists the usable host addresses of an IPv4 network, excluding the
// network and broadcast addresses for prefixes shorter than /31.
func Hosts(n *net.IPNet) []net.IP {
	ip := n.IP.To4()
	if ip == nil {
		return nil
	}
	ones, bits := n.Mask.Size()
	size := uint32(1) << uint(bits-ones)
	start := binary.BigEndian.Uint32(ip)
	first, last := start, start+size-1
	if size > 2 {
		first++
		last--
	}
	hosts := make([]net.IP, 0, last-first+1)
	for v := first; v <= last; v++ {
		h := make(net.IP, 4)
		binary.BigEndian.PutUint32(h, v)
		hosts = append(hosts, h)
	}
	return hosts
}
