// /text 的**请求体形态**：JSON / multipart 表单 / 纯文本。
//
// 与 Go 侧 readTextBody（cloud-clip/lib/handler.go）是同一份契约，两边必须一致：
// 只认 application/json 和 multipart/form-data 两种结构化形态，其余（含不声明、
// 含 urlencoded）一律「整个 body 就是正文」。
//
// 为什么要有结构化形态：快捷指令把**字符串变量**当请求体发出去时字节会变成 UTF-16，
// 而结构化请求体是按 UTF-8 序列化的。
import { TextHandler } from './.build/text.mjs';
import { ContentHandler } from './.build/content.mjs';
import { makeEnv, makeChecker } from './harness.mjs';

const { check, summary } = makeChecker();

// 带中文、行首 `-`、竖线表格 —— 正是会被 markdown 转换器转义的那几种字符
const MD = '# 标题\n- 第一条\n- 第二条\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n';
const BOUNDARY = '----ccgTestBoundary';
const MULTIPART = `--${BOUNDARY}\r\nContent-Disposition: form-data; name="content"\r\n\r\n${MD}\r\n--${BOUNDARY}--\r\n`;

async function postText(env, body, contentType) {
  const headers = { Authorization: 'Bearer 123' };
  if (contentType) {
    headers['Content-Type'] = contentType;
  }
  const req = new Request('http://worker.local/text?room=default', { method: 'POST', headers, body });
  const res = await TextHandler.create(req, env);
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* 不是 JSON 就算了 */ }
  return { status: res.status, json, text };
}

// getById 要从路径取 :id，测试里得自己把 params 挂上（和 board-column 那边一样）。
// 用 **?format=json** 拿 JSON 响应体：这是规范信号（`?json=1` 和 `.json` 后缀是兼容信号，
// 已发布的捷径还在用，别动它们）。不显式要 json 时，文本条目回的是**正文本身**。
async function readBack(env, id) {
  const req = Object.assign(
    new Request(`http://worker.local/content/${id}?room=default&format=json`,
      { headers: { Authorization: 'Bearer 123' } }),
    { params: { id: String(id) } },
  );
  const res = await ContentHandler.getById(req, env);
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* 不是 JSON 就留 null，断言那边会看出来 */ }
  return { status: res.status, json, text };
}

const cases = [
  ['纯文本（老客户端）', MD, 'text/plain', MD],
  ['不声明 Content-Type（老捷径）', MD, null, MD],
  ['JSON', JSON.stringify({ content: MD }), 'application/json', MD],
  ['multipart 表单', MULTIPART, `multipart/form-data; boundary=${BOUNDARY}`, MD],
  // ⚠️ 不是「没实现」，是**故意不认**：curl --data-binary 默认就带这个类型，
  // 当表单解析会得到空的 content 字段 —— 正文会被静默丢掉。
  ['urlencoded 刻意不认，正文原样存下', `content=${MD}`, 'application/x-www-form-urlencoded', `content=${MD}`],
];

for (const [name, body, contentType, want] of cases) {
  // ⚠️ makeEnv() 返回的是 { env, db }，不是 env 本身 —— 直接用会把 env 传成对象字面量，
  // 表现是「DB binding 不存在」→ 503，看着像接口坏了。
  const { env } = makeEnv();
  const created = await postText(env, body, contentType);
  if (created.status !== 200) {
    check(`${name} · POST 成功`, `HTTP ${created.status} ${created.text.slice(0, 120)}`, 'HTTP 200');
    continue;
  }
  const id = created.json?.id;
  const entry = await readBack(env, id);
  // check(name, got, want) 是**精确比较**，不是「条件 + 说明」
  check(`${name} · 正文逐字节一致`, entry.json?.content ?? null, want);
}

// 声明了 JSON 但正文不是 JSON：必须明确报错，不能把半截正文存进去
{
  const { env } = makeEnv();
  const bad = await postText(env, '{"content": ', 'application/json');
  check('坏 JSON · 回 400', bad.status, 400);
  check('坏 JSON · code = invalid_body', bad.json?.code ?? null, 'invalid_body');
  check('坏 JSON · message 指向请求体', String(bad.json?.message || ''), '请求体无法解析');
}

summary();
