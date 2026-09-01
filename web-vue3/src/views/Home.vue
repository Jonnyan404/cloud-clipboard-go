<script setup>import { computed, nextTick, ref } from 'vue';
import { onBeforeRouteUpdate, useRouter } from 'vue-router';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useDisplay } from 'vuetify';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { toast } from '@/plugins/toast';
import QrcodeVue from 'qrcode.vue';
import UnifiedComposer from '@/components/UnifiedComposer.vue';
import ReceivedText from '@/components/received-item/Text.vue';
import ReceivedFile from '@/components/received-item/File.vue';

const mdiTimeline = 'mdi-timeline';


const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const display = useDisplay();
const { t } = useI18n();
const router = useRouter();
const pageQrDialogVisible = ref(false);
const pageQrMode = ref('page');
const composer = ref(null);
const activeRoomName = computed(() => ws.room || t('publicRoom'));
const historyUsageLabel = computed(() => {
    const current = app.received.length;
    const limit = Number(app.config?.server?.history || 0);
    return `${current}/${limit}`;
});
const currentPageUrl = computed(() => {
    const currentRoom = ws.room || '';
    const query = {};
    if (currentRoom) {
        query.room = currentRoom;
    }
    const resolved = router.resolve({ path: '/', query });
    const url = new URL(window.location.pathname, window.location.origin);
    url.hash = resolved.href.startsWith('#') ? resolved.href : `#${resolved.href}`;
    return url.toString();
});
const latestContentUrl = computed(() => {
    const currentRoom = ws.room || '';
    const roomQuery = currentRoom ? `?room=${encodeURIComponent(currentRoom)}` : '';
    return buildAbsoluteRouteUrl(`content/latest${roomQuery}`);
});
const pageQrUrl = computed(() => pageQrMode.value === 'latest' ? latestContentUrl.value : currentPageUrl.value);
function buildAbsoluteRouteUrl(path) {
    const normalizedPath = String(path || '').replace(/^\/+/, '');
    const baseURL = axios.defaults.baseURL || '';
    if (baseURL) {
        return new URL(normalizedPath, `${baseURL.replace(/\/+$/, '')}/`).toString();
    }
    const prefix = app.config?.server?.prefix || '';
    return new URL(`${prefix}/${normalizedPath}`, `${window.location.origin}/`).toString();
}
function focusComposer(type) {
    nextTick(() => {
        if (composer.value && typeof composer.value.focus === 'function') {
            composer.value.focus(type);
        }
    });
}
</script>

