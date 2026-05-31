package keys

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// KeyInfo represents an SSH key with metadata
type KeyInfo struct {
	Path      string    `json:"path"`
	PublicKey string    `json:"public_key,omitempty"`
	Type      string    `json:"type"`       // e.g., "ed25519", "rsa", "ecdsa"
	Comment   string    `json:"comment,omitempty"`
	Fingerprint string  `json:"fingerprint,omitempty"`
	InAgent   bool      `json:"in_agent"`   // loaded in SSH agent
	InKeychain bool     `json:"in_keychain"` // stored in OS keychain
	Modified  time.Time `json:"modified"`
}

// Manager handles SSH keys across platforms
type Manager struct {
	sshDir string
}

// NewManager creates a key manager for the current user
func NewManager() *Manager {
	home, _ := os.UserHomeDir()
	return &Manager{
		sshDir: filepath.Join(home, ".ssh"),
	}
}

// ListKeys discovers all SSH keys from multiple sources
func (m *Manager) ListKeys(ctx context.Context) ([]KeyInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	keys := make(map[string]KeyInfo)

	// 1. Discover key files from ~/.ssh/
	fileKeys, err := m.discoverKeyFiles()
	if err == nil {
		for _, k := range fileKeys {
			keys[k.Path] = k
		}
	}

	// 2. Check SSH agent for loaded keys
	agentKeys, err := m.queryAgent(ctx)
	if err == nil {
		for _, ak := range agentKeys {
			if existing, ok := keys[ak.Path]; ok {
				existing.InAgent = true
				if ak.Fingerprint != "" {
					existing.Fingerprint = ak.Fingerprint
				}
				keys[ak.Path] = existing
			} else {
				ak.InAgent = true
				keys[ak.Path] = ak
			}
		}
	}

	// 3. Check OS keychain
	keychainKeys, err := m.queryKeychain(ctx)
	if err == nil {
		for _, ck := range keychainKeys {
			if existing, ok := keys[ck.Path]; ok {
				existing.InKeychain = true
				keys[ck.Path] = existing
			} else {
				ck.InKeychain = true
				keys[ck.Path] = ck
			}
		}
	}

	// Sort by path
	result := make([]KeyInfo, 0, len(keys))
	for _, k := range keys {
		result = append(result, k)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result, nil
}

// discoverKeyFiles finds private key files in ~/.ssh/
func (m *Manager) discoverKeyFiles() ([]KeyInfo, error) {
	entries, err := os.ReadDir(m.sshDir)
	if err != nil {
		return nil, err
	}

	var keys []KeyInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip public keys, known_hosts, config, etc.
		if strings.HasSuffix(name, ".pub") ||
			name == "known_hosts" ||
			name == "config" ||
			name == "authorized_keys" ||
			name == "environment" ||
			strings.HasSuffix(name, ".bak") {
			continue
		}

		path := filepath.Join(m.sshDir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		// Check if it looks like a private key
		if !isPrivateKey(path) {
			continue
		}

		keyType := detectKeyType(path)
		comment := detectComment(path)
		fingerprint := m.getFingerprint(path)

		keys = append(keys, KeyInfo{
			Path:        path,
			Type:        keyType,
			Comment:     comment,
			Fingerprint: fingerprint,
			InAgent:     false,
			InKeychain:  false,
			Modified:    info.ModTime(),
		})
	}
	return keys, nil
}

// queryAgent asks ssh-agent for loaded keys
func (m *Manager) queryAgent(ctx context.Context) ([]KeyInfo, error) {
	cmd := exec.CommandContext(ctx, "ssh-add", "-l")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// ssh-add -l returns exit code 1 when no keys loaded
		return nil, fmt.Errorf("ssh-agent not running or no keys")
	}

	var keys []KeyInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "2048 SHA256:xxxx /path/to/key (comment)"
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		fingerprint := parts[1]
		path := parts[2]

		// Check if path is absolute, otherwise try to match
		if !filepath.IsAbs(path) {
			path = filepath.Join(m.sshDir, path)
		}

		// Extract comment if present
		comment := ""
		if len(parts) >= 4 {
			comment = strings.Trim(strings.Join(parts[3:], " "), "()\n")
		}

		keyType := ""
		if strings.HasPrefix(fingerprint, "SHA256:") {
			keyType = "unknown" // can't determine from fingerprint alone
		}

		keys = append(keys, KeyInfo{
			Path:        path,
			Type:        keyType,
			Comment:     comment,
			Fingerprint: fingerprint,
			InAgent:     true,
		})
	}
	return keys, nil
}

