---
name: codex-ssh
description: Use when Codex needs SSH access to Linux servers, bastion or jump host traversal, ProxyJump, local port forwarding, SOCKS5 dynamic proxy, long-running remote jobs with tmux or nohup, or local SSH audit logs. 当任务涉及 SSH 连接服务器、堡垒机或跳板机、端口映射、LocalForward、DynamicForward、SOCKS5、tmux 长任务、nohup 后台任务、SSH 审计日志时使用。
---

# Codex SSH

## Overview

这个 skill 提供一个基于本地 `OpenSSH` 的受控封装，适合：

- 通过 SSH 执行远程命令
- 经跳板机访问内网主机
- 临时建立 `LocalForward`
- 临时建立 `SOCKS5` 动态代理
- 托管远端长任务
- 查询本地审计日志

命令名约定：
- 文档中写 `codex-ssh ...` 表示逻辑命令名
- 若本机 `PATH` 未包含 `codex-ssh`，请改用：
  - 仓库内 wrapper：`scripts/codex-ssh.sh ...`
  - 或安装后 wrapper：`~/.codex/skills/codex-ssh/scripts/codex-ssh.sh ...`

它的典型触发词包括：

- `ssh`
- `堡垒机`
- `跳板机`
- `ProxyJump`
- `LocalForward`
- `SOCKS5`
- `动态代理`
- `端口映射`
- `tmux`
- `nohup`
- `长任务`
- `审计日志`

## Workflow

1. 优先使用这个 skill 自带的 wrapper：
   - `scripts/codex-ssh.sh`
2. wrapper 会从 `CODEX_SSH_REPO` 读取源码仓库位置；若未设置，则回退到：
   - `/path/to/codex-ssh`
3. wrapper 会用 `CGO_ENABLED=0` 自动构建或复用 `~/.codex/ssh/build-cache/<repo-hash>/codex-ssh`
4. 读取 `~/.codex/ssh/config.toml` 与 `~/.codex/ssh/hosts.toml`
5. 首次使用先执行以下其中一种：
   - `hosts list`
   - `hosts import-ssh-config`
   - `hosts set <alias> --host <host> ...`
   - `exec --host <host> --user <user> -- "<command>"`
6. inventory 里已有主机时，优先执行：
   - `hosts test <alias>`
   - `diagnose <alias>`
   - 若怀疑 wrapper 本地状态异常，先执行 `doctor [<alias>]`
7. 目标解析遵循：
   - inventory key first：先按 `hosts.toml` key 查找（可直接用 IP 作为 key）
   - bare IP fallback：inventory 未命中时，再按裸 `user@host:port` 或 IP 解析
8. 再根据需求选择：
   - `exec`：一次性命令执行
   - `shell`：交互登录
   - `tunnel`：单端口映射
   - `proxy`：SOCKS5 代理
   - `job`：远端长任务托管
   - `audit`：查询本地审计日志

## First Use

- 如果 `hosts list` 提示 `inventory is empty`，不要回退成手写 `ssh -J`
- 优先顺序：
  - 已有 `~/.ssh/config`：执行 `hosts import-ssh-config`
  - 只有一次性目标：直接用 `exec --host ...` 或 `shell --host ...`
  - 想长期记住主机：执行 `hosts set`
- 任何跳板链或密码认证不确定时，先跑 `diagnose`

## Safety Rules

- 优先使用 `ssh-agent`
- 只引用已有私钥文件，不保存私钥正文
- 如需账号密码登录，使用 `auth = "password"` + `secret_ref`，通过 `secret set/get/delete` 管理密码
- `password` 模式主路径是 `codex-ssh` 自动取密 + askpass 注入，不依赖手工交互输密
- Keychain 密钥存储后端仅支持 macOS（依赖 `security` CLI）
- `password` 模式仅支持前台 `exec/shell/diagnose/job`；后台 `tunnel/proxy` 不支持密码认证
- 默认复用系统 `known_hosts`
- 审计日志保存在 `~/.codex/ssh/logs/`
- 后台 `tunnel/proxy` 与 `job` 都要有本地状态文件
- 涉及跳板链时，优先让 inventory 里的 `via` 驱动 `ProxyJump`
- 涉及临时内网应用访问时，优先使用 `tunnel` 或 `proxy`，不要直接手写 `ssh -L/-D`，除非用户明确要求

