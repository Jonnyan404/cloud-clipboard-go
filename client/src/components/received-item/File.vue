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
                                        :href="expired ? null : fileUrl"
                                        :download="expired ? null : meta.name"
                                        :disabled="expired"
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
                                    <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="copyLink">
                                        <v-icon>{{ mdiLinkVariant }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('copyLink') }}</span>
                            </v-tooltip>

                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="qrDialogVisible = true">
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
                        <pre
                            v-else-if="isPreviewableText"
                            class="timeline-card__text-preview pa-4"
                        >{{ displayedTextPreview }}</pre>
                        <div v-if="isPreviewableText && hasTruncatedTextPreview" class="d-flex justify-space-between align-center mt-2">
                            <div class="caption text--secondary">
                                {{ $t('textPreviewTruncated', { limit: prettyFileSize(textPreviewDisplayLimit) }) }}
                            </div>
                            <v-btn small text color="primary" @click="toggleTextPreview">
                                {{ showFullTextPreview ? $t('collapseTextPreview') : $t('expandTextPreview') }}
                            </v-btn>
                        </div>
                        <img
                            v-else
                            :src="srcPreview"
                            style="max-height:480px;max-width:100%;"
                            class="rounded d-block mx-auto"
                        >
                    </div>
                </v-expand-transition>
            </v-card-text>

            <!-- QR Code Dialog -->
            <v-dialog v-model="qrDialogVisible" max-width="250">
                <v-card>
                    <v-card-title class="headline justify-center">{{ $t('scanToAccess') }}</v-card-title>
                    <v-card-text class="text-center pa-4">
                        <qrcode-vue :value="contentUrl" :size="200" level="H" />
                        <div class="text-caption mt-2" style="word-break: break-all;">{{ contentUrl }}</div>
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
import QrcodeVue from 'qrcode.vue'; // <-- Import
import {
    prettyFileSize,
    percentage,
    formatTimestamp,
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
    mdiPound, // <-- Import Message ID icon
} from '@mdi/js';

export default {
    name: 'received-file',
    components: { QrcodeVue }, // <-- Register
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
            qrDialogVisible: false, // <-- Add dialog visibility flag
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
            mdiPound, // <-- Add Message ID icon
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
            const protocol = window.location.protocol;
            const host = window.location.host;
            const prefix = this.$root.config?.server?.prefix || '';
            const roomQuery = this.$root.room ? `?room=${this.$root.room}` : '';
            const id = this.meta?.id ?? '';
            return `${protocol}//${host}${prefix}/content/${id}${roomQuery}`;
        },
        fileUrl() {
            const protocol = window.location.protocol;
            const host = window.location.host;
            const prefix = this.$root.config?.server?.prefix || '';
            const cache = this.meta?.cache || '';
            const encodedFilename = encodeURIComponent(this.meta?.name || 'file');
            return `${protocol}//${host}${prefix}/file/${cache}/${encodedFilename}`;
        }
    },
    methods: {
        formatTimestamp,
        previewFile() {
            if (this.expand) {
                this.expand = false;
                return;
            } else if (this.srcPreview || this.textPreview) {
                this.expand = true;
                return;
            }
            this.expand = true;
            if (this.isPreviewableVideo || this.isPreviewableAudio) {
                this.srcPreview = `file/${this.meta.cache}/${encodeURIComponent(this.meta.name)}`;
            } else if (this.isPreviewableText) {
                this.showFullTextPreview = false;
                this.loadingPreview = true;
                this.loadedPreview = 0;
                this.$http.get(`file/${this.meta.cache}/${encodeURIComponent(this.meta.name)}`, {
                    responseType: 'text',
                    onDownloadProgress: e => {this.loadedPreview = e.loaded},
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
                    onDownloadProgress: e => {this.loadedPreview = e.loaded},
                }).then(response => {
                    this.srcPreview = URL.createObjectURL(new Blob([response.data]));
                }).catch(error => {
                    if (error.response && error.response.data.msg) {
                        this.$toast(this.$t('fileFetchFailedMsg', { msg: error.response.data.msg })); // Translate toast
                    } else {
                        this.$toast(this.$t('fileFetchFailed')); // Translate toast
                    }
                }).finally(() => {
                    this.loadingPreview = false;
                });
            }
        },
        toggleTextPreview() {
            this.showFullTextPreview = !this.showFullTextPreview;
        },
        copyLink() {
            this.copyToClipboard(this.contentUrl, 'copySuccess');
        },
        copyToClipboard(textToCopy, successMessageKey = 'copySuccess', errorMessageKey = 'copyFailedGeneral') {
            // 优先使用 navigator.clipboard (需要安全上下文)
            if (navigator.clipboard && window.isSecureContext) {
                navigator.clipboard.writeText(textToCopy)
                    .then(() => this.$toast(this.$t(successMessageKey)))
                    .catch(err => {
                        console.error('使用 navigator.clipboard 复制失败:', err);
                        this.$toast(this.$t(errorMessageKey));
                    });
            } else {
                // 后备方案：使用 document.execCommand (兼容性更好，但已不推荐)
                try {
                    const textArea = document.createElement("textarea");
                    textArea.value = textToCopy;
                    // 使 textarea 在屏幕外，避免干扰布局
                    textArea.style.position = "absolute";
                    textArea.style.left = "-9999px";
                    document.body.appendChild(textArea);
                    textArea.select();
                    const successful = document.execCommand('copy');
                    document.body.removeChild(textArea);

                    if (successful) {
                        this.$toast(this.$t(successMessageKey));
                    } else {
                        console.error('使用 document.execCommand 复制失败');
                        this.$toast(this.$t(errorMessageKey));
                    }
                } catch (err) {
                    console.error('复制时发生错误:', err);
                    this.$toast(this.$t(errorMessageKey));
                }
            }
        },
        deleteItem() {
            this.$http.delete(`revoke/${this.meta.id}`, {
                params: new URLSearchParams([['room', this.$root.room]]),
            }).then(() => {
                if (!this.expired && this.meta.cache) {
                    this.$http.delete(`file/${this.meta.cache}`).then(() => {
                        this.$toast(this.$t('deleteSuccessFile', { name: this.meta.name })); // Translate toast
                    }).catch(error => {
                        console.error("删除物理文件失败:", error);
                        if (error.response && error.response.data.msg) {
                            this.$toast(this.$t('deleteFailedFileMsg', { msg: error.response.data.msg })); // Translate toast
                        } else {
                            this.$toast(this.$t('deleteFailedFile')); // Translate toast
                        }
                    });
                } else {
                     this.$toast(this.$t('deleteSuccessFile', { name: this.meta.name })); // Translate toast
                }
            }).catch(error => {
                if (error.response && error.response.data.msg) {
                    this.$toast(this.$t('deleteFailedMessageMsg', { msg: error.response.data.msg })); // Translate toast
                } else {
                    this.$toast(this.$t('deleteFailedMessage')); // Translate toast
                }
            });
        },
        deviceIcon(type) {
            const lowerType = type?.toLowerCase() || '';
            if (lowerType.includes('mobile') || lowerType.includes('phone') || lowerType.includes('tablet') || lowerType.includes('ios') || lowerType.includes('android')) {
                return mdiCellphone;
            }
            return mdiDesktopTower; // Default to desktop
        },
    },
}
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
    background: rgba(30, 41, 59, 0.92);
    color: rgba(226, 232, 240, 0.94);
}
</style>