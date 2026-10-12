# USER_COMPLAINTS

**Audit date**: 2026-09-01

Five personas, simulated from evidence in the codebase. These are not strawmen — each complaint traces to a verified `file:line`. Complaint text is written in the register of the persona, not flattered.

---

## Persona 1 — 首次用户 (First-time user)

> **Context**: Downloads Flore, has no subscriptions, follows the onboarding screen.
> **Apparent severity**: would give up in the first 5 minutes.

> **★★★★☆ 4 stars — "差点就删了"**

**Complaint 1 — "我明明有 200 个订阅源，它却叫我'你还没有订阅任何源'"**

打开应用,等它加载。我有 200 多个源,同步得好好的。结果屏幕上写着 **"一款本地优先的 RSS 阅读器。你还没有订阅任何源,选一种方式开始吧。"**

我以为它把我数据弄丢了。差点去重新导入 OPML。

更糟的是右下角开始刷屏:**"获取订阅源列表失败"、"获取文件夹列表失败"、"获取文章列表失败"、"获取文章总数失败"、"获取稍后阅读数失败"、"获取收藏数失败"、"获取未读数失败"** —— 好几条互相顶掉,只能看见最后 5 条。

> **Evidence**: `App.tsx:909` gates the main area on `sources.length === 0` with no loading or error branch. Cold start waits up to 15s for the Go backend (`backend.go:74-79`), and `loadingSources` — the flag that exists specifically to cover this window — is destructured at `App.tsx:121` and **never rendered anywhere** (verified: sole occurrence). Seven independent catch blocks each call `showToast` (`useSourcesData.ts:39,51,74`; `useItemsData.ts:116,132,148,177`) against a `MAX_TOASTS = 5` cap.
> **Rating impact**: −2 stars, or uninstall.
> **Severity**: **P0**
> **Fix**: add a global `backendStatus: 'connecting' | 'ready' | 'offline'` state; render a skeleton/splash while connecting; suppress per-request toasts while status is not `ready`; show one persistent banner when offline.

**Complaint 2 — "第三张卡片点了没反应"**

首屏三个卡片:手动添加、导入 OPML、浏览推荐。

前两个能点。**第三个点下去什么都不发生。**

它长得跟另外两个一模一样——同样的白底、同样的圆角、同样的阴影、同样的图标底色。我以为应用卡了,又点了几下。

> **Evidence**: `OnboardingWelcome.tsx:75-81` renders the third card as a `<div>`, not a `<button>`, with no click handler — while the other two (`:53-62`, `:64-73`) are buttons with handlers. Visually identical styling.
> **Rating impact**: −1 star; makes the whole app feel unfinished.
> **Severity**: P1
> **Fix**: make it a real button that scrolls to / focuses the recommended list, or remove it and give the list its own heading.

**Complaint 3 — "我订阅了酷壳,然后它一直显示异常"**

用了推荐列表里的几个源。少��派、爱范儿挺正常。但**"酷壳 CoolShell"** 订阅完就显示异常,**"Hi-iD"** 也是。文章列表空的。

一个刚装的应用,推给我的第一批订阅源里有两个是死的。这什么品味?

> **Evidence**: `data/recommended-sources.ts:15-16` hardcodes 9 Chinese feeds with no health check. 酷壳 CoolShell has been defunct for years; Hi-iD updates sporadically. The list has no fallback and no way for the user to report a dead feed.
> **Rating impact**: −1 star.
> **Severity**: P2
> **Fix**: verify all 9 feeds are live before shipping, or add a "report broken feed" affordance. Longer term, fetch the list over the network.

**Complaint 4 — "我不知道点添加之后会发生什么"**

填了名称和 URL,点「添加」。**窗口关了。什么提示都没有。**

然后我等啊等,文章列表还是空的。过了大概半小时刷出来几篇。

对比一下:我导入 OPML 的时候,它会立刻告诉我"正在抓取最新文章…"。同一个应用,两条路线的反馈完全不一样。

> **Evidence**: `App.tsx:555-573` — `handleAddSource` awaits `createSource` then only `fetchSources()`; no toast, no fetch trigger. OPML import at `App.tsx:769-774` *does* toast and *does* trigger `handleFetch`. The asymmetry is unexplained.
> **Rating impact**: −1 star.
> **Severity**: P1
> **Fix**: toast + trigger fetch on manual add, matching the OPML path.

