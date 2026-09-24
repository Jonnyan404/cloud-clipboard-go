// 在 Node 里真跑一遍服务端渲染页面的内联 JS（最小 DOM 桩，不需要浏览器）。
//
// 为什么需要它：`lib/*_test.go` 对页面只能做**字符串检查**，抓不到运行时报错 ——
// 而页面上一个拼错的函数名或元素 id 会让整页空白，这种故障只有真跑一次才看得见。
// 本仓库里有两个服务端渲染页面（`share_landing.go`、`automation_page.go`），
// 它们的内联脚本都适用这个办法。
//
// 用法：
//
//	node tools/page-smoke.mjs http://127.0.0.1:9501/automation?room=home
//
// 退出码 0 表示全部通过。只依赖 Node 18+（用全局 fetch），不需要装任何包 ——
// 环境里装不上浏览器时（比如受限网络）这是唯一能在真 JS 引擎里跑页面逻辑的办法。
//
// 局限（写在这里免得被当成万能）：
//   · 这不是真浏览器：不验 CSS 布局、不验跨页面导航时的 sessionStorage 语义；
//   · DOM 桩只实现页面用到的那几个 API（少一个就会当场抛异常，第 [1] 组变红，
//     不会静默漏测）；
//   · 桩对 innerHTML 只做**属性级**的浅解析，够页面「先 innerHTML 再挂事件」的用法，
//     不支持选择器组合、层级、伪类。
const url = process.argv[2];
if (!url) {
  console.error('用法: node tools/page-smoke.mjs <页面 URL>');
  process.exit(2);
}

const html = await (await fetch(url)).text();
const scripts = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map(m => m[1]);
if (!scripts.length) {
  console.error('页面里没有 <script> 块，无从检查');
  process.exit(2);
}
// 取最后一个脚本块 = 页面的应用逻辑（前面的通常是配置注入）
const app = scripts[scripts.length - 1];

const room = new URL(url).searchParams.get('room') || 'default';

let pass = 0, fail = 0;
const ok = (name, cond, extra) => {
  if (cond) { console.log('  ok  ' + name); pass++; }
  else { console.log('  FAIL ' + name + (extra !== undefined ? ' -> ' + extra : '')); fail++; }
};

function makeStorage(seed) {
  const m = new Map(Object.entries(seed || {}));
  return {
    getItem: k => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => { m.set(k, String(v)); },
    removeItem: k => { m.delete(k); },
    dump: () => Object.fromEntries(m),
  };
}

// 一个最小元素。只实现页面实际用到的成员。
function makeEl(id) {
  return {
    id, value: '', textContent: '', innerHTML: '', className: '', placeholder: '',
    checked: false, disabled: false, style: {}, children: [], _listeners: {},
    addEventListener(type, fn) { (this._listeners[type] = this._listeners[type] || []).push(fn); },
    setAttribute(k, v) { this['attr_' + k] = v; },
    getAttribute(k) { return this['attr_' + k]; },
    // ⚠️ 必须按选择器缓存：真 DOM 里同一次渲染出来的子元素是**同一批对象**，
    // 页面把监听器挂在它们身上。桩要是每次重新解析，「点了没反应」就会被误判成通过。
    querySelectorAll(sel) {
      const key = String(sel);
      this.__kids = this.__kids || {};
      if (!(key in this.__kids)) { this.__kids[key] = parseChildren(this.innerHTML, key); }
      return this.__kids[key];
    },
    focus() {},
    // 真浏览器里禁用的控件**不会派发事件**（点保存、敲输入框都一样）。桩照做，
    // 否则「加载完成前不许输入」这类保护会被测成永远通过。
    fire(type, ev) {
      if (this.disabled) { return; }
      (this._listeners[type] || []).forEach(fn => fn(ev || { stopPropagation() {} }));
    },
  };
}

