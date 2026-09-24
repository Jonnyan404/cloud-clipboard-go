<script setup>import { computed, reactive, ref, watch } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import { SHARE_DEFAULT_TTL, copyTextToClipboard, createShareLink, deviceLabel, errorMessage, formatTimestamp, getClientId, isAutomationMessage, isImageName, isLateMessage, looksLikeMarkdown, prefersRenderedView, renderMarkdownHtml, prettyFileSize } from '@/util.js';
import PageToolbar from '@/components/PageToolbar.vue';
import StickyComposer from '@/components/sticky/StickyComposer.vue';
import ShareLinkButton from '@/components/ShareLinkButton.vue';
import MarkdownBody from '@/components/MarkdownBody.vue';
import { useStickyAutoscroll } from '@/composables/useStickyAutoscroll';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();

// 聊天气泡的 md 渲染 —— 与标准 / 便签**同一套语义**，三条一起看：
//
//   · 个性化里那个开关（panel 上叫「动作图标」，存储键 `app.display.markdown`）只决定
//     **那排动作图标显不显示**；关掉它，图标和渲染视图一起消失，气泡一律回纯文本。
//     它**不决定**默认看哪一份。
//   · 每条气泡默认看哪一份由**内容**决定 —— 任务列表 / 表格默认渲染，其余默认原文。
//     判断复用 util.js 的 prefersRenderedView（标准 / 便签走的是同一个函数）。
//   · 用户在气泡上点一次图标 = 覆盖这一条的默认值（存在 mdOverrides，只记点过的）。
//
// ⚠️ 聊天以前是**全站唯一默认渲染**的地方（气泡无条件渲染 md），Jonny 要求统一成
// 跟标准 / 便签一致 —— 所以现在普通 markdown 气泡默认是原文，点一下才渲染。
// 别再照抄旧的「气泡默认渲染」，语义的集中说明在 data/displayToggles.js 里 markdown 那条。
//
// 渲染结果按 id 先算成一张表，模板里按 id 取，避免同一条渲染两遍。
const mdOverrides = reactive(new Map());
function setBubbleMd(id, mode) {
    mdOverrides.set(id, mode);
}
// 每条文本气泡的正文（HTML 实体解码）。解码要建个 textarea，不便宜 ——
// 模板里的图标、标题、渲染表三处都要用，所以按 id 算一次共享。
const bubbleText = computed(() => {
    const map = new Map();
    for (const item of app.visibleReceived) {
        if (item.type === 'text') map.set(item.id, decodedContent(item));
    }
    return map;
});
function bubbleMdMode(item) {
    const override = mdOverrides.get(item.id);
    if (override) return override;
    return prefersRenderedView(bubbleText.value.get(item.id) || '') ? 'md' : 'raw';
}
const bubbleHtml = computed(() => {
    const map = new Map();
    if (!app.display.markdown) return map;
    for (const item of app.visibleReceived) {
        if (item.type !== 'text') continue;
        if (bubbleMdMode(item) !== 'md') continue;
        const text = bubbleText.value.get(item.id) || '';
        if (looksLikeMarkdown(text)) map.set(item.id, renderMarkdownHtml(text));
    }
    return map;
});
// 内容不像 markdown 时不给图标（跟 useMarkdown 的 available 同一条判断）
function bubbleMdAvailable(item) {
    return app.display.markdown && item.type === 'text' && looksLikeMarkdown(bubbleText.value.get(item.id) || '');
}
const mdiCodeTags = 'mdi-code-tags';
const mdiLanguageMarkdown = 'mdi-language-markdown';

const items = computed(() => app.visibleReceived);
const streamItems = computed(() => [...app.visibleReceived].reverse());
const streamEl = ref(null);
const { pinToBottom } = useStickyAutoscroll(streamEl, {
    items: () => [...streamItems.value],
    room: () => ws.room,
});

const ownIp = ref(localStorage.getItem('ccgMyIp') || '');
async function resolveOwnIp() {
    if (ownIp.value) {
        return;
    }
    try {
        const response = await axios.get('myip', {
            params: new URLSearchParams([['room', ws.room]]),
        });
        if (response.data && response.data.ip) {
            ownIp.value = response.data.ip;
            localStorage.setItem('ccgMyIp', ownIp.value);
        }
    } catch (error) {
        console.error('获取本机IP失败:', error);
    }
}
resolveOwnIp();