**Complaint 5 — "我填错间隔,按钮点了跟没点一样"**

我想把抓取间隔改成 1 分钟(我不知道有最小值)。点「添加」。**页面完全没反应,连红字都没有。**

我以为是应用死了。

> **Evidence**: `AddSourceModal.tsx:146-147` — `if (!trimmedUrl) return; if (form.interval < 5) return;` — silent early returns with **no error message, no field highlight**. The `<input>` has `min={5}` (`:107`) but a typed value of `1` still submits silently.
> **Rating impact**: −0.5 star.
> **Severity**: P1
> **Fix**: replace silent returns with inline field errors; surface the 5-minute minimum at the label, not only as an HTML `min` attribute.

---

## Persona 2 — 移动端用户 (Mobile user)

> **Context**: Reads on a phone in the web build or a narrow window.
> **Apparent severity**: basically unusable on a touch device.

> **★★★☆☆ 3 stars — "这不是给手机做的"**

**Complaint 1 — "文章列表里收藏、稍后读两个按钮,我怎么都点不到"**

手机上滑列表,每行右下角本来有"稍后阅读"和"收藏"两个小按钮。

**我完全看不见它们。** 长按弹右键菜单也不出来(手机浏览器没有右键)。

结果在手机上我没法快速收藏一篇文章,只能点进阅读器,在工具栏里找收藏按钮。一篇一篇点,太慢了。

> **Evidence**: `ArticleList.tsx:733` — `className="... opacity-0 transition-opacity ... group-hover:opacity-100 [&:has(.on)]:opacity-100"`. Pure CSS hover-gating; on a touch device these controls never become visible. No `focus-within` variant, no always-visible fallback at small widths.
> **Rating impact**: −1 star.
> **Severity**: **P1**
> **Fix**: add `focus-within:opacity-100` and make the row controls always visible below the 768px breakpoint (the app already knows `isMobile` in `App.tsx:156-168`).

**Complaint 2 — "设置面板在我手机上根本显示不全"**

点开设置。**一个 920 像素宽的面板直接糊在我 375 像素的屏幕上。** 右边一大截被切掉,标签看不见,开关点不到。

我只能横着把手机歪过来,或者干脆放弃设置。

> **Evidence**: `SettingsModal.tsx:574` uses fixed `920px` width. There is no responsive variant — `isMobile` (`App.tsx:156`) is used only for the three-column→stacked layout swap and the sidebar drawer; it is never passed into the settings stack. The modal is rendered *inside the Sidebar drawer* (`Sidebar.tsx:566-576`), compounding the problem.
> **Rating impact**: −1 star.
> **Severity**: **P1**
> **Fix**: `width: min(920px, 100vw)` plus a stacked single-column layout under 768px.

**Complaint 3 — "搜索框在抽屉里,搜出来的文章在抽屉后面"**

想搜东西。得:点左上角菜单 → 抽屉滑出来 → 点搜索框 → 输入 → 回车。

**然后抽屉还开着。** 结果页在它后面。我得先手动把抽屉关掉才能看到搜索结果。

每次搜索都要多按一次。而且 375 像素的抽屉挤着搜索框和整个设置入口,挤得要命。

> **Evidence**: `Sidebar.tsx:401-407` renders the search box; `App.tsx:138-140` owns `showMobileSidebar`; `handleSearch` (`App.tsx:208-214`) sets `isSearchMode` but does **not** close `showMobileSidebar` — unlike every other navigation handler in the file (`:859`, `:867`, `:873` all call `setShowMobileSidebar(false)`).
> **Rating impact**: −1 star.
> **Severity**: P1
> **Fix**: `setShowMobileSidebar(false)` inside `handleSearch`, matching the other nav handlers.

**Complaint 4 — "我横竖转一下手机,正在读的文章就没了"**

正在读一篇长文,想横过来看。转屏。

**啪,回到了文章列表。** 阅读进度没了,文章还得重新找。

> **Evidence**: `App.tsx:160-163` — on every resize where the viewport is no longer <768px, the effect force-resets `setMobileView('list')` and `setShowMobileSidebar(false)`. Orientation change crosses the 768px boundary, so the reader is dismissed mid-article.
> **Rating impact**: −1 star.
> **Severity**: P1
> **Fix**: only reset when transitioning *into* mobile, not on every non-mobile resize.

