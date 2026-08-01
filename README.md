# Flore

Flore 是一个纯本地的 RSS 阅读器。你的订阅、全文正文和阅读记录只存在你自己的磁盘上——不上传、不注册、不联网也完整可用。

## 核心特性

- **纯本地**：数据仅存储在你的设备上，无任何云服务依赖
- **全文提取**：内置 Readability 全文提取，让 RSS 真正可读
- **FTS5 全文搜索**：SQLite FTS5 虚拟表实现快速全文检索
- **过滤规则**：支持全局/源级/文件夹级过滤规则
- **OPML 双向导入导出**：轻松迁移订阅源
- **代理支持**：自定义代理配置 + 连通性检测
- **防盗链**：内置 image-proxy 和 css-proxy
- **本地备份**：完整备份与恢复机制

## 当前限制（请知悉）

- 仅支持单设备使用，无多端同步功能
- 目前仅支持 Windows 平台
- 不支持需要登录的私有源（Basic Auth 开发中）
- 无自动更新机制

## 安装

从 [Releases](../../releases) 下载最新版，解压即用，无需安装。

## 开发

```bash
# 安装依赖
npm install

# 启动开发模式（Go 后端 + Web 前端）
npm run dev

# 构建桌面应用
npm run build:desktop
```

## 安全

- SSRF 防护：DialContext 逐 IP 校验 + 云元数据域名黑名单
- CORS 白名单默认仅限 localhost
- 可选 API Token 保护（恒定时间比较）
- 所有 SQL 查询使用参数化

## 许可证

GNU General Public License v3.0

## 安全报告

请通过 [SECURITY.md](SECURITY.md) 报告安全漏洞。
