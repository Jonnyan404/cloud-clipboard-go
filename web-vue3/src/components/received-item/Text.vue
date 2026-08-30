<script setup>import { computed, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { toast } from '@/plugins/toast';
import QrcodeVue from 'qrcode.vue';
import {
    formatTimestamp,
    buildCleanAbsoluteRouteUrl,
    createShareLink,
    copyTextToClipboard,
    SHARE_DEFAULT_TTL_MINUTES,
    SHARE_MIN_TTL_MINUTES,
    SHARE_MAX_TTL_MINUTES,
    normalizeShareTTL,
    normalizeShareMaxUses,
    minutesToShareTTL,
    formatShareDuration,
} from '@/util.js';

const mdiCellphone = 'mdi-cellphone';
const mdiChevronDown = 'mdi-chevron-down';
const mdiChevronUp = 'mdi-chevron-up';
const mdiClockOutline = 'mdi-clock-outline';
const mdiClose = 'mdi-close';
const mdiContentCopy = 'mdi-content-copy';
const mdiDesktopTower = 'mdi-desktop-tower';
const mdiIpNetworkOutline = 'mdi-ip-network-outline';
const mdiLinkVariant = 'mdi-link-variant';
const mdiPound = 'mdi-pound';
const mdiQrcode = 'mdi-qrcode';
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
const qrDialogVisible = ref(false);
const shareDialogVisible = ref(false);
const shareDialogMode = ref('copy');
const shareForm = ref({ ttlMinutes: SHARE_DEFAULT_TTL_MINUTES, maxUses: 0 });
const shareTtlMinMinutes = SHARE_MIN_TTL_MINUTES;
const shareTtlMaxMinutes = SHARE_MAX_TTL_MINUTES;
const shareUrlLoading = ref(false);
const shareContentUrl = ref('');
const lastShareMeta = ref(null);
function decodeHtmlEntities(text) {
    const textArea = document.createElement('textarea');
    textArea.innerHTML = text;
    return textArea.value;
}
const decodedContent = computed(() => decodeHtmlEntities(props.meta.content || ''));
const decodedContentPreview = computed(() => decodeHtmlEntities(props.meta.content || ''));
const contentUrl = computed(() => {
    const roomQuery = ws.room ? `?room=${encodeURIComponent(ws.room)}` : '';
    const id = props.meta?.id ?? '';
    return buildCleanAbsoluteRouteUrl(`content/${id}${roomQuery}`, app?.config?.server?.prefix || '');
});
const needsShareProtection = computed(() => Boolean(app?.config?.auth));
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
function openShareDialog(mode = 'copy') {
    shareDialogMode.value = mode;
    if (!needsShareProtection.value) {
        shareUnprotected(mode);
        return;
    }
    shareForm.value = { ttlMinutes: SHARE_DEFAULT_TTL_MINUTES, maxUses: 0 };
    shareDialogVisible.value = true;
}
async function shareUnprotected(mode = 'copy') {
    const url = contentUrl.value;
    shareContentUrl.value = url;
    lastShareMeta.value = null;
    if (mode === 'qr') {
        qrDialogVisible.value = true;
        return;
    }
    await copyToClipboard(url, 'copySuccess');
}
async function confirmShareDialog() {
    const ttl = normalizeShareTTL(shareTtlSeconds.value);
    const maxUses = normalizeShareMaxUses(shareForm.value.maxUses);
    shareUrlLoading.value = true;
    try {
        const data = await createShareLink({ type: 'content', id: props.meta?.id, ttl, maxUses, room: ws.room });
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
        if (shareDialogMode.value === 'qr') {
            qrDialogVisible.value = true;
        } else {
            await copyToClipboard(url, 'copySuccess');
        }
    } catch (error) {
        console.error('生成分享链接失败:', error);
        toast(t('copyFailedGeneral'));
    } finally {
        shareUrlLoading.value = false;
    }
}
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
        if (error.response && error.response.data.msg) {
            toast(t('deleteFailedMessageMsg', { msg: error.response.data.msg }));
        } else {
            toast(t('deleteFailedMessage'));
        }
    });
}
</script>

