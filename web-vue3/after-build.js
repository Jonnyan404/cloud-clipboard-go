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

// 迁移期间默认不覆盖后端已部署的 Vue2 static。
// 仅在显式设置 DEPLOY_STATIC=1 时，才拷贝到 server / server-node / cloud-clip。
if (process.env.DEPLOY_STATIC === '1') {
    for (const dest of ['../server/static', '../server-node/static', '../cloud-clip/lib/static']) {
        const target = fileURLToPath(new URL(dest, import.meta.url));
        rmSync(target, { recursive: true, force: true });
        copyDir(distDir, target);
    }
    console.log('after-build: gz/br generated and copied to server/node/cloud-clip statics.');
} else {
    console.log('after-build: gz/br generated in dist/. Set DEPLOY_STATIC=1 to copy into backend statics.');
}
