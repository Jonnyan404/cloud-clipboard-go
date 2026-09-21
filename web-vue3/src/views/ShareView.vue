<script setup>
// 分享页 —— 收件人打开 `/#/s?t=<token>` 看到的那一页。
//
// 为什么做成前端路由而不是服务端渲染 HTML：
//   - 两个后端（Go / Worker）就都不用碰模板，只需要会拼一个绝对地址；
//   - 静态资源本来就已经打进二进制 / 挂在 Worker assets 上，零额外部署；
//   - 渲染、密码重试、md 切换这些交互天然归前端。
//
// 这个页面**只认 token**：类型、文件名、大小、剩余有效期、要不要密码，
// 全部靠 GET /share 问出来，URL 里不重复携带（见 lib/share_token.go 的 handleShareInfo）。
//
// 所有请求用**相对路径**（不带前导斜杠）—— 与 util.js 的 createShareLink 同一约定：
// 浏览器按当前页面所在目录解析，部署在子路径（prefix）下时不需要任何配置。
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import MarkdownBody from '@/components/MarkdownBody.vue';
import {
    copyTextToClipboard,
    errorMessage,
    formatTimestamp,
    isImageName,
    prettyFileSize,
    renderMarkdownHtml,
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

const token = computed(() => String(route.query.t || ''));
// 发送方可以在链接里带展示偏好（f=md）。默认原文 —— 与站内其它地方一致，md 是显式选择。
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

const fileUrl = computed(() => {
    if (!isFile.value || !info.value?.uuid) {
        return '';
    }
    const name = encodeURIComponent(info.value.name || 'file');
    return `file/${encodeURIComponent(info.value.uuid)}/${name}?t=${encodeURIComponent(token.value)}`;
});
const isImage = computed(() => isFile.value && isImageName(info.value?.name));
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

async function loadInfo() {
    loading.value = true;
    failure.value = '';
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

onMounted(loadInfo);

// ⚠️ 同一条路由上换 token 不会重新挂载组件。
//
// 分享页只有一个 path（`/s`），token 在 query 里 —— 从「一个分享链接」切到「另一个」时
// （改地址栏 hash、点站内链接、扫码后跳转），vue-router 复用同一个组件实例，
// **`onMounted` 不会再跑**。不盯住它，页面会一直显示上一条分享的内容，
// 连「无效 token」都显示成上一条的正文。实测踩到过：8 个场景里 3 个是假绿/假红。
watch(token, () => {
    password.value = '';
    passwordNeeded.value = false;
    passwordError.value = false;
    failure.value = '';
    info.value = null;
    text.value = '';
    mdMode.value = linkedFormat.value;
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
                    <div class="share-page__file">
                        <img v-if="isImage" :src="fileUrl" :alt="info.name" class="share-page__thumb" />
                        <v-icon v-else size="40" class="share-page__file-icon">{{ mdiFileOutline }}</v-icon>
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
.share-page__thumb {
    max-width: 96px;
    max-height: 96px;
    border-radius: 8px;
    object-fit: cover;
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
