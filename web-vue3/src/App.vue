<script setup>
import { computed, onBeforeUnmount, onMounted, provide, ref, watch } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useDisplay } from 'vuetify';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { toast, toastState } from '@/plugins/toast';
import TraditionalColorDialog from '@/components/TraditionalColorDialog.vue';
import RoomList from '@/components/RoomList.vue';
import QrcodeVue from 'qrcode.vue';
import { errorMessage } from '@/util.js';
import { MODES } from '@/views/modes/registry.js';
import { DISPLAY_GROUPS, togglesForMode } from '@/data/displayToggles.js';

const mdiBrightness4 = 'mdi-brightness-4';
const mdiChevronLeft = 'mdi-chevron-left';
const mdiChevronRight = 'mdi-chevron-right';
const mdiClose = 'mdi-close';
const mdiContentPaste = 'mdi-content-paste';
const mdiDiceMultiple = 'mdi-dice-multiple';
const mdiGithub = 'mdi-github';
const mdiHeartOutline = 'mdi-heart-outline';
const mdiOpenInNew = 'mdi-open-in-new';
const mdiPalette = 'mdi-palette';
const mdiPaletteSwatch = 'mdi-palette-swatch';
const mdiCurrencyCny = 'mdi-currency-cny';
const mdiCoffee = 'mdi-coffee';
const mdiTranslate = 'mdi-translate';
const mdiViewList = 'mdi-view-list';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const display = useDisplay();
const { t, locale } = useI18n();
const router = useRouter();
const route = useRoute();

const colorDialog = ref(false);
const pickColorDialog = ref(false);
const settingsDialog = ref(false);
const pageQrDialogVisible = ref(false);
// 设置面板的页签：通用 / 个性化。个性化里是「每个界面模式一组显示开关」，
// 开关会长到几十项，所以必须单独占一页，不能平铺在通用页里。
const settingsTab = ref('general');

// 个性化面板只显示「当前模式真的能用」的开关（见 displayToggles.js 的 modes 声明）
const togglesInGroup = (groupKey) => togglesForMode(app.uiMode, groupKey);
const pageQrMode = ref('page');
const currentPrimary = computed(() => isDark.value ? theme.themes.value.dark.colors.primary : theme.themes.value.light.colors.primary);
const clearAllDialog = ref(false);
const clipboardClearedMessageVisible = ref(false);
const roomSheet = ref(false);
const donateDialog = ref(false);
const roomSearch = ref('');
const roomDockVisible = ref(true);
const roomDockSide = ref('right');
const availableRooms = ref([]);
const roomsLoading = ref(false);

provide('stickyAppActions', {
    openSettings: () => { settingsDialog.value = true; },
    openClearAll: () => { clearAllDialog.value = true; },
    openRoomDialog: () => { ws.roomInput = ws.room; ws.roomDialog = true; },
    toggleConnection: () => {
        if (!ws.websocket && !ws.websocketConnecting) {
            ws.retry = 0;
            ws.connect();
        }
    },
});

provide('pageToolbarActions', {
    openSettings: () => { settingsDialog.value = true; },
    openClearAll: () => { clearAllDialog.value = true; },
    openRoomDialog: () => { ws.roomInput = ws.room; ws.roomDialog = true; },
    toggleConnection: () => {
        if (!ws.websocket && !ws.websocketConnecting) {
            ws.retry = 0;
            ws.connect();
        }
    },
    openRoomBrowser: () => { openRoomBrowser(); },
    openPageQr: () => { pageQrDialogVisible.value = true; },
    goHome: () => { goHome(); },
    roomCount: computed(() => availableRooms.value.length),
    roomListEnabled: computed(() => Boolean(app.config?.server?.roomList)),
});

