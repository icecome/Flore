#!/usr/bin/env node
// scripts/fix-release-order.mjs
// 重排 GitHub Release 的 assets 顺序（按平台分组，mac 在前）。
// 不重新打包二进制：复用 release 已上传的同一份文件（SHA256 不变）。
//
// 前置: 本机已 `gh auth login`，且对 icecome/Flore 有 release 写权限
//
// 用法:
//   node scripts/fix-release-order.mjs --tag v0.1.0-20260805
//   node scripts/fix-release-order.mjs --tag v0.1.0-20260805 --from-local
//   node scripts/fix-release-order.mjs --tag v0.1.0-20260805 --version 0.1.0-20260805

import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { exit } from 'node:process';

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--tag') args.tag = argv[++i];
    else if (a === '--from-local') args.fromLocal = true;
    else if (a === '--version') args.version = argv[++i];
    else if (a === '--help' || a === '-h') args.help = true;
  }
  return args;
}

function usage() {
  console.log(`用法: node scripts/fix-release-order.mjs --tag <tag> [--from-local] [--version <ver>]

选项:
  --tag <tag>        release tag（如 v0.1.0-20260805）必填
  --version <ver>    版本号（用于匹配资产文件名）；不传则从 tag 推断
  --from-local       从 apps/desktop/build/ 取本地文件（无网络下载）
  --help, -h         显示帮助

期望顺序（按平台分组，setup 在前）:
  1. flore-setup-darwin-arm64-<ver>.dmg
  2. flore-portable-darwin-arm64-<ver>.zip
  3. flore-setup-windows-amd64-<ver>.exe
  4. flore-portable-windows-amd64-<ver>.zip
  5. flore-setup_<debVer>-1_amd64.deb
  6. update.json

  注: deb 文件名符合 Debian 包名规范 <pkg>_<ver>-<rev>_<arch>.deb，
      包名 flore-setup（带 setup 标识，与 release 列表的 setup/portable 分组对齐）。
`);
}

function sh(cmd, args, opts = {}) {
  try {
    return execFileSync(cmd, args, {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
      ...opts,
    });
  } catch (e) {
    const stderr = (e.stderr || e.message || '').toString().trim();
    console.error(`命令失败: ${cmd} ${args.join(' ')}\n${stderr}`);
    throw e;
  }
}

function ghJson(args) {
  return JSON.parse(sh('gh', args));
}

function desiredOrder(version) {
  const debVer = version.replaceAll('-', '.');
  return [
    `flore-setup-darwin-arm64-${version}.dmg`,
    `flore-portable-darwin-arm64-${version}.zip`,
    `flore-setup-windows-amd64-${version}.exe`,
    `flore-portable-windows-amd64-${version}.zip`,
    `flore-setup_${debVer}-1_amd64.deb`,
    `update.json`,
  ];
}

// 从 git remote 解析 owner/repo（避免硬编码）
function gitRepo() {
  try {
    const url = sh('git', ['remote', 'get-url', 'origin']).trim();
    // ssh://git@ssh.github.com:443/icecome/Flore.git 或 https://github.com/icecome/Flore.git
    const m = url.match(/[:/]([\w.-]+)\/([\w.-]+?)(?:\.git)?$/);
    if (!m) throw new Error(`无法解析 git remote: ${url}`);
    return `${m[1]}/${m[2]}`;
  } catch (e) {
    console.error(`错误: 无法从 git remote 解析 owner/repo: ${e.message}`);
    exit(1);
  }
}

const args = parseArgs(process.argv.slice(2));
if (args.help) {
  usage();
  exit(0);
}
if (!args.tag) {
  usage();
  exit(1);
}

const version = args.version || args.tag.replace(/^v/, '');
const order = desiredOrder(version);

console.log(`>>> Release: ${args.tag}  版本: ${version}`);
console.log(`>>> 期望顺序:`);
order.forEach((n, i) => console.log(`    ${i + 1}. ${n}`));

// 校验 gh 认证
try {
  sh('gh', ['auth', 'status']);
} catch {
  console.error('错误: gh 未认证，请先执行 gh auth login');
  exit(1);
}

// 获取当前 release assets
// 用 gh api 调 REST API（避免 gh release view --json 在某些版本下把 id 字段映射为 node_id 字符串
// 导致 delete-asset 报 "not found"）
console.log(`>>> 读取 release 现有 assets ...`);
const repo = gitRepo();
const release = ghJson(['api', `repos/${repo}/releases/tags/${args.tag}`]);
const existing = (release.assets || []).map((a) => ({
  id: a.id,
  name: a.name,
  nodeId: a.node_id,
}));
console.log(`    共 ${existing.length} 个 assets:`);
for (const a of existing)
  console.log(`    - ${a.name} (id=${a.id}, node_id=${a.nodeId})`);

// 校验期望 assets 都在
const missing = order.filter((n) => !existing.some((a) => a.name === n));
if (missing.length) {
  console.error(`错误: release 缺少以下期望资产: ${missing.join(', ')}`);
  exit(1);
}

// 准备本地文件路径
let tmpDir;
const fileSources = new Map();
if (args.fromLocal) {
  console.log(`>>> 使用本地 build 产物 (apps/desktop/build/) ...`);
  for (const name of order) {
    const local = join('apps/desktop/build', name);
    if (!existsSync(local)) {
      console.error(`错误: 本地缺失 ${local}`);
      exit(1);
    }
    fileSources.set(name, local);
  }
} else {
  tmpDir = mkdtempSync(join(tmpdir(), 'flore-fix-'));
  console.log(`>>> 从 release 下载到临时目录 ${tmpDir} ...`);
  for (const name of order) {
    sh('gh', ['release', 'download', args.tag, '--pattern', name, '--dir', tmpDir, '--clobber']);
    fileSources.set(name, join(tmpDir, name));
  }
}

// 删除现有 assets（用 REST API 数字 id；若已不存在则跳过，支持幂等重跑）
console.log(`>>> 删除现有 assets ...`);
for (const a of existing) {
  console.log(`    - ${a.name} (id=${a.id})`);
  try {
    sh('gh', ['api', '--method=DELETE', `repos/${repo}/releases/assets/${a.id}`]);
  } catch (e) {
    const stderr = (e.stderr || e.message || '').toString();
    if (stderr.includes('404') || /not found/i.test(stderr)) {
      console.log(`      (已不存在，跳过)`);
      continue;
    }
    throw e;
  }
}

// 按期望顺序串行上传
console.log(`>>> 按期望顺序上传 ...`);
for (const name of order) {
  const file = fileSources.get(name);
  console.log(`    + ${name}`);
  sh('gh', ['release', 'upload', args.tag, file]);
}

if (tmpDir) rmSync(tmpDir, { recursive: true, force: true });
console.log(`>>> 完成！release ${args.tag} assets 已按平台分组重排（mac 在前）`);