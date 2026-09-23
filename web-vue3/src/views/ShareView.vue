<script setup>
// 分享页 —— 收件人打开 `<prefix>/s/<token>` 看到的那一页。
//
// 地址里的 token 由**服务端**用上：它把 OG 标签写进 SPA 外壳再发给我们（见 lib/spa_shell.go），
// 社交平台抓到的预览卡片、和真人跑起来的这一页，是同一份 HTML、同一个地址。
//
// 这个页面**只认 token**：类型、文件名、大小、剩余有效期、要不要密码，
// 全部靠 GET /share 问出来，URL 里不重复携带（见 lib/share_token.go 的 handleShareInfo）。
//
// 接口一律用**相对路径**（不带前导斜杠），由 `axios.defaults.baseURL`（见 main.js/base.js）
// 落到 `<prefix>/` 上 —— 部署在子路径下不需要任何配置，深路径（`<prefix>/s/<token>`）也不会
// 把相对路径算到 `/s/` 底下。
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import MarkdownBody from '@/components/MarkdownBody.vue';
import CodeBlock from '@/components/CodeBlock.vue';
import {
    copyTextToClipboard,
    errorMessage,
    filePreviewKind,
    formatTimestamp,
    prettyFileSize,
    renderMarkdownHtml,
    reportShareVisit,
} from '@/util.js';
import { toast } from '@/plugins/toast';

const mdiAlertCircleOutline = 'mdi-alert-circle-outline';
const mdiCodeTags = 'mdi-code-tags';
const mdiContentCopy = 'mdi-content-copy';
const mdiDownload = 'mdi-download';
const mdiFileOutline = 'mdi-file-outline';
const mdiLanguageMarkdown = 'mdi-language-markdown';
const mdiLockOutline = 'mdi-lock-outline';

const route = useRoute();
const { t } = useI18n();

// 只认路由参数。**没有 `?t=` 兜底** —— 单地址架构下 `/s/:token` 是唯一形态，
// 站内没有任何地方会往页面地址上拼 `?t=`（接口调用里的 `?t=` 是另一回事），那是个死分支。
const token = computed(() => String(route.params.token || ''));
// 链接里可以带展示偏好（f=md）当**初始**格式，页面上仍可切换。
// ⚠️ 站内已经没有任何地方会生成这个参数了 —— 发送方那个「默认展示格式」设置删掉了
// （分享页自带 raw↔md 切换，让发送方替他选一次是多余的）。这里继续读它是为了
// 不让**已经发出去的旧链接**失效，以及留一个手工拼链接的入口。默认原文，md 是显式选择。
const linkedFormat = computed(() => (String(route.query.f || '').toLowerCase() === 'md' ? 'md' : 'raw'));

const loading = ref(true);
const info = ref(null);
const text = ref('');
const mdMode = ref('raw');
const password = ref('');
const passwordNeeded = ref(false);
const passwordError = ref(false);
const failure = ref('');

// 分享页的 401 是**预期内**的（要密码），不能让全局拦截器弹「房间鉴权」对话框
const shareConfig = (extra = {}) => ({
    ...extra,
    headers: password.value ? { 'X-Share-Password': password.value } : undefined,
    __skipRoomAuthHandling: true,
});

const isFile = computed(() => info.value?.kind === 'file');
const isText = computed(() => info.value?.kind === 'text');
const html = computed(() => (mdMode.value === 'md' ? renderMarkdownHtml(text.value) : ''));
const canToggleMd = computed(() => isText.value && Boolean(text.value));

// 文件地址用的令牌。⚠️ **不能直接用原 token** ——
// `<img>` / `<video>` / `<a download>` 是**浏览器自己发的请求，加不了 `X-Share-Password` 头**，
// 而带密码的分享要求那个头：配了密码的实例上图片/视频/下载会一律 401（文本却正常，
// 因为那条是 axios 发的）。服务端因此在验过密码后换发一个短期、无密码的**预览令牌**，
// 专门给这些地址用（见 Go 的 issuePreviewToken / Worker 的同名函数）。
// 不带密码的分享没有预览令牌，也不需要有 —— 原 token 本来就够。
const fileToken = computed(() => String(info.value?.previewToken || '') || token.value);

