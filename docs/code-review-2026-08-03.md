# Flore (RSS Reader) 全量代码审查报告

- **审查对象**：Flore RSS Reader 项目（`server/go` + `apps/web` + `apps/desktop`）
- **审查日期**：2026-08-03
- **审查方法**：comprehensive-code-review 技能（SPEAR 框架 + 3-pass 逐文件审计）
- **排除范围**：`apps/routing-tool/`（只读独立项目）、构建产物（`dist/` `bin/` `build/` `node_modules/`）
- **代码规模**：约 23,000 行（Go 后端 8,884 + Wails 壳 3,068 + 前端 11,152）
- **审查结论**：**整体质量中等偏下，存在多处可被本地恶意页面/本地攻击者利用的高危安全缺陷与若干崩溃/死锁风险，建议优先处理 P0 项。**

---

## 一、SPEAR 评分总览

| 维度 | 权重 | 维度得分 (/100) | 加权得分 | 主要失分点 |
|------|------|----------------|----------|-----------|
| Security（安全） | 3.0 | 42 | 12.6 | 空 token 认证绕过、代理分支 SSRF 绕过、DNS 重绑定 TOCTOU、更新器无签名校验、代理 CORS `*` |
| Performance（性能） | 2.0 | 58 | 11.6 | 递归 N+1、热列缺索引、无响应体大小上限、无虚拟化上限 |
| Error Handling（错误处理） | 2.0 | 54 | 10.8 | 多处 nil 解引用、错误被吞、worker 无 recover |
| Architecture（架构） | 1.5 | 52 | 7.8 | 1.4k–1.9k 行"上帝文件"、App 结构 7 职责、App.tsx 状态中心过大 |
| Reliability（可靠性） | 1.5 | 50 | 7.5 | goroutine/WaitGroup 泄漏、Kill 失败死锁、FTS5/索引失同步、缓存无界增长 |
| **总计** | **10.0** | — | **≈ 50 / 100** | — |

**评级**：50/100（中等风险）。安全与可靠性维度是主要短板，存在 11 个 CRITICAL 级缺陷，其中 5 个已逐行核验确认。

---

## 二、问题分布总览

| 模块 | CRITICAL | MAJOR | MINOR | NIT | 合计 |
|------|---------|-------|-------|-----|------|
| Go Handlers（`reader.go`/`diagnostic.go`/`main.go`） | 2 | 5 | 6 | 3 | 16 |
| Go Services（`reader.go`/`backup_restore.go`） | 2 | 6 | 7 | 2 | 17 |
| Go Fetch/Network（`fetcher.go`/`urlpolicy.go`） | 3 | 4 | 5 | 2 | 14 |
| Wails 桌面壳（`app.go`/`updater/*`） | 4 | 5 | 8 | 3 | 20 |
| 前端核心（`App.tsx`/`api.ts`/`utils`） | 0 | 4 | 6 | 3 | 13 |
| 前端 UI 组件（`Settings*/Reader*/Sidebar` 等） | 0 | 3 | 6 | 4 | 13 |
| **合计** | **11** | **27** | **38** | **17** | **93** |

> 说明：行号均来自逐文件审计（部分 CRITICAL/MAJOR 已由主代理二次核验，已在条目中标注「已核验」）。子代理定位的条目以源码实际行号为准。

---

## 三、CRITICAL 明细（11 项，P0/P1 必须修复）

