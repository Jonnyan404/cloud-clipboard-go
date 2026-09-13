<script setup>import { computed, ref, watch } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
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

const props = defineProps({
    meta: {
        type: Object,
        default: () => ({}),
    },
});

const app = useAppStore();
const ws = useWebSocketStore();
const { t } = useI18n();
const downloading = ref(false);
const expanded = ref(false);
const previewLoading = ref(false);
const srcPreview = ref(null);
const textPreview = ref('');
const showFullTextPreview = ref(false);
const textPreviewDisplayLimit = 16 * 1024;

const isFile = computed(() => props.meta.type === 'file');
const isPreviewableVideo = computed(() => props.meta.name?.match(/\.(mp4|webm|ogv)$/gi));
const isPreviewableAudio = computed(() => props.meta.name?.match(/\.(mp3|wav|ogg|opus|m4a|flac)$/gi));
const isPreviewableText = computed(() => props.meta.name?.match(/\.(txt|text|md|markdown|json|log|csv|tsv|ya?ml|xml|ini|conf|cfg|toml|properties|env|gitignore|dockerfile|js|jsx|mjs|cjs|ts|tsx|vue|css|scss|sass|less|html|htm|sql|sh|bash|zsh|fish|ps1|bat|cmd|go|py|java|kt|kts|rb|php|rs|c|cc|cpp|cxx|h|hh|hpp|hxx|swift|proto)$/gi));
const canPreview = computed(() => isFile.value && !expired.value && (Boolean(props.meta.thumbnail) || isPreviewableVideo.value || isPreviewableAudio.value || isPreviewableText.value));
const hasTruncatedTextPreview = computed(() => textPreview.value.length > textPreviewDisplayLimit);
const displayedTextPreview = computed(() => {
    if (!hasTruncatedTextPreview.value || showFullTextPreview.value) {
        return textPreview.value;
    }
    return `${textPreview.value.slice(0, textPreviewDisplayLimit)}\n\n...`;
});
const decodedContent = computed(() => {
    const textArea = document.createElement('textarea');
    textArea.innerHTML = props.meta.content || '';
    return textArea.value;
});
const isLink = computed(() => !isFile.value && /^https?:\/\/[^\s]+$/i.test(decodedContent.value.trim()));
const noteLabel = computed(() => {
    if (isFile.value) {
        return 'FILE';
    }
    if (isLink.value) {
        return 'LINK';
    }
    return 'TEXT';
});
const colorIndex = computed(() => {
    const id = String(props.meta.id || '');
    let hash = 0;
    for (const ch of id) {
        hash = (hash * 31 + ch.charCodeAt(0)) >>> 0;
    }
    return hash % 5;
});
const rotation = computed(() => {
    const deg = [-1.5, 1.5, -1, 2, -2][colorIndex.value] ?? 0;
    return `${deg}deg`;
});
const expired = computed(() => {
    if (!props.meta.expire || props.meta.expire <= 0) {
        return false;
    }
    return Date.now() / 1000 > props.meta.expire;
});
const isExpirable = computed(() => Boolean(props.meta.expire && props.meta.expire > 0));
const expireLabel = computed(() => {
    if (!isExpirable.value) {
        return '';
    }
    return `${expired.value ? t('expired') : t('expiresAt', { time: formatTimestamp(props.meta.expire) })}`;
});
const timestampLabel = computed(() => {
    if (!props.meta.timestamp) {
        return '';
    }
    const time = formatTimestamp(props.meta.timestamp);
    const device = props.meta.senderDevice?.os || props.meta.senderDevice?.type || '';
    if (device) {
        return `${time} ${t('stickyFromDevice', { device })}`;
    }
    return time;
});
const fileIcon = computed(() => {
    if (!props.meta.name) {
        return '📄';
    }
    if (/\.(png|jpe?g|gif|webp|svg|bmp|ico|avif)$/i.test(props.meta.name)) {
        return '🖼️';
    }
    if (/\.(mp4|webm|ogv|mov)$/i.test(props.meta.name)) {
        return '🎬';
    }
    if (/\.(mp3|wav|ogg|opus|m4a|flac)$/i.test(props.meta.name)) {
        return '🎵';
    }
    return '📄';
});
const fileMetaLabel = computed(() => {
    const size = prettyFileSize(props.meta.size || 0);
    if (/\.(png|jpe?g|gif|webp|svg|bmp|ico|avif)$/i.test(props.meta.name || '')) {
        return `${size} · ${t('stickyImage')}`;
    }
    return size;
});
const contentUrl = computed(() => {
    const roomQuery = ws.room ? `?room=${encodeURIComponent(ws.room)}` : '';
    const id = props.meta?.id ?? '';
    return buildCleanAbsoluteRouteUrl(`content/${id}${roomQuery}`, app?.config?.server?.prefix || '');
});
const fileUrl = computed(() => {
    const cache = props.meta?.cache || '';
    const encodedFilename = encodeURIComponent(props.meta?.name || 'file');
    return buildCleanAbsoluteRouteUrl(`file/${cache}/${encodedFilename}`, app?.config?.server?.prefix || '');
});
const needsShareProtection = computed(() => Boolean(app?.config?.auth));

