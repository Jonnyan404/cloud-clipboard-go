<template>
    <v-hover v-slot:default="{ hover }">
        <v-card :elevation="hover ? 10 : 2" class="timeline-card timeline-card--text mb-3 transition-swing" :class="{ 'timeline-card--dark': $vuetify.theme.dark }">
            <v-card-text>
                <div class="d-flex flex-row align-start">
                    <div class="flex-grow-1 mr-2" style="min-width: 0">
                        <div class="d-flex flex-wrap align-center mb-2 timeline-card__meta" v-if="meta.timestamp && ($root.showTimestamp || $root.showDeviceInfo || $root.showSenderIP)">
                            <v-chip x-small label color="primary" text-color="white" class="mr-2 mb-1">{{ $t('textMessage') }}</v-chip>
                            <template v-if="$root.showTimestamp">
                                <span class="mr-3 mb-1"><v-icon small class="mr-1">{{ mdiClockOutline }}</v-icon>{{ formatTimestamp(meta.timestamp) }}</span>
                            </template>
                            <template v-if="$root.showDeviceInfo && meta.senderDevice?.type">
                                <span class="mr-3 mb-1"><v-icon small class="mr-1">{{ deviceIcon(meta.senderDevice.type) }}</v-icon>{{ meta.senderDevice.os || meta.senderDevice.type }}</span>
                            </template>
                            <template v-if="$root.showSenderIP && meta.senderIP">
                                <span class="mb-1"><v-icon small class="mr-1">{{ mdiIpNetworkOutline }}</v-icon>{{ meta.senderIP }}</span>
                            </template>
                        </div>
                        <div class="title text-truncate text--primary timeline-card__title" @click="expand = !expand">
                            {{ $t('textMessage') }}<v-icon>{{expand ? mdiChevronUp : mdiChevronDown}}</v-icon>
                        </div>
                        <div class="body-2 text--secondary timeline-card__preview text-truncate" @click="expand = !expand">{{ decodedContentPreview }}</div>
                    </div>
                    <div class="align-self-start text-no-wrap d-flex flex-column align-end timeline-card__actions">
                        <div v-if="meta.id" class="caption grey--text text--darken-1 mb-2">
                            <v-icon small class="mr-1">{{ mdiPound }}</v-icon>{{ meta.id }}
                        </div>
                        <div>
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="copyText">
                                        <v-icon>{{mdiContentCopy}}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('copyText') }}</span>
                            </v-tooltip>
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="openShareDialog('copy')">
                                        <v-icon>{{ mdiLinkVariant }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('copyLink') }}</span>
                            </v-tooltip>
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="openShareDialog('qr')">
                                        <v-icon>{{ mdiQrcode }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('showQrCode') }}</span>
                            </v-tooltip>
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="deleteItem">
                                        <v-icon>{{mdiClose}}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('delete') }}</span>
                            </v-tooltip>
                        </div>
                    </div>
                </div>
                <v-expand-transition>
                    <div v-show="expand">
                        <v-divider class="my-2"></v-divider>
                        <div ref="content" style="white-space: pre-wrap; word-break: break-all;">{{ decodedContent }}</div>
                    </div>
                </v-expand-transition>
            </v-card-text>

            <v-dialog v-model="shareDialogVisible" max-width="420" @keydown.enter.prevent="confirmShareDialog">
                <v-card>
                    <v-card-title class="headline">{{ $t('shareLinkSettings') }}</v-card-title>
                    <v-card-text>
                        <div class="body-2 mb-3 text--secondary">{{ $t('shareLinkSettingsHint') }}</div>
                        <div class="mb-1 d-flex justify-space-between align-center">
                            <span class="subtitle-2">{{ $t('shareExpireIn') }}</span>
                            <span class="body-2 primary--text font-weight-medium">{{ shareTtlLabel }}</span>
                        </div>
                        <div class="share-ttl-control mb-2">
                            <input
                                class="share-ttl-range"
                                type="range"
                                :min="shareTtlMinMinutes"
                                :max="shareTtlMaxMinutes"
                                :step="1"
                                :value="shareForm.ttlMinutes"
                                :aria-label="$t('shareExpireIn')"
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
                                :outlined="shareForm.ttlMinutes !== preset.minutes"
                                :color="shareForm.ttlMinutes === preset.minutes ? 'primary' : undefined"
                                class="share-ttl-chip"
                                @click="shareForm.ttlMinutes = preset.minutes"
                            >{{ preset.label }}</v-chip>
                        </div>
                        <div class="caption text--secondary d-flex justify-space-between mb-4">
                            <span>{{ $t('shareTtlMinLabel') }}</span>
                            <span>{{ $t('shareTtlMaxLabel') }}</span>
                        </div>
                        <v-text-field
                            v-model.number="shareForm.maxUses"
                            type="number"
                            min="0"
                            max="1000"
                            :label="$t('shareMaxUses')"
                            :hint="$t('shareMaxUsesHint')"
                            persistent-hint
                            dense
                            outlined
                        ></v-text-field>
                    </v-card-text>
                    <v-card-actions>
                        <v-spacer></v-spacer>
                        <v-btn text @click="shareDialogVisible = false">{{ $t('cancel') }}</v-btn>
                        <v-btn color="primary" text :loading="shareUrlLoading" @click="confirmShareDialog">
                            {{ shareDialogMode === 'qr' ? $t('generateQrCode') : $t('generateAndCopy') }}
                        </v-btn>
                    </v-card-actions>
                </v-card>
            </v-dialog>

            <v-dialog v-model="qrDialogVisible" max-width="280">
                <v-card>
                    <v-card-title class="headline justify-center">{{ $t('scanToAccess') }}</v-card-title>
                    <v-card-text class="text-center pa-4">
                        <v-progress-circular v-if="shareUrlLoading" indeterminate color="primary" class="my-8"></v-progress-circular>
                        <template v-else>
                            <qrcode-vue :value="shareContentUrl || contentUrl" :size="200" level="H" />
                            <div class="text-caption mt-2" style="word-break: break-all;">{{ shareContentUrl || contentUrl }}</div>
                            <div v-if="lastShareMeta" class="caption text--secondary mt-2">
                                {{ $t('shareMetaSummary', lastShareMeta) }}
                            </div>
                        </template>
                    </v-card-text>
                    <v-card-actions>
                        <v-spacer></v-spacer>
                        <v-btn color="primary" text @click="qrDialogVisible = false">{{ $t('close') }}</v-btn>
                    </v-card-actions>
                </v-card>
            </v-dialog>

        </v-card>
    </v-hover>