const fileUrl = computed(() => {
    if (!isFile.value || !info.value?.uuid) {
        return '';
    }
    const name = encodeURIComponent(info.value.name || 'file');
    return `file/${encodeURIComponent(info.value.uuid)}/${name}?t=${encodeURIComponent(fileToken.value)}`;
});
// 能就地预览吗、按哪一类渲染（image / video / audio / text，空串=不预览）。
// 判型收在 util.js 的 filePreviewKind —— **全站唯一实现**（那边注释里写了为什么）。
const previewKind = computed(() => (isFile.value ? filePreviewKind(info.value?.name) : ''));
const previewLoading = ref(false);
// ⚠️ 文本类文件**没有正文**：文件条目只有名字/大小/缩略图，正文要另发一次 GET 取回来。
const fileText = ref('');
const roomLabel = computed(() => {
    const room = String(info.value?.room || 'default');
    return room === 'default' ? t('publicRoom') : room;
});
const expiresText = computed(() => (info.value?.expiresAt ? formatTimestamp(info.value.expiresAt) : ''));
const usesText = computed(() => {
    if (!info.value?.maxUses) {
        return t('shareUsesUnlimited');
    }
    const left = Math.max(0, info.value.maxUses - (info.value.used || 0));
    return t('shareUsesLimited', { count: left });
});

function shareFailureText(code) {
    switch (code) {
        case 'share_token_invalid':
            return t('sharePageInvalid');
        case 'file_expired':
            return t('sharePageExpired');
        case 'content_not_found':
        case 'file_not_found':
            return t('sharePageNotFound');
        default:
            return '';
    }
}

async function loadText() {
    const params = { format: 'json', t: token.value };
    if (info.value?.room) {
        params.room = info.value.room;
    }
    const { data } = await axios.get(`content/${encodeURIComponent(info.value.id)}`, shareConfig({ params }));
    text.value = data?.content ?? '';
}

// 文本类文件的正文：走 fileUrl（相对路径 + token）另取一次，带上分享密码头。
// 失败**不**把整页变成错误页 —— 下面那行「名字 + 大小 + 下载」还在，收件人照样能拿走文件。
async function loadFileText() {
    if (previewKind.value !== 'text' || !fileUrl.value) {
        return;
    }
    previewLoading.value = true;
    try {
        const response = await axios.get(fileUrl.value, shareConfig({ responseType: 'text' }));
        fileText.value = typeof response.data === 'string' ? response.data : String(response.data || '');
    } catch (error) {
        fileText.value = '';
        console.warn('share file preview failed:', errorMessage(error));
    } finally {
        previewLoading.value = false;
    }
}

async function loadInfo() {
    loading.value = true;
    failure.value = '';
    fileText.value = '';
    if (!token.value) {
        failure.value = t('sharePageInvalid');
        loading.value = false;
        return;
    }
    try {
        const { data } = await axios.get('share', shareConfig({ params: { t: token.value } }));
        info.value = data;
        passwordNeeded.value = false;
        passwordError.value = false;
        mdMode.value = linkedFormat.value;
        if (data?.kind === 'text') {
            await loadText();
        } else if (data?.kind === 'file') {
            await loadFileText();
        }
    } catch (error) {
        const status = error?.response?.status;
        const code = String(error?.response?.data?.code || '');
        if (status === 401 && code === 'share_password_required') {
            // 第一次进来是「要密码」，带了密码还进来才是「密码不对」
            passwordError.value = Boolean(password.value);
            passwordNeeded.value = true;
            info.value = null;
            text.value = '';
            fileText.value = '';
            return;
        }
        failure.value = shareFailureText(code) || errorMessage(error) || t('sharePageInvalid');
    } finally {
        loading.value = false;
    }
}

function submitPassword() {
    if (!password.value) {
        return;
    }
    loadInfo();
}

async function copyContent() {
    try {
        await copyTextToClipboard(text.value);
        toast.success(t('copySuccess'));
    } catch {
        toast.error(t('copyFailedGeneral'));
    }
}

// 上报「有真人打开了这条分享」。
//
// 为什么在前端上报，而不是在服务端落地页（/s/<token>）里计数：落地页是给社交平台的
// 抓取程序看的（贴一次链接，微信/Telegram/Slack 都会去抓，而且会按自己的节奏重抓），
// 在那里计数会把机器抓取算成「有人打开」。只有执行了 JS 的这一页能证明是真人。
//
// 不做任何角色判定：token 无效或已过期时服务端本来就不会计数，而且报错也一律静默 ——
// 统计不该影响收件人看内容。
function reportVisit() {
    if (!token.value) {
        return;
    }
    reportShareVisit(token.value, { qr: String(route.query.q || '') === '1' });
}

