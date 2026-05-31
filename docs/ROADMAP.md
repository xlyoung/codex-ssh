# Codex SSH — Feature Roadmap

**Vision:** 实现真正的无人运维 (Unmanned Operations) — 让 AI 助手能够自主、安全、可靠地管理远程服务器集群。

**更新时间:** 2026-05-31

---

## 设计理念

传统 SSH 工具（OpenSSH、mosh）服务于人类交互；Ansible/Fabric 服务于脚本化批量操作。Codex SSH 的定位是 **AI-native 运维控制面**——让 AI 助手能像经验丰富的 SRE 一样，自主完成诊断、执行、修复、验证的完整闭环，而不需要人类逐步指令。

核心原则：
1. **AI 自主闭环** — 工具提供结构化信息，AI 做决策，工具执行并验证
2. **安全边界固定** — 敏感操作（密码、密钥）封装在工具内部，不暴露给 AI prompt
3. **幂等可重试** — 每个操作设计为可安全重试，适合 AI 的试错-修正循环
4. **渐进式信任** — 从只读诊断到写操作，逐步放开权限边界

---

## P0 — MVP 核心 (Must Have)

> 这些功能构成最小可用产品，让 AI 能完成基本的远程运维任务。

### 1. 多主机并行执行

**命令:** `codex-ssh exec --targets "app-1,app-2,app-3" -- "systemctl status nginx"`

**无人运维价值:** AI 需要同时检查多台服务器状态，逐台执行效率太低。并行执行让 AI 一次性获取集群全貌，快速定位问题节点。

**对标工具:** Ansible 的 `--forks` 并行、pssh (parallel-ssh)

**实现要点:**
- 支持 inventory 标签过滤（`--tags app,web`）
- 结果按主机汇总，自动识别异常节点
- 可配置并发度（默认 10）
- 支持滚动更新模式（`--serial 1` 逐台执行）

---

### 2. 结构化远程输出解析

**命令:** `codex-ssh exec myserver -- "df -h" --format json`

**无人运维价值:** AI 无法可靠地从非结构化文本中提取关键信息。结构化输出让 AI 能精确判断"磁盘是否超过 80%"、"服务是否在运行"，而不是靠正则匹配猜测。

**实现要点:**
- 内置常用命令的输出解析器（df、free、uptime、ps、systemctl 等）
- 返回 JSON 格式的结构化数据
- 支持自定义解析脚本（`--parser /path/to/parser.sh`）
- 解析失败时保留原始输出，不丢信息

---

### 3. 远程文件传输

**命令:** 
```bash
codex-ssh push myserver --local ./app.conf --remote /etc/app/app.conf
codex-ssh pull myserver --remote /var/log/app.log --local ./app.log
```

**无人运维价值:** 配置文件分发、日志收集是运维最高频操作。没有文件传输能力，AI 必须退化为 `echo > file` 的低效方式，且容易出错。

**对标工具:** Ansible 的 copy/template 模块、scp

**实现要点:**
- 基于 scp/sftp 实现，不自建传输协议
- 支持目录递归传输（`--recursive`）
- 传输前自动计算 checksum，传输后校验
- 支持 `--diff` 模式：仅在内容变化时传输
- 大文件分块传输，支持断点续传

---

### 4. 远程文件编辑（非覆盖）

**命令:** `codex-ssh edit myserver --remote /etc/nginx/nginx.conf --patch "worker_processes auto;"`

**无人运维价值:** 覆盖整个文件风险极高（可能丢失其他配置项）。精准编辑让 AI 能安全修改单个配置项，降低误操作风险。

**对标工具:** Ansible 的 lineinfile/replace 模块

**实现要点:**
- 支持按行匹配替换（`--match` + `--replace`）
- 支持正则替换（`--regex`）
- 编辑前自动备份原文件
- 支持多行插入/追加
- 返回 diff 输出，供 AI 审查

---

### 5. 服务管理快捷命令

**命令:**
```bash
codex-ssh service myserver --name nginx --action status
codex-ssh service myserver --name nginx --action restart
codex-ssh service myserver --name nginx --action enable
codex-ssh service myserver --name nginx --action logs --lines 50
```

