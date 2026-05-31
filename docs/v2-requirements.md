# Codex SSH v2.0.0 — 需求规划

> 基于用户调研 + 行业趋势 + 竞品分析，定义下一代功能

---

## 一、用户需求排名（按频率）

| 排名 | 需求 | 来源 | 紧迫度 |
|------|------|------|--------|
| 🥇 | **SFTP 文件传输** | ssh-mcp-server/ssh-mcp 多个 issue | 极高 |
| 🥈 | **动态主机管理** | 用户反馈"不想重启添加主机" | 高 |
| 🥉 | **Sudo/Su 提权** | 真实运维场景必备 | 高 |
| 4 | **连接池 + 会话持久化** | PTY 累积 bug、连接断开 | 高 |
| 5 | **Windows 支持** | 跨平台一致性 | 中 |
| 6 | **Kubernetes 集成** | K8s 是 #1 集成需求 | 中 |
| 7 | **Prometheus/监控集成** | 健康检查 + 指标采集 | 中 |
| 8 | **Playbook 引擎** | 批量编排 + 自动化 | 中 |
| 9 | **Web Dashboard** | 可观测性 + 状态视图 | 低 |
| 10 | **告警通知** | Webhook/邮件/Slack | 低 |

---

## 二、行业趋势（2026 AIOps）

| 趋势 | 说明 | 对 v2.0 的启示 |
|------|------|---------------|
| **Agentic ITOps** | AI Agent 自主执行 L1 运维 | codex-ssh 要支持"自主模式" |
| **Self-Healing** | 自动检测 + 自动修复 | 需要健康检查 + 自愈策略引擎 |
| **Root Cause Analysis** | 日志+指标联合分析 | 需要诊断增强 |
| **Alert Correlation** | 20+ 监控工具告警聚合 | 需要告警接口 |
| **Predictive Maintenance** | 预测性维护 | 需要指标采集 + 基线 |

---

## 三、v2.0.0 功能清单

### P0 — 必须有（核心竞争力）

#### 1. SFTP 文件传输
```bash
codex-ssh put myserver localfile.txt /remote/path/
codex-ssh get myserver /remote/file.txt ./local/
codex-ssh sync myserver ./dist/ /var/www/html/
```
- 上传/下载/同步
- 支持通配符
- 进度显示
- 通过 MCP 暴露给 AI

#### 2. Sudo/Su 提权
```bash
codex-ssh exec myserver --sudo "apt update"
codex-ssh exec myserver --su root "systemctl restart nginx"
```
- 通过 askpass 安全传递 sudo 密码
- 不在进程列表中暴露密码
- 支持 NOPASSWD 和密码模式

#### 3. 动态主机管理
```bash
codex-ssh hosts add dynamic-web --host 10.0.0.1 --user deploy
codex-ssh hosts reload                    # 热重载，不中断连接
codex-ssh hosts discover 10.0.0.0/24      # 网段扫描自动发现
```
- 运行时添加/删除主机，无需重启
- 网段扫描自动发现
- SSH 密钥探测

#### 4. 连接池 + 会话复用
- 自动复用 SSH 连接（ControlMaster 增强）
- 连接健康检查 + 自动重连
- 并发连接限制
- 连接状态监控

#### 5. MCP 工具增强
```yaml
新增 MCP Tools:
  - ssh_sftp_put: 上传文件
  - ssh_sftp_get: 下载文件
  - ssh_sudo_exec: 提权执行
  - ssh_hosts_discover: 主机发现
  - ssh_hosts_reload: 热重载
  - ssh_connections: 查看连接状态
```

---

### P1 — 应该有（提升体验）

#### 6. Playbook 引擎
```yaml
# playbooks/deploy.yaml
name: Deploy Application
hosts: @web
steps:
  - name: Pull latest code
    exec: cd /var/www/app && git pull
  - name: Install dependencies
    exec: npm install --production
    sudo: true
  - name: Restart service
    exec: systemctl restart app
    sudo: true
  - name: Health check
    exec: curl -f http://localhost:3000/health
    retries: 3
    delay: 5
```
- YAML 定义运维流程
- 支持条件分支、错误处理、重试
- 支持 sudo
- 支持多主机编排

