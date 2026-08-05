#!/usr/bin/env node
// 跨平台桌面构建编排器：同一命令按当前运行环境构建对应平台的分发包。
//
// 替代原 Windows 专用的 npm 脚本（build:go / build:desktop 写死 .exe 与 -os windows，
// 且使用 %npm_package_version% 这种仅 Windows cmd 才展开的语法）。
//
// 流程：
//   1) sync-version.mjs 注入版本（version.go + wails.json productVersion）
//   2) wails build（原生构建当前平台：mac 产 Flore.app / win 产 Flore.exe / linux 产 Flore）
//      后端代码已编入 Flore 主程序（server/go/backend 包），不再单独构建 florebackend；
//      桌面壳以「自身二进制 --backend」自衍生子进程跑后端，避免被 Gatekeeper 拦截。
//   3) package-tool 打包（正确 -os / -feBin）
//
// 用法：
//   node scripts/build-desktop.mjs                 构建全部（默认）
//   node scripts/build-desktop.mjs --steps desktop  仅 wails build + package-tool

import { execFileSync } from 'node:child_process';
import { readFileSync, rmSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const rootDir = join(__dirname, '..'); // 仓库根
const desktopDir = join(rootDir, 'apps', 'desktop');
const binDir = join(desktopDir, 'build', 'bin');

// --- 步骤参数 ---
const args = process.argv.slice(2);
const stepsIdx = args.indexOf('--steps');
const stepsArg = stepsIdx !== -1 ? args[stepsIdx + 1] : 'all';
const runDesktop = stepsArg === 'all' || stepsArg === 'desktop';

// --- 版本（单一真相源：根 package.json）---
const pkg = JSON.parse(readFileSync(join(rootDir, 'package.json'), 'utf8'));
const version = pkg.version;
if (!version) {
  console.error('build-desktop: 无法读取根 package.json 的 version');
  process.exit(1);
}

// --- 平台映射（Node process -> Go/Wails 命名）---
const osMap = { win32: 'windows', darwin: 'darwin', linux: 'linux' };
const archMap = { x64: 'amd64', arm64: 'arm64' };
const targetOS = osMap[process.platform] || process.platform;
const targetArch = archMap[process.arch] || process.arch;
const exeExt = targetOS === 'windows' ? '.exe' : '';

// --- Go 环境（不依赖 shell profile，显式注入，确保本机/CI 一致）---
const goEnv = {
  ...process.env,
  GOPROXY: process.env.GOPROXY || 'https://goproxy.cn,direct',
  GOTOOLCHAIN: process.env.GOTOOLCHAIN || 'local',
};

// --- 工具 ---
function run(cmd, cmdArgs, opts = {}) {
  console.log(`\n[build-desktop] $ ${cmd} ${cmdArgs.join(' ')}`);
  execFileSync(cmd, cmdArgs, { stdio: 'inherit', ...opts });
}
function wailsBin() {
  const gp = (process.env.GOPATH || join(process.env.HOME || '~', 'go')).trim();
  return join(gp, 'bin', 'wails' + (process.platform === 'win32' ? '.exe' : ''));
}

console.log(`[build-desktop] target=${targetOS}/${targetArch} version=${version}`);

// --- 步骤 1-3：桌面壳 + 打包 ---
if (runDesktop) {
  // 1) 注入版本（version.go + wails.json）
  run('node', ['sync-version.mjs', version], { cwd: desktopDir });

  // 2) wails build（原生当前平台；mac 产 Flore.app，win 产 Flore.exe，linux 产 Flore）
  //    后端代码已编入 Flore 主程序，不再单独构建 florebackend。
  //    必须用 -ldflags 注入后端版本号（appVersion），否则 /api/version 恒为 "dev"；
  //    wails build 会追加自身 ldflags，此处为增量注入，不冲突。
  run(
    wailsBin(),
    ['build', '-ldflags', `-X github.com/rss/go-server/internal/handlers.appVersion=${version}`],
    { cwd: desktopDir, env: goEnv }
  );

  // 3) package-tool 打包
  const feBin =
    targetOS === 'windows'
      ? join(binDir, `Flore${exeExt}`)
      : targetOS === 'darwin'
        ? join(binDir, 'Flore.app')
        : join(binDir, 'Flore');
  run(
    'go',
    [
      'run', './cmd/package-tool',
      '-edition', 'portable',
      '-version', version,
      '-os', targetOS,
      '-arch', targetArch,
      '-feBin', feBin,
    ],
    { cwd: desktopDir, env: goEnv }
  );

}

console.log('[build-desktop] 完成');