const myClientId = getClientId();
// 优先按服务端回传的 senderClientID 归类收发;老消息(无该字段)回退到 IP 判断
const isOwnBubble = (item) => Boolean(
    (item?.senderClientID && item.senderClientID === myClientId)
    || (!item?.senderClientID && item?.senderIP && item.senderIP === ownIp.value),
);

const countLabel = computed(() => t('uiModeChatCount', { count: items.value.length }));

const detailItem = ref(null);
const downloading = ref(false);
const previewLoading = ref(false);
const srcPreview = ref(null);
const textPreview = ref('');
const textPreviewDisplayLimit = 16 * 1024;
const showFullTextPreview = ref(false);
const expired = computed(() => {
    if (!detailItem.value?.expire || detailItem.value.expire <= 0) {
        return false;
    }
    return Date.now() / 1000 > detailItem.value.expire;
});
const isExpirable = computed(() => Boolean(detailItem.value?.expire && detailItem.value.expire > 0));
const expireLabel = computed(() => {
    if (!isExpirable.value) {
        return '';
    }
    return `${expired.value ? t('expired') : t('expiresAt', { time: formatTimestamp(detailItem.value.expire) })}`;
});

const isPreviewableVideo = computed(() => detailItem.value?.name?.match(/\.(mp4|webm|ogv)$/gi));
const isPreviewableAudio = computed(() => detailItem.value?.name?.match(/\.(mp3|wav|ogg|opus|m4a|flac)$/gi));
const isPreviewableText = computed(() => detailItem.value?.name?.match(/\.(txt|text|md|markdown|json|log|csv|tsv|ya?ml|xml|ini|conf|cfg|toml|properties|env|gitignore|dockerfile|js|jsx|mjs|cjs|ts|tsx|vue|css|scss|sass|less|html|htm|sql|sh|bash|zsh|fish|ps1|bat|cmd|go|py|java|kt|kts|rb|php|rs|c|cc|cpp|cxx|h|hh|hpp|hxx|swift|proto)$/gi));
const canPreview = computed(() => Boolean(detailItem.value?.type === 'file' && !expired.value && (Boolean(detailItem.value.thumbnail) || isPreviewableVideo.value || isPreviewableAudio.value || isPreviewableText.value)));
const hasTruncatedTextPreview = computed(() => textPreview.value.length > textPreviewDisplayLimit);
const displayedTextPreview = computed(() => {
    if (!hasTruncatedTextPreview.value || showFullTextPreview.value) {
        return textPreview.value;
    }
    return `${textPreview.value.slice(0, textPreviewDisplayLimit)}\n\n...`;
});

function decodedContent(item) {
    const textArea = document.createElement('textarea');
    textArea.innerHTML = item.content || '';
    return textArea.value;
}

function fileIcon(item) {
    const name = item.name || '';
    if (isImageName(name)) {
        return '🖼️';
    }
    if (/\.(mp4|webm|ogv|mov)$/i.test(name)) {
        return '🎬';
    }
    if (/\.(mp3|wav|ogg|opus|m4a|flac)$/i.test(name)) {
        return '🎵';
    }
    return '📄';
}

const timeLabel = (item) => formatTimestamp(item.timestamp);
const shortTime = (item) => {
    const date = new Date(Number(item.timestamp) * 1000 || Date.now());
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    return `${hours}:${minutes}`;
};

// 气泡页脚：按设置逐段拼，而不是把时间/IP 写死。
// 之前这里是硬编码的「时间 · 类型 · 已同步 · IP」——设备信息压根没出现，
// 时间与 IP 则不看 showTimestamp / showSenderIP，导致同一组设置在不同模式下含义不同。
// 改成数组再 join，顺带解决关掉首段后残留前导分隔符的问题。
// 类型标签不归设置管，它标的是这条气泡是文字还是文件，属于结构信息。
const bubbleFooter = (item) => {
    const parts = [];
    if (app.display.timestamp) {
        parts.push(shortTime(item));
    }
    parts.push(item.type === 'text' ? t('chatTypeText') : t('chatTypeFile'));
    // 定时消息的来源标记。和上面那个类型标签同理，**不归 app.display 那几个开关管** ——
    // 它标的是「这条不是人发的」，属于结构信息，不是可选展示的元信息。
    // 判定在 util.js（单点，那边有完整说明）。
    if (isAutomationMessage(item)) {
        parts.push(t('automationSource'));
    }
    // 补发：正文按原定时刻算、timestamp 是实际发送时刻，标出来免得用户以为内容坏了。
    if (isLateMessage(item)) {
        parts.push(t('automationLate'));
    }
    if (isOwnBubble(item)) {
        parts.push(t('chatSynced'));
    }
    if (app.display.device && item.senderDevice) {
        parts.push(deviceLabel(item.senderDevice));
    }
    if (app.display.ip && item.senderIP) {
        parts.push(item.senderIP);
    }
    return parts.join(' · ');
};


