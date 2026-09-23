<script setup>
// 速览模式的**右侧预览**：选中哪一条，这里就显示它的完整内容。
//
// 为什么单独一个组件：同一个预览要在两个地方出现 —— 宽屏是右侧常驻面板，
// 窄屏是全屏弹窗。两边共用一份，避免「面板里能做的事、弹窗里做不了」。
//
// 和卡片（received-item/*）的区别：卡片是**列表里的一行**，所以正文要截断、要能收起；
// 预览是**一条的完整呈现**，正文不截断，动作行和元信息也都在这里。
import { computed, ref, watch } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import axios from 'axios';
import ShareLinkButton from '@/components/ShareLinkButton.vue';
import MarkdownBody from '@/components/MarkdownBody.vue';
import MarkdownToggle from '@/components/MarkdownToggle.vue';
import { useMarkdown } from '@/composables/useMarkdown.js';
import {
    SHARE_DEFAULT_TTL,
    copyTextToClipboard,
    createShareLink,
    deviceLabel,
    errorMessage,
    filePreviewKind,
    formatTimestamp,
    prettyFileSize,
} from '@/util.js';

const props = defineProps({
    item: { type: Object, default: null },
});

const app = useAppStore();
const ws = useWebSocketStore();
const { t } = useI18n();

const isFile = computed(() => props.item?.type === 'file');
const content = computed(() => props.item?.content || '');
// md 渲染沿用全站约定（useMarkdown 内部：任务列表 / 表格默认就是 md）。
const md = useMarkdown(() => content.value);

// 文件条目的**可预览类型**。判型收在 `util.js` 的 `filePreviewKind`（**全站唯一实现**）——
// 这段正则本来在 7 个文件里各有一份，而且已经漂移（有的认 `.mov`、有的不认）。
// 判型只看扩展名，不看内容：服务端不嗅探、客户端也不该嗅探，两边同一套标准。
const previewKind = computed(() => (isFile.value ? filePreviewKind(props.item?.name) : ''));
const isImage = computed(() => previewKind.value === 'image');
const isVideo = computed(() => previewKind.value === 'video');
const isAudio = computed(() => previewKind.value === 'audio');
const isTextFile = computed(() => previewKind.value === 'text');
const canPreview = computed(() => Boolean(previewKind.value));

const previewLoading = ref(false);
// 媒体（图/视频/音频）走**直连正文**的地址：带 token 的 `rawUrl` 直接塞进 src 就能流式加载，
// 不用把整份字节读进内存。文本文件才需要真的取回文本。
const previewSrc = ref('');
const textPreview = ref('');
const downloading = ref(false);

async function ensureRawFileUrl() {
    if (previewSrc.value) {
        return previewSrc.value;
    }
    const data = await createShareLink({
        type: 'file',
        uuid: props.item?.cache,
        ttl: SHARE_DEFAULT_TTL,
        maxUses: 0,
        room: ws.room,
    });
    return data?.rawUrl || '';
}

async function loadPreview() {
    previewSrc.value = '';
    textPreview.value = '';
    if (!canPreview.value) {
        return;
    }
    previewLoading.value = true;
    try {
        if (isTextFile.value) {
            // ⚠️ 文本文件**必须**另发一次请求取正文：文件条目本身没有 content。
            const response = await axios.get(
                `file/${props.item.cache}/${encodeURIComponent(props.item.name)}`,
                { responseType: 'text' },
            );
            textPreview.value = typeof response.data === 'string' ? response.data : String(response.data || '');
        } else {
            previewSrc.value = await ensureRawFileUrl();
        }
    } catch (error) {
        toast(errorMessage(error) || t('fileFetchFailed'));
    } finally {
        previewLoading.value = false;
    }
}

// 换了条目就重新加载预览，并清掉上一条的缓存 —— 否则会拿上一条的地址去加载。
watch(() => props.item?.id, () => {
    previewSrc.value = '';
    textPreview.value = '';
    loadPreview();
}, { immediate: true });

async function downloadFile() {
    if (downloading.value || !props.item?.cache) {
        return;
    }
    downloading.value = true;
    try {
        const url = await ensureRawFileUrl();
        if (!url) {
            return;
        }
        const target = new URL(url, window.location.origin);
        target.searchParams.set('download', 'true');
        const anchor = document.createElement('a');
        anchor.href = target.toString();
        anchor.download = props.item?.name || 'file';
        anchor.rel = 'noopener';
        document.body.appendChild(anchor);
        anchor.click();
        document.body.removeChild(anchor);
    } catch (error) {
        toast(errorMessage(error) || t('fileFetchFailed'));
    } finally {
        downloading.value = false;
    }
}

async function copyContent() {
    try {
        await copyTextToClipboard(content.value);
        toast(t('copySuccess'));
    } catch {
        toast(t('copyFailedGeneral'));
    }
}

async function deleteItem() {
    try {
        await axios.delete(`revoke/${props.item.id}`, {
            params: new URLSearchParams([['room', ws.room]]),
        });
        toast(t('deleteSuccessText', { name: props.item.name || '' }));
    } catch (error) {
        toast(errorMessage(error) || t('deleteFailedMessage'));
    }
}

