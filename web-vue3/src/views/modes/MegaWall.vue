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
const countLabel = computed(() => t('uiModeMegaCount', { count: items.value.length }));

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

async function copyContent(item) {
    try {
        await copyTextToClipboard(decodedContent(item));
        toast(t('copySuccess'));
    } catch (err) {
        console.error('复制失败:', err);
        toast(t('copyFailedGeneral'));
    }
}

function isItemExpired(item) {
    if (!item?.expire || item.expire <= 0) {
        return false;
    }
    return Date.now() / 1000 > item.expire;
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
        class="mega-wall"
        :class="{ 'mega-wall--dark': isDark }"
    >
        <PageToolbar variant="mega"></PageToolbar>
        <div class="mega-wall__shell mx-auto">
            <div class="mega-wall__head">
                <span>{{ t('uiModeMegaToday') }}</span>
                <span class="mega-wall__count">{{ countLabel }}</span>
            </div>

            <div v-if="items.length" ref="streamEl" class="mega-wall__stream">
                <div v-for="item in streamItems" :key="item.id" class="mega-wall__item">
                    <div class="mega-wall__kicker">
                        <span>{{ timeLabel(item) }} · {{ item.type.toUpperCase() }}</span>
                        <span class="mega-wall__ops">
                            <button v-if="item.type === 'text'" type="button" class="mega-wall__op" :title="t('copyText')" @click="copyContent(item)">
                                <v-icon size="x-large">mdi-content-copy</v-icon>
                            </button>
                            <button v-if="item.type === 'file'" type="button" class="mega-wall__op" :title="isItemExpired(item) ? t('expired') : t('download')" @click="downloadItem(item)">
                                <v-icon size="x-large">mdi-download</v-icon>
                            </button>
                            <button type="button" class="mega-wall__op mega-wall__op--danger" :title="t('delete')" @click="deleteItem(item)">
                                <v-icon size="x-large">mdi-close</v-icon>
                            </button>
                        </span>
                    </div>
                    <div v-if="item.type === 'text'" class="mega-wall__body">
                        {{ decodedContent(item) }}
                    </div>
                    <div v-else class="mega-wall__file" role="button" tabindex="0" @click="detailItem = item" @keydown.enter.prevent="detailItem = item">
                        <span class="mega-wall__file-glyph">{{ fileIcon(item) }}</span>
                        <span class="mega-wall__file-name">{{ item.name || 'file' }}</span>
                        <span class="mega-wall__file-meta">— {{ prettyFileSize(item.size || 0) }}</span>
                        <span class="mega-wall__file-open">查看 →</span>
                    </div>
                </div>
            </div>

            <div v-else class="mega-wall__empty">
                <div class="text-h6 font-weight-medium mb-2">{{ t('emptyTimelineTitle') }}</div>
                <div class="text-body-2 text-medium-emphasis mb-4">{{ t('timelineEmptySubtitle') }}</div>
            </div>

            <div class="mega-wall__composer">
                <sticky-composer variant="mega" @sent="pinToBottom()"></sticky-composer>
            </div>
        </div>

        <v-dialog v-model="detailItem" max-width="560">
            <div v-if="detailItem" class="mega-wall__reader" :class="{ 'mega-wall__reader--dark': isDark }">
                <div class="mega-wall__reader-head">
                    <span class="mega-wall__reader-type">{{ detailItem.type.toUpperCase() }}</span>
                    <span class="mega-wall__reader-time">{{ timeLabel(detailItem) }}</span>
                    <v-btn icon density="compact" size="x-small" variant="text" class="mega-wall__op" @click="detailItem = null">
                        <v-icon size="small">mdi-close</v-icon>
                    </v-btn>
                </div>
                <div v-if="detailItem.type === 'file'" class="mega-wall__reader-file">
                    <span class="mega-wall__reader-glyph">{{ fileIcon(detailItem) }}</span>
                    <span class="mega-wall__reader-name">{{ detailItem.name }}</span>
                    <span class="mega-wall__reader-meta">{{ prettyFileSize(detailItem.size || 0) }}</span>
                </div>
                <div v-if="detailItem.type === 'file' && isExpirable" class="mega-wall__reader-expire" :class="{ 'mega-wall__reader-expire--past': expired }">
                    <v-icon size="x-small">mdi-clock-outline</v-icon>
                    {{ expireLabel }}
                </div>

                <div v-if="detailItem.type === 'file' && canPreview" class="mega-wall__reader-preview">
                    <div v-if="previewLoading" class="mega-wall__preview-loading">
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
                            <pre class="mega-wall__preview-text">{{ displayedTextPreview }}</pre>
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

                <div v-if="detailItem.type === 'file'" class="mega-wall__reader-actions">
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
.mega-wall {
    background: #fdfdfb;
    height: 100vh;
    height: 100dvh;
    display: flex;
    flex-direction: column;
    color: #111827;
    font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", sans-serif;
    -webkit-font-smoothing: antialiased;
}

.mega-wall--dark {
    background: #101318;
    color: #f3f4f6;
}

.mega-wall > .page-toolbar {
    flex-shrink: 0;
}

.mega-wall__shell {
    max-width: 1100px;
    width: 100%;
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    padding: 0 16px 24px;
}

.mega-wall__head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
    padding: 22px 4px 16px;
    font-size: 13px;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: rgba(17, 24, 39, 0.55);
    flex-shrink: 0;
}