// 从 innerHTML 里认出 `class='chip'` / `[data-tgl]` 这类子元素。
//
// 页面渲染列表和胶囊都是「先写 innerHTML，再 querySelectorAll('.xxx') 挂事件」，
// 所以桩只要能把「有几个、各带什么属性」还原出来就够了 —— 不支持层级和选择器组合。
function parseChildren(html, sel) {
  const byClass = /^\.([A-Za-z][\w-]*)$/.exec(String(sel));
  const byAttr = /^\[([\w-]+)\]$/.exec(String(sel));
  if (!byClass && !byAttr) { return []; }

  const out = [];
  for (const tag of String(html).match(/<[a-zA-Z][^>]*>/g) || []) {
    const attrs = {};
    const re = /([\w-]+)\s*=\s*(?:'([^']*)'|"([^"]*)")/g;
    let m;
    while ((m = re.exec(tag))) { attrs[m[1]] = m[2] !== undefined ? m[2] : m[3]; }

    const classes = String(attrs.class || '').split(/\s+/).filter(Boolean);
    if (byClass && !classes.includes(byClass[1])) { continue; }
    if (byAttr && !(byAttr[1] in attrs)) { continue; }

    const el = makeEl(attrs.id || attrs['data-i'] || attrs['data-p'] || 'child');
    for (const k of Object.keys(attrs)) { el[k] = attrs[k]; el['attr_' + k] = attrs[k]; }
    out.push(el);
  }
  return out;
}

// 跑一个场景：返回页面跑完后各元素的最终状态，以及 fetch 收到的请求。
// 服务端的应答由调用方给出，这样调用方可以**忠实模拟鉴权**（没带凭据就不放行）——
// 桩要是永远放行，「免二次登录」这类断言就成了自证。
function runScenario(appSource, { seedSession, respond, language = 'zh-CN' }) {
  const elements = new Map();
  const getEl = (id) => {
    if (!elements.has(id)) { elements.set(id, makeEl(id)); }
    return elements.get(id);
  };

  // 「什么时候」的两档是一组 radio，页面用 document.querySelector 读写它。
  // 桩必须实现**同组互斥**：真浏览器里选中一个会自动取消另一个，桩要是不做，
  // getFreq() 会同时看到两个「选中」，切换逻辑就等于没测。
  const freqRadios = ['cron', 'once'].map((value, i) => {
    const el = makeEl('ccFreq:' + value);
    el.name = 'ccFreq';
    el.value = value;
    let checked = i === 0;
    const assign = (next) => { checked = Boolean(next); };
    el.__assignChecked = assign;
    Object.defineProperty(el, 'checked', {
      get: () => checked,
      set: (next) => {
        assign(next);
        if (next) { freqRadios.forEach(r => { if (r !== el) r.__assignChecked(false); }); }
      },
    });
    return el;
  });

  const calls = [];
  const localStorageStub = makeStorage();
  const sessionStorageStub = makeStorage(seedSession);

  const fetchStub = (target, init) => {
    const headers = (init && init.headers) || {};
    const req = {
      url: target,
      auth: headers['Authorization'] || '',
      method: (init && init.method) || 'GET',
      body: (init && init.body) || '',
    };
    calls.push(req);
    const body = respond(target, req.auth, sessionStorageStub);
    return Promise.resolve({
      ok: true, status: 200, statusText: 'OK',
      text: () => Promise.resolve(JSON.stringify(body)),
      json: () => Promise.resolve(body),
    });
  };

  const documentStub = {
    getElementById: getEl,
    querySelector(sel) {
      if (sel === 'input[name=ccFreq]:checked') { return freqRadios.find(r => r.checked) || null; }
      const m = /^input\[name=ccFreq\]\[value='([^']*)'\]$/.exec(sel);
      if (m) { return freqRadios.find(r => r.value === m[1]) || null; }
      return null;
    },
    querySelectorAll(sel) {
      return sel === 'input[name=ccFreq]' ? freqRadios : [];
    },
    documentElement: {},
  };

  const sandbox = {
    document: documentStub,
    window: { __CC__: { prefix: '', room }, prompt: () => null, confirm: () => false, alert: () => {} },
    navigator: { language },
    location: { search: '?room=' + room },
    localStorage: localStorageStub,
    sessionStorage: sessionStorageStub,
    fetch: fetchStub,
    URLSearchParams,
    // 短延时（< 1s）= 输入防抖（cron 预览），**立刻执行** —— 否则那条路径根本没被走到；
    // 长延时 = 令牌静默续签，不执行（执行会立刻发一次请求，污染请求断言）。
    setTimeout: (fn, delay) => {
      if (typeof fn === 'function' && (delay === undefined || delay < 1000)) { fn(); }
      return 0;
    },
    clearTimeout: () => {},
    console,
  };

  const keys = Object.keys(sandbox);
  new Function(...keys, appSource)(...keys.map(k => sandbox[k]));
  return { getEl, calls, freqRadios, localStorage: localStorageStub, sessionStorage: sessionStorageStub };
}

