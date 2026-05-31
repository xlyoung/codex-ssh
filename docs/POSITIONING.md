# Codex SSH — Project Positioning

> 从 SSH 管理工具到无人值守运维平台的演进路线

---

## 一、当前功能清单（已实现）

### 1.1 基础设施层（Infrastructure）

| 功能 | 状态 | 说明 |
|------|------|------|
| TOML 主机清单 | ✅ | `~/.codex/ssh/hosts.toml`，支持 alias、tags、via 跳板链 |
| SSH Config 导入 | ✅ | `hosts import-ssh-config` 从 `~/.ssh/config` 自动导入 |
| 全局配置 | ✅ | `~/.codex/ssh/config.toml`，含默认用户/端口/认证/超时/keepalive |
| 路径解析 | ✅ | 支持 `CODEX_SSH_HOME` 环境变量自定义数据目录 |
| 目录结构 | ✅ | 标准化 `run/`（control/tunnels/proxies/jobs/askpass）、`logs/` |

### 1.2 管理层（Management）

| 功能 | 状态 | 说明 |
|------|------|------|
| 主机 CRUD | ✅ | `hosts list/show/set/remove` — 增删改查清单 |
| 连接测试 | ✅ | `hosts test` 验证连通性 |
| 跳板机穿透 | ✅ | `via` 字段驱动多级 ProxyJump，自动检测循环引用 |
| 目标解析 | ✅ | 先查 inventory alias，再回退裸 IP/host:port |
| 绑定控制复用 | ✅ | `ControlMaster=auto` + `ControlPersist=10m`，减少重复握手 |

### 1.3 执行层（Execution）

| 功能 | 状态 | 说明 |
|------|------|------|
| 单次命令执行 | ✅ | `exec` — 支持超时、stdout/stderr 捕获、非交互模式 |
| 交互 Shell | ✅ | `shell` — PTY 模式登录，支持 `--cwd` 指定工作目录 |
| 本地端口转发 | ✅ | `tunnel` — LocalForward，支持前台/后台，端口可用性校验 |
| SOCKS5 代理 | ✅ | `proxy` — DynamicForward，后台运行 + 状态持久化 |
| 长任务管理 | ✅ | `job run/status/attach/stop/logs` — tmux/nohup 自动选择，状态 JSON 持久化 |

### 1.4 智能层（Intelligence）

| 功能 | 状态 | 说明 |
|------|------|------|
| 远程诊断 | ✅ | `diagnose` — 检测目标地址/端口/跳板链/认证模式/known_hosts/tmux/nohup/docker/sudo |
| 本地自检 | ✅ | `doctor` — 检查 repo/bin/config/stamp/Keychain 可用性 |
| 自动模式选择 | ✅ | `job run` 自动检测远端 tmux → nohup → raw fallback |
| 诊断输出结构化 | ✅ | 输出地址/端口/跳板链/认证/SSH 路径等诊断信息 |

### 1.5 安全层（Security）

| 功能 | 状态 | 说明 |
|------|------|------|
| macOS Keychain 密码管理 | ✅ | `secret set/get/delete`，通过 `security` CLI 存储 |
| Askpass 机制 | ✅ | 密码通过临时 askpass 脚本注入，避免 CLI 参数泄露 |
| 审计日志 | ✅ | JSONL 格式，按日期分文件，记录所有操作（exec/shell/tunnel/proxy/job） |
| 敏感信息脱敏 | ✅ | 不记录密码到日志，`redact_env=true` |
| known_hosts 复用 | ✅ | 复用系统 `~/.ssh/known_hosts`，不重复存储主机密钥 |
| SSH Agent 优先 | ✅ | 默认推荐 agent 认证 |
| 安全策略配置 | ✅ | `StrictHostKeyChecking`、`AllowPasswordAuth`、`AllowRoot` 开关 |

### 1.6 AI 集成层（AI Integration）

| 功能 | 状态 | 说明 |
|------|------|------|
| SKILL.md 声明 | ✅ | 标准化 skill 格式，含触发词、workflow、safety rules |
| 自动构建包装器 | ✅ | `scripts/codex-ssh.sh` 自动检测源码 → build cache → 二进制 |
| wrapper 级 doctor | ✅ | 检查 repo 路径、iCloud、stamp、Keychain |
| Codex/Claude 集成 | ✅ | 通过 skill 安装到 `~/.codex/skills/codex-ssh` |

---

## 二、架构分层

