import axios from 'axios';
import { marked } from 'marked';
import DOMPurify from 'dompurify';

export function prettyFileSize(size) {
    let units = ['TB', 'GB', 'MB', 'KB'];
    let unit = 'Bytes';
    while (size >= 1024 && units.length) {
        size /= 1024;
        unit = units.pop();
    };
    return `${Math.floor(100 * size) / 100} ${unit}`;
}

export function percentage(value, decimal = 2) {
    return (value * 100).toFixed(decimal) + '%';
}

export function formatTimestamp(timestamp) {
    if (!timestamp) return '';
    let date = new Date(timestamp * 1000);
    // 返回更详细的日期和时间格式，例如: YYYY-MM-DD HH:mm:ss
    return date.toLocaleString(undefined, { // 使用浏览器的默认 locale
        year: 'numeric', month: '2-digit', day: '2-digit',
        hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false // 使用 24 小时制
    });
};

/**
 * 构建不带房间密码的绝对 URL。
 * 分享/下载鉴权应使用服务端签发的短期 token（?t=），而不是 ?auth= 房间密码。
 */
export function buildCleanAbsoluteRouteUrl(path, prefix = '') {
    const normalizedPath = String(path || '').replace(/^\/+/, '');
    return new URL(`${prefix}/${normalizedPath}`, `${window.location.origin}/`).toString();
}

/** 分享链接默认/约束（秒） */
export const SHARE_DEFAULT_TTL = 15 * 60; // 15 分钟
export const SHARE_MIN_TTL = 60; // 1 分钟
export const SHARE_MAX_TTL = 24 * 60 * 60; // 24 小时
export const SHARE_TTL_STEP = 60; // 滑块步进：1 分钟
export const SHARE_MAX_USES_LIMIT = 1000;

/** 分钟 <-> 秒，供 UI 滑块使用 */
export const SHARE_DEFAULT_TTL_MINUTES = Math.floor(SHARE_DEFAULT_TTL / 60);
export const SHARE_MIN_TTL_MINUTES = Math.floor(SHARE_MIN_TTL / 60);
export const SHARE_MAX_TTL_MINUTES = Math.floor(SHARE_MAX_TTL / 60);

export function normalizeShareTTL(ttl) {
    const value = Number(ttl);
    if (!Number.isFinite(value) || value <= 0) {
        return SHARE_DEFAULT_TTL;
    }
    if (value < SHARE_MIN_TTL) {
        return SHARE_MIN_TTL;
    }
    if (value > SHARE_MAX_TTL) {
        return SHARE_MAX_TTL;
    }
    return Math.floor(value);
}

export function minutesToShareTTL(minutes) {
    const mins = Number(minutes);
    if (!Number.isFinite(mins)) {
        return SHARE_DEFAULT_TTL;
    }
    return normalizeShareTTL(Math.round(mins) * 60);
}

export function shareTTLToMinutes(ttlSeconds) {
    return Math.max(
        SHARE_MIN_TTL_MINUTES,
        Math.min(SHARE_MAX_TTL_MINUTES, Math.round(normalizeShareTTL(ttlSeconds) / 60)),
    );
}

/**
 * 将秒数格式化为可读时长。
 * 需要传入 i18n t 函数：t(key, params)
 */
export function formatShareDuration(seconds, t) {
    const total = normalizeShareTTL(seconds);
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    if (hours > 0 && minutes > 0) {
        return t('shareDurationHoursMinutes', { hours, minutes });
    }
    if (hours > 0) {
        return t('shareDurationHours', { hours });
    }
    return t('shareDurationMinutes', { minutes: Math.max(1, minutes) });
}

/** 0 = 不限次数 */
export function normalizeShareMaxUses(maxUses) {
    const value = Number(maxUses);
    if (!Number.isFinite(value) || value <= 0) {
        return 0;
    }
    if (value > SHARE_MAX_USES_LIMIT) {
        return SHARE_MAX_USES_LIMIT;
    }
    return Math.floor(value);
}

/**
 * 向服务端申请分享链接。受保护房间会返回带短期 token 的 URL。
 * @param {{type:string,id?:string|number,uuid?:string,ttl?:number,maxUses?:number}} options
 */
