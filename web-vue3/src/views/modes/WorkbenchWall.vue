<script setup>import { computed, inject, ref, watch } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import {
    formatTimestamp,
    prettyFileSize,
    buildCleanAbsoluteRouteUrl,
    createShareLink,
    copyTextToClipboard,
    SHARE_DEFAULT_TTL,
} from '@/util.js';
import PageToolbar from '@/components/PageToolbar.vue';
import StickyComposer from '@/components/sticky/StickyComposer.vue';
import { useStickyAutoscroll } from '@/composables/useStickyAutoscroll';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();
const actions = inject('pageToolbarActions', {});

const WORKBENCH_ROOMS_KEY = 'workbenchRooms';
const localRooms = ref(loadLocalRooms());
const newRoomDialog = ref(false);
const newRoomName = ref('');
const newRoomNameInput = ref(null);

function loadLocalRooms() {
    const rooms = [];
    try {
        const parsed = JSON.parse(localStorage.getItem(WORKBENCH_ROOMS_KEY) || '[]');
        if (Array.isArray(parsed)) {
            for (const room of parsed) {
                const normalized = ws.normalizeRoomName(room);
                if (!rooms.includes(normalized)) {
                    rooms.push(normalized);
                }
            }
        }
    } catch (error) {
        console.error('解析本地房间列表失败:', error);
    }
    if (!rooms.includes('')) {
        rooms.unshift('');
    }
    return rooms;
}

function saveLocalRooms() {
    localStorage.setItem(WORKBENCH_ROOMS_KEY, JSON.stringify(localRooms.value));
}

const activeRoom = computed(() => ws.room);

function openNewRoomDialog() {
    newRoomName.value = '';
    newRoomDialog.value = true;
    setTimeout(() => {
        if (newRoomNameInput.value && typeof newRoomNameInput.value.focus === 'function') {
            newRoomNameInput.value.focus();
        }
    }, 50);
}

function createRoom() {
    const name = ws.normalizeRoomName(newRoomName.value);
    if (!name) {
        toast(t('workbenchRoomNameInvalid'));
        return;
    }
    if (!localRooms.value.includes(name)) {
        localRooms.value.push(name);
        saveLocalRooms();
    }
    newRoomDialog.value = false;
    ws.switchRoom(name);
    toast(t('workbenchRoomCreated', { room: name }));
}

function removeRoom(room) {
    const index = localRooms.value.indexOf(room);
    if (index === -1) {
        return;
    }
    localRooms.value.splice(index, 1);
    saveLocalRooms();
    if (activeRoom.value === room) {
        ws.switchRoom('');
    }
}

function switchToRoom(room) {
    ws.switchRoom(room);
}

const items = computed(() => app.received);
const streamItems = computed(() => [...items.value].reverse());
const filesPane = computed(() => streamItems.value.filter(item => item.type === 'file'));
const textsPane = computed(() => streamItems.value.filter(item => item.type === 'text'));
const streamEl = ref(null);
const { pinToBottom } = useStickyAutoscroll(streamEl, {
    items: () => [...streamItems.value],
    room: () => ws.room,
});

const detailItem = ref(null);
const downloading = ref(false);
const previewLoading = ref(false);
const srcPreview = ref(null);
const textPreview = ref('');
const textPreviewDisplayLimit = 16 * 1024;
const showFullTextPreview = ref(false);
const expired = computed(() => {
    if (!detailItem.value?.expire || detailItem.value.expire <= 0) {
        return false;
    }
    return Date.now() / 1000 > detailItem.value.expire;
});
const isExpirable = computed(() => Boolean(detailItem.value?.expire && detailItem.value.expire > 0));
const expireLabel = computed(() => {
    if (!isExpirable.value) {
        return '';
    }
    return `${expired.value ? t('expired') : t('expiresAt', { time: formatTimestamp(detailItem.value.expire) })}`;
});

