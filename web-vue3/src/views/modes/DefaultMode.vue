<script setup>import { computed, nextTick, ref, watch } from 'vue';
import { isImageName } from '@/util.js';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import PageToolbar from '@/components/PageToolbar.vue';
import UnifiedComposer from '@/components/UnifiedComposer.vue';
import ReceivedText from '@/components/received-item/Text.vue';
import ReceivedFile from '@/components/received-item/File.vue';

const mdiTimeline = 'mdi-timeline';
const mdiTextBox = 'mdi-text-box-outline';
const mdiImage = 'mdi-image-outline';
const mdiFile = 'mdi-file-outline';
const tlLastRoom = ref('');
let tlJustSwitched = false;


const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();
const composer = ref(null);

// 时间流的分类过滤。选择存 localStorage —— 切模式/刷新后不该莫名回到「全部」。
// 默认 'all'：跟「显示开关默认全开」同一条原则，默认态不能改变既有行为。
const TIMELINE_FILTER_KEY = 'timelineFilter';
const TIMELINE_FILTER_KEYS = ['all', 'text', 'image', 'file'];
const storedFilter = localStorage.getItem(TIMELINE_FILTER_KEY);
const timelineFilter = ref(TIMELINE_FILTER_KEYS.includes(storedFilter) ? storedFilter : 'all');
function setTimelineFilter(key) {
    timelineFilter.value = key;
    localStorage.setItem(TIMELINE_FILTER_KEY, key);
}

const FILTER_OPTIONS = [
    { key: 'all', labelKey: 'filterAll', icon: mdiTimeline },
    { key: 'text', labelKey: 'filterText', icon: mdiTextBox },
    { key: 'image', labelKey: 'filterImage', icon: mdiImage },
    { key: 'file', labelKey: 'filterFile', icon: mdiFile },
];

// 「图片」= 文件条目里文件名是图片扩展名的那些；文本条目永远只归「文本」。
// 判型用 util 的 isImageName（全站唯一实现），别在这里再抄一份正则。
const filteredReceived = computed(() => {
    // 开关关掉时整个过滤不生效（不只是藏起分类条）——
    // 否则用户关掉开关后，列表还停在上次选的分类上，看着像内容丢了。
    // 列表来源是 visibleReceived（已套搜索），不是 received ——
    // 否则纯预览模式下搜索框只在别的模式生效。
    const list = app.visibleReceived;
    if (!app.display.timelineFilter) return list;
    if (timelineFilter.value === 'all') return list;
    if (timelineFilter.value === 'text') return list.filter((item) => item.type === 'text');
    const wantImage = timelineFilter.value === 'image';
    return list.filter((item) => item.type === 'file' && isImageName(item.name) === wantImage);
});
const historyUsageLabel = computed(() => {
    const current = app.received.length;
    const limit = Number(app.config?.server?.history || 0);
    return `${current}/${limit}`;
});
function focusComposer(type) {
    nextTick(() => {
        if (composer.value && typeof composer.value.focus === 'function') {
            composer.value.focus(type);
        }
    });
}

function tlNearTop() {
    return window.scrollY < 120;
}

function tlScrollTop(smooth = true) {
    window.scrollTo({ top: 0, behavior: smooth ? 'smooth' : 'auto' });
}

watch(() => app.received, () => {
    if (tlJustSwitched) {
        tlJustSwitched = false;
        nextTick(() => tlScrollTop(false));
        return;
    }
    if (!tlNearTop()) return;
    nextTick(() => tlScrollTop(true));
});

watch(() => ws.room, (room) => {
    if (tlLastRoom.value !== '' && tlLastRoom.value !== room) {
        tlJustSwitched = true;
    }
    tlLastRoom.value = room;
});

</script>

<template>
    <div class="home-minimal" :class="{ 'home-minimal--dark': isDark }">
        <PageToolbar variant="default"></PageToolbar>
        <v-container fluid class="home-minimal__body px-3 px-md-5 pb-3 pb-md-5">
            <div class="home-minimal__shell mx-auto">
            <v-card class="composer-dock composer-dock--top px-3 px-md-4 py-2 mb-2" :class="{ 'surface-card--dark': isDark }" variant="outlined">
                <unified-composer ref="composer"></unified-composer>
            </v-card>

            <v-card class="timeline-panel" :class="{ 'surface-card--dark': isDark }" variant="outlined">
                <div class="timeline-panel__body px-3 px-md-4 py-2">
                    <!-- 分类条：只在真有内容时出现（空列表上摆一条没用的过滤条更碍事） -->
                    <div v-if="app.received.length && app.display.timelineFilter" class="timeline-panel__filters">
                        <v-chip
                            v-for="opt in FILTER_OPTIONS"
                            :key="opt.key"
                            size="small"
                            label
                            class="timeline-panel__filter"
                            :variant="timelineFilter === opt.key ? 'flat' : 'outlined'"
                            :color="timelineFilter === opt.key ? 'primary' : undefined"
                            @click="setTimelineFilter(opt.key)"
                        >
                            <v-icon start size="16">{{ opt.icon }}</v-icon>{{ t(opt.labelKey) }}
                        </v-chip>
                    </div>

                    <div v-if="app.received.length" class="timeline-panel__stream">
                        <div
                            v-for="item in filteredReceived"
                            :key="item.id"
                            class="timeline-panel__item"
                            :class="{ 'timeline-panel__item--first': item === filteredReceived[0] }"
                        >
                            <v-chip
                                v-if="item === filteredReceived[0]"
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
                        <div
                            v-if="!filteredReceived.length"
                            class="text-center text-caption text-medium-emphasis py-6"
                        >{{ t('filterEmpty') }}</div>
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

                    <div v-else-if="filteredReceived.length" class="text-center text-caption text-medium-emphasis pt-2">{{ t('alreadyAtBottom') }}</div>
                </div>
            </v-card>
        </div>

        </v-container>
    </div>
</template>

<style scoped>
.home-minimal {
    background: transparent;
    min-height: 100vh;
    min-height: 100dvh;
}

.home-minimal__body {
    padding-top: 8px;
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

/* 分类条（全部 / 文本 / 图片 / 文件）。默认关，见 data/displayToggles.js。 */
.timeline-panel__filters {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 6px;
    padding: 2px 0 10px;
}

.timeline-panel__filter {
    cursor: pointer;
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
    top: 0.5rem;
    z-index: 2;
    box-shadow: 0 10px 24px rgba(15, 23, 42, 0.05);
}

@media (max-width: 1263px) {
    .composer-dock--top {
        top: 0.5rem;
    }
}

@media (max-width: 960px) {
    .home-minimal {
        min-height: calc(100vh - 56px);
        min-height: calc(100dvh - 56px);
    }

    .timeline-panel__stream {
        padding-left: 0;
    }

    .timeline-panel__count-chip--overlay {
        left: 50%;
    }
}
</style>