onMounted(() => {
    reportVisit();
    loadInfo();
});

// ⚠️ 同一条路由上换 token 不会重新挂载组件。
//
// 分享页只有一条路由（`/s/:token`），从「一个分享链接」切到「另一个」时（点站内链接、
// 改地址栏、扫码后跳转），vue-router 复用同一个组件实例，**`onMounted` 不会再跑**。
// 不盯住它，页面会一直显示上一条分享的内容，连「无效 token」都显示成上一条的正文。
// 实测踩到过：8 个场景里 3 个是假绿/假红。
watch(token, () => {
    password.value = '';
    passwordNeeded.value = false;
    passwordError.value = false;
    failure.value = '';
    info.value = null;
    text.value = '';
    fileText.value = '';
    mdMode.value = linkedFormat.value;
    reportVisit();
    loadInfo();
});
</script>

<template>
    <div class="share-page">
        <v-card class="share-page__card" rounded="lg" elevation="0">
            <div class="share-page__head">
                <v-icon size="20">{{ mdiLockOutline }}</v-icon>
                <span class="share-page__title">{{ t('sharePageTitle') }}</span>
            </div>

            <v-progress-linear v-if="loading" indeterminate color="primary" class="share-page__bar" />

            <div v-if="loading" class="share-page__center">
                <v-progress-circular indeterminate size="28" width="3" color="primary" />
                <span class="share-page__muted">{{ t('sharePageLoading') }}</span>
            </div>

            <div v-else-if="passwordNeeded" class="share-page__center share-page__center--form">
                <v-icon size="34" color="primary">{{ mdiLockOutline }}</v-icon>
                <p class="share-page__hint">{{ t('sharePagePasswordHint') }}</p>
                <v-text-field
                    v-model="password"
                    :label="t('sharePagePasswordLabel')"
                    :error="passwordError"
                    :error-messages="passwordError ? t('sharePagePasswordWrong') : ''"
                    type="password"
                    variant="outlined"
                    density="comfortable"
                    autocomplete="off"
                    hide-details="auto"
                    class="share-page__field"
                    @keyup.enter="submitPassword"
                />
                <v-btn color="primary" variant="flat" :disabled="!password" @click="submitPassword">
                    {{ t('sharePagePasswordSubmit') }}
                </v-btn>
            </div>

            <div v-else-if="failure" class="share-page__center">
                <v-icon size="34" color="error">{{ mdiAlertCircleOutline }}</v-icon>
                <p class="share-page__hint">{{ failure }}</p>
            </div>

            <template v-else-if="info">
                <div class="share-page__meta">
                    <v-chip size="small" variant="tonal" label>{{ roomLabel }}</v-chip>
                    <span v-if="expiresText" class="share-page__muted">{{ t('sharePageExpires', { time: expiresText }) }}</span>
                    <span class="share-page__muted">{{ usesText }}</span>
                </div>

                <template v-if="isText">
                    <div class="share-page__toolbar">
                        <v-btn
                            v-if="canToggleMd"
                            size="small"
                            variant="text"
                            :prepend-icon="mdMode === 'md' ? mdiCodeTags : mdiLanguageMarkdown"
                            @click="mdMode = mdMode === 'md' ? 'raw' : 'md'"
                        >
                            {{ mdMode === 'md' ? t('rawText') : t('renderMarkdown') }}
                        </v-btn>
                        <v-spacer />
                        <v-btn size="small" variant="text" :prepend-icon="mdiContentCopy" @click="copyContent">
                            {{ t('copyText') }}
                        </v-btn>
                    </div>
                    <pre v-if="mdMode === 'raw'" class="share-page__raw">{{ text }}</pre>
                    <markdown-body v-else :html="html" />
                </template>

                <template v-else-if="isFile">
                    <!-- 能预览就预览：图片 / 视频 / 音频直接渲染（fileUrl 带 token，浏览器流式加载），
                         文本类文件另发一次请求取正文；都不是才退回图标。
                         预览失败不影响下面那行「名字 + 大小 + 下载」—— 收件人至少还能把文件拿走。 -->
                    <div v-if="previewKind" class="share-page__preview">
                        <v-progress-circular
                            v-if="previewLoading"
                            indeterminate
                            size="28"
                            width="3"
                            color="primary"
                        />
                        <img
                            v-else-if="previewKind === 'image'"
                            :src="fileUrl"
                            :alt="info.name"
                            class="share-page__image"
                        >
                        <video
                            v-else-if="previewKind === 'video'"
                            :src="fileUrl"
                            controls
                            preload="metadata"
                            class="share-page__video"
                        ></video>
                        <audio
                            v-else-if="previewKind === 'audio'"
                            :src="fileUrl"
                            controls
                            preload="metadata"
                            class="share-page__audio"
                        ></audio>
                        <!-- 代码文件按扩展名上色（认不出来就纯文本渲染）；见 components/CodeBlock.vue -->
                        <code-block
                            v-else-if="fileText"
                            :text="fileText"
                            :name="info.name"
                            class="share-page__filetext"
                        />
                    </div>

                    <div class="share-page__file">
                        <v-icon v-if="!previewKind" size="40" class="share-page__file-icon">{{ mdiFileOutline }}</v-icon>
                        <div class="share-page__file-meta">
                            <div class="share-page__file-name">{{ info.name }}</div>
                            <div class="share-page__muted">{{ prettyFileSize(Number(info.size || 0)) }}</div>
                        </div>
                        <v-btn color="primary" variant="flat" :prepend-icon="mdiDownload" :href="fileUrl" download>
                            {{ t('download') }}
                        </v-btn>
                    </div>
                </template>
            </template>
        </v-card>
    </div>