// 底部元信息条：字符数 / 字节 / 来源设备 / 时间。
// 字符数和字节数对「这条到底多大」有用（文本条目在列表里看不出来）。
const charCount = computed(() => (isFile.value ? 0 : [...content.value].length));
const byteCount = computed(() => {
    if (isFile.value) {
        return Number(props.item?.size || 0);
    }
    return new TextEncoder().encode(content.value).length;
});
const sourceLabel = computed(() => {
    const device = deviceLabel(props.item?.senderDevice);
    return device || props.item?.senderDevice?.type || '';
});
const timeLabel = computed(() => (props.item?.timestamp ? formatTimestamp(props.item.timestamp) : ''));
</script>

<template>
    <div v-if="item" class="glance-preview">
        <!-- 文件条目没有正文，但**能预览就预览**：
             图片 / 视频 / 音频直接渲染（走带 token 的直连地址，浏览器流式加载），
             文本类文件取回正文显示；都不是的话才退回一个图标。 -->
        <div v-if="isFile" class="glance-preview__file">
            <div class="glance-preview__media">
                <v-progress-circular v-if="previewLoading" indeterminate size="26" width="3" color="primary" />
                <img v-else-if="isImage && previewSrc" :src="previewSrc" :alt="item.name" class="glance-preview__image">
                <video v-else-if="isVideo && previewSrc" :src="previewSrc" controls class="glance-preview__video"></video>
                <audio v-else-if="isAudio && previewSrc" :src="previewSrc" controls class="glance-preview__audio"></audio>
                <pre v-else-if="isTextFile && textPreview" class="glance-preview__filetext">{{ textPreview }}</pre>
                <v-icon v-else size="40" class="glance-preview__glyph">mdi-file-outline</v-icon>
            </div>
            <div class="glance-preview__file-meta">
                <div class="glance-preview__file-name">{{ item.name || 'file' }}</div>
                <div class="glance-preview__muted">{{ prettyFileSize(Number(item.size || 0)) }}</div>
            </div>
        </div>

        <div v-else class="glance-preview__body" :class="{ 'glance-preview__body--md': md.available }">
            <markdown-toggle v-if="md.available" v-model:mode="md.mode"></markdown-toggle>
            <markdown-body v-if="md.html" :html="md.html"></markdown-body>
            <div v-else class="glance-preview__text">{{ content }}</div>
        </div>

        <div class="glance-preview__actions">
            <v-btn v-if="isFile" color="primary" variant="flat" size="small" :loading="downloading" @click="downloadFile">
                <v-icon start size="small">mdi-download</v-icon>{{ t('download') }}
            </v-btn>
            <v-btn v-else variant="text" size="small" @click="copyContent">
                <v-icon start size="small">mdi-content-copy</v-icon>{{ t('copyText') }}
            </v-btn>
            <share-link-button :meta="item" :icon-only="false" />
            <v-spacer></v-spacer>
            <v-btn variant="text" size="small" class="glance-preview__delete" @click="deleteItem">
                <v-icon start size="small">mdi-close-circle-outline</v-icon>{{ t('delete') }}
            </v-btn>
        </div>

        <!-- 元信息条：卡片里放不下这些，而预览是「这一条的完整呈现」，正好放这里。 -->
        <div class="glance-preview__meta">
            <span v-if="!isFile">{{ t('glanceChars', { count: charCount }) }}</span>
            <span>{{ prettyFileSize(byteCount) }}</span>
            <span v-if="timeLabel">{{ timeLabel }}</span>
            <span v-if="sourceLabel">{{ t('glanceFrom', { source: sourceLabel }) }}</span>
        </div>
    </div>
    <div v-else class="glance-preview glance-preview--empty">
        {{ t('glanceNothingSelected') }}
    </div>
</template>

<style scoped>
.glance-preview {
    display: flex;
    flex-direction: column;
    min-height: 0;
    gap: 12px;
}

.glance-preview--empty {
    align-items: center;
    justify-content: center;
    padding: 48px 16px;
    font-size: 13px;
    color: rgba(var(--v-theme-on-surface), 0.5);
}

.glance-preview__body {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    font-size: 13px;
    line-height: 1.65;
}

.glance-preview__text {
    white-space: pre-wrap;
    word-break: break-word;
}

.glance-preview__file {
    display: flex;
    flex-direction: column;
    gap: 10px;
}

/* 媒体区：图片 / 视频按可用宽度铺开（`max-height` 留出下面的动作行和元信息条）。 */
.glance-preview__media {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 60px;
}

.glance-preview__image,
.glance-preview__video {
    max-width: 100%;
    max-height: 46vh;
    border-radius: 8px;
    object-fit: contain;
}

.glance-preview__audio {
    width: 100%;
}

/* 文本类文件：正文可能很长，自己滚。 */
.glance-preview__filetext {
    width: 100%;
    max-height: 46vh;
    overflow: auto;
    margin: 0;
    padding: 10px 12px;
    border-radius: 8px;
    background: rgba(var(--v-theme-on-surface), 0.04);
    font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 12px;
    line-height: 1.6;
    white-space: pre-wrap;
    word-break: break-word;
    color: inherit;
}

.glance-preview__glyph {
    color: rgba(var(--v-theme-on-surface), 0.55);
}

.glance-preview__file-name {
    font-size: 0.9rem;
    font-weight: 500;
    word-break: break-all;
}

.glance-preview__muted {
    font-size: 12px;
    color: rgba(var(--v-theme-on-surface), 0.6);
}

.glance-preview__actions {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
    border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
    padding-top: 8px;
}

.glance-preview__delete {
    color: #dc2626;
}

.glance-preview__meta {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    font-size: 11px;
    color: rgba(var(--v-theme-on-surface), 0.55);
}
</style>
