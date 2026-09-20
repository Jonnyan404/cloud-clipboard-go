<script setup>import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import { errorMessage, getClientId, prettyFileSize } from '@/util.js';

const props = defineProps({
    variant: { type: String, default: 'sticky' },
});
const emit = defineEmits(['sent']);

const app = useAppStore();
const ws = useWebSocketStore();
const clientId = getClientId();
const { t } = useI18n();
const textarea = ref(null);
const selectFile = ref(null);
const sending = ref(false);
const uploadedSizes = ref([]);
const placeholder = computed(() => {
    if (props.variant === 'mega') {
        return t('composerPlaceholder');
    }
    if (props.variant === 'terminal') {
        return t('terminalPlaceholder');
    }
    if (props.variant === 'workbench') {
        return t('workbenchInputHint');
    }
    if (props.variant === 'chat') {
        return t('chatPlaceholder');
    }
    return t('stickyNewNote');
});
const fileSize = computed(() => app.send.files.length ? app.send.files.reduce((acc, cur) => acc += cur.size, 0) : 0);
const uploadedSize = computed(() => uploadedSizes.value.length ? uploadedSizes.value.reduce((acc, cur) => acc += cur, 0) : 0);
const uploadProgress = computed(() => Math.min(fileSize.value !== 0 ? (uploadedSize.value / fileSize.value) : 0, 1));
const sendDisabled = computed(() => !ws.websocket || sending.value || (!app.send.text && !app.send.files.length) || app.send.text.length > app.config.text.limit);

function focus() {
    nextTick(() => {
        if (textarea.value && typeof textarea.value.focus === 'function') {
            textarea.value.focus();
        }
    });
}
defineExpose({ focus, addFiles });
function addFiles(fileList) {
    handleSelectFiles(fileList);
}

function openFilePicker() {
    // 上传开关关掉时这个 input 会被 v-if 摘掉，ref 就是 null —— 不能裸点
    selectFile.value?.click();
}

function handlePaste(event) {
    if (!(event && event.clipboardData)) {
        return;
    }
    const files = [];
    for (const item of Array.from(event.clipboardData.items || [])) {
        if (item.kind === 'file') {
            const file = item.getAsFile();
            if (file) {
                files.push(file);
            }
        }
    }
    if (!files.length) {
        for (const file of Array.from(event.clipboardData.files || [])) {
            if (file) {
                files.push(file);
            }
        }
    }
    if (files.length) {
        event.preventDefault();
        handleSelectFiles(files);
    }
}

onMounted(() => {
    document.addEventListener('paste', handlePaste);
});
onBeforeUnmount(() => {
    document.removeEventListener('paste', handlePaste);
});