// 文件分享：一次签发拿到**两条**地址。
//   raw  直连正文 —— 下载与预览用（分享页是 hash 路由，取不了字节）
//   page 前端分享页 —— 复制给别人用
// 一律签发 token：房间没开密码也发，否则 ttl / 次数限制会被静默丢弃。
async function ensureFileShareLinks(item) {
    const cache = item?.cache;
    if (!cache) {
        return { raw: '', page: '' };
    }
    const data = await createShareLink({ type: 'file', uuid: cache, ttl: SHARE_DEFAULT_TTL, maxUses: 0, room: ws.room });
    return { raw: data?.rawUrl || '', page: data?.url || '' };
}

async function downloadFile() {
    if (expired.value || downloading.value) {
        return;
    }
    downloading.value = true;
    try {
        const { raw: url } = await ensureFileShareLinks(detailItem.value);
        const downloadUrl = new URL(url, window.location.origin);
        downloadUrl.searchParams.set('download', 'true');
        const anchor = document.createElement('a');
        anchor.href = downloadUrl.toString();
        anchor.download = detailItem.value?.name || 'file';
        anchor.rel = 'noopener';
        document.body.appendChild(anchor);
        anchor.click();
        document.body.removeChild(anchor);
    } catch (error) {
        console.error('下载失败:', error);
        toast(t('fileFetchFailed'));
    } finally {
        downloading.value = false;
    }
}

async function downloadItem(item) {
    if (isItemExpired(item) || downloading.value) {
        return;
    }
    downloading.value = true;
    try {
        const { raw: url } = await ensureFileShareLinks(item);
        const downloadUrl = new URL(url, window.location.origin);
        downloadUrl.searchParams.set('download', 'true');
        const anchor = document.createElement('a');
        anchor.href = downloadUrl.toString();
        anchor.download = item?.name || 'file';
        anchor.rel = 'noopener';
        document.body.appendChild(anchor);
        anchor.click();
        document.body.removeChild(anchor);
    } catch (error) {
        console.error('下载失败:', error);
        toast(t('fileFetchFailed'));
    } finally {
        downloading.value = false;
    }
}

async function copyContent(item) {
    try {
        await copyTextToClipboard(decodedContent(item));
        toast(t('copySuccess'));
    } catch (err) {
        console.error('复制失败:', err);
        toast(t('copyFailedGeneral'));
    }
}

// 「复制链接」这个动作已经交给 ShareLinkButton（签名 + 自动复制 + 二维码面板），这里不再自己拼地址。
// ⚠️ 工作台模式**还留着** copyFileLink —— 它卡片上那个「复制」按钮对文件走的是同一条路。
// 所以 ensureFileShareLinks 五个模式都保持返回 { raw, page }，这个文件只用到 raw。

async function deleteItem(item) {
    try {
        await axios.delete(`revoke/${item.id}`, {
            params: new URLSearchParams([['room', ws.room]]),
        });
        toast(t('deleteSuccessText', { name: item.name }));
    } catch (error) {
        const errMsg = errorMessage(error);
        if (errMsg) {
            toast(t('deleteFailedMessageMsg', { msg: errMsg }));
        } else {
            toast(t('deleteFailedMessage'));
        }
    }
}

function isItemExpired(item) {
    if (!item?.expire || item.expire <= 0) {
        return false;
    }
    return Date.now() / 1000 > item.expire;
}

