<template>
    <v-hover v-slot:default="{ hover }">
        <v-card :elevation="hover ? 6 : 2" class="mb-2 transition-swing">
            <v-card-text>
                <!-- Info Line - 移除ID部分 -->
                <div class="caption text--secondary mb-1" v-if="meta.timestamp && ($root.showTimestamp || $root.showDeviceInfo || $root.showSenderIP)">
                    <template v-if="$root.showTimestamp">
                        <v-icon small class="mr-1">{{ mdiClockOutline }}</v-icon>{{ formatTimestamp(meta.timestamp) }}
                    </template>
                    <template v-if="$root.showDeviceInfo && meta.senderDevice && meta.senderDevice.type">
                        <v-icon small class="ml-2 mr-1">{{ deviceIcon(meta.senderDevice.type) }}</v-icon>{{ meta.senderDevice.os || meta.senderDevice.type }}
                    </template>
                    <template v-if="$root.showSenderIP && meta.senderIP">
                        <v-icon small class="ml-2 mr-1">{{ mdiIpNetworkOutline }}</v-icon>{{ meta.senderIP }}
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
                    <!-- ... -->
                    <div class="flex-grow-1 mr-2" style="min-width: 0">
                        <!-- Title -->
                        <div
                            class="title text-truncate text--primary"
                            :style="{'text-decoration': expired ? 'line-through' : ''}"
                            :title="meta.name"
                        >{{meta.name}}</div>
                        <!-- Original Info Line (Size/Expire) -->
                        <div class="caption">
                            {{meta.size | prettyFileSize}}
                            <template v-if="$vuetify.breakpoint.smAndDown"><br></template>
                            <template v-else>|</template>
                            {{ expired ? $t('expiredAt', { time: formatTimestamp(meta.expire) }) : $t('willExpireAt', { time: formatTimestamp(meta.expire) }) }}
                        </div>
                    </div>

                    <!-- Buttons + ID -->
                    <div class="align-self-start text-no-wrap d-flex flex-column align-end">
                        <!-- ID显示在按钮上方 -->
                        <div v-if="meta.id" class="caption grey--text text--darken-1 mb-1">
                            <v-icon small class="mr-1">{{ mdiPound }}</v-icon>{{ meta.id }}
                        </div>
                        <!-- 按钮组 -->
                        <div class="align-self-center text-no-wrap">
                            <!-- Download Button -->
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn
                                        v-on="on"
                                        icon
                                        color="grey"
                                        :href="expired ? null : fileUrl"
                                        :download="expired ? null : meta.name"
                                        :disabled="expired"
                                    >
                                        <v-icon>{{ expired ? mdiDownloadOff : mdiDownload }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ expired ? $t('expired') : $t('download') }}</span>
                            </v-tooltip>

                            <!-- Preview Button -->
                            <template v-if="meta.thumbnail || isPreviewableVideo || isPreviewableAudio">
                                <v-progress-circular
                                    v-if="loadingPreview"
                                    indeterminate
                                    color="grey"
                                >{{loadedPreview / meta.size | percentage(0)}}</v-progress-circular>
                                <v-tooltip bottom>
                                    <template v-slot:activator="{ on }">
                                        <v-btn v-on="on" icon color="grey" @click="!expired && previewFile()">
                                            <v-icon>{{(isPreviewableVideo || isPreviewableAudio) ? mdiMovieSearchOutline : mdiImageSearchOutline}}</v-icon>
                                        </v-btn>
                                    </template>
                                    <span>{{ $t('preview') }}</span>
                                </v-tooltip>
                            </template>

                            <!-- Copy Link Button -->
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" @click="copyLink">
                                        <v-icon>{{ mdiLinkVariant }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('copyLink') }}</span>
                            </v-tooltip>

                            <!-- Show QR Code Button -->
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" @click="qrDialogVisible = true">
                                        <v-icon>{{ mdiQrcode }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('showQrCode') }}</span>
                            </v-tooltip>

                            <!-- Delete Button -->
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" @click="deleteItem" :disabled="loadingPreview">
                                        <v-icon>{{mdiClose}}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('delete') }}</span>
                            </v-tooltip>
                        </div>
                    </div>
                </div>
                <v-expand-transition v-if="meta.thumbnail || isPreviewableVideo || isPreviewableAudio">
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
            loadingPreview: false,
            loadedPreview: 0,
            expand: false,
            srcPreview: null,
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
            } else if (this.srcPreview) {
                this.expand = true;
                return;
            }
            this.expand = true;
            if (this.isPreviewableVideo || this.isPreviewableAudio) {
                this.srcPreview = `file/${this.meta.cache}/${encodeURIComponent(this.meta.name)}`;
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