function handleSelectFiles(fileList) {
    const files = Array.from(fileList || []);
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

function removeFile(index) {
    app.send.files.splice(index, 1);
}

function onKeydown(event) {
    const isMac = /mac|iphone|ipad|ipod/i.test(navigator.userAgent || '');
    if ((event.key === 'Enter' && (event.metaKey || event.ctrlKey)) || (event.key === 'Enter' && event.shiftKey === false && !isMac)) {
        sendAll();
    }
}

async function sendText() {
    if (!app.send.text) {
        return;
    }
    await axios.post(
        'text',
        app.send.text,
        {
            params: new URLSearchParams([['room', ws.room], ['client', clientId]]),
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
    sending.value = true;
    await Promise.all(app.send.files.map(async (file, index) => {
        if (file.size < chunkSize) {
            const formData = new FormData;
            formData.set('file', file);
            await axios.postForm('upload', formData, {
                params: new URLSearchParams([['room', ws.room], ['client', clientId]]),
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
            params: new URLSearchParams([['room', ws.room], ['client', clientId]]),
        });
    }));
    app.send.files.splice(0);
}

// 同 UnifiedComposer：两边都关掉就没有可发的东西，藏起发送按钮。
const canSend = computed(() => Boolean(app.display.composerText || app.display.composerUpload));

async function sendAll() {
    if (sendDisabled.value) {
        return;
    }
    try {
        await sendText();
        await sendFiles();
        toast(t('sendSuccess'));
        focus();
    } catch (error) {
        const errMsg = errorMessage(error);
        if (errMsg) {
            toast(t('sendFailedMsg', { msg: errMsg }));
        } else {
            toast(t('sendFailed'));
        }
    } finally {
        sending.value = false;
        emit('sent');
    }
}
</script>

<template>
    <div class="sticky-composer" :class="`sticky-composer--${props.variant}`">
        <div v-if="app.display.composerUpload && app.send.files.length" class="sticky-composer__files">
            <span
                v-for="(file, index) in app.send.files"
                :key="file.name + index"
                class="sticky-composer__file"
            >{{ file.name }} <b class="sticky-composer__closer" @click="removeFile(index)">✕</b></span>
            <span v-if="sending" class="sticky-composer__progress">{{ Math.round(uploadProgress * 100) }}%</span>
        </div>
        <div class="sticky-composer__row">
            <button v-if="app.display.composerUpload" type="button" class="sticky-composer__attach" title="📎" @click="openFilePicker">➕</button>
            <textarea
                v-if="app.display.composerText"
                ref="textarea"
                v-model="app.send.text"
                class="sticky-composer__area"
                rows="1"
                :placeholder="placeholder"
                @keydown="onKeydown"
            ></textarea>
<template v-if="canSend">
            <button
                v-if="props.variant === 'mega'"
                type="button"
                class="sticky-composer__go sticky-composer__go--mega"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('send') }}</button>
            <button
                v-else-if="props.variant === 'terminal'"
                type="button"
                class="sticky-composer__go sticky-composer__go--terminal"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('terminalEnter') }}</button>
            <button
                v-else-if="props.variant === 'workbench'"
                type="button"
                class="sticky-composer__go sticky-composer__go--workbench"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('send') }}</button>
            <button
                v-else-if="props.variant === 'chat'"
                type="button"
                class="sticky-composer__go sticky-composer__go--chat"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('send') }}</button>
            <button
                v-else
                type="button"
                class="sticky-composer__go"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('stickyStick') }}</button>
            </template>
            <input
                v-if="app.display.composerUpload"
                ref="selectFile"
                type="file"
                multiple
                class="d-none"
                @change="handleSelectFiles(Array.from($event.target.files)); $event.target.value = ''"
            >
        </div>
    </div>
</template>

<style scoped>
.sticky-composer {
    background: #fffbe8;
    border: 1.5px dashed #d5c49a;
    border-radius: 12px;
    padding: 10px 13px;
    box-shadow: 0 2px 6px rgba(68, 64, 42, 0.08);
}

.sticky-composer__files {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 8px;
}

.sticky-composer__file {
    font-size: 11px;
    background: rgba(213, 196, 154, 0.28);
    color: #6f6548;
    border-radius: 6px;
    padding: 3px 8px;
}

.sticky-composer__closer {
    font-weight: 700;
    cursor: pointer;
    margin-left: 4px;
    opacity: 0.6;
}

.sticky-composer__progress {
    font-size: 11px;
    font-weight: 700;
    color: #0a0d24;
}

.sticky-composer__row {
    display: flex;
    align-items: center;
    gap: 9px;
}

.sticky-composer__attach {
    font-size: 16px;
    background: none;
    border: none;
    cursor: pointer;
    padding: 0;
    line-height: 1;
    color: #b8ae9a;
}

.sticky-composer__area {
    flex: 1;
    min-width: 0;
    resize: none;
    border: none;
    outline: none;
    background: transparent;
    font-size: 12px;
    line-height: 1.5;
    color: #444034;
    font-family: inherit;
    padding: 6px 0;
}

.sticky-composer__area::placeholder {
    color: #b8ae9a;
}

.sticky-composer__go {
    background: #d97706;
    color: #fff;
    border: none;
    border-radius: 8px;
    font-size: 12px;
    padding: 6px 14px;
    font-weight: 650;
    cursor: pointer;
    flex-shrink: 0;
}

.sticky-composer__go:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}

.sticky-composer__go--mega {
    background: #111827;
    color: #fff;
    border-radius: 10px;
    font-size: 13px;
    padding: 7px 18px;
    font-weight: 700;
}

