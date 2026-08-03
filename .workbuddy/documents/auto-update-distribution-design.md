# Flore 自动更新分发设计（REL-06 前置：分发渠道选型）

**决策日期**：2026-08-02
**选型结论**：多镜像 manifest 思路保留；二进制主通道定为 **Cloudflare R2 免费档**（零 egress、无 20MB 限制、国内连通性优于 GitHub），GitHub Release 直链作兜底；COS/OSS 降级为「未来若需 guaranteed 中国可靠性再付费」的可选项。jsDelivr / jsdmirror 仍不可用（见 §0）。
**约束背景**：个人开发者、无收费计划、不使用付费业务。代码托管 GitHub（REL-05 CI 用 GitHub Actions），GitHub 在部分地区（含中国）访问不稳；代码不存放于国内 git。结论：**分发渠道与代码托管地解耦**——源码留 GitHub 作真源，编译产物经 R2 边缘投送，无需把代码搬到国内 git。
**框架约束**：当前 Wails v2.13 无内置更新器（v3 才有 `app.Updater` + Ed25519 签名），REL-06 需在 v2 上自写更新逻辑，不升级 v3。

---

## 0. 关键修正：jsDelivr / jsdmirror 不能做二进制通道（2026-08-02 评估）

用户提出用 `cdn.jsdmirror.com` / `cdn.jsdmirror.cn` 替代 jsDelivr，称国内更快更稳。**评估结论：国内速度属实，但不可用作更新二进制通道。**

**jsdmirror 是什么（已核实）**
- 真实存在的免费开源 CDN 镜像，完全兼容 jsDelivr API（仅替换域名，路径规则一致，`/gh/`、`/npm/` 等都支持）。
- 后端基于**腾讯云 EdgeOne + 多家 CDN**，国内覆盖电信/联通/移动/教育网，自称平均 30ms。国内速度优于 jsDelivr **属实**。
- 由**个人自费运营，明确"不承诺 SLA"**；内置腾讯云内容审核（运营方对 serving 内容有管控权）。

**为什么不能承载我们的更新二进制（两条硬伤 + 一条软伤）**
1. **单文件 20MB 上限（致命）**：jsDelivr 对 GitHub 来源文件硬性限制「单文件 >20MB 默认不支持」。Flore 便携版 ZIP 约 30–60MB，必然超限。jsdmirror 宣称"完全兼容 jsDelivr"，几乎必然继承同一限制。
2. **`/gh/` 只服务仓库文件树，不服务 Release 附件（致命）**：构建物作为 **GitHub Release 资产（附件）**上传，不在 git 文件树里；`cdn.jsdelivr.net/gh/user/repo@tag/file` 只能取仓库内文件。要让它可取须把二进制提交进仓库——既违反 AGENTS.md「二进制不入库」，又造成仓库膨胀。
3. **运营风险（软但关键）**：个人运营、无 SLA、内容可控。即便大小与路径没问题，也不应作为生产自动更新的唯一/主通道。

**结论**：jsdmirror 仅可作 manifest 这类极小文件的国内加速；更新二进制必须走别处。原计划「jsDelivr `gh/` 代理 Release 资产」假设不成立，已删除。

---

## 0.1 Cloudflare R2 评估（2026-08-02，用户提出）

用户因"无付费计划"倾向用 Cloudflare R2 对象存储作分发。**评估结论：R2 免费档是当前约束下的最优解，推荐作为二进制主通道。**

**R2 免费档（已核实官方定价）**
- 存储：**10 GB-month / 月** 免费（足够保留多版本 30–60MB 便携包）。
- 操作：Class A（写入）**100 万次/月** 免费；Class B（读取/下载）**1000 万次/月** 免费——对 hobby 项目绰绰有余。
- **Egress（出网流量）全免**：这是 R2 最大的优势，向海量用户分发二进制**不产生带宽费用**。
- **单对象上限 5 TiB**：30–60MB 便携 ZIP **毫无压力**，彻底解决 jsDelivr/jsdmirror 的 20MB 死穴。
- S3 兼容 API：CI 用 `wrangler` 或任意 S3 SDK（如 `aws-cli`/`rclone`）即可上传，工具链成熟。

**账号成本（关键澄清）**
- 启用 R2 **需在 Cloudflare 账号绑定支付方式**（Visa / MasterCard / PayPal，含国内 PayPal），但**免费额度内不扣费**——这点与用户"不付费业务"的诉求**不冲突**：绑卡 ≠ 付费。若担心，可用虚拟卡。
- 若用 `r2.dev` 默认子域（`https://pub-xxxx.r2.dev`）则**零域名成本**，但该子域**有速率限制、官方标注仅适合开发测试**，不适合作为生产更新主通道。
- 生产推荐挂**自定义域名**（需自备一个域名，约 $8–12/年，DNS 托管到 Cloudflare 并开启代理 → 边缘缓存 + 无限速）。域名算一次性小额支出，非"持续付费业务"；若连域名也不愿买，Alpha 阶段可先用 `r2.dev`，但需接受限速风险。

