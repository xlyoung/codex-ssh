# SSH Password Secrets And IP Resolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `codex-ssh` 增加“inventory 优先、裸 IP fallback”的目标解析，以及基于 macOS Keychain 的密码凭据管理与自动取密执行链。

**Architecture:** 在现有 CLI/hosts/sshargs/executor 基础上，新增 `secrets` 与 `askpass` 两个明确边界的模块。目标解析仍留在 `internal/cli/target.go`，凭据存储与读取放在 `internal/secrets`，一次性 askpass 脚本和环境注入放在 `internal/askpass` 与 `internal/executor`，避免把密码逻辑散落进各命令分支。

**Tech Stack:** Go 1.22、OpenSSH、macOS `security` CLI、TOML、JSONL

---

### Task 1: Extend Data Model For Secret References And Askpass Runtime

**Files:**
- Modify: `pkg/model/types.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `defaults/config.toml`
- Modify: `defaults/hosts.toml`
- Modify: `scripts/bootstrap_runtime_files.sh`

- [ ] **Step 1: Write the failing tests**

```go
func TestResolvePathsIncludesAskpassDir(t *testing.T) {
    t.Setenv("CODEX_SSH_HOME", t.TempDir())
    paths, err := ResolvePaths()
    if err != nil {
        t.Fatal(err)
    }
    if paths.AskpassDir == "" {
        t.Fatal("expected askpass dir")
    }
}