export async function createShareLink({ type, id, uuid, ttl, maxUses, room } = {}) {
    const params = new URLSearchParams();
    if (room) {
        params.set('room', room);
    }

    const body = { type };
    if (id !== undefined && id !== null && id !== '') {
        body.id = String(id);
    }
    if (uuid) {
        body.uuid = uuid;
    }
    if (ttl !== undefined && ttl !== null && ttl !== '') {
        body.ttl = normalizeShareTTL(ttl);
    }
    if (maxUses !== undefined && maxUses !== null && maxUses !== '') {
        const uses = normalizeShareMaxUses(maxUses);
        if (uses > 0) {
            body.maxUses = uses;
        }
    }

    const response = await axios.post('share', body, { params });
    return response.data;
}

export function copyTextToClipboard(textToCopy) {
    if (navigator.clipboard && window.isSecureContext) {
        return navigator.clipboard.writeText(textToCopy);
    }
    return new Promise((resolve, reject) => {
        try {
            const textArea = document.createElement('textarea');
            textArea.value = textToCopy;
            textArea.style.position = 'absolute';
            textArea.style.left = '-9999px';
            document.body.appendChild(textArea);
            textArea.select();
            const successful = document.execCommand('copy');
            document.body.removeChild(textArea);
            if (successful) {
                resolve();
            } else {
                reject(new Error('execCommand copy failed'));
            }
        } catch (err) {
            reject(err);
        }
    });
}

const CLIENT_ID_KEY = 'ccgDeviceId';

export function getClientId() {
    try {
        let id = localStorage.getItem(CLIENT_ID_KEY);
        if (!id) {
            id = (globalThis.crypto && typeof crypto.randomUUID === 'function')
                ? crypto.randomUUID()
                : `ccg-${Date.now()}-${Math.random().toString(16).slice(2)}`;
            localStorage.setItem(CLIENT_ID_KEY, id);
        }
        return id;
    } catch (err) {
        return '';
    }
}

// 设备显示名：客户端声明的 name 优先，没有才回落 UA 推断出的 os / type。
// 快捷指令、curl 这类 UA 认不出来的来源，就是靠 name 显示成可读的名字。
export function deviceLabel(senderDevice, fallback = '') {
    if (!senderDevice) {
        return fallback;
    }
    return senderDevice.name || senderDevice.os || senderDevice.type || fallback;
}

/**
 * 从 axios 错误里取出「给人看」的文案。
 *
 * 服务端统一返回 { code, error, message }：message 是中文人话，error 是英文人话。
 * 优先用 message，退回 error。老服务端或反向代理（Cloudflare 502 之类）返回的可能是
 * 纯文本甚至 HTML，那就截断后原样兜出来 —— 总比只显示一句泛化的「失败」强。
 *
 * 之前的写法是直接读 data.msg，而服务端从来不返回 msg 字段，所以这段逻辑一直是死的，
 * 前端永远只显示泛化提示。
 */
export function errorMessage(error) {
    const data = error?.response?.data;
    if (!data) return '';
    if (typeof data === 'string') return data.trim().slice(0, 200);
    return data.message || data.error || '';
}

/**
 * 内容像不像 markdown。
 *
 * 为什么要判断而不是无脑渲染：剪贴板里绝大多数是普通文本，而 markdown 的标记跟日常
 * 符号高度重合 —— `5 * 3 = 15` 会被渲染成斜体、`1. 打开设置` 会被当成有序列表。
 * 所以宁可漏判：漏判的代价是用户看到原文，误判的代价是内容被改形。
 */
export function looksLikeMarkdown(text) {
    const s = String(text || '');
    // 太长不渲染：一个几万字的条目渲染一次就够列表卡一下了
    if (!s.trim() || s.length > 20000) return false;
    return /(^|\n)\s{0,3}(#{1,6}\s|>\s|[-*+]\s|\d+\.\s|```)/.test(s)
        || /\[[^\]]+\]\([^)\s]+\)/.test(s)                    // [文字](链接)
        || /\*\*[^\s][^*]*\*\*|__[^\s][^_]*__/.test(s)         // 粗体
        || /`[^`\n]+`/.test(s);                                  // 行内代码
}

/**
 * 把 markdown 渲染成可以安全插进 DOM 的 HTML。
 *
 * **必须清洗**：内容可能是别人发过来的，`<img src=x onerror=...>` 这类注入是真实风险
 * —— 剪贴板本身就是个「别人能往你这里塞字符串」的通道。DOMPurify 默认配置会去掉
 * script、事件属性、javascript: 这类 URL。
 */
export function renderMarkdownHtml(text) {
    const html = marked.parse(String(text || ''), { breaks: true, gfm: true });
    return DOMPurify.sanitize(html, { USE_PROFILES: { html: true } });
}
