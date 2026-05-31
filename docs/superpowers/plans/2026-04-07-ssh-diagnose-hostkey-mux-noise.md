# SSH Diagnose / Host Key / Multiplex Noise Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 `codex-ssh` 的三个连接体验问题：`diagnose` 错误细节缺失、首次连接缺少 host key 接受写入流程、控制连接复用时的重复噪音。

**Architecture:** 在 CLI 层加入首次连接 host key 预热流程，在 SSH 参数构造层改为稳定且可检查的控制 socket 路径，并在失败输出路径上保留底层 `ssh stderr`。尽量不改现有命令语义，只增强连接前处理和错误展示。

**Tech Stack:** Go, OpenSSH (`ssh`, `ssh-keyscan`, `ssh-keygen`), Go testing

---

### Task 1: Diagnose 错误细节透传

**Files:**
- Modify: `internal/cli/app.go`
- Test: `internal/cli/app_test.go`

- [ ] **Step 1: Write the failing test**
  新增 `diagnose` 失败测试，断言 stderr 同时包含高层错误和底层 `ssh stderr` 文本。

- [ ] **Step 2: Run test to verify it fails**
  Run: `go test ./internal/cli -run TestDiagnoseFailureIncludesSSHStderr`
  Expected: FAIL，因为当前实现只打印 `exit status 255`。

- [ ] **Step 3: Write minimal implementation**
  在 `runDiagnose` 的失败分支拼接 `result.Stderr` 摘要。

- [ ] **Step 4: Run test to verify it passes**
  Run: `go test ./internal/cli -run TestDiagnoseFailureIncludesSSHStderr`
  Expected: PASS

### Task 2: 首次连接自动接受并写入 host key

**Files:**
- Modify: `internal/cli/app.go`
- Test: `internal/cli/app_test.go`

- [ ] **Step 1: Write the failing test**
  新增测试，断言当前台命令连接一个未缓存 host key 的目标时，会先执行 host key 接受流程，再继续 SSH 命令。

- [ ] **Step 2: Run test to verify it fails**
  Run: `go test ./internal/cli -run TestDiagnoseAcceptsAndWritesMissingHostKeyBeforeConnect`
  Expected: FAIL，因为当前实现没有 host key 预热流程。

- [ ] **Step 3: Write minimal implementation**
  在前台命令入口共享一段 `ensureKnownHosts` 逻辑，对目标与跳板链逐个探测并写入 `known_hosts`。

- [ ] **Step 4: Run test to verify it passes**
  Run: `go test ./internal/cli -run TestDiagnoseAcceptsAndWritesMissingHostKeyBeforeConnect`
  Expected: PASS

### Task 3: 控制连接降噪

**Files:**
- Modify: `internal/sshargs/sshargs.go`
- Test: `internal/sshargs/sshargs_test.go`

- [ ] **Step 1: Write the failing test**
  新增参数测试，断言控制路径使用稳定的实际 socket 路径，并能根据路径存在性切换为更安静的复用模式。

- [ ] **Step 2: Run test to verify it fails**
  Run: `go test ./internal/sshargs -run TestBuildExecArgsUsesStableControlSocketPath`
  Expected: FAIL，因为当前实现仍使用 `%C` 模板且无法做存在性判断。

- [ ] **Step 3: Write minimal implementation**
  生成稳定短路径的 control socket，已存在时改用连接已有 socket 的参数组合，避免重复噪音。

- [ ] **Step 4: Run test to verify it passes**
  Run: `go test ./internal/sshargs -run TestBuildExecArgsUsesStableControlSocketPath`
  Expected: PASS

### Task 4: 全量回归

**Files:**
- Verify: `internal/cli/app_test.go`
- Verify: `internal/sshargs/sshargs_test.go`
- Verify: `internal/jobs/jobs_test.go`
- Verify: `internal/executor/executor_test.go`

- [ ] **Step 1: Run targeted packages**
  Run: `go test ./internal/cli ./internal/sshargs ./internal/jobs ./internal/executor`
  Expected: PASS

- [ ] **Step 2: Run full test suite**
  Run: `go test ./...`
  Expected: PASS
