package ops

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strings"
)

// LocalIPs lists this machine's IPv4 addresses, the one it uses to reach the
// internet first. No packet is sent: "dialing" UDP only picks a route.
func LocalIPs() []string {
	var out []string
	if c, err := net.Dial("udp", "8.8.8.8:53"); err == nil {
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok && a.IP.To4() != nil {
			out = append(out, a.IP.String())
		}
		c.Close()
	}
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil || n.IP.IsLinkLocalUnicast() {
				continue
			}
			if ip := n.IP.String(); !slices.Contains(out, ip) {
				out = append(out, ip)
			}
		}
	}
	return out
}

// IsPrivate reports whether the URL's host is an address that only works
// inside a local network (192.168.x.x and the like, or localhost).
func IsPrivate(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Hostname() == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(u.Hostname())
	return err == nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}

// NormalizeHubURL turns what an admin typed ("85.10.20.30", "85.10.20.30:8080",
// "http://...") into a base URL; a bare host gets the players' default port.
func NormalizeHubURL(s, defaultPort string) (string, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" {
		return "", nil
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" {
		return "", errors.New("invalid hub address")
	}
	if u.Port() == "" && u.Scheme == "http" && defaultPort != "" && defaultPort != "80" {
		u.Host = net.JoinHostPort(u.Hostname(), defaultPort)
	}
	return u.String(), nil
}
