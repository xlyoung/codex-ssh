# Codex SSH Skill Design

**Date:** 2026-04-03

## Goal

构建一个面向 Codex 的本地 SSH skill。它采用 `Ansible` 风格的控制器设计，复用系统 `OpenSSH`，并覆盖以下场景：

- 服务器清单管理
- 直连与跳板机链路执行
- 结构化审计日志
- 临时应用访问能力：`LocalForward` 与 `DynamicForward`
- 长任务后台托管，避免本地终端断开后任务中止
- 密码认证主机的安全存储与自动取密
- 直接使用裸 IP 或 `user@host:port` 的 ad-hoc 执行

## Non-Goals

以下内容不进入当前版本：

- 自己实现 SSH 协议栈
- 中央化堡垒机平台与多用户 RBAC
- 默认完整会话录像
- 整网段透明代理
- 私钥正文托管或自建密钥仓库
- Windows 兼容
- Linux Secret Service、`pass`、1Password CLI 等多后端并发支持
- 在 `hosts.toml`、`config.toml` 或命令行参数里保存明文密码

## Product Direction

产品主路径不是“模型记得先去读密码”，而是“`codex-ssh` 自己解析目标、自动取密、自动注入给 `ssh`”。这样能力边界固定在 wrapper 内，避免把安全路径依赖到提示词命中或模型记忆上。

## Architecture

整体由 8 个模块组成：

1. `config`
加载全局配置与运行目录默认值。

2. `hosts`
加载主机 inventory，解析目标主机、默认用户、端口、跳板链、认证方式。

3. `sshconfig`
导入本机 `~/.ssh/config`，抽取 `Host`、`HostName`、`User`、`Port`、`ProxyJump`、`IdentityFile` 等字段。

4. `sshargs`
将配置转为 `ssh` 命令参数，统一注入：

- `ProxyJump`
- `ControlMaster`
- `ControlPersist`
- `ServerAliveInterval`
- `ServerAliveCountMax`
- `BatchMode`
- `PasswordAuthentication`

5. `executor`
调用系统 `ssh` 执行命令、交互 shell、建立 tunnel/proxy、运行远端后台任务，并在需要时注入 askpass 环境变量。

6. `secrets`
负责密码凭据的 `set/get/delete`，当前版本默认使用 macOS Keychain。

7. `audit`
写入结构化 `JSONL` 审计日志，记录开始/结束、目标主机、命令、退出码、耗时、隧道信息。

8. `runtime`
维护本地活动状态，包括：

- control socket 目录
- tunnel/proxy PID 与状态文件
- job 状态文件
- askpass 临时脚本目录

## Storage Layout

Skill 代码与运行数据分离。

默认运行目录：

- `~/.codex/ssh/config.toml`
- `~/.codex/ssh/hosts.toml`
- `~/.codex/ssh/logs/YYYY-MM-DD.jsonl`
- `~/.codex/ssh/run/control/`
- `~/.codex/ssh/run/tunnels/`
- `~/.codex/ssh/run/proxies/`
- `~/.codex/ssh/run/jobs/`
- `~/.codex/ssh/run/askpass/`

macOS Keychain 保存密码本体，不在工作目录落地。

## Configuration Model

`config.toml` 保存全局默认值：

- 默认用户、端口、认证方式
- keepalive 与 connect timeout
- control master/persist
- 默认长任务模式
- 审计采集策略
- `security.allow_password_auth`

`hosts.toml` 保存主机清单：

- `host`
- `user`
- `port`
- `via`
- `tags`
- `workdir`
- `auth`
- `identity_file`
- `secret_ref`

其中：

- `via` 使用 alias 数组表达跳板链，执行阶段转换为 `-J jump1,jump2`
- `secret_ref` 是可选字段，用于显式绑定密码凭据引用
- 如果未配置 `secret_ref`，则默认按 `ssh://user@host:port` 推导

## Target Resolution

位置参数目标解析顺序固定如下：

1. 先查 inventory 精确 key
2. 未命中时，再按裸目标解析

这意味着：

- `shell 192.168.1.101` 会先匹配 `[hosts."192.168.1.101"]`
- 若存在该 key，则自动复用其中的 `via`、`auth`、`secret_ref`、`workdir`
- 若不存在，则把 `192.168.1.101` 当作 ad-hoc 目标处理
- 裸目标不会自动猜测跳板链；如果 inventory 没有对应 key，就必须显式传 `--via`

支持的裸目标格式：

- `192.168.1.101`
- `appuser@192.168.1.101`
- `appuser@192.168.1.101:22`

## Command Surface

当前 CLI 命令面：

- `hosts list`
- `hosts show <alias>`
- `hosts set <alias> ...`
- `hosts remove <alias>`
- `hosts test <alias>`
- `hosts import-ssh-config`
- `exec [<alias> | --host <host>] -- <command>`
- `shell [<alias> | --host <host>]`
- `tunnel [<alias> | --host <host>] --local <port> --target <host:port>`
- `tunnel list`
- `tunnel stop <id>`
- `proxy [<alias> | --host <host>] --local <port>`
- `proxy list`
- `proxy stop <id>`
- `job run <alias> -- <command>`
- `job status <job-id>`
- `job attach <job-id>`
- `job stop <job-id>`
- `job logs <job-id>`
- `audit tail`
- `audit query`
- `diagnose [<alias> | --host <host>]`

新增密码管理命令组：

- `secret set --host <host> --user <user> [--port <port>]`
- `secret get --host <host> --user <user> [--port <port>] [--show]`
- `secret delete --host <host> --user <user> [--port <port>]`

第一版不提供 `secret status`，避免命令面继续扩散。

## Authentication Strategy