async function loadPreview() {
    if (!canPreview.value) {
        return;
    }
    srcPreview.value = null;
    textPreview.value = '';
    showFullTextPreview.value = false;
    if (isPreviewableVideo.value || isPreviewableAudio.value) {
        previewLoading.value = true;
        try {
            srcPreview.value = (await ensureFileShareLinks(detailItem.value)).raw;
        } catch (error) {
            console.error('生成预览链接失败:', error);
            toast(t('fileFetchFailed'));
        } finally {
            previewLoading.value = false;
        }
    } else if (isPreviewableText.value) {
        previewLoading.value = true;
        try {
            const response = await axios.get(`file/${detailItem.value.cache}/${encodeURIComponent(detailItem.value.name)}`, {
                responseType: 'text',
            });
            textPreview.value = typeof response.data === 'string' ? response.data : String(response.data || '');
        } catch (error) {
            const errMsg = errorMessage(error);
            if (errMsg) {
                toast(t('fileFetchFailedMsg', { msg: errMsg }));
            } else {
                toast(t('fileFetchFailed'));
            }
        } finally {
            previewLoading.value = false;
        }
    } else {
        previewLoading.value = true;
        try {
            const response = await axios.get(`file/${detailItem.value.cache}/${encodeURIComponent(detailItem.value.name)}`, {
                responseType: 'arraybuffer',
            });
            srcPreview.value = URL.createObjectURL(new Blob([response.data]));
        } catch (error) {
            const errMsg = errorMessage(error);
            if (errMsg) {
                toast(t('fileFetchFailedMsg', { msg: errMsg }));
            } else {
                toast(t('fileFetchFailed'));
            }
        } finally {
            previewLoading.value = false;
        }
    }
}

function toggleTextPreview() {
    showFullTextPreview.value = !showFullTextPreview.value;
}

watch(detailItem, (item) => {
    if (item) {
        loadPreview();
    }
});
</script>

