<p align="center">
  <h1 align="center">⚙️ Codex SSH</h1>
  <p align="center"><strong>AI-native SSH management — let your AI assistant manage servers for you.</strong></p>
</p>

<p align="center">
  <a href="https://github.com/xlyoung/codex-ssh/actions/workflows/ci.yml"><img src="https://github.com/xlyoung/codex-ssh/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/xlyoung/codex-ssh"><img src="https://goreportcard.com/badge/github.com/xlyoung/codex-ssh" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg" alt="License: MIT"></a>
  <a href="https://github.com/xlyoung/codex-ssh/releases"><img src="https://img.shields.io/github/v/release/xlyoung/codex-ssh" alt="Release"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/go-1.22+-blue.svg" alt="Go Version"></a>
</p>

<p align="center">
  <a href="README.md">English</a> · <a href="README_CN.md">中文</a>
</p>

---

## What is Codex SSH?

Codex SSH is an **AI-native SSH management tool** built for AI assistants like Codex, Claude, and Hermes. It provides a structured, auditable, and secure interface between AI agents and your remote servers — turning natural language into reliable infrastructure operations.

Traditional SSH tools serve humans. Codex SSH serves **AI agents** — with an inventory system, jump host chaining, keychain-backed secrets, structured audit logging, and a native **MCP (Model Context Protocol)** server that any AI tool can consume.

> **One binary. Zero dependencies. AI-first.**

---

## ✨ Features

- 🤖 **AI-Native Design** — Purpose-built for Codex, Claude, and Hermes with MCP server support
- 🔐 **Security First** — macOS Keychain secrets, Askpass injection, structured audit logs with redaction
- 🚀 **Single Go Binary** — Cross-platform, zero dependencies, ships anywhere
- 🔌 **MCP Protocol** — Standard AI tool interface (`ssh_exec`, `ssh_diagnose`, `ssh_hosts_list`, `ssh_audit`)
- 🌐 **Jump Host Tunneling** — Automatic multi-hop ProxyJump chain resolution
- ⚡ **Parallel Execution** — Run commands across multiple servers simultaneously with `@tag` syntax
- 🔍 **Diagnostics** — One-command health checks: tmux, nohup, docker, sudo detection
- 🔧 **Shell Completions** — Bash, Zsh, and Fish with dynamic host/tag awareness

---

## 🚀 Quick Start

### Install

**Homebrew** (macOS / Linux):

```bash
brew install xlyoung/tap/codex-ssh
```

**Pre-built binary** (Linux / macOS):

```bash
curl -fsSL https://raw.githubusercontent.com/xlyoung/codex-ssh/main/install.sh | bash
```

**go install** (requires Go 1.22+):

```bash
go install github.com/xlyoung/codex-ssh/cmd/codex-ssh@latest
```

### Configure

```bash
# Import your existing SSH config
codex-ssh hosts import-ssh-config

# Or add a server manually
codex-ssh hosts set myserver --host 192.168.1.100 --user deploy
```

### Verify

```bash
codex-ssh hosts list        # List all managed servers
codex-ssh hosts test myserver  # Test connectivity
codex-ssh doctor            # Run local health checks
```

---

## 💻 Usage

### Execute commands on remote servers

```bash
# Single server
codex-ssh exec myserver -- "uname -a"

# All servers tagged 'web'
codex-ssh exec @web -- "systemctl status nginx"

# All servers in inventory
codex-ssh exec @all -- "df -h"
```

### Interactive shell

```bash
codex-ssh shell myserver --cwd /srv/app
```

### Port forwarding & SOCKS5 proxy

```bash
# Forward local:8080 to remote 127.0.0.1:80
codex-ssh tunnel myserver --local 8080 --target 127.0.0.1:80

# Start a SOCKS5 proxy
codex-ssh proxy myserver --local 1080 --background
```

### Diagnostics

```bash
codex-ssh diagnose myserver
```

### Audit logs

```bash
codex-ssh audit query --host myserver --format text
```

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────┐
│                  AI Agent Layer                      │
│         Codex · Claude · Hermes · Cursor             │
└──────────────────────────┬──────────────────────────┘
                           │ MCP (stdio)