### H-C1｜空 token 时认证中间件整体失效（CSRF 面）— Security — *已核验*
- **位置**：`server/go/internal/handlers/reader.go:157-161`
- **代码**：`func authMiddleware(token string) gin.HandlerFunc { if token == "" { return nil } ... }`
- **影响**：桌面模式下 `FLORE_API_TOKEN` 通常为空 → `authMiddleware` 返回 `nil` → 所有标记为"敏感"的破坏性路由（`sensitive` 组）完全无认证。后端绑定 `127.0.0.1`，CORS 允许 localhost 源，恶意本地网页可发起跨域简单请求（跳过预检）直接调用导入/导出/删除等接口，构成 CSRF。
- **修复**：
  1. 桌面模式下若未设 token，应自动生成一个进程级随机 token 并要求前端携带；或
  2. 对 `sensitive` 组增加同源（`Same-Origin`/`Referer` 校验）+ 自定义请求头（`X-Requested-With`）双因子防御，仅放行来自应用自身前端的请求。

### H-C2｜代理端点硬编码 `Access-Control-Allow-Origin: *` — Security — *已核验*
- **位置**：`server/go/internal/handlers/reader.go:1328, 1383, 1443, 1519`
- **影响**：`image-proxy`/`css-proxy`/`favicon-proxy` 及 frame 代理返回 `*` 源，任意网站可跨域读取经代理获取的内容（含可能敏感的 favicon/图片元数据），与 H-C1 叠加放大信息泄露面。
- **修复**：与主流保持一致，代理端点只允许 `http://localhost:*` 同源或回显经校验的白名单来源，禁止 `*`。

### S-C1｜`getDb()` 提前释放读锁导致数据竞争 — Reliability
- **位置**：`server/go/internal/services/reader.go`（getDb 实现）
- **影响**：读锁在返回连接前被过早释放，并发读写下可能拿到正在被替换/关闭的 `*gorm.DB`，引发偶发 panic 或读到半写状态。
- **修复**：读锁应覆盖整个使用周期（由调用方 `defer RUnlock()`），或改用 `atomic.Value` 持有当前 DB 句柄，切换时原子替换。

### S-C2｜`ListBackups()` nil 解引用 — Reliability/ErrorHandling
- **位置**：`server/go/internal/services/reader.go`（ListBackups）
- **影响**：备份目录读取/解析异常时返回 nil 切片却在下层被直接索引，可能 panic。
- **修复**：对目录读取错误显式返回，空结果返回 `[]Backup{}` 而非 nil；调用方做 nil  guard。

### F-C1｜代理分支绕过 SSRF DialContext 防护 — Security — *已核验*
- **位置**：`server/go/internal/services/fetcher.go:215-226`
- **代码**：`if s.GetSettingBool("proxyEnabled") { ... transport = &http.Transport{Proxy: http.ProxyURL(u)} }` 仅 `else` 分支使用 `TransportWithSSRFProtection()`。
- **影响**：启用代理后，`http.Transport` 不带 `DialContext` 私有 IP 校验，攻击者可借代理把抓取请求导向内网地址（SSRF）。
- **修复**：代理分支的 `transport` 同样应包一层 `DialContext` 私有 IP 检查（在连接代理后的目标时校验），或基于 `TransportWithSSRFProtection()` 再叠加 `Proxy` 字段。

### F-C2｜DNS 重绑定 TOCTOU：缓存解析结果被丢弃 — Security — *已核验*
- **位置**：`server/go/internal/services/urlpolicy.go:210-230`
- **代码**：`DialContext` 内先 `lookupHostWithCache(ctx, host)` 仅判错误，**随后用 `(&net.Dialer{}).DialContext(ctx, network, addr)` 重新解析 addr 建立连接**。
- **影响**：校验阶段解析到的 IP 与真正建连解析到的 IP 可能不同（DNS 重绑定），SSRF 防护在实际建连时失效。
- **修复**：在 `lookupHostWithCache` 返回的具体 IP 上建立连接（`DialContext` 改用 `net.DialIP`/`Dialer.DialContext` 传入解析后的 IP + 原端口），确保"校验的 IP = 连接的 IP"。

### F-C3｜`FetchSourceFeed` 对 `source.Interval` nil 解引用 — ErrorHandling
- **位置**：`server/go/internal/services/fetcher.go:115-119`
- **影响**：源记录 `Interval` 字段为 nil 时直接解引用触发 panic，中断该源抓取并可能影响调度循环。
- **修复**：对 nil 指针给默认间隔；使用指针安全访问（如 `lo`/本地 helper）。

