<template>
    <div class="send-panel" :class="{ 'send-panel--compact': compact }">
        <div v-if="!hideTitle" class="text-h5 text-primary mb-4">{{ t('sendText') }}</div>
        <v-textarea
            ref="textarea"
            no-resize
            variant="outlined"
            :density="compact ? 'compact' : 'default'"
            :rows="compact ? 4 : 6"
            :counter="app.config.text.limit"
            :placeholder="t('enterTextToSend')"
            v-model="app.send.text"
            class="send-panel__textarea"
        ></v-textarea>
        <div class="text-right send-panel__actions">
            <v-btn
                variant="flat"
                color="primary"
                :block="display.smAndDown"
                :disabled="isDisabled"
                @click="send"
            >{{ t('send') }}</v-btn>
        </div>
    </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useDisplay } from 'vuetify';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { toast } from '@/plugins/toast';

defineProps({
    hideTitle: {
        type: Boolean,
        default: false,
    },
    compact: {
        type: Boolean,
        default: false,
    },
});

const app = useAppStore();
const ws = useWebSocketStore();
const display = useDisplay();
const { t } = useI18n();

const textarea = ref(null);

const isDisabled = computed(() => !app.send.text || !ws.websocket || app.send.text.length > app.config.text.limit);

function focus() {
    if (textarea.value && typeof textarea.value.focus === 'function') {
        textarea.value.focus();
    }
}

function send() {
    axios.post(
        'text',
        app.send.text,
        {
            params: new URLSearchParams([['room', ws.room]]),
            headers: {
                'Content-Type': 'text/plain',
            },
        },
    ).then(response => {
        toast(t('sendSuccess'));
        app.send.text = '';
        focus();
    }).catch(error => {
        if (error.response && error.response.data.msg) {
            toast(t('sendFailedMsg', { msg: error.response.data.msg }));
        } else {
            toast(t('sendFailed'));
        }
    });
}

onMounted(() => {
    focus();
});
</script>

<style scoped>
.send-panel__textarea :deep(.v-field) {
    border-radius: 18px;
    background: rgba(248, 250, 252, 0.72);
}

.v-theme--dark .send-panel__textarea :deep(.v-field) {
    background: rgba(30, 41, 59, 0.92);
}

.send-panel__actions {
    margin-top: -0.25rem;
}
</style>