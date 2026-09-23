// 资源层路由契约检查。
//
// 为什么需要这个测试：/file/* 与 /content/* 是给「浏览器直连」用的
// （点下载链接、打开分享链接），而浏览器发出的这些请求带 Sec-Fetch-Mode: navigate，
// 属于「导航请求」。只要 [assets] 里写了 not_found_handling = "single-page-application"，
// 资源层就会抢在 Worker 前面把这些请求回成 index.html，
// 表现是「下载文件得到一份 html」。前端 XHR 打 /server、/text 是非导航请求，
// 所以只靠手工点页面测不出来 —— 必须由测试盯住这份配置。
//
// 这里不启动 wrangler（沿用 run.sh「不联网、不需要 wrangler」的约定），
// 只断言配置与 Worker 兜底路由这两件事同时成立。
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { makeChecker } from './harness.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..');
const check = makeChecker();

// wrangler.toml 被 .gitignore 忽略，模板才是进仓库的那份；本地两份都在时都要查。
const configs = ['wrangler.toml.template', 'wrangler.toml']
  .map(name => ({ name, path: join(root, name) }))
  .filter(c => existsSync(c.path));

check.check('至少找到一份 wrangler 配置', configs.length > 0, true);

for (const { name, path } of configs) {
  const text = readFileSync(path, 'utf8');

  check.check(
    `${name}: not_found_handling 必须是 none`,
    /^\s*not_found_handling\s*=\s*"none"\s*$/m.test(text),
    true,
  );

  // run_worker_first 的数组形式里，未匹配的路径同样不会回落到 Worker，
  // 必须把所有接口前缀列全，漏一个就静默变 html。别走这条路。
  check.check(
    `${name}: 不能用 run_worker_first 白名单`,
    /^\s*run_worker_first\s*=/m.test(text),
    false,
  );
}

const index = readFileSync(join(root, 'src/index.js'), 'utf8');
// 读外壳那一步抽到了 spa-shell.js（分享落地页和兜底路由共用同一份注入逻辑），
// 所以「怎么读」的断言跟着看那边。
const spaShell = readFileSync(join(root, 'src/spa-shell.js'), 'utf8');

// Worker 必须自己兜底回前端外壳，否则 SPA 深链会变成 500（路由没命中时返回 undefined）。
check.check('Worker 注册了 catch-all', /router\.all\(\s*'\*'/.test(index), true);
check.check('兜底走 readShellHtml 取外壳', /readShellHtml\(/.test(index), true);
check.check('读外壳走 env.ASSETS', /env\.ASSETS\.fetch\(/.test(spaShell), true);
check.check('读的是 /index.html', /'\/index\.html'/.test(spaShell), true);
// history 路由下深路径刷新会落到兜底：不注入 <base> 的话相对资源全 404（白屏）。
check.check('兜底注入 <base>', /injectShellTags\(/.test(index), true);

// catch-all 必须在所有具名路由之后注册，否则会把它们全吃掉。
const catchAllAt = index.indexOf("router.all('*'");
const lastNamedAt = Math.max(
  index.indexOf("router.get('/health'"),
  index.indexOf("router.get('/push'"),
  index.indexOf("router.get('/file/:uuid"),
);
check.check('catch-all 注册在具名路由之后', catchAllAt > lastNamedAt && lastNamedAt > 0, true);

check.summary('静态资源路由契约');