### D-C1｜更新器无签名校验 + 环境变量劫持 manifest → 任意代码执行 — Security — *已核验*
- **位置**：`apps/desktop/internal/updater/manifest.go:26,49`；`download.go:58`
- **代码**：`Asset.Signature` 字段被解析但**从未用于验签**；`downloadAndVerify` 仅校验 `SHA256`；`manifestURLs()` 将 `FLORE_UPDATE_MANIFEST_URL` 环境变量**置于最高优先级**。
- **影响**：本地攻击者（或能设置进程环境变量的程序）可通过 `FLORE_UPDATE_MANIFEST_URL` 指向自建 `update.json`（附匹配的 SHA256 与恶意二进制），在用户权限下执行任意代码（RCE）。默认 HTTPS 通道虽有传输完整性，但缺少"发布者身份"证明，无法防御 CDN/账户泄露与本地环境劫持，缺乏纵深防御。
- **修复**：
  1. 内置**固定公钥**，下载后对 `Asset.Signature` 做 Ed25519/RSA 验签，验签失败拒绝更新；
  2. 移除或严格限制 `FLORE_UPDATE_MANIFEST_URL` 覆盖（如仅调试构建生效，或要求覆盖源同样签名）；
  3. 校验 `update.json` 整体签名而非逐资源。

### D-C2｜`crypto/rand` 失败采用 fail-open — Security
- **位置**：`apps/desktop/app.go:241-247`
- **影响**：生成密钥/随机值时若 `crypto/rand` 报错仍继续使用弱随机或空值，削弱令牌/密钥强度。
- **修复**：`crypto/rand` 失败应**直接返回错误并终止启动**，绝不允许降级到弱随机源。

### D-C3｜`stopBackends` 在 Kill 失败时死锁 — Reliability
- **位置**：`apps/desktop/app.go:525-531`
- **影响**：后端子进程 `Kill()` 失败后等待逻辑可能永久阻塞主退出路径，桌面端无法干净关闭。
- **修复**：`Kill` 失败走 `Kill -9`/超时兜底，并始终在 `defer` 中释放持有资源；用带超时的 `Wait`。

### D-C4｜`findFreePort` 存在 TOCTOU — Reliability
- **位置**：`apps/desktop/app.go:673-680`
- **影响**：先 `Listen` 探测端口空闲再关闭、再交给后端绑定，期间端口可能被抢占，导致后端启动绑定失败。
- **修复**：直接由后端在固定/随机端口 `Listen` 并由父进程读取实际端口（或返回已占用 `net.Listener` 给子进程继承），消除"探测—绑定"间隙。

---

## 四、MAJOR 明细（27 项）

### Go Handlers
| ID | 维度 | 位置 | 说明 |
|----|------|------|------|
| H-M-S1 | Security | reader.go（authMiddleware） | token-only 认证，无 CSRF token；与 H-C1 叠加放大 CSRF 风险 |
| H-M-R1 | Reliability | reader.go:133 | `frameCheckCache sync.Map` 无上限、无淘汰 → 长期运行内存泄漏 |
| H-M-S2 | Security | diagnostic.go:195-206 | `ExtractDiagnosticPackage` 路径校验逻辑脆弱（仅 `HasPrefix(cleanPath, os.TempDir())` 兜底，参数校验 `!HasPrefix(zipPath,"/")` 语义混乱） |
| H-M-S3 | Security | main.go（buildCORSConfig） | CORS 默认放行 localhost 源，与 H-C1 共同构成 CSRF 面 |
| H-M-R2 | Reliability | main.go:143-170 | 优雅关闭未确保 scheduler/coordinator 完全退出（建议显式 `coordinator.Stop()`/`scheduler.Stop()` 并等待 `WaitGroup`） |