.mega-wall--dark .mega-wall__head {
    color: rgba(243, 244, 246, 0.5);
}

.mega-wall__count {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.1em;
    color: rgba(17, 24, 39, 0.45);
    background: rgba(17, 24, 39, 0.05);
    border-radius: 999px;
    padding: 3px 12px;
}

.mega-wall--dark .mega-wall__count {
    color: rgba(243, 244, 246, 0.5);
    background: rgba(255, 255, 255, 0.08);
}

.mega-wall__stream {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 4px 4px;
}

.mega-wall__item {
    padding: 8px 2px 20px;
    border-bottom: 1px solid rgba(17, 24, 39, 0.12);
}

.mega-wall--dark .mega-wall__item {
    border-bottom-color: rgba(255, 255, 255, 0.1);
}

.mega-wall__kicker {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.18em;
    text-transform: uppercase;
    color: rgba(17, 24, 39, 0.4);
    margin-bottom: 8px;
}

.mega-wall--dark .mega-wall__kicker {
    color: rgba(243, 244, 246, 0.4);
}

.mega-wall__body {
    font-size: clamp(22px, 4.6vw, 44px);
    font-weight: 800;
    line-height: 1.28;
    letter-spacing: -0.01em;
    word-break: break-word;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
    max-width: 30em;
}

.mega-wall__file {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: clamp(16px, 2.6vw, 26px);
    font-weight: 700;
    flex-wrap: wrap;
    cursor: pointer;
}

.mega-wall__file-glyph {
    font-size: clamp(20px, 3vw, 30px);
    flex-shrink: 0;
}

.mega-wall__file-name {
    word-break: break-all;
}

.mega-wall__file-meta {
    font-size: 13px;
    font-weight: 500;
    color: rgba(17, 24, 39, 0.5);
}

.mega-wall--dark .mega-wall__file-meta {
    color: rgba(243, 244, 246, 0.5);
}

.mega-wall__ops {
    display: none;
    gap: 2px;
    align-items: center;
}

.mega-wall__item:hover .mega-wall__ops,
.mega-wall__item:focus-within .mega-wall__ops {
    display: inline-flex;
}

@media (hover: none) {
    .mega-wall__ops {
        display: inline-flex;
        gap: 0;
    }

    .mega-wall__op {
        width: 40px;
        height: 40px;
        font-size: 22px;
    }
}

.mega-wall__op {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    border-radius: 50%;
    width: 46px;
    height: 46px;
    font-size: 24px;
    cursor: pointer;
    color: rgba(17, 24, 39, 0.5);
    transition: background 0.15s, color 0.15s;
}

.mega-wall--dark .mega-wall__op {
    color: rgba(243, 244, 246, 0.5);
}

.mega-wall__op:hover {
    background: rgba(17, 24, 39, 0.08);
    color: #111827;
}

.mega-wall--dark .mega-wall__op:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #f3f4f6;
}

.mega-wall__op--danger:hover {
    color: #d32f2f;
}

.mega-wall--dark .mega-wall__op--danger:hover {
    color: #ef5350;
}