<template>
    <v-container fluid class="home-minimal pa-3 pa-md-5" :class="{ 'home-minimal--dark': isDark }">
        <div class="home-minimal__shell mx-auto">
            <v-card class="composer-dock composer-dock--top px-3 px-md-4 py-2 mb-2" :class="{ 'surface-card--dark': isDark }" variant="outlined">
                <unified-composer ref="composer" @show-qr="pageQrDialogVisible = true"></unified-composer>
            </v-card>

            <v-card class="timeline-panel" :class="{ 'surface-card--dark': isDark }" variant="outlined">
                <div class="timeline-panel__body px-3 px-md-4 py-2">
                    <div v-if="app.received.length" class="timeline-panel__stream">
                        <v-fade-transition group>
                            <div
                                v-for="item in app.received"
                                :key="item.id"
                                class="timeline-panel__item"
                                :class="{ 'timeline-panel__item--first': item === app.received[0] }"
                            >
                                <v-chip
                                    v-if="item === app.received[0]"
                                    size="x-small"
                                    :variant="'outlined'"
                                    color="primary"
                                    class="timeline-panel__count-chip timeline-panel__count-chip--overlay"
                                >
                                    {{ historyUsageLabel }}
                                </v-chip>
                                <component
                                    :is="item.type === 'text' ? ReceivedText : ReceivedFile"
                                    :meta="item"
                                />
                            </div>
                        </v-fade-transition>
                    </div>

                    <v-sheet
                        v-if="!app.received.length"
                        class="empty-timeline py-10 px-6 text-center"
                        :class="{ 'empty-timeline--dark': isDark }"
                        rounded="lg"
                        color="transparent"
                    >
                        <v-icon size="42" color="primary">{{ mdiTimeline }}</v-icon>
                        <div class="text-h6 font-weight-medium mt-4 mb-2">{{ t('emptyTimelineTitle') }}</div>
                        <div class="text-body-2 text-medium-emphasis mb-4">{{ t('timelineEmptySubtitle') }}</div>
                        <v-btn size="small" variant="flat" color="primary" @click="focusComposer('text')">
                            {{ t('quickSend') }}
                        </v-btn>
                    </v-sheet>

                    <div v-else class="text-center text-caption text-medium-emphasis pt-2">{{ t('alreadyAtBottom') }}</div>
                </div>
            </v-card>
        </div>

        <v-dialog v-model="pageQrDialogVisible" max-width="250">
            <v-card>
                <v-card-title class="text-h5 justify-center">{{ t('scanToAccess') }}</v-card-title>
                <v-card-text class="text-center pa-4">
                    <v-btn-toggle v-model="pageQrMode" mandatory density="compact" class="mb-3">
                        <v-btn size="small" value="page">{{ t('currentShare') }}</v-btn>
                        <v-btn size="small" value="latest">{{ t('latestShare') }}</v-btn>
                    </v-btn-toggle>
                    <qrcode-vue :value="pageQrUrl" :size="200" level="H" />
                    <div class="text-caption mt-2" style="word-break: break-all;">{{ pageQrUrl }}</div>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn color="primary" variant="text" @click="pageQrDialogVisible = false">{{ t('close') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>
    </v-container>
</template>

<style scoped>
.home-minimal {
    background: transparent;
    min-height: calc(100vh - 64px);
}

.home-minimal--dark {
    color: rgba(226, 232, 240, 0.96);
}

.home-minimal__shell {
    max-width: 980px;
}

.surface-card--dark,
.timeline-panel,
.composer-dock {
    border-radius: 20px;
    border-color: rgba(148, 163, 184, 0.24) !important;
    box-shadow: 0 12px 30px rgba(15, 23, 42, 0.05);
    background: rgba(255, 255, 255, 0.92);
    transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.surface-card--dark {
    border-color: rgba(71, 85, 105, 0.72) !important;
    box-shadow: 0 18px 36px rgba(2, 6, 23, 0.36);
    background: rgba(15, 23, 42, 0.92);
}

.timeline-panel {
    overflow: hidden;
}

.timeline-panel__body {
    min-height: 24rem;
}

.timeline-panel__stream {
    position: relative;
}

.timeline-panel__item {
    position: relative;
}

.timeline-panel__item--first {
    padding-top: 10px;
}

.timeline-panel__count-chip {
    height: 22px;
    padding: 0 6px;
    border-radius: 999px;
}

.timeline-panel__count-chip--overlay {
    position: absolute;
    top: 0;
    left: 50%;
    transform: translateX(-50%);
    z-index: 1;
    backdrop-filter: blur(8px);
    background: rgba(255, 255, 255, 0.78);
}

.home-minimal--dark .timeline-panel__count-chip--overlay {
    background: rgba(15, 23, 42, 0.78);
}

.empty-timeline {
    border: 1px dashed rgba(148, 163, 184, 0.35);
    background: rgba(248, 250, 252, 0.68) !important;
}

.empty-timeline--dark {
    border-color: rgba(71, 85, 105, 0.72);
    background: rgba(15, 23, 42, 0.42) !important;
}

.composer-dock {
    border-radius: 18px;
}

.composer-dock--top {
    position: sticky;
    top: calc(64px + 0.5rem);
    z-index: 2;
    box-shadow: 0 10px 24px rgba(15, 23, 42, 0.05);
}

@media (max-width: 1263px) {
    .composer-dock--top {
        top: calc(56px + 0.5rem);
    }
}

@media (max-width: 960px) {
    .home-minimal {
        min-height: calc(100vh - 56px);
    }

    .timeline-panel__stream {
        padding-left: 0;
    }

    .timeline-panel__count-chip--overlay {
        left: 50%;
    }
}
</style>