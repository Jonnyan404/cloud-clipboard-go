<template>
    <v-card
        class="unified-composer"
        :class="{ 'unified-composer--dark': isDark, 'unified-composer--dragover': dragover }"
        variant="outlined"
        @dragenter.prevent="dragover = true"
        @dragover.prevent="dragover = true"
        @dragleave.prevent="handleDragLeave"
        @drop.prevent="handleDrop"
    >
        <div class="unified-composer__body pa-3 pa-md-4">
            <v-textarea
                ref="textarea"
                v-model="app.send.text"
                auto-grow
                no-resize
                variant="solo"
                flat
                density="compact"
                rows="3"
                :placeholder="t('enterTextToSend')"
                hide-details
                class="unified-composer__textarea"
                :style="composerTextareaStyle"
                @keydown.ctrl.enter.prevent="onSendShortcut"
                @keydown.meta.enter.prevent="onSendShortcut"
            ></v-textarea>

            <div class="unified-composer__meta px-1 pb-2">
                <span class="text-caption text-medium-emphasis mr-3">{{ textLimitLabel }}</span>
                <span class="text-caption text-medium-emphasis">{{ fileLimitLabel }}</span>
            </div>

            <div v-if="app.send.files.length" class="unified-composer__attachments px-1 pb-2">
                <v-chip
                    v-for="(file, index) in app.send.files"
                    :key="file.name + file.size + index"
                    close
                    :variant="'outlined'"
                    size="small"
                    class="mr-2 mb-2"
                    @click:close="removeFile(index)"
                >
                    {{ file.name }} · {{ prettyFileSize(file.size) }}
                </v-chip>
            </div>

            <div v-if="progress" class="px-1 pb-2">
                <small class="d-block text-right text-medium-emphasis mb-1">
                    {{ prettyFileSize(Math.min(uploadedSize, fileSize)) }} / {{ prettyFileSize(fileSize) }}
                </small>
                <v-progress-linear :value="uploadProgress * 100"></v-progress-linear>
            </div>

            <div class="unified-composer__footer pt-1">
                <div class="unified-composer__footer-main d-flex align-center flex-wrap">
                    <v-btn icon density="comfortable" variant="text" size="small" color="grey-darken-1" @click="openFilePicker">
                        <v-icon>{{ mdiPaperclip }}</v-icon>
                    </v-btn>
                    <v-btn icon density="comfortable" variant="text" size="small" color="grey-darken-1" @click="emit('show-qr')">
                        <v-icon>{{ mdiQrcode }}</v-icon>
                    </v-btn>
                    <div class="text-caption text-medium-emphasis ml-2 unified-composer__hint">
                        {{ footerHint }}
                        <span class="unified-composer__shortcut">{{ sendShortcutLabel }}</span>
                    </div>
                </div>

                <v-btn
                    variant="flat"
                    color="primary"
                    class="unified-composer__send"
                    :disabled="sendDisabled"
                    @click="sendAll"
                >
                    <v-icon start size="small">{{ mdiSend }}</v-icon>
                    {{ t('send') }}
                </v-btn>
            </div>
        </div>

        <input
            ref="selectFile"
            type="file"
            class="d-none"
            multiple
            @change="handleSelectFiles(Array.from($event.target.files))"
        >
    </v-card>
</template>

<script setup>import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useDisplay } from 'vuetify';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { toast } from '@/plugins/toast';
import { prettyFileSize } from '@/util.js';

const mdiPaperclip = 'mdi-paperclip';
const mdiQrcode = 'mdi-qrcode';
const mdiSend = 'mdi-send';