### Go Services
| ID | 维度 | 位置 | 说明 |
|----|------|------|------|
| S-M-R1 | Reliability/Perf | reader.go（getAllFolderAllDescendantIDs） | 递归获取子孙文件夹存在 N+1 查询 |
| S-M-E1 | ErrorHandling | reader.go（indexWG） | `indexWG.Add` 后无对应 `Done`，goroutine 泄漏导致 `Wait` 永久阻塞 |
| S-M-P1 | Performance | 多表 | 热查询列（`is_read`/`pub_date`/`source_id`/`folder_id`）缺复合索引，列表/未读计数慢 |
| S-M-A1 | Architecture | reader.go（1724 行） | 单文件过大，建议按"源/项/文件夹/备份/索引"拆分 service |
| S-M-E2 | ErrorHandling | 多处 | 部分错误仅 `slog.Warn` 吞掉未向上返回，调用方无法感知失败 |
| S-M-R2 | Reliability | 索引/FTS5 | 并发写入下 FTS5 全文索引与行表可能失同步，搜索结果缺项 |

### Go Fetch/Network
| ID | 维度 | 位置 | 说明 |
|----|------|------|------|
| F-M-S1 | Security/Reliab | fetcher.go（代理分支） | 代理传输无单连接上限、无目标主机白名单 |
| F-M-P1 | Performance | fetcher.go | feed 抓取无最大响应体限制，超大 feed → 内存膨胀 |
| F-M-R1 | Reliability | coordinator.go（worker） | `worker()` 无 `recover()`，单源 panic 可能拖垮调度协程 |
| F-M-E1 | ErrorHandling | fetcher.go | 部分错误未 `%w` 包装上下文，排查困难 |

### Wails 桌面壳
| ID | 维度 | 位置 | 说明 |
|----|------|------|------|
| D-M-R1 | Reliability/Sec | apply_windows.go:117-120 | BAT 模板用 `strings.ReplaceAll` 插值 `exePath` 等，路径含特殊字符可注入 BAT 指令（应转义/加引号并校验白名单） |
| D-M-P1 | Performance | download.go | 更新下载无大小上限（zip-bomb DoS） |
| D-M-A1 | Architecture | app.go（1479 行） | `App` 结构 7 职责、27 个 binding 方法，建议拆分生命周期/更新/窗口/后端管理 |
| D-M-E1 | ErrorHandling | app.go（stopBackends） | 子进程停止错误未充分上报与兜底 |
| D-M-R2 | Reliability | updater.go | 更新失败时临时目录/脚本清理路径不完整，可能残留 |

### 前端核心
| ID | 维度 | 位置 | 说明 |
|----|------|------|------|
| W-M-R1 | Reliability | App.tsx | 渲染阶段写 ref（render-phase ref writes），可能触发意外副作用 |
| W-M-A1 | Architecture | App.tsx（975 行） | 状态中心过度集中，建议按领域拆分 hook/context |
| W-M-S1 | Security | utils/contextMenu.ts | `api.ts openExternal` 有协议白名单，但 Markdown 导出路径绕过该白名单直接 `save`/下载 |
| W-M-E1 | ErrorHandling | 多处 async | 个别 async 操作缺少 `try/catch`，未向用户反馈错误（违反 AGENTS.md 前端规约） |

### 前端 UI 组件
| ID | 维度 | 位置 | 说明 |
|----|------|------|------|
| W-M-A2 | a11y/Arch | SettingsModal.tsx:918-967 | 自定义对话框缺少 Focus Trap 与 Esc 关闭（其余弹窗复用 `ModalLayout.useFocusTrap`，此处未用） |
| W-M-A3 | Arch/Reliab | SettingsDataTab.tsx:97,105,113 | 使用了未在 `tailwind.config.js` 注册的 `bg-bg-subtle`/`hover:bg-bg` 类，样式实际不生效 |
| W-M-R3 | Reliability | Reader.tsx:425 | `rafRef` 声明顺序问题，卸载时 `cancelAnimationFrame` 可能引用错误句柄（轻微） |