<template>
    <div
        class="chat-wall"
        :class="{ 'chat-wall--dark': isDark }"
    >
        <PageToolbar variant="chat"></PageToolbar>

        <div class="chat-wall__content">
            <div class="chat-wall__hdr">
                <div class="chat-wall__room">
                    <span class="chat-wall__room-icon">💻</span>
                    <span class="chat-wall__room-name">{{ ws.room || t('publicRoom') }}</span>
                    <span class="chat-wall__room-dot"></span>
                </div>
                <div class="chat-wall__status">
                    <span class="chat-wall__chip"><b>{{ items.length }}</b>/{{ app.config?.server?.history || '100' }}</span>
                    <span class="chat-wall__gear">⚙</span>
                </div>
            </div>

            <div v-if="items.length" ref="streamEl" class="chat-wall__stream">
                <div
                    v-for="item in streamItems"
                    :key="item.id"
                    class="chat-wall__bubble"
                    :class="isOwnBubble(item) ? 'chat-wall__bubble--out' : 'chat-wall__bubble--in'"
                >
                    <div v-if="item.type === 'file'" class="chat-wall__file" role="button" tabindex="0" @click="detailItem = item" @keydown.enter.prevent="detailItem = item">
                        <span class="chat-wall__file-icon">{{ fileIcon(item) }}</span>
                        <span class="chat-wall__file-info">
                            <span class="chat-wall__file-name">{{ item.name || 'file' }}</span>
                            <span class="chat-wall__file-meta">{{ prettyFileSize(item.size || 0) }}</span>
                        </span>
                    </div>
                    <markdown-body v-else-if="bubbleHtml.get(item.id)" :html="bubbleHtml.get(item.id)"></markdown-body>
                    <div v-else class="chat-wall__text">{{ decodedContent(item) }}</div>
                    <span class="chat-wall__bubble-time">{{ bubbleFooter(item) }}</span>
                    <span class="chat-wall__bubble-ops">
                        <!-- 这一段就是个性化里「动作图标」开关管的东西：开关关掉，
                             图标和渲染视图一起消失（bubbleMdAvailable 里带了那个开关）。
                             图标只在内容真的像 markdown 时出现；显示哪种图标标的是
                             「点一下会变成什么」—— 渲染态给代码图标（切回原文），
                             原文态给 markdown 图标。 -->
                        <button
                            v-if="bubbleMdAvailable(item)"
                            type="button"
                            class="chat-wall__op"
                            :title="bubbleMdMode(item) === 'md' ? t('rawText') : t('renderMarkdown')"
                            @click="setBubbleMd(item.id, bubbleMdMode(item) === 'md' ? 'raw' : 'md')"
                        >
                            <v-icon size="14">{{ bubbleMdMode(item) === 'md' ? mdiCodeTags : mdiLanguageMarkdown }}</v-icon>
                        </button>
                        <button v-if="item.type === 'text'" type="button" class="chat-wall__op" :title="t('copyText')" @click="copyContent(item)">
                            <v-icon size="large">mdi-content-copy</v-icon>
                        </button>
                        <button v-if="item.type === 'file'" type="button" class="chat-wall__op" :title="isItemExpired(item) ? t('expired') : t('download')" @click="downloadItem(item)">
                            <v-icon size="large">mdi-download</v-icon>
                        </button>
                        <button type="button" class="chat-wall__op chat-wall__op--danger" :title="t('delete')" @click="deleteItem(item)">
                            <v-icon size="large">mdi-delete-outline</v-icon>
                        </button>
                    </span>
                </div>
                <div class="chat-wall__day">{{ t('dateToday') }} · {{ timeLabel(streamItems[streamItems.length - 1]) }}</div>
            </div>

            <div v-else class="chat-wall__empty">
                <div class="text-h6 font-weight-medium mb-2">{{ t('emptyTimelineTitle') }}</div>
                <div class="text-body-2 text-medium-emphasis mb-4">{{ t('timelineEmptySubtitle') }}</div>
            </div>

            <div class="chat-wall__composer">
                <sticky-composer variant="chat" @sent="pinToBottom()"></sticky-composer>
            </div>
        </div>

        <v-dialog v-model="detailItem" max-width="560">
            <div v-if="detailItem" class="chat-wall__reader" :class="{ 'chat-wall__reader--dark': isDark }">
                <div class="chat-wall__reader-head">
                    <span class="chat-wall__reader-type">{{ detailItem.type.toUpperCase() }}</span>
                    <span class="chat-wall__reader-time">{{ timeLabel(detailItem) }}</span>
                    <v-btn icon density="compact" size="x-small" variant="text" class="chat-wall__op" @click="detailItem = null">
                        <v-icon size="small">mdi-close</v-icon>
                    </v-btn>
                </div>
                <div v-if="detailItem.type === 'file'" class="chat-wall__reader-file">
                    <span class="chat-wall__reader-glyph">{{ fileIcon(detailItem) }}</span>
                    <span class="chat-wall__reader-name">{{ detailItem.name }}</span>
                    <span class="chat-wall__reader-meta">{{ prettyFileSize(detailItem.size || 0) }}</span>
                </div>
                <div v-if="detailItem.type === 'file' && isExpirable" class="chat-wall__reader-expire" :class="{ 'chat-wall__reader-expire--past': expired }">
                    <v-icon size="x-small">mdi-clock-outline</v-icon>
                    {{ expireLabel }}
                </div>

                <div v-if="detailItem.type === 'file' && canPreview" class="chat-wall__reader-preview">
                    <div v-if="previewLoading" class="chat-wall__preview-loading">
                        <v-progress-circular indeterminate color="primary" size="36"></v-progress-circular>
                    </div>
                    <template v-else>
                        <video
                            v-if="isPreviewableVideo"
                            :src="srcPreview"
                            style="max-height:60vh;max-width:100%;"
                            controls
                            preload="metadata"
                        ></video>
                        <audio
                            v-else-if="isPreviewableAudio"
                            :src="srcPreview"
                            style="width:100%"
                            controls
                            preload="metadata"
                        ></audio>
                        <template v-else-if="isPreviewableText">
                            <pre class="chat-wall__preview-text">{{ displayedTextPreview }}</pre>
                            <div v-if="hasTruncatedTextPreview" class="d-flex justify-space-between align-center mt-2">
                                <div class="text-caption text-medium-emphasis">
                                    {{ t('textPreviewTruncated', { limit: prettyFileSize(textPreviewDisplayLimit) }) }}
                                </div>
                                <v-btn size="small" variant="text" color="primary" @click="toggleTextPreview">
                                    {{ showFullTextPreview ? t('collapseTextPreview') : t('expandTextPreview') }}
                                </v-btn>
                            </div>
                        </template>
                        <img
                            v-else
                            :src="srcPreview || detailItem.thumbnail"
                            style="max-height:60vh;max-width:100%;"
                        >
                    </template>
                </div>

                <div v-if="detailItem.type === 'file'" class="chat-wall__reader-actions">
                    <v-btn
                        color="primary"
                        variant="flat"
                        size="small"
                        :loading="downloading"
                        :disabled="expired"
                        @click="downloadFile"
                    >
                        <v-icon start size="small">mdi-download</v-icon>{{ expired ? t('expired') : t('download') }}
                    </v-btn>
                    <share-link-button :meta="detailItem" :icon-only="false" />
                </div>
            </div>
        </v-dialog>
    </div>
