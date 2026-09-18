// 房间鉴权在 Worker 上的两条硬约束：
//
//  A. /file/ 按「文件自己记录的房间」鉴权，不看客户端传的 ?room=。
//     客户端可以随便写 ?room=，若被信任，`?room=default` 就能把受保护房间的文件读出来。
//     （Go 侧曾真的如此，见 cloud-clip/lib/auth.go 的 inferRequestRoom。）
//
//  B. 会话令牌的签名密钥材料必须包含房间密码。
//     parseRoomAuth 返回 {password, fileExpire} 对象，写成 normalizeAuthValue(roomAuth[room])
//     会 String() 成 '[object Object]' —— 只设房间密码时密钥完全可预测，
//     任何人都能自己签一个 scope=global 的令牌绕过所有房间密码。
//
//  C. 受保护房间下，客户端拿到的 url 必须能靠 ?auth= 自己取到 ——
//     Android 快捷指令的第二步下载就靠这个（第一次请求的凭据不会跟着走）。
import { FileHandler } from './.build/file.mjs';
import { ContentHandler } from './.build/content.mjs';
import { issueRoomSessionToken, validateRoomSessionToken } from './.build/auth.mjs';
import { makeEnv, makeChecker, getJson } from './harness.mjs';

const { check, summary } = makeChecker();

const PNG = new Uint8Array([
  0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
  0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
]);

async function uploadTo(env, db, room, filename, { auth }) {
  const form = new FormData();
  form.append('file', new Blob([PNG], { type: 'image/png' }), filename);
  const headers = {};
  if (auth) headers.Authorization = `Bearer ${auth}`;
  const res = await FileHandler.upload(
    new Request(`http://worker.local/upload?room=${encodeURIComponent(room)}`, {
      method: 'POST', headers, body: form,
    }),
    env,
  );
  const json = await res.json();
  // 注意 /upload 返回的 url 是 /content/<id>（和 Go 侧一致），拿不到文件 uuid —— 从库里读。
  const uuid = db.prepare('SELECT uuid FROM messages WHERE id = ?').get(Number(json.id))?.uuid;
  return { ...json, uuid };
}

// itty-router 的 params 由路由层塞进去，测试里手工挂上
function downloadRequest(url, params) {
  return Object.assign(new Request(url), { params });
}

console.log('\n── A. /file/ 的房间来自文件本身，?room= 伪造无效 ──');
{
  const { env, db } = makeEnv();
  env.AUTH_PASSWORD = '';                                   // 只设房间密码，逼出这条路径
  env.ROOM_AUTH_JSON = JSON.stringify({ vault: 'vaultpw' });

  const up = await uploadTo(env, db, 'vault', 'secret.png', { auth: 'vaultpw' });
  const uuid = up.uuid;
  check('上传到 vault 成功', typeof uuid, 'string');

  const spoof = await FileHandler.download(
    downloadRequest(`http://worker.local/file/${uuid}/secret.png?room=default`, { uuid, filename: 'secret.png' }),
    env,
  );
  check('谎报 ?room=default 必须 401', spoof.status, 401);
  check('不得吐出字节', (await spoof.text()).includes('PNG'), false);

  const emptyRoom = await FileHandler.download(
    downloadRequest(`http://worker.local/file/${uuid}/secret.png?room=`, { uuid, filename: 'secret.png' }),
    env,
  );
  check('谎报空 room 必须 401', emptyRoom.status, 401);

  const noCreds = await FileHandler.download(
    downloadRequest(`http://worker.local/file/${uuid}/secret.png?room=vault`, { uuid, filename: 'secret.png' }),
    env,
  );
  check('不传凭据也必须 401', noCreds.status, 401);

  const ok = await FileHandler.download(
    downloadRequest(`http://worker.local/file/${uuid}/secret.png?room=vault&auth=vaultpw`, { uuid, filename: 'secret.png' }),
    env,
  );
  check('房间密码正确才放行', ok.status, 200);
}

console.log('\n── B. 会话令牌的签名密钥必须含房间密码 ──');
{
  const { env: env1 } = makeEnv();
  env1.AUTH_PASSWORD = '';
  env1.ROOM_AUTH_JSON = JSON.stringify({ vault: 'pw-one' });

  const { env: env2 } = makeEnv();
  env2.AUTH_PASSWORD = '';
  env2.ROOM_AUTH_JSON = JSON.stringify({ vault: 'pw-two' });

  const token = await issueRoomSessionToken(env1, 'vault');
  check('同配置下令牌有效', await validateRoomSessionToken(env1, 'vault', token), true);
  // 房间密码变了 → 密钥材料变了 → 旧令牌必须失效。
  // 若这里仍是 true，说明房间密码根本没参与派生（'[object Object]' 的老毛病）。
  check('换掉房间密码后旧令牌失效', await validateRoomSessionToken(env2, 'vault', token), false);
}

console.log('\n── C. 受保护房间：/content/latest 的 url 能靠 ?auth= 自己取到 ──');
{
  const { env, db } = makeEnv();
  env.AUTH_PASSWORD = 'pw123';
  env.ROOM_AUTH_JSON = '{}';

  const up = await uploadTo(env, db, 'default', 'shot.png', { auth: 'pw123' });

  const noAuth = await getJson(ContentHandler.getLatest, env, '/content/latest?json=1&room=default', { auth: null });
  check('不带凭据读元数据 401', noAuth.status, 401);

  const withAuth = await getJson(ContentHandler.getLatest, env, '/content/latest?json=1&room=default&auth=pw123', { auth: null });
  check('带 ?auth= 读元数据 200', withAuth.status, 200);
  check('url 形态完好（不能出现 http:/ 这种少一个斜杠）', withAuth.json.url.startsWith('http://worker.local/file/'), true);
  check('url 带上了文件名', withAuth.json.url.endsWith('/shot.png'), true);

  // 第二步：客户端照 url 去取。Android 快捷指令就是在这里漏了 auth。
  const uuid = up.uuid;
  const step2NoAuth = await FileHandler.download(
    downloadRequest(`http://worker.local/file/${uuid}/shot.png`, { uuid, filename: 'shot.png' }),
    env,
  );
  check('第二步不带凭据 → 401（这就是用户报的现象）', step2NoAuth.status, 401);

  const step2WithAuth = await FileHandler.download(
    downloadRequest(`http://worker.local/file/${uuid}/shot.png?auth=pw123`, { uuid, filename: 'shot.png' }),
    env,
  );
  check('第二步补上 ?auth= → 200', step2WithAuth.status, 200);
}

summary('房间鉴权在 Worker 上成立');