// queryKeychain checks OS-specific keychain for SSH keys
func (m *Manager) queryKeychain(ctx context.Context) ([]KeyInfo, error) {
	switch {
	case isDarwin():
		return m.queryMacOSKeychain(ctx)
	case isLinux():
		return m.querySecretService(ctx)
	default:
		return nil, fmt.Errorf("unsupported platform for keychain")
	}
}

// queryMacOSKeychain checks macOS Keychain for SSH key passphrases
func (m *Manager) queryMacOSKeychain(ctx context.Context) ([]KeyInfo, error) {
	// List keychain entries related to SSH
	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-s", "ssh", "-g", "-a", "codex-ssh", "-w")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("no keychain entries found: %s", string(out))
	}

	var keys []KeyInfo
	// Parse keychain entries (simplified)
	_ = strings.TrimSpace(string(out))
	return keys, nil
}

// querySecretService checks Linux Secret Service (GNOME Keyring, etc.)
func (m *Manager) querySecretService(ctx context.Context) ([]KeyInfo, error) {
	// Try secret-tool if available
	cmd := exec.CommandContext(ctx, "secret-tool", "search",
		"application", "codex-ssh")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("secret-service not available: %s", string(out))
	}

	var keys []KeyInfo
	_ = strings.TrimSpace(string(out))
	return keys, nil
}

// getFingerprint gets the SHA256 fingerprint of a key
func (m *Manager) getFingerprint(path string) string {
	cmd := exec.Command("ssh-keygen", "-lf", path, "-E", "sha256")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	parts := strings.Fields(string(out))
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// GetAgentSocket returns the SSH agent socket path
func GetAgentSocket() string {
	if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
		return socket
	}
	return ""
}

// IsAgentRunning checks if SSH agent is running
func IsAgentRunning(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "ssh-add", "-l")
	err := cmd.Run()
	return err == nil
}

// ExportKey exports a key's public portion
func (m *Manager) ExportKey(ctx context.Context, path string) (string, error) {
	pubPath := path + ".pub"
	data, err := os.ReadFile(pubPath)
	if err != nil {
		// Try ssh-keygen to extract public key
		cmd := exec.CommandContext(ctx, "ssh-keygen", "-y", "-f", path)
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("cannot read public key: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	return strings.TrimSpace(string(data)), nil
}

// isPrivateKey checks if a file looks like an SSH private key
func isPrivateKey(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	buf := make([]byte, 100)
	n, err := file.Read(buf)
	if err != nil || n < 10 {
		return false
	}

	header := string(buf[:n])
	return strings.Contains(header, "OPENSSH PRIVATE KEY") ||
		strings.Contains(header, "RSA PRIVATE KEY") ||
		strings.Contains(header, "EC PRIVATE KEY") ||
		strings.Contains(header, "DSA PRIVATE KEY") ||
		strings.Contains(header, "BEGIN PRIVATE KEY")
}

// detectKeyType detects the key type from file content
func detectKeyType(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return "unknown"
	}
	defer file.Close()

	buf := make([]byte, 200)
	n, err := file.Read(buf)
	if err != nil {
		return "unknown"
	}

	header := string(buf[:n])
	switch {
	case strings.Contains(header, "openssh-key-v1"):
		// Modern OpenSSH format - check key type from base64
		re := regexp.MustCompile(`ssh-(rsa|ed25519|dss|ecdsa)`)
		if match := re.FindString(header); match != "" {
			return strings.TrimPrefix(match, "ssh-")
		}
		return "openssh"
	case strings.Contains(header, "RSA PRIVATE KEY"):
		return "rsa"
	case strings.Contains(header, "EC PRIVATE KEY"):
		return "ecdsa"
	case strings.Contains(header, "DSA PRIVATE KEY"):
		return "dsa"
	}
	return "unknown"
}

// detectComment extracts the key comment from file content
func detectComment(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil {
		return ""
	}

	// Look for comment pattern in OpenSSH key
	content := string(buf[:n])
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Last line of base64 block often has comment
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		// Check for comment at end
		re := regexp.MustCompile(`#\s*(.+)$`)
		if match := re.FindStringSubmatch(line); len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}
