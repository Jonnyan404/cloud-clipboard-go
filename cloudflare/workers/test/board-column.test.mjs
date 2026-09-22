// 看板的列：`POST /content/:id/column`。
//
// 与 Go 侧 handleContentColumn 是同一份契约 —— 固定三列（todo / doing / done）、
// 卡片就是剪贴板条目本身、不建新表、**不动 timestamp**。
// 这里钉的是契约，不是实现：列名归一化、错误码、以及「挪列不重排时间流」。
import { ContentHandler } from './.build/content.mjs';
import { TextHandler } from './.build/text.mjs';
import { makeEnv, makeChecker, postJson, getJson } from './harness.mjs';

const { check, summary } = makeChecker();

// harness 的 postJson/getJson 不带 params，而这两个接口要从路径里取 :id ——
// itty-router 平时把 params 塞进 request，测试里得自己挂上（和 auth-room 的 downloadRequest 一样）。
// ⚠️ 凭据也要带：makeEnv 里 AUTH_PASSWORD='123'，不带就是「认证失败」，
// 而报出来是空对象，很容易误读成「接口坏了」。
async function withParams(handler, env, path, { method = 'GET', body = null, params = {} } = {}) {
  const headers = { Authorization: 'Bearer 123' };
  if (body !== null) headers['Content-Type'] = 'application/json';
  const req = Object.assign(
    new Request(`http://worker.local${path}`, { method, headers, body }),
    { params },
  );
  const res = await handler(req, env);
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch {}
  return { status: res.status, json, text };
}

async function createText(env, content, room = 'default') {
  const r = await postJson(TextHandler.create, env, `/text?room=${room}`, content);
  return r.json;
}

const setColumn = (env, id, column, opts = {}) => withParams(
  ContentHandler.setColumn, env, `/content/${id}/column?room=${opts.room || 'default'}`,
  { method: 'POST', body: opts.rawBody !== undefined ? opts.rawBody : JSON.stringify({ column }), params: { id: String(id) } },
);

const readContent = (env, id, room = 'default') => withParams(
  ContentHandler.getById, env, `/content/${id}?room=${room}&json=1`, { params: { id: String(id) } },
);

console.log('\n── A. 挪列：生效、落库、且不动时间戳 ──');
{
  const { env } = makeEnv();
  const created = await createText(env, '待办事项');
  const id = created.id;

  const before = (await readContent(env, id)).json;
  check('新条目默认列是空串（= 待办）', before.column, '');

  const res = await setColumn(env, id, 'doing');
  check('挪列返回 200', res.status, 200);
  check('挪列响应带 id', res.json.id, String(id));
  check('挪列响应带 type', res.json.type, 'text');
  check('挪列响应带 column', res.json.column, 'doing');

  const after = (await readContent(env, id)).json;
  check('挪列落库了', after.column, 'doing');
  // ⚠️ 挪列**不能**改时间戳 —— 改了卡片会在时间流里跳到最前面，等于拖一下重排整个列表
  check('挪列不动 timestamp', after.timestamp, before.timestamp);
}

console.log('\n── B. 列名归一化 ──');
{
  const { env } = makeEnv();
  const { id } = await createText(env, '归一化');

  await setColumn(env, id, '');
  check('空串 → todo', (await readContent(env, id)).json.column, 'todo');

  await setColumn(env, id, '  DONE  ');
  check('空白 + 大写 → done', (await readContent(env, id)).json.column, 'done');

  await setColumn(env, id, undefined);
  check('字段缺失 → todo', (await readContent(env, id)).json.column, 'todo');
}

console.log('\n── C. 错误路径（错误码要和 Go 侧逐字一致）──');
{
  const { env } = makeEnv();
  const { id } = await createText(env, '错误路径');

  const bad = await setColumn(env, id, 'nope');
  check('未知列 → 400', bad.status, 400);
  check('未知列 → invalid_column', bad.json.code, 'invalid_column');

  const missing = await setColumn(env, 999999, 'todo');
  check('不存在的 id → 404', missing.status, 404);
  check('不存在的 id → content_not_found', missing.json.code, 'content_not_found');

  const badBody = await setColumn(env, id, null, { rawBody: 'not json' });
  check('请求体不是 JSON → 400', badBody.status, 400);
  check('请求体不是 JSON → invalid_body', badBody.json.code, 'invalid_body');

  const badId = await setColumn(env, 'abc', 'todo');
  check('id 不是数字 → 400', badId.status, 400);
  check('id 不是数字 → invalid_content_id', badId.json.code, 'invalid_content_id');
}

console.log('\n── D. 列要能一路走到前端（刷新后卡片不能全回待办）──');
{
  const { env, db } = makeEnv();
  const { id } = await createText(env, '载荷');
  await setColumn(env, id, 'doing');

  // 库里存的是 boardColumn 列（`column` 是 SQL 关键字，故意不叫那个名字）
  const row = db.prepare('SELECT boardColumn FROM messages WHERE id = ?').get(Number(id));
  check('库里存进 boardColumn 列', row.boardColumn, 'doing');
  // 但对外必须叫 column —— HTTP 载荷和 WebSocket 握手载荷用的是同一个 builder，
  // 只改一处会让「刷新后卡片全回待办」这种问题极难查。
  check('HTTP 载荷映射成 column', (await readContent(env, id)).json.column, 'doing');
}

console.log('\n── E. 空库上也能用（boardColumn 列是自愈加出来的）──');
{
  // makeEnv 刻意停在「没有 boardColumn 列」的旧表结构上，所以这一条真的会走 ALTER
  const { env } = makeEnv();
  const { id } = await createText(env, '自愈加列');
  const res = await setColumn(env, id, 'done');
  check('旧表结构下挪列成功', res.status, 200);
  check('旧表结构下列也落库了', (await readContent(env, id)).json.column, 'done');
}

summary('看板列在 Worker 上成立');