支持以下认证模式：

- `agent`
- `identity_file`
- `password`

边界约束：

- 不保存私钥正文
- 不保存 passphrase
- 不在 `hosts.toml`、`config.toml`、命令行参数、审计日志中保存明文密码
- 优先复用 `ssh-agent`
- 复用系统 `known_hosts`
- `password` 模式当前仅对 macOS Keychain 提供自动取密能力

## Secret Backend Design

密码后端主路径采用“独立 secret 管理 + Keychain”：

1. `secret set`
   - 交互录入密码
   - 使用 `security add-generic-password`
   - 默认不接受命令行明文密码参数

2. `secret get`
   - 默认只供内部调用
   - 人工调试时可通过 `--show` 显式打印

3. `secret delete`
   - 删除对应的 Keychain 条目

默认凭据引用规则：

- 若 `secret_ref` 已配置，则优先使用
- 否则按 `ssh://user@host:port` 推导

推荐用户体验：

- 对长期主机，直接配置 `[hosts."192.168.1.101"]`
- 使用 `secret set --host 192.168.1.101 --user appuser`
- 后续直接执行 `shell 192.168.1.101`

## Askpass Execution Strategy

密码不会通过标准输入喂给 `ssh`。正确路径是 askpass：

1. `codex-ssh` 解析出目标为 `auth=password`
2. 根据 `secret_ref` 或 `ssh://user@host:port` 取出密码
3. 生成一次性 askpass 脚本到 `run/askpass/`
4. 给 `ssh` 注入：
   - `SSH_ASKPASS=<script>`
   - `SSH_ASKPASS_REQUIRE=force`
   - `DISPLAY=dummy`
5. 运行完成后删除 askpass 脚本

这样能力边界稳定：

- Codex 不需要显式先执行一次“读密码”
- `codex-ssh` 自己决定何时取密
- 密码不出现在命令行参数和审计日志中

## Execution Strategy

### exec

默认走非交互执行路径，记录 stdout/stderr 摘要与长度。对于 `auth=password` 的目标，自动取密并走 askpass。

### shell

为人工排障提供交互 shell。对于 `auth=password` 的目标，自动取密并走 askpass。当前不默认录制全量会话。

### tunnel

通过 `ssh -L` 建立本地端口映射，用于临时访问内网应用。后台模式仍然不支持 `password` 认证。

### proxy

通过 `ssh -D` 建立本地 `SOCKS5` 动态代理。后台模式仍然不支持 `password` 认证。

### job

长任务默认优先 `tmux`，当远端不可用时降级 `nohup`。当前版本不扩展“后台 job + password secret”的额外守护机制，只沿用现有连接策略。

### diagnose

用于首次接入和排障，输出：

- 目标地址、用户、端口
- 跳板链
- 认证模式
- `known_hosts` 状态
- 远端 `tmux/nohup/docker/sudo` 可用性

## Audit Model

审计日志采用按天滚动的 `JSONL` 结构化事件。

每条事件至少包含：

- 时间戳
- 事件 ID
- 动作类型
- host alias
- 解析后的主机地址
- 用户、端口
- 跳板链
- 执行命令
- 开始/结束时间
- 耗时
- 退出码
- stdout/stderr 字节数
- 状态

针对 `tunnel/proxy/job` 额外记录：

- 本地监听地址与端口
- 远端目标地址与端口
- PID
- 后台状态
- job/session 名称

敏感信息约束：

- 不记录密码
- 不记录 askpass 脚本内容
- 不记录 Keychain 返回值
- 最多记录 `auth=password` 或 `secret_source=keychain` 级别的元数据

## Error Handling

关键错误路径要明确、可操作：

- inventory 未命中且未提供 `--via` 时，不猜测跳板机
- `auth=password` 但未找到凭据时，明确提示执行 `secret set`
- 非 macOS 平台使用密码 secret 时，明确提示当前平台不支持该后端
- 后台 `tunnel/proxy` 遇到 `password` 认证时，直接拒绝并提示改用前台或改用密钥认证

## Testing Strategy

测试分四层：

1. 单元测试
- config 默认值与路径展开
- hosts 解析与跳板链展开
- ssh 参数生成
- `sshconfig` 导入
- secret key 计算
- askpass 环境变量注入
- audit 写入与查询
- runtime 状态文件读写

2. 轻量集成测试
- inventory 优先、裸目标 fallback 的解析顺序
- `[hosts."192.168.1.101"]` 可通过 `shell 192.168.1.101` 命中
- `auth=password + via` 时正确查找 secret
- `secret_ref` 优先级正确
- 缺 secret 时错误消息正确

3. 安装态验证
- skill 安装后 wrapper 可见新命令
- `hosts list`、`diagnose`、`secret` 命令帮助输出正确

4. 手工验收
- 已批准用户主路径（inventory IP key + via + password + 自动取密）按以下命令顺序执行：
  - `codex-ssh secret set --host 192.168.1.101 --user appuser`
  - `codex-ssh diagnose 192.168.1.101`
  - `codex-ssh shell 192.168.1.101`
  - `codex-ssh exec 192.168.1.101 -- "docker pull ..."`
- 其余回归项：
  - 首次 host key 接受后重试
  - tunnel
  - proxy
  - 长任务 attach/status/logs

## Milestones

### M1

- inventory 优先、裸目标 fallback
- `secret_ref` 数据模型
- `secret` 命令组
- macOS Keychain backend

### M2

- askpass 自动取密执行链
- `exec/shell/hosts test/diagnose` 接入 secret
- 审计脱敏校验

### M3

- 文档、skill、安装脚本更新
- 安装态验证
- 真实远端联调与错误处理细化
