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
export function buildCleanAbsoluteRouteUrl(vm, path) {
    const normalizedPath = String(path || '').replace(/^\/+/, '');
    const baseURL = vm?.$http?.defaults?.baseURL || '';

    if (baseURL) {
        return new URL(normalizedPath, `${baseURL.replace(/\/+$/, '')}/`).toString();
    }

    const prefix = vm?.$root?.config?.server?.prefix || '';
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
export async function createShareLink(vm, { type, id, uuid, ttl, maxUses } = {}) {
    const room = vm?.$root?.room || '';
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

    const response = await vm.$http.post('share', body, { params });
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