#### 7. 健康检查系统
```bash
codex-ssh health check              # 检查所有主机
codex-ssh health check --tag web    # 检查 web 组
codex-ssh health report             # 生成健康报告
```
- 定期扫描 inventory 中所有主机
- CPU/内存/磁盘/网络/进程 基础指标
- 状态聚合 + 历史趋势
- 异常告警

#### 8. 跨平台密钥管理
- Linux: pass / gnome-keyring / kwallet
- macOS: Keychain（已有）
- Windows: Windows Credential Manager
- 统一接口，透明切换

#### 9. 审计增强
```bash
codex-ssh audit query --format json     # JSON 输出
codex-ssh audit query --format table    # 表格输出
codex-ssh audit export --since 7d       # 导出最近7天
codex-ssh audit stats                   # 统计概览
```
- 多格式输出（JSON/表格/文本）
- 时间范围查询
- 操作统计
- 合规报告导出

---

### P2 — 可以有（差异化）

#### 10. Kubernetes 集成
```bash
codex-ssh k8s pods                     # 列出 pod
codex-ssh k8s exec pod-name -- ls      # 在 pod 中执行
codex-ssh k8s logs pod-name            # 查看日志
codex-ssh k8s port-forward pod 8080    # 端口转发
```
- 通过 SSH 隧道访问 K8s API
- 无需本地 kubectl 配置
- AI 通过 SSH 管理 K8s

#### 11. Prometheus 指标导出
```bash
codex-ssh metrics serve --port 9090    # 暴露 /metrics 端点
```
- 导出连接数、命令执行数、错误率等指标
- 兼容 Prometheus 格式
- 可接入 Grafana

#### 12. 告警通知
```bash
codex-ssh notify webhook --url https://hooks.slack.com/xxx
codex-ssh notify email --to admin@example.com
```
- 执行结果推送
- 健康检查异常通知
- Webhook/邮件/Slack/企微

#### 13. Web Dashboard（可选）
- 轻量级 Web UI
- 主机状态视图
- 连接管理
- 审计日志查看
- 技术栈：Go + 简单 HTML（不需要 React）

---

## 四、技术实现优先级

```
v2.0.0-alpha (4周)
├── SFTP 文件传输
├── Sudo/Su 提权
├── 动态主机管理
├── 连接池增强
└── MCP 工具更新

v2.0.0-beta (4周)
├── Playbook 引擎
├── 健康检查系统
├── 跨平台密钥
└── 审计增强

v2.0.0-rc (2周)
├── Bug fixes
├── 文档完善
├── 性能优化
└── 社区反馈
```

---

## 五、与 v1.0.0 的对比

| 维度 | v1.0.0 | v2.0.0 |
|------|--------|--------|
| **定位** | SSH 管理工具 | AI 运维平台 |
| **文件传输** | ❌ | ✅ SFTP |
| **提权执行** | ❌ | ✅ sudo/su |
| **动态管理** | ❌ 热重载 | ✅ 运行时增删 |
| **连接管理** | 基础 | ✅ 连接池 + 健康检查 |
| **自动化** | 单命令 | ✅ Playbook 编排 |
| **可观测性** | 审计日志 | ✅ 健康检查 + 指标 |
| **AI 集成** | MCP 基础 | ✅ MCP 全功能 |
| **平台** | macOS/Linux | ✅ +Windows |

---

## 六、成功标准

| 指标 | 目标 |
|------|------|
| GitHub Stars | 500+ (v1.0 时 0) |
| MCP 工具数 | 12+ (v1.0 时 4) |
| 安装方式 | Homebrew + Binary + go install + install.sh |
| 测试覆盖率 | 70%+ |
| 文档完整度 | CLI 参考 + 使用指南 + 架构文档 |
| 社区 | 3+ 贡献者, 10+ Issues |
