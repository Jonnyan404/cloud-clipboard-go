<template>
    <v-hover v-slot:default="{ hover }">
        <v-card :elevation="hover ? 10 : 2" class="timeline-card timeline-card--file mb-3 transition-swing" :class="{ 'timeline-card--dark': $vuetify.theme.dark }">
            <v-card-text>
                <div class="d-flex flex-wrap align-center mb-2 timeline-card__meta" v-if="meta.timestamp && ($root.showTimestamp || $root.showDeviceInfo || $root.showSenderIP)">
                    <v-chip x-small label color="secondary" text-color="white" class="mr-2 mb-1">{{ $t('fileMessage') }}</v-chip>
                    <template v-if="$root.showTimestamp">
                        <span class="mr-3 mb-1"><v-icon small class="mr-1">{{ mdiClockOutline }}</v-icon>{{ formatTimestamp(meta.timestamp) }}</span>
                    </template>
                    <template v-if="$root.showDeviceInfo && meta.senderDevice && meta.senderDevice.type">
                        <span class="mr-3 mb-1"><v-icon small class="mr-1">{{ deviceIcon(meta.senderDevice.type) }}</v-icon>{{ meta.senderDevice.os || meta.senderDevice.type }}</span>
                    </template>
                    <template v-if="$root.showSenderIP && meta.senderIP">
                        <span class="mb-1"><v-icon small class="mr-1">{{ mdiIpNetworkOutline }}</v-icon>{{ meta.senderIP }}</span>
                    </template>
                </div>

                <!-- Row for Thumbnail, Title, Size/Expire, Buttons -->
                <div class="d-flex flex-row align-center">
                    <v-img
                        v-if="meta.thumbnail && (!isPreviewableVideo && !isPreviewableAudio)"
                        :src="meta.thumbnail"
                        class="mr-3 flex-grow-0 hidden-sm-and-down"
                        width="2.5rem"
                        height="2.5rem"
                        style="border-radius: 3px"
                    ></v-img>
                        <!-- 为音频文件添加专门的图标 -->
                    <v-icon
                        v-else-if="isPreviewableAudio"
                        class="mr-3 flex-grow-0 hidden-sm-and-down"
                        size="2.5rem"
                        color="grey"
                    >{{ mdiMusicNote }}</v-icon>
                    <!-- 为视频文件添加专门的图标 -->
                    <v-icon
                        v-else-if="isPreviewableVideo"
                        class="mr-3 flex-grow-0 hidden-sm-and-down"
                        size="2.5rem"
                        color="grey"
                    >{{ mdiMovie }}</v-icon>
                    <div class="flex-grow-1 mr-2" style="min-width: 0">
                        <div
                            class="title text-truncate text--primary timeline-card__title"
                            :style="{'text-decoration': expired ? 'line-through' : ''}"
                            :title="meta.name"
                        >{{meta.name}}</div>
                        <div class="caption timeline-card__file-meta">
                            {{meta.size | prettyFileSize}}
                            <template v-if="$vuetify.breakpoint.smAndDown"><br></template>
                            <template v-else>|</template>
                            {{ expired ? $t('expiredAt', { time: formatTimestamp(meta.expire) }) : $t('willExpireAt', { time: formatTimestamp(meta.expire) }) }}
                        </div>
                    </div>

                    <div class="align-self-start text-no-wrap d-flex flex-column align-end timeline-card__actions">
                        <div v-if="meta.id" class="caption grey--text text--darken-1 mb-2">
                            <v-icon small class="mr-1">{{ mdiPound }}</v-icon>{{ meta.id }}
                        </div>
                        <div class="align-self-center text-no-wrap">
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn
                                        v-on="on"
                                        icon
                                        color="grey"
                                        class="timeline-card__icon-button"
                                        :loading="downloading"
                                        :disabled="expired || downloading"
                                        @click="downloadFile"
                                    >
                                        <v-icon>{{ expired ? mdiDownloadOff : mdiDownload }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ expired ? $t('expired') : $t('download') }}</span>
                            </v-tooltip>

                            <template v-if="meta.thumbnail || isPreviewableVideo || isPreviewableAudio || isPreviewableText">
                                <v-progress-circular
                                    v-if="loadingPreview"
                                    indeterminate
                                    color="grey"
                                >{{loadedPreview / meta.size | percentage(0)}}</v-progress-circular>
                                <v-tooltip bottom>
                                    <template v-slot:activator="{ on }">
                                        <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="!expired && previewFile()">
                                                    <v-icon>{{ previewIcon }}</v-icon>
                                        </v-btn>
                                    </template>
                                    <span>{{ $t('preview') }}</span>
                                </v-tooltip>
                            </template>

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
                                    <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="deleteItem" :disabled="loadingPreview">
                                        <v-icon>{{mdiClose}}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('delete') }}</span>
                            </v-tooltip>
                        </div>
                    </div>
                </div>
                <v-expand-transition v-if="meta.thumbnail || isPreviewableVideo || isPreviewableAudio || isPreviewableText">
                    <div v-show="expand">
                        <v-divider class="my-2"></v-divider>
                        <video
                            v-if="isPreviewableVideo"
                            :src="srcPreview"
                            style="max-height:480px;max-width:100%;"
                            class="rounded d-block mx-auto"
                            controls
                            preload="metadata"
                        ></video>
                        <audio
                            v-else-if="isPreviewableAudio"
                            :src="srcPreview"
                            style="width:100%"
                            class="rounded d-block mx-auto"
                            controls
                            preload="metadata"
                        ></audio>
                        <template v-else-if="isPreviewableText">
                            <pre class="timeline-card__text-preview pa-4">{{ displayedTextPreview }}</pre>
                            <div v-if="hasTruncatedTextPreview" class="d-flex justify-space-between align-center mt-2">
                                <div class="caption text--secondary">
                                    {{ $t('textPreviewTruncated', { limit: prettyFileSize(textPreviewDisplayLimit) }) }}
                                </div>
                                <v-btn small text color="primary" @click="toggleTextPreview">
                                    {{ showFullTextPreview ? $t('collapseTextPreview') : $t('expandTextPreview') }}
                                </v-btn>
                            </div>
                        </template>
                        <img
                            v-else
                            :src="srcPreview"
                            style="max-height:480px;max-width:100%;"
                            class="rounded d-block mx-auto"
                        >
                    </div>
                </v-expand-transition>
            </v-card-text>

            <!-- Share options dialog -->
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

            <!-- QR Code Dialog -->
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
    buildCleanAbsoluteRouteUrl,
    createShareLink,
    copyTextToClipboard,
    prettyFileSize,
    percentage,
    formatTimestamp,
    SHARE_DEFAULT_TTL,
    SHARE_DEFAULT_TTL_MINUTES,
    SHARE_MIN_TTL_MINUTES,
    SHARE_MAX_TTL_MINUTES,
    normalizeShareTTL,
    normalizeShareMaxUses,
    minutesToShareTTL,
    formatShareDuration,
} from '@/util.js';
import {
    mdiContentCopy,
    mdiDownload,
    mdiDownloadOff,
    mdiClose,
    mdiImageSearchOutline,
    mdiLinkVariant,
    mdiMovieSearchOutline,
    mdiClockOutline,
    mdiDesktopTower,
    mdiCellphone,
    mdiIpNetworkOutline,
    mdiQrcode,
    mdiMusicNote,
    mdiMovie,
    mdiTextBoxSearchOutline,
    mdiPound,
} from '@mdi/js';