const languageOptions = [
    { code: 'zh', name: '简体中文' },
    { code: 'zh-TW', name: '繁體中文' },
    { code: 'en', name: 'English' },
    { code: 'ja', name: '日本語' },
];
const currentLocaleCode = computed(() => {
    const match = languageOptions.find(option => option.code === locale.value);
    return match ? match.code : 'zh';
});
const isDesktopRoomDockEnabled = computed(() => {
    return display.width.value > 1263 && app.config && app.config.server && app.config.server.roomList;
});
const latencyValue = computed(() => {
    if (ws.latency === null) {
        return '';
    }
    return `${Math.round(ws.latency)} ms`;
});
const latencyColor = computed(() => {
    if (ws.latency === null) {
        return 'grey';
    }
    if (ws.latency < 60) {
        return 'success';
    }
    if (ws.latency < 120) {
        return 'warning';
    }
    return 'error';
});
const latencyHexColor = computed(() => {
    if (ws.latency === null) {
        return '';
    }
    const colorName = latencyColor.value;
    const themeColors = theme.themes.value[isDark.value ? 'dark' : 'light'].colors;
    return themeColors[colorName] || colorName;
});
const isDesktopRoomDockVisible = computed(() => isDesktopRoomDockEnabled.value && roomDockVisible.value);
const filteredRooms = computed(() => {
    let rooms = availableRooms.value.slice();
    if (roomSearch.value) {
        rooms = rooms.filter(room =>
            (room.name || t('publicRoom')).toLowerCase().includes(roomSearch.value.toLowerCase())
        );
    }
    return rooms.sort((a, b) => {
        if (a.isFavorite !== b.isFavorite) {
            return b.isFavorite - a.isFavorite;
        }
        return 0;
    });
});
const currentRoomEntry = computed(() => {
    const currentRoomName = ws.room || '';
    const matching = filteredRooms.value.find(room => room.name === currentRoomName);
    if (matching) {
        return matching;
    }
    if (roomSearch.value) {
        return null;
    }
    return createOptimisticRoom(currentRoomName);
});
const favoriteRooms = computed(() => {
    const currentRoomName = ws.room || '';
    return filteredRooms.value.filter(room => room.isFavorite && room.name !== currentRoomName);
});
const activeRooms = computed(() => {
    const currentRoomName = ws.room || '';
    return filteredRooms.value.filter(room => !room.isFavorite && room.isActive && room.name !== currentRoomName);
});
const otherRooms = computed(() => {
    const currentRoomName = ws.room || '';
    return filteredRooms.value.filter(room => !room.isFavorite && !room.isActive && room.name !== currentRoomName);
});
const favoriteRoomCount = computed(() => availableRooms.value.filter(room => room.isFavorite).length);
const activeRoomCount = computed(() => availableRooms.value.filter(room => room.isActive).length);
const roomGroups = computed(() => [
    {
        key: 'favorites',
        title: t('favoriteRoomsLabel'),
        rooms: favoriteRooms.value,
    },
    {
        key: 'active',
        title: t('activeRoomsLabel'),
        rooms: activeRooms.value,
    },
    {
        key: 'other',
        title: t('otherRoomsLabel'),
        rooms: otherRooms.value,
    },
].filter(group => group.rooms.length > 0));

const darkModeOptions = [
    { value: 'time', title: t('switchByTime'), desc: t('switchByTimeDesc') },
    { value: 'prefer', title: t('switchBySystem'), desc: t('switchBySystemDesc') },
    { value: 'enable', title: t('keepEnabled'), desc: '' },
    { value: 'disable', title: t('keepDisabled'), desc: '' },
];

