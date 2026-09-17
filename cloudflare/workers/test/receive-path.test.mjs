// 验证 Receive 依赖的完整链路：上传 → /content/latest?json=1 → Receive 的分支决策。
//
// 这条链路此前只在 Go 后端上验过。Worker 版的返回结构若与 Go 不一致，
// Receive 在 Worker 部署上就会坏 —— 与「Send 因缺端点而 404」是同一类问题。
//
// 断言方式不是比字段，而是**模拟 Receive 源码里的决策树**，看它最终会做什么：
//   uuid 存在 → 走文件分支（type=image 则进剪贴板，否则弹保存对话框）
//   否则取 content → 进剪贴板
import { RawUploadHandler } from './.build/raw-upload.mjs';
import { FileHandler } from './.build/file.mjs';
import { ContentHandler } from './.build/content.mjs';
import { makeEnv, makeChecker, postJson, getJson } from './harness.mjs';

const { check, summary } = makeChecker();

// 模拟 Receive 的决策树，返回它会执行的动作
function whatReceiveWouldDo(record) {
  const uuid = record.uuid;
  const name = record.name;
  const type = record.type;
  if (uuid) {
    if (name) {
      return type === 'image' ? '下载并放入剪贴板(图片)' : '下载并弹出保存对话框(文件)';
    }
    return '停止(有 uuid 但无文件名)';
  }
  if (!record.content) return '报警:服务器没有返回文字内容';
  return '文字放入剪贴板';
}

const PNG = new Uint8Array([
  0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
  0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
]);

console.log('\n── A. 上传纯文本 → Receive 应当把文字放进剪贴板 ──');
{
  const { env } = makeEnv();
  await postJson(RawUploadHandler.upload, env, '/upload/raw?room=default&client=ios-shortcuts', '阿岚的验收文本 12345');
  const r = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default&json=1');
  check('HTTP 200', r.status, 200);
  check('type=text', r.json.type, 'text');
  check('content 正确', r.json.content, '阿岚的验收文本 12345');
  check('uuid 不存在（Receive 才会走文字分支）', r.json.uuid, undefined);
  check('Receive 会做什么', whatReceiveWouldDo(r.json), '文字放入剪贴板');
}

console.log('\n── B. 上传 PNG → Receive 应当把图片放进剪贴板 ──');
{
  const { env } = makeEnv();
  await postJson(RawUploadHandler.upload, env, '/upload/raw?room=default&client=ios-shortcuts', PNG);
  const r = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default&json=1');
  check('type=image', r.json.type, 'image');
  check('有 uuid', typeof r.json.uuid, 'string');
  check('name=clipboard.png', r.json.name, 'clipboard.png');
  check('Receive 会做什么', whatReceiveWouldDo(r.json), '下载并放入剪贴板(图片)');
}

console.log('\n── C. 上传 .txt 文件（multipart）→ Receive 应当弹保存对话框 ──');
{
  const { env } = makeEnv();
  // 走 multipart —— 快捷指令的文件分支就是这样，part 自带真实文件名
  const form = new FormData();
  form.append('file', new Blob([Buffer.from('django==4.2.*')], { type: 'text/plain' }), 'requirements.txt');
  await FileHandler.upload(new Request('http://worker.local/upload?room=default&client=ios-shortcuts', {
    method: 'POST', headers: { Authorization: 'Bearer 123' }, body: form,
  }), env);
  const r = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default&json=1');
  check('type=file', r.json.type, 'file');
  // multipart 的 part 自带真实文件名，所以不再是 clipboard.txt
  check('文件名保真', r.json.name, 'requirements.txt');
  check('Receive 会做什么', whatReceiveWouldDo(r.json), '下载并弹出保存对话框(文件)');
}

console.log('\n── D. 三种触发 JSON 的方式都要生效（Receive 用的是 ?json=1）──');
{
  const { env } = makeEnv();
  await postJson(RawUploadHandler.upload, env, '/upload/raw?room=default', 'json 触发方式测试');

  const byParam = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default&json=1');
  check('?json=1 返回 JSON', byParam.json?.type, 'text');

  const byJsonTrue = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default&json=true');
  check('?json=true 返回 JSON', byJsonTrue.json?.type, 'text');

  const bySuffix = await getJson(ContentHandler.getLatest, env, '/content/latest.json?room=default');
  check('路径后缀 .json 返回 JSON', bySuffix.json?.type, 'text');

  const byAccept = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default', { accept: 'application/json' });
  check('Accept: application/json 返回 JSON', byAccept.json?.type, 'text');
}

console.log('\n── E. 房间隔离：不同房间互不可见 ──');
{
  const { env } = makeEnv();
  await postJson(RawUploadHandler.upload, env, '/upload/raw?room=roomA', 'A 房的内容');
  const inA = await getJson(ContentHandler.getLatest, env, '/content/latest?room=roomA&json=1');
  check('A 房看得到', inA.json.content, 'A 房的内容');
  const inB = await getJson(ContentHandler.getLatest, env, '/content/latest?room=roomB&json=1');
  check('B 房看不到（404）', inB.status, 404);
}

console.log('\n── F. 未认证时拿不到内容 ──');
{
  const { env } = makeEnv();
  await postJson(RawUploadHandler.upload, env, '/upload/raw?room=default', 'secret');
  const r = await getJson(ContentHandler.getLatest, env, '/content/latest?room=default&json=1', { auth: 'Bearer wrong' });
  check('非 200', r.status !== 200, true);
}

summary('Receive 链路在 Worker 上可用');
