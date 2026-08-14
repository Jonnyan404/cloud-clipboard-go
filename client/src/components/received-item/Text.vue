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
                                    <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="copyLink">
                                        <v-icon>{{ mdiLinkVariant }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('copyLink') }}</span>
                            </v-tooltip>
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" class="timeline-card__icon-button" @click="openQrDialog">
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

            <!-- QR Code Dialog -->
            <v-dialog v-model="qrDialogVisible" max-width="250">
                <v-card>
                    <v-card-title class="headline justify-center">{{ $t('scanToAccess') }}</v-card-title>
                    <v-card-text class="text-center pa-4">
                        <v-progress-circular v-if="shareUrlLoading" indeterminate color="primary" class="my-8"></v-progress-circular>
                        <template v-else>
                            <qrcode-vue :value="shareContentUrl || contentUrl" :size="200" level="H" />
                            <div class="text-caption mt-2" style="word-break: break-all;">{{ shareContentUrl || contentUrl }}</div>
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
import { formatTimestamp, buildCleanAbsoluteRouteUrl, createShareLink } from '@/util.js';

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
            shareUrlLoading: false,
            shareContentUrl: '',
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
    },
    methods: {
        formatTimestamp,
        async ensureContentShareUrl() {
            if (this.shareContentUrl) {
                return this.shareContentUrl;
            }
            const data = await createShareLink(this, {
                type: 'content',
                id: this.meta?.id,
            });
            this.shareContentUrl = data?.url || '';
            return this.shareContentUrl;
        },
        async openQrDialog() {
            this.qrDialogVisible = true;
            this.shareUrlLoading = true;
            try {
                await this.ensureContentShareUrl();
            } catch (error) {
                console.error('生成分享链接失败:', error);
                this.$toast(this.$t('copyFailedGeneral'));
                this.shareContentUrl = this.contentUrl;
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
        copyToClipboard(textToCopy, successMessageKey = 'copySuccess', errorMessageKey = 'copyFailedGeneral') {
            if (navigator.clipboard && window.isSecureContext) {
                navigator.clipboard.writeText(textToCopy)
                    .then(() => this.$toast(this.$t(successMessageKey)))
                    .catch(err => {
                        console.error('使用 navigator.clipboard 复制失败:', err);
                        this.$toast(this.$t(errorMessageKey));
                    });
            } else {
                try {
                    const textArea = document.createElement('textarea');
                    textArea.value = textToCopy;
                    textArea.style.position = 'absolute';
                    textArea.style.left = '-9999px';
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
        copyText() {
            this.copyToClipboard(this.decodedContent, 'copySuccess');
        },
        async copyLink() {
            try {
                const url = await this.ensureContentShareUrl();
                this.copyToClipboard(url, 'copySuccess');
            } catch (error) {
                console.error('复制分享链接失败:', error);
                this.$toast(this.$t('copyFailedGeneral'));
            }
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
</style>
