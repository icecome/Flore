// 版本同步脚本：将单一真相源（根 package.json 的 version，或命令行传入）写入：
//   1) ./version.go            —— Wails 桌面壳版本，供 updater 比较
//   2) ./wails.json productVersion —— Windows exe 文件属性
// 避免 productVersion 滞留旧值（曾因此发布过版本号错乱的包）。
//
// 用法：
//   node sync-version.mjs          读取 ../package.json 的 version
//   node sync-version.mjs 1.2.3   使用传入版本（CI tag / dispatch 场景）

import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const desktopDir = dirname(fileURLToPath(import.meta.url));

// 命令行参数优先，否则读取根 package.json
let version = process.argv[2]?.trim();
if (!version) {
  const rootPkg = JSON.parse(readFileSync(join(desktopDir, '..', '..', 'package.json'), 'utf8'));
  version = rootPkg.version;
}
if (!version) {
  console.error('sync-version: 无法解析版本号');
  process.exit(1);
}

// 1) version.go
writeFileSync(join(desktopDir, 'version.go'), `package main\n\nvar version = "${version}"\n`);
console.log(`sync-version: version.go -> ${version}`);

// 2) wails.json 的 productVersion（正则替换，保留其余格式与缩进）
const wailsPath = join(desktopDir, 'wails.json');
const wailsRaw = readFileSync(wailsPath, 'utf8');
const updated = wailsRaw.replace(
  /("productVersion":\s*)"[^"]*"/,
  `$1"${version}"`
);
if (updated === wailsRaw) {
  console.warn('sync-version: 未在 wails.json 中找到 productVersion，跳过');
} else {
  writeFileSync(wailsPath, updated);
  console.log(`sync-version: wails.json productVersion -> ${version}`);
}