**中国连通性（已核实，客观结论）**
- Cloudflare 在中国大陆**无 ICP 许可**，其免费 CDN 边缘会就近落到香港/新加坡/日本等节点，从大陆**通常可达但非高速**（实测小文件首次加载约 1.5s）。
- 与 GitHub 对比：**GitHub Release 下载（`objects.githubusercontent.com`）在大陆通常被限速/阻断且跨运营商普遍**；R2 **整体可达性优于 GitHub**——即用户直觉"比 GitHub 连通性更好"**成立**。
- **弱点**：**中国移动（移动宽带/蜂窝）部分区域会出现 R2 域名不可达**；偶发按区域节点屏蔽；长期存在"一刀切"政策风险。因此 R2 **不应是唯一下载源**，必须保留 GitHub 等兜底镜像。
- 结论：R2 是"免费 + 大文件 + 国内基本可达"三者兼得的唯一现实方案；比 GitHub 稳，但非 100% 可靠，故仍走多镜像 manifest。

---

## 1. 架构（采用 R2 主通道）

```
GitHub (canonical, source of truth)
  └─ GitHub Actions release.yml
        ├─ 构建产物 (.exe/.zip) + SHA256SUMS  → GitHub Release (兜底下载源)
        ├─ 生成 update.json (manifest)         → 随 Release 发布 + 同步 R2
        └─ 同步产物+manifest → Cloudflare R2 桶 (国内主通道, 零 egress)
              └─ 经 Cloudflare 自定义域名代理 (边缘缓存, 提速)

客户端更新器 (Wails v2 自写):
  CheckUpdate → 拉 manifest (先 R2 自定义域名, 兜底 GitHub raw)
    → 比对版本 → 按 urls 顺序尝试下载直到成功
    → 校验 sha256 (+ Ed25519 签名)
    → go-update 替换 / 便携版解压覆盖 → 重启
    → 任意步骤失败 → 保留现有安装与数据 (回退)
```

## 2. Manifest 结构 (update.json)

```json
{
  "schemaVersion": 1,
  "app": "flore",
  "latest": "0.4.0",
  "minSupported": "0.1.0",
  "publishedAt": "2026-09-01T00:00:00Z",
  "notes": "Markdown 更新说明",
  "assets": [
    {
      "platform": "windows/amd64",
      "variant": "portable",
      "fileName": "flore-windows-amd64.zip",
      "size": 48234112,
      "sha256": "ab12...",
      "signature": "base64-ed25519...",
      "urls": [
        "https://cdn.flore.example.com/v0.4.0/flore-windows-amd64.zip",
        "https://github.com/<user>/flore/releases/download/v0.4.0/flore-windows-amd64.zip"
      ]
    }
  ]
}
```

- `minSupported`：低于此版本强制更新，防止过旧客户端无法解析新 manifest。
- `urls` 顺序即客户端尝试顺序；任一 URL 下载成功并校验即通过。
- 主 URL 指向 **R2 自定义域名**（零 egress、无大小限制、国内基本可达、边缘缓存提速）；兜底为 GitHub Release 直链（无大小限制但国内常不通）。
- 若 Alpha 暂未购域名，主 URL 可用 `https://pub-xxxx.r2.dev/...`，但需接受限速。

## 3. CI (release.yml) 关键步骤

