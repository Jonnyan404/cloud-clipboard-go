<script setup>
// 分享：一个图标，一个面板。
//
// 「复制链接」和「二维码」以前是两个图标，但它们产出的**是同一个 URL**（分享页地址），
// 区别只剩呈现方式 —— 拆成两个等于让用户先做一个没有意义的决定。现在合成一条路径：
// 点图标 →（可选：有效期/次数/密码设置框）→ 链接进剪贴板 + 弹出面板，
// 面板里二维码、链接、复制按钮、有效期/次数一次给全。
//
// 「默认展示格式」这个设置**故意没有**：分享页自己就带 raw↔md 切换，收件人当场就能换，
// 发送方再替他选一次是多余的一道决定（选错了收件人还得自己找按钮换回来）。
//
// 为什么收成一个组件：这段东西（图标 + 两个弹窗 + 有效期滑块的全部样式）以前在
// received-item/Text.vue 和 received-item/File.vue 里逐字节各存了一份 —— 改一处要改两处，
// 而且其余五个模式也要接同一个入口，再抄下去就是七份。
//
// 调用点两种形态：
//     <share-link-button :meta="meta" class="timeline-card__icon-button" />   ← 卡片图标
//     <share-link-button :meta="detailItem" :icon-only="false" />             ← 弹窗动作行
// 卡片形态是否显示由组件内部按 `app.display.cardShare` 判断，调用点不用管。
//
// ⚠️ 弹窗（v-dialog）一律 teleport 出应用子树，所以这里所有样式都必须挂在自己身上，
// 不能指望祖先的类或 CSS 变量（见仓库里那条覆盖层约定）。样式块因此是 scoped 的
// 本组件样式，而不是留在卡片里。
import { computed, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import QrcodeVue from 'qrcode.vue';
import {
    SHARE_DEFAULT_TTL_MINUTES,
    SHARE_MAX_TTL_MINUTES,
    SHARE_MIN_TTL_MINUTES,
    buildCleanAbsoluteRouteUrl,
    copyTextToClipboard,
    createShareLink,
    formatShareDuration,
    formatTimestamp,
    minutesToShareTTL,
    normalizeShareMaxUses,
    normalizeShareTTL,
} from '@/util.js';

const mdiContentCopy = 'mdi-content-copy';
const mdiShareVariant = 'mdi-share-variant';

// 根节点不止一个（图标 + 两个弹窗），class 没法自动落到按钮上 —— 关掉继承，
// 在按钮上自己把 `$attrs` 展开进去，调用点才能照旧写 `class="..."`。
// ⚠️ 只能合成**一个** v-bind：同一元素上写两个 `v-bind="..."` 会被 SFC 解析器判成
// "Duplicate attribute"（`mergeProps` 那套是运行时行为，编译期先拦）。
// tooltip 的 activator 属性里只有事件和 `aria-describedby`、**没有 class**，
// 所以 `{ ...activatorProps, ...$attrs }` 不会互相覆盖。
defineOptions({ inheritAttrs: false });

const props = defineProps({
    meta: {
        type: Object,
        default: () => ({}),
    },
    // 两种形态，对应两种入口：
    //   true （默认）= 卡片右上角那排里的纯图标按钮，靠 `app.display.cardShare` 控制显隐；
    //   false       = 详情 / 阅读器弹窗动作行里的带文字按钮，**不受开关管** ——
    //                 那个开关叫「卡片图标」，只管卡片上那排图标（见 data/displayToggles.js）。
    iconOnly: {
        type: Boolean,
        default: true,
    },
});

const app = useAppStore();
const ws = useWebSocketStore();
const { t } = useI18n();

const shareResultVisible = ref(false);
const shareDialogVisible = ref(false);
const shareForm = ref({ ttlMinutes: SHARE_DEFAULT_TTL_MINUTES, maxUses: 0, password: '' });
const shareTtlMinMinutes = SHARE_MIN_TTL_MINUTES;
const shareTtlMaxMinutes = SHARE_MAX_TTL_MINUTES;
const shareUrlLoading = ref(false);
const shareContentUrl = ref('');
const lastShareMeta = ref(null);

// 兜底地址：服务端现在一律签发 token 并回分享页地址，只有在拿不到 `url` 时才用这条。
const contentUrl = computed(() => {
    const roomQuery = ws.room ? `?room=${encodeURIComponent(ws.room)}` : '';
    const id = props.meta?.id ?? '';
    return buildCleanAbsoluteRouteUrl(`content/${id}${roomQuery}`, app?.config?.server?.prefix || '');
});
const shareTtlSeconds = computed(() => minutesToShareTTL(shareForm.value.ttlMinutes));
const shareTtlLabel = computed(() => formatShareDuration(shareTtlSeconds.value, (key, params) => t(key, params)));
const shareTtlProgress = computed(() => {
    const min = shareTtlMinMinutes;
    const max = shareTtlMaxMinutes;
    const value = Number(shareForm.value.ttlMinutes);
    if (!Number.isFinite(value) || max <= min) {
        return 0;
    }
    const ratio = (value - min) / (max - min);
    return Math.max(0, Math.min(100, ratio * 100));
});
const shareTtlPresets = computed(() => [
    { minutes: 15, label: t('shareDurationMinutes', { minutes: 15 }) },
    { minutes: 60, label: t('shareDurationHours', { hours: 1 }) },
    { minutes: 360, label: t('shareDurationHours', { hours: 6 }) },
    { minutes: 1440, label: t('shareDurationHours', { hours: 24 }) },
]);
function onShareTtlInput(event) {
    const next = Number(event && event.target ? event.target.value : shareForm.value.ttlMinutes);
    shareForm.value.ttlMinutes = Number.isFinite(next) ? next : SHARE_DEFAULT_TTL_MINUTES;
}
function openShareDialog() {
    // 「分享时弹出设置框」关掉时，直接用设置里存好的默认值建链接，不弹框。
    // 先把默认值灌进表单再走同一条确认路径 —— 这样两条路只有一个建链接的地方。
    if (!app.display.shareDialog) {
        shareForm.value = { ...app.shareDefaults };
        confirmShareDialog();
        return;
    }
    shareForm.value = {
        ttlMinutes: SHARE_DEFAULT_TTL_MINUTES,
        maxUses: 0,
        password: '',
    };
    shareDialogVisible.value = true;
}
async function copyShareLink() {
    if (shareContentUrl.value) {
        await copyToClipboard(shareContentUrl.value, 'copySuccess');
    }
}
async function confirmShareDialog() {
    const ttl = normalizeShareTTL(shareTtlSeconds.value);
    const maxUses = normalizeShareMaxUses(shareForm.value.maxUses);
    const password = String(shareForm.value.password || '').trim();
    shareUrlLoading.value = true;
    try {
        const data = await createShareLink({
            type: 'content',
            id: props.meta?.id,
            ttl,
            maxUses,
            password,
            room: ws.room,
        });
        // 服务端一律签发 token 并返回分享页地址，前端不再往上挂任何展示参数 ——
        // 展示格式由分享页自己切换（见本文件顶部的说明）。
        const url = data?.url || contentUrl.value;
        shareContentUrl.value = url;
        lastShareMeta.value = {
            ttl: data?.ttl ?? ttl,
            maxUses: data?.maxUses ?? maxUses,
            expiresAtText: formatTimestamp(data?.expiresAt || (Math.floor(Date.now() / 1000) + ttl)),
            usesText: (data?.maxUses ?? maxUses) > 0
                ? t('shareUsesLimited', { count: data?.maxUses ?? maxUses })
                : t('shareUsesUnlimited'),
        };
        shareDialogVisible.value = false;
        shareResultVisible.value = true;
        await copyToClipboard(url, 'copySuccess');
    } catch (error) {
        console.error('生成分享链接失败:', error);
        toast(t('copyFailedGeneral'));
    } finally {
        shareUrlLoading.value = false;
    }
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
</script>

<template>
    <!-- 卡片图标形态：纯图标 + tooltip，受「卡片图标 → 分享链接」开关控制 -->
    <v-tooltip v-if="iconOnly && app.display.cardShare" :text="t('shareLink')" location="top">
        <template v-slot:activator="{ props: activatorProps }">
            <v-btn
                v-bind="{ ...activatorProps, ...$attrs }"
                icon
                density="compact"
                variant="text"
                color="grey"
                @click.stop="openShareDialog"
            >
                <v-icon>{{ mdiShareVariant }}</v-icon>
            </v-btn>
        </template>
    </v-tooltip>

    <!-- 弹窗动作行形态：图标 + 文字。标签本身就说清楚了，不用再套 tooltip。 -->
    <v-btn
        v-else-if="!iconOnly"
        v-bind="$attrs"
        variant="text"
        size="small"
        @click.stop="openShareDialog"
    >
        <v-icon start size="small">{{ mdiShareVariant }}</v-icon>{{ t('shareLink') }}
    </v-btn>

    <v-dialog v-model="shareDialogVisible" max-width="420" @keydown.enter.prevent="confirmShareDialog">
        <v-card>
            <v-card-title class="text-h5">{{ t('shareLinkSettings') }}</v-card-title>
            <v-card-text>
                <div class="text-body-2 mb-3 text-medium-emphasis">{{ t('shareLinkSettingsHint') }}</div>
                <div class="mb-1 d-flex justify-space-between align-center">
                    <span class="text-subtitle-2">{{ t('shareExpireIn') }}</span>
                    <span class="text-body-2 text-primary font-weight-medium">{{ shareTtlLabel }}</span>
                </div>
                <div class="share-ttl-control mb-2">
                    <input
                        class="share-ttl-range"
                        type="range"
                        :min="shareTtlMinMinutes"
                        :max="shareTtlMaxMinutes"
                        :step="1"
                        :value="shareForm.ttlMinutes"
                        :aria-label="t('shareExpireIn')"
                        :aria-valuemin="shareTtlMinMinutes"
                        :aria-valuemax="shareTtlMaxMinutes"
                        :aria-valuenow="shareForm.ttlMinutes"
                        :aria-valuetext="shareTtlLabel"
                        @input="onShareTtlInput"
                    >
                    <div class="share-ttl-progress" :style="{ width: shareTtlProgress + '%' }"></div>
                </div>
                <div class="d-flex flex-wrap mb-2" style="gap: 6px;">
                    <v-chip
                        v-for="preset in shareTtlPresets"
                        :key="preset.minutes"
                        small
                        label
                        :variant="shareForm.ttlMinutes !== preset.minutes ? 'outlined' : 'flat'"
                        :color="shareForm.ttlMinutes === preset.minutes ? 'primary' : undefined"
                        class="share-ttl-chip"
                        @click="shareForm.ttlMinutes = preset.minutes"
                    >{{ preset.label }}</v-chip>
                </div>
                <div class="text-caption text-medium-emphasis d-flex justify-space-between mb-4">
                    <span>{{ t('shareTtlMinLabel') }}</span>
                    <span>{{ t('shareTtlMaxLabel') }}</span>
                </div>
                <v-text-field
                    v-model.number="shareForm.maxUses"
                    type="number"
                    min="0"
                    max="1000"
                    :label="t('shareMaxUses')"
                    :hint="t('shareMaxUsesHint')"
                    persistent-hint
                    density="compact"
                    variant="outlined"
                ></v-text-field>

                <v-text-field
                    v-model="shareForm.password"
                    type="password"
                    autocomplete="new-password"
                    :label="t('sharePasswordLabel')"
                    :hint="t('sharePasswordHint')"
                    persistent-hint
                    density="compact"
                    variant="outlined"
                    class="mt-4"
                ></v-text-field>
            </v-card-text>
            <v-card-actions>
                <v-spacer></v-spacer>
                <v-btn variant="text" @click="shareDialogVisible = false">{{ t('cancel') }}</v-btn>
                <v-btn color="primary" variant="text" :loading="shareUrlLoading" @click="confirmShareDialog">
                    {{ t('generateAndCopy') }}
                </v-btn>
            </v-card-actions>
        </v-card>
    </v-dialog>

    <!-- 分享结果面板：复制与二维码合并后的唯一落点 -->
    <v-dialog v-model="shareResultVisible" max-width="340">
        <v-card>
            <v-card-title class="text-h5 justify-center">{{ t('shareLink') }}</v-card-title>
            <v-card-text class="text-center pa-4">
                <v-progress-circular v-if="shareUrlLoading" indeterminate color="primary" class="my-8"></v-progress-circular>
                <template v-else>
                    <qrcode-vue :value="shareContentUrl" :size="200" level="H" />
                    <div class="text-caption mt-2" style="word-break: break-all;">{{ shareContentUrl }}</div>
                    <div v-if="lastShareMeta" class="text-caption text-medium-emphasis mt-2">
                        {{ t('shareMetaSummary', lastShareMeta) }}
                    </div>
                </template>
            </v-card-text>
            <v-card-actions>
                <v-spacer></v-spacer>
                <v-btn color="primary" variant="text" :prepend-icon="mdiContentCopy" @click="copyShareLink">{{ t('copyLink') }}</v-btn>
                <v-btn variant="text" @click="shareResultVisible = false">{{ t('close') }}</v-btn>
            </v-card-actions>
        </v-card>
    </v-dialog>
</template>

<style scoped>
.share-ttl-control {
    position: relative;
    height: 28px;
    display: flex;
    align-items: center;
    padding: 0 2px;
}

.share-ttl-control::before {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    height: 6px;
    border-radius: 999px;
    background: rgba(148, 163, 184, 0.35);
}

.share-ttl-progress {
    position: absolute;
    left: 0;
    height: 6px;
    border-radius: 999px;
    background: var(--v-primary-base, #1976d2);
    pointer-events: none;
    max-width: 100%;
}

.share-ttl-range {
    position: relative;
    z-index: 1;
    width: 100%;
    margin: 0;
    appearance: none;
    -webkit-appearance: none;
    background: transparent;
    height: 28px;
    cursor: pointer;
}

.share-ttl-range:focus {
    outline: none;
}

.share-ttl-range::-webkit-slider-runnable-track {
    height: 6px;
    background: transparent;
    border-radius: 999px;
}

.share-ttl-range::-moz-range-track {
    height: 6px;
    background: transparent;
    border-radius: 999px;
}

.share-ttl-range::-webkit-slider-thumb {
    -webkit-appearance: none;
    appearance: none;
    width: 18px;
    height: 18px;
    margin-top: -6px;
    border-radius: 50%;
    background: var(--v-primary-base, #1976d2);
    border: 2px solid #fff;
    box-shadow: 0 1px 4px rgba(15, 23, 42, 0.35);
}

.share-ttl-range::-moz-range-thumb {
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: var(--v-primary-base, #1976d2);
    border: 2px solid #fff;
    box-shadow: 0 1px 4px rgba(15, 23, 42, 0.35);
}

.share-ttl-chip {
    cursor: pointer;
}
</style>