.sticky-composer--mega {
    background: #fff;
    border: 1px solid #e8ebf0;
    border-radius: 14px;
    padding: 12px 15px;
    box-shadow: none;
}

.sticky-composer--mega .sticky-composer__row {
    gap: 10px;
}

.sticky-composer--mega .sticky-composer__attach {
    font-size: 16px;
    color: #aab2bd;
}

.sticky-composer--mega .sticky-composer__area {
    font-size: 13px;
    color: #1f2937;
    padding: 4px 0;
}

.sticky-composer--mega .sticky-composer__area::placeholder {
    color: #c6ccd4;
}

.sticky-composer--mega .sticky-composer__file {
    background: #f1f5f9;
    color: #475569;
}

.sticky-composer--mega .sticky-composer__progress {
    color: #1f2937;
}

.sticky-composer--terminal {
    background: #161b22;
    border: 1px solid #30363d;
    border-radius: 8px;
    padding: 9px 11px;
    box-shadow: none;
    font-family: 'SF Mono', 'Menlo', 'Consolas', monospace;
}

.sticky-composer--terminal .sticky-composer__row {
    gap: 8px;
}

.sticky-composer--terminal .sticky-composer__attach {
    font-size: 13px;
    color: #8b949e;
}

.sticky-composer--terminal .sticky-composer__area {
    font-family: inherit;
    font-size: 12px;
    color: #c9d1d9;
    padding: 4px 0;
}

.sticky-composer--terminal .sticky-composer__area::placeholder {
    color: #6e7681;
}

.sticky-composer--terminal .sticky-composer__file {
    background: #21262d;
    color: #c9d1d9;
}

.sticky-composer--terminal .sticky-composer__progress {
    color: #58a6ff;
}

.sticky-composer__go--terminal {
    background: #238636;
    color: #fff;
    border-radius: 6px;
    font-size: 11px;
    padding: 5px 12px;
    font-weight: 700;
    font-family: 'SF Mono', 'Menlo', 'Consolas', monospace;
}

.sticky-composer--workbench {
    background: #fff;
    border: 1px solid #dfe4ea;
    border-radius: 10px;
    padding: 11px 13px;
    box-shadow: 0 1px 3px rgba(30, 45, 62, 0.06);
}

.sticky-composer--workbench .sticky-composer__row {
    gap: 9px;
}

.sticky-composer--workbench .sticky-composer__attach {
    font-size: 15px;
    color: #9ca3af;
}

.sticky-composer--workbench .sticky-composer__area {
    font-size: 12px;
    color: #1a2332;
    padding: 4px 0;
}

.sticky-composer--workbench .sticky-composer__area::placeholder {
    color: #aab2bd;
}

.sticky-composer--workbench .sticky-composer__file {
    background: #f1f5f9;
    color: #475569;
}

.sticky-composer--workbench .sticky-composer__progress {
    color: #1a2332;
}

.sticky-composer__go--workbench {
    background: #1e88e5;
    border-radius: 9px;
    font-size: 12px;
    padding: 8px 15px;
    font-weight: 650;
}

.sticky-composer--chat {
    background: #fff;
    border: 1px solid #dfe4ea;
    border-radius: 14px;
    padding: 11px 14px;
    box-shadow: 0 1px 3px rgba(30, 45, 62, 0.05);
}

.sticky-composer--chat .sticky-composer__attach {
    font-size: 15px;
    color: #9ca3af;
}

.sticky-composer--chat .sticky-composer__area {
    font-size: 12px;
    color: #111827;
    padding: 4px 0;
}

.sticky-composer--chat .sticky-composer__area::placeholder {
    color: #aab2bd;
}

.sticky-composer--chat .sticky-composer__file {
    background: #f1f5f9;
    color: #475569;
}

.sticky-composer--chat .sticky-composer__progress {
    color: #111827;
}

.sticky-composer__go--chat {
    background: #1e88e5;
    border-radius: 9px;
    font-size: 12px;
    padding: 8px 15px;
    font-weight: 650;
}
</style>