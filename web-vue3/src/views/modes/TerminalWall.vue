<script setup>import { computed, ref, watch } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import {
    prettyFileSize,
    buildCleanAbsoluteRouteUrl,
    createShareLink,
    copyTextToClipboard,
    SHARE_DEFAULT_TTL,
} from '@/util.js';
import PageToolbar from '@/components/PageToolbar.vue';
import StickyComposer from '@/components/sticky/StickyComposer.vue';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();

const items = computed(() => app.received);
const countLabel = computed(() => t('terminalLogCount', { count: items.value.length }));

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

function fileGlyph(item) {
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

function timeLabel(item) {
    if (!item?.timestamp) {
        return '';
    }
    const date = new Date(item.timestamp * 1000);
    const pad = value => String(value).padStart(2, '0');
    return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

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

async function downloadFromItem(item) {
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
        class="terminal-wall"
        :class="{ 'terminal-wall--dark': isDark }"
    >
        <PageToolbar variant="terminal"></PageToolbar>
        <div class="terminal-wall__shell mx-auto">
            <div class="terminal-wall__head">
                <span class="terminal-wall__prompt">$</span>
                <span class="terminal-wall__cmd">{{ ws.room || t('publicRoom') }}</span>
                <span class="terminal-wall__count">{{ countLabel }}</span>
            </div>

            <div v-if="items.length" class="terminal-wall__stream">
                <div v-for="item in items" :key="item.id" class="terminal-wall__log">
                    <span class="terminal-wall__ts">{{ timeLabel(item) }}</span>
                    <span v-if="item.type === 'file'" class="terminal-wall__tag terminal-wall__tag--file">[FILE]</span>
                    <span v-else class="terminal-wall__tag terminal-wall__tag--text">[TEXT]</span>
                    <template v-if="item.type === 'text'">
                        <span class="terminal-wall__val">{{ decodedContent(item) }}</span>
                        <span class="terminal-wall__ops">
                            <button type="button" class="terminal-wall__op" :title="t('copyText')" @click="copyContent(item)">
                                <v-icon size="x-small">mdi-content-copy</v-icon>
                            </button>
                            <button type="button" class="terminal-wall__op terminal-wall__op--danger" :title="t('delete')" @click="deleteItem(item)">
                                <v-icon size="x-small">mdi-close</v-icon>
                            </button>
                        </span>
                    </template>
                    <template v-else>
                        <span class="terminal-wall__file" role="button" tabindex="0" @click="detailItem = item" @keydown.enter.prevent="detailItem = item">
                            {{ fileGlyph(item) }} {{ item.name || 'file' }}
                        </span>
                        <span class="terminal-wall__dim">
                            {{ prettyFileSize(item.size || 0) }}{{ isItemExpired(item) ? ` · ${t('expired')}` : '' }}
                        </span>
                        <span class="terminal-wall__ops">
                            <button type="button" class="terminal-wall__op" :title="isItemExpired(item) ? t('expired') : t('download')" @click="downloadFromItem(item)">
                                <v-icon size="x-small">mdi-download</v-icon>
                            </button>
                            <button type="button" class="terminal-wall__op terminal-wall__op--danger" :title="t('delete')" @click="deleteItem(item)">
                                <v-icon size="x-small">mdi-close</v-icon>
                            </button>
                        </span>
                    </template>
                </div>
            </div>

            <div v-else class="terminal-wall__empty">
                <span class="terminal-wall__empty-prompt">$ cat</span>
                <span class="terminal-wall__empty-msg">NO OUTPUT</span>
            </div>

            <div class="terminal-wall__cursor">
                <span class="terminal-wall__prompt">$</span>
                <span>{{ t('terminalCursor') }}<b class="terminal-wall__block">▊</b></span>
            </div>

            <div class="terminal-wall__composer">
                <sticky-composer variant="terminal"></sticky-composer>
            </div>
        </div>

        <v-dialog v-model="detailItem" max-width="560">
            <div v-if="detailItem" class="terminal-wall__reader">
                <div class="terminal-wall__reader-head">
                    <span class="terminal-wall__tag" :class="detailItem.type === 'file' ? 'terminal-wall__tag--file' : 'terminal-wall__tag--text'">[{{ detailItem.type.toUpperCase() }}]</span>
                    <span class="terminal-wall__ts">{{ timeLabel(detailItem) }}</span>
                    <v-btn icon density="compact" size="x-small" variant="text" class="terminal-wall__op" @click="detailItem = null">
                        <v-icon size="small">mdi-close</v-icon>
                    </v-btn>
                </div>
                <div v-if="detailItem.type === 'file'" class="terminal-wall__reader-file">
                    <span class="terminal-wall__reader-glyph">{{ fileGlyph(detailItem) }}</span>
                    <span class="terminal-wall__reader-name">{{ detailItem.name }}</span>
                    <span class="terminal-wall__reader-meta">{{ prettyFileSize(detailItem.size || 0) }}</span>
                </div>
                <div v-if="detailItem.type === 'file' && isExpirable" class="terminal-wall__reader-expire" :class="{ 'terminal-wall__reader-expire--past': expired }">
                    <v-icon size="x-small">mdi-clock-outline</v-icon>
                    {{ expired ? t('expired') : t('expiresAt', { time: timeLabel(detailItem) }) }}
                </div>

                <div v-if="detailItem.type === 'file' && canPreview" class="terminal-wall__reader-preview">
                    <div v-if="previewLoading" class="terminal-wall__preview-loading">
                        <v-progress-circular indeterminate color="primary" size="36"></v-progress-circular>
                    </div>
                    <template v-else>
                        <video v-if="isPreviewableVideo" :src="srcPreview" style="max-height:60vh;max-width:100%;" controls preload="metadata"></video>
                        <audio v-else-if="isPreviewableAudio" :src="srcPreview" style="width:100%" controls preload="metadata"></audio>
                        <template v-else-if="isPreviewableText">
                            <pre class="terminal-wall__preview-text">{{ displayedTextPreview }}</pre>
                            <div v-if="hasTruncatedTextPreview" class="d-flex justify-space-between align-center mt-2">
                                <div class="text-caption text-medium-emphasis">
                                    {{ t('textPreviewTruncated', { limit: prettyFileSize(textPreviewDisplayLimit) }) }}
                                </div>
                                <v-btn size="small" variant="text" color="primary" @click="toggleTextPreview">
                                    {{ showFullTextPreview ? t('collapseTextPreview') : t('expandTextPreview') }}
                                </v-btn>
                            </div>
                        </template>
                        <img v-else :src="srcPreview || detailItem.thumbnail" style="max-height:60vh;max-width:100%;">
                    </template>
                </div>

                <div v-if="detailItem.type === 'file'" class="terminal-wall__reader-actions">
                    <v-btn color="primary" variant="flat" size="small" :loading="downloading" :disabled="expired" @click="downloadFromItem(detailItem)">
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
.terminal-wall {
    --tw-bg: #ffffff;
    --tw-surface: #f6f8fa;
    --tw-border: #d0d7de;
    --tw-border-strong: #afb8c1;
    --tw-text: #1f2328;
    --tw-muted: #57606a;
    --tw-subtle: #6e7781;
    --tw-accent: rgba(var(--v-theme-primary), 1);
    --tw-tag-text: #0969da;
    --tw-tag-file: #cf222e;
    --tw-danger: #d1242f;
    --tw-reader-expire-bg: #f6f8fa;
    background: var(--tw-bg);
    height: 100vh;
    height: 100dvh;
    display: flex;
    flex-direction: column;
    color: var(--tw-text);
    font-family: 'SF Mono', 'Menlo', 'Consolas', monospace;
    -webkit-font-smoothing: antialiased;
    overflow: hidden;
}

.terminal-wall--dark {
    --tw-bg: #0d1117;
    --tw-surface: #161b22;
    --tw-border: #30363d;
    --tw-border-strong: #484f58;
    --tw-text: #c9d1d9;
    --tw-muted: #8b949e;
    --tw-subtle: #6e7681;
    --tw-tag-text: #79c0ff;
    --tw-tag-file: #ffa657;
    --tw-danger: #f85149;
    --tw-reader-expire-bg: #21262d;
}

.terminal-wall > .page-toolbar {
    flex-shrink: 0;
}

.terminal-wall__shell {
    max-width: 1100px;
    width: 100%;
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    padding: 0 16px 24px;
}

.terminal-wall__head {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 14px 4px 6px;
    font-size: 12px;
    color: var(--tw-muted);
    flex-shrink: 0;
}

.terminal-wall__prompt {
    color: var(--tw-accent);
    font-weight: 700;
}

.terminal-wall__cmd {
    color: var(--tw-text);
    font-weight: 700;
    background: var(--tw-surface);
    border: 1px solid var(--tw-border);
    border-radius: 6px;
    padding: 2px 8px;
}

.terminal-wall__count {
    margin-left: auto;
    color: var(--tw-muted);
}

.terminal-wall__stream {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 8px 4px;
}

.terminal-wall__log {
    font-size: 11.5px;
    line-height: 2.0;
    color: var(--tw-muted);
    display: flex;
    align-items: baseline;
    gap: 8px;
    flex-wrap: wrap;
    border-bottom: 1px solid transparent;
}

.terminal-wall__ts {
    color: var(--tw-subtle);
    flex-shrink: 0;
}

.terminal-wall__tag {
    display: inline-block;
    width: 46px;
    flex-shrink: 0;
    font-weight: 700;
}

.terminal-wall__tag--text {
    color: var(--tw-tag-text);
}

.terminal-wall__tag--file {
    color: var(--tw-tag-file);
}

.terminal-wall__val {
    color: var(--tw-text);
    word-break: break-word;
    overflow-wrap: anywhere;
    flex: 1;
    min-width: 0;
}

.terminal-wall__file {
    color: var(--tw-text);
    word-break: break-all;
    cursor: pointer;
    flex: 1;
    min-width: 0;
}

.terminal-wall__file:hover {
    color: var(--tw-accent);
}

.terminal-wall__dim {
    color: var(--tw-subtle);
    flex-shrink: 0;
}

.terminal-wall__ops {
    display: none;
    gap: 2px;
    align-items: center;
    flex-shrink: 0;
}

.terminal-wall__log:hover .terminal-wall__ops,
.terminal-wall__log:focus-within .terminal-wall__ops {
    display: inline-flex;
}

@media (hover: none) {
    .terminal-wall__ops {
        display: inline-flex;
    }
}

.terminal-wall__op {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    border-radius: 50%;
    width: 22px;
    height: 22px;
    cursor: pointer;
    color: var(--tw-muted);
    transition: background 0.15s, color 0.15s;
}

.terminal-wall__op:hover {
    background: var(--tw-border);
    color: var(--tw-text);
}

.terminal-wall__op--danger:hover {
    color: var(--tw-danger);
}

.terminal-wall__empty {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    color: var(--tw-subtle);
    font-size: 12px;
}

.terminal-wall__empty-prompt {
    color: var(--tw-accent);
}

.terminal-wall__cursor {
    font-size: 11.5px;
    color: var(--tw-accent);
    padding: 2px 4px;
    display: flex;
    gap: 8px;
    align-items: center;
    flex-shrink: 0;
}

.terminal-wall__block {
    font-weight: 400;
    background: var(--tw-accent);
    color: var(--tw-bg);
    padding: 0 3px;
    animation: terminal-blink 1.1s steps(2, start) infinite;
}

@keyframes terminal-blink {
    0%, 50% {
        opacity: 1;
    }
    51%, 100% {
        opacity: 0;
    }
}

.terminal-wall__composer {
    margin-top: 12px;
    flex-shrink: 0;
    padding-bottom: env(safe-area-inset-bottom);
}

.terminal-wall__composer :deep(.sticky-composer--terminal) {
    background: var(--tw-surface);
    border-color: var(--tw-border);
}

.terminal-wall__composer :deep(.sticky-composer--terminal .sticky-composer__area) {
    color: var(--tw-text);
}

.terminal-wall__composer :deep(.sticky-composer--terminal .sticky-composer__area::placeholder) {
    color: var(--tw-subtle);
}

.terminal-wall__composer :deep(.sticky-composer--terminal .sticky-composer__attach) {
    color: var(--tw-muted);
}

.terminal-wall__composer :deep(.sticky-composer--terminal .sticky-composer__file) {
    background: var(--tw-reader-expire-bg);
    color: var(--tw-text);
}

.terminal-wall__composer :deep(.sticky-composer--terminal .sticky-composer__progress) {
    color: var(--tw-accent);
}

.terminal-wall__reader {
    border-radius: 10px;
    background: var(--tw-surface);
    border: 1px solid var(--tw-border);
    padding: 18px;
    color: var(--tw-text);
    font-family: 'SF Mono', 'Menlo', 'Consolas', monospace;
}

.terminal-wall__reader-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 14px;
}

.terminal-wall__reader-file {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
    margin-bottom: 12px;
}

.terminal-wall__reader-glyph {
    font-size: 24px;
    flex-shrink: 0;
}

.terminal-wall__reader-name {
    font-weight: 700;
    word-break: break-all;
}

.terminal-wall__reader-meta {
    font-size: 12px;
    color: var(--tw-muted);
}

.terminal-wall__reader-expire {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: var(--tw-muted);
    background: var(--tw-reader-expire-bg);
    border-radius: 6px;
    padding: 3px 8px;
    margin-bottom: 8px;
}

.terminal-wall__reader-expire--past {
    color: var(--tw-danger);
    background: color-mix(in srgb, var(--tw-danger) 15%, transparent);
}

.terminal-wall__reader-preview {
    margin-top: 6px;
}

.terminal-wall__preview-loading {
    display: flex;
    justify-content: center;
    padding: 24px 0;
}

.terminal-wall__preview-text {
    font-family: inherit;
    font-size: 12px;
    line-height: 1.6;
    color: var(--tw-text);
    background: var(--tw-bg);
    border: 1px solid var(--tw-border);
    border-radius: 8px;
    padding: 12px;
    max-height: 50vh;
    overflow: auto;
    white-space: pre-wrap;
    word-break: break-word;
}

.terminal-wall__reader-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    margin-top: 16px;
}
</style>