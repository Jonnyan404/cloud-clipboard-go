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

/**
 * 向服务端申请分享链接。受保护房间会返回带短期 token 的 URL。
 */
export async function createShareLink(vm, { type, id, uuid, ttl } = {}) {
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
    if (ttl) {
        body.ttl = ttl;
    }

    const response = await vm.$http.post('share', body, { params });
    return response.data;
}