const isPreviewableVideo = computed(() => detailItem.value?.name?.match(/\.(mp4|webm|ogv)$/gi));
const isPreviewableAudio = computed(() => detailItem.value?.name?.match(/\.(mp3|wav|ogg|opus|m4a|flac)$/gi));
const isPreviewableText = computed(() => detailItem.value?.name?.match(/\.(txt|text|md|markdown|json|log|csv|tsv|ya?ml|xml|ini|conf|cfg|toml|properties|env|gitignore|dockerfile|js|jsx|mjs|cjs|ts|tsx|vue|css|scss|sass|less|html|htm|sql|sh|bash|zsh|fish|ps1|bat|cmd|go|py|java|kt|kts|rb|php|rs|c|cc|cpp|cxx|h|hh|hpp|hxx|swift|proto)$/gi));
const canPreview = computed(() => Boolean(detailItem.value?.type === 'file' && !expired.value && (Boolean(detailItem.value.thumbnail) || isPreviewableVideo.value || isPreviewableAudio.value || isPreviewableText.value)));
const hasTruncatedTextPreview = computed(() => textPreview.value.length > textPreviewDisplayLimit);
const displayedTextPreview = computed(() => {
    if (!hasTruncatedTextPreview.value || showFullTextPreview.value) {
        return textPreview.value;
    }
    return `${textPreview.value.slice(0, textPreviewDisplayLimit)}\n\n...`;
});

function decodedContent(item) {
    const textArea = document.createElement('textarea');
    textArea.innerHTML = item.content || '';
    return textArea.value;
}

function fileIcon(item) {
    const name = item.name || '';
    if (/\.(png|jpe?g|gif|webp|svg|bmp|ico|avif)$/i.test(name)) {
        return '🖼️';
    }
    if (/\.(mp4|webm|ogv|mov)$/i.test(name)) {
        return '🎬';
    }
    if (/\.(mp3|wav|ogg|opus|m4a|flac)$/i.test(name)) {
        return '🎵';
    }
    return '📄';
}

const timeLabel = (item) => formatTimestamp(item.timestamp);

const needsShareProtection = computed(() => Boolean(app.config?.auth));

async function ensureFileShareUrl(item) {
    const cache = item?.cache;
    if (!cache) {
        return '';
    }
    if (!needsShareProtection.value) {
        const encodedFilename = encodeURIComponent(item.name || 'file');
        return buildCleanAbsoluteRouteUrl(`file/${cache}/${encodedFilename}`, app.config?.server?.prefix || '');
    }
    const data = await createShareLink({ type: 'file', uuid: cache, ttl: SHARE_DEFAULT_TTL, maxUses: 0, room: ws.room });
    return data?.url || '';
}

async function downloadFile() {
    if (expired.value || downloading.value) {
        return;
    }
    downloading.value = true;
    try {
        const url = await ensureFileShareUrl(detailItem.value);
        const downloadUrl = new URL(url, window.location.origin);
        downloadUrl.searchParams.set('download', 'true');
        const anchor = document.createElement('a');
        anchor.href = downloadUrl.toString();
        anchor.download = detailItem.value?.name || 'file';
        anchor.rel = 'noopener';
        document.body.appendChild(anchor);
        anchor.click();
        document.body.removeChild(anchor);
    } catch (error) {
        console.error('下载失败:', error);
        toast(t('fileFetchFailed'));
    } finally {
        downloading.value = false;
    }
}

async function downloadItem(item) {
    if (isItemExpired(item) || downloading.value) {
        return;
    }
    downloading.value = true;
    try {
        const url = await ensureFileShareUrl(item);
        const downloadUrl = new URL(url, window.location.origin);
        downloadUrl.searchParams.set('download', 'true');
        const anchor = document.createElement('a');
        anchor.href = downloadUrl.toString();
        anchor.download = item?.name || 'file';
        anchor.rel = 'noopener';
        document.body.appendChild(anchor);
        anchor.click();
        document.body.removeChild(anchor);
    } catch (error) {
        console.error('下载失败:', error);
        toast(t('fileFetchFailed'));
    } finally {
        downloading.value = false;
    }
}