**无人运维价值:** 服务管理是运维的核心操作。封装 systemctl/init.d 差异，让 AI 用统一接口管理服务，不需要记忆每个系统的命令差异。

**实现要点:**
- 自动检测 init 系统（systemd / sysvinit / openrc）
- 统一 status/start/stop/restart/enable/disable/logs 接口
- logs 子命令返回最后 N 行日志 + 关键错误高亮
- 状态返回结构化数据（running/stopped/failed + PID + uptime）

---

### 6. 系统健康快照

**命令:** `codex-ssh snapshot myserver --output json`

**无人运维价值:** AI 诊断问题时需要全面了解系统状态。自动采集关键指标比 AI 逐条执行 `top`/`df`/`free` 高效 10 倍，且不会遗漏关键信息。

**对标工具:** Ansible facts、Gather system info

**实现要点:**
- 一次性采集：CPU、内存、磁盘、网络、负载、运行时间
- 检测异常并标注（磁盘 > 80%、内存 > 90%、负载 > CPU核数）
- 输出 JSON，包含采集时间戳
- 可对比两次快照差异（`codex-ssh snapshot diff`）

---

### 7. 命令输出捕获与流式返回

**命令:** `codex-ssh exec myserver -- "tail -f /var/log/syslog" --stream --timeout 30`

**无人运维价值:** AI 需要观察命令的实时输出（如日志尾随、部署进度），而不是等命令结束后才拿到结果。流式输出让 AI 能实时判断"部署是否成功"。

**实现要点:**
- 支持 `--stream` 模式实时输出 stdout/stderr
- 支持 `--timeout` 设置执行超时
- 支持 `--max-bytes` 限制输出大小（防止 OOM）
- 支持 `--abort-on "ERROR"` 自动终止匹配的命令

---

### 8. 连接健康检查与重试

**命令:** `codex-ssh health myserver --auto-retry`

**无人运维价值:** 网络不稳定时 AI 的命令可能随机失败。自动重试和连接健康检查让操作更可靠，AI 不需要手动处理连接超时。

**实现要点:**
- 连接前自动检测 SSH 端口可达性
- 连接失败时自动重试（可配置次数和间隔）
- 支持 ControlMaster 连接复用
- 检测断开的连接并自动重建
- 返回结构化健康状态（reachable/latency/auth_ok）

---

### 9. 操作前预览（Dry Run）

**命令:** `codex-ssh exec myserver -- "rm -rf /var/cache/*" --dry-run`

**无人运维价值:** AI 做破坏性操作前需要先预览。Dry run 让 AI 能向用户确认"我将要执行什么"，或者自主判断操作是否安全。

**实现要点:**
- 显示即将执行的完整命令
- 对于文件操作，显示将影响的文件列表
- 对于服务操作，显示将产生的状态变化
- 返回 `safe`/`caution`/`dangerous` 风险等级
- AI 可据此决定是否继续执行

---

## P1 — 重要增强 (Important for Adoption)

> 这些功能显著提升工具的实用性和可靠性，是获得广泛采用的关键。

### 10. Playbook / 任务编排

**命令:**
```bash
codex-ssh play myserver --tasks deploy.yaml
codex-ssh play --targets "app-*" --tasks rolling-update.yaml --serial 2
```

**无人运维价值:** 复杂运维操作包含多个步骤（备份 → 更新 → 验证 → 切换），AI 需要可靠的编排能力来执行多步操作，而不是每次都从头规划。

**对标工具:** Ansible playbook

**实现要点:**
- YAML 格式的任务定义
- 支持步骤间条件判断（`when`）
- 支持变量替换
- 每步执行后自动验证成功/失败
- 失败时自动回滚或停止
- 支持 `--check`（dry run 整个 playbook）

---

### 11. 配置文件管理（模板化）

**命令:** `codex-ssh template myserver --src ./nginx.conf.j2 --dest /etc/nginx/nginx.conf --var port=8080`

**无人运维价值:** 配置文件管理是运维的核心挑战。模板化让 AI 能根据环境差异生成正确的配置，避免硬编码和手动替换错误。

**对标工具:** Ansible template 模块、Jinja2