.mega-wall__file-open {
    font-size: 12px;
    font-weight: 600;
    color: #1e88e5;
    margin-left: 4px;
    letter-spacing: 0.02em;
}

.mega-wall--dark .mega-wall__file-open {
    color: #7cb3f0;
}

.mega-wall__empty {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    padding: 60px 16px;
}

.mega-wall__composer {
    margin-top: 14px;
    flex-shrink: 0;
    padding-bottom: env(safe-area-inset-bottom);
}

.mega-wall--dark .mega-wall__composer :deep(.sticky-composer) {
    background: rgba(255, 255, 255, 0.06);
    border-color: rgba(255, 255, 255, 0.12);
}

.mega-wall--dark .mega-wall__composer :deep(.sticky-composer__area) {
    color: rgba(243, 244, 246, 0.95);
}

.mega-wall--dark .mega-wall__composer :deep(.sticky-composer__area::placeholder) {
    color: rgba(243, 244, 246, 0.4);
}

.mega-wall--dark .mega-wall__composer :deep(.sticky-composer__attach) {
    color: rgba(243, 244, 246, 0.4);
}

.mega-wall--dark .mega-wall__composer :deep(.sticky-composer__file) {
    background: rgba(255, 255, 255, 0.12);
    color: #e5e7eb;
}

.mega-wall__reader {
    border-radius: 10px;
    padding: 18px 20px;
    background: #ffffff;
    color: #111827;
    box-shadow: 0 6px 2px rgba(0, 0, 0, 0.08), 0 18px 40px rgba(17, 24, 39, 0.18);
}

.mega-wall__reader--dark {
    background: #1a1d24;
    color: #f3f4f6;
}

.mega-wall__reader-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 14px;
}

.mega-wall__reader-type {
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: #1e88e5;
}

.mega-wall--dark .mega-wall__reader-type {
    color: #7cb3f0;
}

.mega-wall__reader-time {
    font-size: 11px;
    letter-spacing: 0.06em;
    color: rgba(17, 24, 39, 0.5);
    margin-left: auto;
}

.mega-wall--dark .mega-wall__reader-time {
    color: rgba(243, 244, 246, 0.5);
}

.mega-wall__reader-file {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    margin-bottom: 8px;
}

.mega-wall__reader-glyph {
    font-size: 26px;
    flex-shrink: 0;
}

.mega-wall__reader-name {
    font-size: 18px;
    font-weight: 700;
    word-break: break-all;
}

.mega-wall__reader-meta {
    font-size: 12px;
    color: rgba(17, 24, 39, 0.55);
}

.mega-wall--dark .mega-wall__reader-meta {
    color: rgba(243, 244, 246, 0.55);
}

.mega-wall__reader-expire {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: rgba(17, 24, 39, 0.6);
    background: rgba(17, 24, 39, 0.06);
    border-radius: 6px;
    padding: 3px 8px;
    margin-bottom: 8px;
}

.mega-wall--dark .mega-wall__reader-expire {
    color: rgba(243, 244, 246, 0.65);
    background: rgba(255, 255, 255, 0.1);
}

.mega-wall__reader-expire--past {
    color: #c62828;
    background: rgba(198, 40, 40, 0.1);
}

.mega-wall--dark .mega-wall__reader-expire--past {
    color: #ef5350;
    background: rgba(239, 83, 80, 0.15);
}

.mega-wall__reader-preview {
    margin: 10px 0 4px;
}

.mega-wall__reader-preview > video,
.mega-wall__reader-preview > img {
    display: block;
    margin: 0 auto;
    border-radius: 8px;
}

.mega-wall__reader-preview > audio {
    display: block;
}

.mega-wall__preview-loading {
    display: flex;
    justify-content: center;
    padding: 48px 0;
}

.mega-wall__preview-text {
    white-space: pre-wrap;
    word-break: break-word;
    font-size: 13px;
    line-height: 1.6;
    background: rgba(0, 0, 0, 0.04);
    border-radius: 8px;
    padding: 12px;
    max-height: 40vh;
    overflow-y: auto;
}

.mega-wall--dark .mega-wall__preview-text {
    background: rgba(255, 255, 255, 0.06);
}

.mega-wall__reader-actions {
    display: flex;
    gap: 8px;
    margin-top: 16px;
}

.mega-wall__reader-actions .v-btn {
    font-size: 12px;
}
</style>