async function copyContent(item) {
    try {
        await copyTextToClipboard(decodedContent(item));
        toast(t('copySuccess'));
    } catch (err) {
        console.error('复制失败:', err);
        toast(t('copyFailedGeneral'));
    }
}

async function copyFileLink(item) {
    try {
        const url = item.cache ? await ensureFileShareUrl(item) : contentUrlOf(item);
        await copyTextToClipboard(url);
        toast(t('copySuccess'));
    } catch (err) {
        console.error('复制失败:', err);
        toast(t('copyFailedGeneral'));
    }
}

function contentUrlOf(item) {
    const roomQuery = ws.room ? `?room=${encodeURIComponent(ws.room)}` : '';
    return buildCleanAbsoluteRouteUrl(`content/${item.id}${roomQuery}`, app?.config?.server?.prefix || '');
}

async function deleteItem(item) {
    try {
        await axios.delete(`revoke/${item.id}`, {
            params: new URLSearchParams([['room', ws.room]]),
        });
        toast(t('deleteSuccessText', { name: item.name }));
    } catch (error) {
        if (error.response && error.response.data.msg) {
            toast(t('deleteFailedMessageMsg', { msg: error.response.data.msg }));
        } else {
            toast(t('deleteFailedMessage'));
        }
    }
}

function isItemExpired(item) {
    if (!item?.expire || item.expire <= 0) {
        return false;
    }
    return Date.now() / 1000 > item.expire;
}

async function loadPreview() {
    if (!canPreview.value) {
        return;
    }
    srcPreview.value = null;
    textPreview.value = '';
    showFullTextPreview.value = false;
    if (isPreviewableVideo.value || isPreviewableAudio.value) {
        previewLoading.value = true;
        try {
            srcPreview.value = await ensureFileShareUrl(detailItem.value);
        } catch (error) {
            console.error('生成预览链接失败:', error);
            toast(t('fileFetchFailed'));
        } finally {
            previewLoading.value = false;
        }
    } else if (isPreviewableText.value) {
        previewLoading.value = true;
        try {
            const response = await axios.get(`file/${detailItem.value.cache}/${encodeURIComponent(detailItem.value.name)}`, {
                responseType: 'text',
            });
            textPreview.value = typeof response.data === 'string' ? response.data : String(response.data || '');
        } catch (error) {
            if (error.response && error.response.data.msg) {
                toast(t('fileFetchFailedMsg', { msg: error.response.data.msg }));
            } else {
                toast(t('fileFetchFailed'));
            }
        } finally {
            previewLoading.value = false;
        }
    } else {
        previewLoading.value = true;
        try {
            const response = await axios.get(`file/${detailItem.value.cache}/${encodeURIComponent(detailItem.value.name)}`, {
                responseType: 'arraybuffer',
            });
            srcPreview.value = URL.createObjectURL(new Blob([response.data]));
        } catch (error) {
            if (error.response && error.response.data.msg) {
                toast(t('fileFetchFailedMsg', { msg: error.response.data.msg }));
            } else {
                toast(t('fileFetchFailed'));
            }
        } finally {
            previewLoading.value = false;
        }
    }
}

function toggleTextPreview() {
    showFullTextPreview.value = !showFullTextPreview.value;
}

watch(detailItem, (item) => {
    if (item) {
        loadPreview();
    }
});
</script>