**实现要点:**
- 支持 Go template 语法（与 Jinja2 类似）
- 变量从命令行参数、环境变量、inventory 中获取
- 渲染前自动备份目标文件
- `--diff` 模式显示变更内容
- `--check` 模式只渲染不部署

---

### 12. 状态感知与变更验证

**命令:** `codex-ssh verify myserver --check "nginx" --expected running`

**无人运维价值:** 执行操作后验证结果是运维闭环的关键。AI 需要工具告诉它"操作是否成功"，而不是靠猜测。状态感知让 AI 能自主完成"执行 → 验证 → 报告"的闭环。

**实现要点:**
- 内置常见服务状态检查（service running, port open, process alive）
- 支持自定义检查脚本
- 返回 pass/fail + 详细信息
- 支持组合条件（AND/OR）
- 可作为 playbook 的验证步骤

---

### 13. 会话录制与回放

**命令:**
```bash
codex-ssh record start myserver --session-id deploy-20260531
codex-ssh record stop --session-id deploy-20260531
codex-ssh record replay --session-id deploy-20260531 --format text
```

**无人运维价值:** 合规要求所有运维操作可审计。会话录制让 AI 的每次操作都有完整记录，便于事后审查和问题追溯。

**对标工具:** asciinema、screen session recording

**实现要点:**
- 基于 script 或 asciinema 协议录制终端会话
- 保存时附带元数据（用户、主机、时间、操作摘要）
- 支持回放为文本或时间戳格式
- 与审计日志关联
- 自动过期清理

---

### 14. Inventory 动态发现

**命令:** 
```bash
codex-ssh hosts discover --source aws --region us-east-1
codex-ssh hosts discover --source ssh-config --file ~/.ssh/config
codex-ssh hosts discover --source cmdb --url http://cmdb.internal/api/hosts
```

**无人运维价值:** 手动维护服务器清单容易过期。动态发现让 AI 能自动获取最新服务器列表，确保操作对象是最新的。

**对标工具:** Ansible dynamic inventory

**实现要点:**
- SSH config 导入（已有基础）
- AWS EC2 / 阿里云 ECS 实例发现
- 自定义 HTTP API 作为数据源
- 定时自动刷新
- 与现有 inventory 合并策略（merge/replace）

---

### 15. 权限升级管理

**命令:** `codex-ssh exec myserver -- "apt update" --sudo --sudo-user root`

**无人运维价值:** 很多运维操作需要 root 权限。封装 sudo 提升路径，让 AI 能安全地执行特权操作，而不是让 AI 自己处理 sudo 密码交互。

**对标工具:** Ansible become

**实现要点:**
- 支持 `--sudo` / `--become` 标志
- 自动检测 sudo 是否需要密码
- 通过 askpass 注入 sudo 密码（如果需要）
- 支持 `--sudo-user` 指定目标用户
- 与 audit 日志关联，记录权限提升

---

### 16. 网络诊断工具包

**命令:**
```bash
codex-ssh netdiag myserver --target web-server:443
codex-ssh netdiag myserver --target web-server:443 --trace
```

**无人运维价值:** 网络问题是运维最常见的故障类型。内置网络诊断让 AI 能快速排查连通性、延迟、DNS 解析等问题，而不需要逐条执行 ping/traceroute/nc。

**实现要点:**
- 自动组合测试：DNS 解析 → TCP 连通 → TLS 握手 → HTTP 响应
- 支持 `--trace` 显示完整路径
- 返回结构化结果（每步耗时、状态）
- 支持多目标批量诊断
- 支持 `--compare` 对比多台主机的网络状态

---

### 17. 变量与环境管理

**命令:**
```bash
codex-ssh var set myserver --name APP_VERSION --value 2.1.0
codex-ssh var get myserver --name APP_VERSION
codex-ssh var list myserver
```

**无人运维价值:** 环境变量和配置值分散在多台服务器上，AI 需要统一的接口来管理这些状态。变量管理让 AI 能查询和设置远程环境，而不需要 SSH 进去编辑文件。

**实现要点:**
- 支持读取 /etc/environment、~/.profile、systemd Environment
- 支持设置（写入对应配置文件）
- 返回所有环境变量的结构化列表
- 支持按前缀过滤（如 `APP_`）
- 与 inventory 标签关联，支持跨主机变量对比

