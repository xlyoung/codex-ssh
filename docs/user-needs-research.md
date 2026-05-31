
================================================================================
USER NEEDS RESEARCH: SSH Management and AI Operations Tools
================================================================================
Sources: GitHub Issues/Discussions (10 repos, 100+ issues), HN discussions
Date: 2026-05-31

======================================================================
1. MOST REQUESTED FEATURES (ranked by frequency in issues/discussions)
======================================================================

TIER 1 - HIGH FREQUENCY (asked in multiple repos, multiple issues)

[A] Multi-Host / Dynamic Host Management
    - Multiple servers need switching without restart (ssh-mcp-server #3, #5)
    - Named aliases instead of raw IPs (#17)
    - Docker support and multi-host connection pooling (ssh-mcp #3)
    - HTTP MCP server for dynamic multi-host SSH execution (ssh-mcp)
    - Bastillion: Web-based multi-server management with key management
    - sshm: Beautiful CLI for managing many SSH hosts with search

[B] SFTP / File Transfer
    - Password-based upload failures (ssh-mcp-server #15)
    - SFTP upload-file/download-file tools (ssh-mcp)
    - SCP, SFTP browser, folder download (sshm)
    - mcp-ssh-manager: ssh_upload, ssh_download, ssh_sync tools

[C] Sudo/Su Support and Privilege Escalation
    - Interactive su support (ssh-mcp-server)
    - improve sudo implementation and disableSudo flag (ssh-mcp #1)
    - PTY Session Accumulation issues with suPassword (ssh-mcp)
    - mcp-ssh-manager: ssh_execute_sudo tool with password handling

[D] Jump Host / Bastion / Proxy Support
    - SOCKS proxy for jump server scenarios (ssh-mcp-server #8)
    - ProxyCommand support for SOCKS5 (ssh-mcp, mcp-ssh-manager)
    - ProxyJump / Bastion Host Support (mcp-ssh-manager v3.2.0)
    - Codex-ssh already has: jump host tunneling, SOCKS5 proxy

[E] Connection Pooling and Session Persistence
    - Persistent SSH Sessions for AI Assistants (HN: MCP ShellKeeper)
    - Multi-host connection pooling (ssh-mcp)
    - mcp-ssh-manager: Connection pooling with health checks

TIER 2 - MEDIUM FREQUENCY

[F] Security / Audit / Policy Control
    - Command injection vulnerabilities (ssh-mcp)
    - Info exposure through server log files (ssh-mcp - 2 CVEs)
    - Per-server readonly/restricted/unrestricted modes (mcp-ssh-manager v3.5.0)
    - Audit log with redaction (mcp-ssh-manager)
    - sudo password visible in process list (mcp-ssh-manager)
    - Bastillion: Active Directory/LDAP auth (27 comments - highest!)

[G] Windows Support
    - chmod fails, PowerShell encoding issues
    - /bin/bash shim errors on global install
    - mcp-ssh-manager v3.4.0: Windows OpenSSH fixes
    - Docker image packaging requested (Bastillion - 15 comments)

[H] Config from ~/.ssh/config
    - Read connection config from ssh config file
    - sshm: Syncs ~/.ssh/config with cloud infrastructure
    - Codex-ssh: already reads SSH config

[I] Timeout and Process Management
    - Non-blocking commands like telnet hang forever (ssh-mcp-server #6)
    - ssh_execute timeout silently capped at 30s (mcp-ssh-manager)
    - Configurable timeout with graceful process abortion (ssh-mcp)
    - Brief command descriptions for permission requests (ssh-mcp)

TIER 3 - EMERGING

[J] Pipe/Command Chaining Support
    - pipe operators blocked by command validation (ssh-mcp-server #9)
    - Users want: docker logs --since=3m app | head -50

[K] Multi-AI Agent Support
    - Choose different AI assistant per session (claude-squad)
    - Amazon Q, Gemini, multiple model support (claude-squad)
    - DevOpsGPT: Azure, OpenAI, Palm, Anthropic, Cohere

[L] 2FA / MFA Authentication
    - 2FA input verification needed (ssh-mcp-server)
    - Passphrase support for encrypted SSH keys (ssh-mcp)

[M] Session/PTY Management
    - tmux window management for SSH (purple)
    - Error capturing pane content (claude-squad - 18 comments)
    - Restore dead tmux sessions (claude-squad)

[N] Database Operations via SSH
    - ssh_db_dump, ssh_db_import tools (mcp-ssh-manager)
    - Full DevOps pipeline integration (DevOpsGPT)

[O] Deployment and Sync
    - rsync-like sync, git deploy, Docker deploy over SSH (mcp-ssh-manager)

======================================================================
2. TOP PAIN POINTS (from issues and discussions)
======================================================================

PAIN 1: Setup Complexity / Poor Onboarding
  - No setup guide for adding to agent (ssh-mcp-server)
  - Upgrade documentation missing (Bastillion - 17 comments)
  - Repeated error messages, confusing errors (DevOpsGPT)
  - Config mismatches between documentation and code

PAIN 2: Security Vulnerabilities in MCP SSH Tools
  - Command Injection via su mode description field (ssh-mcp)
  - Information Exposure Through Server Log Files (ssh-mcp - 2 CVEs)
  - Credentials accepted via command-line only (ssh-mcp)
  - sudo password visible in process list (mcp-ssh-manager)
  - HN: MCP servers mass-forked = supply-chain attack vector
  - HN: MCP-Shield for security scanning (134 points!)
  - HN: GuardiAgent sandboxing for MCP servers

PAIN 3: Connection Reliability and Error Handling
  - Connection drops (ssh-mcp-server)
  - PTY Session Accumulation causing Channel open failure (ssh-mcp)
  - Timeout waiting for shell prompt on custom shells
  - SSH ping parsing failures on Windows

PAIN 4: Cross-Platform Compatibility
  - Windows: chmod fails, PowerShell encoding, /bin/bash shim
  - macOS: Input conflict with tmux prefix
  - Various shell compatibility issues (fish, zsh, oh-my-zsh)

PAIN 5: LLM Agent Integration Friction
  - Tool exec not found errors across different AI clients
  - No tools, prompts, or resources display issues
  - No unified way to configure across different AI clients
  - Multiple Chinese users confused about setup

PAIN 6: Scalability at Server Count
  - Managing 100+ servers is painful
  - No good way to organize/group/tag servers
  - Search/filter servers is basic or missing

PAIN 7: Command Output Parsing Issues
  - Logger writing to stdout breaks MCP JSON-RPC
  - Shell prompt detection fragile across different distros
  - Custom prompts (fish, zsh with oh-my-zsh) break detection

======================================================================
3. WHAT WOULD MAKE USERS SWITCH FROM EXISTING TOOLS
======================================================================

SWITCH FACTOR 1: True Zero Config SSH Management
  - Read from ~/.ssh/config automatically
  - Support SSH aliases natively
  - Auto-detect auth methods (key, password, agent)

SWITCH FACTOR 2: Security-First Architecture
  - Credentials never exposed to AI models
  - Per-server permission policies
  - Audit logging with redaction
  - Sandboxed command execution
  - HN: MCP-Shield (134 points) shows massive security demand

SWITCH FACTOR 3: Native Multi-Agent Support
  - Multiple AI assistants on same servers
  - Session isolation between different AI agents
  - Claude Squad / Codex / Claude Code / OpenCode compatibility

SWITCH FACTOR 4: Production-Grade Reliability
  - Connection pooling with health checks
  - Automatic reconnection
  - Timeout handling with process cleanup
  - Real-time monitoring and status dashboards

SWITCH FACTOR 5: Unified Operations Console
  - Not just SSH execution, but full operations workflow:
    * Server inventory/discovery
    * Health monitoring
    * Log aggregation
    * Backup/restore
    * Deployment pipelines
    * Incident response
  - mcp-ssh-manager leads with 37 tools
  - Codex-ssh goal: unmanned operations platform

======================================================================
4. COMPETITIVE LANDSCAPE SUMMARY
======================================================================

Tool                  | Stars | Language | Key Strength
---------------------|-------|----------|------------------
Claude Squad          | 7,669 | Go       | Multi-agent AI terminal mgmt
DevOpsGPT             | 5,959 | Python   | Full DevOps AI pipeline
Bastillion            | 3,471 | Java     | Web SSH console + key mgmt
sshm                  | 1,190 | Rust     | Beautiful CLI SSH manager
ssh-mcp-server        |   504 | TS/Node  | Simple MCP SSH bridge
ssh-mcp               |   474 | TS/Node  | MCP SSH with sudo/su
mcp-ssh-manager       |   225 | TS/Node  | Most feature-rich (37 tools)

KEY INSIGHT: mcp-ssh-manager (225 stars) has the most features but lowest
stars. Simpler tools with better UX get more traction. The Go-based
claude-squad (7,669 stars) shows Go + TUI is the winning combination.

======================================================================
5. RECOMMENDED PRIORITY FEATURES FOR CODEX-SSH
======================================================================

MUST HAVE (v0.2.0):
  1. SFTP file transfer (upload/download) - TOP REQUESTED
  2. Sudo/su privilege escalation support
  3. Read from ~/.ssh/config with alias support
  4. Configurable timeouts with graceful cleanup
  5. Pipe/chain command support

SHOULD HAVE (v0.3.0):
  6. Per-server security policies (readonly/restricted modes)
  7. Connection pooling with health checks
  8. Audit logging with sensitive data redaction
  9. 2FA/MFA authentication support
  10. Database backup/restore over SSH

NICE TO HAVE (v0.4.0):
  11. Interactive TUI server dashboard (like Claude Squad)
  12. Multiple AI agent session management
  13. Deployment workflow (rsync, git deploy, docker)
  14. Server health monitoring (uptime, disk, memory, CPU)
  15. Incident response playbooks

UNIQUE ADVANTAGE (codex-ssh differentiator):
  - Go binary = single binary, no Node.js/npm dependency
  - Already has: MCP server, parallel exec, jump host, SOCKS5, audit
  - Position as the Ansible for AI agents - production-grade SSH
  - Security-first: built-in policies, audit, credential isolation

