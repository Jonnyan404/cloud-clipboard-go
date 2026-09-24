// cron「描述」的端到端核对：**真实服务端**给形状、**页面自己的**逻辑拼句子，逐条打印。
//
// 为什么不能只测一端：
//   · 只测服务端（TestDescribeCron*）会漏掉拼句子那一半；
//   · 只用桩造的 desc 测页面，会漏掉形状本身的问题。
// 描述是这两半接起来的，所以这里把真的一半接上真的另一半。
//
// 用法（需要先起一个实例）：
//
//	node tools/cron-desc-check.mjs http://127.0.0.1:9501/automation?room=home [房间密码]
//
// 退出码 0 = 每条都渲染出了非空、且不含表达式符号的句子。
const pageUrl = process.argv[2];
const password = process.argv[3] || 'home-pass';
if (!pageUrl) {
  console.error('用法: node tools/cron-desc-check.mjs <页面 URL> [房间密码]');
  process.exit(2);
}
const base = new URL(pageUrl).origin;
const room = new URL(pageUrl).searchParams.get('room') || 'default';

// 覆盖：常见写法、各字段带步长、以及刻意「说不准」的那些
const EXPRS = [
  '30 9 * * *', '0 10 * * 1', '0 0 1 * *', '0 9 1 * 1', '0 9 * JAN MON',
  '*/30 9-18 * * 1-5', '0,30 9-18 * * *', '0 9-18 * * *', '4 * * * *',
  '*/4 * * * *', '0/4 * * * *', '*/5 * * * *', '5/10 9 * * *', '0-30 9 * * *',
  '* 9 * * *', '* 9-18 * * 1-5', '0 */2 * * *', '0 */6 * * *', '* */2 * * *',
  '0 9 */2 * *', '0 9 */15 * *', '0 9 */3 * *', '0 9 * */2 *', '0 9 * */3 *',
  '0 9-18/2 * * *', '15 3 1 */4 *', '0 9 */15 * 1', '*/5 */2 */3 */2 */2', '* * * * *',
  // 「每隔 N 天」：列不全时用近似说法，短则精确列出
  '0 9 */3 * *', '0 9 1/3 * *', '0 9 */2 * *', '0 9 */3 * 1', '0 9 1/4 * *',
  // 分钟带步长 + 小时带步长：hours 读不出来时必须整体退回「说不准」，
  // 绝不能只说「每 5 分钟」而把小时限制丢掉
  '*/5 */2 * * *',
];

// ① 预取真实服务端的 desc
const real = {};
for (const expr of EXPRS) {
  const q = encodeURIComponent(expr);
  const res = await fetch(
    base + '/tasks/cron?room=' + room + '&expr=' + q + '&tz=Asia%2FShanghai',
    { headers: { Authorization: 'Bearer ' + password } },
  );
  real[expr] = await res.json();
}

// ② 把真实页面跑起来，喂真实响应
const html = await (await fetch(pageUrl)).text();
const app = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map(m => m[1]).pop();

const elements = new Map();
const makeEl = (id) => ({
  id, value: '', textContent: '', innerHTML: '', className: '', checked: false, disabled: false,
  style: {}, _listeners: {},
  addEventListener(t, f) { (this._listeners[t] = this._listeners[t] || []).push(f); },
  setAttribute(k, v) { this['attr_' + k] = v; },
  getAttribute(k) { return this['attr_' + k]; },
  querySelectorAll() { return []; },
  fire(t, e) {
    // 禁用的控件在真浏览器里不派发事件，桩照做
    if (this.disabled) { return; }
    (this._listeners[t] || []).forEach(fn => fn(e || { stopPropagation() {} }));
  },
});
const getEl = (id) => { if (!elements.has(id)) { elements.set(id, makeEl(id)); } return elements.get(id); };

const radios = ['cron', 'once'].map((v, i) => {
  const el = makeEl('ccFreq:' + v);
  el.name = 'ccFreq';
  el.value = v;
  let checked = i === 0;
  const assign = (n) => { checked = Boolean(n); };
  Object.defineProperty(el, 'checked', {
    get: () => checked,
    set: (n) => { assign(n); if (n) { radios.forEach(r => { if (r !== el) r.__assign(false); }); } },
  });
  el.__assign = assign;
  return el;
});