</template>

<script>
import QrcodeVue from 'qrcode.vue';
import {
    mdiChevronUp,
    mdiChevronDown,
    mdiContentCopy,
    mdiClose,
    mdiLinkVariant,
    mdiClockOutline,
    mdiDesktopTower,
    mdiCellphone,
    mdiIpNetworkOutline,
    mdiQrcode,
    mdiPound,
} from '@mdi/js';
import {
    formatTimestamp,
    buildCleanAbsoluteRouteUrl,
    createShareLink,
    copyTextToClipboard,
    SHARE_DEFAULT_TTL,
    SHARE_DEFAULT_TTL_MINUTES,
    SHARE_MIN_TTL_MINUTES,
    SHARE_MAX_TTL_MINUTES,
    normalizeShareTTL,
    normalizeShareMaxUses,
    minutesToShareTTL,
    formatShareDuration,
} from '@/util.js';

function decodeHtmlEntities(text) {
    const textArea = document.createElement('textarea');
    textArea.innerHTML = text;
    return textArea.value;
}

export default {
    name: 'received-text',
    components: { QrcodeVue },
    props: {
        meta: {
            type: Object,
            default() {
                return {};
            },
        },
    },
    data() {
        return {
            expand: false,
            qrDialogVisible: false,
            shareDialogVisible: false,
            shareDialogMode: 'copy',
            shareForm: {
                ttlMinutes: SHARE_DEFAULT_TTL_MINUTES,
                maxUses: 0,
            },
            shareTtlMinMinutes: SHARE_MIN_TTL_MINUTES,
            shareTtlMaxMinutes: SHARE_MAX_TTL_MINUTES,
            shareUrlLoading: false,
            shareContentUrl: '',
            lastShareMeta: null,
            mdiChevronUp,
            mdiChevronDown,
            mdiContentCopy,
            mdiClose,
            mdiLinkVariant,
            mdiClockOutline,
            mdiDesktopTower,
            mdiCellphone,
            mdiIpNetworkOutline,
            mdiQrcode,
            mdiPound,
        };
    },
    computed: {
        decodedContent() {
            return decodeHtmlEntities(this.meta.content || '');
        },
        decodedContentPreview() {
            return decodeHtmlEntities(this.meta.content || '');
        },
        contentUrl() {
            const roomQuery = this.$root.room ? `?room=${encodeURIComponent(this.$root.room)}` : '';
            const id = this.meta?.id ?? '';
            return buildCleanAbsoluteRouteUrl(this, `content/${id}${roomQuery}`);
        },
        needsShareProtection() {
            // 仅全局加密或当前房间加密时，才需要短期 token 与有效期/次数设置
            return Boolean(this.$root?.config?.auth);
        },
        shareTtlSeconds() {
            return minutesToShareTTL(this.shareForm.ttlMinutes);
        },
        shareTtlLabel() {
            return formatShareDuration(this.shareTtlSeconds, (key, params) => this.$t(key, params));
        },
        shareTtlProgress() {
            const min = this.shareTtlMinMinutes;
            const max = this.shareTtlMaxMinutes;
            const value = Number(this.shareForm.ttlMinutes);
            if (!Number.isFinite(value) || max <= min) {
                return 0;
            }
            const ratio = (value - min) / (max - min);
            return Math.max(0, Math.min(100, ratio * 100));
        },
        shareTtlPresets() {
            return [
                { minutes: 15, label: this.$t('shareDurationMinutes', { minutes: 15 }) },
                { minutes: 60, label: this.$t('shareDurationHours', { hours: 1 }) },
                { minutes: 360, label: this.$t('shareDurationHours', { hours: 6 }) },
                { minutes: 1440, label: this.$t('shareDurationHours', { hours: 24 }) },
            ];
        },
    },
    methods: {
        formatTimestamp,
        onShareTtlInput(event) {
            const next = Number(event && event.target ? event.target.value : this.shareForm.ttlMinutes);
            this.shareForm.ttlMinutes = Number.isFinite(next) ? next : SHARE_DEFAULT_TTL_MINUTES;
        },
        openShareDialog(mode = 'copy') {
            this.shareDialogMode = mode;
            // 未加密场景：直接复制/展示公开链接，无需有效期与次数
            if (!this.needsShareProtection) {
                this.shareUnprotected(mode);
                return;
            }
            this.shareForm = {
                ttlMinutes: SHARE_DEFAULT_TTL_MINUTES,
                maxUses: 0,
            };
            this.shareDialogVisible = true;
        },
        async shareUnprotected(mode = 'copy') {
            const url = this.contentUrl;
            this.shareContentUrl = url;
            this.lastShareMeta = null;
            if (mode === 'qr') {
                this.qrDialogVisible = true;
                return;
            }
            await this.copyToClipboard(url, 'copySuccess');
        },
        async confirmShareDialog() {
            const ttl = normalizeShareTTL(this.shareTtlSeconds);
            const maxUses = normalizeShareMaxUses(this.shareForm.maxUses);
            this.shareUrlLoading = true;
            try {
                const data = await createShareLink(this, {
                    type: 'content',
                    id: this.meta?.id,
                    ttl,
                    maxUses,
                });
                const url = data?.url || this.contentUrl;
                this.shareContentUrl = url;
                this.lastShareMeta = {
                    ttl: data?.ttl ?? ttl,
                    maxUses: data?.maxUses ?? maxUses,
                    expiresAtText: formatTimestamp(data?.expiresAt || (Math.floor(Date.now() / 1000) + ttl)),
                    usesText: (data?.maxUses ?? maxUses) > 0
                        ? this.$t('shareUsesLimited', { count: data?.maxUses ?? maxUses })
                        : this.$t('shareUsesUnlimited'),
                };
                this.shareDialogVisible = false;
                if (this.shareDialogMode === 'qr') {
                    this.qrDialogVisible = true;
                } else {
                    await this.copyToClipboard(url, 'copySuccess');
                }
            } catch (error) {
                console.error('生成分享链接失败:', error);
                this.$toast(this.$t('copyFailedGeneral'));
            } finally {
                this.shareUrlLoading = false;
            }
        },
        deviceIcon(type) {
            const lowerType = (type || '').toLowerCase();
            if (lowerType.includes('mobile') || lowerType.includes('phone') || lowerType.includes('tablet') || lowerType.includes('ios') || lowerType.includes('android')) {
                return mdiCellphone;
            }
            return mdiDesktopTower;
        },
        async copyToClipboard(textToCopy, successMessageKey = 'copySuccess', errorMessageKey = 'copyFailedGeneral') {
            try {
                await copyTextToClipboard(textToCopy);
                this.$toast(this.$t(successMessageKey));
            } catch (err) {
                console.error('复制失败:', err);
                this.$toast(this.$t(errorMessageKey));
            }
        },
        copyText() {
            this.copyToClipboard(this.decodedContent, 'copySuccess');
        },
        deleteItem() {
            this.$http.delete(`revoke/${this.meta.id}`, {
                params: new URLSearchParams([['room', this.$root.room]]),
            }).then(() => {
                this.$toast(this.$t('deleteSuccessText'));
            }).catch(error => {
                if (error.response && error.response.data.msg) {
                    this.$toast(this.$t('deleteFailedMessageMsg', { msg: error.response.data.msg }));
                } else {
                    this.$toast(this.$t('deleteFailedMessage'));
                }
            });
        },
    },
};
</script>

<style scoped>
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
.timeline-card--dark .grey--text {
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
