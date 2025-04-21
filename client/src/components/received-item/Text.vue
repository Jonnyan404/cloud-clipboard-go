<template>
    <v-hover
        v-slot:default="{ hover }"
    >
        <v-card :elevation="hover ? 6 : 2" class="mb-2 transition-swing">
            <v-card-text>
                <div class="d-flex flex-row align-center">
                    <div class="flex-grow-1 mr-2" style="min-width: 0">
                        <div class="title text-truncate text--primary" @click="expand = !expand">
                            {{ $t('textMessage') }}<v-icon>{{expand ? mdiChevronUp : mdiChevronDown}}</v-icon>
                        </div>
                        <!-- Use textContent for preview to avoid potential XSS if content is not sanitized -->
                        <div class="text-truncate" @click="expand = !expand">{{ decodedContentPreview }}</div>
                    </div>

                    <div class="align-self-center text-no-wrap">
                        <v-tooltip bottom>
                            <template v-slot:activator="{ on }">
                                <v-btn v-on="on" icon color="grey" @click="copyText">
                                    <v-icon>{{mdiContentCopy}}</v-icon>
                                </v-btn>
                            </template>
                            <span>{{ $t('copyText') }}</span>
                        </v-tooltip>
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
                                <v-btn v-on="on" icon color="grey" @click="deleteItem">
                                    <v-icon>{{mdiClose}}</v-icon>
                                </v-btn>
                            </template>
                            <span>{{ $t('delete') }}</span>
                        </v-tooltip>
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
        </v-card>
    </v-hover>
</template>

<script>
import {
    mdiChevronUp,
    mdiChevronDown,
    mdiContentCopy,
    mdiClose,
    mdiLinkVariant,
} from '@mdi/js';

// Helper function to decode HTML entities
function decodeHtmlEntities(text) {
    const textArea = document.createElement('textarea');
    textArea.innerHTML = text;
    return textArea.value;
}

export default {
    name: 'received-text',
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
            mdiChevronUp,
            mdiChevronDown,
            mdiContentCopy,
            mdiClose,
            mdiLinkVariant,
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
        }
    },
    methods: {
        copyToClipboard(textToCopy, successMessageKey = 'copySuccess', errorMessageKey = 'copyFailedGeneral') { // Use keys
            if (navigator.clipboard && window.isSecureContext) {
                navigator.clipboard.writeText(textToCopy)
                    .then(() => this.$toast(this.$t(successMessageKey))) // Translate toast
                    .catch(err => {
                        console.error('使用 navigator.clipboard 复制失败:', err);
                        this.$toast(this.$t(errorMessageKey)); // Translate toast
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
                        this.$toast(this.$t(successMessageKey)); // Translate toast
                    } else {
                        console.error('使用 document.execCommand 复制失败');
                        this.$toast(this.$t(errorMessageKey)); // Translate toast
                    }
                } catch (err) {
                    console.error('复制时发生错误:', err);
                    this.$toast(this.$t(errorMessageKey)); // Translate toast
                }
            }
        },
        copyText() {
            const textToCopy = decodeHtmlEntities(this.meta.content || '');
            this.copyToClipboard(textToCopy, 'copySuccess'); // Pass key
        },
        copyLink() {
            const linkToCopy = `${location.protocol}//${location.host}/content/${this.meta.id}${this.$root.room ? `?room=${this.$root.room}` : ''}`;
            this.copyToClipboard(linkToCopy, 'copySuccess'); // Pass key
        },
        deleteItem() {
            this.$http.delete(`revoke/${this.meta.id}`, {
                params: new URLSearchParams([['room', this.$root.room]]),
            }).then(() => {
                this.$toast(this.$t('deleteSuccessText')); // Translate toast
            }).catch(error => {
                if (error.response && error.response.data.msg) {
                    this.$toast(this.$t('deleteFailedMessageMsg', { msg: error.response.data.msg })); // Translate toast
                } else {
                    this.$toast(this.$t('deleteFailedMessage')); // Translate toast
                }
            });
        },
    },
}
</script>