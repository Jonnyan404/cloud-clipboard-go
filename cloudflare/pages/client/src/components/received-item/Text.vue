<template>
    <v-hover v-slot:default="{ hover }">
        <v-card :elevation="hover ? 6 : 2" class="mb-2 transition-swing">
            <v-card-text>
                <div class="d-flex flex-row align-center">
                    <div class="flex-grow-1 mr-2" style="min-width: 0">
                        <!-- Info Line - 移除ID部分 -->
                        <div class="caption text--secondary mb-1" v-if="meta.timestamp && ($root.showTimestamp || $root.showDeviceInfo || $root.showSenderIP)">
                            <template v-if="$root.showTimestamp">
                                <v-icon small class="mr-1">{{ mdiClockOutline }}</v-icon>{{ formatTimestamp(meta.timestamp) }}
                            </template>
                            <template v-if="$root.showDeviceInfo && meta.senderDevice?.type">
                                <v-icon small class="ml-2 mr-1">{{ deviceIcon(meta.senderDevice.type) }}</v-icon>{{ meta.senderDevice.os || meta.senderDevice.type }}
                            </template>
                            <template v-if="$root.showSenderIP && meta.senderIP">
                                <v-icon small class="ml-2 mr-1">{{ mdiIpNetworkOutline }}</v-icon>{{ meta.senderIP }}
                            </template>
                        </div>
                        <!-- Title -->
                        <div class="title text-truncate text--primary" @click="expand = !expand">
                            {{ $t('textMessage') }}<v-icon>{{expand ? mdiChevronUp : mdiChevronDown}}</v-icon>
                        </div>
                        <!-- Preview -->
                        <div class="text-truncate" @click="expand = !expand">{{ decodedContentPreview }}</div>
                    </div>
                    <!-- Buttons + ID -->
                    <div class="align-self-start text-no-wrap d-flex flex-column align-end">
                        <!-- ID显示在按钮上方 -->
                        <div v-if="meta.id" class="caption grey--text text--darken-1 mb-1">
                            <v-icon small class="mr-1">{{ mdiPound }}</v-icon>{{ meta.id }}
                        </div>
                        <!-- 按钮组 -->
                        <div>
                            <!-- Copy Text Button -->
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn v-on="on" icon color="grey" @click="copyText">
                                        <v-icon>{{mdiContentCopy}}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('copyText') }}</span>
                            </v-tooltip>

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
                                    <v-btn v-on="on" icon color="grey" @click="deleteItem">
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
                        <!-- Use v-text or properly sanitize if using v-html -->
                        <div ref="content" style="white-space: pre-wrap; word-break: break-all;">{{ decodedContent }}</div>
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
    mdiPound, // 添加ID图标
} from '@mdi/js';
import { formatTimestamp } from '@/util.js';

// Helper function to decode HTML entities
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
            mdiPound, // 添加ID图标
        };
    },
    computed: {
        // Decode content for display, preventing potential XSS
        decodedContent() {
            return decodeHtmlEntities(this.meta.content || '');
        },
        // Decode content for preview
        decodedContentPreview() {
            // Limit preview length if needed
            const decoded = decodeHtmlEntities(this.meta.content || '');
            return decoded; // You might want to truncate this further
        },
        contentUrl() {
            const protocol = window.location.protocol;
            const host = window.location.host;
            const prefix = this.$root.config?.server?.prefix || '';
            const roomQuery = this.$root.room ? `?room=${this.$root.room}` : '';
            const id = this.meta?.id ?? '';
            return `${protocol}//${host}${prefix}/content/${id}${roomQuery}`;
        }
    },
    methods: {
        formatTimestamp, // Make formatter available
        deviceIcon(type) {
            const lowerType = type.toLowerCase();
            if (lowerType.includes('mobile') || lowerType.includes('phone') || lowerType.includes('tablet') || lowerType.includes('ios') || lowerType.includes('android')) {
                return mdiCellphone;
            }
            return mdiDesktopTower; // Default to desktop
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
                    const textArea = document.createElement("textarea");
                    textArea.value = textToCopy;
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
        copyText() {
            this.copyToClipboard(this.decodedContent, 'copySuccess');
        },
        copyLink() {
            this.copyToClipboard(this.contentUrl, 'copySuccess');
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
}
</script>