1. 构建三平台产物（便携 ZIP + Windows NSIS 安装包）。
2. `sha256sum` 生成 `SHA256SUMS`。
3. 运行 `gen-manifest` 小工具（Go 脚本）输出 `update.json`，注入版本、各资产多 URL、notes。
4. `gh release create` 上传产物 + SHA256SUMS + update.json（GitHub 兜底源）。
5. **必要 step — 上传到 R2（推荐 AWS CLI，runner 预装）**：
   在 Cloudflare 控制台 R2 → Manage API tokens → Create API token（权限 Object Read & Write），得到 Access Key ID / Secret Access Key；把 `R2_ACCOUNT_ID`、`R2_ACCESS_KEY_ID`、`R2_SECRET_ACCESS_KEY`、`R2_BUCKET` 四个值存为 GitHub Actions Secrets。
   上传走 S3 兼容端点（aws-cli 在 GitHub ubuntu runner 默认预装，无需安装步骤；其他镜像可加 `pip install awscli` 或 `aws-actions/setup-aws-cli`）：
   ```yaml
   - name: Upload artifacts to R2
     env:
       AWS_ACCESS_KEY_ID: ${{ secrets.R2_ACCESS_KEY_ID }}
       AWS_SECRET_ACCESS_KEY: ${{ secrets.R2_SECRET_ACCESS_KEY }}
       AWS_ENDPOINT_URL_S3: https://${{ secrets.R2_ACCOUNT_ID }}.r2.cloudflarestorage.com
       AWS_DEFAULT_REGION: auto
     run: |
       VER=v${{ steps.meta.outputs.version }}
       for f in dist/*; do
         aws s3 cp "$f" "s3://${{ secrets.R2_BUCKET }}/$VER/$(basename "$f")" \
           --content-type "$(file -b --mime-type "$f")" --cache-control "public, max-age=3600"
       done
       # manifest 同时上传到稳定地址（客户端固定拉这个）
       aws s3 cp dist/update.json "s3://${{ secrets.R2_BUCKET }}/update.json" \
         --content-type application/json --cache-control "public, max-age=300"
   ```
   - 桶需开启公开访问（R2 设置 Allow Access 或绑定自定义域），否则客户端无法 HTTPS 下载。
   - 备选：`npx wrangler r2 object put <bucket>/$VER/flore.zip --file=dist/flore.zip`（用 `CLOUDFLARE_API_TOKEN` + `CLOUDFLARE_ACCOUNT_ID`，无需 S3 密钥，但冷启动较慢）。
   - 客户端下载用普通 `http.Get` 拉 R2 公网 URL 即可，**更新器无需引入 S3 SDK**——下载时流式校验 sha256。

## 4. 更新器模块 (Wails v2 自写, apps/desktop/internal/updater)

- `updater.go`：`CheckUpdate()` 拉 manifest（带超时与重试，先 R2 自定义域名后 GitHub raw 兜底读取 manifest 本身）→ 版本比较 → 返回可用更新。
- `download.go`：按 `urls` 顺序下载，流式校验 sha256；全部失败则报错并保留现状。
- `apply.go`：用 `github.com/inconshreveable/go-update` 替换可执行文件；便携版解压 ZIP 覆盖目录；安装版启动 NSIS 静默升级。
- `verify.go`：Ed25519 验签（公钥 `//go:embed updater-key.pub`）。若 manifest 中引入任何第三方镜像 URL，则**必须从首日启用签名**；若仅用 R2 + GitHub，Alpha 可先 sha256、Beta 补签名。
- 失败回退：下载/校验/替换任一失败 → 不触碰现有安装，提示手动下载，数据零丢失。
- 版本号注入：构建时 `wails build -ldflags "-X .../updater.Version=vX.Y.Z"`（REL-04 已含版本单一真源）。

## 5. 校验与安全

- 完整性：sha256 必选（防损坏/篡改字节）。
- 真实性：Ed25519 签名（公钥构建期嵌入，防伪造 manifest/资产）。含第三方镜像时首日启用。
- 隐私：manifest/资产为静态字节，无遥测，与项目 local-first 一致。
- 供应链：二进制走 R2（S3 兼容 API + CI 密钥上传，HTTPS 投送），客户端强制 sha256 校验，Beta 起 Ed25519 验签——供应链风险可控。

## 6. 阶段落地（R2 主通道）

- Alpha：manifest + **R2 主通道**（自定义域名优先，无域名先用 `r2.dev`）+ GitHub 兜底；sha256 校验；手动"检查更新"触发。
- Beta：启用 Ed25519 验签；后台定时检查；评估是否引入 Gitee 资产镜像作额外免费国内备线（仍受上限/不稳约束）。
- GA：全平台签名（REL-07）+ R2 主通道稳定 + 边缘缓存配置完善。

## 7. 待用户拍板的小决策

- Q1 触发方式：仅手动按钮，还是也做后台定时检查？（建议 Alpha 仅手动）
- Q2 签名：仅用 R2+GitHub 时 Alpha 先 sha256、Beta 补签名；若引入任何第三方镜像则首日启用。（建议按此）
- Q3 manifest 读取：双源（R2 优先 + GitHub raw 兜底）还是单源？（建议双源）
- Q4 域名与账号：是否接受「绑定支付方式（不扣费）+ 购一个域名（~$10/年）挂 R2 自定义域名」以获得生产级主通道？还是 Alpha 先用 `r2.dev` 零成本但限速？（**推荐前者，但若坚持零支出可后者**）
- Q5 免费国内备线：是否额外用 Gitee 仅作 Release 资产镜像（国内免费可达，但有上限/偶发不稳/第三方依赖）？（建议 Beta 再评估）