const settle = async () => { await new Promise(r => setImmediate(r)); await new Promise(r => setImmediate(r)); };

// 切档位：真浏览器里点一个 radio 是**两步** —— 先选中（同组自动互斥），再触发 change。
// 桩只 fire('change') 的话 checked 根本没变，切换逻辑就测了个空。
function selectFreq(scenario, value) {
  const radio = scenario.freqRadios.find(r => r.value === value);
  radio.checked = true;
  radio.fire('change');
  return radio;
}

// 五个字段框：读出来拼成表达式（与页面 readCron 同义）
const CRON_FIELDS = ['fMin', 'fHour', 'fDom', 'fMonth', 'fDow'];
const readCron = (s) => CRON_FIELDS.map(id => String(s.getEl(id).value || '').trim() || '*').join(' ');
// 敲进某个字段并触发 input（真浏览器里每次按键都会发 input）
const typeCron = (s, id, value) => { s.getEl(id).value = value; s.getEl(id).fire('input'); };

// ── 通用：脚本能不能跑起来 ──────────────────────────────────────────
console.log('[1] 内联脚本能跑起来（不崩）');
let error = null;
try {
  runScenario(app, { seedSession: {}, respond: () => ({}) });
} catch (e) {
  error = e;
}
ok('执行无异常', !error, error && (error.message + ' | ' + String(error.stack).split('\n')[1]));
if (error) { console.log('\n通过 ' + pass + '，失败 ' + (fail + 1)); process.exit(1); }