</template>

<style scoped>
.share-page {
    display: flex;
    align-items: flex-start;
    justify-content: center;
    min-height: 100vh;
    min-height: 100dvh;
    padding: 32px 16px;
}
.share-page__card {
    width: 100%;
    max-width: 720px;
    padding: 20px 22px 22px;
    border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}
.share-page__head {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.95rem;
    font-weight: 500;
    color: rgba(var(--v-theme-on-surface), 0.87);
}
.share-page__title {
    flex: 1;
}
.share-page__bar {
    margin-top: 12px;
}
.share-page__center {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 40px 0 28px;
    text-align: center;
}
.share-page__center--form {
    gap: 14px;
}
.share-page__hint {
    margin: 0;
    font-size: 0.875rem;
    color: rgba(var(--v-theme-on-surface), 0.7);
}
.share-page__field {
    width: 100%;
    max-width: 280px;
}
.share-page__meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    margin-top: 14px;
}
.share-page__muted {
    font-size: 0.8125rem;
    color: rgba(var(--v-theme-on-surface), 0.6);
}
.share-page__toolbar {
    display: flex;
    align-items: center;
    margin: 8px 0 4px;
}
.share-page__raw {
    margin: 0;
    max-height: 60vh;
    overflow: auto;
    padding: 12px 14px;
    border-radius: 8px;
    background: rgba(var(--v-theme-on-surface), 0.04);
    font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 0.8125rem;
    line-height: 1.6;
    white-space: pre-wrap;
    word-break: break-word;
    color: rgba(var(--v-theme-on-surface), 0.87);
}
.share-page__file {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-top: 12px;
    padding: 16px;
    border-radius: 10px;
    background: rgba(var(--v-theme-on-surface), 0.04);
}
/* 媒体区：图片/视频按卡片宽度铺开、高度封顶 —— 别把下面那行下载按钮挤出屏幕。 */
.share-page__preview {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 60px;
    margin-top: 14px;
}
.share-page__image,
.share-page__video {
    max-width: 100%;
    max-height: 62vh;
    border-radius: 8px;
    object-fit: contain;
}
.share-page__audio {
    width: 100%;
}
/* 文本类文件：正文可能很长，自己滚（滚动与配色都在 CodeBlock 里）。
   ⚠️ 媒体区是 flex，flex 项默认按内容宽 —— 这里要显式撑满。 */
.share-page__filetext {
    width: 100%;
}
.share-page__file-icon {
    color: rgba(var(--v-theme-on-surface), 0.55);
}
.share-page__file-meta {
    flex: 1;
    min-width: 0;
}
.share-page__file-name {
    font-size: 0.875rem;
    font-weight: 500;
    word-break: break-all;
    color: rgba(var(--v-theme-on-surface), 0.87);
}
</style>
