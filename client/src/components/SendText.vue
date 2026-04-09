<template>
    <div class="send-panel" :class="{ 'send-panel--compact': compact }">
        <div v-if="!hideTitle" class="headline text--primary mb-4">{{ $t('sendText') }}</div>
        <v-textarea
            ref="textarea"
            no-resize
            outlined
            :dense="compact"
            :rows="compact ? 4 : 6"
            :counter="$root.config.text.limit"
            :placeholder="$t('enterTextToSend')"
            v-model="$root.send.text"
            class="send-panel__textarea"
        ></v-textarea>
        <div class="text-right send-panel__actions">
            <v-btn
                depressed
                color="primary"
                :block="$vuetify.breakpoint.smAndDown"
                :disabled="isDisabled"
                @click="send"
            >{{ $t('send') }}</v-btn>
        </div>
    </div>
</template>

<script>
export default {
    name: 'send-text',
    props: {
        hideTitle: {
            type: Boolean,
            default: false,
        },
        compact: {
            type: Boolean,
            default: false,
        },
    },
    computed: {
        isDisabled() {
            return !this.$root.send.text || !this.$root.websocket || this.$root.send.text.length > this.$root.config.text.limit;
        },
    },
    methods: {
        focus() {
            if (this.$refs.textarea && typeof this.$refs.textarea.focus === 'function') {
                this.$refs.textarea.focus();
            }
        },
        send() {
            this.$http.post(
                'text',
                this.$root.send.text,
                {
                    params: new URLSearchParams([['room', this.$root.room]]),
                    headers: {
                        'Content-Type': 'text/plain',
                    },
                },
            ).then(response => {
                this.$toast(this.$t('sendSuccess'));
                this.$root.send.text = '';
                this.focus(); // 发送成功后重新聚焦输入框
            }).catch(error => {
                if (error.response && error.response.data.msg) {
                    this.$toast(this.$t('sendFailedMsg', { msg: error.response.data.msg }));
                } else {
                    this.$toast(this.$t('sendFailed'));
                }
            });
        },
    },
    mounted() {
        // 组件加载完成后自动聚焦
        this.focus();
    },
}
</script>

<style scoped>
.send-panel__textarea ::v-deep .v-input__slot {
    border-radius: 18px;
    background: rgba(248, 250, 252, 0.72);
}

.theme--dark .send-panel__textarea ::v-deep .v-input__slot {
    background: rgba(30, 41, 59, 0.92);
}

.send-panel__actions {
    margin-top: -0.25rem;
}
</style>