// highlight.js 的语言注册表。
//
// ⚠️ **只有这个模块是被 `import()` 动态加载的**（见 `src/highlight.js` 的 `loadHighlighter`），
// 所以它和它引的语言包会单独打成一个 chunk —— 不预览代码的人不为它付流量。
//
// 要加语言就在**这一处**加：`import` + 下面 `LANGUAGES` 里挂上 + `src/highlight.js` 的
// 扩展名映射表里登记。三处一起改，别在组件里各引一份（那段判型正则抄了 7 份的教训）。
import hljs from 'highlight.js/lib/core';

import bash from 'highlight.js/lib/languages/bash';
import c from 'highlight.js/lib/languages/c';
import cpp from 'highlight.js/lib/languages/cpp';
import css from 'highlight.js/lib/languages/css';
import diff from 'highlight.js/lib/languages/diff';
import dockerfile from 'highlight.js/lib/languages/dockerfile';
import dos from 'highlight.js/lib/languages/dos';
import go from 'highlight.js/lib/languages/go';
import ini from 'highlight.js/lib/languages/ini';
import java from 'highlight.js/lib/languages/java';
import javascript from 'highlight.js/lib/languages/javascript';
import json from 'highlight.js/lib/languages/json';
import kotlin from 'highlight.js/lib/languages/kotlin';
import less from 'highlight.js/lib/languages/less';
import markdown from 'highlight.js/lib/languages/markdown';
import php from 'highlight.js/lib/languages/php';
import powershell from 'highlight.js/lib/languages/powershell';
import protobuf from 'highlight.js/lib/languages/protobuf';
import python from 'highlight.js/lib/languages/python';
import ruby from 'highlight.js/lib/languages/ruby';
import rust from 'highlight.js/lib/languages/rust';
import scss from 'highlight.js/lib/languages/scss';
import sql from 'highlight.js/lib/languages/sql';
import swift from 'highlight.js/lib/languages/swift';
import typescript from 'highlight.js/lib/languages/typescript';
import xml from 'highlight.js/lib/languages/xml';
import yaml from 'highlight.js/lib/languages/yaml';

const LANGUAGES = {
    bash,
    c,
    cpp,
    css,
    diff,
    dockerfile,
    dos,
    go,
    ini,
    java,
    javascript,
    json,
    kotlin,
    less,
    markdown,
    php,
    powershell,
    protobuf,
    python,
    ruby,
    rust,
    scss,
    sql,
    swift,
    typescript,
    xml,
    yaml,
};

for (const [name, definition] of Object.entries(LANGUAGES)) {
    hljs.registerLanguage(name, definition);
}

export default hljs;
