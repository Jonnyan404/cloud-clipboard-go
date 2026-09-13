<script setup>import { computed, inject } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { MODES } from '@/views/modes/registry.js';

const props = defineProps({
    variant: { type: String, default: 'default' },
});

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();

const actions = inject('pageToolbarActions', {});

const roomName = computed(() => ws.room || t('publicRoom'));
const isProtected = computed(() => Boolean(ws.roomProtectionCache?.[ws.normalizeRoomName(ws.room)]));
const latencyValue = computed(() => {
    if (ws.latency === null) {
        return '';
    }
    return `${Math.round(ws.latency)} ms`;
});
const latencyHexColor = computed(() => {
    if (ws.latency === null) {
        return '';
    }
    let colorName = 'success';
    if (ws.latency >= 60 && ws.latency < 120) {
        colorName = 'warning';
    } else if (ws.latency >= 120) {
        colorName = 'error';
    }
    const themeColors = theme.themes.value[isDark.value ? 'dark' : 'light'].colors;
    return themeColors[colorName] || colorName;
});

function setMode(mode) {
    app.setUiMode(mode);
}
</script>

<template>
    <div
        class="page-toolbar"
        :class="[
            `page-toolbar--${variant}`,
            { 'page-toolbar--dark': isDark }
        ]"
    >
        <div class="page-toolbar__inner">
            <div class="page-toolbar__leading">
                <v-tooltip v-if="ws.room" :text="t('backToDefaultRoom')" location="bottom">
                    <template v-slot:activator="{ props }">
                        <v-btn icon density="compact" size="small" variant="text" class="page-toolbar__home-btn" v-bind="props" @click="ws.switchRoom('')">
                            <v-icon size="small">mdi-home-outline</v-icon>
                        </v-btn>
                    </template>
                </v-tooltip>

                <v-chip
                    size="small"
                    variant="tonal"
                    :color="variant === 'sticky' ? 'amber-darken-1' : 'primary'"
                    class="page-toolbar__room"
                    :title="t('showQrCode')"
                    @click="actions.openPageQr && actions.openPageQr()"
                >
                    <v-icon start size="x-small">
                        {{ isProtected ? 'mdi-lock' : 'mdi-earth' }}
                    </v-icon>
                    <span v-if="ws.room" class="page-toolbar__roomname">{{ roomName }}</span>
                    <span v-else>{{ roomName }}</span>
                    <span
                        v-if="ws.websocket && ws.latency !== null"
                        class="page-toolbar__latency"
                        :style="{ color: latencyHexColor }"
                    >
                        {{ latencyValue }}
                    </span>
                </v-chip>
            </div>

            <div class="page-toolbar__actions">
                <div class="page-toolbar__modes">
                    <button
                        v-for="mode in MODES"
                        :key="mode.key"
                        class="page-toolbar__mode"
                        :title="t(mode.labelKey)"
                        :class="{ 'page-toolbar__mode--on': app.uiMode === mode.key }"
                        @click="setMode(mode.key)"
                    >
                        <v-icon size="small">{{ mode.icon }}</v-icon>
                        <span class="page-toolbar__mode-label d-none d-sm-inline">{{ t(mode.labelKey) }}</span>
                    </button>
                </div>

                <v-tooltip v-if="actions.roomListEnabled" :text="t('roomList')" location="bottom">
                    <template v-slot:activator="{ props }">
                        <v-btn icon density="compact" size="small" variant="text" v-bind="props" @click="actions.openRoomBrowser && actions.openRoomBrowser()">
                            <v-badge :content="actions.roomCount" :model-value="actions.roomCount > 0" color="accent" overlap>
                                <v-icon size="small">mdi-view-list</v-icon>
                            </v-badge>
                        </v-btn>
                    </template>
                </v-tooltip>

                <v-tooltip :text="t('enterRoom')" location="bottom">
                    <template v-slot:activator="{ props }">
                        <v-btn icon density="compact" size="small" variant="text" v-bind="props" @click="actions.openRoomDialog && actions.openRoomDialog()">
                            <v-icon size="small">mdi-door-open</v-icon>
                        </v-btn>
                    </template>
                </v-tooltip>

                <v-tooltip location="bottom">
                    <template v-slot:activator="{ props }">
                        <v-btn icon density="compact" size="small" variant="text" v-bind="props" @click="actions.toggleConnection && actions.toggleConnection()">
                            <v-icon v-if="ws.websocket" size="small">mdi-lan-connect</v-icon>
                            <v-icon v-else-if="ws.websocketConnecting" size="small">mdi-lan-pending</v-icon>
                            <v-icon v-else size="small">mdi-lan-disconnect</v-icon>
                        </v-btn>
                    </template>
                    <span v-if="ws.websocket">{{ t('connected') }}</span>
                    <span v-else-if="ws.websocketConnecting">{{ t('connecting') }}</span>
                    <span v-else>{{ t('disconnected') }}</span>
                </v-tooltip>

                <v-tooltip :text="t('clearClipboard')" location="bottom">
                    <template v-slot:activator="{ props }">
                        <v-btn icon density="compact" size="small" variant="text" v-bind="props" @click="actions.openClearAll && actions.openClearAll()">
                            <v-icon size="small">mdi-broom</v-icon>
                        </v-btn>
                    </template>
                </v-tooltip>

                <v-tooltip :text="t('settings')" location="bottom">
                    <template v-slot:activator="{ props }">
                        <v-btn icon density="compact" size="small" variant="text" v-bind="props" @click="actions.openSettings && actions.openSettings()">
                            <v-icon size="small">mdi-cog</v-icon>
                        </v-btn>
                    </template>
                </v-tooltip>
            </div>
        </div>
    </div>
