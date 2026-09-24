// 真浏览器端到端验证：Service Worker 生效后，从 SPA 点「定时任务」入口
// 必须落到服务端渲染的自动化管理页，而不是被 SW 导航兜底吞回 SPA 首页。
//
// 这就是用户报的那个现象（「界面不是定时任务的，回到了 SPA 首页」）。
// 只依赖 Node 18+ 的全局 WebSocket 与 fetch，不需要装任何包。
//
// 用法：
//   node tools/spa-entry-e2e.mjs http://127.0.0.1:9602
//
// 退出码 0 = 全部通过。

import { spawn } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const BASE = (process.argv[2] || 'http://127.0.0.1:9602').replace(/\/+$/, '');
const CHROME = process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
// ⚠️ 调试端口**必须避开**跑这个项目的那些端口（9501 开发、9599 冒烟、9600/9602 两个实例）。
// 撞上任何一个，`/json/version` 会拿到那份 SPA 的 index.html（200 + HTML），
// `r.json()` 抛异常、循环空转到超时，最后报出来的是一句莫名其妙的 `Invalid URL` ——
// 排查半天才发现是端口撞了。9700 起跳，离那些端口远远的。
const DEBUG_PORT = 9700 + Math.floor(Math.random() * 200);

let pass = 0, fail = 0;
const ok = (name, cond, extra) => {
  const line = `  ${cond ? 'ok  ' : 'FAIL'} ${name}` + (extra !== undefined ? ` -> ${extra}` : '');
  console.log(line);
  cond ? pass++ : fail++;
};

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ---- 启动一个干净的 Chrome（独立 user-data-dir，避免蹭到旧 SW 注册） ----
const profileDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ccg-e2e-'));
const chrome = spawn(CHROME, [
  '--headless=new',
  `--remote-debugging-port=${DEBUG_PORT}`,
  `--user-data-dir=${profileDir}`,
  '--no-first-run', '--no-default-browser-check', '--disable-gpu',
  '--disable-features=Translate', '--hide-scrollbars',
  '--window-size=1280,900',
  'about:blank',
], { stdio: 'ignore' });

const cleanup = () => {
  try { chrome.kill('SIGKILL'); } catch { /* 已退出 */ }
  try { fs.rmSync(profileDir, { recursive: true, force: true }); } catch { /* 忽略 */ }
};
process.on('exit', cleanup);

