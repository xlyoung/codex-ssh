# Codex SSH — 需求改造文档

> 对标 Claude Squad / browser-use 级别，打造 AI 运维工具标杆

---

## 改造总览

| # | 模块 | 优先级 | 工作量 | 说明 |
|---|------|--------|--------|------|
| 1 | README 重写 | P0 | 2h | 对标顶级开源项目 |
| 2 | MCP 协议支持 | P0 | 4h | AI 工具的标准接口 |
| 3 | Homebrew 分发 | P0 | 1h | `brew install codex-ssh` |
| 4 | Shell 补全 | P0 | 1h | bash/zsh/fish |
| 5 | 多机并行执行 | P0 | 3h | `exec @all` 核心能力 |
| 6 | GitHub CI/CD | P0 | 2h | test + lint + release |
| 7 | Issue 模板 + CONTRIBUTING | P1 | 0.5h | 社区基础 |
| 8 | Logo (SVG) | P1 | 0.5h | 暗色/亮色双模式 |
| 9 | install.sh | P1 | 0.5h | 一键安装脚本 |
| 10 | 构建验证 + 推送 | P0 | 1h | 确保一切正常 |

---

## 1. README 重写（对标 Claude Squad）

### 结构模板

```markdown
# Codex SSH

> 🤖 让 AI 助手自动管理你的 SSH 服务器 — Go 单二进制，MCP 原生

[![CI](https://github.com/xlyoung/codex-ssh/actions/workflows/ci.yml/badge.svg)](...)
[![Go Report Card](https://goreportcard.com/badge/github.com/xlyoung/codex-ssh)](...)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](...)
[![Release](https://img.shields.io/github/v/release/xlyoung/codex-ssh)](...)

[English](README.md) | [中文](README_CN.md) | [日本語](README_JA.md)

---

## What is Codex SSH?

一句话描述...

## ✨ Features

- 🤖 **AI-Native** — 为 Codex/Claude/Hermes 设计的 SSH 管理
- 🔐 **安全优先** — Keychain + Askpass + 审计日志
- 🚀 **Go 单二进制** — 无依赖，跨平台
- 🔌 **MCP 协议** — 标准 AI 工具接口
- 🌊 **跳板机穿透** — 自动处理多级 ProxyJump
- 📊 **可观测性** — 结构化审计日志

## 🚀 Quick Start

### 安装
[Homebrew / 二进制 / go install]

### 使用
[3-5 个核心命令示例]

## 🏗️ Architecture

[架构图]

## 📖 Documentation

[文档链接]

## 🤝 Contributing

[贡献指南]

## 📄 License

MIT
```

### 要求
- 暗色 Logo（SVG）
- 徽章行（CI / Go Report / License / Release）
- 一行价值主张
- GIF 演示（或代码截图）
- 3 种安装方式
- 5 个核心命令示例
- 架构图（ASCII 或 Mermaid）
- 中英文双语

---

## 2. MCP 协议支持

### 目标
```bash
codex-ssh mcp serve    # 启动 MCP Server
```

### MCP Tools 定义

| Tool | 描述 | 参数 |
|------|------|------|
| `ssh_hosts_list` | 列出所有主机 | 无 |
| `ssh_hosts_add` | 添加主机 | name, host, user, port, via |
| `ssh_exec` | 执行远程命令 | host, command, timeout |
| `ssh_shell` | 获取交互式 shell | host, cwd |
| `ssh_tunnel` | 创建端口转发 | host, local_port, target |
| `ssh_diagnose` | 诊断服务器 | host |
| `ssh_audit` | 查询审计日志 | host, since, limit |

### 实现方案
- 使用 `github.com/mark3labs/mcp-go` 或类似 MCP Go SDK
- Stdio transport（标准 MCP 模式）
- 复用现有 internal/ 模块

---

## 3. Homebrew 分发

### 目标
```bash
brew install xlyoung/tap/codex-ssh
```

### 文件结构
```
codex-ssh-homebrew/
├── Formula/
│   └── codex-ssh.rb
└── README.md
```