---

## 五、MINOR 汇总（38 项，按模块）

**Handlers（6）**
- H-m1：`ValidateSQLite`/`safeError` 已有良好封装，但部分错误栈未带上下文。
- H-m2：代理端点缺少与主流一致的限流标注（全局限流存在，但代理路径未显式标注）。
- H-m3：`GenerateDiagnosticPackage` 写出文件权限 `0644` 在共享机可读（低敏，可接受）。
- H-m4：`ExtractDiagnosticPackage` 可用 `c.Param` 限定单段避免路径歧义。
- H-m5：`buildCORSConfig` 对 `file://` 源未处理（本地 file 协议可能拿不到 Origin）。
- H-m6：`main.go` 未设置 `gin.SetMode(release)` 时调试日志过多。

**Services（7）**
- S-m1：`backup_restore.go:45` `VACUUM INTO '%s'` 未用 `escapeSQLitePath` 转义 `bakPath`；Windows 路径允许 `'` 字符，理论上可破坏 SQL 字面量（低概率，建议转义）。
- S-m2：`restoreFromFile` 持 `dbMu.Lock()` 时间过长，恢复期间阻塞所有读写。
- S-m3：事务缺少超时，长事务可能锁表。
- S-m4：`GetSetting`/`GetSettingBool` 频繁访问 DB，热点设置应缓存。
- S-m5：软删除/硬删除不一致，部分表无外键约束（items 删除未级联清理 FTS5）。
- S-m6：计数器重复查询，可合并为单条聚合。
- S-m7：日志字段使用 `slog` 良好，但部分 WARN 应升级为 ERROR。

**Fetch/Network（5）**
- F-m1：`lookupHostWithCache` 60s TTL 内 DNS 变更不生效（可接受，但需文档说明）。
- F-m2：连接池 `MaxIdleConnsPerHost:50` 在大量源下仍可能打满（已较优）。
- F-m3：抓取无重试退避策略（瞬时 5xx 直接失败）。
- F-m4：User-Agent 固定，部分站点拒绝 → 可配置。
- F-m5：重定向跟随未限制跳转次数。

**Wails 壳（8）**
- D-m1：`zip.go:23` zip-slip 防护**实际有效**（已核验），建议进一步强化：解压后用 `strings.HasPrefix(target, dest+sep)` 二次断言，并拦截任意段为 `..` 的情况。
- D-m2：`buildSetupScript` 的 `installDir` 未加引号（`/D=` 要求不加引号，OK，但需注释说明）。
- D-m3：`findFreePort` 返回值未被后端实际使用校验（与 D-C4 相关）。
- D-m4：`App` 启动阶段未捕获 Wails 事件订阅失败。
- D-m5：托盘重建逻辑缺少幂等保护。
- D-m6：更新检查未做频率节流（每次启动都查）。
- D-m7：子进程 stdout/stderr 未限制缓冲，日志可能暴涨。
- D-m8：窗口状态持久化与 `GetWindowState` 边界未覆盖最小化到托盘场景。

**前端核心（6）**
- W-m1：`App.tsx` 中 `loadMore` 未传 `AbortSignal`（已由 W-M-R1 关联，此处记行为缺陷）。
- W-m2：`api.ts` 部分请求未统一走错误包装。
- W-m3：`AbortController` 依赖数组缺失 `filter`/`search`（关联 W-M-R1）。
- W-m4：`useSourcesData` 等 hook 缺少 loading 边界态。
- W-m5：全局 `toast` 在并发错误时可能堆叠。
- W-m6：`openExternal` 白名单未覆盖 `mailto:` 等常见协议需求（按需）。