<template>
    <div
        class="workbench-wall"
        :class="{ 'workbench-wall--dark': isDark }"
    >
        <PageToolbar variant="workbench"></PageToolbar>

        <div class="workbench-wall__body">
            <div class="workbench-wall__tabs">
                <button
                    v-for="room in localRooms"
                    :key="'wb-'+room"
                    type="button"
                    class="workbench-wall__tab"
                    :class="{ 'workbench-wall__tab--active': activeRoom === room }"
                    @click="switchToRoom(room)"
                ><i></i><span class="workbench-wall__tab-name">{{ room || t('publicRoom') }}</span>
                    <span
                        v-if="room"
                        class="workbench-wall__tab-close"
                        role="button"
                        :title="t('delete')"
                        @click.stop="removeRoom(room)"
                    >✕</span>
                </button>
                <div class="workbench-wall__tabs-spacer"></div>
                <button type="button" class="workbench-wall__plus" :title="t('workbenchNewRoom')" @click="openNewRoomDialog">＋</button>
            </div>

            <div ref="streamEl" class="workbench-wall__panes">
                <section class="workbench-wall__pane">
                    <header class="workbench-wall__pane-head">
                        <span class="workbench-wall__pane-title">📄 {{ t('workbenchTodayFiles') }}</span>
                        <span class="workbench-wall__pane-count">{{ t('workbenchCount', { count: filesPane.length }) }}</span>
                    </header>
                    <div v-if="filesPane.length" class="workbench-wall__pane-body">
                        <div
                            v-for="item in filesPane"
                            :key="item.id"
                            class="workbench-wall__row workbench-wall__row--file"
                            role="button"
                            tabindex="0"
                            @click="detailItem = item"
                            @keydown.enter.prevent="detailItem = item"
                        >
                            <span class="workbench-wall__row-icon">{{ fileIcon(item) }}</span>
                            <span class="workbench-wall__row-title">{{ item.name || 'file' }}</span>
                            <span class="workbench-wall__row-meta">{{ prettyFileSize(item.size || 0) }}</span>
                            <span class="workbench-wall__row-time">{{ timeLabel(item) }}</span>
                        </div>
                    </div>
                    <div v-else class="workbench-wall__pane-empty">{{ t('workbenchNoFiles') }}</div>
                </section>

                <section class="workbench-wall__pane">
                    <header class="workbench-wall__pane-head">
                        <span class="workbench-wall__pane-title">✉️ {{ t('workbenchRecentTexts') }}</span>
                        <span class="workbench-wall__pane-count">{{ t('workbenchCount', { count: textsPane.length }) }}</span>
                    </header>
                    <div v-if="textsPane.length" class="workbench-wall__pane-body">
                        <div
                            v-for="item in textsPane"
                            :key="item.id"
                            class="workbench-wall__row workbench-wall__row--text"
                        >
                            <span class="workbench-wall__row-icon">📝</span>
                            <span class="workbench-wall__row-title">{{ decodedContent(item) }}</span>
                            <span class="workbench-wall__row-time">{{ timeLabel(item) }}</span>
                            <span class="workbench-wall__row-ops">
                                <button type="button" class="workbench-wall__op" :title="t('copyText')" @click="copyContent(item)">
                                    <v-icon size="medium">mdi-content-copy</v-icon>
                                </button>
                                <button type="button" class="workbench-wall__op workbench-wall__op--danger" :title="t('delete')" @click="deleteItem(item)">
                                    <v-icon size="medium">mdi-delete-outline</v-icon>
                                </button>
                            </span>
                        </div>
                    </div>
                    <div v-else class="workbench-wall__pane-empty">{{ t('workbenchNoTexts') }}</div>
                </section>

                <section class="workbench-wall__pane workbench-wall__pane--all">
                    <header class="workbench-wall__pane-head">
                        <span class="workbench-wall__pane-title">🔗 {{ t('workbenchAllRecords') }}</span>
                        <span class="workbench-wall__pane-count">{{ t('workbenchCount', { count: items.length }) }}</span>
                    </header>
                    <div v-if="items.length" class="workbench-wall__pane-body">
                        <div
                            v-for="item in streamItems"
                            :key="'all-' + item.id"
                            class="workbench-wall__row"
                            :class="`workbench-wall__row--${item.type}`"
                            :role="item.type === 'file' ? 'button' : undefined"
                            :tabindex="item.type === 'file' ? 0 : undefined"
                            @click="item.type === 'file' && (detailItem = item)"
                            @keydown.enter.prevent="item.type === 'file' && (detailItem = item)"
                        >
                            <span class="workbench-wall__row-icon">{{ item.type === 'file' ? fileIcon(item) : '📝' }}</span>
                            <span class="workbench-wall__row-title">{{ item.type === 'file' ? (item.name || 'file') : decodedContent(item) }}</span>
                            <span v-if="item.type === 'file'" class="workbench-wall__row-meta">{{ prettyFileSize(item.size || 0) }}</span>
                            <span class="workbench-wall__row-time">{{ timeLabel(item) }}</span>
                            <span class="workbench-wall__row-ops">
                                <button v-if="item.type === 'file'" type="button" class="workbench-wall__op" :title="isItemExpired(item) ? t('expired') : t('download')" @click.stop="item.cache && downloadItem(item)">
                                    <v-icon size="medium">mdi-download</v-icon>
                                </button>
                                <button type="button" class="workbench-wall__op" :title="t('copyText')" @click.stop="item.type === 'text' ? copyContent(item) : copyFileLink(item)">
                                    <v-icon size="medium">mdi-content-copy</v-icon>
                                </button>
                                <button type="button" class="workbench-wall__op workbench-wall__op--danger" :title="t('delete')" @click.stop="deleteItem(item)">
                                    <v-icon size="medium">mdi-delete-outline</v-icon>
                                </button>
                            </span>
                        </div>
                    </div>
                    <div v-else class="workbench-wall__pane-empty">{{ t('workbenchNoRecords') }}</div>
                </section>
            </div>
        </div>

        <div class="workbench-wall__composer">
            <sticky-composer variant="workbench" @sent="pinToBottom()"></sticky-composer>
        </div>

        <v-dialog v-model="newRoomDialog" max-width="360">
            <div class="workbench-wall__dialog" :class="{ 'workbench-wall__dialog--dark': isDark }">
                <div class="workbench-wall__dialog-title">{{ t('workbenchNewRoom') }}</div>
                <v-text-field
                    ref="newRoomNameInput"
                    v-model="newRoomName"
                    :placeholder="t('workbenchRoomPlaceholder')"
                    density="compact"
                    variant="outlined"
                    hide-details
                    autofocus
                    @keydown.enter.prevent="createRoom"
                ></v-text-field>
                <div class="workbench-wall__dialog-actions">
                    <v-spacer></v-spacer>
                    <v-btn variant="text" size="small" @click="newRoomDialog = false">{{ t('cancel') }}</v-btn>
                    <v-btn color="primary" variant="flat" size="small" @click="createRoom">{{ t('workbenchCreate') }}</v-btn>
                </div>
            </div>
        </v-dialog>

        <v-dialog v-model="detailItem" max-width="560">
            <div v-if="detailItem" class="workbench-wall__reader" :class="{ 'workbench-wall__reader--dark': isDark }">
                <div class="workbench-wall__reader-head">
                    <span class="workbench-wall__reader-type">{{ detailItem.type.toUpperCase() }}</span>
                    <span class="workbench-wall__reader-time">{{ timeLabel(detailItem) }}</span>
                    <v-btn icon density="compact" size="x-small" variant="text" class="workbench-wall__op" @click="detailItem = null">
                        <v-icon size="small">mdi-close</v-icon>
                    </v-btn>
                </div>
                <div v-if="detailItem.type === 'file'" class="workbench-wall__reader-file">
                    <span class="workbench-wall__reader-glyph">{{ fileIcon(detailItem) }}</span>
                    <span class="workbench-wall__reader-name">{{ detailItem.name }}</span>
                    <span class="workbench-wall__reader-meta">{{ prettyFileSize(detailItem.size || 0) }}</span>
                </div>
                <div v-if="detailItem.type === 'file' && isExpirable" class="workbench-wall__reader-expire" :class="{ 'workbench-wall__reader-expire--past': expired }">
                    <v-icon size="x-small">mdi-clock-outline</v-icon>
                    {{ expireLabel }}
                </div>

                <div v-if="detailItem.type === 'file' && canPreview" class="workbench-wall__reader-preview">
                    <div v-if="previewLoading" class="workbench-wall__preview-loading">
                        <v-progress-circular indeterminate color="primary" size="36"></v-progress-circular>
                    </div>
                    <template v-else>
                        <video
                            v-if="isPreviewableVideo"
                            :src="srcPreview"
                            style="max-height:60vh;max-width:100%;"
                            controls
                            preload="metadata"
                        ></video>
                        <audio
                            v-else-if="isPreviewableAudio"
                            :src="srcPreview"
                            style="width:100%"
                            controls
                            preload="metadata"
                        ></audio>
                        <template v-else-if="isPreviewableText">
                            <pre class="workbench-wall__preview-text">{{ displayedTextPreview }}</pre>
                            <div v-if="hasTruncatedTextPreview" class="d-flex justify-space-between align-center mt-2">
                                <div class="text-caption text-medium-emphasis">
                                    {{ t('textPreviewTruncated', { limit: prettyFileSize(textPreviewDisplayLimit) }) }}
                                </div>
                                <v-btn size="small" variant="text" color="primary" @click="toggleTextPreview">
                                    {{ showFullTextPreview ? t('collapseTextPreview') : t('expandTextPreview') }}
                                </v-btn>
                            </div>
                        </template>
                        <img
                            v-else
                            :src="srcPreview || detailItem.thumbnail"
                            style="max-height:60vh;max-width:100%;"
                        >
                    </template>
                </div>

                <div v-if="detailItem.type === 'file'" class="workbench-wall__reader-actions">
                    <v-btn
                        color="primary"
                        variant="flat"
                        size="small"
                        :loading="downloading"
                        :disabled="expired"
                        @click="downloadFile"
                    >
                        <v-icon start size="small">mdi-download</v-icon>{{ expired ? t('expired') : t('download') }}
                    </v-btn>
                    <v-btn variant="text" size="small" @click="copyFileLink(detailItem)">
                        <v-icon start size="small">mdi-link-variant</v-icon>{{ t('copyLink') }}
                    </v-btn>
                </div>
            </div>
        </v-dialog>
    </div>