const emit = defineEmits(['show-qr']);
const app = useAppStore();
const ws = useWebSocketStore();
const display = useDisplay();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();
defineExpose({ focus, openFilePicker });
const progress = ref(false);
const dragover = ref(false);
const uploadedSizes = ref([]);
const isMac = /mac|iphone|ipad|ipod/i.test(navigator.userAgent || '');
const textarea = ref(null);
const selectFile = ref(null);
const fileSize = computed(() => app.send.files.length ? app.send.files.reduce((acc, cur) => acc += cur.size, 0) : 0);
const uploadedSize = computed(() => uploadedSizes.value.length ? uploadedSizes.value.reduce((acc, cur) => acc += cur, 0) : 0);
const uploadProgress = computed(() => Math.min(fileSize.value !== 0 ? (uploadedSize.value / fileSize.value) : 0, 1));
const sendDisabled = computed(() => !ws.websocket || progress.value || (!app.send.text && !app.send.files.length) || app.send.text.length > app.config.text.limit);
const footerHint = computed(() => {
    if (app.send.files.length) {
        return t('composerFilesSelected', { count: app.send.files.length });
    }
    const pasteKey = isMac ? '⌘+V' : 'Ctrl+V';
    return t('dragDropPasteTip', { keys: pasteKey });
});
const sendShortcutLabel = computed(() => t('sendShortcutTip', {
    keys: isMac ? '⌘+Enter' : 'Ctrl+Enter',
}));
const textLimitLabel = computed(() => t('composerTextLimit', {
    current: app.send.text.length,
    limit: app.config.text.limit,
}));
const fileLimitLabel = computed(() => t('fileSizeLimit', {
    limit: prettyFileSize(app.config.file.limit),
}));
const composerTextareaStyle = computed(() => ({
    maxHeight: '12rem',
}));
function focus(type) {
    if (type === 'file') {
        openFilePicker();
        return;
    }
    if (textarea.value && typeof textarea.value.focus === 'function') {
        textarea.value.focus();
    }
}
function openFilePicker() {
    selectFile.value.click();
}
function onSendShortcut() {
    if (!sendDisabled.value) {
        sendAll();
    }
}
function removeFile(index) {
    app.send.files.splice(index, 1);
}
function handleSelectFiles(files) {
    if (!files.length) {
        return;
    }
    if (files.some(file => !file.size)) {
        toast(t('cannotSendEmptyFile'));
        return;
    }
    if (files.some(file => file.size > app.config.file.limit)) {
        toast(t('fileSizeExceeded', { limit: prettyFileSize(app.config.file.limit) }));
        return;
    }
    app.send.files.splice(0);
    app.send.files.push(...files);
}
async function sendText() {
    if (!app.send.text) {
        return;
    }
    await axios.post(
        'text',
        app.send.text,
        {
            params: new URLSearchParams([['room', ws.room]]),
            headers: {
                'Content-Type': 'text/plain',
            },
        },
    );
    app.send.text = '';
}
async function sendFiles() {
    if (!app.send.files.length) {
        return;
    }
    const chunkSize = app.config.file.chunk;
    uploadedSizes.value.splice(0);
    uploadedSizes.value.push(...Array(app.send.files.length).fill(0));
    progress.value = true;
    await Promise.all(app.send.files.map(async (file, index) => {
        if (file.size < chunkSize) {
            const formData = new FormData;
            formData.set('file', file);
            await axios.postForm('upload', formData, {
                params: new URLSearchParams([['room', ws.room]]),
                onUploadProgress: event => uploadedSizes.value[index] = event.loaded,
            });
            return;
        }
        const response = await axios.post('upload/chunk', file.name, {
            headers: { 'Content-Type': 'text/plain' },
            params: new URLSearchParams([['room', ws.room]]),
        });
        const uuid = response.data.result.uuid;
        let uploadedSize = 0;
        while (uploadedSize < file.size) {
            const chunk = file.slice(uploadedSize, uploadedSize + chunkSize);
            await axios.post(`upload/chunk/${uuid}`, chunk, {
                headers: { 'Content-Type': 'application/octet-stream' },
                onUploadProgress: event => uploadedSizes.value[index] = uploadedSize + event.loaded,
            });
            uploadedSize += chunkSize;
        }
        await axios.post(`upload/finish/${uuid}`, null, {
            params: new URLSearchParams([['room', ws.room]]),
        });
    }));
    app.send.files.splice(0);
}
async function sendAll() {
    try {
        if (app.send.text) {
            await sendText();
        }
        if (app.send.files.length) {
            await sendFiles();
        }
        toast(t('sendSuccess'));
        focus();
    } catch (error) {
        if (error.response && error.response.data.msg) {
            toast(t('sendFailedMsg', { msg: error.response.data.msg }));
        } else {
            toast(t('sendFailed'));
        }
    } finally {
        progress.value = false;
    }
}
function handleDragLeave(event) {
    if (event.currentTarget.contains(event.relatedTarget)) {
        return;
    }
    dragover.value = false;
}
function handleDrop(event) {
    dragover.value = false;
    if (!(event && event.dataTransfer)) {
        return;
    }
    const files = Array.from(event.dataTransfer.files || []);
    if (files.length) {
        handleSelectFiles(files);
    }
}
function handlePaste(event) {
    if (!(event && event.clipboardData)) {
        return;
    }
    const items = Array.from(event.clipboardData.items || []);
    const files = items.filter(item => item.kind === 'file').map(item => item.getAsFile()).filter(Boolean);
    if (files.length) {
        handleSelectFiles(files);
    }
}
onMounted(() => {
    document.addEventListener('paste', handlePaste);
    nextTick(() => {
        focus();
    });
});
onBeforeUnmount(() => {
    document.removeEventListener('paste', handlePaste);
});
</script>