```
┌─────────────────────────────────────────────────────────┐
│                    AI Integration Layer                   │
│   SKILL.md · 触发词解析 · 自动构建 · Codex/Claude 集成    │
├─────────────────────────────────────────────────────────┤
│                    Intelligence Layer                     │
│   diagnose · doctor · 模式自动选择 · 目标解析 · 诊断输出   │
├─────────────────────────────────────────────────────────┤
│                    Execution Layer                        │
│   exec · shell · tunnel · proxy · job · sshargs 构建     │
├─────────────────────────────────────────────────────────┤
│                    Management Layer                       │
│   hosts CRUD · import-ssh-config · jump host 链路解析     │
├─────────────────────────────────────────────────────────┤
│                    Security Layer                         │
│   Keychain · askpass · audit log · 脱敏 · known_hosts     │
├─────────────────────────────────────────────────────────┤
│                    Infrastructure Layer                   │
│   路径管理 · TOML 解析 · runtime state · 目录结构 · config │
└─────────────────────────────────────────────────────────┘
```

---

## 三、无人值守运维（Unmanned Operations）愿景

### 3.1 什么是无人值守运维？

**无人值守运维 = AI 代替人类完成从"发现问题 → 诊断 → 决策 → 执行 → 验证"的完整闭环，无需人工介入。**

当前 codex-ssh 的定位是 **"AI 的 SSH 手"** —— AI 通过它连接服务器、执行命令。
要成为真正的无人值守运维平台，需要升级为 **"AI 的运维大脑 + 手"** —— AI 自己发现异常、制定方案、执行修复、验证结果。

### 3.2 目标场景

| 场景 | 当前能力 | 无人值守目标 |
|------|----------|-------------|
| 服务器磁盘满了 | AI 手动 `exec` 检查 | 自动检测 → 分析大文件 → 清理 → 验证 |
| 服务宕机 | AI 手动诊断 → 重启 | 监控告警触发 → 自动诊断根因 → 自动重启 → 验证恢复 |
| SSL 证书即将过期 | 无 | 自动扫描 → 申请续期 → 部署 → 验证 |
| 安全补丁 | AI 手动 `exec` 升级 | 扫描 CVE → 评估影响 → 在维护窗口自动打补丁 → 回归测试 |
| 性能退化 | AI 手动查看日志 | 指标异常 → 分析瓶颈 → 自动调整配置 → 对比验证 |
| 多服务器批量运维 | 单机串行 | 并行编排 + 失败回滚 + 进度追踪 |

### 3.3 架构演进路线

#### Phase 1: 观察能力（Observability）
- **健康检查系统**: 定期扫描 inventory 中所有主机
- **指标采集**: CPU/内存/磁盘/网络/进程 基础指标
- **日志聚合**: 远程日志自动采集 + 关键词过滤
- **告警引擎**: 阈值/模式匹配告警
- **状态仪表盘**: 全局主机状态视图

#### Phase 2: 自动化能力（Automation）
- **Playbook 引擎**: YAML/JSON 定义运维流程，支持条件分支和错误处理
- **批量编排**: 多主机并行执行 + 依赖关系 + 失败策略（skip/rollback/abort）
- **模板系统**: 常用运维操作模板化（重启服务、清理日志、备份等）
- **定时任务**: Cron-like 定时执行 playbooks
- **回滚机制**: 执行前快照 + 失败自动回滚

#### Phase 3: 智能决策（Intelligence）
- **异常检测**: 基于历史数据的异常识别
- **根因分析**: 日志 + 指标联合分析，定位问题根因
- **方案推荐**: 根据异常类型推荐修复方案
- **执行决策**: AI 自主决定执行/人工确认/跳过
- **学习机制**: 记录决策结果，持续优化

#### Phase 4: 自主闭环（Autonomous Loop）
```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│  Observe  │───→│  Analyze  │───→│  Decide   │───→│  Execute  │
│  (观察)   │    │  (分析)   │    │  (决策)   │    │  (执行)   │
└──────────┘    └──────────┘    └──────────┘    └────┬─────┘
     ↑                                                │
     └────────────────────────────────────────────────┘
                     Verify (验证)
```

---

## 四、差距分析（Gap Analysis）

### 4.1 已有 vs 缺失对照