**Complaint 5 — "上面写着让我按 J / K,我按了没用"**

第一次打开,中间是空的,上面提示:**"从左侧列表挑选一篇文章,或按 J / K 上下浏览。"**

我按 J。按 K。**什么反应都没有。**

我又试着按上下箭头——那个能用。

所以这个提示在骗我。

> **Evidence**: `Reader.tsx:549` renders the copy. `App.tsx:364-412` binds `ArrowDown`/`ArrowUp`/`Enter`/`m`/`s`/`l`/`r`/`x`/`?`/`Escape` — **no `j` or `k` handler exists anywhere in the codebase** (verified by grep). The app contradicts itself across two screens.
> **Rating impact**: −1 star. This is the kind of small lie that makes people distrust the rest of the UI.
> **Severity**: **P1** (trivial to fix — bind two keys, or correct the copy)
> **Fix**: bind `j`/`k` in `handleKeyboardShortcut`. Note `ShortcutsHelpModal` and `SettingsShortcutsTab` list only the 9 real bindings, so the empty state is the sole liar.

---

## Persona 3 — 回流用户 (Returning user)

> **Context**: Subscribed months ago, comes back after weeks. Cares about data and keyboard speed.
> **Apparent severity**: loyal but increasingly annoyed; at risk of reverting to a hosted reader.

> **★★★★☆ 4 stars — "好用的地方还是好用,但越来越糙"**

**Complaint 1 — "所有提示长得一模一样,我分不清成功还是失败"**

备份成功了——右下角弹出个提示,紫色边。

备份**失败**了——右下角弹出个提示,**紫色边,一模一样的图标**。

我根本分不出来哪个是哪个。

> **Evidence**: `toast.ts:69` defines `showToast(message, type = 'info')`. `ToastContainer.tsx:6-10` has full success/error/Info icon-and-colour configs. **All 122 `showToast` call sites pass only a message** (verified by grep for a second argument: zero matches). The success and error branches are unreachable code. `App.tsx:299-310` shows the pattern: `await setItemStarred(...)` then, in the catch, `showToast('收藏失败')` — indistinguishable from the `showToast('已订阅「'+item.name+'」')` at `OnboardingWelcome.tsx:27`.
> **Rating impact**: −1 star. Structurally weakens every piece of error feedback in the app.
> **Severity**: **P0** (the fix is cheap and the payoff is app-wide)
> **Fix**: pass `'error'` in every catch-block toast and `'success'` on confirmed-success toasts. The rendering layer already exists and is correct.

**Complaint 2 — "一个源一直报错,我就只能把它删掉"**

有个源三个月了一直连接失败。我不想再看到它了,想**暂停**它。

右键菜单:抓取、编辑、重命名、移动、删除。**没有"暂停"。**

设置里改不了。数据库字段后端是有的(`active`),抓取调度也确实用它过滤——但**整个应用里没有一���能改它的地方。**

唯一的办法是删除。而删除会把这个源的文章历史一起删掉。

> **Evidence**: `Source.Active` (`models.go:20`) is honoured by `coordinator.go:165` (`WHERE active = true`) and `:225`, and `source_handlers.go:57` accepts it on update — but `EditSourceModal.tsx` exposes only `isPrivate` and `hideInTimeline`. The same story for `Source.Interval` (`models.go:19`): written by the client, never read by the scheduler. Backend complete, UI absent.
> **Rating impact**: −1 star. Pausing a noisy feed is table stakes for an RSS reader.
> **Severity**: **P1** (and it is ~50 lines of UI for work already done)
> **Fix**: add a 启用/暂停 toggle to `EditSourceModal` and a per-source interval field. Both endpoints already exist and are tested server-side.

**Complaint 3 — "点'全部标为已读'之前它不问我一句"**

我误点了一下**「全部标为已读」**。

**整个文件夹的文章瞬间全变已读了。** 没有确认框。没有撤销。

删订阅源会弹确认框(合理)。把整个文件夹清空也会(合理)。**把几十篇文章标记已读,不问。**

风险防护不对称到荒谬。