</template>

<style scoped>
.page-toolbar {
    position: sticky;
    top: 0;
    z-index: 40;
}

.page-toolbar--default {
    background: #f5f7fa;
    border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.page-toolbar--sticky {
    background: #f3ead2;
    border-bottom: 1px solid rgba(120, 90, 40, 0.16);
}

.page-toolbar--dark.page-toolbar--default {
    background: #1e1e24;
    border-bottom-color: rgba(148, 163, 184, 0.22);
}

.page-toolbar--dark.page-toolbar--sticky {
    background: #211d12;
    border-bottom-color: rgba(238, 232, 214, 0.12);
}

.page-toolbar__inner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    max-width: 1100px;
    margin: 0 auto;
    padding: 8px 16px;
    flex-wrap: nowrap;
}

.page-toolbar__leading {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1 1 auto;
    min-width: 0;
}

.page-toolbar__home-btn {
    flex-shrink: 0;
}

.page-toolbar__room {
    cursor: pointer;
    flex: 0 1 auto;
    min-width: 0;
    max-width: 46%;
}

.page-toolbar__room :deep(.v-chip__content) {
    min-width: 0;
}

.page-toolbar__room :deep(.v-chip__content > span) {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
}

.page-toolbar__roomname {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
}

.page-toolbar__latency {
    font-size: 0.7rem;
    font-weight: 700;
    margin-left: 6px;
    flex-shrink: 0;
}

.page-toolbar__room :deep(.v-icon) {
    flex-shrink: 0;
}

.page-toolbar__actions {
    display: flex;
    align-items: center;
    gap: 2px;
    flex-shrink: 0;
    min-width: 0;
}

.page-toolbar__modes {
    display: flex;
    align-items: center;
    gap: 2px;
    margin-right: 6px;
    border: 1px solid rgba(148, 163, 184, 0.35);
    border-radius: 999px;
    padding: 2px;
    background: rgba(255, 255, 255, 0.6);
}

.page-toolbar--dark .page-toolbar__modes {
    background: rgba(0, 0, 0, 0.25);
}

.page-toolbar__mode {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    border: none;
    background: transparent;
    border-radius: 999px;
    padding: 3px 10px;
    font-size: 0.72rem;
    cursor: pointer;
    color: rgba(100, 116, 139, 0.9);
    transition: background 0.15s, color 0.15s;
}

.page-toolbar--dark .page-toolbar__mode {
    color: rgba(148, 163, 184, 0.9);
}

.page-toolbar__mode--on {
    background: #1e88e5;
    color: #fff;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
}

.page-toolbar--dark .page-toolbar__mode--on {
    background: #1e88e5;
    color: #fff;
}

.page-toolbar__mode--on :deep(.v-icon) {
    color: #fff;
}

@media (max-width: 600px) {
    .page-toolbar__room {
        flex: 1 1 auto;
        max-width: none;
    }
}
</style>