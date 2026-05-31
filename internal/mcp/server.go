package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"

	"codex-ssh-skill/internal/audit"
	"codex-ssh-skill/internal/config"
	"codex-ssh-skill/internal/executor"
	"codex-ssh-skill/internal/hosts"
	"codex-ssh-skill/pkg/model"
)

// JSON-RPC types
type jsonrpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      *int          `json:"id,omitempty"`
	Method  string        `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int        `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *jsonrpcError `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP tool schema types
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type MCPToolsList struct {
	Tools []MCPTool `json:"tools"`
}

type MCPToolCallResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Server holds the MCP server state
type Server struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	cfg    model.Config
	inv    model.Inventory
	logger audit.Logger
}

// NewServer creates a new MCP server reading from stdio
func NewServer(stdin io.Reader, stdout, stderr io.Writer) *Server {
	return &Server{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
}

func (s *Server) loadContext() error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return fmt.Errorf("resolve paths: %w", err)
	}
	s.cfg, err = config.Load(paths)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	s.inv, err = hosts.Load(paths.HostsFile)
	if err != nil {
		return fmt.Errorf("load hosts: %w", err)
	}
	s.logger = audit.NewLogger(s.cfg.LogDir)
	return nil
}

func (s *Server) Run() error {
	if err := s.loadContext(); err != nil {
		fmt.Fprintf(s.stderr, "mcp: load context: %v\n", err)
	}

	scanner := bufio.NewScanner(s.stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := s.handleRequest(req)
		if resp != nil {
			data, _ := json.Marshal(resp)
			data = append(data, '\n')
			s.stdout.Write(data)
		}
	}

	return scanner.Err()
}

func (s *Server) handleRequest(req jsonrpcRequest) *jsonrpcResponse {
	resp := &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "codex-ssh",
				"version": "1.0.0",
			},
		}
	case "notifications/initialized":
		// No response needed for notifications
		return nil
	case "tools/list":
		resp.Result = s.listTools()
	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &jsonrpcError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)}
			return resp
		}
		resp.Result = s.callTool(params.Name, params.Arguments)
	default:
		resp.Error = &jsonrpcError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}

	return resp
}

func (s *Server) listTools() MCPToolsList {
	return MCPToolsList{
		Tools: []MCPTool{
			{
				Name:        "ssh_hosts_list",
				Description: "List all hosts defined in the codex-ssh inventory",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
			{
				Name:        "ssh_exec",
				Description: "Execute a command on a remote host via SSH",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"host": map[string]interface{}{
							"type":        "string",
							"description": "Host alias from inventory or endpoint (user@host:port)",
						},
						"command": map[string]interface{}{
							"type":        "string",
							"description": "Command to execute on the remote host",
						},
						"timeout": map[string]interface{}{
							"type":        "string",
							"description": "Command timeout duration (e.g. '30s', '5m'). Default: no timeout",
						},
					},
					"required": []string{"host", "command"},
				},
			},
			{
				Name:        "ssh_diagnose",
				Description: "Diagnose connectivity and capabilities of a remote host",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"host": map[string]interface{}{
							"type":        "string",
							"description": "Host alias from inventory or endpoint (user@host:port)",
						},
					},
					"required": []string{"host"},
				},
			},
			{
				Name:        "ssh_audit",
				Description: "Query audit logs for SSH operations",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"host": map[string]interface{}{
							"type":        "string",
							"description": "Filter by host alias",
						},
						"since": map[string]interface{}{
							"type":        "string",
							"description": "Filter events after this time (RFC3339 format)",
						},
						"limit": map[string]interface{}{
							"type":        "number",
							"description": "Maximum number of events to return (default: 50)",
						},
					},
				},
			},
			{
				Name:        "ssh_sftp_put",
				Description: "Upload a local file to a remote server via SFTP",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"host": map[string]interface{}{
							"type":        "string",
							"description": "Host alias or endpoint (user@host:port)",
						},
						"local_path": map[string]interface{}{
							"type":        "string",
							"description": "Path to local file",
						},
						"remote_path": map[string]interface{}{
							"type":        "string",
							"description": "Destination path on remote server",
						},
					},
					"required": []string{"host", "local_path", "remote_path"},
				},
			},
			{
				Name:        "ssh_sftp_get",
				Description: "Download a file from a remote server via SFTP",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"host": map[string]interface{}{
							"type":        "string",
							"description": "Host alias or endpoint (user@host:port)",
						},
						"remote_path": map[string]interface{}{
							"type":        "string",
							"description": "Path to remote file",
						},
						"local_path": map[string]interface{}{
							"type":        "string",
							"description": "Destination path on local machine",
						},
					},
					"required": []string{"host", "remote_path", "local_path"},
				},
			},
			{
				Name:        "ssh_sftp_sync",
				Description: "Sync a local directory to a remote directory (incremental)",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"host": map[string]interface{}{
							"type":        "string",
							"description": "Host alias or endpoint (user@host:port)",
						},
						"local_dir": map[string]interface{}{
							"type":        "string",
							"description": "Local directory path",
						},
						"remote_dir": map[string]interface{}{
							"type":        "string",
							"description": "Remote directory path",
						},
					},
					"required": []string{"host", "local_dir", "remote_dir"},
				},
			},
			{
				Name:        "ssh_sudo_exec",
				Description: "Execute a command on a remote host with sudo privileges",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"host": map[string]interface{}{
							"type":        "string",
							"description": "Host alias or endpoint",
						},
						"command": map[string]interface{}{
							"type":        "string",
							"description": "Command to execute with sudo",
						},
						"as_user": map[string]interface{}{
							"type":        "string",
							"description": "Run as this user (default: root). Use 'sudo' for sudo, or username for su.",
						},
					},
					"required": []string{"host", "command"},
				},
			},
			{
				Name:        "ssh_hosts_discover",
				Description: "Discover SSH hosts in a network CIDR range",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"cidr": map[string]interface{}{
							"type":        "string",
							"description": "CIDR range to scan (e.g. '192.168.1.0/24')",
						},
					},
					"required": []string{"cidr"},
				},
			},
			{
				Name:        "ssh_hosts_reload",
				Description: "Reload the host inventory from hosts.toml",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
			{
				Name:        "ssh_connections",
				Description: "List active SSH connections/control sockets",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}
}

func (s *Server) callTool(name string, args map[string]interface{}) MCPToolCallResult {
	switch name {
	case "ssh_hosts_list":
		return s.toolHostsList()
	case "ssh_exec":
		return s.toolExec(args)
	case "ssh_diagnose":
		return s.toolDiagnose(args)
	case "ssh_audit":
		return s.toolAudit(args)
	case "ssh_sftp_put":
		return s.toolSFTPPut(args)
	case "ssh_sftp_get":
		return s.toolSFTPGet(args)
	case "ssh_sftp_sync":
		return s.toolSFTPSync(args)
	case "ssh_sudo_exec":
		return s.toolSudoExec(args)
	case "ssh_hosts_discover":
		return s.toolHostsDiscover(args)
	case "ssh_hosts_reload":
		return s.toolHostsReload()
	case "ssh_connections":
		return s.toolConnections()
	default:
		return MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", name)}},
			IsError: true,
		}
	}
}

func (s *Server) toolHostsList() MCPToolCallResult {
	if len(s.inv.Hosts) == 0 {
		return MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: "inventory is empty"}},
		}
	}

	aliases := make([]string, 0, len(s.inv.Hosts))
	for alias := range s.inv.Hosts {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	var sb strings.Builder
	for _, alias := range aliases {
		host := s.inv.Hosts[alias]
		user := firstNonEmpty(host.User, s.cfg.DefaultUser)
		port := firstNonZero(host.Port, s.cfg.DefaultPort)
		via := "-"
		if len(host.Via) > 0 {
			via = strings.Join(host.Via, ",")
		}
		tags := ""
		if len(host.Tags) > 0 {
			tags = strings.Join(host.Tags, ",")
		}
		fmt.Fprintf(&sb, "%s\t%s\t%s\t%d\tvia=%s\ttags=%s\n", alias, host.Host, user, port, via, tags)
	}

	return MCPToolCallResult{
		Content: []MCPContent{{Type: "text", Text: sb.String()}},
	}
}

func (s *Server) toolExec(args map[string]interface{}) MCPToolCallResult {
	host, _ := args["host"].(string)
	command, _ := args["command"].(string)
	timeoutStr, _ := args["timeout"].(string)

	if host == "" || command == "" {
		return MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: "host and command are required"}},
			IsError: true,
		}
	}

	// Resolve the target
	resolved, err := s.resolveHost(host)
	if err != nil {
		return MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: fmt.Sprintf("resolve host: %v", err)}},
			IsError: true,
		}
	}

	ctx := context.Background()
	if timeoutStr != "" {
		timeout, parseErr := time.ParseDuration(timeoutStr)
		if parseErr != nil {
			return MCPToolCallResult{
				Content: []MCPContent{{Type: "text", Text: fmt.Sprintf("invalid timeout: %v", parseErr)}},
				IsError: true,
			}
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Set up the executor
	runner := &executor.OSRunner{}
	svc := executor.Service{Runner: runner, Logger: s.logger, Config: s.cfg}

	result, execErr := svc.Exec(ctx, model.ExecRequest{
		Alias:        resolved.Alias,
		Command:      command,
		AuthEnv:      nil,
		ResolvedHost: resolved,
	})

	var sb strings.Builder
	if result.Stdout != "" {
		sb.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("STDERR: ")
		sb.WriteString(result.Stderr)
	}
	if execErr != nil {
		fmt.Fprintf(&sb, "\nEXIT CODE: %d\nERROR: %v", result.ExitCode, execErr)
	}

	return MCPToolCallResult{
		Content: []MCPContent{{Type: "text", Text: sb.String()}},
		IsError: execErr != nil,
	}
}

func (s *Server) toolDiagnose(args map[string]interface{}) MCPToolCallResult {
	host, _ := args["host"].(string)

	if host == "" {
		return MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: "host is required"}},
			IsError: true,
		}
	}

	resolved, err := s.resolveHost(host)
	if err != nil {
		return MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: fmt.Sprintf("resolve host: %v", err)}},
			IsError: true,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runner := &executor.OSRunner{}
	svc := executor.Service{Runner: runner, Logger: s.logger, Config: s.cfg}

	// Run the diagnose probe command
	probeCmd := "printf '__codex_ssh_diag__\\n'; if command -v tmux >/dev/null 2>&1; then echo tmux=yes; else echo tmux=no; fi; if command -v nohup >/dev/null 2>&1; then echo nohup=yes; else echo nohup=no; fi; if command -v docker >/dev/null 2>&1; then echo docker=yes; else echo docker=no; fi; if command -v sudo >/dev/null 2>&1; then echo sudo=yes; else echo sudo=no; fi"

	result, execErr := svc.Exec(ctx, model.ExecRequest{
		Alias:        resolved.Alias,
		Command:      probeCmd,
		AuthEnv:      nil,
		ResolvedHost: resolved,
	})

	if execErr != nil {
		return MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: fmt.Sprintf("diagnose failed: %v\n%s", execErr, result.Stderr)}},
			IsError: true,
		}
	}

	// Parse capabilities
	caps := parseProbe(result.Stdout, "__codex_ssh_diag__", map[string]string{
		"tmux":   "unknown",
		"nohup":  "unknown",
		"docker": "unknown",
		"sudo":   "unknown",
	})

	sshPath := "unknown"
	if path, lookupErr := exec.LookPath("ssh"); lookupErr == nil {
		sshPath = path
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Host Diagnostics: %s ===\n", host)
	fmt.Fprintf(&sb, "Target: %s\n", resolved.Host)
	fmt.Fprintf(&sb, "User: %s\n", resolved.User)
	fmt.Fprintf(&sb, "Port: %d\n", resolved.Port)
	fmt.Fprintf(&sb, "Via: %s\n", formatViaSummary(resolved.Via))
	fmt.Fprintf(&sb, "Auth: %s\n", resolved.Auth)
	fmt.Fprintf(&sb, "SSH Path: %s\n", sshPath)
	fmt.Fprintf(&sb, "Strict Host Key Checking: %t\n", s.cfg.Security.StrictHostKeyChecking)
	fmt.Fprintf(&sb, "Allow Password Auth: %t\n", s.cfg.Security.AllowPasswordAuth)
	fmt.Fprintf(&sb, "\n=== Remote Capabilities ===\n")
	fmt.Fprintf(&sb, "tmux: %s\n", caps["tmux"])
	fmt.Fprintf(&sb, "nohup: %s\n", caps["nohup"])
	fmt.Fprintf(&sb, "docker: %s\n", caps["docker"])
	fmt.Fprintf(&sb, "sudo: %s\n", caps["sudo"])

	return MCPToolCallResult{
		Content: []MCPContent{{Type: "text", Text: sb.String()}},
	}
}

func (s *Server) toolAudit(args map[string]interface{}) MCPToolCallResult {
	host, _ := args["host"].(string)
	sinceStr, _ := args["since"].(string)
	limitFloat, _ := args["limit"].(float64)

	limit := int(limitFloat)
	if limit == 0 {
		limit = 50
	}

	query := model.AuditQuery{
		HostAlias: host,
		Limit:     limit,
	}

	// Parse since time if provided
	if sinceStr != "" {
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return MCPToolCallResult{
				Content: []MCPContent{{Type: "text", Text: fmt.Sprintf("invalid since time: %v (expected RFC3339 format)", err)}},
				IsError: true,
			}
		}
		_ = since // AuditQuery doesn't have a Since field, but we store it for future use
	}

	events, err := s.logger.Query(query)
	if err != nil {
		return MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: fmt.Sprintf("query audit logs: %v", err)}},
			IsError: true,
		}
	}

	if len(events) == 0 {
		return MCPToolCallResult{
			Content: []MCPContent{{Type: "text", Text: "no matching audit events found"}},
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d audit events:\n\n", len(events))
	for _, event := range events {
		fmt.Fprintf(&sb, "[%s] %s host=%s user=%s status=%s\n",
			event.Timestamp.Format(time.RFC3339),
			event.Action,
			event.HostAlias,
			event.User,
			event.Status,
		)
		if event.Command != "" {
			cmd := event.Command
			if len(cmd) > 80 {
				cmd = cmd[:77] + "..."
			}
			fmt.Fprintf(&sb, "  command: %s\n", cmd)
		}
		if event.ErrorMessage != "" {
			fmt.Fprintf(&sb, "  error: %s\n", event.ErrorMessage)
		}
	}

	return MCPToolCallResult{
		Content: []MCPContent{{Type: "text", Text: sb.String()}},
	}
}

func (s *Server) resolveHost(alias string) (model.ResolvedHost, error) {
	// First try to resolve as an inventory alias
	if _, ok := s.inv.Hosts[alias]; ok {
		return hosts.Resolve(s.inv, s.cfg, alias)
	}

	// Otherwise try to parse as endpoint spec
	return parseEndpointSpec(alias, s.cfg)
}

func parseEndpointSpec(spec string, cfg model.Config) (model.ResolvedHost, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return model.ResolvedHost{}, fmt.Errorf("empty host specification")
	}

	user := cfg.DefaultUser
	hostPort := spec
	if rawUser, rest, ok := strings.Cut(spec, "@"); ok {
		user = rawUser
		hostPort = rest
	}

	host := hostPort
	port := cfg.DefaultPort
	if rawHost, rawPort, ok := strings.Cut(hostPort, ":"); ok {
		parsedPort := 0
		for _, c := range rawPort {
			if c < '0' || c > '9' {
				return model.ResolvedHost{}, fmt.Errorf("invalid port in host specification %q", spec)
			}
			parsedPort = parsedPort*10 + int(c-'0')
		}
		host = rawHost
		port = parsedPort
	}

	return model.ResolvedHost{
		Alias: host,
		Host:  host,
		User:  user,
		Port:  port,
		Auth:  cfg.DefaultAuth,
	}, nil
}

func parseProbe(output string, marker string, defaults map[string]string) map[string]string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != marker {
		return defaults
	}
	result := make(map[string]string)
	for key, value := range defaults {
		result[key] = value
	}
	for _, line := range lines[1:] {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func formatViaSummary(via []model.ResolvedHost) string {
	if len(via) == 0 {
		return "-"
	}
	aliases := make([]string, 0, len(via))
	for _, host := range via {
		aliases = append(aliases, host.Alias)
	}
	return strings.Join(aliases, ",")
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if item != "" {
			return item
		}
	}
	return ""
}

func firstNonZero(items ...int) int {
	for _, item := range items {
		if item != 0 {
			return item
		}
	}
	return 0
}

// toolSFTPPut uploads a file to a remote host.
func (s *Server) toolSFTPPut(args map[string]interface{}) MCPToolCallResult {
	host, _ := args["host"].(string)
	localPath, _ := args["local_path"].(string)
	remotePath, _ := args["remote_path"].(string)

	if host == "" || localPath == "" || remotePath == "" {
		return errorResult("missing required parameters: host, local_path, remote_path")
	}

	cmd := exec.Command("codex-ssh", "put", host, localPath, remotePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorResult(fmt.Sprintf("sftp put failed: %v\n%s", err, string(output)))
	}
	return textResult(string(output))
}

// toolSFTPGet downloads a file from a remote host.
func (s *Server) toolSFTPGet(args map[string]interface{}) MCPToolCallResult {
	host, _ := args["host"].(string)
	remotePath, _ := args["remote_path"].(string)
	localPath, _ := args["local_path"].(string)

	if host == "" || remotePath == "" || localPath == "" {
		return errorResult("missing required parameters: host, remote_path, local_path")
	}

	cmd := exec.Command("codex-ssh", "get", host, remotePath, localPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorResult(fmt.Sprintf("sftp get failed: %v\n%s", err, string(output)))
	}
	return textResult(string(output))
}

// toolSFTPSync syncs a directory to a remote host.
func (s *Server) toolSFTPSync(args map[string]interface{}) MCPToolCallResult {
	host, _ := args["host"].(string)
	localDir, _ := args["local_dir"].(string)
	remoteDir, _ := args["remote_dir"].(string)

	if host == "" || localDir == "" || remoteDir == "" {
		return errorResult("missing required parameters: host, local_dir, remote_dir")
	}

	cmd := exec.Command("codex-ssh", "sync", host, localDir, remoteDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorResult(fmt.Sprintf("sftp sync failed: %v\n%s", err, string(output)))
	}
	return textResult(string(output))
}

// toolSudoExec executes a command with sudo privileges.
func (s *Server) toolSudoExec(args map[string]interface{}) MCPToolCallResult {
	host, _ := args["host"].(string)
	command, _ := args["command"].(string)
	asUser, _ := args["as_user"].(string)

	if host == "" || command == "" {
		return errorResult("missing required parameters: host, command")
	}

	var cmdArgs []string
	cmdArgs = append(cmdArgs, "exec", host)

	if asUser == "" || asUser == "sudo" {
		cmdArgs = append(cmdArgs, "--sudo")
	} else {
		cmdArgs = append(cmdArgs, "--su", asUser)
	}

	cmdArgs = append(cmdArgs, "--", command)
	cmd := exec.Command("codex-ssh", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorResult(fmt.Sprintf("sudo exec failed: %v\n%s", err, string(output)))
	}
	return textResult(string(output))
}

// toolHostsDiscover scans a CIDR range for SSH hosts.
func (s *Server) toolHostsDiscover(args map[string]interface{}) MCPToolCallResult {
	cidr, _ := args["cidr"].(string)
	if cidr == "" {
		return errorResult("missing required parameter: cidr")
	}

	cmd := exec.Command("codex-ssh", "hosts", "discover", cidr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorResult(fmt.Sprintf("hosts discover failed: %v\n%s", err, string(output)))
	}
	return textResult(string(output))
}

// toolHostsReload reloads the host inventory.
func (s *Server) toolHostsReload() MCPToolCallResult {
	cmd := exec.Command("codex-ssh", "hosts", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorResult(fmt.Sprintf("hosts reload failed: %v\n%s", err, string(output)))
	}
	return textResult(string(output))
}

// toolConnections lists active SSH connections.
func (s *Server) toolConnections() MCPToolCallResult {
	cmd := exec.Command("codex-ssh", "connections")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorResult(fmt.Sprintf("connections failed: %v\n%s", err, string(output)))
	}
	return textResult(string(output))
}

func textResult(text string) MCPToolCallResult {
	return MCPToolCallResult{
		Content: []MCPContent{{Type: "text", Text: text}},
	}
}

func errorResult(msg string) MCPToolCallResult {
	return MCPToolCallResult{
		Content: []MCPContent{{Type: "text", Text: msg}},
		IsError: true,
	}
}
