<template>
    <v-hover v-slot:default="{ hover }">
        <v-card :elevation="hover ? 6 : 2" class="mb-2 transition-swing">
            <v-card-text>
                <!-- New Info Line - Moved Here (Outside and Above the flex row) -->
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
                        v-if="meta.thumbnail"
                        :src="meta.thumbnail"
                        class="mr-3 flex-grow-0 hidden-sm-and-down"
                        width="2.5rem"
                        height="2.5rem"
                        style="border-radius: 3px"
                    ></v-img>
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

                    <div class="align-self-center text-no-wrap">
                        <v-tooltip bottom>
                            <template v-slot:activator="{ on }">
                                <v-btn
                                    v-on="on"
                                    icon
                                    color="grey"
                                    :href="expired ? null : `file/${meta.cache}/${encodeURIComponent(meta.name)}`"
                                    :download="expired ? null : meta.name"
                                >
                                    <v-icon>{{expired ? mdiDownloadOff : mdiDownload}}</v-icon>
                                </v-btn>
                            </template>
                            <span>{{ expired ? $t('expired') : $t('download') }}</span>
                        </v-tooltip>
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
                        <v-tooltip bottom>
                            <template v-slot:activator="{ on }">
                                <v-btn v-on="on" icon color="grey" @click="copyLink">
                                    <v-icon>{{mdiLinkVariant}}</v-icon>
                                </v-btn>
                            </template>
                            <span>{{ $t('copyLink') }}</span>
                        </v-tooltip>
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
        </v-card>
    </v-hover>
</template>

<script>
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
} from '@mdi/js';

export default {
    name: 'received-file',
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
            return this.meta.name.match(/\.(wav|ogg|opus|m4a|flac)$/gi);
        },
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
                this.srcPreview = `file/${this.meta.cache}/${encodeURIComponent(meta.name)}`;
            } else {
                this.loadingPreview = true;
                this.loadedPreview = 0;
                this.$http.get(`file/${this.meta.cache}/${encodeURIComponent(meta.name)}`, {
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
            const textToCopy = `${location.protocol}//${location.host}/content/${this.meta.id}${this.$root.room ? `?room=${this.$root.room}` : ''}`;

            // 优先使用 navigator.clipboard (需要安全上下文)
            if (navigator.clipboard && window.isSecureContext) {
                navigator.clipboard.writeText(textToCopy)
                    .then(() => this.$toast(this.$t('copySuccess'))) // Translate toast
                    .catch(err => {
                        console.error('使用 navigator.clipboard 复制失败:', err);
                        this.$toast(this.$t('copyFailedGeneral')); // Translate toast
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
                        this.$toast(this.$t('copySuccess')); // Translate toast
                    } else {
                        console.error('使用 document.execCommand 复制失败');
                        this.$toast(this.$t('copyFailedGeneral')); // Translate toast
                    }
                } catch (err) {
                    console.error('复制时发生错误:', err);
                    this.$toast(this.$t('copyFailedGeneral')); // Translate toast
                }
            }
        },
        deleteItem() {
            this.$http.delete(`revoke/${this.meta.id}`, {
                params: new URLSearchParams([['room', this.$root.room]]),
            }).then(() => {
                if (this.expired) return;
                this.$http.delete(`file/${this.meta.cache}`).then(() => {
                    this.$toast(this.$t('deleteSuccessFile', { name: this.meta.name })); // Translate toast
                }).catch(error => {
                    if (error.response && error.response.data.msg) {
                        this.$toast(this.$t('deleteFailedFileMsg', { msg: error.response.data.msg })); // Translate toast
                    } else {
                        this.$toast(this.$t('deleteFailedFile')); // Translate toast
                    }
                });
            }).catch(error => {
                if (error.response && error.response.data.msg) {
                    this.$toast(this.$t('deleteFailedMessageMsg', { msg: error.response.data.msg })); // Translate toast
                } else {
                    this.$toast(this.$t('deleteFailedMessage')); // Translate toast
                }
            });
        },
        deviceIcon(type) {
            const lowerType = type.toLowerCase();
            if (lowerType.includes('mobile') || lowerType.includes('phone') || lowerType.includes('tablet') || lowerType.includes('ios') || lowerType.includes('android')) {
                return mdiCellphone;
            }
            return mdiDesktopTower; // Default to desktop
        },
    },
}
</script>