async function ensureFileShareUrl() {
    if (!needsShareProtection.value) {
        return fileUrl.value;
    }
    const data = await createShareLink({ type: 'file', uuid: props.meta?.cache, ttl: SHARE_DEFAULT_TTL, maxUses: 0, room: ws.room });
    return data?.url || '';
}
async function downloadFile() {
    if (expired.value || downloading.value) {
        return;
    }
    downloading.value = true;
    try {
        const url = await ensureFileShareUrl();
        const downloadUrl = new URL(url, window.location.origin);
        downloadUrl.searchParams.set('download', 'true');
        const anchor = document.createElement('a');
        anchor.href = downloadUrl.toString();
        anchor.download = props.meta?.name || 'file';
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
async function copyContent() {
    try {
        await copyTextToClipboard(decodedContent.value);
        toast(t('copySuccess'));
    } catch (err) {
        console.error('复制失败:', err);
        toast(t('copyFailedGeneral'));
    }
}
async function copyLink() {
    let url = contentUrl.value;
    if (needsShareProtection.value) {
        try {
            const data = await createShareLink({ type: 'content', id: props.meta?.id, ttl: SHARE_DEFAULT_TTL, maxUses: 0, room: ws.room });
            url = data?.url || url;
        } catch (error) {
            toast(t('copyFailedGeneral'));
            return;
        }
    }
    try {
        await copyTextToClipboard(url);
        toast(t('copySuccess'));
    } catch (err) {
        toast(t('copyFailedGeneral'));
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
            srcPreview.value = await ensureFileShareUrl();
        } catch (error) {
            console.error('生成预览链接失败:', error);
            toast(t('fileFetchFailed'));
        } finally {
            previewLoading.value = false;
        }
    } else if (isPreviewableText.value) {
        previewLoading.value = true;
        try {
            const response = await axios.get(`file/${props.meta.cache}/${encodeURIComponent(props.meta.name)}`, {
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
            const response = await axios.get(`file/${props.meta.cache}/${encodeURIComponent(props.meta.name)}`, {
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

watch(expanded, (open) => {
    if (open) {
        loadPreview();
    }
});

function toggleTextPreview() {
    showFullTextPreview.value = !showFullTextPreview.value;
}

async function deleteItem() {
    try {
        await axios.delete(`revoke/${props.meta.id}`, {
            params: new URLSearchParams([['room', ws.room]]),
        });
        toast(t(isFile.value ? 'deleteSuccessFile' : 'deleteSuccessText', { name: props.meta.name }));
    } catch (error) {
        if (error.response && error.response.data.msg) {
            toast(t('deleteFailedMessageMsg', { msg: error.response.data.msg }));
        } else {
            toast(t('deleteFailedMessage'));
        }
    }
}
</script>

<template>
    <div class="sticky-note"
         :class="[`sticky-note--c${colorIndex}`, { 'sticky-note--expired': expired }]"
         :style="{ '--rt': rotation }"
         role="button"
         tabindex="0"
         @click="expanded = true"
         @keydown.enter.prevent="expanded = true"
    >
        <div v-if="isFile" class="sticky-note__file">
            <span class="sticky-note__fic">{{ fileIcon }}</span>
            <span class="sticky-note__fname" :title="meta.name">{{ meta.name }}</span>
        </div>
        <div class="sticky-note__label">{{ noteLabel }}</div>
        <div v-if="isFile" class="sticky-note__meta">{{ fileMetaLabel }}</div>
        <div v-else class="sticky-note__text"
             :class="isLink ? 'sticky-note__text--link' : ''"
             :title="decodedContent"
        >{{ decodedContent }}</div>
        <span class="sticky-note__time">{{ timestampLabel }}</span>

        <span class="sticky-note__ops" @click.stop>
            <v-tooltip :text="isFile ? (expired ? t('expired') : t('download')) : t('copyText')" location="top">
                <template v-slot:activator="{ props }">
                    <v-btn
                        v-bind="props"
                        icon
                        density="compact"
                        size="small"
                        variant="text"
                        class="sticky-note__op"
                        :loading="isFile && downloading"
                        :disabled="isFile && expired"
                        @click.stop="isFile ? downloadFile() : copyContent()"
                    >
                        <v-icon size="large">{{ isFile ? 'mdi-download' : 'mdi-content-copy' }}</v-icon>
                    </v-btn>
                </template>
            </v-tooltip>
            <v-tooltip v-if="!isFile" :text="t('copyLink')" location="top">
                <template v-slot:activator="{ props }">
                    <v-btn v-bind="props" icon density="compact" size="small" variant="text" class="sticky-note__op" @click.stop="copyLink">
                        <v-icon size="large">mdi-link-variant</v-icon>
                    </v-btn>
                </template>
            </v-tooltip>
            <v-tooltip :text="t('delete')" location="top">
                <template v-slot:activator="{ props }">
                    <v-btn v-bind="props" icon density="compact" size="small" variant="text" class="sticky-note__op" @click.stop="deleteItem">
                        <v-icon size="large">mdi-close</v-icon>
                    </v-btn>
                </template>
            </v-tooltip>
        </span>

        <v-dialog v-model="expanded" max-width="520">
            <div class="sticky-note__reader" :class="[`sticky-note__reader--c${colorIndex}`]">
                <div class="sticky-note__reader-head">
                    <div class="sticky-note__label">{{ noteLabel }}</div>
                    <span class="sticky-note__reader-time">{{ timestampLabel }}</span>
                    <v-btn icon density="compact" size="x-small" variant="text" class="sticky-note__op" @click="expanded = false">
                        <v-icon size="small">mdi-close</v-icon>
                    </v-btn>
                </div>
                <div v-if="isFile" class="sticky-note__reader-file">
                    <span class="sticky-note__fic">{{ fileIcon }}</span>
                    <span class="sticky-note__reader-name">{{ meta.name }}</span>
                    <span class="sticky-note__meta">{{ fileMetaLabel }}</span>
                </div>
                <div v-if="isFile && isExpirable" class="sticky-note__reader-expire" :class="{ 'sticky-note__reader-expire--past': expired }">
                    <v-icon size="x-small">mdi-clock-outline</v-icon>
                    {{ expireLabel }}
                </div>
                <div v-else class="sticky-note__reader-text" :class="isLink ? 'sticky-note__text--link' : ''">{{ decodedContent }}</div>

                <div v-if="isFile && canPreview" class="sticky-note__reader-preview">
                    <div v-if="previewLoading" class="sticky-note__preview-loading">
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
                            <pre class="sticky-note__preview-text">{{ displayedTextPreview }}</pre>
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
                            :src="srcPreview || props.meta.thumbnail"
                            style="max-height:60vh;max-width:100%;"
                        >
                    </template>
                </div>

                <div class="sticky-note__reader-actions">
                    <v-btn
                        v-if="isFile"
                        color="primary"
                        variant="flat"
                        size="small"
                        :loading="downloading"
                        :disabled="expired"
                        @click="downloadFile"
                    >
                        <v-icon start size="small">mdi-download</v-icon>{{ expired ? t('expired') : t('download') }}
                    </v-btn>
                    <v-btn v-else color="primary" variant="flat" size="small" @click="copyContent">
                        <v-icon start size="small">mdi-content-copy</v-icon>{{ t('copyText') }}
                    </v-btn>
                    <v-btn v-if="!isFile" variant="text" size="small" @click="copyLink">
                        <v-icon start size="small">mdi-link-variant</v-icon>{{ t('copyLink') }}
                    </v-btn>
                    <v-btn variant="text" size="small" color="error" class="sticky-note__reader-delete" @click="deleteItem">
                        <v-icon start size="small">mdi-delete-outline</v-icon>{{ t('delete') }}
                    </v-btn>
                </div>
            </div>
        </v-dialog>
    </div>
</template>

<style scoped>
.sticky-note {
    position: relative;
    border-radius: 4px;
    padding: 12px 14px 10px;
    box-shadow: 0 2px 1px rgba(0, 0, 0, 0.06), 0 6px 14px rgba(68, 64, 42, 0.12);
    transform: rotate(var(--rt, 0deg));
    font-family: 'Comic Sans MS', 'Kaiti', 'PingFang SC', sans-serif;
    min-height: 92px;
    height: 100%;
    display: flex;
    flex-direction: column;
    overflow: hidden;
}

.sticky-note::before {
    content: '';
    position: absolute;
    top: -8px;
    left: 50%;
    transform: translateX(-50%);
    width: 40px;
    height: 13px;
    background: rgba(0, 0, 0, 0.08);
    border-radius: 2px;
    z-index: 2;
}

.sticky-note--c0 {
    background: #fff8c5;
}

.sticky-note--c1 {
    background: #ffd8d8;
}

.sticky-note--c2 {
    background: #d6f0d6;
}

.sticky-note--c3 {
    background: #d9e6ff;
}

.sticky-note--c4 {
    background: #e9d9ff;
}

.sticky-note--expired .sticky-note__fname {
    text-decoration: line-through;
}

.sticky-note__label {
    font-size: 10px;
    font-weight: 700;
    opacity: 0.55;
    margin-bottom: 6px;
    letter-spacing: 0.06em;
}

.sticky-note__file {
    display: flex;
    align-items: baseline;
    gap: 5px;
    font-size: 12px;
    line-height: 1.45;
    word-break: break-all;
}

.sticky-note__fic {
    flex-shrink: 0;
    font-size: 13px;
}

.sticky-note__fname {
    font-weight: 700;
    font-size: 12px;
    line-height: 1.45;
    min-width: 0;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
}

.sticky-note__meta {
    font-size: 11px;
    margin-top: 6px;
    opacity: 0.7;
}

.sticky-note__text {
    font-size: 13px;
    line-height: 1.5;
    font-weight: 500;
    word-break: break-word;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
}

.sticky-note__text--link {
    color: #1e6bb8;
    text-decoration: underline;
    font-size: 12.5px;
    word-break: break-all;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
}

.sticky-note__time {
    font-size: 10px;
    opacity: 0.55;
    margin-top: auto;
    padding-top: 8px;
}

.sticky-note__ops {
    position: absolute;
    top: 6px;
    right: 6px;
    display: none;
    gap: 2px;
    background: rgba(255, 255, 255, 0.55);
    border-radius: 999px;
    padding: 2px 3px;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.12);
    z-index: 3;
}

.sticky-note:hover .sticky-note__ops {
    display: flex;
}

@media (hover: none) {
    .sticky-note__ops {
        display: flex;
    }
}

.sticky-note__op {
    color: rgba(68, 64, 42, 0.72);
}

.sticky-note__reader {
    border-radius: 6px;
    padding: 16px 18px;
    box-shadow: 0 4px 2px rgba(0, 0, 0, 0.1), 0 14px 30px rgba(68, 64, 42, 0.28);
    color: #444034;
}

.sticky-note__reader--c0 {
    background: #fff8c5;
}

.sticky-note__reader--c1 {
    background: #ffd8d8;
}

.sticky-note__reader--c2 {
    background: #d6f0d6;
}

.sticky-note__reader--c3 {
    background: #d9e6ff;
}

.sticky-note__reader--c4 {
    background: #e9d9ff;
}

.sticky-note__reader-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
}

.sticky-note__reader-head .sticky-note__label {
    margin-bottom: 0;
}

.sticky-note__reader-time {
    font-size: 11px;
    opacity: 0.55;
    margin-left: auto;
}

.sticky-note__reader-text {
    font-size: 15px;
    line-height: 1.7;
    font-weight: 500;
    word-break: break-word;
    white-space: pre-wrap;
    max-height: 55vh;
    overflow-y: auto;
}

.sticky-note__reader-text--link {
    word-break: break-all;
}

.sticky-note__reader-file {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    margin-bottom: 6px;
}

.sticky-note__reader-name {
    font-weight: 700;
    font-size: 15px;
    word-break: break-all;
}

.sticky-note__reader-file .sticky-note__meta {
    margin-top: 0;
}

.sticky-note__reader-expire {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: rgba(68, 64, 42, 0.65);
    background: rgba(0, 0, 0, 0.06);
    border-radius: 6px;
    padding: 3px 8px;
    margin-top: 8px;
}

.sticky-note__reader-expire--past {
    color: #c62828;
    background: rgba(198, 40, 40, 0.1);
}

.sticky-note__reader-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    margin-top: 16px;
    padding-top: 14px;
    border-top: 1px dashed rgba(68, 64, 42, 0.16);
}

.sticky-note__reader-delete {
    margin-left: auto;
    opacity: 0.72;
}

.sticky-note__reader-actions .v-btn {
    font-size: 12px;
}

.sticky-note__preview-loading {
    display: flex;
    justify-content: center;
    padding: 48px 0;
}

.sticky-note__reader-preview {
    margin: 4px 0 2px;
}

.sticky-note__reader-preview > video,
.sticky-note__reader-preview > img {
    display: block;
    margin: 0 auto;
    border-radius: 8px;
}

.sticky-note__reader-preview > audio {
    display: block;
}

.sticky-note__preview-text {
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
</style>