┌──────────────────────────▼──────────────────────────┐
│                 MCP Server Layer                     │
│    ssh_exec · ssh_hosts_list · ssh_diagnose ·        │
│    ssh_audit                                         │
└──────────────────────────┬──────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────┐
│                  CLI / Command Layer                  │
│   exec · shell · tunnel · proxy · job · audit ·      │
│   diagnose · hosts · secret · completion              │
└──────────────────────────┬──────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────┐
│                  Core Engine Layer                    │
│         executor · hosts · secrets · config          │
│         tunnel · proxy · jobs · audit                │
└──────────────────────────┬──────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────┐
│                  Transport Layer                     │
│       OpenSSH · SSH Agent · Keychain · Askpass       │
└──────────────────────────┬──────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────┐
│                  Target Layer                        │
│      Direct · Jump Hosts · ProxyJump Chains          │
└─────────────────────────────────────────────────────┘
```

---

## 🔌 MCP Integration

Codex SSH includes a built-in **MCP (Model Context Protocol) server** that exposes SSH operations as standard AI tools.

### Start the MCP server

```bash
codex-ssh mcp serve
```

### Available MCP Tools

| Tool | Description |
|------|-------------|
| `ssh_hosts_list` | List all hosts in the inventory |
| `ssh_exec` | Execute a command on a remote host (with timeout support) |
| `ssh_diagnose` | Diagnose connectivity and remote capabilities |
| `ssh_audit` | Query audit logs for SSH operations |

### Claude Desktop Configuration

Add to your Claude Desktop `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "codex-ssh": {
      "command": "codex-ssh",
      "args": ["mcp", "serve"]
    }
  }
}
```

### Cursor / Windsurf Configuration

Add to your MCP settings:

```json
{
  "mcpServers": {
    "codex-ssh": {
      "command": "codex-ssh",
      "args": ["mcp", "serve"]
    }
  }
}
```

Once connected, your AI assistant can list servers, execute commands, run diagnostics, and query audit logs — all through structured tool calls.

---

## 📖 Documentation

| Document | Description |
|----------|-------------|
| [Requirements](docs/REQUIREMENTS.md) | Feature specifications and design details |
| [Roadmap](docs/ROADMAP.md) | Full feature roadmap (P0 → P2) |
| [Contributing Guide](CONTRIBUTING.md) | How to contribute to the project |
| [Code of Conduct](CODE_OF_CONDUCT.md) | Community guidelines |

### Project Structure

```
codex-ssh/
├── cmd/codex-ssh/          # Main entry point
├── internal/               # Internal packages
│   ├── cli/               # CLI commands & shell completions
│   ├── config/            # Configuration management
│   ├── hosts/             # Host inventory
│   ├── secrets/           # Keychain password management
│   ├── sshargs/           # SSH argument builder
│   ├── sshconfig/         # ~/.ssh/config parser
│   ├── executor/          # Remote command execution
│   ├── tunnel/            # Port forwarding
│   ├── proxy/             # SOCKS5 proxy
│   ├── jobs/              # Background job management
│   ├── audit/             # Structured audit logging
│   ├── askpass/           # Password injection
│   ├── mcp/               # MCP server (JSON-RPC)
│   ├── runtime/           # Runtime state
│   └── validate/          # Input validation
├── pkg/model/             # Shared data models
├── scripts/               # Build & install scripts
├── defaults/              # Default config templates
└── docs/                  # Documentation
```

---

## 🤝 Contributing

We welcome contributions! Please read the [Contributing Guide](CONTRIBUTING.md) for details.

```bash
# Fork and clone
git clone https://github.com/<your-username>/codex-ssh.git
cd codex-ssh

# Build
go build ./cmd/codex-ssh

# Test
go test -race ./...

# Lint
golangci-lint run
```

We follow [Conventional Commits](https://www.conventionalcommits.org/):
`feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `perf:`, `ci:`, `chore:`

---

## 📄 License

[MIT License](LICENSE) — Copyright (c) zhuohua yang

---

<p align="center">
  <strong>Codex SSH</strong> · Built with ❤️ by <a href="https://github.com/xlyoung">zhuohua yang</a>
</p>