</template>

<style scoped>
.workbench-wall {
    background: #eef0f4;
    height: 100vh;
    height: 100dvh;
    display: flex;
    flex-direction: column;
    color: #1a2332;
    font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", sans-serif;
    -webkit-font-smoothing: antialiased;
}

.workbench-wall--dark {
    background: #13161c;
    color: #e7eaf0;
}

.workbench-wall > .page-toolbar {
    flex-shrink: 0;
}

.workbench-wall__body {
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: 1100px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    padding: 0 10px 12px;
}

.workbench-wall__tabs {
    display: flex;
    gap: 6px;
    padding: 10px 4px 6px;
    flex-shrink: 0;
}

.workbench-wall__tab {
    flex: 1;
    max-width: 220px;
    min-width: 0;
    background: #fff;
    border: 1px solid #e4e8ee;
    border-radius: 9px;
    padding: 6px 10px;
    font-size: 11px;
    color: #475569;
    display: flex;
    align-items: center;
    gap: 5px;
    cursor: pointer;
    white-space: nowrap;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
}

.workbench-wall__tab-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
}

.workbench-wall__tab-close {
    margin-left: auto;
    padding: 0 3px;
    border-radius: 5px;
    font-size: 10px;
    line-height: 1.4;
    opacity: 0;
    transition: opacity 0.15s;
    flex-shrink: 0;
}