</template>

<style scoped>
.chat-wall {
    background: #f6f7fa;
    height: 100vh;
    height: 100dvh;
    display: flex;
    flex-direction: column;
    color: #111827;
    font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", sans-serif;
    -webkit-font-smoothing: antialiased;
}

.chat-wall--dark {
    background: #15171c;
    color: #e7eaf0;
}

.chat-wall > .page-toolbar {
    flex-shrink: 0;
}

.chat-wall__content {
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: 720px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    padding: 0 16px 16px;
}

.chat-wall__hdr {
    padding: 12px 2px 8px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-shrink: 0;
}

.chat-wall__room {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 14px;
    font-weight: 650;
    min-width: 0;
}

.chat-wall__room-icon {
    width: 26px;
    height: 26px;
    border-radius: 50%;
    background: #dbeafe;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    flex-shrink: 0;
}

.chat-wall--dark .chat-wall__room-icon {
    background: #232a35;
}

.chat-wall__room-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.chat-wall__room-dot {
    width: 7px;
    height: 7px;
    background: #22c55e;
    border-radius: 50%;
    flex-shrink: 0;
}

.chat-wall__status {
    font-size: 11px;
    color: #9ca3af;
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
}

.chat-wall__chip {
    background: #fff;
    border: 1px solid #e5e7eb;
    border-radius: 999px;
    padding: 2px 9px;
    font-size: 11px;
    color: #64748b;
}

.chat-wall--dark .chat-wall__chip {
    background: #1d2128;
    border-color: #2b3138;
    color: #9aa3ad;
}

.chat-wall__chip b {
    font-weight: 700;
}

.chat-wall__gear {
    cursor: pointer;
    opacity: 0.7;
}

.chat-wall__stream {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 4px 2px 10px;
    display: flex;
    flex-direction: column;
    gap: 9px;
}

.chat-wall__day {
    text-align: center;
    font-size: 10px;
    color: #c3c9d0;
    margin: 2px 0;
    flex-shrink: 0;
}

.chat-wall--dark .chat-wall__day {
    color: #5b6470;
}

.chat-wall__bubble {
    max-width: 78%;
    padding: 10px 13px;
    border-radius: 14px;
    font-size: 13px;
    line-height: 1.5;
    position: relative;
    word-break: break-word;
    overflow-wrap: anywhere;
}

.chat-wall__bubble--in {
    align-self: flex-start;
    background: #fff;
    border: 1px solid #eef0f3;
    border-bottom-left-radius: 5px;
}

.chat-wall--dark .chat-wall__bubble--in {
    background: #1d2128;
    border-color: #2b3138;
    color: #e7eaf0;
}

.chat-wall__bubble--out {
    align-self: flex-end;
    background: #1e88e5;
    color: #fff;
    border-bottom-right-radius: 5px;
}

.chat-wall__bubble--out .chat-wall__file-icon {
    background: rgba(255, 255, 255, 0.18);
}

.chat-wall__bubble--out .chat-wall__file-name,
.chat-wall__bubble--out .chat-wall__bubble-time {
    color: #fff;
}

.chat-wall__bubble--out .chat-wall__op {
    color: rgba(255, 255, 255, 0.85);
}

.chat-wall__bubble--out .chat-wall__op:hover {
    color: #fff;
}

.chat-wall__text {
    white-space: pre-wrap;
}

.chat-wall__file {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    cursor: pointer;
}

.chat-wall__file-icon {
    width: 26px;
    height: 26px;
    border-radius: 8px;
    background: rgba(0, 0, 0, 0.06);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 13px;
    flex-shrink: 0;
}

.chat-wall--dark .chat-wall__file-icon {
    background: rgba(255, 255, 255, 0.1);
}

.chat-wall__file-info {
    display: flex;
    flex-direction: column;
    min-width: 0;
}