const store = (seed) => {
  const m = new Map(Object.entries(seed || {}));
  return {
    getItem: k => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => { m.set(k, String(v)); },
    removeItem: k => { m.delete(k); },
    dump: () => Object.fromEntries(m),
  };
};
const roomKey = room === 'default' ? '__default__' : room;
const session = { roomAuthCache: JSON.stringify({ [roomKey]: { token: 'x', expiresAt: 4102444800 } }) };

const respond = (target) => {
  const m = /[?&]expr=([^&]*)/.exec(target);
  if (target.includes('/tasks/cron') && m) {
    return real[decodeURIComponent(m[1])] || { valid: false, error: '未预取' };
  }
  if (target.includes('/server')) {
    return {
      automation: {
        enabled: true, room, tier: 'room', allowed: true, admin: false, max: 20,
        defaultTZ: 'Asia/Shanghai', vars: ['date'], actions: [],
      },
    };
  }
  if (target.includes('/tasks')) {
    return { room, tier: 'room', admin: false, max: 20, tasks: [], vars: ['date'], actions: [] };
  }
  return {};
};

const sandbox = {
  document: {
    getElementById: getEl,
    querySelectorAll: () => radios,
    querySelector: (sel) => {
      if (sel === 'input[name=ccFreq]:checked') { return radios.find(r => r.checked) || null; }
      const m = /value='([^']*)'/.exec(sel);
      return m ? (radios.find(r => r.value === m[1]) || null) : null;
    },
    documentElement: {},
  },
  window: { __CC__: { prefix: '', room }, prompt: () => null, confirm: () => false, alert: () => {} },
  navigator: { language: 'zh-CN' },
  location: { search: '?room=' + room },
  localStorage: store(),
  sessionStorage: store(session),
  fetch: (t) => {
    const b = respond(t);
    return Promise.resolve({
      ok: true, status: 200, statusText: 'OK',
      text: () => Promise.resolve(JSON.stringify(b)),
      json: () => Promise.resolve(b),
    });
  },
  URLSearchParams,
  // 短延时（输入防抖）立刻执行；长延时（令牌续签）不执行
  setTimeout: (fn, d) => {
    if (typeof fn === 'function' && (d === undefined || d < 1000)) { fn(); }
    return 0;
  },
  clearTimeout: () => {},
  console,
};
const keys = Object.keys(sandbox);
new Function(...keys, app)(...keys.map(k => sandbox[k]));

const settle = async () => { await new Promise(r => setImmediate(r)); await new Promise(r => setImmediate(r)); };
// ⚠️ 必须先等首屏 refresh 走完再驱动输入 —— 否则 selectTask 随后会覆盖我们塞进去的值，
// 看到的是「每分钟」（第一版就栽在这里，白排查了一轮）。
await settle();
await settle();

const FIELDS = ['fMin', 'fHour', 'fDom', 'fMonth', 'fDow'];
let bad = 0;
console.log('表达式'.padEnd(24) + '描述'.padEnd(48) + '未来第一次');
console.log('-'.repeat(104));
for (const expr of EXPRS) {
  const parts = expr.trim().split(/\s+/);
  FIELDS.forEach((id, i) => { getEl(id).value = parts[i]; });
  getEl('fMin').fire('input');
  await settle();

  const warned = getEl('cronWarn').className.includes('hide') ? '' : ' ⚠️';
  const text = getEl('schedHint').textContent + warned;
  const next = getEl('cronNextHint').textContent.replace(/^[^：:]*[：:]/, '').split(' · ')[0];
  console.log(expr.padEnd(24) + text.padEnd(48) + next);
  if (!text.trim()) { console.log('    ↑ 空描述'); bad++; }
  // 描述里出现表达式符号就说明又把代码念了一遍
  if (/[*?]|\//.test(text)) { console.log('    ↑ 描述里含表达式符号'); bad++; }
}

console.log('\n' + (bad === 0 ? '全部通过' : bad + ' 条有问题'));
process.exit(bad === 0 ? 0 : 1);
