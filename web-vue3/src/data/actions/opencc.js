// 简繁转换。
//
// ⚠️ **这个模块必须走动态 import**：opencc-js 带词典，装完 6MB（打包后也不小），
// 而简繁转换是低频动作 —— 放进主包是明显的浪费。

import { Converter } from 'opencc-js';

// ⚠️ Converter 实例要**缓存**：它内部要建词典索引，每次 run 都新建会明显卡顿。
// 模块级变量就够（ESM 保证模块体只执行一次），不必再套一层 Map。
let toSimplifiedConverter = null;
let toTraditionalConverter = null;

/** 繁体 → 简体。 */
export function toSimplified(text) {
    if (!toSimplifiedConverter) {
        toSimplifiedConverter = Converter({ from: 'tw', to: 'cn' });
    }
    return toSimplifiedConverter(String(text || ''));
}

/** 简体 → 繁体。 */
export function toTraditional(text) {
    if (!toTraditionalConverter) {
        toTraditionalConverter = Converter({ from: 'cn', to: 'tw' });
    }
    return toTraditionalConverter(String(text || ''));
}
