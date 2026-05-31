# Codex SSH

> Vibe Coding + Vibe运维：让 AI 助手自动管理你的 SSH 连接

[![Go Version](https://img.shields.io/badge/go-1.22+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

## 🎯 这是什么？

Codex SSH 是一个为 AI 助手（如 Codex、Claude）设计的 SSH 管理工具。它让 AI 能够：

- 🔐 **自动管理 SSH 连接** - 通过 inventory 管理服务器清单
- 🚇 **智能跳板机穿透** - 自动处理 ProxyJump 链路
- 🔑 **安全的密码管理** - 使用系统 Keychain 存储凭据
- 📊 **结构化审计日志** - 记录所有 SSH 操作
- 🎯 **Vibe 运维** - AI 自动诊断、执行、维护远程服务

## 🚀 快速开始

### 1. 安装

```bash
# 克隆仓库
git clone https://github.com/yourusername/codex-ssh.git
cd codex-ssh

# 运行安装脚本
./scripts/install_skill.sh
```

### 2. 配置

```bash
# 复制示例配置
cp defaults/hosts.example.toml ~/.codex/ssh/hosts.toml

# 编辑配置文件，添加你的服务器
vim ~/.codex/ssh/hosts.toml
```

### 3. 使用

```bash
# 查看帮助
codex-ssh --help

# 添加服务器到 inventory
codex-ssh hosts set myserver --host 192.168.1.101 --user admin

# 测试连接
codex-ssh hosts test myserver

# 执行远程命令
codex-ssh exec myserver -- "uname -a"

# 启动交互 shell
codex-ssh shell myserver

# 诊断服务器
codex-ssh diagnose myserver
```

## 📖 详细使用指南

### 服务器配置

在 `~/.codex/ssh/hosts.toml` 中配置你的服务器：

```toml
version = 1

# 堡垒机/跳板机
[hosts."bastion"]
host = "bastion.example.com"
user = "admin"
port = 22
auth = "agent"
tags = ["bastion", "jump"]

# 应用服务器（通过跳板机）
[hosts."app-server"]
host = "app.internal.example.com"
user = "appuser"
port = 22
via = ["bastion"]
auth = "password"
secret_ref = "ssh://appuser@app.internal.example.com:22"
tags = ["app", "password-auth"]

# 数据库服务器（通过跳板机）
[hosts."db-server"]
host = "db.internal.example.com"
user = "dbadmin"
port = 22
via = ["bastion"]
auth = "agent"
tags = ["database"]
```

### 认证方式

#### 1. SSH Agent（推荐）
```toml
[hosts."myserver"]
host = "192.168.1.101"
user = "admin"
auth = "agent"
```

#### 2. 密码认证
```bash
# 先存储密码到 Keychain
codex-ssh secret set --host 192.168.1.101 --user admin

# 然后配置服务器
[hosts."myserver"]
host = "192.168.1.101"
user = "admin"
auth = "password"
secret_ref = "ssh://admin@192.168.1.101:22"
```

#### 3. 密钥文件
```toml
[hosts."myserver"]
host = "192.168.1.101"
user = "admin"
auth = "identity_file"
identity_file = "~/.ssh/id_rsa"
```

### 跳板机配置

```toml
# 跳板机
[hosts."bastion"]
host = "bastion.example.com"
user = "admin"
auth = "agent"

# 通过跳板机访问的服务器
[hosts."internal-server"]
host = "internal.example.com"
user = "appuser"
via = ["bastion"]
auth = "agent"
```

### 常用命令

```bash
# 服务器管理
codex-ssh hosts list                    # 列出所有服务器
codex-ssh hosts show myserver           # 查看服务器详情
codex-ssh hosts set myserver --host 192.168.1.101 --user admin  # 添加服务器
codex-ssh hosts remove myserver         # 删除服务器
codex-ssh hosts test myserver           # 测试连接
codex-ssh hosts import-ssh-config       # 导入 ~/.ssh/config

# 密码管理
codex-ssh secret set --host 192.168.1.101 --user admin  # 存储密码
codex-ssh secret get --host 192.168.1.101 --user admin  # 查看密码（隐藏）
codex-ssh secret get --host 192.168.1.101 --user admin --show  # 显示密码
codex-ssh secret delete --host 192.168.1.101 --user admin  # 删除密码

# 执行命令
codex-ssh exec myserver -- "uname -a"   # 执行单次命令
codex-ssh shell myserver                # 启动交互 shell
codex-ssh shell myserver --cwd /srv/app # 指定工作目录

# 端口转发
codex-ssh tunnel myserver --local 8080 --target 127.0.0.1:80  # 本地端口转发
codex-ssh tunnel myserver --local 8080 --target 127.0.0.1:80 --background  # 后台运行
codex-ssh tunnel list                   # 列出所有隧道
codex-ssh tunnel stop <id>              # 停止隧道

# SOCKS5 代理
codex-ssh proxy myserver --local 1080   # 启动 SOCKS5 代理
codex-ssh proxy myserver --local 1080 --background  # 后台运行
codex-ssh proxy list                    # 列出所有代理
codex-ssh proxy stop <id>               # 停止代理

# 长任务管理
codex-ssh job run myserver -- "bash deploy.sh"  # 运行长任务
codex-ssh job status <job-id>           # 查看任务状态
codex-ssh job attach <job-id>           # 附加到任务
codex-ssh job stop <job-id>             # 停止任务
codex-ssh job logs <job-id>             # 查看任务日志

# 审计日志
codex-ssh audit query                   # 查询审计日志
codex-ssh audit query --format text     # 文本格式输出
codex-ssh audit query --host myserver   # 按服务器过滤

# 诊断
codex-ssh diagnose myserver             # 诊断服务器
codex-ssh doctor                        # 本地自检
codex-ssh doctor myserver               # 本地自检 + 远程诊断
```

## 🤖 与 AI 助手集成

### Codex 集成

安装后，Codex 会自动识别 codex-ssh skill。你可以直接说：

- "用 codex-ssh 连服务器"
- "用 codex-ssh 执行命令"
- "用 codex-ssh 导入 ~/.ssh/config"

### Claude 集成

在 Claude 中使用：

1. 安装 skill：`./scripts/install_skill.sh`
2. 重启 Claude
3. 直接说："用 codex-ssh 连服务器"

### Vibe 运维示例

```bash
# AI 自动诊断服务器问题
"诊断 app-server 服务器"

# AI 自动部署应用
"在 app-server 上部署最新版本"

# AI 自动查看日志
"查看 app-server 的应用日志"

# AI 自动重启服务
"重启 app-server 上的 nginx 服务"
```

## 🔒 安全特性

- ✅ **密码不存储在配置文件中** - 使用 macOS Keychain
- ✅ **审计日志脱敏** - 不记录密码和敏感信息
- ✅ **Askpass 机制** - 避免密码泄露到命令行
- ✅ **复用系统 known_hosts** - 不重复存储主机密钥
- ✅ **不保存私钥正文** - 只引用已有私钥文件

## 📁 目录结构

```
~/.codex/ssh/
├── config.toml          # 全局配置
├── hosts.toml           # 服务器清单
├── logs/                # 审计日志
│   └── YYYY-MM-DD.jsonl
└── run/                 # 运行时数据
    ├── control/         # SSH 控制 socket
    ├── tunnels/         # 隧道状态
    ├── proxies/         # 代理状态
    ├── jobs/            # 任务状态
    └── askpass/         # 临时 askpass 脚本
```

## 🛠️ 开发

### 本地开发

```bash
# 克隆仓库
git clone https://github.com/yourusername/codex-ssh.git
cd codex-ssh

# 运行测试
CGO_ENABLED=0 go test ./...

# 构建
CGO_ENABLED=0 go build -o codex-ssh ./cmd/codex-ssh

# 使用本地构建
./codex-ssh --help
```

### 项目结构

```
codex-ssh/
├── cmd/codex-ssh/       # 主程序入口
├── internal/            # 内部包
│   ├── cli/            # 命令行界面
│   ├── config/         # 配置管理
│   ├── hosts/          # 主机清单
│   ├── secrets/        # 密码管理
│   ├── sshargs/        # SSH 参数生成
│   ├── executor/       # 命令执行
│   ├── tunnel/         # 隧道管理
│   ├── proxy/          # 代理管理
│   ├── jobs/           # 任务管理
│   ├── audit/          # 审计日志
│   └── runtime/        # 运行时状态
├── pkg/model/          # 数据模型
├── scripts/            # 脚本
├── defaults/           # 默认配置
└── docs/               # 文档
```

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

## 🤝 贡献

欢迎贡献！请阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 了解详情。

## 🙏 致谢

- [OpenSSH](https://www.openssh.com/) - SSH 协议实现
- [BurntSushi/toml](https://github.com/BurntSushi/toml) - TOML 解析器
- [Codex](https://openai.com/codex/) - AI 助手平台

---

**Vibe Coding + Vibe 运维 = 🚀**