---

### 18. 滚动操作与编排

**命令:** `codex-ssh exec --targets "web-*" --serial 2 -- "systemctl reload nginx"`

**无人运维价值:** 大规模部署不能同时重启所有服务器。滚动操作让 AI 能安全地逐批更新服务，确保不中断整体可用性。

**对标工具:** Ansible `serial`、rolling update

**实现要点:**
- `--serial N` 每批 N 台
- 批次间自动等待健康检查通过
- 支持 `--max-failures N` 超过失败阈值自动停止
- 支持 `--pre-task` / `--post-task` 在每批前后执行
- 实时显示进度（已完成/进行中/待执行）

---

### 19. 差异检测与一致性校验

**命令:**
```bash
codex-ssh diff myserver --file /etc/nginx/nginx.conf --expected ./nginx.conf
codex-ssh drift --targets "app-*" --file /etc/app/config.yaml
```

**无人运维价值:** 配置漂移是运维的大敌。AI 需要知道"服务器上的配置是否和预期一致"，差异检测让 AI 能发现并修复配置不一致的问题。

**对标工具:** Ansible `--check --diff`、cfengine

**实现要点:**
- 文件级 diff 对比
- 支持多主机一致性扫描
- 返回差异报告（哪些主机不一致、具体差异内容）
- 支持基于 checksum 的快速一致性检查
- 可与 playbook 配合自动修复

---

### 20. 错误恢复与自动修复

**命令:** `codex-ssh recover myserver --scenario "service-crash" --auto-fix`

**无人运维价值:** 简单的自动化恢复（重启服务、释放磁盘空间、清理临时文件）不需要人工介入。自动修复让 AI 能处理 80% 的常见故障，只有复杂问题才升级到人工。

**实现要点:**
- 预定义常见故障场景和修复动作
- 自动诊断故障类型
- 执行修复前先备份/快照
- 修复后自动验证
- 记录修复过程和结果
- 支持自定义修复脚本

---

## P2 — 增强特性 (Nice to Have)

> 这些功能扩展工具的能力边界，适合在核心功能稳定后逐步添加。

### 21. 多协议支持（非 SSH）

**命令:** `codex-ssh exec myserver --protocol kubectl -- "get pods"`

**无人运维价值:** 现代运维不止 SSH。容器化环境需要 kubectl，云环境需要 API 调用。多协议让 AI 能用统一工具管理不同类型的基础设施。

**实现要点:**
- 插件式协议适配器
- 优先支持 kubectl（Kubernetes 命令执行）
- 支持 Docker exec
- 支持自定义协议插件
- 统一的审计日志格式

---

### 22. 分布式任务执行

**命令:** `codex-ssh fabric myserver --task ./tasks/rolling-update.py`

**无人运维价值:** 复杂的运维任务需要在多台服务器上协调执行（如数据库迁移需要在所有节点上按顺序执行）。分布式任务让 AI 能编排跨主机的复杂流程。

**对标工具:** Fabric

**实现要点:**
- Python/Go 脚本定义任务
- 任务间共享状态
- 支持条件分支和循环
- 失败回滚策略
- 执行过程可视化

---

### 23. 告警与通知集成

**命令:** `codex-ssh notify --channel slack --message "Deploy complete: 3/3 nodes updated"`

**无人运维价值:** 运维操作需要通知相关人员。AI 完成操作后自动发通知，让团队知道发生了什么，而不需要 AI 额外处理通知逻辑。

**实现要点:**
- 支持 Webhook（Slack、钉钉、企业微信）
- 支持邮件通知
- 与审计日志关联，自动附带操作摘要
- 支持告警规则（如：操作失败时通知）
- 支持静默时间段

---

### 24. 成本与资源分析

**命令:** `codex-ssh analyze myserver --resource cpu --period 7d`

**无人运维价值:** AI 能帮助识别资源浪费和优化机会。成本分析让 AI 不仅能执行操作，还能给出优化建议。