.workbench-wall__tab:hover .workbench-wall__tab-close,
.workbench-wall__tab--active .workbench-wall__tab-close {
    opacity: 0.8;
}

.workbench-wall__tab-close:hover {
    opacity: 1;
    background: rgba(255, 255, 255, 0.25);
}

.workbench-wall__tab i {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #9ca3af;
    flex-shrink: 0;
}

.workbench-wall__tab--active {
    background: #1e88e5;
    color: #fff;
    border-color: #1e88e5;
}

.workbench-wall__tab--active i {
    background: #fff;
}

.workbench-wall--dark .workbench-wall__tab {
    background: #1d2128;
    border-color: #2b3138;
    color: #9aa3ad;
}

.workbench-wall--dark .workbench-wall__tab--active {
    background: #1e88e5;
    color: #fff;
    border-color: #1e88e5;
}

.workbench-wall__tabs-spacer {
    flex: 1;
}

.workbench-wall__plus {
    background: none;
    border: 1px dashed #c5cdd6;
    border-radius: 9px;
    padding: 5px 10px;
    font-size: 13px;
    color: #8896a7;
    flex-shrink: 0;
    cursor: pointer;
}

.workbench-wall--dark .workbench-wall__plus {
    border-color: #323a44;
    color: #828c97;
}

