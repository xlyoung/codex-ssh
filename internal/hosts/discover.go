package hosts

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// DiscoveredHost represents a host found during network scanning.
type DiscoveredHost struct {
	Host   string
	Port   int
	Banner string
}

// Discover scans a CIDR range for hosts with an open port.
// It attempts an SSH handshake to verify the service.
func Discover(cidr string, port int, timeout time.Duration) ([]DiscoveredHost, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse CIDR: %w", err)
	}

	// Collect all IPs in the range
	var ips []net.IP
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); ip = incIP(ip) {
		ipCopy := make(net.IP, len(ip))
		copy(ipCopy, ip)
		ips = append(ips, ipCopy)
	}

	if len(ips) == 0 {
		return nil, nil
	}

	var discovered []DiscoveredHost
	var mu sync.Mutex
	var wg sync.WaitGroup

	sem := make(chan struct{}, 64)

	for _, targetIP := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(addr net.IP) {
			defer func() { <-sem; wg.Done() }()

			addrStr := addr.String()
			target := fmt.Sprintf("%s:%d", addrStr, port)

			conn, err := net.DialTimeout("tcp", target, timeout)
			if err != nil {
				return
			}
			defer conn.Close()

			// Set deadline for SSH handshake
			conn.SetDeadline(time.Now().Add(timeout))

			// Try SSH handshake to verify it's a real SSH server
			banner := "tcp-open"
			sshConn, _, _, err := ssh.NewClientConn(conn, target, &ssh.ClientConfig{
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			})
			if err == nil {
				banner = string(sshConn.ServerVersion())
				sshConn.Close()
			}

			mu.Lock()
			discovered = append(discovered, DiscoveredHost{
				Host:   addrStr,
				Port:   port,
				Banner: banner,
			})
			mu.Unlock()
		}(targetIP)
	}
	wg.Wait()

	sort.Slice(discovered, func(i, j int) bool {
		return ipToUint32(discovered[i].Host) < ipToUint32(discovered[j].Host)
	})

	return discovered, nil
}

func incIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for j := len(next) - 1; j >= 0; j-- {
		next[j]++
		if next[j] > 0 {
			break
		}
	}
	return next
}

func ipToUint32(ip string) uint32 {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return 0
	}
	var result uint32
	for _, part := range parts {
		v, _ := strconv.Atoi(part)
		result = result*256 + uint32(v)
	}
	return result
}