> **Evidence**: `App.tsx:669-677` — `handleMarkAllRead` calls `markAllRead(scope)` immediately with no confirmation state, while `handleDeleteSource` (`:649-666`), `handleDeleteFolder` (`:607-623`) and `handleClearFolder` (`:730-745`) all route through `setConfirmAction`. The list header button at `ArticleList.tsx:379-386` fires it directly.
> **Rating impact**: −1 star.
> **Severity**: P1
> **Fix**: route mark-all-read through `ConfirmDialog`, naming the affected scope ("确定将「技术」文件夹的 128 篇未读标记为已读？").

**Complaint 4 — "搜不到东西,它跟我说'没有更多文章了'"**

我搜一个冷门的词。零结果。

屏幕中央:**一个对勾图标,写着"没有更多文章了"。**

……我搜的是"没有更多",不是"没有更多文章"。这不是"读完了"的意思。

而且搜索只能出 50 条。我搜一个常见词,结果超过 50 条,**后面的悄悄没了**,也没告诉我。

> **Evidence**: `ArticleList.tsx:428-432` — one branch (`visibleItems.length === 0`) renders `CheckCheck` + "没有更多文章了" for **all four** cases: search-no-results, search-failed (`useItemsData.ts:204-207` sets `items=[]` on error), unread-cleared, and genuinely-empty-list. Search pagination is absent: `useItemsData.ts:197` hardcodes `limit=50` and `hasMore` is never set for the search path.
> **Rating impact**: −1 star.
> **Severity**: P1
> **Fix**: branch the empty state by `isSearchMode` and by whether the request succeeded; add search pagination.

**Complaint 5 — "导出的时候,窗口'啪'一下就没了"**

点「导出」。**窗口立刻关闭。** 屏幕上什么也没有。

我以为失败了,正准备重来一次。然后……大概 40 秒后,浏览器弹出了下载。

> **Evidence**: `ExportArticlesModal.tsx:138` closes the modal on submit with no start feedback, while the request may run up to 120s (`api.ts:363` `timeoutMs: 120000`). No toast, no progress, no cancellation.
> **Rating impact**: −0.5 star.
> **Severity**: P1
> **Fix**: keep the modal open with a determinate progress state, or toast "正在导出…" + a completion toast.

---

## Persona 4 — 管理员 / 高级用户 (Power user / self-hosting admin)

> **Context**: Runs a remote Miniflux instance, manages 300+ feeds, cares about sync and data safety.
> **Apparent severity**: productive, but distrusts the edges.

> **★★★★☆ 4 stars — "核心能用,但我不敢完全依赖它"**

**Complaint 1 — "文件同步整个是个空壳,它还收我的密钥"**

同步设置里有「WebDAV」和「S3 兼容存储」两个选项,还有一堆字段:Endpoint、Region、Bucket、Access Key、Secret Key、前缀。

我选了 WebDAV,填了坚果云地址和密码,点「测试连接」。**通过了。**

然后我去同步,点「立即同步」。**它开始拉 Miniflux 的数据了 —— 但 WebDAV 一个字节都没用上。**

我把 S3 那一堆字段填完,点「测试连接」:**"not supported yet"**。

所以:**UI 里有 S3 选项,后端根本没实现;WebDAV 只会发一个探测请求,从不真的读写数据。** 而它**照样收集我的 S3 Access Key 和 Secret Key 存进数据库**。

> **Evidence**: `SettingsSyncTab.tsx:23-26` offers `s3` as a selectable provider; `sync_engine.go:255-256` returns `fmt.Errorf("file provider \"s3\" not supported yet")`. The entire `file*` feature consists of `TestFileConnection` (`sync_engine.go:250-262`) which only performs a `PROPFIND` probe (`:266-295`) — **there is no code path that reads or writes any package data to WebDAV**. The `FileProvider` interface (`internal/sync/provider.go:173-185`, with `FetchPackage`/`StorePackage`) **has zero implementations**. Eleven config fields (`SettingsSyncTab.tsx:56-65`) collect S3 credentials that are stored but never used.
> **Rating impact**: −1 star; actively misleading.
> **Severity**: **P1** — not "cut it" but **"either implement it or delete it."** Shipping a credential form that collects keys for a feature that cannot work is worse than shipping nothing.
> **Fix**: remove the S3 option and the 6 S3-only fields immediately. Either implement `StorePackage`/`FetchPackage` against WebDAV, or reduce the section to a single honest line stating that file sync is not yet supported.

**Complaint 2 — "订阅源上的未读数字,和文章列表里的对不上"**

