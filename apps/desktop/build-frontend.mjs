// 跨平台前端构建脚本，替代原 Windows 专用的 build-frontend.ps1。
// 在 ../web 执行 npm install / npm run build / npm run dev，
// 构建完成后将 ../web/dist 拷贝到 ./frontend/dist（Wails 前端资源目录）。
//
// 用法：
//   手动在 apps/desktop/ 执行：  node ./build-frontend.mjs build
//   由 wails.json 调用（Wails 从 frontend/ 目录执行，需上一级）：
//     node ../build-frontend.mjs install
//     node ../build-frontend.mjs build
//     node ../build-frontend.mjs dev

import { execSync } from 'node:child_process';
import { cpSync, rmSync, existsSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const desktopDir = dirname(fileURLToPath(import.meta.url));
const webDir = join(desktopDir, '..', 'web');
const distSource = join(webDir, 'dist');
const distTarget = join(desktopDir, 'frontend', 'dist');

const mode = process.argv[2] || 'build';

if (mode === 'install') {
  execSync('npm install', { cwd: webDir, stdio: 'inherit' });
} else if (mode === 'dev') {
  execSync('npm run dev', { cwd: webDir, stdio: 'inherit' });
} else {
  execSync('npm run build', { cwd: webDir, stdio: 'inherit' });
  if (existsSync(distTarget)) {
    rmSync(distTarget, { recursive: true, force: true });
  }
  mkdirSync(dirname(distTarget), { recursive: true });
  cpSync(distSource, distTarget, { recursive: true });
  console.log(`Copied web dist to ${distTarget}`);
}
