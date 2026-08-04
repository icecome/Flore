#!/usr/bin/env node
/**
 * sign-update.mjs — 为 update.json 中的每个资产计算 SHA256 并写入 Ed25519 签名。
 *
 * 用法：
 *   node scripts/sign-update.mjs <update.json> <private-key.pem> [-o output.json]
 *
 * 行为：
 *   - 读取 update.json，遍历 assets；
 *   - 对每个 asset 的第一个可下载 URL 拉取文件（或本地路径），计算 SHA256 摘要；
 *   - 用 Ed25519 私钥对摘要（32 字节原始字节）签名，base64 写入 asset.signature；
 *   - 同步修正 asset.sha256（与签名一致），输出到 -o 指定文件（默认覆盖原文件）。
 *
 * 安全提示：私钥文件（PKCS8 PEM）务必妥善保管，勿提交到仓库。
 * 私钥与内嵌在 apps/desktop/internal/updater/verify.go 中的公钥必须配对，
 * 否则桌面端更新会因签名校验失败而拒绝。
 */

import { createPrivateKey, createHash, sign } from 'crypto';
import { readFileSync, writeFileSync } from 'fs';
import { fileURLToPath } from 'url';
import path from 'path';
import https from 'https';
import http from 'http';

const argv = process.argv.slice(2);
if (argv.length < 2) {
  console.error('用法: node scripts/sign-update.mjs <update.json> <private-key.pem> [-o output.json]');
  process.exit(1);
}

const manifestPath = argv[0];
const keyPath = argv[1];
const outIdx = argv.indexOf('-o');
const outPath = outIdx >= 0 ? argv[outIdx + 1] : manifestPath;

const privateKey = createPrivateKey(readFileSync(keyPath));
const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));

function fetchBytes(url) {
  return new Promise((resolve, reject) => {
    if (url.startsWith('file://')) {
      resolve(readFileSync(new URL(url)));
      return;
    }
    const mod = url.startsWith('https:') ? https : http;
    mod.get(url, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        res.resume();
        resolve(fetchBytes(res.headers.location));
        return;
      }
      if (res.statusCode !== 200) {
        res.resume();
        reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        return;
      }
      const chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => resolve(Buffer.concat(chunks)));
    }).on('error', reject);
  });
}

let changed = false;
for (const asset of manifest.assets || []) {
  const urls = asset.urls || [];
  const fileUrl = urls.find((u) => u.startsWith('file://')) || urls[0];
  if (!fileUrl) {
    console.warn(`[warn] ${asset.fileName || '?'} 无 urls，跳过`);
    continue;
  }
  try {
    const data = await fetchBytes(fileUrl);
    const digest = createHash('sha256').update(data).digest();
    asset.sha256 = digest.toString('hex');
    asset.signature = sign(null, digest, privateKey).toString('base64');
    console.log(`[ok] ${asset.fileName} sha256=${asset.sha256.slice(0, 16)}…`);
    changed = true;
  } catch (err) {
    console.error(`[fail] ${asset.fileName}: ${err.message}`);
    process.exitCode = 1;
  }
}

if (changed) {
  writeFileSync(outPath, JSON.stringify(manifest, null, 2) + '\n');
  console.log(`已写入 ${outPath}`);
}

// 本文件仅被 scripts/ 引用，无独立入口时保持可被 node 直接执行。
void path;
void fileURLToPath;