export default {
    name: 'received-file',
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
            textPreviewDisplayLimit: 16 * 1024,
            loadingPreview: false,
            loadedPreview: 0,
            expand: false,
            srcPreview: null,
            textPreview: '',
            showFullTextPreview: false,
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
            shareFileUrl: '',
            lastShareMeta: null,
            downloading: false,
            mdiContentCopy,
            mdiDownload,
            mdiDownloadOff,
            mdiClose,
            mdiImageSearchOutline,
            mdiLinkVariant,
            mdiMovieSearchOutline,
            mdiClockOutline,
            mdiDesktopTower,
            mdiCellphone,
            mdiIpNetworkOutline,
            mdiQrcode,
            mdiMusicNote,
            mdiMovie,
            mdiTextBoxSearchOutline,
            mdiPound,
        };
    },
    computed: {
        expired() {
            return this.$root.date.getTime() / 1000 > this.meta.expire;
        },
        isPreviewableVideo() {
            return this.meta.name.match(/\.(mp4|webm|ogv)$/gi);
        },
        isPreviewableAudio() {
            return this.meta.name.match(/\.(mp3|wav|ogg|opus|m4a|flac)$/gi);
        },
        isPreviewableText() {
            return this.meta.name.match(/\.(txt|text|md|markdown|json|log|csv|tsv|ya?ml|xml|ini|conf|cfg|toml|properties|env|gitignore|dockerfile|js|jsx|mjs|cjs|ts|tsx|vue|css|scss|sass|less|html|htm|sql|sh|bash|zsh|fish|ps1|bat|cmd|go|py|java|kt|kts|rb|php|rs|c|cc|cpp|cxx|h|hh|hpp|hxx|swift|proto)$/gi);
        },
        hasTruncatedTextPreview() {
            return this.textPreview.length > this.textPreviewDisplayLimit;
        },
        displayedTextPreview() {
            if (!this.hasTruncatedTextPreview || this.showFullTextPreview) {
                return this.textPreview;
            }
            return `${this.textPreview.slice(0, this.textPreviewDisplayLimit)}\n\n...`;
        },
        previewIcon() {
            if (this.isPreviewableVideo || this.isPreviewableAudio) {
                return mdiMovieSearchOutline;
            }
            if (this.isPreviewableText) {
                return mdiTextBoxSearchOutline;
            }
            return mdiImageSearchOutline;
        },
        contentUrl() {
            const roomQuery = this.$root.room ? `?room=${encodeURIComponent(this.$root.room)}` : '';
            const id = this.meta?.id ?? '';
            return buildCleanAbsoluteRouteUrl(this, `content/${id}${roomQuery}`);
        },
        fileUrl() {
            const cache = this.meta?.cache || '';
            const encodedFilename = encodeURIComponent(this.meta?.name || 'file');
            return buildCleanAbsoluteRouteUrl(this, `file/${cache}/${encodedFilename}`);
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
        prettyFileSize,
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
        async ensureFileShareUrl() {
            // 未加密房间可直接使用公开文件 URL
            if (!this.needsShareProtection) {
                return this.fileUrl;
            }
            // 本人预览/下载使用不限次数的短期 token，避免占用对外分享次数
            if (this.shareFileUrl) {
                return this.shareFileUrl;
            }
            const data = await createShareLink(this, {
                type: 'file',
                uuid: this.meta?.cache,
                ttl: SHARE_DEFAULT_TTL,
                maxUses: 0,
            });
            this.shareFileUrl = data?.url || '';
            return this.shareFileUrl;
        },
        async downloadFile() {
            if (this.expired || this.downloading) {
                return;
            }
            this.downloading = true;
            try {
                const url = await this.ensureFileShareUrl();
                const anchor = document.createElement('a');
                anchor.href = url;
                anchor.download = this.meta?.name || 'file';
                anchor.rel = 'noopener';
                document.body.appendChild(anchor);
                anchor.click();
                document.body.removeChild(anchor);
            } catch (error) {
                console.error('下载失败:', error);
                this.$toast(this.$t('fileFetchFailed'));
            } finally {
                this.downloading = false;
            }
        },
        async previewFile() {
            if (this.expand) {
                this.expand = false;
                return;
            }
            if (this.srcPreview || this.textPreview) {
                this.expand = true;
                return;
            }
            this.expand = true;
            if (this.isPreviewableVideo || this.isPreviewableAudio) {
                try {
                    this.srcPreview = await this.ensureFileShareUrl();
                } catch (error) {
                    console.error('生成预览链接失败:', error);
                    this.$toast(this.$t('fileFetchFailed'));
                }
            } else if (this.isPreviewableText) {
                this.showFullTextPreview = false;
                this.loadingPreview = true;
                this.loadedPreview = 0;
                this.$http.get(`file/${this.meta.cache}/${encodeURIComponent(this.meta.name)}`, {
                    responseType: 'text',
                    onDownloadProgress: e => { this.loadedPreview = e.loaded; },
                }).then(response => {
                    this.textPreview = typeof response.data === 'string' ? response.data : String(response.data || '');
                }).catch(error => {
                    if (error.response && error.response.data.msg) {
                        this.$toast(this.$t('fileFetchFailedMsg', { msg: error.response.data.msg }));
                    } else {
                        this.$toast(this.$t('fileFetchFailed'));
                    }
                }).finally(() => {
                    this.loadingPreview = false;
                });
            } else {
                this.loadingPreview = true;
                this.loadedPreview = 0;
                this.$http.get(`file/${this.meta.cache}/${encodeURIComponent(this.meta.name)}`, {
                    responseType: 'arraybuffer',
                    onDownloadProgress: e => { this.loadedPreview = e.loaded; },
                }).then(response => {
                    this.srcPreview = URL.createObjectURL(new Blob([response.data]));
                }).catch(error => {
                    if (error.response && error.response.data.msg) {
                        this.$toast(this.$t('fileFetchFailedMsg', { msg: error.response.data.msg }));
                    } else {
                        this.$toast(this.$t('fileFetchFailed'));
                    }
                }).finally(() => {
                    this.loadingPreview = false;
                });
            }
        },
        toggleTextPreview() {
            this.showFullTextPreview = !this.showFullTextPreview;
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
        deleteItem() {
            this.$http.delete(`revoke/${this.meta.id}`, {
                params: new URLSearchParams([['room', this.$root.room]]),
            }).then(() => {
                if (!this.expired && this.meta.cache) {
                    this.$http.delete(`file/${this.meta.cache}`).then(() => {
                        this.$toast(this.$t('deleteSuccessFile', { name: this.meta.name }));
                    }).catch(error => {
                        console.error('删除物理文件失败:', error);
                        if (error.response && error.response.data.msg) {
                            this.$toast(this.$t('deleteFailedFileMsg', { msg: error.response.data.msg }));
                        } else {
                            this.$toast(this.$t('deleteFailedFile'));
                        }
                    });
                } else {
                    this.$toast(this.$t('deleteSuccessFile', { name: this.meta.name }));
                }
            }).catch(error => {
                if (error.response && error.response.data.msg) {
                    this.$toast(this.$t('deleteFailedMessageMsg', { msg: error.response.data.msg }));
                } else {
                    this.$toast(this.$t('deleteFailedMessage'));
                }
            });
        },
        deviceIcon(type) {
            const lowerType = type?.toLowerCase() || '';
            if (lowerType.includes('mobile') || lowerType.includes('phone') || lowerType.includes('tablet') || lowerType.includes('ios') || lowerType.includes('android')) {
                return mdiCellphone;
            }
            return mdiDesktopTower;
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

.timeline-card--file {
    box-shadow: 0 14px 32px rgba(15, 23, 42, 0.06);
}

.timeline-card--file::before {
    content: '';
    display: block;
    height: 4px;
    background: linear-gradient(90deg, #10b981, #06b6d4);
}

.timeline-card__meta {
    color: rgba(71, 85, 105, 0.9);
}

.timeline-card__title {
    margin-bottom: 0.35rem;
}

.timeline-card__file-meta {
    color: rgba(71, 85, 105, 0.88);
}

.timeline-card__actions {
    min-width: 9rem;
}

.timeline-card__icon-button {
    background: rgba(248, 250, 252, 0.92);
    margin-left: 0.125rem;
}

.timeline-card__text-preview {
    margin: 0;
    max-height: 30rem;
    overflow: auto;
    border-radius: 14px;
    background: rgba(241, 245, 249, 0.9);
    white-space: pre-wrap;
    word-break: break-word;
    font-size: 0.875rem;
    line-height: 1.6;
}

.timeline-card--dark .timeline-card__meta,
.timeline-card--dark .timeline-card__file-meta,
.timeline-card--dark .timeline-card__actions,
.timeline-card--dark .grey--text {
    color: rgba(226, 232, 240, 0.72) !important;
}

.timeline-card--dark .timeline-card__icon-button {
    background: rgba(30, 41, 59, 0.92);
}

.timeline-card--dark .timeline-card__text-preview {
    background: rgba(30, 41, 59, 0.88);
    color: rgba(226, 232, 240, 0.92);
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
