<template>
    <v-card class="unified-composer" :class="{ 'unified-composer--dark': $vuetify.theme.dark }" outlined>
        <div class="unified-composer__body pa-3 pa-md-4">
            <v-textarea
                ref="textarea"
                v-model="$root.send.text"
                auto-grow
                no-resize
                flat
                solo
                dense
                rows="3"
                :placeholder="$t('enterTextToSend')"
                :hide-details="true"
                class="unified-composer__textarea"
                :style="composerTextareaStyle"
            ></v-textarea>

            <div class="unified-composer__meta px-1 pb-2">
                <span class="caption text--secondary mr-3">{{ textLimitLabel }}</span>
                <span class="caption text--secondary">{{ fileLimitLabel }}</span>
            </div>

            <div v-if="$root.send.files.length" class="unified-composer__attachments px-1 pb-2">
                <v-chip
                    v-for="(file, index) in $root.send.files"
                    :key="file.name + file.size + index"
                    close
                    outlined
                    small
                    class="mr-2 mb-2"
                    @click:close="removeFile(index)"
                >
                    {{ file.name }} · {{ prettyFileSize(file.size) }}
                </v-chip>
            </div>

            <div v-if="progress" class="px-1 pb-2">
                <small class="d-block text-right text--secondary mb-1">
                    {{ Math.min(uploadedSize, fileSize) | prettyFileSize }} / {{ fileSize | prettyFileSize }}
                </small>
                <v-progress-linear :value="uploadProgress * 100"></v-progress-linear>
            </div>

            <div class="unified-composer__footer pt-1">
                <div class="unified-composer__footer-main d-flex align-center flex-wrap">
                    <v-btn icon small color="grey darken-1" @click="openFilePicker">
                        <v-icon>{{ mdiPaperclip }}</v-icon>
                    </v-btn>
                    <v-btn icon small color="grey darken-1" @click="$emit('show-qr')">
                        <v-icon>{{ mdiQrcode }}</v-icon>
                    </v-btn>
                    <div class="caption text--secondary ml-2 unified-composer__hint">
                        {{ footerHint }}
                    </div>
                </div>

                <v-btn
                    depressed
                    color="primary"
                    class="unified-composer__send"
                    :disabled="sendDisabled"
                    @click="sendAll"
                >
                    <v-icon left small>{{ mdiSend }}</v-icon>
                    {{ $t('send') }}
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

<script>
import { prettyFileSize } from '@/util.js';
import {
    mdiPaperclip,
    mdiQrcode,
    mdiSend,
} from '@mdi/js';