function submitRoomChange() {
    const roomName = ws.roomInput || '';
    ensureRoomPresent(roomName);
    ws.roomDialog = false;
    ws.navigateToRoom(roomName);
}
function createOptimisticRoom(roomName = ws.room || '') {
    if (roomName === undefined || roomName === null) {
        return null;
    }
    const normalizedRoomName = roomName || '';
    return {
        name: normalizedRoomName,
        isFavorite: getFavoriteRooms().includes(normalizedRoomName),
        isProtected: Boolean(ws.roomProtectionCache?.[normalizedRoomName]),
        isActive: true,
        messageCount: 0,
        deviceCount: 0,
        lastActive: Math.floor(Date.now() / 1000),
    };
}
function ensureRoomPresent(roomName = ws.room || '') {
    const normalizedRoomName = roomName || '';
    if (availableRooms.value.some(room => room.name === normalizedRoomName)) {
        return;
    }
    availableRooms.value.unshift(createOptimisticRoom(normalizedRoomName));
}
function syncAvailableRooms(rooms) {
    const nextRooms = Array.isArray(rooms) ? rooms : [];
    const existingByName = new Map(availableRooms.value.map(room => [room.name, room]));
    const orderedRooms = nextRooms.map(roomData => {
        const existing = existingByName.get(roomData.name);
        if (existing) {
            Object.assign(existing, roomData);
            return existing;
        }
        return roomData;
    });
    const currentRoomName = ws.room || '';
    if (!orderedRooms.some(room => room.name === currentRoomName)) {
        orderedRooms.unshift(existingByName.get(currentRoomName) || createOptimisticRoom(currentRoomName));
    }
    availableRooms.value.splice(0, availableRooms.value.length, ...orderedRooms);
}
function openRoomBrowser() {
    if (isDesktopRoomDockEnabled.value) {
        roomDockVisible.value = true;
        persistRoomBrowserPreferences();
        ensureRoomPresent();
        fetchRoomList();
        return;
    }
    roomSheet.value = true;
    ensureRoomPresent();
    fetchRoomList();
}
function hideDesktopRoomDock() {
    roomDockVisible.value = false;
    persistRoomBrowserPreferences();
}
function toggleRoomDockSide() {
    roomDockSide.value = roomDockSide.value === 'right' ? 'left' : 'right';
    persistRoomBrowserPreferences();
}
function persistRoomBrowserPreferences() {
    localStorage.setItem('roomDockVisible', String(roomDockVisible.value));
    localStorage.setItem('roomDockSide', roomDockSide.value);
}
function restoreRoomBrowserPreferences() {
    const storedVisible = localStorage.getItem('roomDockVisible');
    const storedSide = localStorage.getItem('roomDockSide');
    roomDockVisible.value = storedVisible === null ? true : storedVisible === 'true';
    roomDockSide.value = storedSide === 'left' ? 'left' : 'right';
}
async function clearAll() {
    try {
        await axios.delete('revoke/all', {
            params: { room: ws.room },
        });
    } catch (error) {
        console.log(error);
        clipboardClearedMessageVisible.value = false;
        const errMsg = errorMessage(error);
        if (errMsg) {
            toast(t('clearClipboardFailedMsg', { msg: errMsg }));
        } else {
            toast(t('clearClipboardFailed'));
        }
    }
}
function changeLocale(localeValue) {
    if (locale.value !== localeValue) {
        locale.value = localeValue;
        localStorage.setItem('locale', localeValue);
    }
}
function goHome() {
    if (route.path !== '/' || Object.keys(route.query).length > 0) {
        router.push('/');
    }
}
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
function copyQrUrl() {
    const url = pageQrUrl.value;
    if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(url)
            .then(() => toast(t('copySuccess')))
            .catch(() => toast(t('copyFailedGeneral')));
    } else {
        try {
            const textArea = document.createElement("textarea");
            textArea.value = url;
            textArea.style.position = "absolute";
            textArea.style.left = "-9999px";
            document.body.appendChild(textArea);
            textArea.select();
            document.execCommand('copy');
            document.body.removeChild(textArea);
            toast(t('copySuccess'));
        } catch {
            toast(t('copyFailedGeneral'));
        }
    }
}
async function fetchRooms() {
    const candidateTokens = typeof ws.getKnownAuthTokens === 'function' ? ws.getKnownAuthTokens() : [];
    const dedupedTokens = Array.from(new Set(candidateTokens.map(token => (token || '').trim()).filter(Boolean)));
    const response = await axios.get('rooms', {
        headers: dedupedTokens.length ? { 'X-Room-Auth-Tokens': JSON.stringify(dedupedTokens) } : undefined,
        __skipRoomAuthHandling: true,
    });
    return Array.isArray(response.data?.rooms) ? response.data.rooms : [];
}
async function fetchRoomList() {
    if (!app.config || !app.config.server || !app.config.server.roomList) {
        return;
    }
    if (roomsLoading.value) {
        return;
    }
    roomsLoading.value = true;
    try {
        const rooms = await fetchRooms();
        const favoriteRooms = getFavoriteRooms();
        syncAvailableRooms(rooms.map(room => ({ ...room, isFavorite: favoriteRooms.includes(room.name) })));
        ensureRoomPresent();
    } catch (error) {
        console.error('Failed to fetch room list:', error);
        toast(t('failedToLoadRooms'));
    } finally {
        roomsLoading.value = false;
    }
}
async function switchRoom(roomName) {
    roomSheet.value = false;
    await ws.navigateToRoom(roomName);
}
function getFavoriteRooms() {
    try {
        return JSON.parse(localStorage.getItem('favoriteRooms') || '[]');
    } catch {
        return [];
    }
}
function toggleFavoriteRoom(roomName) {
    const favorites = getFavoriteRooms();
    const index = favorites.indexOf(roomName);
    if (index > -1) {
        favorites.splice(index, 1);
        toast(t('removedFromFavorites', { room: roomName || t('publicRoom') }));
    } else {
        favorites.push(roomName);
        toast(t('addedToFavorites', { room: roomName || t('publicRoom') }));
    }
    localStorage.setItem('favoriteRooms', JSON.stringify(favorites));
    const room = availableRooms.value.find(r => r.name === roomName);
    if (room) {
        room.isFavorite = !room.isFavorite;
    }
}
function getRoomDisplayName(room) {
    return room && room.name ? room.name : t('publicRoom');
}
function randomRoomName() {
    const names = ['reimu', 'marisa', 'rumia', 'cirno', 'meiling', 'patchouli', 'sakuya', 'remilia', 'flandre', 'letty', 'chen', 'lyrica', 'lunasa', 'merlin', 'youmu', 'yuyuko', 'ran', 'yukari', 'suika', 'mystia', 'keine', 'tewi', 'reisen', 'eirin', 'kaguya', 'mokou'];
    return names[Math.floor(Math.random() * names.length)] + '-' + Math.random().toString(16).substring(2, 6);
}

onMounted(() => {
    restoreRoomBrowserPreferences();

    const darkPrimary = localStorage.getItem('darkPrimary');
    const lightPrimary = localStorage.getItem('lightPrimary');
    if (darkPrimary) {
        theme.themes.value.dark.colors.primary = darkPrimary;
    }
    if (lightPrimary) {
        theme.themes.value.light.colors.primary = lightPrimary;
    }
});