async function waitForDevtools() {
  for (let i = 0; i < 100; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${DEBUG_PORT}/json/version`);
      if (r.ok) return (await r.json()).webSocketDebuggerUrl;
    } catch { /* 还没起来 */ }
    await sleep(100);
  }
  throw new Error('Chrome 的 devtools 端口没起来');
}

// ---- 极简 CDP 客户端 ----
class CDP {
  constructor(url) {
    this.ws = new WebSocket(url);
    this.id = 0;
    this.pending = new Map();
    this.waiters = [];
    this.ready = new Promise((res, rej) => {
      this.ws.addEventListener('open', res, { once: true });
      this.ws.addEventListener('error', () => rej(new Error('CDP WebSocket 连接失败')), { once: true });
    });
    this.ws.addEventListener('message', (ev) => {
      const msg = JSON.parse(ev.data);
      if (process.env.CDP_DEBUG) {
        console.log('   <<', JSON.stringify(msg).slice(0, 220));
      }
      if (msg.id !== undefined) {
        const p = this.pending.get(msg.id);
        if (!p) return;
        this.pending.delete(msg.id);
        msg.error ? p.rej(new Error(`${JSON.stringify(msg.error)}`)) : p.res(msg.result);
        return;
      }
      for (const w of [...this.waiters]) {
        if (w.method === msg.method) { this.waiters.splice(this.waiters.indexOf(w), 1); w.res(msg.params); }
      }
    });
  }
  send(method, params = {}, sessionId) {
    const id = ++this.id;
    return new Promise((res, rej) => {
      this.pending.set(id, { res, rej });
      this.ws.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
      if (process.env.CDP_DEBUG) {
        console.log('   >>', method, sessionId ? '(session)' : '(browser)');
      }
      setTimeout(() => {
        if (this.pending.delete(id)) rej(new Error(`${method} 超时`));
      }, 30000);
    });
  }
  // 带重试的 send。
  //
  // 为什么要：`Page.enable` 偶发超时 —— 刚跑完一次重量级构建（vite 三分钟）时 Chrome
  // 的渲染进程起来得慢，devtools 端口已经应答、target 也建出来了，但那个 session 还没就绪。
  // 第一次遇到时我以为是脚本写错了，其实是时序抖动，重试一次就过。
  // 只用在启动阶段的几条命令上；有副作用的命令（Page.navigate）不能盲目重试。
  async sendRetry(method, params = {}, sessionId, attempts = 3) {
    let lastErr;
    for (let i = 0; i < attempts; i++) {
      try {
        return await this.send(method, params, sessionId);
      } catch (err) {
        lastErr = err;
        if (process.env.CDP_DEBUG) {
          console.log(`   !! ${method} 第 ${i + 1} 次失败：${err.message}`);
        }
        await new Promise((r) => setTimeout(r, 500));
      }
    }
    throw lastErr;
  }
  once(method, ms = 15000) {
    return new Promise((res, rej) => {
      const w = { method, res };
      this.waiters.push(w);
      setTimeout(() => {
        const i = this.waiters.indexOf(w);
        if (i >= 0) { this.waiters.splice(i, 1); rej(new Error(`等 ${method} 超时`)); }
      }, ms);
    });
  }
}

async function main() {
  const wsUrl = await waitForDevtools();
  const cdp = new CDP(wsUrl);
  await cdp.ready;

  const { targetId } = await cdp.send('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await cdp.send('Target.attachToTarget', { targetId, flatten: true });

  await cdp.sendRetry('Page.enable', {}, sessionId);
  await cdp.sendRetry('Runtime.enable', {}, sessionId);
  await cdp.sendRetry('Emulation.setDeviceMetricsOverride',
    { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sessionId);

  const evaluate = async (expression) => {
    const r = await cdp.send('Runtime.evaluate',
      { expression, awaitPromise: true, returnByValue: true }, sessionId);
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.text + ' :: ' +
      (r.exceptionDetails.exception?.description || ''));
    return r.result.value;
  };

  const navigate = async (url) => {
    const loaded = cdp.once('Page.loadEventFired');
    await cdp.send('Page.navigate', { url }, sessionId);
    await loaded;
  };

  // [1] SW 必须真的注册好并接管页面 —— 否则下面的点击测的是「没有 SW 的世界」，
  //     而那种情况下本来就是好的，测了等于没测。
  await navigate(`${BASE}/`);
  const swState = await evaluate(`(async () => {
    if (!('serviceWorker' in navigator)) return { supported: false };
    const reg = await navigator.serviceWorker.ready.catch(() => null);
    for (let i = 0; i < 100 && !navigator.serviceWorker.controller; i++) {
      await new Promise(r => setTimeout(r, 100));
    }
    return {
      supported: true,
      active: !!reg && !!reg.active,
      scope: reg && reg.scope,
      controlled: !!navigator.serviceWorker.controller,
      controllerUrl: navigator.serviceWorker.controller && navigator.serviceWorker.controller.scriptURL,
    };
  })()`);
  ok('浏览器支持 Service Worker', swState.supported === true);
  ok('sw.js 已激活', swState.active === true, swState.scope);
  ok('当前页面已被 SW 接管（这条不成立的话整个 E2E 无意义）',
    swState.controlled === true, swState.controllerUrl);

  // 拿一份 SW 实际下发的产物，留下证据。
  // ⚠️ 别按属性名去抠数组：产物是 **minify 过的**，`navigateFallbackDenylist` 这个键名
  // 在压缩后不一定还在（第一版就是这么写的，结果静默取到空串、断言红得莫名其妙）。
  // 直接看整份脚本里有没有那条锚定正则，稳定得多。
  const swText = await evaluate(`(async () => (await (await fetch('./sw.js')).text()))()`);
  ok('下发的 sw.js 里 /automation 在导航兜底白名单外（denylist）',
    typeof swText === 'string' && swText.includes('/^\\/automation/'),
    typeof swText === 'string' ? `sw.js ${swText.length} 字节` : String(swText));

  // [2] 真的去点 SPA 里那个入口按钮
  const click = await evaluate(`(() => {
    const links = [...document.querySelectorAll('a')];
    const a = links.find(el => (el.getAttribute('href') || '').includes('/automation'));
    if (!a) {
      return { found: false, sample: links.map(x => x.getAttribute('href')).filter(Boolean).slice(0, 12) };
    }
    a.click();
    return { found: true, href: a.href };
  })()`);
  ok('SPA 工具栏里找得到定时任务入口', click.found === true,
    click.found ? click.href : JSON.stringify(click.sample));
  if (!click.found) return finish();

  // 等导航落地
  await sleep(2500);

  const landed = await evaluate(`({
    url: location.href,
    title: document.title,
    h1: (document.querySelector('h1') || {}).textContent?.trim() || '',
    spaRoot: !!document.getElementById('app'),
    buildId: document.documentElement.dataset.buildId || null,
  })`);

  // [3] 落到的是自动化管理页，而不是 SPA 外壳
  //     —— SPA 外壳的特征：标题固定 'Cloud Clipboard'，且带 data-build-id / #app。
  ok('落地的不是 SPA 首页（用户报的这个现象）', landed.spaRoot === false,
    `#app=${landed.spaRoot} buildId=${landed.buildId}`);
  ok('URL 停在 /automation', landed.url.includes('/automation'), landed.url);
  ok('页面标题是自动化页', landed.title.includes('定时自动化'), JSON.stringify(landed.title));
  ok('页面主体是自动化页', landed.h1.includes('定时自动化'), JSON.stringify(landed.h1));

  // [4] 页面上那句「回到 SPA 首页」的返回链接也得是在的（不然用户被困住）
  const back = await evaluate(`(() => {
    const a = [...document.querySelectorAll('a')].find(el => {
      const h = el.getAttribute('href') || '';
      return h && !h.includes('/automation') && !h.startsWith('http');
    });
    return a ? { href: a.getAttribute('href'), text: a.textContent.trim() } : null;
  })()`);
  ok('自动化页上有返回主界面的链接', back !== null, back ? `${back.text} -> ${back.href}` : '没找到');

  // [5] 原生刷新一次也要稳（SW 第二次介入的路径）
  await navigate(`${BASE}/automation`);
  const second = await evaluate(`({ title: document.title, spaRoot: !!document.getElementById('app') })`);
  ok('直接访问 /automation（刷新路径）同样是自动化页',
    second.spaRoot === false && second.title.includes('定时自动化'), JSON.stringify(second));

  finish();
}

function finish() {
  console.log(`\n${fail === 0 ? 'ALL PASS' : fail + ' FAILED'}  (${pass} passed)`);
  cleanup();
  process.exit(fail === 0 ? 0 : 1);
}

main().catch((err) => {
  console.error('E2E 崩了：', err && err.message);
  cleanup();
  process.exit(3);
});
