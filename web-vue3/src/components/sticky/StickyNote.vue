<script setup>import { computed, ref, watch } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useMarkdown } from '@/composables/useMarkdown.js';
import { useTaskListToggle } from '@/composables/useTaskListToggle.js';
import MarkdownBody from '@/components/MarkdownBody.vue';
import MarkdownToggle from '@/components/MarkdownToggle.vue';
import ShareLinkButton from '@/components/ShareLinkButton.vue';
import { useWebSocketStore } from '@/store/websocket';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import { SHARE_DEFAULT_TTL, copyTextToClipboard, createShareLink, deviceLabel, errorMessage, formatTimestamp, isImageName, prettyFileSize } from '@/util.js';

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
// 正文（含任务列表打勾 + 落盘）交给共享 composable —— 标准模式卡片那边是同一套逻辑。
// 复制文本也用它返回的 text：用户看到什么就复制什么。
const { text: decodedContent, onMdClick } = useTaskListToggle(props.meta, () => ws.room);
// md 渲染只接在**阅读器**（点开便签后的大视图）—— 便签卡片本身是贴纸风格，
// 面积也小，渲染排版反而破坏那个感觉。默认跟随个性化开关，阅读器右上角可临时切。
//
// 取文源必须分岔：文本便签在 meta.content 里，**文件的 FileReceive 没有 content 字段**
// （见 Go 侧 type.go），正文只能等 loadPreview 抓回来的 textPreview。
// 之前两种都接 decodedContent，于是 .md 文件永远拿到空串 —— 图标点得动，但渲染出空白。
const md = useMarkdown(
    () => (isFile.value ? displayedTextPreview.value : decodedContent.value),
    () => /\.(md|markdown|mdown|mkd)$/i.test(props.meta.name || ''),
);

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
// 便签页脚：时间与设备各由对应设置控制，与默认模式、气泡模式保持一致。
// 之前这两段是写死恒显的 —— 同一组「显示设置」在不同模式下含义不同，很费解。
// 两段都关时返回空串，模板那边靠 v-if 收起整行（.sticky-note__time 有 padding-top，
// 留着空标签会凭空多出 8px 空隙）。
const timestampLabel = computed(() => {
    const parts = [];
    if (props.meta.timestamp && app.display.timestamp) {
        parts.push(formatTimestamp(props.meta.timestamp));
    }
    if (app.display.device) {
        const device = deviceLabel(props.meta.senderDevice);
        if (device) {
            parts.push(t('stickyFromDevice', { device }));
        }
    }
    return parts.join(' ');
});
// 详情弹窗那一行 = 卡片页脚那行 + IP。便签卡片太小，塞不下 IP；而「显示发送者IP」
// 这个开关之前在 sticky 里完全没有生效点（mega / terminal / workbench 也一样）。
// 放不下的元信息落到详情里，开关的含义就在所有模式下一致了：
// 它管的是看不看得见，不是在哪看得见。
const readerTimeLabel = computed(() => {
    const parts = [timestampLabel.value];
    if (app.display.ip && props.meta.senderIP) {
        parts.push(props.meta.senderIP);
    }
    return parts.filter(Boolean).join(' · ');
});
const fileIcon = computed(() => {
    if (!props.meta.name) {
        return '📄';
    }
    if (isImageName(props.meta.name)) {
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
    if (isImageName(props.meta.name || '')) {
        return `${size} · ${t('stickyImage')}`;
    }
    return size;
});
// 文件分享：一次签发拿到**两条**地址。
//   raw  直连正文 —— 下载与预览用（分享页是 hash 路由，取不了字节）
//   page 前端分享页 —— 复制给别人用
// 一律签发 token：房间没开密码也发，否则 ttl / 次数限制会被静默丢弃。
async function ensureFileShareLinks() {
    const data = await createShareLink({ type: 'file', uuid: props.meta?.cache, ttl: SHARE_DEFAULT_TTL, maxUses: 0, room: ws.room });
    return { raw: data?.rawUrl || '', page: data?.url || '' };
}
async function downloadFile() {
    if (expired.value || downloading.value) {
        return;
    }
    downloading.value = true;
    try {
        const { raw: url } = await ensureFileShareLinks();
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
        // 复制**当前视图**的正文（切到代码 / JSON 美化 / 压缩后拿到的就是那一份）。
        // 和卡片头部那个复制按钮同一语义 —— 同一个卡片上「复制」不能有两种含义。
        await copyTextToClipboard(md.copyText);
        toast(t('copySuccess'));
    } catch (err) {
        console.error('复制失败:', err);
        toast(t('copyFailedGeneral'));
    }
}
// 卡片上曾经还有一个「复制链接」图标（mdi-link-variant），已删：
// 分享动作统一收在阅读器弹窗里的 ShareLinkButton（签名 + 自动复制 + 二维码面板），
// 卡片上再放一个只复制、不弹面板的入口，等于同一个功能两种行为。
// 跟着它一起下线的还有 copyLink() 和 contentUrl（只被它用）。
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
            srcPreview.value = (await ensureFileShareLinks()).raw;
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
            const response = await axios.get(`file/${props.meta.cache}/${encodeURIComponent(props.meta.name)}`, {
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
        const errMsg = errorMessage(error);
        if (errMsg) {
            toast(t('deleteFailedMessageMsg', { msg: errMsg }));
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
             :class="[isLink ? 'sticky-note__text--link' : '', md.html ? 'sticky-note__text--rendered' : '']"
             :title="decodedContent"
             @click="onMdClick"
        >
            <!-- 卡片上也渲染 md（任务列表 / 表格默认就是 md，见 useMarkdown）。
                 普通文本仍然走原文 —— 便签的贴纸观感靠的就是那一版。 -->
            <markdown-body v-if="md.html" :html="md.html"></markdown-body>
            <template v-else>{{ decodedContent }}</template>
        </div>
        <span v-if="timestampLabel" class="sticky-note__time">{{ timestampLabel }}</span>

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
                    <span v-if="readerTimeLabel" class="sticky-note__reader-time">{{ readerTimeLabel }}</span>
                    <v-btn icon density="compact" size="x-small" variant="text" class="sticky-note__op" @click="expanded = false">
                        <v-icon size="small">mdi-close</v-icon>
                    </v-btn>
                </div>
                <!-- 文件：名字/大小/过期**永远**显示。之前这几行跟着 `!md.available` 一起被
                     藏掉，.md 文件就变成「没有名字、没有大小、没有过期」的一张空对话框。 -->
                <template v-if="isFile">
                    <div class="sticky-note__reader-file">
                        <span class="sticky-note__fic">{{ fileIcon }}</span>
                        <span class="sticky-note__reader-name">{{ meta.name }}</span>
                        <span class="sticky-note__meta">{{ fileMetaLabel }}</span>
                    </div>
                    <div v-if="isExpirable" class="sticky-note__reader-expire" :class="{ 'sticky-note__reader-expire--past': expired }">
                        <v-icon size="x-small">mdi-clock-outline</v-icon>
                        {{ expireLabel }}
                    </div>
                </template>
                <!-- 文本便签的正文块。文件不走这里 —— 文件的正文在下面的预览块里，
                     md 开关也得跟着正文走（与标准模式的 File.vue 一致）。
                     图标放在**不滚动**的外层、正文放里层：否则内容一长往下滚，图标跟着滚走。 -->
                <div v-else class="md-preview" @click="onMdClick">
                    <markdown-toggle
                v-if="md.available"
                v-model:mode="md.mode"
                :json-available="md.jsonAvailable"
                :json-compact-available="md.jsonCompactAvailable"
                :code-available="md.codeAvailable"
            ></markdown-toggle>
                    <div
                        class="sticky-note__reader-text"
                        :class="[
                            isLink ? 'sticky-note__text--link' : '',
                            md.available ? 'sticky-note__reader-text--md' : '',
                            md.leadsWithBlock ? 'sticky-note__reader-text--block' : '',
                            md.html ? 'sticky-note__reader-text--rendered' : '',
                        ]"
                    >
                        <markdown-body v-if="md.html" :html="md.html"></markdown-body>
                        <template v-else>{{ decodedContent }}</template>
                    </div>
                </div>

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
                            <div class="md-preview">
                                <markdown-toggle
                v-if="md.available"
                v-model:mode="md.mode"
                :json-available="md.jsonAvailable"
                :json-compact-available="md.jsonCompactAvailable"
                :code-available="md.codeAvailable"
            ></markdown-toggle>
                                <!-- 渲染态：正文是裸 markdown，自己当滚动盒 -->
                                <div
                                    v-if="md.html"
                                    class="sticky-note__preview-scroll"
                                    :class="{
                                        'sticky-note__preview-scroll--md': md.available,
                                        'sticky-note__preview-scroll--block': md.leadsWithBlock,
                                    }"
                                >
                                    <markdown-body :html="md.html"></markdown-body>
                                </div>
                                <!-- 原文态：pre 自己当滚动盒。两者互斥，别套成两层滚动 -->
                                <pre
                                    v-else
                                    class="sticky-note__preview-text"
                                    :class="{ 'sticky-note__preview-text--md': md.available }"
                                >{{ displayedTextPreview }}</pre>
                            </div>
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
                    <share-link-button v-if="!isFile" :meta="meta" :icon-only="false" />
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
    /*
     * 便签底色是固定的浅色（见下面的 --c0..--c4），跟主题无关。
     * 不写死文字颜色的话会继承主题的 on-surface，深色主题下就变成浅色字配浅色底，
     * 对比度低到几乎读不出来。详情弹窗 .sticky-note__reader 一直是写死的，卡片这边漏了。
     */
    color: #444034;
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

    white-space: pre-wrap;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
}

/* 卡片上的渲染态。上面那条 4 行 clamp 是给**纯文本**的（贴纸观感），
   渲染态必须放开：否则一张表只露 4 行、复选框也点不到。改成限高 + 滚动。 */
.sticky-note__text--rendered {
    display: block;
    -webkit-line-clamp: unset;
    white-space: normal;
    max-height: 46vh;
    overflow: auto;
}

.sticky-note__text--rendered :deep(.markdown-body) {
    font-size: 12px;
    /* 便签正文本身是 500，markdown 段落跟着变粗会很难看 */
    font-weight: 400;
}

.sticky-note__text--rendered :deep(.markdown-body > :first-child) {
    margin-top: 0;
}

.sticky-note__text--rendered :deep(.markdown-body > :last-child) {
    margin-bottom: 0;
}

.sticky-note__text--rendered :deep(.markdown-body table) {
    font-size: 11px;
}

.sticky-note__text--link {
    color: #1e6bb8;    text-decoration: underline;
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
    flex-wrap: wrap;
    gap: 8px;
    row-gap: 4px;
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

/* 浮动图标的定位基准 —— MarkdownToggle 内部是 absolute。
   这一层**不滚动**，滚动交给里面的正文盒：图标才不会跟着内容滚走。
   注意别在这里加 overflow —— 图标是 absolute 且高 24px，容器一旦是 0 高 + overflow:auto
   （便签裸文本没内容时就是这样），图标会被整个裁掉：DOM 在、visibility 也是 visible，
   但屏幕上什么都没有。这类「看不见」最难查，因为它不报错。 */
.md-preview {
    position: relative;
}

/* 图标浮在右上角，正文得给它让位 —— 加在**滚动盒**上，因为滚动条永远贴着滚动盒的右沿，
   加在外层不滚动的盒子上是白加（滚动条不会跟着挪）。
   数值来自 MarkdownToggle 的两个 --md-toggle-* 变量，改图标尺寸只改那一处。 */
.sticky-note__reader-text--md::before,
.sticky-note__preview-text--md::before,
.sticky-note__preview-scroll--md::before {
    content: '';
    float: right;
    width: var(--md-toggle-gutter);
    /* 22px 给不支持 lh 单位的浏览器兜底；下面一行才是准的（正好一个行高） */
    height: 22px;
    height: var(--md-toggle-height);
}

/* 同 Text.vue：正文以 <pre> 开头时浮动占位会把它挤成窄列，改成给 pre 留右边距。 */
.sticky-note__reader-text--block::before,
.sticky-note__preview-text--block::before,
.sticky-note__preview-scroll--block::before {
    float: none;
    width: 0;
    height: 0;
}
.sticky-note__reader-text--block :deep(pre:first-child),
.sticky-note__preview-text--block :deep(pre:first-child),
.sticky-note__preview-scroll--block :deep(pre:first-child) {
    padding-right: var(--md-toggle-gutter);
}


/* 渲染 markdown 时收掉 white-space: pre-wrap —— 它是给纯文本保留换行的，
   套在 HTML 结构上会凭空多出空白。 */
.sticky-note__reader-text--rendered {
    white-space: normal;
}

/* 渲染态的滚动盒。原文态不用它：那边的 pre 自带 max-height + overflow。 */
.sticky-note__preview-scroll {
    max-height: 40vh;
    overflow-y: auto;
}

/* 滚动条跟着便签配色走：阅读器底色是固定的暖色（#fff8c5 这一套），
   浏览器默认那条灰白滚动条压在上面很出戏。
   Chrome / Safari 走 ::-webkit-scrollbar；Firefox 没有对应写法，回落默认样式（可接受）。
   ⚠️ 不要同时写标准的 scrollbar-color / scrollbar-width —— Chrome 一旦认了那两个，
   就会忽略下面的 ::-webkit-scrollbar，宽度不再可控，上面那份「让位」就算错了。 */
.sticky-note__reader-text::-webkit-scrollbar,
.sticky-note__preview-text::-webkit-scrollbar,
.sticky-note__preview-scroll::-webkit-scrollbar {
    width: 8px;
    height: 8px;
}

.sticky-note__reader-text::-webkit-scrollbar-track,
.sticky-note__preview-text::-webkit-scrollbar-track,
.sticky-note__preview-scroll::-webkit-scrollbar-track {
    background: transparent;
}

.sticky-note__reader-text::-webkit-scrollbar-thumb,
.sticky-note__preview-text::-webkit-scrollbar-thumb,
.sticky-note__preview-scroll::-webkit-scrollbar-thumb {
    background: rgba(68, 64, 42, 0.45);
    border-radius: 4px;
}

.sticky-note__reader-text::-webkit-scrollbar-thumb:hover,
.sticky-note__preview-text::-webkit-scrollbar-thumb:hover,
.sticky-note__preview-scroll::-webkit-scrollbar-thumb:hover {
    background: rgba(68, 64, 42, 0.62);
}
</style>