watch(() => theme.themes.value.dark.colors.primary, (newVal) => {
    localStorage.setItem('darkPrimary', newVal);
});
watch(() => theme.themes.value.light.colors.primary, (newVal) => {
    localStorage.setItem('lightPrimary', newVal);
});
const useDark = computed(() => app.useDark);
watch(useDark, (value) => {
    theme.change(value ? 'dark' : 'light');
});
watch(() => app.dark, (newVal) => {
    localStorage.setItem('darkmode', newVal);
    applyDarkMode();
    setupDarkModeTimers();
});
let darkModeTimer = null;
let darkMediaQuery = null;
function applyDarkMode() {
    theme.change(app.useDark ? 'dark' : 'light');
}
function setupDarkModeTimers() {
    if (darkModeTimer) {
        clearInterval(darkModeTimer);
        darkModeTimer = null;
    }
    if (darkMediaQuery) {
        darkMediaQuery.removeEventListener('change', applyDarkMode);
        darkMediaQuery = null;
    }
    if (app.dark === 'time') {
        darkModeTimer = setInterval(applyDarkMode, 1000);
    } else if (app.dark === 'prefer') {
        darkMediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
        darkMediaQuery.addEventListener('change', applyDarkMode);
    }
}
onMounted(() => {
    applyDarkMode();
    setupDarkModeTimers();
});
onBeforeUnmount(() => {
    if (darkModeTimer) {
        clearInterval(darkModeTimer);
    }
    if (darkMediaQuery) {
        darkMediaQuery.removeEventListener('change', applyDarkMode);
    }
});
watch(() => Boolean(ws.websocket), (connected) => {
    if (connected && app.config && app.config.server && app.config.server.roomList) {
        fetchRoomList();
    }
});
watch(isDesktopRoomDockVisible, (newVal) => {
    if (newVal) {
        ensureRoomPresent();
        fetchRoomList();
    }
}, { immediate: true });
watch(() => route.fullPath, () => {
    clipboardClearedMessageVisible.value = false;
    const routeRoom = ws.normalizeRoomName(route.query.room || '');
    if (ws.normalizeRoomName(ws.room) !== routeRoom) {
        ws.switchRoom(routeRoom);
    }
    if (app.config && app.config.server && app.config.server.roomList) {
        ensureRoomPresent(routeRoom);
    }
});
</script>