<template>
    <v-hover v-slot="{ isHovering, props }">
        <v-card :elevation="isHovering ? 10 : 2" v-bind="props" class="timeline-card timeline-card--text mb-3 transition-swing" :class="{ 'timeline-card--dark': isDark }">
            <v-card-text>
                <div class="d-flex flex-row align-start">
                    <div class="flex-grow-1 mr-2" style="min-width: 0">
                        <div class="d-flex flex-wrap align-center mb-2 timeline-card__meta" v-if="meta.timestamp && (app.showTimestamp || app.showDeviceInfo || app.showSenderIP)">
                            <v-chip size="x-small" label variant="flat" color="primary" class="mr-2 mb-1">{{ t('textMessage') }}</v-chip>
                            <template v-if="app.showTimestamp">
                                <span class="mr-3 mb-1"><v-icon size="x-small" class="mr-1">{{ mdiClockOutline }}</v-icon>{{ formatTimestamp(meta.timestamp) }}</span>
                            </template>
                            <template v-if="app.showDeviceInfo && meta.senderDevice?.type">
                                <span class="mr-3 mb-1"><v-icon size="x-small" class="mr-1">{{ deviceIcon(meta.senderDevice.type) }}</v-icon>{{ meta.senderDevice.os || meta.senderDevice.type }}</span>
                            </template>
                            <template v-if="app.showSenderIP && meta.senderIP">
                                <span class="mb-1"><v-icon size="x-small" class="mr-1">{{ mdiIpNetworkOutline }}</v-icon>{{ meta.senderIP }}</span>
                            </template>
                        </div>
                        <div class="text-h6 text-truncate text-on-surface timeline-card__title" @click="expand = !expand">
                            {{ t('textMessage') }}<v-icon>{{expand ? mdiChevronUp : mdiChevronDown}}</v-icon>
                        </div>
                        <div class="text-body-2 text-medium-emphasis timeline-card__preview text-truncate" @click="expand = !expand">{{ decodedContentPreview }}</div>
                    </div>
                    <div class="align-self-start text-nowrap d-flex flex-column align-end timeline-card__actions">
                        <div v-if="meta.id" class="text-caption text-grey-darken-1 mb-2">
                            <v-icon size="x-small" class="mr-1">{{ mdiPound }}</v-icon>{{ meta.id }}
                        </div>
                        <div>
                            <v-tooltip :text="t('copyText')" location="bottom">
                                <template v-slot:activator="{ props }">
                                    <v-btn v-bind="props" icon density="comfortable" variant="text" color="grey" class="timeline-card__icon-button" @click="copyText">
                                        <v-icon>{{mdiContentCopy}}</v-icon>
                                    </v-btn>
                                </template>
                            </v-tooltip>
                            <v-tooltip :text="t('copyLink')" location="bottom">
                                <template v-slot:activator="{ props }">
                                    <v-btn v-bind="props" icon density="comfortable" variant="text" color="grey" class="timeline-card__icon-button" @click="openShareDialog('copy')">
                                        <v-icon>{{ mdiLinkVariant }}</v-icon>
                                    </v-btn>
                                </template>
                            </v-tooltip>
                            <v-tooltip :text="t('showQrCode')" location="bottom">
                                <template v-slot:activator="{ props }">
                                    <v-btn v-bind="props" icon density="comfortable" variant="text" color="grey" class="timeline-card__icon-button" @click="openShareDialog('qr')">
                                        <v-icon>{{ mdiQrcode }}</v-icon>
                                    </v-btn>
                                </template>
                            </v-tooltip>
                            <v-tooltip :text="t('delete')" location="bottom">
                                <template v-slot:activator="{ props }">
                                    <v-btn v-bind="props" icon density="comfortable" variant="text" color="grey" class="timeline-card__icon-button" @click="deleteItem">
                                        <v-icon>{{mdiClose}}</v-icon>
                                    </v-btn>
                                </template>
                            </v-tooltip>
                        </div>
                    </div>
                </div>
                <v-expand-transition>
                    <div v-show="expand">
                        <v-divider class="my-2"></v-divider>
                        <div style="white-space: pre-wrap; word-break: break-all;">{{ decodedContent }}</div>
                    </div>
                </v-expand-transition>
            </v-card-text>

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
                    </v-card-text>
                    <v-card-actions>
                        <v-spacer></v-spacer>
                        <v-btn text @click="shareDialogVisible = false">{{ t('cancel') }}</v-btn>
                        <v-btn color="primary" text :loading="shareUrlLoading" @click="confirmShareDialog">
                            {{ shareDialogMode === 'qr' ? t('generateQrCode') : t('generateAndCopy') }}
                        </v-btn>
                    </v-card-actions>
                </v-card>
            </v-dialog>

            <v-dialog v-model="qrDialogVisible" max-width="280">
                <v-card>
                    <v-card-title class="text-h5 justify-center">{{ t('scanToAccess') }}</v-card-title>
                    <v-card-text class="text-center pa-4">
                        <v-progress-circular v-if="shareUrlLoading" indeterminate color="primary" class="my-8"></v-progress-circular>
                        <template v-else>
                            <qrcode-vue :value="shareContentUrl || contentUrl" :size="200" level="H" />
                            <div class="text-caption mt-2" style="word-break: break-all;">{{ shareContentUrl || contentUrl }}</div>
                            <div v-if="lastShareMeta" class="text-caption text-medium-emphasis mt-2">
                                {{ t('shareMetaSummary', lastShareMeta) }}
                            </div>
                        </template>
                    </v-card-text>
                    <v-card-actions>
                        <v-spacer></v-spacer>
                        <v-btn color="primary" text @click="qrDialogVisible = false">{{ t('close') }}</v-btn>
                    </v-card-actions>
                </v-card>
            </v-dialog>

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
}

.timeline-card__title {
    cursor: pointer;
}

.timeline-card__preview {
    cursor: pointer;
    margin-top: 0.25rem;
}

.timeline-card__actions {
    min-width: 8rem;
}

.timeline-card__icon-button {
    background: rgba(248, 250, 252, 0.92);
    margin-left: 0.125rem;
}

.timeline-card--dark .timeline-card__meta,
.timeline-card--dark .timeline-card__preview,
.timeline-card--dark .timeline-card__actions,
.timeline-card--dark .text-grey {
    color: rgba(226, 232, 240, 0.72) !important;
}

.timeline-card--dark .timeline-card__icon-button {
    background: rgba(30, 41, 59, 0.92);
}

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