func TestHostModelSupportsSecretRef(t *testing.T) {
    host := model.Host{Host: "192.168.1.101", SecretRef: "ssh://appuser@192.168.1.101:22"}
    if host.SecretRef == "" {
        t.Fatal("expected secret ref")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/config ./pkg/model`
Expected: FAIL because `AskpassDir` / `SecretRef` do not exist.

- [ ] **Step 3: Write minimal implementation**

```go
type Paths struct {
    // ...
    AskpassDir string
}

type Host struct {
    // ...
    SecretRef string `toml:"secret_ref" json:"secret_ref,omitempty"`
}

type ResolvedHost struct {
    // ...
    SecretRef string
}
```

- [ ] **Step 4: Update defaults and bootstrap directories**

Make `buildPaths()` populate `run/askpass`, and ensure bootstrap scripts create it.

- [ ] **Step 5: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/config ./...`
Expected: PASS for config/model-related tests with no regressions in unrelated packages.

- [ ] **Step 6: Commit**

```bash
git add pkg/model/types.go internal/config/config.go internal/config/config_test.go defaults/config.toml defaults/hosts.toml scripts/bootstrap_runtime_files.sh
git commit -m "扩展密码凭据引用和 askpass 运行目录模型"
```

### Task 2: Make Positional Target Resolution Prefer Inventory Then Fallback To Raw Endpoint

**Files:**
- Modify: `internal/cli/target.go`
- Modify: `internal/cli/app_test.go`
- Modify: `internal/hosts/hosts_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestResolveTargetPrefersInventoryKeyBeforeEndpointFallback(t *testing.T) {
    cfg := model.Config{DefaultUser: "root", DefaultPort: 22, DefaultAuth: "agent"}
    inv := model.Inventory{
        Hosts: map[string]model.Host{
            "192.168.1.101": {
                Host: "192.168.1.101",
                User: "appuser",
                Via: []string{"192.168.1.100"},
                Auth: "password",
                SecretRef: "ssh://appuser@192.168.1.101:22",
            },
            "192.168.1.100": {Host: "192.168.1.100", User: "root"},
        },
    }

    resolved, err := resolveTarget(cfg, inv, "192.168.1.101", targetInput{})
    if err != nil {
        t.Fatal(err)
    }
    if resolved.User != "appuser" || resolved.Auth != "password" || len(resolved.Via) != 1 {
        t.Fatalf("unexpected resolved host: %+v", resolved)
    }
}

func TestResolveTargetFallsBackToEndpointWhenInventoryMisses(t *testing.T) {
    cfg := model.Config{DefaultUser: "root", DefaultPort: 22, DefaultAuth: "agent"}
    resolved, err := resolveTarget(cfg, model.Inventory{Hosts: map[string]model.Host{}}, "appuser@192.168.1.101:2222", targetInput{})
    if err != nil {
        t.Fatal(err)
    }
    if resolved.User != "appuser" || resolved.Host != "192.168.1.101" || resolved.Port != 2222 {
        t.Fatalf("unexpected resolved host: %+v", resolved)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/cli ./internal/hosts`
Expected: FAIL because alias mode currently requires inventory hit and does not fallback to endpoint parsing.

- [ ] **Step 3: Write minimal implementation**

```go
func resolveTarget(cfg model.Config, inv model.Inventory, alias string, input targetInput) (model.ResolvedHost, error) {
    if alias != "" && noExplicitHostFlags(input) {
        if _, ok := inv.Hosts[alias]; ok {
            return hosts.Resolve(inv, cfg, alias)
        }
        return parseEndpointSpec(alias, cfg)
    }
    // existing --host path
}
```

- [ ] **Step 4: Carry inventory-only fields into resolved host**

When resolving from inventory, preserve `SecretRef`, `Via`, `Auth`, `Workdir`. When falling back to endpoint parsing, do not invent `Via`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/cli ./internal/hosts`
Expected: PASS, including existing ad-hoc and diagnose tests.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/target.go internal/cli/app_test.go internal/hosts/hosts_test.go
git commit -m "支持 inventory 优先和裸目标回退解析"
```

### Task 3: Add Secret Reference Resolution And macOS Keychain Backend

**Files:**
- Create: `internal/secrets/secrets.go`
- Create: `internal/secrets/secrets_darwin.go`
- Create: `internal/secrets/secrets_unsupported.go`
- Create: `internal/secrets/secrets_test.go`
- Create: `internal/secrets/secrets_darwin_test.go`
- Modify: `internal/cli/app.go`
- Modify: `pkg/model/types.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestSecretRefDefaultsToSSHURL(t *testing.T) {
    host := model.ResolvedHost{Host: "192.168.1.101", User: "appuser", Port: 22}
    ref := secrets.RefForHost(host)
    if ref != "ssh://appuser@192.168.1.101:22" {
        t.Fatalf("unexpected ref: %s", ref)
    }
}

func TestSecretRefPrefersExplicitValue(t *testing.T) {
    host := model.ResolvedHost{
        Host: "192.168.1.101", User: "appuser", Port: 22,
        SecretRef: "ssh://custom/app-171",
    }
    if got := secrets.RefForHost(host); got != "ssh://custom/app-171" {
        t.Fatalf("unexpected ref: %s", got)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/secrets`
Expected: FAIL because package and helpers do not exist.

- [ ] **Step 3: Write minimal implementation**

```go
func RefForHost(host model.ResolvedHost) string {
    if strings.TrimSpace(host.SecretRef) != "" {
        return host.SecretRef
    }
    return fmt.Sprintf("ssh://%s@%s:%d", host.User, host.Host, host.Port)
}
```

- [ ] **Step 4: Add backend interface and darwin implementation**

Implement methods:

```go
type Store interface {
    Set(ctx context.Context, ref string, password string) error
    Get(ctx context.Context, ref string) (string, error)
    Delete(ctx context.Context, ref string) error
}
```

Use `security add-generic-password`, `find-generic-password -w`, `delete-generic-password`.

- [ ] **Step 5: Add unsupported-platform guard**

`secrets_unsupported.go` should return a clear error like `password secret backend is not available on this platform`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/secrets`
Expected: PASS with unit tests using a fake command runner or helper abstraction instead of hitting real Keychain.

- [ ] **Step 7: Commit**

```bash
git add internal/secrets pkg/model/types.go internal/cli/app.go
git commit -m "增加 macOS keychain 密码凭据后端"
```

### Task 4: Add Askpass Script Builder And Executor Environment Injection

**Files:**
- Create: `internal/askpass/askpass.go`
- Create: `internal/askpass/askpass_test.go`
- Modify: `internal/executor/executor.go`
- Modify: `internal/executor/executor_test.go`
- Modify: `pkg/model/types.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestPrepareAskpassWritesScriptAndEnv(t *testing.T) {
    dir := t.TempDir()
    result, err := askpass.Prepare(dir, "secret-value")
    if err != nil {
        t.Fatal(err)
    }
    if result.ScriptPath == "" || result.Env["SSH_ASKPASS_REQUIRE"] != "force" {
        t.Fatalf("unexpected askpass result: %+v", result)
    }
}

func TestExecUsesAskpassEnvWhenProvided(t *testing.T) {
    runner := &fakeRunner{}
    svc := executor.Service{Runner: runner, Config: model.Config{}}
    _, _ = svc.Exec(context.Background(), model.ExecRequest{
        Command: "hostname",
        ResolvedHost: model.ResolvedHost{Host: "192.168.1.101", User: "appuser"},
        AuthEnv: map[string]string{"SSH_ASKPASS_REQUIRE": "force"},
    })
    if runner.env["SSH_ASKPASS_REQUIRE"] != "force" {
        t.Fatalf("expected askpass env, got %+v", runner.env)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/askpass ./internal/executor`
Expected: FAIL because askpass package and env-aware runner path do not exist.

- [ ] **Step 3: Write minimal implementation**

```go
type Prepared struct {
    ScriptPath string
    Env map[string]string
    Cleanup func() error
}
```

Write a small shell script that prints the password from environment, chmod `0700`, and return cleanup function removing the script.

- [ ] **Step 4: Extend executor Runner interface carefully**

Refactor runner methods to accept env without breaking background execution tests. A minimal shape is:

```go
Run(ctx context.Context, name string, args []string, interactive bool, env map[string]string) (model.CommandResult, error)
```

Update all fake runners and callers in one patch.

- [ ] **Step 5: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/askpass ./internal/executor`
Expected: PASS with cleanup verified and existing password-auth tests still green.

- [ ] **Step 6: Commit**

```bash
git add internal/askpass internal/executor pkg/model/types.go
git commit -m "增加 askpass 脚本生成和执行环境注入"
```

### Task 5: Wire Secret Commands Into CLI With TDD And No Password Leakage

**Files:**
- Modify: `internal/cli/app.go`
- Modify: `internal/cli/app_test.go`
- Modify: `internal/cli/target.go`
- Modify: `internal/audit/audit_test.go`
- Modify: `internal/executor/executor_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestSecretSetStoresPasswordWithoutPrintingIt(t *testing.T) {
    stdout := &bytes.Buffer{}
    stderr := &bytes.Buffer{}
    app := New(stdout, stderr, &fakeRunner{})
    app.SecretStore = &fakeSecretStore{}

    code := app.Run([]string{"secret", "set", "--host", "192.168.1.101", "--user", "appuser"})
    if code != 0 {
        t.Fatalf("expected success, stderr=%s", stderr.String())
    }
    if strings.Contains(stdout.String(), "1qa2ws") {
        t.Fatal("password leaked to stdout")
    }
}

func TestShellUsesStoredSecretForInventoryIPAddressKey(t *testing.T) {
    // inventory contains [hosts."192.168.1.101"] with via/password
    // secret store returns value
    // expect executor env contains SSH_ASKPASS_REQUIRE=force
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/cli ./internal/audit ./internal/executor`
Expected: FAIL because `secret` subcommands and secret-aware shell/exec paths do not exist.

- [ ] **Step 3: Write minimal implementation**

Implement:

- `secret set/get/delete`
- host locator shared by secret commands
- secret lookup before `exec/shell/hosts test/diagnose`
- clear error when secret missing for `auth=password`

- [ ] **Step 4: Keep audit metadata only**

Ensure emitted `AuditEvent` records `auth=password` or `secret_source=keychain` if needed, but never password or raw secret output.

- [ ] **Step 5: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/cli ./internal/audit ./internal/executor`
Expected: PASS, with no password in stdout/stderr/audit fixtures.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/app.go internal/cli/app_test.go internal/cli/target.go internal/audit/audit_test.go internal/executor/executor_test.go
git commit -m "接入 keychain 凭据和 secret 命令"
```

### Task 6: Update Runtime Commands That Should Use Secrets And Preserve Existing Restrictions

**Files:**
- Modify: `internal/cli/app.go`
- Modify: `internal/cli/app_test.go`
- Modify: `internal/jobs/jobs.go`
- Modify: `internal/jobs/jobs_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestDiagnoseUsesStoredSecretForPasswordHost(t *testing.T) {
    // fake secret store returns password
    // diagnose should succeed and invoke executor with askpass env
}

func TestBackgroundProxyStillRejectsPasswordAuthEvenWithSecret(t *testing.T) {
    // expect exit code 2 and explicit rejection message
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test ./internal/cli ./internal/jobs`
Expected: FAIL because diagnose/jobs do not share the new secret-aware execution path.

- [ ] **Step 3: Write minimal implementation**

Refactor a small helper in `app.go` to build execution auth context once, then reuse it for:

- `hosts test`
- `diagnose`
- `exec`
- `shell`
- `job run`

Do not add secret support to background `tunnel/proxy`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `CGO_ENABLED=0 go test ./internal/cli ./internal/jobs`
Expected: PASS and existing background-password rejection tests remain green.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/app.go internal/cli/app_test.go internal/jobs/jobs.go internal/jobs/jobs_test.go
git commit -m "在诊断和任务执行中复用密码凭据上下文"
```

### Task 7: Refresh Skill Docs, Help Text, Defaults, And Installed Wrapper Verification

**Files:**
- Modify: `SKILL.md`
- Modify: `agents/openai.yaml`
- Modify: `scripts/install_skill.sh`
- Modify: `scripts/codex-ssh.sh`
- Modify: `defaults/hosts.toml`
- Modify: `defaults/config.toml`

- [ ] **Step 1: Write the failing verification expectations**

Record the expected help text and docs examples before editing:

```text
codex-ssh secret set --host 192.168.1.101 --user appuser
codex-ssh shell 192.168.1.101
codex-ssh exec 192.168.1.101 -- "docker pull ..."
```

- [ ] **Step 2: Update documentation and help**

Add:

- `secret` command examples
- `[hosts."192.168.1.101"]` inventory example
- “inventory key first, bare IP fallback” explanation
- Keychain caveat: macOS-only

- [ ] **Step 3: Reinstall the skill locally**

Run: `./scripts/install_skill.sh`
Expected: install completes without error and updates `~/.codex/skills/codex-ssh`.

- [ ] **Step 4: Verify installed wrapper output**

Run: `~/.codex/skills/codex-ssh/scripts/codex-ssh.sh --help`
Expected: help includes `secret set|get|delete` and IP-based examples in docs.

- [ ] **Step 5: Run full test suite**

Run: `CGO_ENABLED=0 go test ./...`
Expected: PASS across all packages.

- [ ] **Step 6: Commit**

```bash
git add SKILL.md agents/openai.yaml scripts/install_skill.sh scripts/codex-ssh.sh defaults/hosts.toml defaults/config.toml
git commit -m "更新密码凭据和裸 IP 使用文档"
```

### Task 8: Final Local Acceptance Checks For The Approved User Scenario

**Files:**
- Modify: `internal/cli/app_test.go`
- Modify: `docs/superpowers/specs/2026-04-03-ssh-skill-design.md`
- Modify: `docs/superpowers/plans/2026-04-03-ssh-password-secrets-and-ip-resolution.md`

- [ ] **Step 1: Add the end-to-end acceptance test fixture**

```go
func TestShellIPAddressKeyUsesViaAndSecretRef(t *testing.T) {
    // hosts:
    // [hosts."192.168.1.100"] jump
    // [hosts."192.168.1.101"] via jump, auth=password
    // expect shell 192.168.1.101 to resolve via + askpass automatically
}
```

- [ ] **Step 2: Run targeted verification**

Run: `CGO_ENABLED=0 go test ./internal/cli -run 'IPAddressKey|Secret' -v`
Expected: PASS for the exact user scenario.

- [ ] **Step 3: Capture manual verification commands in docs**

Update plan/spec notes with the exact commands to run manually after implementation (approved user scenario, execute in order):

```bash
codex-ssh secret set --host 192.168.1.101 --user appuser
codex-ssh diagnose 192.168.1.101
codex-ssh shell 192.168.1.101
codex-ssh exec 192.168.1.101 -- "docker pull ..."
```

- [ ] **Step 4: Run final full verification**

Run: `CGO_ENABLED=0 go test ./...`
Expected: PASS with no secret leakage in output.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/app_test.go docs/superpowers/specs/2026-04-03-ssh-skill-design.md docs/superpowers/plans/2026-04-03-ssh-password-secrets-and-ip-resolution.md
git commit -m "补齐密码凭据和 IP 直连验收覆盖"
```