.chat-wall__file-name {
    font-weight: 650;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.chat-wall__file-meta {
    font-size: 10px;
    opacity: 0.6;
}

.chat-wall__bubble-time {
    font-size: 9px;
    opacity: 0.6;
    margin-top: 4px;
    display: block;
    text-align: left;
    padding-right: 58px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.chat-wall__bubble-ops {
    position: absolute;
    right: 4px;
    bottom: 6px;
    display: none;
    gap: 2px;
    align-items: center;
    z-index: 2;
}

.chat-wall__bubble:hover .chat-wall__bubble-ops,
.chat-wall__bubble:focus-within .chat-wall__bubble-ops {
    display: flex;
}

/* 触屏没有 hover，气泡里的操作图标得常显。
   宽度那条是原有的（窄屏本来就该常显）；`hover: none` 补的是**宽屏触屏设备**（平板）——
   光看宽度会漏掉它们，另外三个模式的 `@media (hover: none)` 覆盖得到、聊天模式覆盖不到，
   表现就不一致了。两条并列，取并集。 */
@media (hover: none), (max-width: 768px) {
    .chat-wall__bubble-ops {
        display: flex;
    }
}

.chat-wall__op {
    background: none;
    border: none;
    cursor: pointer;
    padding: 2px;
    color: #64748b;
    display: inline-flex;
    align-items: center;
    justify-content: center;
}

.chat-wall--dark .chat-wall__op {
    color: #9aa3ad;
}

.chat-wall__op:hover {
    color: #1e88e5;
}

.chat-wall__op--danger:hover {
    color: #d32f2f;
}

.chat-wall__empty {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 24px;
    text-align: center;
}

.chat-wall__composer {
    flex-shrink: 0;
    padding-bottom: env(safe-area-inset-bottom);
}

.chat-wall__composer :deep(.sticky-composer--chat) {
    background: var(--v-theme-surface);
}

.chat-wall--dark .chat-wall__composer :deep(.sticky-composer--chat) {
    background: #1d2128;
    border-color: #2b3138;
}

.chat-wall--dark .chat-wall__composer :deep(.sticky-composer--chat .sticky-composer__area) {
    color: #e7eaf0;
}

.chat-wall--dark .chat-wall__composer :deep(.sticky-composer--chat .sticky-composer__area::placeholder) {
    color: #6d7681;
}

.chat-wall--dark .chat-wall__composer :deep(.sticky-composer--chat .sticky-composer__attach) {
    color: #828c97;
}

.chat-wall__reader {
    border-radius: 12px;
    background: #fff;
    border: 1px solid #e2e7ee;
    padding: 18px;
    color: #111827;
    font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", sans-serif;
}

.chat-wall__reader--dark {
    background: #1d2128;
    border-color: #2b3138;
    color: #e7eaf0;
}

.chat-wall__reader-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
    font-size: 12px;
    color: #64748b;
}

.chat-wall__reader-type {
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    font-size: 11px;
    color: #1e88e5;
}

.chat-wall__reader-time {
    color: #9ca3af;
    font-size: 11px;
}

.chat-wall__reader-head .chat-wall__op {
    margin-left: auto;
}

.chat-wall__reader-file {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
}

.chat-wall__reader-glyph {
    font-size: 28px;
    flex-shrink: 0;
}

.chat-wall__reader-name {
    font-size: 15px;
    font-weight: 650;
    word-break: break-all;
}

.chat-wall__reader-meta {
    font-size: 12px;
    color: #64748b;
    flex-shrink: 0;
    margin-left: auto;
}

.chat-wall__reader-expire {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: #64748b;
    background: #f1f5f9;
    border-radius: 999px;
    padding: 3px 10px;
    margin-bottom: 12px;
}

.chat-wall__reader-expire--past {
    color: #b91c1c;
    background: #fef2f2;
}

.chat-wall__reader-preview {
    margin-bottom: 12px;
}

.chat-wall__preview-loading {
    display: flex;
    justify-content: center;
    padding: 24px 0;
}

.chat-wall__preview-text {
    background: #f6f7f9;
    border: 1px solid #eef0f2;
    border-radius: 10px;
    padding: 12px;
    font-size: 12px;
    line-height: 1.6;
    overflow: auto;
    max-height: 60vh;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: 'SF Mono', 'Menlo', 'Consolas', monospace;
}

.chat-wall__reader-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
}
</style>