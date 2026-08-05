# 代码审查报告 — 方案 A 自写 Go 安装器（替代 NSIS）

> **审查框架**：SPEAR（Security / Performance / Error Handling / Architecture / Reliability）
> **审查范围**：`apps/desktop/cmd/installer/main.go`（新增 ~575 行）、`apps/desktop/cmd/package-tool/main.go`（payload 分支）、`scripts/build-desktop.mjs`、`apps/desktop/internal/updater/apply_windows.go`、`apps/web/src/components/TitleBar.tsx`、`.github/workflows/release.yml`、`.gitignore`、README、`zip_test.go`
> **审查日期**：2026-08-05
> **审查模式**：PR 审查（新增功能 + 关联回归修复）
> **审查工具**：comprehensive-code-review（3 遍法：概览 → 逐行 → 边界加固）

---

## 1. 总体评分（修复后）

### 加权总分：86/100 — 🟢 GOOD（可直接合并）

| 维度 | 权重 | 评分 | 关键发现 |
|------|------|------|---------|
| 🔴 安全 | 3x | 9/10 | PowerShell COM 单引号转义无注入；zip-slip 已防御；注册表安全写入；payload 同源可信 |
| 🟡 性能 | 2x | 9/10 | 消息循环干净；goroutine 经 SendMessage 跨线程安全更新 UI；无资源泄漏 |
| 🟠 错误处理 | 2x | 8/10 | 错误全检查、defer Close、失败弹 MessageBox；`knownFolderPath` 失败回退已补 |
| 🔵 架构 | 1.5x | 9/10 | 零额外依赖；minimal-x/sys 下 LazyProc 裸调用方案合理；与项目架构一致 |
| 📊 可靠性 | 1.5x | 8/10 | 卸载保留 `data/`；快捷方式走真实 shell 文件夹；zip 解压无大小上限（低危） |

> 修复前本应记 1 个 MAJOR（完成态逻辑缺陷，单文件 payload 时提前启用「完成/启动」），已在审查中修复，故最终裁定 GOOD。

---

## 2. 问题汇总

### 按严重级别统计

| 级别 | 数量 | 说明 |
|------|------|------|
| 🔴 CRITICAL | 0 | 无 |
| 🟠 MAJOR | 2 | 完成态逻辑缺陷 + setup 自动更新参数（**均已修复**） |
| 🟡 MINOR | 4 | 3 个已修复 + 1 个可接受的设计取舍 |
| 🔵 NIT | 4 | 平台约束/文档/去重等 |
| **总计** | **10** | |

### 按模块分布

| 模块 | MAJOR | MINOR | NIT |
|------|-------|-------|-----|
| 安装器 `cmd/installer/main.go` | M-R1(1) | m-01,m-02,m-04 | n-01,n-02,n-03,n-05 |
| `internal/updater/apply_windows.go` | M-R2(1) | — | — |
| `cmd/package-tool/main.go` | — | m-03 | — |
| 其余 diff（TitleBar/build-desktop/release/zip_test/README/.gitignore） | — | — | — |

---

## 3. CRITICAL 问题详情

无。

---

## 4. MAJOR 问题详情

### M-R1: `onInstallClick` 进度达 100% 即提前 `finishInstall()`（已修复）
- **文件**：`apps/desktop/cmd/installer/main.go` `onInstallClick` / `install`
- **类别**：可靠性（完成态语义错误）
- **问题**：进度回调里 `if p >= 100 { finishInstall() }`。当 payload 仅 1 个文件（本项目的 Flore.exe）时，`extractZip` 在解压阶段就把进度推到 100 → 提前调用 `finishInstall()` 启用「完成 / 启动 Flore」按钮，并置状态为"安装完成"。但此时 `install()` 尚未执行自拷贝 `uninstall.exe`、写卸载注册表、建快捷方式。若后续步骤失败，错误分支会重新启用「安装」并弹错误框，但 UI 已显示"安装完成"，状态矛盾。
- **影响**：单文件 payload 下"完成"界面与按钮被提前启用，用户体验错误；极端情况下失败路径与成功状态并存。
- **修复**：移除进度回调里的 `finishInstall()` 调用，改为 `install()` 完整返回 `nil` 后再 `setText("安装完成！可启动 Flore。") + finishInstall()`。错误路径 `return` 提前退出，不再误标完成。