侧边栏里某个源显示 **23 篇未读**。我点进去。**列表里只有 18 条。**

我把那个源标记为"隐藏私密"之后,更离谱了:侧边栏数字**还是 23**,列表里**一篇都没有**。

> **Evidence**: `source_service.go:81-99` `computeUnreadCountMap` builds the sidebar badge from `SELECT sourceId, COUNT(*) FROM Item WHERE isRead=false` — with **no join to Source and no filter on `hideInTimeline` or `isPrivate`**. Meanwhile `GetUnreadCount` (`item_service.go:203-214`) applies `Source.hideInTimeline = false` and conditionally `Source.isPrivate = false`, and `CountItems` (`:217-227`) does the same via `applyCountScopeFilters`. Three code paths, different answers, for the same number.
> **Rating impact**: −1 star. A number that disagrees with the list it labels destroys trust in every other number.
> **Severity**: **P1**
> **Fix**: route all three through one shared query builder. This is the highest-value single fix in the backend — it is also the duplication this codebase has in the worst place.

**Complaint 3 — "同步要转 5 分钟的圈,我不敢关面板"**

点「立即同步」。按钮变成"同步中..."。然后就那样了。

我看了一下右上角同步状态接口,超时设的是 **300 秒**。

我不敢切走,不敢关设置面板,怕状态丢了。**5 分钟盯着一个不会动的按钮。**

> **Evidence**: `api.ts:479` `timeoutMs: 300000`; `SettingsSyncTab.tsx:542` renders only a button label. The only other signal is the sidebar spinner (`Sidebar.tsx:543-559`) — which requires the settings panel to be closed and the sidebar to be visible. There is no progress, no phase indicator, no partial result streaming, and no cancel.
> **Rating impact**: −0.5 star.
> **Severity**: P1
> **Fix**: the backend already tracks `sync_last_pull_at` / `sync_last_result` (`sync_engine.go:375-379`); surface per-phase progress and let the panel close safely.

**Complaint 4 — "点'测试连接'居然把我的配置存进去了"**

我想试一下新的 Endpoint。我填了一半,**故意没填完**,点「测试连接」。

然后我切回 API 那一栏——**我那半截配置已经存进数据库了。**

测试连接是个只读操作,不是吗?

> **Evidence**: `SettingsSyncTab.tsx:181-183` — `handleTest` calls `await save()` before probing. Side effect is undocumented and not indicated in the button's label or tooltip.
> **Rating impact**: −0.5 star.
> **Severity**: P2
> **Fix**: probe against in-memory form state; save only on explicit save.

---

## Persona 5 — 怀疑的客户 / 投资人 (Skeptical client or investor)

> **Context**: evaluating Flore for adoption, or deciding whether to fund/continue it.
> **Apparent severity**: the product looks more finished than it is; that gap is the problem.

> **★★☆☆☆ 2 stars — "现在还不能给我们用"**

**Complaint 1 — "你们最值钱的那段代码，一行测试都没有"**

我把仓库拉下来看。构建是好的：`go build` 过，`tsc -b` 过，`vite build` 过。测试在 HEAD 上也过——我专门建了个干净 worktree 验证。

但覆盖率是这样的：

```
internal/sync      67.4%
internal/logging   34.2%
internal/services  22.2%
internal/handlers   0%   ← 全部路由、CSRF、CORS、鉴权、限流
internal/database   0%   ← 5 个迁移 + FTS5 建表
internal/models     0%
backend             0%
pkg/paths           0%
apps/web            没有测试框架，也没有 ESLint
```

而 `urlpolicy.go` —— 那 258 行、实现了四层 SSRF 防护、还专门用一个 dial-context 解决 DNS rebinding TOCTOU 的文件 —— **零测试**。

这是整个项目里技术含量最高、最难重建的正确性证明。**你们对这个边界投入了真实的工程理解，然后没有给它任何保护网。**

顺便：工作区里有 4 个未提交的测试文件，其中一个连编译都过不去：

```
internal\services\item_service_test.go:116:2: declared and not used: normal
```

HEAD 是绿的，只因为那些文件还没提交。**谁把它们提交了，CI 立刻在第 2 步（`go vet`）红掉**，后面 6 步——包括前端类型检查和前端构建——全部不再执行。这不是"CI 坏了"，这是"CI 坏在下一步"，而下一步就是最正常的那个动作。

