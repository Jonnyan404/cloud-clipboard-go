<script setup>import { computed, nextTick, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import { prettyFileSize } from '@/util.js';
import PageToolbar from '@/components/PageToolbar.vue';
import StickyNote from '@/components/sticky/StickyNote.vue';
import StickyComposer from '@/components/sticky/StickyComposer.vue';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();
const composer = ref(null);
const pageDragover = ref(false);
const dragDepth = ref(0);
const historyUsageLabel = computed(() => {
    const current = app.received.length;
    const limit = Number(app.config?.server?.history || 0);
    return `${current}/${limit}`;
});
function focusComposer() {
    nextTick(() => {
        if (composer.value && typeof composer.value.focus === 'function') {
            composer.value.focus();
        }
    });
}
function handleDragEnter() {
    dragDepth.value += 1;
    pageDragover.value = true;
}
function handleDragLeave() {
    dragDepth.value = Math.max(0, dragDepth.value - 1);
    if (dragDepth.value === 0) {
        pageDragover.value = false;
    }
}
function handlePageDrop(event) {
    dragDepth.value = 0;
    pageDragover.value = false;
    if (!(event && event.dataTransfer)) {
        return;
    }
    const files = Array.from(event.dataTransfer.files || []);
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
    if (composer.value && typeof composer.value.addFiles === 'function') {
        composer.value.addFiles(files);
    }
}
</script>

<template>
    <div
        class="sticky-wall"
        :class="{ 'sticky-wall--dark': isDark, 'sticky-wall--dragover': pageDragover }"
        @dragenter.prevent="handleDragEnter"
        @dragover.prevent
        @dragleave.prevent="handleDragLeave"
        @drop.prevent="handlePageDrop"
    >
        <div v-if="pageDragover" class="sticky-wall__dropglow">
            <span>{{ t('stickyDropHere') }}</span>
        </div>
        <PageToolbar variant="sticky"></PageToolbar>
        <div class="sticky-wall__shell mx-auto">
            <div class="sticky-wall__head">
                <span class="sticky-wall__pinned">📌</span>
                <span class="sticky-wall__count">{{ historyUsageLabel }} {{ t('uiModeStickyCount') }}</span>
            </div>

            <div v-if="app.received.length" class="sticky-wall__stream">
                <div
                    v-for="item in app.received"
                    :key="item.id"
                    class="sticky-wall__item"
                >
                    <sticky-note :meta="item"></sticky-note>
                </div>
            </div>

            <div v-else class="sticky-wall__empty">
                <div class="text-h6 font-weight-medium mb-2">{{ t('emptyTimelineTitle') }}</div>
                <div class="text-body-2 text-medium-emphasis mb-4">{{ t('timelineEmptySubtitle') }}</div>
                <v-btn size="small" variant="flat" color="primary" @click="focusComposer">
                    {{ t('quickSend') }}
                </v-btn>
            </div>

            <div class="sticky-wall__composer">
                <sticky-composer ref="composer"></sticky-composer>
            </div>
        </div>
    </div>
</template>

<style scoped>
.sticky-wall {
    background: #fdf7e4;
    height: 100vh;
    height: 100dvh;
    display: flex;
    flex-direction: column;
    color: #444034;
}

.sticky-wall__dropglow {
    position: fixed;
    inset: 0;
    z-index: 999;
    background: rgba(217, 119, 6, 0.14);
    backdrop-filter: blur(2px);
    display: flex;
    align-items: center;
    justify-content: center;
    pointer-events: none;
    font-size: 20px;
    font-weight: 700;
    color: #d97706;
    border: 3px dashed #d97706;
}

.sticky-wall--dragover {
    cursor: copy;
}

.sticky-wall--dragover::after {
    content: '';
    position: fixed;
    inset: 0;
    z-index: 998;
    background: rgba(217, 119, 6, 0.08);
    pointer-events: none;
}

.sticky-wall--dark {
    background: #211d12;
    color: rgba(238, 232, 214, 0.95);
}

.sticky-wall > .page-toolbar {
    flex-shrink: 0;
}

.sticky-wall__shell {
    max-width: 1100px;
    width: 100%;
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    padding: 0 16px 24px;
}

.sticky-wall__head {
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 8px;
    padding: 18px 4px 10px;
    font-size: 17px;
    font-weight: 700;
    color: #6f6548;
    flex-shrink: 0;
}

.sticky-wall--dark .sticky-wall__head {
    color: #e6ddc3;
}

.sticky-wall__pinned {
    font-size: 15px;
}

.sticky-wall__count {
    font-size: 11px;
    font-weight: 500;
    color: #bbaa85;
    background: rgba(187, 170, 133, 0.18);
    border-radius: 999px;
    padding: 2px 10px;
}

.sticky-wall--dark .sticky-wall__count {
    color: #9a8b62;
    background: rgba(154, 139, 98, 0.22);
}

.sticky-wall__stream {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    display: flex;
    flex-wrap: wrap;
    gap: 14px;
    padding: 14px 2px;
    align-content: flex-start;
}

.sticky-wall__item {
    flex: 1 1 220px;
    max-width: 100%;
    min-width: 0;
    display: flex;
}

.sticky-wall__item > * {
    width: 100%;
}

.sticky-wall__empty {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    text-align: center;
    padding: 60px 16px;
    border: 1.5px dashed #d5c49a;
    background: rgba(255, 251, 232, 0.6);
    border-radius: 12px;
    margin: 14px 2px;
}

.sticky-wall--dark .sticky-wall__empty {
    border-color: #4a3f28;
    background: rgba(33, 29, 18, 0.5);
}

.sticky-wall--dark .sticky-wall__empty .text-medium-emphasis {
    color: rgba(238, 232, 214, 0.6) !important;
}

.sticky-wall__composer {
    margin-top: 14px;
    flex-shrink: 0;
    padding-bottom: env(safe-area-inset-bottom);
}

.sticky-wall--dark .sticky-wall__composer :deep(.sticky-composer) {
    background: rgba(255, 251, 232, 0.06);
    border-color: #4a3f28;
}

.sticky-wall--dark .sticky-wall__composer :deep(.sticky-composer__area) {
    color: rgba(238, 232, 214, 0.95);
}

.sticky-wall--dark .sticky-wall__composer :deep(.sticky-composer__area::placeholder) {
    color: rgba(184, 174, 154, 0.7);
}

.sticky-wall--dark .sticky-wall__composer :deep(.sticky-composer__file) {
    background: rgba(154, 139, 98, 0.24);
    color: #e6ddc3;
}

@media (min-width: 601px) and (max-width: 960px) {
    .sticky-wall__item {
        flex: 1 1 44%;
    }
}

@media (max-width: 600px) {
    .sticky-wall__item {
        flex: 1 1 100%;
    }
}
</style>