## 代码审查报告

> **审查框架：** SPEAR（Security / Performance / Error Handling / Architecture / Reliability）
> **审查范围：** 暂存区 27 个新文件（Go 6 + TS/React 8 + 配置 9 + 二进制 4）
> **审查日期：** 2026-08-02
> **审查模式：** 代码审计

---

## 目录

1. [总体评分](#1-总体评分)
2. [问题汇总](#2-问题汇总)
3. [CRITICAL 问题详情](#3-critical-问题详情)
4. [MAJOR 问题详情](#4-major-问题详情)
5. [MINOR 问题列表](#5-minor-问题列表)
6. [NIT 问题列表](#6-nit-问题列表)
7. [做得好的地方](#7-做得好的地方)
8. [改进建议（按优先级）](#8-改进建议按优先级)
9. [覆盖率校验](#9-覆盖率校验)

---

## 1. 总体评分

### 加权总分：91.5/100 — EXCELLENT

| 维度 | 权重 | 评分 | 关键发现 |
|------|------|------|---------|
| 安全 | 3x | 10/10 | 无注入、无敏感数据泄露、无路径遍历 |
| 性能 | 2x | 9/10 | useBackups.removeMany 顺序删除，可并行 |
| 错误处理 | 2x | 9/10 | 所有 async 操作有 try/catch，Go 错误检查完整 |
| 架构 | 1.5x | 8/10 | 文件职责清晰，少量可提取的公共模式 |
| 可靠性 | 1.5x | 9/10 | 空状态处理、边界值检查、migration 事务安全 |

**计算公式：** (10x3) + (9x2) + (9x2) + (8x1.5) + (9x1.5) = 30 + 18 + 18 + 12 + 13.5 = 91.5

---

## 2. 问题汇总

### 按严重级别统计

| 级别 | 数量 | 说明 |
|------|------|------|
| CRITICAL | 0 | 安全漏洞、数据丢失、功能完全错误 |
| MAJOR | 0 | Bug、逻辑错误、重大性能退化 |
| MINOR | 6 | 降低维护成本的改进 |
| NIT | 4 | 风格偏好、命名建议 |
| **总计** | **10** | 各级别之和 |

### 按模块分布

| 模块 | CRITICAL | MAJOR | MINOR | NIT |
|------|----------|-------|-------|-----|
| 桌面壳 Go (job_windows/other) | 0 | 0 | 1 | 1 |
| 后端迁移 (migrate_pubdate) | 0 | 0 | 1 | 1 |
| 前端组件 (BackupList/PolicyForm/Retention) | 0 | 0 | 2 | 1 |
| 前端组件 (EditSourceModal) | 0 | 0 | 1 | 1 |
| 前端 Hook (useBackups) | 0 | 0 | 1 | 1 |
| 前端工具 (apiBase) | 0 | 0 | 0 | 0 |
| 配置/模板 (Wails build/*) | 0 | 0 | 0 | 0 |
| 设计文档 | 0 | 0 | 0 | 0 |

---

## 3. CRITICAL 问题详情

无。

---

## 4. MAJOR 问题详情

无。

---

## 5. MINOR 问题列表

### m-01: job_windows.go 中 WaitForSingleObject 结果被丢弃

- **文件：** `apps/desktop/job_windows.go:113`
- **类别：** 可靠性
- **问题：** `windows.WaitForSingleObject` 的返回值（WAIT_OBJECT_0 / WAIT_TIMEOUT / WAIT_FAILED）被丢弃，如果超时或失败，调用方无从得知。
- **影响：** `waitForProcessExit` 在超时或失败时仍正常返回，调用方（RestartApp）可能在新实例启动时与旧实例冲突。
- **建议修复：** 检查返回值，超时时返回 error 或记录 warn 日志。

### m-02: migrate_pubdate.go 中解析失败的记录被静默跳过

- **文件：** `server/go/internal/database/migrate_pubdate.go:52`
- **类别：** 可靠性
- **问题：** 当 pubDate 无法解析时，`slog.Warn` 打印日志后 `continue`，跳过该记录。如果存在大量解析失败的记录，迁移完成后没有概要报告。
- **影响：** 运维人员可能不知道有记录被跳过，导致数据不一致。
- **建议修复：** 在函数末尾统计并输出 `skipped` 计数，与 `migrated` 一起记录。

### m-03: BackupPolicyForm / RetentionForm 中 Number(e.target.value) 空值风险

- **文件：** `apps/web/src/components/settings/BackupPolicyForm.tsx:71,90` 和 `RetentionForm.tsx:30,50`
- **类别：** 可靠性
- **问题：** 当 input 值为空字符串时，`Number("")` 返回 `0`，可能被写入设置。虽然浏览器 `min` 属性阻止了空值提交，但直接通过 JS 修改可绕过。
- **影响：** 设置可能被意外设为 0，导致备份策略异常（如保留 0 个备份）。
- **建议修复：** 在 `onChange` 中添加 `if (val === '') return` 或 `if (isNaN(val)) return` 保护。

### m-04: EditSourceModal 使用自定义 Toggle 而非项目已有组件

- **文件：** `apps/web/src/components/EditSourceModal.tsx:97-107,115-125`
- **类别：** 架构
- **问题：** 文件中的私密订阅和隐藏开关使用了手写 Toggle 实现（`<label><input type="checkbox"><span>...</span></label>`），而项目已有 `Toggle` 组件（`SettingsShared.tsx`）。
- **影响：** 两处 Toggle 实现可能产生视觉不一致，增加维护成本。
- **建议修复：** 导入并使用 `Toggle` 组件，统一 Toggle 的实现和样式。

### m-05: useBackups.removeMany 使用顺序删除而非并行

- **文件：** `apps/web/src/hooks/useBackups.ts:86-92`
- **类别：** 性能
- **问题：** `removeMany` 使用 `for...of` + `await` 顺序删除，每次删除需等待前一次完成。如果删除 10 个备份，总耗时 = 10 次网络往返。
- **影响：** 批量删除时用户体验慢，特别是网络延迟较高时。
- **建议修复：** 使用 `Promise.allSettled` 并行删除，同时保留失败计数逻辑。

### m-06: job_other.go 中 waitForProcessExit 使用忙轮询

- **文件：** `apps/desktop/job_other.go:27-41`
- **类别：** 性能（桌面环境）
- **问题：** `waitForProcessExit` 每 50ms 轮询一次进程状态，持续 `timeout` 时长。在非 Windows 平台无法使用 `WaitForSingleObject` 等效机制，但轮询间隔 50ms 偏短。
- **影响：** 在 `timeout` 较长时（如 30s），会产生 600 次系统调用，CPU 占用虽小但非必要。
- **建议修复：** 将轮询间隔增加到 200ms，或使用 `os.Process.Wait()`（阻塞等待，最优雅）。

---

## 6. NIT 问题列表

### n-01: migrate_pubdate.go 中双重 time.Parse 可简化

- **文件：** `server/go/internal/database/migrate_pubdate.go:46-50`
- **类别：** 风格
- **问题：** 先尝试 `time.RFC3339Nano`，失败后再试 `time.RFC3339`。由于 RFC3339Nano 是 RFC3339 的超集（支持分数秒），且 `time.Parse` 对无分数秒的字符串也能正确解析，单次 `time.RFC3339Nano` 即可覆盖两种格式。
- **建议：** 移除第二个 `time.Parse` 调用，单次 `time.Parse(time.RFC3339Nano, ...)` 即可。

### n-02: EditSourceModal 中部分缩进不一致

- **文件：** `apps/web/src/components/EditSourceModal.tsx:64-131`
- **类别：** 风格
- **问题：** 从第 64 行开始，缩进从 6 个空格变为 8 个空格，与文件开头（第 42-62 行）的 6 空格不一致。
- **建议：** 统一缩进为 6 个空格，与文件开头一致。

### n-03: useBackups 中 catch 块使用 console.error

- **文件：** `apps/web/src/hooks/useBackups.ts:47,61,75,91,107,122,135,151`
- **类别：** 风格
- **问题：** 所有 catch 块中既调用 `showToast` 给用户反馈，同时也调用 `console.error` 打印原始错误。虽然这是项目通用模式，但 `console.error` 在生产中不可见。
- **建议：** 考虑未来集成日志服务时替换 `console.error` 为统一日志接口。

### n-04: ThemeToggle 中三元表达式嵌套复杂

- **文件：** `apps/web/src/components/settings/ThemeToggle.tsx:21`
- **类别：** 可读性
- **问题：** 第 21 行 `isDark ? <Moon /> : current === 'system' ? <Monitor /> : <Sun />` 包含嵌套三元，阅读时需要多次解析。
- **建议：** 提取为独立函数或变量，如：
  ```tsx
  const icon = isDark ? <Moon /> : current === 'system' ? <Monitor /> : <Sun />;
  const label = isDark ? '深色模式' : current === 'system' ? '跟随系统' : '浅色模式';
  ```

---

## 7. 做得好的地方

1. **构建标签分离（Go）：** `dialog_other.go` / `dialog_windows.go` 和 `job_other.go` / `job_windows.go` 使用 `//go:build` 标签正确分离平台实现，非 Windows 平台退化清晰，Windows 平台利用 Job Object 防止孤儿进程。

2. **Job Object 正确使用：** `job_windows.go` 中的 `ensureJobObject` 使用 `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` + `JOB_OBJECT_LIMIT_BREAKAWAY_OK`，既防止孤儿进程，又允许 RestartApp 新实例脱离。

3. **迁移事务安全：** `migrate_pubdate.go` 使用分批次事务（`DB.Transaction`），每 100 条一批，避免大事务锁定数据库。

4. **前端错误处理完整：** `useBackups.ts` 中每个 async 操作都有 try/catch/finally，错误时 toast 提示用户，忙状态禁用按钮防止重复提交。

5. **空状态处理：** `BackupList.tsx` 在 `backups.length === 0` 时显示友好提示，而非空表格。

6. **apiBase 设计：** `apiBase.ts` 单独成文件避免循环依赖，注释清晰说明设计动机，桌面端/Web 端端口检测逻辑合理。

7. **.gitattributes 配置周全：** 统一 LF 换行符，Go/TS 源码指定 eol=lf，PS1 脚本保持 CRLF，二进制文件标记为 binary。

8. **Wails 构建资源与 .gitignore 协调：** `build/` 目录下资源文件通过 `!` 规则精确豁免，既避免构建产物入库，又确保必要的打包资源被跟踪。

---

## 8. 改进建议（按优先级）

### P2 — 架构改进

| # | 行动项 | 文件 | 关联问题 |
|---|--------|------|---------|
| 1 | 使用 `Toggle` 组件替代 EditSourceModal 中的手写 Toggle | `EditSourceModal.tsx` | m-04 |

### P3 — 渐进改进

| # | 行动项 | 文件 | 关联问题 |
|---|--------|------|---------|
| 2 | 提取 IconButton 公共组件，减少 BackupList 中按钮样式重复 | `BackupList.tsx` | 建议 |
| 3 | useBackups.removeMany 改用 Promise.allSettled 并行删除 | `useBackups.ts` | m-05 |
| 4 | 迁移完成后统计并输出 skipped 计数 | `migrate_pubdate.go` | m-02 |
| 5 | 为 input 空值添加保护（`if (val === '') return`） | `BackupPolicyForm.tsx`, `RetentionForm.tsx` | m-03 |
| 6 | job_other.go 中 waitForProcessExit 轮询间隔改为 200ms 或改用 Wait() | `job_other.go` | m-06 |
| 7 | job_windows.go 中检查 WaitForSingleObject 返回值 | `job_windows.go` | m-01 |
| 8 | 简化 migrate_pubdate.go 中双重 time.Parse | `migrate_pubdate.go` | n-01 |
| 9 | 统一 EditSourceModal 中缩进 | `EditSourceModal.tsx` | n-02 |

---

## 9. 覆盖率校验

| 指标 | 数值 |
|------|------|
| 总问题数 | 10 |
| 行动项总数 | 9 |
| 已覆盖问题数 | 7 |
| 覆盖率 | 7/10 = 70% |
| 延期处理问题 | n-03, n-04（console.error 和三元嵌套为项目通用模式，建议后续统一重构） |

> 校验公式：总问题数 = 已覆盖问题数 + 延期处理问题数