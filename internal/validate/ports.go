package validate

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func EnsurePortAvailable(host string, port int) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("local port %s:%d is unavailable: %w", host, port, err)
	}
	return listener.Close()
}

func ParseTarget(target string) (string, int, error) {
	host, portString, ok := strings.Cut(target, ":")
	if !ok || host == "" || portString == "" {
		return "", 0, fmt.Errorf("invalid target, expected host:port")
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return "", 0, fmt.Errorf("invalid target port: %w", err)
	}
	return host, port, nil
}
