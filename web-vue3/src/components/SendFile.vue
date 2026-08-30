<template>
    <div class="send-panel" :class="{ 'send-panel--compact': compact }">
        <div v-if="!hideTitle" class="text-h5 text-primary mb-4">{{ t('sendFile') }}</div>
        <v-card
            variant="outlined"
            class="send-file-dropzone pa-3 mb-6 d-flex flex-row align-center"
            @dragenter="$event.preventDefault()"
            @dragover="$event.preventDefault()"
            @dragleave="$event.preventDefault()"
            @drop="$event.preventDefault(); handleSelectFiles(Array.from($event.dataTransfer.files))"
        >
            <template v-if="app.send.files.length">
                <template v-if="progress">
                    <div class="flex-grow-1">
                        <small class="d-block text-right text-medium-emphasis">
                            {{prettyFileSize(Math.min(uploadedSize, fileSize))}} / {{prettyFileSize(fileSize)}} ({{percentage(uploadProgress)}})
                        </small>
                        <v-progress-linear :value="uploadProgress * 100"></v-progress-linear>
                    </div>
                </template>
                <template v-else>
                    <v-img
                        v-if="isUploadingImage"
                        :src="imagePreview"
                        class="mr-3 flex-grow-0"
                        width="2.5rem"
                        height="2.5rem"
                        style="border-radius: 3px"
                    ></v-img>
                    <div class="flex-grow-1 mr-2" style="min-width: 0">
                        <div
                            class="text-truncate"
                            :title="app.send.files[0].name + ' ' + (app.send.files.length > 1 ? `等 ${app.send.files.length} 个文件` : '')"
                        >{{app.send.files[0].name}} {{app.send.files.length > 1 ? `等 ${app.send.files.length} 个文件` : ''}}
                        </div>
                        <div class="text-caption">{{prettyFileSize(fileSize)}}</div>
                    </div>
                    <div class="align-self-center">
                        <v-btn icon density="comfortable" variant="text" color="grey" @click="app.send.files.splice(0)">
                            <v-icon>{{mdiClose}}</v-icon>
                        </v-btn>
                    </div>
                </template>
            </template>
            <template v-else>
                <v-btn
                    variant="text"
                    color="primary"
                    size="large"
                    class="d-block mx-auto"
                    @click="focus"
                >
                    <div :title="t('dragDropPasteTip')">
                        {{ t('selectFileToSend') }}<span class="d-none d-xl-inline">{{ t('dragDropPasteTip') }}</span>
                        <br>
                        <small class="text-medium-emphasis">{{ t('fileSizeLimit', { limit: prettyFileSize(app.config.file.limit) }) }}</small>
                    </div>
                </v-btn>
                <input
                    ref="selectFile"
                    type="file"
                    class="d-none"
                    multiple
                    @change="handleSelectFiles(Array.from($event.target.files))"
                >
            </template>
        </v-card>
        <div class="text-right">
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

<script setup>import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useDisplay } from 'vuetify';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { toast } from '@/plugins/toast';
import { prettyFileSize, percentage } from '@/util.js';

const mdiClose = 'mdi-close';


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
const progress = ref(false);
const uploadedSizes = ref([]);
const imagePreview = ref('');
const uploading = ref(false);
const selectFile = ref(null);
const fileSize = computed(() => {
    return app.send.files.length ? app.send.files.reduce((acc, cur) => acc += cur.size, 0) : 0;
});
const uploadedSize = computed(() => {
    return uploadedSizes.value.length ? uploadedSizes.value.reduce((acc, cur) => acc += cur, 0) : 0;
});
const uploadProgress = computed(() => {
    return Math.min(fileSize.value !== 0 ? (uploadedSize.value / fileSize.value) : 0, 1);
});
const isDisabled = computed(() => {
    return !app.send.files.length || !ws.websocket || progress.value;
});
const isUploadingImage = computed(() => {
    return app.send.files.length && app.send.files[0].type.startsWith('image/');
});
function focus() {
    selectFile.value.click();
}
function handleSelectFiles(files) {
    if (files.some(e => !e.size)) {
        toast(t('cannotSendEmptyFile'));
    } else if (files.some(e => e.size > app.config.file.limit)) {
        toast(t('fileSizeExceeded', { limit: prettyFileSize(app.config.file.limit) }));
    } else {
        app.send.files.splice(0);
        app.send.files.push(...files);
        if (isUploadingImage.value) {
            URL.revokeObjectURL(imagePreview.value);
            imagePreview.value = URL.createObjectURL(files[0]);
        }
    }
}
async function send() {
    try {
        const chunkSize = app.config.file.chunk;
        uploadedSizes.value.splice(0);
        uploadedSizes.value.push(...Array(app.send.files.length).fill(0));
        await Promise.all(app.send.files.map(async (file, i) => {
            if (file.size < chunkSize) {
                const fd = new FormData;
                fd.set('file', file);
                progress.value = true;
                await axios.postForm('upload', fd, {
                    params: new URLSearchParams([['room', ws.room]]),
                    onUploadProgress: e => uploadedSizes.value[i] = e.loaded,
                });
            } else {
                const response = await axios.post('upload/chunk', file.name, {
                    headers: {'Content-Type': 'text/plain'},
                    params: new URLSearchParams([['room', ws.room]]),
                });
                const uuid = response.data.result.uuid;
                let uploadedSize = 0;
                progress.value = true;
                while (uploadedSize < file.size) {
                    const chunk = file.slice(uploadedSize, uploadedSize + chunkSize);
                    await axios.post(`upload/chunk/${uuid}`, chunk, {
                        headers: {'Content-Type': 'application/octet-stream'},
                        onUploadProgress: e => uploadedSizes.value[i] = uploadedSize + e.loaded,
                    });
                    uploadedSize += chunkSize;
                }
                await axios.post(`upload/finish/${uuid}`, null, {
                    params: new URLSearchParams([['room', ws.room]]),
                });
            }
        }));
        toast(t('sendSuccess'));
        app.send.files.splice(0);
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
function onPaste(e) {
    if (!(e && e.clipboardData)) return;
    const items = Array.from(e.clipboardData.items);
    if (!(items.length && items.every(e => e.kind === 'file'))) return;
    handleSelectFiles(items.map(e => e.getAsFile()));
}
onMounted(() => {
    document.onpaste = onPaste;
});
onBeforeUnmount(() => {
    document.onpaste = null;
});
</script>

<style scoped>
.send-file-dropzone {
    border-radius: 20px;
    border-style: dashed !important;
    border-width: 1.5px !important;
    border-color: rgba(14, 165, 233, 0.35) !important;
    background: rgba(248, 250, 252, 0.75);
}

.v-theme--dark .send-file-dropzone {
    border-color: rgba(56, 189, 248, 0.45) !important;
    background: rgba(30, 41, 59, 0.84);
}

.send-panel--compact .send-file-dropzone {
    margin-bottom: 1rem !important;
}
</style>