export default {
    name: 'unified-composer',
    data() {
        return {
            progress: false,
            uploadedSizes: [],
            mdiPaperclip,
            mdiQrcode,
            mdiSend,
        };
    },
    computed: {
        fileSize() {
            return this.$root.send.files.length ? this.$root.send.files.reduce((acc, cur) => acc += cur.size, 0) : 0;
        },
        uploadedSize() {
            return this.uploadedSizes.length ? this.uploadedSizes.reduce((acc, cur) => acc += cur, 0) : 0;
        },
        uploadProgress() {
            return Math.min(this.fileSize !== 0 ? (this.uploadedSize / this.fileSize) : 0, 1);
        },
        sendDisabled() {
            return !this.$root.websocket || this.progress || (!this.$root.send.text && !this.$root.send.files.length) || this.$root.send.text.length > this.$root.config.text.limit;
        },
        footerHint() {
            if (this.$root.send.files.length) {
                return this.$t('composerFilesSelected', { count: this.$root.send.files.length });
            }
            return this.$t('dragDropPasteTip');
        },
        textLimitLabel() {
            return this.$t('composerTextLimit', {
                current: this.$root.send.text.length,
                limit: this.$root.config.text.limit,
            });
        },
        fileLimitLabel() {
            return this.$t('fileSizeLimit', {
                limit: prettyFileSize(this.$root.config.file.limit),
            });
        },
        composerTextareaStyle() {
            return {
                maxHeight: '12rem',
            };
        },
    },
    methods: {
        prettyFileSize,
        focus(type) {
            if (type === 'file') {
                this.openFilePicker();
                return;
            }
            if (this.$refs.textarea && typeof this.$refs.textarea.focus === 'function') {
                this.$refs.textarea.focus();
            }
        },
        openFilePicker() {
            this.$refs.selectFile.click();
        },
        removeFile(index) {
            this.$root.send.files.splice(index, 1);
        },
        handleSelectFiles(files) {
            if (!files.length) {
                return;
            }
            if (files.some(file => !file.size)) {
                this.$toast(this.$t('cannotSendEmptyFile'));
                return;
            }
            if (files.some(file => file.size > this.$root.config.file.limit)) {
                this.$toast(this.$t('fileSizeExceeded', { limit: prettyFileSize(this.$root.config.file.limit) }));
                return;
            }
            this.$root.send.files.splice(0);
            this.$root.send.files.push(...files);
        },
        async sendText() {
            if (!this.$root.send.text) {
                return;
            }
            await this.$http.post(
                'text',
                this.$root.send.text,
                {
                    params: new URLSearchParams([['room', this.$root.room]]),
                    headers: {
                        'Content-Type': 'text/plain',
                    },
                },
            );
            this.$root.send.text = '';
        },
        async sendFiles() {
            if (!this.$root.send.files.length) {
                return;
            }
            const chunkSize = this.$root.config.file.chunk;
            this.uploadedSizes.splice(0);
            this.uploadedSizes.push(...Array(this.$root.send.files.length).fill(0));
            this.progress = true;

            await Promise.all(this.$root.send.files.map(async (file, index) => {
                if (file.size < chunkSize) {
                    const formData = new FormData;
                    formData.set('file', file);
                    await this.$http.postForm('upload', formData, {
                        params: new URLSearchParams([['room', this.$root.room]]),
                        onUploadProgress: event => this.$set(this.uploadedSizes, index, event.loaded),
                    });
                    return;
                }

                const response = await this.$http.post('upload/chunk', file.name, { headers: { 'Content-Type': 'text/plain' } });
                const uuid = response.data.result.uuid;

                let uploadedSize = 0;
                while (uploadedSize < file.size) {
                    const chunk = file.slice(uploadedSize, uploadedSize + chunkSize);
                    await this.$http.post(`upload/chunk/${uuid}`, chunk, {
                        headers: { 'Content-Type': 'application/octet-stream' },
                        onUploadProgress: event => this.$set(this.uploadedSizes, index, uploadedSize + event.loaded),
                    });
                    uploadedSize += chunkSize;
                }

                await this.$http.post(`upload/finish/${uuid}`, null, {
                    params: new URLSearchParams([['room', this.$root.room]]),
                });
            }));

            this.$root.send.files.splice(0);
        },
        async sendAll() {
            try {
                if (this.$root.send.text) {
                    await this.sendText();
                }
                if (this.$root.send.files.length) {
                    await this.sendFiles();
                }
                this.$toast(this.$t('sendSuccess'));
                this.focus();
            } catch (error) {
                if (error.response && error.response.data.msg) {
                    this.$toast(this.$t('sendFailedMsg', { msg: error.response.data.msg }));
                } else {
                    this.$toast(this.$t('sendFailed'));
                }
            } finally {
                this.progress = false;
            }
        },
        handlePaste(event) {
            if (!(event && event.clipboardData)) {
                return;
            }
            const items = Array.from(event.clipboardData.items || []);
            const files = items.filter(item => item.kind === 'file').map(item => item.getAsFile()).filter(Boolean);
            if (files.length) {
                this.handleSelectFiles(files);
            }
        },
    },
    mounted() {
        document.addEventListener('paste', this.handlePaste);
        this.$nextTick(() => {
            this.focus();
        });
    },
    beforeDestroy() {
        document.removeEventListener('paste', this.handlePaste);
    },
}
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

.unified-composer__textarea ::v-deep .v-input__slot {
    box-shadow: none !important;
    border-radius: 16px;
    background: rgba(248, 250, 252, 0.95) !important;
    padding: 0.25rem 0.25rem 0 0.25rem;
}

.unified-composer--dark .unified-composer__textarea ::v-deep .v-input__slot {
    background: rgba(30, 41, 59, 0.96) !important;
}

.unified-composer--dark .unified-composer__textarea ::v-deep textarea,
.unified-composer--dark .unified-composer__meta,
.unified-composer--dark .unified-composer__hint,
.unified-composer--dark .unified-composer__attachments {
    color: rgba(226, 232, 240, 0.92) !important;
}

.unified-composer__textarea ::v-deep textarea {
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