### Formula 模板
```ruby
class CodexSsh < Formula
  desc "AI-native SSH management for AI assistants"
  homepage "https://github.com/xlyoung/codex-ssh"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/xlyoung/codex-ssh/releases/download/v0.1.0/codex-ssh_darwin_amd64.tar.gz"
      sha256 "..."
    end
    on_arm do
      url "https://github.com/xlyoung/codex-ssh/releases/download/v0.1.0/codex-ssh_darwin_arm64.tar.gz"
      sha256 "..."
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/xlyoung/codex-ssh/releases/download/v0.1.0/codex-ssh_linux_amd64.tar.gz"
      sha256 "..."
    end
    on_arm do
      url "https://github.com/xlyoung/codex-ssh/releases/download/v0.1.0/codex-ssh_linux_arm64.tar.gz"
      sha256 "..."
    end
  end

  def install
    bin.install "codex-ssh"
  end

  test do
    system "#{bin}/codex-ssh", "--version"
  end
end
```

---

## 4. Shell 补全

### 目标
```bash
codex-ssh completion bash    # Bash 补全
codex-ssh completion zsh     # Zsh 补全
codex-ssh completion fish    # Fish 补全
```

### 实现
- 使用 cobra 的内置补全功能
- 添加 `completion` 子命令

---

## 5. 多机并行执行

### 目标
```bash
codex-ssh exec @all "uname -a"           # 所有主机
codex-ssh exec @web "systemctl status nginx"  # 按 tag 过滤
codex-ssh exec @web,@db "df -h"          # 多 tag
```

### 实现
- `exec` 命令支持 `@tag` 语法
- 并行执行（goroutine + WaitGroup）
- 结果带主机前缀输出
- 失败主机单独报告

---

## 6. GitHub CI/CD

### workflows/ci.yml
```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test -v ./...
      - run: go vet ./...
      - uses: golangci/golangci-lint-action@v4
        with:
          version: latest
```

### workflows/release.yml
```yaml
name: Release
on:
  push:
    tags: ['v*']
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: goreleaser/goreleaser-action@v5
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### .goreleaser.yml
```yaml
builds:
  - binary: codex-ssh
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"
checksum:
  name_template: 'checksums.txt'
changelog:
  sort: asc
```

---

## 7. Issue 模板

### .github/ISSUE_TEMPLATE/bug_report.md
```markdown
---
name: Bug Report
about: Report a bug
labels: bug
---

## Describe the bug

## Steps to reproduce

## Expected behavior

## Environment
- OS:
- Go version:
- codex-ssh version:
```

### .github/ISSUE_TEMPLATE/feature_request.md
```markdown
---
name: Feature Request
about: Suggest a feature
labels: enhancement
---

## Problem

## Solution

## Alternatives
```

---

## 8. Logo

- SVG 格式
- 暗色/亮色双模式
- 元素：Terminal 图标 + AI/机器人元素 + SSH 锁
- 128x128 或更大
- 放在 `assets/logo.svg`

---

## 9. install.sh

```bash
#!/bin/bash
# 一键安装 codex-ssh
set -e

VERSION="${1:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# 检测系统和架构
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# 下载并安装
# ...

echo "✅ codex-ssh 已安装到 $INSTALL_DIR/codex-ssh"
```

---

## 10. 构建验证

### 验证清单
- [ ] `go build ./...` 编译通过
- [ ] `go test ./...` 测试通过
- [ ] `go vet ./...` 无警告
- [ ] `codex-ssh --version` 正常输出
- [ ] `codex-ssh --help` 显示完整帮助
- [ ] `codex-ssh completion bash` 输出补全脚本
- [ ] `codex-ssh mcp serve` 可启动（测试 MCP）
- [ ] README 渲染正确
- [ ] CI workflow 语法正确
- [ ] Release workflow 语法正确

---

## 执行顺序

```
Step 1: 创建需求文档 ✅（当前）
Step 2: 添加 Shell 补全 + 多机并行（功能补全）
Step 3: 添加 MCP 支持（核心差异化）
Step 4: 重写 README + 添加 Logo
Step 5: 配置 CI/CD + Release
Step 6: 添加 Issue 模板 + CONTRIBUTING
Step 7: 添加 Homebrew + install.sh
Step 8: 构建验证 + 推送
```
