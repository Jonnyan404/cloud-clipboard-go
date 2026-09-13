<script setup>import { computed, ref, watch } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import {
    formatTimestamp,
    prettyFileSize,
    buildCleanAbsoluteRouteUrl,
    createShareLink,
    copyTextToClipboard,
    SHARE_DEFAULT_TTL,
} from '@/util.js';
import PageToolbar from '@/components/PageToolbar.vue';
import StickyComposer from '@/components/sticky/StickyComposer.vue';
import { useStickyAutoscroll } from '@/composables/useStickyAutoscroll';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();

const items = computed(() => app.received);
const streamItems = computed(() => [...app.received].reverse());
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

const isOwnBubble = (item) => Boolean(item?.senderIP && item.senderIP === ownIp.value);

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
    if (/\.(png|jpe?g|gif|webp|svg|bmp|ico|avif)$/i.test(name)) {
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

const needsShareProtection = computed(() => Boolean(app.config?.auth));

async function ensureFileShareUrl(item) {
    const cache = item?.cache;
    if (!cache) {
        return '';
    }
    if (!needsShareProtection.value) {
        const encodedFilename = encodeURIComponent(item.name || 'file');
        return buildCleanAbsoluteRouteUrl(`file/${cache}/${encodedFilename}`, app.config?.server?.prefix || '');
    }
    const data = await createShareLink({ type: 'file', uuid: cache, ttl: SHARE_DEFAULT_TTL, maxUses: 0, room: ws.room });
    return data?.url || '';
}

async function downloadFile() {
    if (expired.value || downloading.value) {
        return;
    }
    downloading.value = true;
    try {
        const url = await ensureFileShareUrl(detailItem.value);
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
        const url = await ensureFileShareUrl(item);
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

async function copyFileLink(item) {
    try {
        const url = item.cache ? await ensureFileShareUrl(item) : contentUrlOf(item);
        await copyTextToClipboard(url);
        toast(t('copySuccess'));
    } catch (err) {
        console.error('复制失败:', err);
        toast(t('copyFailedGeneral'));
    }
}

function contentUrlOf(item) {
    const roomQuery = ws.room ? `?room=${encodeURIComponent(ws.room)}` : '';
    return buildCleanAbsoluteRouteUrl(`content/${item.id}${roomQuery}`, app?.config?.server?.prefix || '');
}

async function deleteItem(item) {
    try {
        await axios.delete(`revoke/${item.id}`, {
            params: new URLSearchParams([['room', ws.room]]),
        });
        toast(t('deleteSuccessText', { name: item.name }));
    } catch (error) {
        if (error.response && error.response.data.msg) {
            toast(t('deleteFailedMessageMsg', { msg: error.response.data.msg }));
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
            srcPreview.value = await ensureFileShareUrl(detailItem.value);
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
            if (error.response && error.response.data.msg) {
                toast(t('fileFetchFailedMsg', { msg: error.response.data.msg }));
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
            if (error.response && error.response.data.msg) {
                toast(t('fileFetchFailedMsg', { msg: error.response.data.msg }));
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
                    <div v-else class="chat-wall__text">{{ decodedContent(item) }}</div>
                    <span class="chat-wall__bubble-time">
                        {{ shortTime(item) }} · {{ item.type === 'text' ? t('chatTypeText') : t('chatTypeFile') }}<template v-if="isOwnBubble(item)"> · {{ t('chatSynced') }}</template><template v-if="item.senderIP"> · {{ item.senderIP }}</template>
                    </span>
                    <span class="chat-wall__bubble-ops">
                        <button v-if="item.type === 'text'" type="button" class="chat-wall__op" :title="t('copyText')" @click="copyContent(item)">
                            <v-icon size="x-small">mdi-content-copy</v-icon>
                        </button>
                        <button v-if="item.type === 'file'" type="button" class="chat-wall__op" :title="isItemExpired(item) ? t('expired') : t('download')" @click="downloadItem(item)">
                            <v-icon size="x-small">mdi-download</v-icon>
                        </button>
                        <button type="button" class="chat-wall__op chat-wall__op--danger" :title="t('delete')" @click="deleteItem(item)">
                            <v-icon size="x-small">mdi-delete-outline</v-icon>
                        </button>
                    </span>
                </div>
                <div class="chat-wall__day">{{ t('chatToday') }} · {{ timeLabel(streamItems[streamItems.length - 1]) }}</div>
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
                    <v-btn variant="text" size="small" @click="copyFileLink(detailItem)">
                        <v-icon start size="small">mdi-link-variant</v-icon>{{ t('copyLink') }}
                    </v-btn>
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

@media (max-width: 768px) {
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