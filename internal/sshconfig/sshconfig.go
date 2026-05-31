package sshconfig

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"codex-ssh-skill/pkg/model"
)

func Parse(r io.Reader) (model.Inventory, error) {
	inv := model.Inventory{
		Version: 1,
		Hosts:   map[string]model.Host{},
	}

	scanner := bufio.NewScanner(r)
	currentAliases := []string{}
	currentValues := map[string]string{}

	flush := func() error {
		if len(currentAliases) == 0 {
			return nil
		}
		for _, alias := range currentAliases {
			host := buildHost(alias, currentValues)
			if host.Host == "" {
				continue
			}
			inv.Hosts[alias] = host
		}
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := splitDirective(line)
		if !ok {
			continue
		}
		if strings.EqualFold(key, "Host") {
			if err := flush(); err != nil {
				return model.Inventory{}, err
			}
			currentAliases = filterAliases(strings.Fields(value))
			currentValues = map[string]string{}
			continue
		}
		if len(currentAliases) == 0 {
			continue
		}
		currentValues[strings.ToLower(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return model.Inventory{}, err
	}
	if err := flush(); err != nil {
		return model.Inventory{}, err
	}
	return inv, nil
}

func buildHost(alias string, values map[string]string) model.Host {
	host := model.Host{
		Host:         firstNonEmpty(values["hostname"], alias),
		User:         values["user"],
		IdentityFile: values["identityfile"],
	}
	if port := strings.TrimSpace(values["port"]); port != "" {
		if parsed, err := strconv.Atoi(port); err == nil {
			host.Port = parsed
		}
	}
	if proxyJump := strings.TrimSpace(values["proxyjump"]); proxyJump != "" {
		host.Via = parseProxyJump(proxyJump)
	}
	if host.IdentityFile != "" {
		host.Auth = "identity_file"
	}
	return host
}

func parseProxyJump(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		host := part
		if _, hostOnly, ok := strings.Cut(part, "@"); ok {
			host = hostOnly
		}
		if strings.Contains(host, ":") {
			host, _, _ = strings.Cut(host, ":")
		}
		out = append(out, host)
	}
	return out
}

func splitDirective(line string) (string, string, bool) {
	index := strings.IndexAny(line, " \t")
	if index <= 0 {
		return "", "", false
	}
	return line[:index], strings.TrimSpace(line[index+1:]), true
}

func filterAliases(aliases []string) []string {
	out := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if alias == "" || strings.ContainsAny(alias, "*?") {
			continue
		}
		out = append(out, alias)
	}
	return out
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return item
		}
	}
	return ""
}

func Merge(dst model.Inventory, src model.Inventory) model.Inventory {
	if dst.Hosts == nil {
		dst.Hosts = map[string]model.Host{}
	}
	for alias, host := range src.Hosts {
		dst.Hosts[alias] = host
	}
	return dst
}

func LoadFile(path string) (model.Inventory, error) {
	file, err := os.Open(path)
	if err != nil {
		return model.Inventory{}, fmt.Errorf("open ssh config: %w", err)
	}
	defer file.Close()
	return Parse(file)
}