如果我把这个产品部署到 500 台机器上，我没有任何信心说它不会在某次重构里静默丢掉 SSRF 防护。

> **Evidence**: measured on a clean `git worktree` of HEAD `a268fbf` — `go vet`/`go test` exit 0, coverage as listed. Working tree: both exit 1 on `item_service_test.go:116`. `git status` shows that file plus `filter_test.go`, `folder_service_test.go`, `source_service_test.go`, `updateinfo_test.go` as `??` (untracked). `.github/workflows/ci.yml` runs `go vet ./...` as its second step. No test file exists for `urlpolicy.go`, `handlers`, `database`, `models`, `backend`, or `pkg/paths`.
> **Rating impact**: disqualifying for enterprise/production evaluation.
> **Severity**: **P0**
> **Fix**: one character at `item_service_test.go:116`, committed **before and separately from** the test files. Then add tests for `urlpolicy.go` before anyone touches the fetch path again.

**Complaint 2 — "你们这个'本地优先'产品，默认不备份"**

这一条比上一条更让我意外。

`backupAutoEnabled: false`（`settings.ts:217`）。调度器读到 false 就直接 return（`scheduler.go:186-188`）。

所以一个刚装好 Flore、订阅了 300 个源、认真读了三个星期的用户，如果磁盘坏了一块、或者数据库文件被误删、或者某次恢复中途断电——**他的全部订阅和全部阅读记录就没了。应用从头到尾没有提示过一次。**

而备份机制本身写得相当好：zip-slip 防护、zip-bomb 上限、WAL checkpoint、`VACUUM INTO` 快照、恢复失败还会回滚并重建连接。**你们花了功夫造一个保险箱，然后把它锁在柜子里没发给用户。**

> **Evidence**: `settings.ts:217` default `false`; `scheduler.go:186-188` early-return. `backupMaxKeep: 10` / `backupMaxDays: 30` / `backupAutoInterval: 1440` (`settings.ts:215-218`) are all already sensible — the only missing piece is the boolean.
> **Severity**: **P0**
> **Fix**: flip the default, and ideally run one backup on first launch rather than waiting a full interval.

**Complaint 3 — "你们 45 个设置项里,有一个是假的"**

设置 → 通知与驻留 → **「显示托盘图标」**。

我关掉它。重启应用。**托盘还在。**

我再打开、关掉、再重启。**还在。**

数据库里那个字段确实在变(`trayEnabled` 被写进去了),但桌面壳从不读它 —— `app.go:255` 无条件调用 `startTray()`。

一个**看得到、点得动、但什么都不会发生**的开关。

这不只是一个 bug 的问题。**它让我对整个设置面板的信任度归零了** —— 我没法知道其他 44 项里还有没有同样是摆设。

> **Evidence**: `SettingsNotifyTab.tsx:50-51` renders the toggle and writes `settings.trayEnabled`; `settings.ts:153,229` declares and defaults it. Grep for `trayEnabled` across `apps/desktop/*.go`: **zero hits**. `app.go:255` calls `a.startTray()` unconditionally. `window.go:69-70` reads only `minimizeBehavior`/`closeBehavior`.
> **Rating impact**: −1 star; undermines confidence in the whole product.
> **Severity**: **P1** (same class: `faviconMode` is *also* non-functional — `SourceAvatar.tsx:27-29` snapshots it with `useState(() => getCachedSettings())`, so changing it in settings has no effect until reload; and the server-stored `appTheme` is never re-applied at startup, so a dark preference does not travel to a second machine)
> **Fix**: either wire the shell to read it, or delete the toggle. Ship no visible control that does nothing.

**Complaint 4 — "你们的 README 说'本地优先、数据完全本地存储'"**

我信了。所以我看了下网络请求。

**站点图标默认从 Yandex 的第三方服务取。**(`favicon-proxy` → `SourceAvatar.tsx:44`)

一个宣称隐私优先的本地阅读器,把用户订阅的全部域名列表发给了 Yandex。

