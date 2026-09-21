<script setup>import { computed, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { toast } from '@/plugins/toast';
import { useMarkdown } from '@/composables/useMarkdown.js';
import MarkdownBody from '@/components/MarkdownBody.vue';
import MarkdownToggle from '@/components/MarkdownToggle.vue';
import ShareLinkButton from '@/components/ShareLinkButton.vue';
import { copyTextToClipboard, deviceLabel, errorMessage, formatTimestamp } from '@/util.js';

const mdiCellphone = 'mdi-cellphone';
const mdiChevronRight = 'mdi-chevron-right';
const mdiClockOutline = 'mdi-clock-outline';
const mdiClose = 'mdi-close';
const mdiContentCopy = 'mdi-content-copy';
const mdiDesktopTower = 'mdi-desktop-tower';
const mdiIpNetworkOutline = 'mdi-ip-network-outline';
const mdiCodeTags = 'mdi-code-tags';
const mdiLanguageMarkdown = 'mdi-language-markdown';
const mdiPound = 'mdi-pound';
const props = defineProps({
    meta: {
        type: Object,
        default: () => ({}),
    },
});
const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();
const expand = ref(false);
function decodeHtmlEntities(text) {
    const textArea = document.createElement('textarea');
    textArea.innerHTML = text;
    return textArea.value;
}
const decodedContent = computed(() => decodeHtmlEntities(props.meta.content || ''));

// md 渲染：默认跟随个性化里的开关，内容旁的按钮可以临时覆盖这一条（见 useMarkdown）。
// v-html 的内容在 renderMarkdownHtml 里已经过了一遍 DOMPurify（内容是别人发的）。
const md = useMarkdown(() => decodedContent.value);
const decodedContentPreview = computed(() => decodeHtmlEntities(props.meta.content || ''));
function deviceIcon(type) {
    const lowerType = (type || '').toLowerCase();
    if (lowerType.includes('mobile') || lowerType.includes('phone') || lowerType.includes('tablet') || lowerType.includes('ios') || lowerType.includes('android')) {
        return mdiCellphone;
    }
    return mdiDesktopTower;
}
async function copyToClipboard(textToCopy, successMessageKey = 'copySuccess', errorMessageKey = 'copyFailedGeneral') {
    try {
        await copyTextToClipboard(textToCopy);
        toast(t(successMessageKey));
    } catch (err) {
        console.error('复制失败:', err);
        toast(t(errorMessageKey));
    }
}
function copyText() {
    copyToClipboard(decodedContent.value, 'copySuccess');
}
async function deleteItem() {
    await axios.delete(`revoke/${props.meta.id}`, {
        params: new URLSearchParams([['room', ws.room]]),
    }).then(() => {
        toast(t('deleteSuccessText'));
    }).catch(error => {
        const errMsg = errorMessage(error);
        if (errMsg) {
            toast(t('deleteFailedMessageMsg', { msg: errMsg }));
        } else {
            toast(t('deleteFailedMessage'));
        }
    });
}
</script>

<template>
    <v-hover v-slot="{ isHovering, props }">
        <v-card :elevation="isHovering ? 10 : 2" v-bind="props" class="timeline-card timeline-card--text timeline-card--id-float mb-3 transition-swing" :class="{ 'timeline-card--dark': isDark }">
            <div v-if="meta.id" class="text-caption text-grey-darken-1 timeline-card__id-float">
                <v-icon size="x-small" class="mr-1">{{ mdiPound }}</v-icon>{{ meta.id }}
            </div>
            <v-card-text>
                <div class="d-flex flex-row align-start">
                    <div class="flex-grow-1" style="min-width: 0">
                        <div class="text-caption d-flex flex-nowrap align-center mb-2 timeline-card__meta" v-if="meta.timestamp && (app.display.timestamp || app.display.device || app.display.ip)">
                            <v-chip size="x-small" label variant="flat" color="primary" class="mr-2 flex-shrink-0">{{ t('textMessage') }}</v-chip>
                            <template v-if="app.display.timestamp">
                                <span class="mr-3 text-no-wrap flex-shrink-0"><v-icon size="x-small" class="mr-1">{{ mdiClockOutline }}</v-icon>{{ formatTimestamp(meta.timestamp) }}</span>
                            </template>
                            <template v-if="app.display.device && meta.senderDevice?.type">
                                <span class="mr-3 text-no-wrap flex-shrink-0"><v-icon size="x-small" class="mr-1">{{ deviceIcon(meta.senderDevice.type) }}</v-icon>{{ deviceLabel(meta.senderDevice) }}</span>
                            </template>
                            <template v-if="app.display.ip && meta.senderIP">
                                <span class="text-no-wrap flex-shrink-0"><v-icon size="x-small" class="mr-1">{{ mdiIpNetworkOutline }}</v-icon>{{ meta.senderIP }}</span>
                            </template>
                        </div>
                        <div class="text-body-2 text-medium-emphasis d-flex align-center timeline-card__preview" @click="expand = !expand">
                            <v-icon size="small" class="me-1 timeline-card__expand-icon flex-shrink-0" :class="{ 'timeline-card__expand-icon--open': expand }">{{ mdiChevronRight }}</v-icon>
                            <span class="text-truncate flex-grow-1">{{ decodedContentPreview }}</span>
                            <div class="d-flex flex-nowrap align-center timeline-card__icon-row timeline-card__preview-actions" @click.stop>
                                <v-tooltip v-if="app.display.cardCopy" :text="t('copyText')" location="top">
                                    <template v-slot:activator="{ props }">
                                        <v-btn v-bind="props" icon density="compact" variant="text" color="grey" class="timeline-card__icon-button" @click.stop="copyText">
                                            <v-icon>{{mdiContentCopy}}</v-icon>
                                        </v-btn>
                                    </template>
                                </v-tooltip>
                                <share-link-button :meta="meta" class="timeline-card__icon-button" />
                                <v-tooltip v-if="app.display.cardDelete" :text="t('delete')" location="top">
                                    <template v-slot:activator="{ props }">
                                        <v-btn v-bind="props" icon density="compact" variant="text" color="grey" class="timeline-card__icon-button" @click.stop="deleteItem">
                                            <v-icon>{{mdiClose}}</v-icon>
                                        </v-btn>
                                    </template>
                                </v-tooltip>
                            </div>
                        </div>
                    </div>
                </div>
                <v-expand-transition>
                    <div v-show="expand">
                        <v-divider class="my-2"></v-divider>
                                                <div class="md-preview" :class="{ 'md-preview--md': md.available }">
                            <markdown-toggle v-if="md.available" v-model:mode="md.mode"></markdown-toggle>
                            <markdown-body v-if="md.html" :html="md.html"></markdown-body>
                            <div v-else style="white-space: pre-wrap; word-break: break-all;">{{ decodedContent }}</div>
                        </div>
                    </div>
                </v-expand-transition>
            </v-card-text>

        </v-card>
    </v-hover>
</template>

<style scoped>
.timeline-card :deep(.v-card-text) {
    padding: 16px 24px 20px;
}

.timeline-card {
    border-radius: 22px;
    border: 1px solid rgba(148, 163, 184, 0.26);
    overflow: hidden;
    background: rgba(255, 255, 255, 0.9);
    transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.timeline-card--dark {
    border-color: rgba(71, 85, 105, 0.72);
    background: rgba(15, 23, 42, 0.9);
}

.timeline-card--text {
    box-shadow: 0 14px 32px rgba(15, 23, 42, 0.06);
}

.timeline-card--text::before {
    content: '';
    display: block;
    height: 4px;
    background: linear-gradient(90deg, #0ea5e9, #14b8a6);
}

.timeline-card__meta {
    color: rgba(71, 85, 105, 0.9);
    overflow: visible;
}

.timeline-card__expand-icon {
    transition: transform 0.2s ease;
    vertical-align: -0.12em;
}

.timeline-card__expand-icon--open {
    transform: rotate(90deg);
}

.timeline-card__preview {
    cursor: pointer;
    margin-top: 0.25rem;
}

/* G: ID 固定右上角,操作按钮与预览行同行 */
.timeline-card--id-float {
    position: relative;
}

.timeline-card__id-float {
    position: absolute;
    top: 0.4rem;
    right: 1.5rem;
    z-index: 1;
    pointer-events: none;
}

.timeline-card__preview-actions {
    margin-left: 0.5rem;
    flex-shrink: 0;
}

.timeline-card__icon-row {
    flex-wrap: nowrap;
    white-space: nowrap;
}

.timeline-card__icon-button {
    background: rgba(248, 250, 252, 0.92);
    margin-left: 0.125rem;
    flex: 0 0 auto;
}

.timeline-card--dark .timeline-card__meta,
.timeline-card--dark .timeline-card__preview,
.timeline-card--dark .text-grey {
    color: rgba(226, 232, 240, 0.72) !important;
}

.timeline-card--dark .timeline-card__icon-button {
    background: rgba(30, 41, 59, 0.92);
}

/* 浮动图标的定位基准 —— MarkdownToggle 内部是 absolute */
.md-preview {
    position: relative;
}

/* 有 md 图标时给图标让位。这一层既没底色也不是滚动盒（正文是裸 div），
   所以直接加在这里最省事 —— 数值见 MarkdownToggle 的两个 --md-toggle-* 变量。 */
/* 只在图标所在的高度内让位（见 MarkdownToggle 的两个 --md-toggle-* 变量）。
   以前是给整个容器 padding-right，把每一行都压窄 64px。 */
.md-preview--md::before {
    content: '';
    float: right;
    width: var(--md-toggle-gutter);
    /* 22px 给不支持 lh 单位的浏览器兜底；下面一行才是准的（正好一个行高） */
    height: 22px;
    height: var(--md-toggle-height);
}
</style>