## Common Commands

以下示例默认使用逻辑命令名 `codex-ssh`（若不在 PATH，请按上文替换为 wrapper 路径）：

```bash
codex-ssh hosts list
codex-ssh doctor
codex-ssh doctor app-server
codex-ssh hosts import-ssh-config
codex-ssh hosts set bastion --host bastion.example.com --user admin
codex-ssh hosts set app-server --host app.internal.example.com --user appuser --via bastion
codex-ssh secret set --host app.internal.example.com --user appuser
codex-ssh secret get --host app.internal.example.com --user appuser
codex-ssh secret delete --host app.internal.example.com --user appuser
codex-ssh hosts test app-server
codex-ssh diagnose app-server
codex-ssh exec app-server -- "uname -a"
codex-ssh exec --host app.internal.example.com --user appuser --via bastion.example.com --auth password -- "uname -a"
codex-ssh shell app-server --cwd /srv/app
codex-ssh tunnel bastion --local 18080 --target db.internal.example.com:8080 --background
codex-ssh proxy bastion --local 1080 --background
codex-ssh job run app-server -- "bash deploy.sh"
codex-ssh audit query --format text --host app-server
```

## Password Auth

- inventory 写法：

```toml
[hosts."bastion"]
host = "bastion.example.com"
user = "admin"
auth = "agent"

[hosts."app-server"]
host = "app.internal.example.com"
user = "appuser"
via = ["bastion"]
auth = "password"
secret_ref = "ssh://appuser@app.internal.example.com:22"
```

- `allow_password_auth` 默认行为：
  - fresh install 或缺省 runtime config 时，默认允许（`allow_password_auth = true`）
  - 只有用户在 `~/.codex/ssh/config.toml` 显式设置为 `false` 时，password 主路径不可用

- 如需显式关闭，可配置：

```toml
[security]
allow_password_auth = false
```

- 先存密码到 Keychain（macOS）：

```bash
codex-ssh secret set --host app.internal.example.com --user appuser
```

- 连接时由 `codex-ssh` 自动读取 `secret_ref` 并通过 askpass 提供给 OpenSSH。

## Target Resolution

- positional 目标（如 `shell app-server`）会先按 inventory key 解析，再回退到裸 IP/endpoint。
- 所以推荐把常用机器直接写成：

```toml
[hosts."app-server"]
host = "app.internal.example.com"
user = "appuser"
auth = "password"
secret_ref = "ssh://appuser@app.internal.example.com:22"
```

- ad-hoc 场景仍可使用：
  - `codex-ssh exec --host 192.168.1.101 --user admin --auth password -- "uname -a"`

## Diagnose

- `diagnose` 会输出：
  - 目标地址、用户、端口、跳板链、认证模式
  - `ssh_path`
  - `known_hosts` 是否存在
  - 远端 `tmux/nohup/docker/sudo` 可用性
- 适合在以下场景优先执行：
  - 新主机首次接入
  - 堡垒机链路不确定
  - 密码认证是否允许不确定
  - 长任务依赖 `tmux` 或 `nohup`

## Doctor

- `doctor` 是 wrapper 级本地自检入口，先看本机这一层是否健康，再决定要不要继续怀疑远端
- 它会输出：
  - `repo_dir` / `repo_in_icloud`
  - `bin_dir` / `bin_path`
  - `stamp_status` 是否与源码 fingerprint 匹配
  - `config_path` / `hosts_path` / `logs_dir`
  - Keychain 后端是否可用
- `doctor <alias>` 会在本地自检后继续执行一次 `diagnose <alias>`
- 适合在以下场景优先执行：
  - 怀疑还在复用旧 binary
  - 不确定是不是 iCloud 路径导致
  - 前台看起来卡住，但想先分清是 wrapper 还是远端

## Notes

- 如果这个 skill 被安装到 `~/.codex/skills/codex-ssh`，重启 Codex 后才能被自动发现。
- 如果当前会话没有自动命中它，显式说：
  - `用 codex-ssh 连服务器`
  - `用 codex-ssh 临时执行 --host 命令`
  - `用 codex-ssh 导入 ~/.ssh/config`
  - `用 codex-ssh 开 SOCKS5 代理`
  - `用 codex-ssh 通过跳板机执行命令`
  这样可以强制提升命中率。