| 维度 | 已有 | 缺失（需要建设） |
|------|------|-------------------|
| **连接管理** | 单机 SSH 连接、跳板机链路 | 多集群管理、连接池、并发连接限制 |
| **主机发现** | 手动配置 inventory | 自动发现（扫描网段）、CMDB 集成、云 API 拉取 |
| **命令执行** | 单机串行执行 | 多机并行执行、编排引擎、依赖关系 |
| **任务管理** | tmux/nohup 后台任务 | 工作流引擎、DAG 编排、重试/回滚 |
| **监控观测** | 无 | 指标采集、日志聚合、健康检查、告警 |
| **配置管理** | TOML 静态配置 | 配置模板、变量替换、环境分组（dev/staging/prod） |
| **变更管理** | 无 | 变更审批流程、变更记录、回滚计划 |
| **密钥管理** | macOS Keychain 单机 | 跨平台密钥后端、Vault 集成、密钥轮转 |
| **权限控制** | `AllowRoot` 开关 | RBAC（角色/权限/主机组）、操作审批 |
| **审计** | JSONL 本地日志 | 集中审计、合规报告、操作录像（完整命令+输出） |
| **诊断** | 单机 diagnose | 全局拓扑诊断、跨主机关联分析 |
| **AI 决策** | 无（AI 通过 skill 手动触发） | 异常检测、根因分析、自动修复决策 |
| **自愈能力** | 无 | 服务自动重启、故障自动切换、自愈策略 |
| **多环境** | 单环境 | dev/staging/prod 环境隔离、环境变量管理 |
| **协作** | 无 | 多人共享清单、操作通知、会话共享 |
| **API/插件** | 无 | HTTP API、Webhook、插件系统、第三方集成 |

### 4.2 优先级建议

#### P0 — 立即可做（增强当前架构）

1. **多机并行执行** — 扩展 `exec` 支持多主机，`exec @all "uname -a"`
2. **Playbook 引擎** — YAML 定义运维流程，支持步骤/条件/错误处理
3. **主机自动发现** — 扫描网段、SSH 密钥探测
4. **跨平台密钥管理** — Linux 支持 pass/gnome-keyring/kwallet
5. **操作通知** — 执行结果推送（webhook/邮件/Slack）

#### P1 — 中期目标（智能运维基础）

1. **健康检查系统** — 定期扫描 + 状态聚合
2. **指标采集** — 基础系统指标（disk/cpu/mem/net）通过 SSH 采集
3. **批量编排** — 多主机并行 + 依赖 + 失败策略
4. **配置模板** — 变量替换 + 环境分组
5. **变更管理** — 变更记录 + 回滚计划

#### P2 — 远期愿景（自主运维）

1. **异常检测引擎** — 基于阈值/统计的异常识别
2. **根因分析** — 日志+指标联合分析
3. **自愈策略** — 故障自动修复 + 验证
4. **AI 决策引擎** — 自主决策 + 人工审批双模式
5. **合规审计** — 操作录像 + 合规报告

---

## 五、差异化定位

### 5.1 竞品对比

| 产品 | 定位 | codex-ssh 的差异点 |
|------|------|---------------------|
| **Ansible** | 配置管理+应用部署 | codex-ssh 是 AI-native，无需 inventory 编写 YAML；通过对话驱动 |
| **Fabric** | Python SSH 库 | codex-ssh 是 Go 单二进制，AI skill 原生集成 |
| **Teleport** | 零信任访问平台 | codex-ssh 轻量级，无服务端，本地运行 |
| **MobaXterm/PuTTY** | GUI SSH 客户端 | codex-ssh 是 CLI/AI 驱动，可编程 |

### 5.2 核心差异化

1. **AI-Native**: 不是"给 AI 用的工具"，而是"为 AI 设计的运维平台"
2. **零配置上手**: 从 `~/.ssh/config` 一键导入，AI 自动理解
3. **单二进制部署**: Go 编译，无依赖，跨平台
4. **安全优先**: Keychain + askpass + 脱敏审计，安全基线内置
5. **渐进式自动化**: 从手动命令 → 半自动 → 全自动的演进路径

---

## 六、总结

codex-ssh 当前是一个 **成熟的 SSH 管理工具**，具备了：
- ✅ 完善的基础设施（清单/配置/路径/运行时）
- ✅ 可靠的执行引擎（exec/shell/tunnel/proxy/job）
- ✅ 安全的安全层（Keychain/askpass/audit）
- ✅ AI 集成能力（SKILL.md/触发词/自动构建）

要成为 **无人值守运维平台**，核心需要补齐：
- 🔄 **观测能力** — 看得见（监控/告警/指标）
- 🔄 **编排能力** — 管得住（批量/并行/回滚）
- 🔄 **智能能力** — 想得明（检测/分析/决策）
- 🔄 **自愈能力** — 自动修（发现→诊断→修复→验证）

**建议路径**: 在保持当前 SSH 管理核心稳定的基础上，按 Phase 1→4 逐步演进，每个 Phase 都有独立可用的价值。
