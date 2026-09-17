// 测试脚手架：mock 一个 Worker env（D1 用 node:sqlite，R2 用 Map，Durable Object 用链式桩）。
// 被 sniff/upload/receive 三个测试共用。
import { DatabaseSync } from 'node:sqlite';

export function makeEnv() {
  const db = new DatabaseSync(':memory:');
  // 与线上 D1 的 messages 表结构一致
  db.exec(`CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT, content TEXT, name TEXT, size INTEGER,
    room TEXT, timestamp INTEGER, senderIP TEXT, senderClientID TEXT, userAgent TEXT,
    uuid TEXT, expireTime INTEGER, url TEXT)`);

  const DB = {
    prepare(sql) {
      const stmt = db.prepare(sql);
      let args = [];
      const api = {
        bind(...a) { args = a; return api; },
        async run() { const r = stmt.run(...args); return { meta: { last_row_id: Number(r.lastInsertRowid) } }; },
        async first() { return stmt.get(...args) ?? null; },
        async all() { return { results: stmt.all(...args) }; },
      };
      return api;
    },
  };

  const r2 = new Map();
  const R2_BUCKET = {
    async put(key, body, opts) { r2.set(key, { body, opts }); },
    async delete(key) { r2.delete(key); },
    async get(key) { return r2.has(key) ? { body: r2.get(key) } : null; },
  };

  // Durable Object 桩：链式调用不断，且必须让 then 为 undefined ——
  // 否则 await 一个 Proxy 会无限递归（Proxy 对 then 也返回自身，成为永不 resolve 的 thenable）。
  const makeStub = () => new Proxy(function () {}, {
    get: (t, prop) => (prop === 'then' ? undefined : makeStub()),
    apply: () => Promise.resolve(makeStub()),
  });

  const env = {
    DB, R2_BUCKET, WEBSOCKET_ROOM: makeStub(),
    AUTH_PASSWORD: '123', ROOM_AUTH_JSON: '{}', HISTORY_LIMIT: '50',
    TEXT_LIMIT: '40960', FILE_LIMIT: '204857600', FILE_EXPIRE: '3600',
  };
  return { env, db, r2 };
}

export function makeChecker() {
  let pass = 0, fail = 0;
  const failures = [];
  return {
    check(name, got, want) {
      const g = JSON.stringify(got), w = JSON.stringify(want);
      if (g === w) { pass++; console.log(`  ✓ ${name}`); }
      else {
        fail++;
        failures.push(`${name}\n      实际: ${g}\n      期望: ${w}`);
        console.log(`  ✗ ${name}  实际=${g} 期望=${w}`);
      }
    },
    summary(label) {
      console.log(`\n通过 ${pass} 项，失败 ${fail} 项`);
      if (failures.length) {
        console.log('\n失败明细：');
        for (const f of failures) console.log('  ✗ ' + f);
        process.exit(1);
      }
      console.log(`${label} ✅`);
    },
  };
}

// 发一个 POST 请求并解析响应
export async function postJson(handler, env, path, body, { auth = 'Bearer 123' } = {}) {
  const headers = {};
  if (auth) headers.Authorization = auth;
  const req = new Request(`http://worker.local${path}`, { method: 'POST', headers, body });
  const res = await handler(req, env);
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch {}
  return { status: res.status, json, text };
}

// 发一个 GET 请求并解析响应
export async function getJson(handler, env, path, { auth = 'Bearer 123', accept } = {}) {
  const headers = {};
  if (auth) headers.Authorization = auth;
  if (accept) headers.Accept = accept;
  const req = new Request(`http://worker.local${path}`, { method: 'GET', headers });
  const res = await handler(req, env);
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch {}
  return { status: res.status, json, text, headers: res.headers };
}