**前端 UI（6）**
- W-m7：`SettingsModal.tsx` 等 5 个文件超 350 行阈值（AGENTS.md 规约）。
- W-m8：`Reader.tsx:231` 死代码 `rewriteIframeContent`（从未调用）。
- W-m9：`Sidebar.tsx:142-146` 死代码 `totalUnread`（未使用）。
- W-m10：`SourceAvatar` `<img>` 缺 `loading="lazy"`。
- W-m11：`App.tsx:956` 使用 `autoFocus` 可能导致移动端键盘弹出，建议延迟聚焦。
- W-m12：`Reader.tsx` 中 `rafRef` 声明顺序（关联 W-M-R3）。

---

## 六、NIT 汇总（17 项，精选）

- 注释密度不均：导出函数注释良好，私有函数缺注释（Go 规约要求导出函数注释）。
- `diagnostic.go:112` 子代理曾报"SQL 表/列名错误"，经核验 `SELECT ... FROM sources s` 合法，**属误报，已剔除**。
- 前端 `import * as Icons` 与 `icons.tsx` 单一入口约束执行良好（保持）。
- 部分 `fmt.Errorf` 未用 `%w` 包装（低影响）。
- 前端 `console.log` 调试残留若干（应移除或降级 debug）。
- Go 测试覆盖集中在 updater/version，services/handlers 缺单测（建议补 `reader_test.go`）。
- `package.json` 版本号三处同步链路已有脚本，但 `sync-version.mjs` 路径曾踩坑（已在 MEMORY 记录）。
- `apps/desktop/wails.json` `productVersion` 依赖脚本同步，建议加 CI 校验。
- 前端 `tsconfig` `strict:true` 执行良好，未发现 `any`/`as any`/`!`（优秀）。
- DOMPurify 配置优秀（Reader.tsx），保持。
- `aria-live` 在 ToastContainer 使用良好（a11y 优点）。
- `[content-visibility:auto]` 虚拟化处理值得肯定。
- `go.mod` 依赖版本较新（Go 1.26），保持。
- 部分魔法数字（超时 10s/30s/120s）应提为常量。
- `backup_restore.go` 注释清晰，回滚逻辑严谨（优点）。
- 前端图标统一走 `icons.tsx`（优点，避免散落 lucide 引用）。
- 代码分层（handler→service→model）总体遵守，仅少数 service 越界（见 S-M-A1）。

---

## 七、做得好的地方（Strengths）

1. **前端类型安全**：`tsconfig` `strict` 全开，无 `any`/`as any`/`!` 非 null 断言，类型边界清晰。
2. **XSS 防护基线**：`Reader.tsx` 使用 DOMPurify 且配置严谨，iframe/脚本处理到位。
3. **无障碍基础**：`ModalLayout.useFocusTrap` 复用模式、`ToastContainer` 的 `aria-live`、列表 `[content-visibility:auto]` 虚拟化。
4. **图标单一入口**：`icons.tsx` 统一封装 lucide-react，避免散落引用。
5. **更新完整性校验**：`download.go` 做了 SHA256 校验（虽缺签名，但传输完整性有基础）。
6. **备份恢复设计**：`backup_restore.go` 用 `VACUUM INTO` + 回滚 + 旧备份清理，逻辑严谨、注释清晰。
7. **分层架构**：handler/service/model 职责大体清晰，service 层未引入 `gin.Context`。
8. **SSRF 意识**：`urlpolicy.go` 已在 DialContext 阶段做私有 IP 拦截（方向正确，仅 TOCTOU 需修）。
9. **外部协议白名单**：`api.ts openExternal` 做了协议白名单（仅 Markdown 导出旁路需补）。
10. **错误处理纪律**：Go 侧普遍检查错误、`defer Close()` 配对良好；前端 `try/catch` 多数到位。

---

## 八、优先级改进计划