**实现要点:**
- 采集历史资源使用数据
- 识别高负载/低利用率时段
- 生成优化建议（如：缩减实例规格）
- 支持多主机聚合分析
- 与云账单 API 集成（可选）

---

### 25. 证书与密钥自动轮换

**命令:** `codex-ssh cert rotate myserver --type ssh --user deploy`

**无人运维价值:** 证书过期是常见的运维事故。自动轮换让 AI 能定期更新证书和密钥，避免人工忘记导致的服务中断。

**对标工具:** Vault PKI、cert-manager

**实现要点:**
- 检测证书过期时间
- 自动申请新证书
- 自动部署新证书
- 自动重启受影响的服务
- 与审计日志关联

---

### 26. 蓝绿部署支持

**命令:** `codex-ssh deploy myserver --strategy blue-green --app myapp --version 2.1.0`

**无人运维价值:** 零停机部署需要精确的流量切换。蓝绿部署让 AI 能安全地发布新版本，自动处理流量切换和回滚。

**实现要点:**
- 自动管理蓝/绿环境
- 流量切换（通过 nginx/upstream 配置）
- 健康检查验证
- 自动回滚（如果健康检查失败）
- 支持自定义切换逻辑

---

### 27. 日志聚合与分析

**命令:**
```bash
codex-ssh logs myserver --service nginx --since 1h
codex-ssh logs --targets "app-*" --service nginx --since 1h --search "error"
```

**无人运维价值:** 集中查看多台服务器的日志是排障的关键。日志聚合让 AI 能跨主机搜索日志，快速定位问题根因。

**实现要点:**
- 支持 systemd journal、syslog、自定义日志文件
- 跨主机日志聚合
- 支持关键词搜索和正则过滤
- 支持时间范围过滤
- 返回结构化日志（时间、级别、消息、来源主机）

---

### 28. 自定义检查与告警规则

**命令:**
```bash
codex-ssh check add --name "disk-space" --script ./checks/disk.sh --interval 5m
codex-ssh check run myserver --name disk-space
codex-ssh check list
```

**无人运维价值:** 标准化的健康检查让 AI 能持续监控服务器状态，发现异常及时处理，而不是等用户报告问题。

**实现要点:**
- 支持自定义检查脚本
- 支持定时执行
- 支持阈值告警
- 检查结果记录到审计日志
- 支持多主机批量检查

---

### 29. 配置版本控制集成

**命令:**
```bash
codex-ssh config track myserver --file /etc/nginx/nginx.conf --repo ./infra-configs
codex-ssh config sync myserver --file /etc/nginx/nginx.conf --from ./infra-configs
```

**无人运维价值:** 所有配置变更应该版本化。配置版本控制让 AI 的每次配置修改都能追溯，支持回滚和审计。

**实现要点:**
- 自动检测本地 Git 仓库
- 配置变更自动提交
- 支持差异比较
- 支持回滚到指定版本
- 与审计日志关联

---

### 30. AI 上下文优化

**命令:** `codex-ssh context myserver --compress`

**无人运维价值:** AI 的上下文窗口有限。上下文优化让 AI 能用更少的 token 获取更完整的信息，提高决策质量。

**实现要点:**
- 压缩命令输出（只保留关键信息）
- 智能摘要（长输出自动生成摘要）
- 上下文缓存（避免重复采集相同信息）
- 优先级排序（先返回最关键的系统状态）
- 支持 `--detail` / `--brief` 切换详细度

---

### 31. 远程环境一致性管理

**命令:**
```bash
codex-ssh env sync myserver --role web --env production
codex-ssh env diff --targets "app-*" --role web
```

**无人运维价值:** 环境不一致是问题根源。一致性管理让 AI 能确保所有服务器的运行环境一致，减少"在我机器上能跑"的问题。

**对标工具:** Ansible roles、Docker

**实现要点:**
- 定义角色（web、db、cache）所需的环境
- 自动检测缺失的依赖
- 自动安装缺失的依赖
- 环境差异报告
- 支持锁定版本

---

### 32. 智能故障分析

**命令:** `codex-ssh analyze myserver --incident "nginx 502 errors since 14:00"`

**无人运维价值:** 故障分析需要跨多个数据源（日志、指标、配置）综合判断。智能分析让 AI 能像资深 SRE 一样，系统性地分析故障根因。