<style scoped>
.unified-composer {
    border-radius: 22px;
    border-color: rgba(148, 163, 184, 0.22) !important;
    box-shadow: 0 10px 28px rgba(15, 23, 42, 0.06);
    background: rgba(255, 255, 255, 0.96);
    transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.unified-composer--dark {
    border-color: rgba(71, 85, 105, 0.72) !important;
    box-shadow: 0 18px 36px rgba(2, 6, 23, 0.3);
    background: rgba(15, 23, 42, 0.94);
}

.unified-composer--dragover {
    border-color: var(--v-primary-base, #1976d2) !important;
    box-shadow: 0 0 0 2px var(--v-primary-base, #1976d2);
}

.unified-composer--dragover * {
    pointer-events: none;
}

.unified-composer__textarea :deep(.v-input__slot) {
    box-shadow: none !important;
    border-radius: 16px;
    background: rgba(248, 250, 252, 0.95) !important;
    padding: 0.25rem 0.25rem 0 0.25rem;
}

.unified-composer--dark .unified-composer__textarea :deep(.v-input__slot) {
    background: rgba(30, 41, 59, 0.96) !important;
}

.unified-composer--dark .unified-composer__textarea :deep(textarea),
.unified-composer--dark .unified-composer__meta,
.unified-composer--dark .unified-composer__hint,
.unified-composer--dark .unified-composer__attachments {
    color: rgba(226, 232, 240, 0.92) !important;
}

.unified-composer__textarea :deep(textarea) {
    max-height: 10.5rem !important;
    overflow-y: auto !important;
}

.unified-composer__meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0;
}

.unified-composer__attachments {
    min-height: 1.5rem;
}

.unified-composer__footer {
    display: grid;
    align-items: end;
    gap: 0.75rem;
    grid-template-columns: minmax(0, 1fr) auto;
    border-top: 1px solid rgba(226, 232, 240, 0.9);
    padding-top: 0.5rem;
}

.unified-composer--dark .unified-composer__footer {
    border-top-color: rgba(71, 85, 105, 0.72);
}

.unified-composer__footer-main {
    min-width: 0;
}

.unified-composer__hint {
    line-height: 1.4;
    min-width: 0;
    word-break: break-word;
}

.unified-composer__shortcut {
    white-space: nowrap;
}

.unified-composer__send {
    justify-self: end;
    flex-shrink: 0;
    white-space: nowrap;
}

@media (max-width: 960px) {
    .unified-composer__footer {
        align-items: flex-start;
        grid-template-columns: 1fr;
    }

    .unified-composer__send {
        justify-self: stretch;
    }
}
</style>