.workbench-wall__panes {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 2px 2px 8px;
}

.workbench-wall__pane {
    background: #fff;
    border: 1px solid #e2e7ee;
    border-radius: 12px;
    margin: 0 0 10px;
    overflow: hidden;
}

.workbench-wall--dark .workbench-wall__pane {
    background: #1d2128;
    border-color: #2b3138;
}

.workbench-wall__pane-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 9px 12px;
    border-bottom: 1px solid #f0f2f5;
}

.workbench-wall--dark .workbench-wall__pane-head {
    border-bottom-color: #262c34;
}

.workbench-wall__pane-title {
    font-size: 12px;
    font-weight: 650;
}

.workbench-wall__pane-count {
    font-size: 10px;
    color: #a8b1bb;
}

.workbench-wall--dark .workbench-wall__pane-count {
    color: #6d7681;
}

.workbench-wall__pane-body {
    padding: 3px 0;
}

.workbench-wall__row {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 7px 12px;
    border-bottom: 1px solid #f6f7f9;
    min-width: 0;
}

.workbench-wall--dark .workbench-wall__row {
    border-bottom-color: #262c34;
}

.workbench-wall__row:last-child {
    border-bottom: none;
}

.workbench-wall__row--file {
    cursor: pointer;
}

.workbench-wall__row--file:hover {
    background: #f4f7fb;
}

.workbench-wall--dark .workbench-wall__row--file:hover {
    background: #232932;
}

.workbench-wall__row-icon {
    width: 26px;
    height: 26px;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    font-size: 13px;
}

.workbench-wall__row--file .workbench-wall__row-icon {
    background: #eef3fa;
}

.workbench-wall--dark .workbench-wall__row--file .workbench-wall__row-icon {
    background: #2a313b;
}

.workbench-wall__row--text .workbench-wall__row-icon {
    background: #f0f6f4;
}

.workbench-wall--dark .workbench-wall__row--text .workbench-wall__row-icon {
    background: #272e33;
}

.workbench-wall__row-title {
    flex: 1;
    min-width: 0;
    font-size: 11px;
    color: #334155;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.workbench-wall--dark .workbench-wall__row-title {
    color: #c6ccd4;
}

.workbench-wall__row--file .workbench-wall__row-title {
    color: #1a2332;
    font-weight: 650;
}

.workbench-wall--dark .workbench-wall__row--file .workbench-wall__row-title {
    color: #e7eaf0;
}

.workbench-wall__row-meta,
.workbench-wall__row-time {
    font-size: 10px;
    color: #b6bcc6;
    flex-shrink: 0;
}

.workbench-wall--dark .workbench-wall__row-meta,
.workbench-wall--dark .workbench-wall__row-time {
    color: #6d7681;
}

.workbench-wall__row-time {
    display: none;
}

@media (min-width: 480px) {
    .workbench-wall__row-time {
        display: inline;
    }
}

.workbench-wall__row-ops {
    display: none;
    gap: 2px;
    align-items: center;
    flex-shrink: 0;
}

.workbench-wall__row:hover .workbench-wall__row-ops,
.workbench-wall__row:focus-within .workbench-wall__row-ops {
    display: flex;
}

.workbench-wall__op {
    background: none;
    border: none;
    cursor: pointer;
    padding: 2px;
    color: #8896a7;
    display: inline-flex;
    align-items: center;
    justify-content: center;
}

.workbench-wall--dark .workbench-wall__op {
    color: #828c97;
}

.workbench-wall__op:hover {
    color: #1e88e5;
}

.workbench-wall__op--danger:hover {
    color: #d32f2f;
}