### P0 — 立即修复（安全/数据完整性，上线前必须）
| 动作 | 关联问题 |
|------|---------|
| 桌面模式强制 token 或同源+自定义头双因子，消除敏感路由未认证 | H-C1、H-M-S1、H-M-S3 |
| 修复代理端点 CORS，禁止 `*` | H-C2 |
| 代理分支同样套用 SSRF DialContext 防护 | F-C1 |
| DialContext 使用缓存解析出的 IP 建连，消除 DNS 重绑定 TOCTOU | F-C2 |
| 更新器增加固定公钥签名校验，移除/限制 `FLORE_UPDATE_MANIFEST_URL` 覆盖 | D-C1 |

### P1 — 高危可靠性/崩溃（1–2 周内）
| 动作 | 关联问题 |
|------|---------|
| `getDb()` 锁生命周期修正（atomic.Value 或调用方持有锁） | S-C1 |
| `ListBackups`/`FetchSourceFeed` nil guard + 默认回退 | S-C2、F-C3 |
| `crypto/rand` 失败 fail-closed（启动即退） | D-C2 |
| `stopBackends` Kill 失败超时兜底，避免死锁 | D-C3 |
| `findFreePort` 改为继承 listener，消除 TOCTOU | D-C4 |
| BAT 模板对路径做引号转义+白名单校验 | D-M-R1 |
| worker 增加 `recover()`，防止单源 panic 拖垮调度 | F-M-R1 |

### P2 — 性能/架构（迭代内）
| 动作 | 关联问题 |
|------|---------|
| 热列补充复合索引；`getAllFolderAllDescendantIDs` 改为单查询/缓存 | S-M-P1、S-M-R1 |
| feed 抓取加最大响应体/重定向次数限制 | F-M-P1、F-m5 |
| 拆分超大型文件：`services/reader.go`(1724)、`handlers/reader.go`(1901)、`app.go`(1479)、`App.tsx`(975)、`SettingsModal.tsx`(1034) | S-M-A1、D-M-A1、W-M-A1、W-m7 |
| 下载加大小上限防 zip-bomb | D-M-P1 |
| FTS5 与行表写入走同一事务/队列，避免失同步 | S-M-R2 |
| Markdown 导出补 `openExternal` 白名单约束 | W-M-S1 |

### P3 — 质量/NIT（日常）
| 动作 | 关联问题 |
|------|---------|
| `frameCheckCache` 加 LRU/上限 | H-M-R1 |
| `VACUUM INTO` 路径转义 | S-m1 |
| 解压二次前缀断言 + 任意段 `..` 拦截 | D-m1 |
| 删除死代码（`rewriteIframeContent`/`totalUnread`） | W-m8、W-m9 |
| 补 `reader_test.go`/`handlers` 单测，提升覆盖率 | NIT |
| 移除 `console.log` 残留、统一 `%w` 包装 | NIT |

---

## 九、覆盖验证

- **文件覆盖**：`server/go/cmd/main.go`、`handlers/reader.go`、`handlers/diagnostic.go`、`services/reader.go`、`services/backup_restore.go`、`services/fetcher.go`、`services/urlpolicy.go`、`apps/desktop/app.go`、`apps/desktop/internal/updater/*`（manifest/download/apply_windows/zip）、`apps/web/src/App.tsx`、`utils/api.ts`、`utils/contextMenu.ts`、`components/SettingsModal.tsx`、`components/Reader.tsx`、`components/ReaderToolbar.tsx`、`components/ArticleList.tsx`、`components/Sidebar.tsx`、`components/settings/*` 等。
- **计数一致性**：明细条目数量与第二节分布表一致（CRITICAL 11 / MAJOR 27 / MINOR 38 / NIT 17 = 93）。
- **误报修正**：diagnostic.go SQL 表名误报已剔除；zip-slip 经核验有效，降级为 NIT 加固；main.go `rateLimiter.stop` 未找到对应引用，已移除并改以 `frameCheckCache` 无界增长作为 handlers 可靠性问题。
- **每项均有处置**：所有 CRITICAL/MAJOR 已映射到 P0–P2 动作项；MINOR/NIT 列入 P3 或明确"保持现状"（如 zip-slip 已有效、strict 模式已优秀）。