// ── 通用：页面是否声明了多语言文案表 ────────────────────────────────
if (app.includes('var MSG = {')) {
  console.log('\n[2] 文案表每条都有全部语言（靠 Go 侧的自动化页面契约测试覆盖，这里只做存在性确认）');
  const langs = (app.match(/var LANGS = \[([^\]]*)\]/) || [])[1] || '';
  const codes = langs.split(',').map(s => s.trim()).filter(Boolean);
  ok('声明了语言列表', codes.length >= 2, codes.join('/'));

  // ── 专用：自动化管理页 ──────────────────────────────────────────
  const ACTIONS = [
    { id: 'text.trimLines', group: 'text', groupKey: 'actionGroupText', key: 'actionTrimLines' },
    { id: 'date.add', group: 'date', groupKey: 'actionGroupDate', key: 'actionDateAdd' },
  ];
  const VARS = ['date', 'weekday', 'time', 'datetime', 'timestamp', 'uuid', 'task', 'room', 'latest'];
  const SERVER = (authed) => ({
    automation: authed
      ? { enabled: true, room, tier: 'room', allowed: true, admin: false, max: 20,
          defaultTZ: 'Asia/Shanghai', vars: VARS, actions: ACTIONS }
      : { enabled: true, room, tier: 'none', allowed: false, admin: false, max: 0,
          defaultTZ: 'Asia/Shanghai', vars: VARS, actions: ACTIONS },
  });

  // 简化版 DescribeCron：只覆盖这几条断言需要的形状。
  // 真判断逻辑由 Go 的 `TestDescribeCronShapes` 覆盖；这里存在的意义是让页面拿到
  // **随输入变化**的 desc —— 否则「改了输入，翻译和提醒会跟着变」这件事根本没被验证。
  const describe = (expr) => {
    const [, , dom, , dow] = String(expr).trim().split(/\s+/);
    const [mi, ho] = String(expr).trim().split(/\s+/);
    // 星期名要**展开成具体值**（真服务端就是这么做的）：`1-5` → [1,2,3,4,5]。
    // 少了这一步，页面上会拼出光秃秃一个「每」—— 那是桩的错，不是页面的错。
    const weekdays = [];
    if (dow && dow !== '*') {
      for (const part of String(dow).split(',')) {
        const m = /^(\d+)-(\d+)$/.exec(part);
        if (m) { for (let i = Number(m[1]); i <= Number(m[2]); i++) { weekdays.push(i % 7); } }
        else if (/^\d+$/.test(part)) { weekdays.push(Number(part) % 7); }
      }
    }
    const out = {};
    if (weekdays.length) { out.day = 'weekly'; out.weekdays = weekdays; }
    else if (/^[*\d]+\/\d+$/.test(dom)) {
      // 「日」带步长 → 每隔 N 天（真服务端在列不全时就是这么归的）
      Object.assign(out, { day: 'everyNDays', dayN: Number(dom.split('/')[1]) });
    } else {
      out.day = (dom && dom !== '*') ? 'monthly' : 'daily';
      if (out.day === 'monthly') { out.dom = dom; }
    }
    if (mi === '*' && ho === '*') { return Object.assign(out, { mode: 'everyMinute' }); }
    if (/^\*\/\d+$/.test(ho) && /^\d+$/.test(mi)) {
      // `0 */2 * * *` = 每 2 小时的第 0 分（真服务端有专门一档，别展开成 12 个时刻）
      return Object.assign(out, { mode: 'everyNHours', n: Number(ho.slice(2)), minutes: mi });
    }
    if (/^\*\/\d+$/.test(mi)) {
      Object.assign(out, { mode: 'everyNMinutes', n: Number(mi.slice(2)) });
      if (ho !== '*') { out.hours = ho; }
      return out;
    }
    const times = (/^\d+$/.test(mi) && /^\d+$/.test(ho))
      ? [String(ho).padStart(2, '0') + ':' + String(mi).padStart(2, '0')] : [];
    return Object.assign(out, { mode: 'times', times });
  };

  const TASKS = {
    room, tier: 'room', admin: false, max: 20, now: 0, vars: VARS, actions: ACTIONS,
    tasks: [{ id: 't1', name: '示例任务', enabled: true, freq: 'cron', cron: '*/30 9-18 * * 1-5',
              desc: describe('*/30 9-18 * * 1-5'), tz: 'Asia/Shanghai', room, template: 'X {{date}}',
              chain: ['text.trimLines'], keepHistory: false, sender: '',
              nextRunAt: 0, lastRunAt: 0, lastStatus: '', lastError: '', lastOutput: '' }],
  };

  const respond = (target, auth) => {
    if (target.includes('/tasks/cron')) {
      const expr = decodeURIComponent((/[?&]expr=([^&]*)/.exec(target) || [])[1] || '');
      return {
        valid: true, tz: 'Asia/Shanghai', expr, desc: describe(expr),
        nextFormatted: ['2026-09-24 09:00 Thu', '2026-09-24 09:30 Thu', '2026-09-24 10:00 Thu'],
      };
    }
    if (target.includes('/server')) { return SERVER(Boolean(auth)); }
    if (target.includes('/tasks')) { return TASKS; }
    return {};
  };

  console.log('\n[3] 没有凭据时：要求输入，而不是显示一堆空控件');
  const anon = runScenario(app, { seedSession: {}, respond });
  await settle();
  ok('横幅给出输入入口', /输入凭据|Enter password|認証情報を入力/.test(anon.getEl('gate').innerHTML));
  ok('编辑器被收起', anon.getEl('panel').className.includes('hide'), anon.getEl('panel').className);

  console.log('\n[4] 已有会话令牌时：不再问密码（免二次登录）');
  const token = 'tok-smoke';
  const authed = runScenario(app, {
    seedSession: { roomAuthCache: JSON.stringify({ [room === 'default' ? '__default__' : room]: { token, expiresAt: 4102444800 } }) },
    respond,
  });
  await settle();
  ok('没有要求输入凭据', !/输入凭据|Enter password/.test(authed.getEl('gate').innerHTML));
  ok('每个请求都带上了那枚令牌',
    authed.calls.length >= 2 && authed.calls.every(c => c.auth === 'Bearer ' + token),
    JSON.stringify(authed.calls.map(c => [c.url, c.auth])));
  ok('动作显示人话名字而不是 id',
    authed.getEl('actionBox').innerHTML.includes('>text.trimLines<') === false,
    authed.getEl('actionBox').innerHTML.slice(0, 120));
  ok('动作 id 留在 tooltip', authed.getEl('actionBox').innerHTML.includes("title='text.trimLines'"));
  ok('变量胶囊显示人话', authed.getEl('varChips').innerHTML.length > 0);
  // 「某房间的最新消息」是唯一需要额外解释的变量（读外部状态 + 跨房间权限），
  // 说明挂在悬停里 —— 别让用户踩了坑才知道规则。
  const latestChip = authed.getEl('varChips').querySelectorAll('.chip').find(c => c['data-v'] === '{{latest}}');
  ok('有「某房间的最新消息」胶囊', Boolean(latestChip));
  ok('它的悬停说明里写了怎么指定房间', /latest:/.test(String(latestChip && latestChip.title)), latestChip && latestChip.title);
  ok('localStorage 里没有明文密码键', !('ccgAutomationAuth' in authed.localStorage.dump()));

  console.log('\n[5] 循环：五个字段框各管一段');
  ok('表达式被拆进五个框', readCron(authed) === '*/30 9-18 * * 1-5', readCron(authed));
  ok('默认档位是循环（字段框可用）', authed.getEl('fMin').disabled === false);
  const hint = authed.getEl('schedHint').textContent;
  ok('「翻译」说人话而不是照抄表达式',
    hint.includes('每周一') && hint.includes('每 30 分钟') && hint.includes('9-18') && !hint.includes('*/30'), hint);
  ok('同时给出未来几次具体时刻', authed.getEl('cronNextHint').textContent.includes('2026-09-24'), authed.getEl('cronNextHint').textContent);
  ok('30 分钟一次不算刷屏（不显示警告）', authed.getEl('cronWarn').className.includes('hide'), authed.getEl('cronWarn').className);

  console.log('\n[6] 常用写法按钮：只动「日 / 月 / 周」，不动已写好的时刻');
  const presets = authed.getEl('cronPresets').querySelectorAll('.chip');
  ok('渲染出了常用写法按钮', presets.length >= 5, presets.length);
  // 顺序：0=每天 1=每隔 3 天 2=工作日 3=每周一 4=每月 1 日
  const every3 = presets.find(p => p['data-p'] === '1');
  every3.fire('click');
  await settle();
  ok('点「每隔 3 天」把「日」写成 1/3', authed.getEl('fDom').value === '1/3', authed.getEl('fDom').value);
  ok('描述认得出「每隔 3 天」', authed.getEl('schedHint').textContent.includes('每隔 3 天'), authed.getEl('schedHint').textContent);
  // 按钮上那句「每隔 3 天」是近似说法（cron 按月算，跨月那一次间隔会短），
  // 所以必须带悬停说明 —— 否则就是一句静默的假话。
  ok('「每隔 3 天」带悬停说明', String(every3.title || '').includes('跨月'), every3.title);
  ok('「每隔 3 天」没动时刻', authed.getEl('fMin').value === '*/30' && authed.getEl('fHour').value === '9-18');

  presets.find(p => p['data-p'] === '2').fire('click'); // 工作日
  await settle();
  ok('点「工作日」把「周」写成 1-5', authed.getEl('fDow').value === '1-5', authed.getEl('fDow').value);
  ok('「分」「时」没被动过（*/30 与 9-18 保持原样）',
    authed.getEl('fMin').value === '*/30' && authed.getEl('fHour').value === '9-18',
    authed.getEl('fMin').value + ' / ' + authed.getEl('fHour').value);
  presets.find(p => p['data-p'] === '0').fire('click'); // 每天
  await settle();
  ok('点「每天」把「周」清回 *', authed.getEl('fDow').value === '*', authed.getEl('fDow').value);
  ok('时刻依旧没被动过', authed.getEl('fMin').value === '*/30' && authed.getEl('fHour').value === '9-18');

  console.log('\n[7] 刷屏提醒：默认的「全 *」会被拦一句');
  typeCron(authed, 'fMin', '*');
  typeCron(authed, 'fHour', '*');
  await settle();
  ok('翻译变成「每分钟」', authed.getEl('schedHint').textContent.includes('每分钟'), authed.getEl('schedHint').textContent);
  ok('出现刷屏提醒', !authed.getEl('cronWarn').className.includes('hide'), authed.getEl('cronWarn').className);
  ok('提醒说的是「会持续往房间发消息」', /房间/.test(authed.getEl('cronWarn').textContent), authed.getEl('cronWarn').textContent);
  typeCron(authed, 'fMin', '30');
  typeCron(authed, 'fHour', '9');
  await settle();
  ok('改成每天 09:30 后提醒消失', authed.getEl('cronWarn').className.includes('hide'), authed.getEl('cronWarn').className);
  ok('翻译跟着变成「每天 …」', authed.getEl('schedHint').textContent.includes('每天'), authed.getEl('schedHint').textContent);

  console.log('\n[8] 仅一次：与循环互斥');
  selectFreq(authed, 'once');
  ok('五个字段框全部被禁用', CRON_FIELDS.every(id => authed.getEl(id).disabled === true));
  ok('「仅一次」的输入被启用', authed.getEl('fRunAt').disabled === false);
  ok('翻译换成一次性的说法', /一次/.test(authed.getEl('schedHint').textContent), authed.getEl('schedHint').textContent);
  ok('切档后刷屏提醒被清掉', authed.getEl('cronWarn').className.includes('hide'));

  // 时刻框是个**日期时间选择器**：切进来时不能是空的 —— 空框 + 原生选择器停在今天
  // 00:00，用户得一路点到他要的时刻。默认补一小时后，且必须在未来（过去的时刻服务端
  // 会当到期事件，保存即发送）。
  const prefill = String(authed.getEl('fRunAt').value || '');
  const STAMP_RE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/;
  ok('空框自动补一个时刻', STAMP_RE.test(prefill), prefill);
  ok('min 与选择器同格式（挡得住过去的日期）', STAMP_RE.test(String(authed.getEl('fRunAt').min || '')), String(authed.getEl('fRunAt').min));
  ok('补的是未来时刻（不会一保存就发）', String(authed.getEl('fRunAt').min) < prefill,
    'min=' + authed.getEl('fRunAt').min + ' v=' + prefill);
  ok('提示里说的是人话格式（不带 T）', !/\dT\d/.test(authed.getEl('schedHint').textContent), authed.getEl('schedHint').textContent);

  authed.getEl('fRunAt').value = '';
  authed.getEl('fRunAt').fire('input');
  ok('清空后提示「还没选时间」', /还没选时间|no time picked/.test(authed.getEl('schedHint').textContent), authed.getEl('schedHint').textContent);
  const beforeEmptySave = authed.calls.length;
  authed.getEl('btnSave').fire('click');
  await settle();
  ok('清空后保存被前端拦住（不发请求）', authed.calls.length === beforeEmptySave,
    authed.calls.slice(beforeEmptySave).map(c => c.url).join(' '));
  ok('状态栏说明原因', /还没选时间|no time picked/.test(authed.getEl('status').textContent), authed.getEl('status').textContent);

  selectFreq(authed, 'cron');
  ok('切回循环后字段框恢复可用', CRON_FIELDS.every(id => authed.getEl(id).disabled === false));

  console.log('\n[9] 保存时发的字段跟着档位走');
  // 记刻度再取：refresh() 会顺带发别的请求，「倒数第 n 条」会漂。
  const savesSince = (mark) => authed.calls.slice(mark)
    .filter(c => c.method === 'POST' && c.url.includes('/tasks'))
    .map(c => { try { return JSON.parse(c.body); } catch (e) { return null; } });
  let mark = authed.calls.length;
  selectFreq(authed, 'once');
  authed.getEl('fRunAt').value = '2026-10-01T09:30';
  authed.getEl('btnSave').fire('click');
  await settle();
  const onceBody = savesSince(mark)[0];
  ok('「仅一次」只发 runAt，不发 cron',
    onceBody && onceBody.freq === 'once' && onceBody.runAt === '2026-10-01T09:30' && onceBody.cron === undefined,
    onceBody && JSON.stringify(onceBody));

  mark = authed.calls.length;
  selectFreq(authed, 'cron');
  typeCron(authed, 'fMin', '0');
  typeCron(authed, 'fHour', '10');
  typeCron(authed, 'fDow', '1');
  await settle();
  authed.getEl('btnSave').fire('click');   // 别忘了真的点保存
  await settle();
  const cronBody = savesSince(mark)[0];
  ok('循环档把五个框拼回一个表达式发给服务端',
    cronBody && cronBody.freq === 'cron' && cronBody.cron === '0 10 * * 1',
    cronBody && JSON.stringify(cronBody));
  ok('循环档不发 runAt / time / byWeekday',
    cronBody && cronBody.runAt === undefined && cronBody.time === undefined && cronBody.byWeekday === undefined,
    cronBody && JSON.stringify(cronBody));

  console.log('\n[10] 首次加载完成前，排期字段不可编辑');
  // 服务端慢一点（第一次请求还没回来），此时用户能看到的字段必须是禁用的 ——
  // 否则他敲进去的值会被随后到达的 selectTask 静默覆盖。
  const slow = runScenario(app, {
    seedSession: { roomAuthCache: JSON.stringify({ [room === 'default' ? '__default__' : room]: { token: 'x', expiresAt: 4102444800 } }) },
    respond,
  });
  ok('首屏请求还没回来时，五个字段框是禁用的',
    CRON_FIELDS.every(id => slow.getEl(id).disabled === true),
    CRON_FIELDS.map(id => id + '=' + slow.getEl(id).disabled).join(' '));
  ok('此时保存按钮也不可点', slow.getEl('btnSave').disabled === true);
  ok('常用写法按钮此时不可点（有 off 类）', slow.getEl('cronPresets').className.includes('off'), slow.getEl('cronPresets').className);
  // 禁用的控件在真浏览器里不派发事件；桩也照做，所以这次输入不该生效
  slow.getEl('fMin').value = '7';
  slow.getEl('fMin').fire('input');
  ok('禁用状态下输入不产生请求',
    !slow.calls.some(c => c.url.includes('/tasks/cron') && c.url.includes('7')),
    slow.calls.map(c => c.url).join(' ').slice(0, 120));
  await settle();
  ok('加载完成后字段恢复可用',
    CRON_FIELDS.every(id => slow.getEl(id).disabled === false),
    CRON_FIELDS.map(id => id + '=' + slow.getEl(id).disabled).join(' '));

  console.log('\n[11] 每 N 小时与「描述里不许出现表达式符号」');
  typeCron(authed, 'fMin', '0');
  typeCron(authed, 'fHour', '*/2');
  typeCron(authed, 'fDow', '*');
  await settle();
  const h2 = authed.getEl('schedHint').textContent;
  ok('「每 2 小时」有专门的说法', h2.includes('每 2 小时'), h2);
  ok('没有把 `*/2` 原样念出来', !/[*?]|\//.test(h2), h2);

  // 通用不变量：任何输入的描述里都不该出现表达式符号 ——
  // 出现了就说明又把 token 直接拼进句子了（这正是这次修掉的那类 bug）。
  const samples = ['*/30 9-18 * * 1-5', '30 9 * * *', '* * * * *', '0 9 */15 * *', '0 */6 * * *'];
  let leaked = '';
  for (const expr of samples) {
    const parts = expr.split(' ');
    CRON_FIELDS.forEach((id, i) => typeCron(authed, id, parts[i]));
    await settle();
    const text = authed.getEl('schedHint').textContent;
    if (/[*?]|\//.test(text)) { leaked = expr + ' → ' + text; break; }
    if (!text.trim()) { leaked = expr + ' → （空描述）'; break; }
  }
  ok('样本文案里都没有泄漏表达式符号', leaked === '', leaked);

  console.log('\n[12] 回去的路带着房间');
  const back = authed.getEl('backLink');
  ok('返回链接带上了当前房间', String(back.href || '').includes('room=' + encodeURIComponent(room)), back.href);

  console.log('\n[13] 「仅一次」：服务端存回来的 RFC3339 要能落到选择器里');
  // 服务端把 runAt 归一化成带时区的 RFC3339（`2026-10-01T09:30:00+08:00`），
  // 而 `input[type=datetime-local]` 只认 `YYYY-MM-DDTHH:mm` —— 直接赋值是**静默失败**，
  // value 变成空串，用户看到的是「这条任务的时间不见了」。这一组就是钉住那次转换。
  const ONCE_TASKS = Object.assign({}, TASKS, {
    tasks: [Object.assign({}, TASKS.tasks[0], {
      freq: 'once', runAt: '2026-10-01T09:30:00+08:00', cron: '',
    })],
  });
  const respondOnce = (target, auth) => {
    if (target.includes('/tasks/cron')) { return respond(target, auth); }
    if (target.includes('/tasks')) { return ONCE_TASKS; }
    return respond(target, auth);
  };
  const onceEdit = runScenario(app, {
    seedSession: { roomAuthCache: JSON.stringify({ [room === 'default' ? '__default__' : room]: { token, expiresAt: 4102444800 } }) },
    respond: respondOnce,
  });
  await settle();
  ok('RFC3339 被削成选择器认的 YYYY-MM-DDTHH:mm（不是空串）',
    onceEdit.getEl('fRunAt').value === '2026-10-01T09:30', JSON.stringify(onceEdit.getEl('fRunAt').value));
  ok('那一档被选中', onceEdit.freqRadios.find(r => r.value === 'once').checked === true);
  ok('五个字段框在「仅一次」下被禁用', CRON_FIELDS.every(id => onceEdit.getEl(id).disabled === true));
  ok('提示里是人话时刻（把 T 换成空格）',
    /只在 2026-10-01 09:30 触发一次/.test(onceEdit.getEl('schedHint').textContent),
    onceEdit.getEl('schedHint').textContent);
  ok('未来时刻不报「已经过去」', onceEdit.getEl('cronWarn').className.includes('hide'),
    onceEdit.getEl('cronWarn').textContent);
}

console.log('\n========================');
console.log('通过 ' + pass + ' 项，失败 ' + fail + ' 项');
process.exit(fail === 0 ? 0 : 1);