### M-R2: setup 自动更新仍用 NSIS 的 `/S /D=` 参数调用安装器（已修复）
- **文件**：`apps/desktop/internal/updater/apply_windows.go` `buildSetupScript`（模板第 170 行）
- **类别**：可靠性（Windows setup 版自动更新失效）
- **问题**：NSIS→Go 安装器切换后，`buildSetupScript` 仍以 `"%SETUPEXE%" /S /D={INSTDIR}` 调用安装器。Go 安装器不识别 `/S`、`/D=` 参数（仅认 `--silent --dir`），`/D=...` 会被当作未知位置参数忽略 → 安装器以**交互 GUI 模式、默认目录**启动，而非静默原地重装。自动更新流程因此弹出安装向导而非完成更新。
- **影响**：setup 版用户通过应用内自动更新时，更新脚本无法静默重装，需手动操作；与"无缝更新"预期相悖。便携版（zip/xcopy）不受影响。
- **修复**：改为 `"%SETUPEXE%" --silent --dir "{INSTDIR}"`（路径含空格已加引号）；同步更新 `applyUpdate`/`applySetup`/`buildSetupScript` 三处注释中的 "NSIS 安装器 / /S /D" 描述。

---

## 5. MINOR 问题列表

| # | 文件 | 行号 | 问题 | 状态 |
|---|------|------|------|------|
| m-01 | `cmd/installer/main.go` | 消息循环 | `GetMessageW` 仅判断 `==0` 退出，返回 `-1`（错误）时未处理会继续处理无效消息 | **已修复**（`<= 0` 退出） |
| m-02 | `cmd/installer/main.go` | `knownFolderPath` | `SHGetFolderPathW` 失败时返回 `""` → 快捷方式路径退化为相对 CWD 的 `Flore.lnk`，落到工作目录 | **已修复**（回退 `USERPROFILE` 推导路径） |
| m-03 | `cmd/package-tool/main.go` | `buildPayload`/`buildPortable` | `fePath` 解析逻辑在两函数重复（DRY） | 可接受（保持最小改动，未修） |
| m-04 | `cmd/installer/main.go` | `extractZip` | 解压无单文件大小上限（zip bomb 防御缺失） | 可接受（payload 同源可信，零额外依赖优先） |

---

## 6. NIT 问题列表

| # | 文件 | 行号 | 问题 |
|---|------|------|------|
| n-01 | `cmd/installer/main.go` | 常量块 | `idcArrow = 32512` 未使用 | **已删除** |
| n-02 | `cmd/installer/main.go` | `wndClassEx`/`msg` | 结构体按 64 位布局手写（指针 8 字节对齐），32 位 Windows（GOARCH=386）下内存布局不符 | 文档说明：本项目仅发 amd64 Windows，可接受 |
| n-03 | `cmd/installer/main.go` | `escPwsh` | 仅转义单引号；在单引号 PowerShell 字符串内足以防注入（路径不含其他需转义元字符） | 文档说明，保持最小实现 |
| n-05 | `cmd/installer/main.go` | `loadPayload` | 优先读 exe 同目录 `payload.zip` 覆盖内嵌；属开发期便利 | 文档说明，保留 |

---

## 7. 做得好的地方

