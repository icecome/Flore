# Flore

一个 RSS 阅读器。

## 关于

Flore 是一个使用 Vibe Coding 方式制作的 RSS 阅读器，提供桌面端安装包和便携包。不保证使用和维护，随缘更新。

## 安装

从 [Releases](../../releases) 下载最新版，解压即用，无需安装。

### 校验下载完整性

每个 Release 附带 `SHA256SUMS` 文件，可核对安装包是否与发布者上传的一致：

```bash
# 在 SHA256SUMS 所在目录执行
sha256sum -c SHA256SUMS
```

### 关于代码签名

安装包目前**未做代码签名**，首次打开时可能出现系统安全提示：

- **Windows**：SmartScreen 提示"Windows 保护了你的电脑"时，点击"更多信息"→"仍要运行"。
- **macOS**：Gatekeeper 拦截时，右键点击应用图标选择"打开"，或在终端执行
  `xattr -d com.apple.quarantine <应用路径>` 后再次打开。

我们优先保证安装包可通过上方 SHA256SUMS 校验完整性；正式证书签名将在预算到位后补齐。

## 开发

```bash
# 安装依赖
npm install

# 启动开发模式（Go 后端 + Web 前端）
npm run dev

# 构建桌面应用
npm run build:desktop
```

## 许可证

GNU General Public License v3.0