import { readFileSync, writeFileSync, readdirSync, statSync, rmSync, copyFileSync, mkdirSync } from 'node:fs';
import { gzipSync, brotliCompressSync } from 'node:zlib';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const distDir = fileURLToPath(new URL('./dist', import.meta.url));

function compress(dir) {
    for (const name of readdirSync(dir)) {
        const full = join(dir, name);
        const s = statSync(full);
        if (s.isDirectory()) {
            compress(full);
        } else if (/\.(js|css|html|svg)$/.test(name)) {
            const buf = readFileSync(full);
            writeFileSync(full + '.gz', gzipSync(buf));
            writeFileSync(full + '.br', brotliCompressSync(buf));
        }
    }
}

function copyDir(src, dest) {
    mkdirSync(dest, { recursive: true });
    for (const name of readdirSync(src)) {
        const full = join(src, name);
        const target = join(dest, name);
        const s = statSync(full);
        if (s.isDirectory()) {
            copyDir(full, target);
        } else {
            copyFileSync(full, target);
        }
    }
}

compress(distDir);

// 默认只压缩 dist，不动后端产物。
// 显式设置 DEPLOY_STATIC=1 时才拷进 cloud-clip/lib/static —— 那是 Go 版唯一的内嵌静态目录
// （Vue2 时代的 server/ 与 server-node/ 两个产物目录已删，它们当年是被这套脚本覆盖的）。
if (process.env.DEPLOY_STATIC === '1') {
    const target = fileURLToPath(new URL('../cloud-clip/lib/static', import.meta.url));
    rmSync(target, { recursive: true, force: true });
    copyDir(distDir, target);
    console.log('after-build: gz/br generated and copied to cloud-clip/lib/static.');
} else {
    console.log('after-build: gz/br generated in dist/. Set DEPLOY_STATIC=1 to copy into cloud-clip/lib/static.');
}