**实现要点:**
- 自动关联多个数据源
- 生成故障时间线
- 列出可能的根因（按概率排序）
- 推荐修复步骤
- 支持自定义分析规则

---

## 实施优先级总览

| 阶段 | 功能编号 | 功能名称 | 预计周期 |
|------|---------|---------|---------|
| P0 | #1 | 多主机并行执行 | 1-2 周 |
| P0 | #2 | 结构化远程输出解析 | 1 周 |
| P0 | #3 | 远程文件传输 | 1-2 周 |
| P0 | #4 | 远程文件编辑（非覆盖） | 1 周 |
| P0 | #5 | 服务管理快捷命令 | 1 周 |
| P0 | #6 | 系统健康快照 | 1 周 |
| P0 | #7 | 命令输出捕获与流式返回 | 1 周 |
| P0 | #8 | 连接健康检查与重试 | 1 周 |
| P0 | #9 | 操作前预览（Dry Run） | 1 周 |
| P1 | #10 | Playbook / 任务编排 | 2-3 周 |
| P1 | #11 | 配置文件管理（模板化） | 1-2 周 |
| P1 | #12 | 状态感知与变更验证 | 1-2 周 |
| P1 | #13 | 会话录制与回放 | 1-2 周 |
| P1 | #14 | Inventory 动态发现 | 2 周 |
| P1 | #15 | 权限升级管理 | 1 周 |
| P1 | #16 | 网络诊断工具包 | 1-2 周 |
| P1 | #17 | 变量与环境管理 | 1 周 |
| P1 | #18 | 滚动操作与编排 | 1-2 周 |
| P1 | #19 | 差异检测与一致性校验 | 1-2 周 |
| P1 | #20 | 错误恢复与自动修复 | 2 周 |
| P2 | #21-#32 | 高级特性 | 各 1-3 周 |

---

## 与现有功能的衔接

### 已实现（基线）

| 功能 | 状态 | 对应 Roadmap |
|------|------|-------------|
| Inventory 管理 | ✅ 已有 | #14 基础 |
| 跳板机穿透 | ✅ 已有 | — |
| 密码管理 (Keychain) | ✅ 已有 | #15 基础 |
| 命令执行 (exec/shell) | ✅ 已有 | #7 基础 |
| 端口转发 (tunnel) | ✅ 已有 | — |
| SOCKS5 代理 | ✅ 已有 | — |
| 长任务管理 (jobs) | ✅ 已有 | #10 基础 |
| 审计日志 | ✅ 已有 | #13 基础 |
| 诊断 (diagnose) | ✅ 已有 | #16 基础 |
| 本地自检 (doctor) | ✅ 已有 | #8 基础 |
| SSH config 导入 | ✅ 已有 | #14 部分 |

### 未实现（Roadmap 目标）

主要缺口集中在：
1. **批量操作** — 并行执行、滚动更新
2. **文件管理** — 传输、编辑、模板
3. **状态管理** — 健康检查、一致性校验、自动修复
4. **编排能力** — Playbook、多步任务
5. **AI 协作优化** — 结构化输出、上下文优化

---

## 安全设计原则（贯穿所有功能）

1. **最小权限** — 每个功能只请求必要的权限
2. **审计完备** — 所有操作都记录到审计日志
3. **密码隔离** — 密码永远不出现在命令行、日志、配置文件中
4. **预览优先** — 破坏性操作必须有预览和确认
5. **回滚能力** — 每个写操作都有对应的回滚路径
6. **速率限制** — 防止 AI 误操作导致的雪崩效应

---

## AI 协作设计原则

1. **结构化交互** — 所有输入输出都是机器可解析的
2. **错误友好** — 错误信息包含具体的修复建议
3. **幂等操作** — 重复执行相同操作结果一致
4. **上下文感知** — 工具记住操作历史，避免重复采集
5. **渐进式披露** — 默认返回摘要，需要时提供详细信息
6. **安全边界** — 工具内部处理敏感操作，不暴露给 AI prompt

---

*This roadmap is a living document. Priorities may shift based on user feedback and usage patterns.*