.workbench-wall__pane-empty {
    padding: 16px;
    text-align: center;
    color: #b6bcc6;
    font-size: 11px;
}

.workbench-wall--dark .workbench-wall__pane-empty {
    color: #6d7681;
}

.workbench-wall__composer {
    flex-shrink: 0;
    width: 100%;
    max-width: 1100px;
    margin: 0 auto;
    padding: 0 10px 14px;
    padding-bottom: calc(14px + env(safe-area-inset-bottom));
}

.workbench-wall__composer :deep(.sticky-composer--workbench) {
    background: var(--v-theme-surface);
}

.workbench-wall--dark .workbench-wall__composer :deep(.sticky-composer--workbench) {
    background: #1d2128;
    border-color: #2b3138;
}

.workbench-wall--dark .workbench-wall__composer :deep(.sticky-composer--workbench .sticky-composer__area) {
    color: #e7eaf0;
}

.workbench-wall--dark .workbench-wall__composer :deep(.sticky-composer--workbench .sticky-composer__area::placeholder) {
    color: #6d7681;
}

.workbench-wall--dark .workbench-wall__composer :deep(.sticky-composer--workbench .sticky-composer__attach) {
    color: #828c97;
}

.workbench-wall--dark .workbench-wall__composer :deep(.sticky-composer--workbench .sticky-composer__file) {
    background: #2b3138;
    color: #c6ccd4;
}

.workbench-wall--dark .workbench-wall__composer :deep(.sticky-composer--workbench .sticky-composer__progress) {
    color: #e7eaf0;
}

.workbench-wall__dialog {
    background: #fff;
    border-radius: 12px;
    border: 1px solid #e2e7ee;
    padding: 18px;
    color: #1a2332;
    font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", sans-serif;
}

.workbench-wall__dialog--dark {
    background: #1d2128;
    border-color: #2b3138;
    color: #e7eaf0;
}

.workbench-wall__dialog-title {
    font-size: 15px;
    font-weight: 700;
    margin-bottom: 14px;
}

.workbench-wall__dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 16px;
}

.workbench-wall__reader {
    border-radius: 12px;
    background: #fff;
    border: 1px solid #e2e7ee;
    padding: 18px;
    color: #1a2332;
    font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", sans-serif;
}

.workbench-wall__reader--dark {
    background: #1d2128;
    border-color: #2b3138;
    color: #e7eaf0;
}

.workbench-wall__reader-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
    font-size: 12px;
    color: #8896a7;
}

.workbench-wall__reader-type {
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    font-size: 11px;
    color: #1e88e5;
}

.workbench-wall__reader-time {
    color: #b6bcc6;
    font-size: 11px;
}

.workbench-wall__reader-head .workbench-wall__op {
    margin-left: auto;
}

.workbench-wall__reader-file {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
}

.workbench-wall__reader-glyph {
    font-size: 28px;
    flex-shrink: 0;
}

.workbench-wall__reader-name {
    font-size: 15px;
    font-weight: 650;
    word-break: break-all;
}

.workbench-wall__reader-meta {
    font-size: 12px;
    color: #8896a7;
    flex-shrink: 0;
    margin-left: auto;
}

.workbench-wall__reader-expire {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: #64748b;
    background: #f1f5f9;
    border-radius: 999px;
    padding: 3px 10px;
    margin-bottom: 12px;
}

.workbench-wall__reader-expire--past {
    color: #b91c1c;
    background: #fef2f2;
}

.workbench-wall__reader-preview {
    margin-bottom: 12px;
}

.workbench-wall__preview-loading {
    display: flex;
    justify-content: center;
    padding: 24px 0;
}

.workbench-wall__preview-text {
    background: #f6f7f9;
    border: 1px solid #eef0f2;
    border-radius: 10px;
    padding: 12px;
    font-size: 12px;
    line-height: 1.6;
    overflow: auto;
    max-height: 60vh;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: 'SF Mono', 'Menlo', 'Consolas', monospace;
}

.workbench-wall__reader-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
}
</style>