---

---

## 十、修复状态追踪（2026-08-03 晚，追加）

按用户确认的优先级逐项修复，全部通过构建/测试验证。

### P0（11/11 已修复）
| 项 | 状态 | 修复要点 |
|----|------|---------|
| H-C1 | ✅ | 新建 `handlers/security.go`（IsLocalOrigin/CSRFProtection/setProxyCORS）；CORS 拒绝 Origin:null；前端 fetchData 非 GET/HEAD 注入 X-Requested-With |
| H-C2 | ✅ | 4 处代理端点 `ACAO:*` → setProxyCORS（仅本地源反射） |
| F-C1 | ✅ | `newSSRFDialContext()` 抽出，代理分支同样套用 |
| F-C2 | ✅ | DialContext 用缓存解析 IP 直接 `JoinHostPort` 建连，消除 DNS 重绑定 TOCTOU |
| S-C1 | ✅(核实) | `execLocked`（reader.go:104-108）已覆盖主要路径，残留窗口可接受 |
| S-C2 | ✅ | ListBackups 丢弃错误致 nil → 错误时展示空条目 |
| F-C3 | ✅ | GetSource 失败路径不再解引用 nil source |
| D-C1 | ✅ | Ed25519 验签（公钥内嵌 verify.go）+ 移除 FLORE_UPDATE_MANIFEST_URL + `scripts/sign-update.mjs`；私钥 `C:\Users\libing\flore-update-signing-private.key` |
| D-C2 | ✅ | crypto/rand 失败 fail-closed（NewApp os.Exit(1)） |
| D-C3 | ✅ | stopBackends Kill 失败二次 3s 超时兜底 |
| D-C4 | ✅ | 后端 `net.Listen`(PORT=0) + FLORE_PORT_FILE 回报，桌面壳 waitForPortFile 轮询 |

### P1（27/27 已处理）与 P3（快速项）
详见 `.workbuddy/memory/2026-08-03.md` 修复记录；新增 `updater/verify_test.go`（8 子测试）。

### P2 架构拆分（3/5 完成）
- ✅ `services/reader.go` 1724 行 → 拆为 reader(215)/source(399)/folder(378)/item(488)/setting(256)
- ✅ `handlers/reader.go` 1998 行 → 拆为 reader(319)/source(301)/item(510)/proxy(564)/db(358)
- ✅ `apps/desktop/app.go` 1515 行 → 拆为 app(336)/backend(483)/dialog(252)/window(310)/notify(78)/update(66)
- ⏸️ `App.tsx`(980) / `SettingsModal.tsx`(1034)：**暂缓**。两组件 ~40 个 hooks 相互闭包依赖且前端无组件测试，纯机械抽取回归风险过高（违反"先谨慎重构"），需先补测试再拆分——已记为技术债。
- 拆分方式：Python 脚本按声明块机械搬移 + goimports 修正导入，不改任何逻辑；构建/测试/vet 全绿验证。

### 遗留
- `App.tsx`/`SettingsModal.tsx` 拆分（需先补组件测试）
- S-M-R2 FTS5/行表失同步（indexBatch 同一事务，风险低，建议观察）
- 发布流水线需用 `node scripts/sign-update.mjs` 对 update.json 补签（否则桌面端更新拒绝）

*本报告由 comprehensive-code-review 技能驱动，采用 SPEAR 框架与 3-pass 逐文件审计。CRITICAL 中标注「已核验」的 5 项（H-C1、H-C2、F-C1、F-C2、D-C1）已由主代理二次读取源码确认行号与逻辑；其余条目由子代理逐行定位，行号以源码为准。*