<template>
    <v-app class="app-shell" :class="{ 'app-shell--dark': isDark }">
        <v-alert
            v-model="clipboardClearedMessageVisible"
            type="error"
            dismissible
            dense
            class="ma-0 text-center"
            style="position: sticky; top: 0; z-index: 5;"
        >
            {{ t('clipboardClearedRefresh') }}
        </v-alert>

        <v-main class="app-shell__main">
            <div
                class="app-shell__workspace"
                :class="{
                    'app-shell__workspace--dock-right': isDesktopRoomDockEnabled && roomDockSide === 'right',
                    'app-shell__workspace--dock-left': isDesktopRoomDockEnabled && roomDockSide === 'left'
                }"
            >
                <div class="app-shell__content">
                    <router-view v-slot="{ Component }">
                        <keep-alive v-if="route.meta.keepAlive"><component :is="Component" /></keep-alive>
                        <component v-else :is="Component" />
                    </router-view>
                </div>

                <aside
                    v-if="isDesktopRoomDockVisible"
                    class="room-browser room-browser--dock"
                    :class="[
                        { 'room-browser--dark': isDark },
                        roomDockSide === 'left' ? 'room-browser--dock-left' : 'room-browser--dock-right'
                    ]"
                >
                    <RoomList
                        v-model:search="roomSearch"
                        :groups="roomGroups"
                        :current-room="currentRoomEntry"
                        :current-room-name="getRoomDisplayName({ name: ws.room })"
                        :current-room-label="t('currentRoomLabel')"
                        :title="t('roomList')"
                        :count="availableRooms.length"
                        :favorite-count="favoriteRoomCount"
                        :active-count="activeRoomCount"
                        :loading="roomsLoading"
                        :has-rooms="filteredRooms.length > 0"
                        @select="switchRoom"
                        @favorite="toggleFavoriteRoom"
                        variant="dock"
                    >
                        <template #actions>
                            <v-tooltip left>
                                <template v-slot:activator="{ props }">
                                    <v-btn icon density="comfortable" variant="text" v-bind="props" @click="toggleRoomDockSide()">
                                        <v-icon>{{ roomDockSide === 'right' ? mdiChevronLeft : mdiChevronRight }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ roomDockSide === 'right' ? t('dockLeft') : t('dockRight') }}</span>
                            </v-tooltip>
                            <v-tooltip bottom>
                                <template v-slot:activator="{ props }">
                                    <v-btn icon density="comfortable" variant="text" size="small" v-bind="props" @click="hideDesktopRoomDock()">
                                        <v-icon>{{ mdiClose }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ t('hideRoomBrowser') }}</span>
                            </v-tooltip>
                        </template>
                    </RoomList>
                </aside>
            </div>
        </v-main>

        <v-dialog v-model="settingsDialog" max-width="480">
            <v-card class="cc-settings">
                <v-card-title class="cc-settings__header">
                    <div class="flex-grow-1 cc-settings__header-wrap">
                        <div class="cc-settings__title">{{ t('settings') }}</div>
                        <div class="cc-settings__version">
                            <span class="mr-1">{{ t('cloudClipboard') }}</span>
                            <span v-if="app.config && app.config.version">{{ app.config.version }}</span>
                        </div>
                    </div>
                    <v-btn icon variant="text" @click="settingsDialog = false">
                        <v-icon>{{ mdiClose }}</v-icon>
                    </v-btn>
                </v-card-title>
                <v-divider></v-divider>
                <v-tabs v-model="settingsTab" density="comfortable" color="primary" class="cc-settings__tabs">
                    <v-tab value="general">{{ t('settingsGeneral') }}</v-tab>
                    <v-tab value="personalization">{{ t('personalization') }}</v-tab>
                </v-tabs>
                <v-tabs-window v-model="settingsTab">
                    <v-tabs-window-item value="general">
                        <v-card-text class="cc-settings__body" style="max-height: 62vh; overflow-y: auto;">
                            <div class="cc-settings__group">
                            <v-list-subheader class="cc-settings__subheader">{{ t('appearance') }}</v-list-subheader>
                            <v-list class="cc-settings__list" density="comfortable">
                                <v-list-item class="cc-settings__item">
                                    <template v-slot:prepend>
                                        <v-icon color="primary">{{ mdiBrightness4 }}</v-icon>
                                    </template>
                                    <v-list-item-title>{{ t('darkMode') }}</v-list-item-title>
                                    <template v-slot:append>
                                        <v-select
                                            :model-value="app.dark"
                                            :items="darkModeOptions"
                                            item-title="title"
                                            item-value="value"
                                            hide-details
                                            density="compact"
                                            class="cc-settings-select"
                                            @update:model-value="v => app.dark = v"
                                        ></v-select>
                                    </template>
                                </v-list-item>
                                <v-list-item class="cc-settings__item">
                                    <template v-slot:prepend>
                                        <v-icon color="primary">{{ mdiPalette }}</v-icon>
                                    </template>
                                    <v-list-item-title>{{ t('changeThemeColor') }}</v-list-item-title>
                                    <template v-slot:append>
                                        <div class="cc-settings__theme-actions">
                                            <v-btn
                                                variant="tonal"
                                                size="small"
                                                class="cc-settings__theme-btn"
                                                @click="pickColorDialog = true"
                                            >
                                                <span class="cc-settings__swatch" :style="{ background: currentPrimary }"></span>
                                                {{ t('colorPicker') }}
                                            </v-btn>
                                            <v-btn
                                                variant="tonal"
                                                size="small"
                                                class="cc-settings__theme-btn"
                                                @click="colorDialog = true"
                                            >
                                                <v-icon start size="16">{{ mdiPaletteSwatch }}</v-icon>
                                                {{ t('traditionalColors') }}
                                            </v-btn>
                                        </div>
                                    </template>
                                </v-list-item>
                            </v-list>
                        </div>
                            <div class="cc-settings__group">
                            <v-list-subheader class="cc-settings__subheader">{{ t('language') }}</v-list-subheader>
                            <v-list class="cc-settings__list" density="comfortable">
                                <v-list-item class="cc-settings__item">
                                    <template v-slot:prepend>
                                        <v-icon color="primary">{{ mdiTranslate }}</v-icon>
                                    </template>
                                    <v-list-item-title>{{ t('language') }}</v-list-item-title>
                                    <template v-slot:append>
                                        <v-select
                                            :model-value="currentLocaleCode"
                                            :items="languageOptions"
                                            item-title="name"
                                            item-value="code"
                                            hide-details
                                            density="compact"
                                            class="cc-settings-select"
                                            @update:model-value="changeLocale"
                                        ></v-select>
                                    </template>
                                </v-list-item>
                            </v-list>
                        </div>
                            <div class="cc-settings__group">
                            <v-list-subheader class="cc-settings__subheader">{{ t('about') }}</v-list-subheader>
                            <v-list class="cc-settings__list" density="comfortable">
                                <v-list-item class="cc-settings__item">
                                    <template v-slot:prepend>
                                        <v-icon color="primary">{{ mdiGithub }}</v-icon>
                                    </template>
                                    <v-list-item-title>
                                        <a href="https://github.com/Jonnyan404/cloud-clipboard-go" target="_blank" rel="noopener" class="cc-settings__link">
                                            {{ t('github') }}
                                            <v-icon size="16" class="cc-settings__external">{{ mdiOpenInNew }}</v-icon>
                                        </a>
                                    </v-list-item-title>
                                </v-list-item>
                            </v-list>
                            <v-divider class="my-2"></v-divider>
                            <v-btn block color="error" variant="outlined" size="small"
                                   class="cc-settings__donate-btn"
                                   @click="donateDialog = true">
                                <template v-slot:prepend>
                                    <v-icon>{{ mdiHeartOutline }}</v-icon>
                                </template>
                                {{ t('donatePrompt') }}
                                <template v-slot:append>
                                    <v-icon size="16">{{ mdiChevronRight }}</v-icon>
                                </template>
                            </v-btn>
                        </div>
                        </v-card-text>
                    </v-tabs-window-item>
                    <v-tabs-window-item value="personalization">
                        <v-card-text class="cc-settings__body" style="max-height: 62vh; overflow-y: auto;">
                            <div class="text-caption text-medium-emphasis mb-3">{{ t('personalizationHint') }}</div>
                                                    <v-btn-toggle
                                                        :model-value="app.uiMode"
                                                        @update:model-value="app.setUiMode"
                                                        mandatory
                                                        density="comfortable"
                                                        class="flex-wrap mb-2"
                                                        style="height: auto;"
                                                    >
                                                        <v-btn v-for="mode in MODES" :key="mode.key" :value="mode.key" size="small" class="text-none">
                                                            <v-icon start size="18">{{ mode.icon }}</v-icon>{{ t(mode.labelKey) }}
                                                        </v-btn>
                                                    </v-btn-toggle>
                                                    <template v-for="group in DISPLAY_GROUPS" :key="group.key">
                                                        <template v-if="togglesInGroup(group.key).length">
                                                            <v-list-subheader class="cc-settings__subheader">{{ t(group.labelKey) }}</v-list-subheader>
                                                            <v-list class="cc-settings__list cc-settings__toggles" density="comfortable">
                                                                <v-list-item v-for="toggle in togglesInGroup(group.key)" :key="toggle.key" class="cc-settings__item">
                                                                    <template v-slot:prepend>
                                                                        <v-icon color="primary">{{ toggle.icon }}</v-icon>
                                                                    </template>
                                                                    <v-list-item-title>{{ t(toggle.labelKey) }}</v-list-item-title>
                                                                    <template v-slot:append>
                                                                        <v-switch
                                                                            :model-value="app.display[toggle.key]"
                                                                            @update:model-value="value => app.setDisplayToggle(toggle.key, value)"
                                                                            color="primary"
                                                                            hide-details
                                                                            inset
                                                                        ></v-switch>
                                                                    </template>
                                                                </v-list-item>
                                                            </v-list>
                                                        </template>
                                                    </template>
                        </v-card-text>
                    </v-tabs-window-item>
                </v-tabs-window>
            </v-card>
        </v-dialog>

        <v-dialog v-model="donateDialog" max-width="420">
            <v-card>
                <v-card-title class="text-h6 d-flex align-center">
                    <v-icon color="error" class="mr-2">{{ mdiHeartOutline }}</v-icon>
                    {{ t('donatePrompt') }}
                </v-card-title>
                <v-divider></v-divider>
                <v-card-text class="text-center pa-4">
                    <div class="text-body-2 font-weight-medium cc-settings__link-title mb-2">{{ t('supportSectionTitle') }}</div>
                    <v-row class="cc-settings__reward-row" dense>
                        <v-col class="text-center">
                            <div class="cc-settings__reward-label">微信</div>
                            <img src="/reward-wechat.png" alt="WeChat Reward QR" class="cc-settings__reward-qr" />
                        </v-col>
                        <v-col class="text-center">
                            <div class="cc-settings__reward-label">支付宝</div>
                            <img src="/reward-alipay.png" alt="Alipay Reward QR" class="cc-settings__reward-qr" />
                        </v-col>
                    </v-row>
                    <div class="text-body-2 text-medium-emphasis mt-3 cc-settings__warm-text">{{ t('rewardHint') }}</div>
                    <v-btn class="mt-3" color="#ff5f5f" variant="tonal" block
                           href="https://ko-fi.com/jonnyan404"
                           target="_blank" rel="noopener">
                        <v-icon start>{{ mdiCoffee }}</v-icon>
                        <span>Buy Me a Coffee</span>
                        <v-icon end size="16">{{ mdiOpenInNew }}</v-icon>
                    </v-btn>
                    <v-divider class="my-4"></v-divider>
                    <div class="cc-settings__warm-box">
                        <div class="text-body-2 text-medium-emphasis cc-settings__warm-text">{{ t('cloudPromoHint') }}</div>
                        <div class="d-flex flex-column ga-2 mt-3">
                        <v-btn variant="outlined" color="primary"
                               href="https://cloud.tencent.com/act/cps/redirect?redirect=6150&cps_key=0b1dfaf9bb573dac05abef76202dc8cc&from=console"
                               target="_blank" rel="noopener" block>
                            <v-icon start>{{ mdiCurrencyCny }}</v-icon>
                            腾讯云 2C2G ¥99/年
                            <v-icon end size="16">{{ mdiOpenInNew }}</v-icon>
                        </v-btn>
                        <v-btn variant="outlined" color="primary"
                               href="https://www.aliyun.com/daily-act/ecs/activity_selection?userCode=79h2wrag"
                               target="_blank" rel="noopener" block>
                            <v-icon start>{{ mdiCurrencyCny }}</v-icon>
                            阿里云 2C2G ¥99/年
                            <v-icon end size="16">{{ mdiOpenInNew }}</v-icon>
                        </v-btn>
                        </div>
                    </div>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn color="primary" variant="text" @click="donateDialog = false">{{ t('close') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>

        <traditional-color-dialog v-model="colorDialog"></traditional-color-dialog>

        <v-dialog v-model="pickColorDialog" max-width="340">
            <v-card>
                <v-card-title>{{ t('selectThemeColor') }}</v-card-title>
                <v-card-text class="cc-picker-dialog__body">
                    <v-color-picker v-if="isDark" v-model="theme.themes.value.dark.colors.primary" show-swatches hide-inputs></v-color-picker>
                    <v-color-picker v-else v-model="theme.themes.value.light.colors.primary" show-swatches hide-inputs></v-color-picker>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn color="primary" variant="text" @click="pickColorDialog = false">{{ t('ok') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>

        <v-dialog v-model="ws.authCodeDialog" persistent max-width="360">
            <v-card>
                <v-card-title class="text-h5">{{ t('authRequired') }}</v-card-title>
                <v-card-text>
                    <p>{{ t('authPrompt') }}</p>
                    <p class="text-caption text-medium-emphasis mb-3">
                        {{ t('room') }}: {{ getRoomDisplayName({ name: ws.authPendingRoom || ws.room }) }}
                    </p>
                    <v-text-field
                        v-model="ws.inputPassword"
                        :label="t('password')"
                        variant="outlined"
                        bg-color="transparent"
                        class="cc-dialog-field"
                        :loading="ws.authDialogLoading"
                        :disabled="ws.authDialogLoading"
                        :error-messages="ws.authCodeError ? [ws.authCodeError] : []"
                        hide-details="auto"
                        @update:model-value="ws.authCodeError = ''"
                        @keyup.enter="ws.submitAuthCodeForPendingRoom()"
                        autofocus
                    ></v-text-field>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn
                        color="primary-darken-1"
                        variant="text"
                        :loading="ws.authDialogLoading"
                        @click="ws.submitAuthCodeForPendingRoom()"
                    >{{ t('submit') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>

        <v-dialog v-model="ws.roomDialog" persistent max-width="360">
            <v-card>
                <v-card-title class="text-h5">{{ t('clipboardRoom') }}</v-card-title>
                <v-card-text>
                    <p>{{ t('roomPrompt1') }}</p>
                    <p>{{ t('roomPrompt2') }}</p>
                    <v-text-field
                        v-model="ws.roomInput"
                        :label="t('roomName')"
                        variant="outlined"
                        bg-color="transparent"
                        class="cc-dialog-field"
                        :append-icon="mdiDiceMultiple"
                        @click:append="ws.roomInput = randomRoomName()"
                        @keyup.enter="submitRoomChange()"
                        autofocus
                    ></v-text-field>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn
                        color="primary-darken-1"
                        variant="text"
                        @click="ws.roomDialog = false"
                    >{{ t('cancel') }}</v-btn>
                    <v-btn
                        color="primary-darken-1"
                        variant="text"
                        @click="submitRoomChange()"
                    >{{ t('enterRoom') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>

        <v-dialog v-model="clearAllDialog" max-width="360">
            <v-card>
                <v-card-title class="text-h5">{{ t('clearClipboardConfirmTitle') }}</v-card-title>
                <v-card-text>
                    <p>{{ t('clearClipboardConfirmText') }}</p>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn
                        color="primary-darken-1"
                        variant="text"
                        @click="clearAllDialog = false"
                    >{{ t('cancel') }}</v-btn>
                    <v-btn
                        color="primary-darken-1"
                        variant="text"
                        @click="clearAllDialog = false; clearAll(); clipboardClearedMessageVisible = true;"
                    >{{ t('ok') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>

        <v-dialog v-model="pageQrDialogVisible" max-width="250">
            <v-card>
                <v-card-title class="text-h5 d-flex align-center">
                    {{ t('scanToAccess') }}
                    <v-spacer></v-spacer>
                    <v-btn icon variant="text" @click="pageQrDialogVisible = false">
                        <v-icon>{{ mdiClose }}</v-icon>
                    </v-btn>
                </v-card-title>
                <v-card-text class="text-center pa-4 pt-0">
                    <v-btn-toggle v-model="pageQrMode" mandatory density="compact" class="mb-3">
                        <v-btn size="small" value="page">{{ t('currentShare') }}</v-btn>
                        <v-btn size="small" value="latest">{{ t('latestShare') }}</v-btn>
                    </v-btn-toggle>
                    <div>
                        <qrcode-vue :value="pageQrUrl" :size="200" level="H" />
                    </div>
                    <div
                        class="text-caption mt-2 d-flex align-center justify-center"
                        style="word-break: break-all; cursor: pointer;"
                        :title="t('copyLink')"
                        @click="copyQrUrl"
                    >
                        <span class="flex-grow-1">{{ pageQrUrl }}</span>
                        <v-icon size="small" class="ml-1">{{ mdiContentPaste }}</v-icon>
                    </div>
                </v-card-text>
            </v-card>
        </v-dialog>

        <v-bottom-sheet v-model="roomSheet" scrollable max-width="820">
            <v-card class="room-browser" :class="{ 'room-browser--dark': isDark }">
                <RoomList
                    v-model:search="roomSearch"
                    :groups="roomGroups"
                    :current-room="currentRoomEntry"
                    :current-room-name="getRoomDisplayName({ name: ws.room })"
                    :current-room-label="t('currentRoomLabel')"
                    :title="t('roomList')"
                    :count="availableRooms.length"
                    :favorite-count="favoriteRoomCount"
                    :active-count="activeRoomCount"
                    :loading="roomsLoading"
                    :has-rooms="filteredRooms.length > 0"
                    @select="switchRoom"
                    @favorite="toggleFavoriteRoom"
                    variant="sheet"
                >
                    <template #actions>
                        <v-btn icon density="comfortable" variant="text" @click="roomSheet = false">
                            <v-icon>{{ mdiClose }}</v-icon>
                        </v-btn>
                    </template>
                </RoomList>
            </v-card>
        </v-bottom-sheet>

        <v-snackbar
            v-model="toastState.visible"
            :color="toastState.color"
            :timeout="toastState.timeout"
            location="top"
        >
            {{ toastState.text }}
        </v-snackbar>

    </v-app>
</template>

<style scoped>
.app-shell {
    background: #f4f7fb;
    transition: background-color 0.2s ease;
}

.app-shell--dark {
    background: #0f172a;
}

.app-shell__main {
    background: transparent;
}

.app-shell__workspace {
    display: flex;
    align-items: flex-start;
    gap: 20px;
    min-height: 100vh;
    padding: 0;
}

.app-shell__workspace--dock-left {
    flex-direction: row;
}

.app-shell__workspace--dock-right {
    flex-direction: row-reverse;
}

.app-shell__content {
    flex: 1;
    min-width: 0;
}

.cc-settings-select {
    max-width: 180px;
    min-width: 140px;
}

.cc-settings__header {
    display: flex;
    align-items: center;
    padding: 10px 16px;
}

.cc-settings__header-wrap {
    line-height: 1.3;
}

.cc-settings__title {
    font-size: 18px;
    font-weight: 600;
    line-height: 1.4;
}

.cc-settings__version {
    font-size: 12px;
    color: rgb(var(--v-theme-on-background));
    opacity: 0.7;
}

.cc-settings__item {
    padding: 2px 4px !important;
    min-height: 40px;
}

.cc-settings__group {
    margin-bottom: 10px;
}

.cc-settings__subheader {
    padding: 0 4px 2px;
    font-size: 12px;
    font-weight: 500;
    color: rgb(var(--v-theme-on-background));
    opacity: 0.75;
}

.cc-settings__list {
    background: transparent;
    padding: 0;
}

.cc-settings__reward-row {
    justify-content: center;
}

.cc-settings__reward-label {
    font-size: 12px;
    color: rgba(0, 0, 0, 0.6);
    margin-bottom: 4px;
}

.cc-settings__reward-qr {
    width: 150px;
    height: 150px;
    object-fit: contain;
    border-radius: 8px;
}

.cc-settings__warm-text {
    white-space: pre-line;
    line-height: 1.7;
}

.cc-settings__warm-box {
    background: rgba(99, 102, 241, 0.06);
    border: 1px solid rgba(148, 163, 184, 0.25);
    border-radius: 10px;
    padding: 0.75rem 1rem;
}

.cc-settings__link-title {
    color: inherit;
}

.cc-picker-dialog__body {
    padding-top: 0;
}

.cc-picker-dialog__body .v-color-picker {
    width: 100%;
    max-width: 340px;
}

.cc-settings__theme-actions {
    display: flex;
    align-items: center;
    gap: 8px;
}

.cc-settings__theme-btn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    white-space: nowrap;
}

.cc-settings__donate-btn {
    margin-top: 2px;
}

.cc-settings__swatch {
    display: inline-block;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 1px solid rgba(var(--v-theme-on-background), 0.2);
}

.cc-settings__link {
    display: inline-flex;
    align-items: center;
    color: inherit;
    text-decoration: none;
}

.cc-settings__link:hover {
    color: rgb(var(--v-theme-primary));
}

.cc-settings__external {
    margin-left: 4px;
    opacity: 0.55;
}

.v-alert {
    top: 0;
    z-index: 5;
}

.room-browser {
    border-top-left-radius: 24px;
    border-top-right-radius: 24px;
    background: rgba(255, 255, 255, 0.96);
}

.room-browser--dark {
    background: rgba(15, 23, 42, 0.96);
}

/* 头部和列表内部样式都在 RoomList.vue 里 —— 它们必须跟着当前模式走，
   而模式皮肤是那组 --rl-* 变量。留在这里的只有「容器」这一层。 */

.room-browser--dock {
    position: sticky;
    top: 64px;
    flex: 0 0 380px;
    width: 380px;
    max-height: calc(100vh - 84px);
    border-radius: 24px;
    border: 1px solid rgba(148, 163, 184, 0.18);
    box-shadow: 0 24px 60px rgba(15, 23, 42, 0.14);
    overflow: hidden;
}

@media (max-width: 1263px) {
    .app-shell__workspace {
        display: block;
        padding: 0;
    }
}

/* 认证密码与进入房间弹窗的输入框：去掉灰色填充底色 */
:deep(.cc-dialog-field .v-field__field) {
    background-color: transparent !important;
}
:deep(.cc-dialog-field.v-field--error) {
    background-color: transparent !important;
}

</style>