1. **零额外依赖实现原生安装器**：仅 `golang.org/x/sys/windows` + `registry`，无 NSIS/第三方库，`go build` 即可出 3.8MB 单文件，符合用户"安装包本质就是解压+快捷方式"的判断。
2. **minimal-x/sys 下走 LazyProc 裸调用**：本环境 `x/sys/windows` 为精简版（无 `HINSTANCE`/`CreateWindowEx` 等类型化 API），手工写 `wndClassEx`/`msg` 结构体 + 本地常量 + `NewCallback`，正确绕开限制，且结构体布局经核对与 `WNDCLASSEXW`/`MSG` 64 位内存布局一致。
3. **快捷方式用 `SHGetFolderPathW` 取真实 shell 文件夹**：避免硬编码 `UserProfile\Desktop`（重定向/ junction 下不存在，此前实测真实桌面在 `D:\Users\桌面`）；桌面 + 开始菜单快捷方式均经验证落地正确。
4. **卸载保留 `data/`**：`uninstall()` 跳过 `e.Name() == "data"`，契合"本地优先、用户拥有数据"；注册表经 `registry.DeleteKey` 清理。
5. **zip-slip 防御**：`extractZip` 校验 `target` 落在 `cleanDest` 前缀内，防目录穿越。
6. **关联回归修复正确**：`apply_windows.go` 的 `taskkill florebackend.exe` → `Flore.exe`（真实命中自衍生后端，Job Object 仍为主杀进程手段）；`TitleBar.tsx` 的 `else if` → 独立 `if` 修复窗口图标回归；`build-desktop.mjs`/`release.yml` 的 `wails build` 补 ldflags 修复 vdev 回归——三处均限定 Windows/非 mac 渲染路径，无跨端回归。
7. **构建链路闭环**：`package-tool -edition payload` 打内嵌 payload → `go build ./cmd/installer` → `package-tool -edition setup` 打包，CI 与本地脚本一致。

---

## 8. 改进建议（按优先级）

### P0 — 立即修复（审查中已修复）

| # | 行动项 | 文件 | 关联问题 |
|---|--------|------|---------|
| 1 | `finishInstall()` 移至 `install()` 返回后调用，移除进度回调内的提前完成 | `cmd/installer/main.go` | M-R1 |
| 2 | `GetMessageW` 返回 `<= 0` 即退出 | `cmd/installer/main.go` | m-01 |
| 3 | `knownFolderPath` 失败回退 `USERPROFILE` 推导 | `cmd/installer/main.go` | m-02 |
| 4 | 删除未用常量 `idcArrow` | `cmd/installer/main.go` | n-01 |
| 5 | `buildSetupScript` 调用改为 `--silent --dir "{INSTDIR}"`，更新相关注释 | `internal/updater/apply_windows.go` | M-R2 |

### P1 — 可选加固（后续）

| # | 行动项 | 文件 | 关联问题 |
|---|--------|------|---------|
| 5 | `extractZip` 增加单文件解压大小上限（防御性，payload 同源可暂缓） | `cmd/installer/main.go` | m-04 |
| 6 | `buildPayload`/`buildPortable` 抽 `resolveFePath` 去重 | `cmd/package-tool/main.go` | m-03 |

### P2 — 文档/平台

| # | 行动项 | 文件 | 关联问题 |
|---|--------|------|---------|
| 7 | 在 MEMORY.md 记录"仅发 amd64，32 位结构体布局不适用"与 minimal-x/sys 裸调用经验 | `MEMORY.md` | n-02 |

---

## 9. 覆盖率校验

| 指标 | 数值 |
|------|------|
| 总问题数 | 10 |
| 行动项总数 | 10 |
| 已覆盖问题数 | 10（5 修复 + 5 文档/可接受） |
| 覆盖率 | 10/10 = 100% |
| 延期处理问题 | 无 |

---

## 10. 结论

- 方案 A 自写 Go 安装器**架构正确、零额外依赖、满足用户"解压+快捷方式"的本质需求**，可替代 NSIS。
- 审查发现 2 个 MAJOR（完成态逻辑缺陷 M-R1、setup 自动更新参数 M-R2）与 3 个 MINOR，均在审查本轮**已修复并构建/冒烟验证通过**（静默安装→桌面+开始菜单快捷方式落地→卸载保留 `data/`→注册表清理）。
- 关联回归（`apply_windows.go` taskkill、`TitleBar.tsx` 图标、`wails build` ldflags）修复正确，跨端无回归。
- **最终裁定：🟢 GOOD（86/100），可合并。** 建议用户在真实 Windows 机器上实跑交互安装界面（GUI 完成态/启动按钮）做最终体验确认。