> **Evidence**: `api.ts:733-735` `getFaviconProxyBase()` → `/api/favicon-proxy`, which proxies to Yandex; consumed at `SourceAvatar.tsx:44`. Mitigating: `faviconMode` defaults to `'off'` (`settings.ts:240`) so it is opt-in. But `AGENTS.md`/`PROJECT.md` claim "数据完全本地存储" and "关注隐私和数据自主权", and `SettingsNetworkTab.tsx:77-84` offers "Yandex 图标" as a first-class choice with no explanation of what it discloses.
> **Rating impact**: −1 star for any privacy-conscious buyer; a due-diligence flag.
> **Severity**: P2 (mitigated by default-off, but the framing is wrong)
> **Fix**: keep default-off, label the option with exactly what it sends ("向 Yandex 发送所访问站点的域名"), or drop the third-party favicon service entirely and keep only `/favicon-direct`.

**Complaint 5 — "macOS 用户第一次打开会被系统拦住"**

release workflow 自己的注释里写着:

> **注意**:macOS 包未做 Apple 开发者签名/公证,下载后首次打开会被 Gatekeeper 拦截,用户需右键「打开」或 `xattr -d com.apple.quarantine <文件>` 放行。

一个桌面产品的分发前提是"下载 → 双击 → 能用"。你们的分发流程是"下载 → 右键 → 打开 → 相信我"。

> **Evidence**: `.github/workflows/release.yml` header comment, lines 16-19. Builds are darwin-arm64 only; no notarization step; no Developer ID signing.
> **Rating impact**: blocks adoption on macOS.
> **Severity**: P1 (operational, not code)
> **Fix**: Apple Developer ID + notarization is a prerequisite for any real macOS distribution. Until then, be explicit on the download page.

**Complaint 6 — "你们自己的审查文档说过要砍的东西,还在"**

仓库里有 `docs/planning/hai-razor-frontend-review.md`(362 行),里面逐条论证了 `/diagnostic/*` 三个端点(283 行)零调用、`trayEnabled` 不生效、`Source.Interval` 只写不读,建议删除或补线。

那份文档写得很清楚。**然后什么都没有发生。** 那三个端点还在。`trayEnabled` 还在设置面板里躺着。

这说明文档和执行之间没有闭环。而且这也解释了为什么会有三个不同的 settings key 各自以不同方式坏掉——**没有任何一个人被指派对这些声明负责。**

> **Evidence**: `docs/planning/hai-razor-frontend-review.md:120,286,416` recommend cutting the diagnostic subsystem; `handlers/reader.go:297-299` still registers all three routes with no caller. `SettingsNotifyTab.tsx:50-51` still exposes the dead toggle. `models.go:19` `Interval` still written-never-read. My own audit re-derived `trayEnabled` and `Interval` independently — which is itself the waste this complaint is about.
> **Rating impact**: signals that audit findings are not acted on — a governance concern, not a code one.
> **Severity**: P2 (process)
> **Fix**: bind every audit finding to an issue with an owner and a due date. An audit with no closure mechanism gets repeated and ignored.

---

## Cross-persona synthesis

| Complaint | Personas affected | Severity | Common thread |
|---|---|:--:|---|
| Cold start shows the first-run screen to returning users | 1, 5 | **P0** | No global connection state |
| All toasts look identical | 3, 4 | **P0** | Success/error indistinguishable |
| SSRF boundary untested; CI breaks on next test commit | 5 | **P0** | No safety net |
| Auto-backup ships OFF | 5 | **P0** | Data-sovereignty product with no protection |
| Pause-source / per-source interval missing | 3, 5 | P1 | Backend built, UI absent |
| Mobile hover-only controls unreachable | 2 | P1 | Desktop-first, mobile bolted on |
| Settings panel overflows mobile | 2 | P1 | Fixed 920px |
| Search empty/failure copy lies | 3 | P1 | Four cases, one string |
| J/K advertised but unbound | 2 | P1 | Product contradicts itself |
| Mark-all-read has no confirmation | 3 | P1 | Risk asymmetry vs delete |
| S3 selectable but unimplemented | 4 | P1 | Collects keys for nothing |
| Sidebar unread badge disagrees with list | 4 | P1 | Three divergent queries |
| `trayEnabled` / `faviconMode` / `appTheme` do nothing | 5 | P1 | Visible controls with no effect |
| macOS + Windows binaries unsigned | 5 | P1 | Distribution blocker |

### The one-line diagnosis

**Eleven of the fourteen top complaints share a single root cause: the app declares a capability — a state, a toggle, a shortcut, a chart, a backend feature — and the declaration outlives the implementation.** Every persona's worst moment is not a missing feature; it is a promise the code makes and does not keep.