package webhook

import (
	"fmt"
	"net"
	"strings"
)

// IsSafeTargetURL checks if a URL's resolved IPs are safe (not private/loopback/metadata).
// Returns error if unsafe.
func IsSafeTargetURL(rawURL string) error {
	// Strip scheme
	host := rawURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("dns lookup failed: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("no ips resolved")
	}
	for _, ip := range ips {
		if !isSafeIP(ip) {
			return fmt.Errorf("unsafe ip: %s", ip.String())
		}
	}
	return nil
}

func isSafeIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	// Cloud metadata (169.254.169.254)
	if ip.Equal(net.ParseIP("169.254.169.254")) {
		return false
	}
	// Private ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return false
		}
	}
	return true
}
