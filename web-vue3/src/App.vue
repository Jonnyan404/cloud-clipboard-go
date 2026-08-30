<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useDisplay } from 'vuetify';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { toast, toastState } from '@/plugins/toast';

const mdiBrightness4 = 'mdi-brightness-4';
const mdiBulletinBoard = 'mdi-bulletin-board';
const mdiChevronLeft = 'mdi-chevron-left';
const mdiChevronRight = 'mdi-chevron-right';
const mdiClockOutline = 'mdi-clock-outline';
const mdiClose = 'mdi-close';
const mdiContentPaste = 'mdi-content-paste';
const mdiDevices = 'mdi-devices';
const mdiDiceMultiple = 'mdi-dice-multiple';
const mdiHeart = 'mdi-heart';
const mdiHeartOutline = 'mdi-heart-outline';
const mdiHome = 'mdi-home';
const mdiHomeOutline = 'mdi-home-outline';
const mdiInformation = 'mdi-information';
const mdiIpNetworkOutline = 'mdi-ip-network-outline';
const mdiLanConnect = 'mdi-lan-connect';
const mdiLanDisconnect = 'mdi-lan-disconnect';
const mdiLanPending = 'mdi-lan-pending';
const mdiLock = 'mdi-lock';
const mdiMagnify = 'mdi-magnify';
const mdiNotificationClearAll = 'mdi-notification-clear-all';
const mdiPalette = 'mdi-palette';
const mdiTranslate = 'mdi-translate';
const mdiViewGrid = 'mdi-view-grid';

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const display = useDisplay();
const { t, locale } = useI18n();
const router = useRouter();
const route = useRoute();

const drawer = ref(false);
const colorDialog = ref(false);
const clearAllDialog = ref(false);
const clipboardClearedMessageVisible = ref(false);
const roomSheet = ref(false);
const roomSearch = ref('');
const roomDockVisible = ref(true);
const roomDockSide = ref('right');
const availableRooms = ref([]);
const roomsLoading = ref(false);

const currentLanguageName = computed(() => {
    switch (locale.value) {
        case 'zh': return '简体中文';
        case 'zh-TW': return '繁體中文';
        case 'ja': return '日本語';
        default: return 'English';
    }
});
const isDesktopRoomDockEnabled = computed(() => {
    return !display.mdAndDown && app.config && app.config.server && app.config.server.roomList;
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
    return filteredRooms.value.find(room => room.name === currentRoomName) || createOptimisticRoom(currentRoomName);
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
const sidebarRoomGroups = computed(() => [
    {
        key: 'favorites',
        title: t('favoriteRoomsLabel'),
        rooms: favoriteRooms.value,
    },
    {
        key: 'other',
        title: t('otherRoomsLabel'),
        rooms: otherRooms.value.concat(activeRooms.value),
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
        if (error.response && error.response.data.msg) {
            toast(t('clearClipboardFailedMsg', { msg: error.response.data.msg }));
        } else {
            toast(t('clearClipboardFailed'));
        }
    }
}
function copyRoomName(roomName) {
    if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(roomName)
            .then(() => toast(t('copiedRoomName', { room: roomName })))
            .catch(err => toast(t('copyFailed', { err })));
    } else {
        try {
            const textArea = document.createElement("textarea");
            textArea.value = roomName;
            textArea.style.position = "absolute";
            textArea.style.left = "-9999px";
            document.body.appendChild(textArea);
            textArea.select();
            document.execCommand('copy');
            document.body.removeChild(textArea);
            toast(t('copiedRoomName', { room: roomName }));
        } catch (err) {
            toast(t('copyFailed', { err }));
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
function formatTime(timestamp) {
    if (!timestamp || timestamp === 0) return t('never');
    const now = Math.floor(Date.now() / 1000);
    const messageTime = timestamp;
    const diff = now - messageTime;
    if (diff < 0) {
        return t('justNow');
    }
    if (diff < 60) {
        return t('justNow');
    } else if (diff < 3600) {
        return t('minutesAgo', { minutes: Math.floor(diff / 60) });
    } else if (diff < 86400) {
        return t('hoursAgo', { hours: Math.floor(diff / 3600) });
    } else {
        return t('daysAgo', { days: Math.floor(diff / 86400) });
    }
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
    const routeRoom = route.query.room || '';
    if (ws.room !== routeRoom) {
        ws.room = routeRoom;
        ws.disconnect();
        ws.connect();
    }
    if (app.config && app.config.server && app.config.server.roomList) {
        ensureRoomPresent();
        fetchRoomList();
    }
});
</script>

<template>
    <v-app class="app-shell" :class="{ 'app-shell--dark': isDark }">
        <v-navigation-drawer
            v-model="drawer"
            temporary
            app
        >
            <v-list>
                <v-list-item link :href="`#/?room=${ws.room}`">
                    <v-list-item-title>{{ t('clipboard') }}</v-list-item-title>
                </v-list-item>
                <v-list-item link href="#/device">
                    <v-list-item-title>{{ t('deviceList') }}</v-list-item-title>
                </v-list-item>

                <v-menu
                    offset-x
                    transition="slide-x-transition"
                    open-on-click
                    open-on-hover
                    :close-on-content-click="false"
                >
                    <template v-slot:activator="{ props }">
                        <v-list-item link v-bind="props">
                            <v-list-item-title>{{ t('darkMode') }}</v-list-item-title>
                        </v-list-item>
                    </template>
                    <v-list>
                        <v-list-item
                            v-for="opt in darkModeOptions"
                            :key="opt.value"
                            :active="app.dark === opt.value"
                            link
                            @click="app.dark = opt.value"
                        >
                            <v-list-item-title>{{ opt.title }}</v-list-item-title>
                            <v-list-item-subtitle v-if="opt.desc"><code>prefers-color-scheme</code> {{ opt.desc }}</v-list-item-subtitle>
                        </v-list-item>
                    </v-list>
                </v-menu>

                <v-list-item link @click="colorDialog = true; drawer = false">
                    <v-list-item-title>{{ t('changeThemeColor') }}</v-list-item-title>
                </v-list-item>

                <v-menu offset-x transition="slide-x-transition">
                    <template v-slot:activator="{ props }">
                        <v-list-item link v-bind="props">
                            <v-list-item-title>{{ t('language') }}</v-list-item-title>
                            <v-list-item-subtitle>{{ currentLanguageName }}</v-list-item-subtitle>
                        </v-list-item>
                    </template>
                    <v-list>
                        <v-list-item @click="changeLocale('zh')"><v-list-item-title>简体中文</v-list-item-title></v-list-item>
                        <v-list-item @click="changeLocale('zh-TW')"><v-list-item-title>繁體中文</v-list-item-title></v-list-item>
                        <v-list-item @click="changeLocale('en')"><v-list-item-title>English</v-list-item-title></v-list-item>
                        <v-list-item @click="changeLocale('ja')"><v-list-item-title>日本語</v-list-item-title></v-list-item>
                    </v-list>
                </v-menu>

                <v-divider></v-divider>
                <v-list-subheader>{{ t('displaySettings') }}</v-list-subheader>

                <v-list-item>
                    <template v-slot:prepend><v-icon>{{ mdiClockOutline }}</v-icon></template>
                    <v-list-item-title @click="app.showTimestamp = !app.showTimestamp" style="cursor: pointer;">{{ t('showTimestamp') }}</v-list-item-title>
                    <template v-slot:append>
                        <v-switch v-model="app.showTimestamp" color="primary" class="ma-0 pa-0" hide-details></v-switch>
                    </template>
                </v-list-item>

                <v-list-item>
                    <template v-slot:prepend><v-icon>{{ mdiDevices }}</v-icon></template>
                    <v-list-item-title @click="app.showDeviceInfo = !app.showDeviceInfo" style="cursor: pointer;">{{ t('showDeviceInfo') }}</v-list-item-title>
                    <template v-slot:append>
                        <v-switch v-model="app.showDeviceInfo" color="primary" class="ma-0 pa-0" hide-details></v-switch>
                    </template>
                </v-list-item>

                <v-list-item>
                    <template v-slot:prepend><v-icon>{{ mdiIpNetworkOutline }}</v-icon></template>
                    <v-list-item-title @click="app.showSenderIP = !app.showSenderIP" style="cursor: pointer;">{{ t('showSenderIP') }}</v-list-item-title>
                    <template v-slot:append>
                        <v-switch v-model="app.showSenderIP" color="primary" class="ma-0 pa-0" hide-details></v-switch>
                    </template>
                </v-list-item>

                <v-divider></v-divider>

                <v-list-item link href="#/about">
                    <v-list-item-title>{{ t('about') }}</v-list-item-title>
                </v-list-item>
            </v-list>
        </v-navigation-drawer>

        <v-app-bar
            app
            color="primary"
            dark
            flat
            class="app-shell__bar"
        >
            <v-app-bar-nav-icon @click.stop="drawer = !drawer" />
            <v-toolbar-title @click="goHome" style="cursor: pointer;">
                {{ t('cloudClipboard') }}
                <span class="d-none d-sm-inline" v-if="ws.room">
                    （<v-icon
                        v-if="currentRoomEntry && currentRoomEntry.isProtected"
                        x-small
                        class="room-title__lock-icon"
                    >{{ mdiLock }}</v-icon>
                    {{ t('room') }}：
                    <abbr :title="t('copyRoomName')" style="cursor:pointer" @click.stop="copyRoomName(ws.room)">{{ws.room}}</abbr>）
                </span>
            </v-toolbar-title>
            <v-spacer></v-spacer>

            <v-tooltip left v-if="app.config && app.config.server && app.config.server.roomList">
                <template v-slot:activator="{ props }">
                    <v-btn icon density="comfortable" variant="text" v-bind="props" @click="openRoomBrowser()">
                        <v-badge
                            :content="availableRooms.length"
                            :model-value="availableRooms.length > 0"
                            color="accent"
                            overlap
                        >
                            <v-icon>{{mdiViewGrid}}</v-icon>
                        </v-badge>
                    </v-btn>
                </template>
                <span>{{ t('roomList') }} ({{ availableRooms.length }})</span>
            </v-tooltip>

            <v-tooltip left>
                <template v-slot:activator="{ props }">
                    <v-btn icon density="comfortable" variant="text" v-bind="props" @click="clearAllDialog = true">
                        <v-icon>{{mdiNotificationClearAll}}</v-icon>
                    </v-btn>
                </template>
                <span>{{ t('clearClipboard') }}</span>
            </v-tooltip>
            <v-tooltip left>
                <template v-slot:activator="{ props }">
                    <v-btn icon density="comfortable" variant="text" v-bind="props" @click="ws.roomInput = ws.room; ws.roomDialog = true">
                        <v-icon>{{mdiBulletinBoard}}</v-icon>
                    </v-btn>
                </template>
                <span>{{ t('enterRoom') }}</span>
            </v-tooltip>
            <v-tooltip left>
                <template v-slot:activator="{ props }">
                    <v-btn icon density="comfortable" variant="text" v-bind="props" @click="if (!ws.websocket && !ws.websocketConnecting) {ws.retry = 0; ws.connect();}">
                        <v-icon v-if="ws.websocket">{{mdiLanConnect}}</v-icon>
                        <v-icon v-else-if="ws.websocketConnecting">{{mdiLanPending}}</v-icon>
                        <v-icon v-else>{{mdiLanDisconnect}}</v-icon>
                    </v-btn>
                </template>
                <span v-if="ws.websocket">{{ t('connected') }}</span>
                <span v-else-if="ws.websocketConnecting">{{ t('connecting') }}</span>
                <span v-else>{{ t('disconnected') }}</span>
            </v-tooltip>
        </v-app-bar>

        <v-alert
            v-model="clipboardClearedMessageVisible"
            type="error"
            dismissible
            dense
            class="ma-0 text-center"
            style="position: sticky; top: 64px; z-index: 5;"
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
                    <div class="room-browser__header room-browser__header--dock d-flex align-center">
                        <div class="d-flex align-center room-browser__title-wrap">
                            <v-icon start>{{ mdiViewGrid }}</v-icon>
                            <span>{{ t('roomList') }}</span>
                            <v-chip class="ml-2" size="small" :variant="'outlined'">{{ availableRooms.length }} {{ t('rooms') }}</v-chip>
                        </div>
                        <div class="d-flex align-center">
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
                        </div>
                    </div>

                    <div class="room-browser__body room-browser__body--dock">
                        <div class="room-browser__toolbar">
                            <v-text-field
                                v-model="roomSearch"
                                :placeholder="t('searchRooms')"
                                :prepend-inner-icon="mdiMagnify"
                                variant="outlined"
                                density="compact"
                                clearable
                                hide-details
                                class="room-browser__search"
                            ></v-text-field>
                        </div>

                        <div class="room-browser__summary">
                            <v-chip size="small" :variant="'outlined'" color="primary">{{ getRoomDisplayName({ name: ws.room }) }}</v-chip>
                            <v-chip size="small" :variant="'outlined'">{{ favoriteRooms.length }} {{ t('favoriteRoomsLabel') }}</v-chip>
                            <v-chip size="small" :variant="'outlined'">{{ activeRooms.length }} {{ t('activeRoomsLabel') }}</v-chip>
                        </div>

                        <div v-if="roomsLoading" class="text-center py-4">
                            <v-progress-circular indeterminate color="primary"></v-progress-circular>
                            <div class="mt-2">{{ t('loadingRooms') }}</div>
                        </div>

                        <div v-else-if="filteredRooms.length === 0" class="text-center py-8">
                            <v-icon size="64" color="grey-lighten-1">{{ mdiHomeOutline }}</v-icon>
                            <div class="mt-2 text-grey">{{ t('noRoomsFound') }}</div>
                        </div>

                        <div v-else class="room-browser__sections">
                            <section v-if="currentRoomEntry" class="room-group">
                                <div class="room-group__label">{{ t('currentRoomLabel') }}</div>
                                <v-list class="room-list" density="compact">
                                    <v-list-item
                                        class="room-entry room-entry--current"
                                        @click="switchRoom(currentRoomEntry.name)"
                                    >
                                        <template v-slot:prepend>
                                            <v-avatar size="42" class="room-entry__avatar room-entry__avatar--current">
                                                <v-icon color="primary">{{ currentRoomEntry.name === '' ? mdiHomeOutline : mdiHome }}</v-icon>
                                            </v-avatar>
                                        </template>
                                        <v-list-item-title>
                                            <div class="room-entry__title-row">
                                                <div class="room-entry__name">{{ getRoomDisplayName(currentRoomEntry) }}</div>
                                                <div class="room-entry__badges">
                                                    <v-chip
                                                        v-if="currentRoomEntry.isProtected"
                                                        size="x-small"
                                                        :variant="'outlined'"
                                                        class="room-entry__security-chip"
                                                    >
                                                        <v-icon size="x-small" start>{{ mdiLock }}</v-icon>
                                                        {{ t('protectedRoom') }}
                                                    </v-chip>
                                                    <v-chip size="x-small" variant="flat" color="primary">{{ t('currentRoomShortLabel') }}</v-chip>
                                                </div>
                                            </div>
                                        </v-list-item-title>
                                        <v-list-item-subtitle class="room-entry__meta">
                                            {{ currentRoomEntry.deviceCount || 0 }} {{ t('devices') }} · {{ t('messages') }} {{ currentRoomEntry.messageCount || 0 }}
                                        </v-list-item-subtitle>
                                        <v-list-item-subtitle class="room-entry__activity">
                                            {{ t('lastActive') }} · {{ formatTime(currentRoomEntry.lastActive) }}
                                        </v-list-item-subtitle>
                                        <template v-slot:append>
                                            <v-btn
                                                icon
                                                size="small"
                                                @click.stop="toggleFavoriteRoom(currentRoomEntry.name)"
                                                :color="currentRoomEntry.isFavorite ? 'error' : ''"
                                            >
                                                <v-icon size="small">
                                                    {{ currentRoomEntry.isFavorite ? mdiHeart : mdiHeartOutline }}
                                                </v-icon>
                                            </v-btn>
                                        </template>
                                    </v-list-item>
                                </v-list>
                            </section>

                            <section
                                v-for="group in sidebarRoomGroups"
                                :key="`dock-${group.key}`"
                                class="room-group"
                            >
                                <div class="room-group__label">{{ group.title }}</div>
                                <v-list class="room-list" density="compact">
                                    <v-list-item
                                        v-for="room in group.rooms"
                                        :key="room.name"
                                        class="room-entry"
                                        :class="{ 'room-entry--active': room.isActive }"
                                        @click="switchRoom(room.name)"
                                    >
                                        <template v-slot:prepend>
                                            <v-avatar size="42" class="room-entry__avatar">
                                                <v-icon :color="room.isActive ? 'success' : 'primary'">
                                                    {{ room.name === '' ? mdiHomeOutline : mdiHome }}
                                                </v-icon>
                                            </v-avatar>
                                        </template>
                                        <v-list-item-title>
                                            <div class="room-entry__title-row">
                                                <div class="room-entry__name">{{ getRoomDisplayName(room) }}</div>
                                                <div class="room-entry__badges">
                                                    <v-chip
                                                        v-if="room.isProtected"
                                                        size="x-small"
                                                        :variant="'outlined'"
                                                        class="room-entry__security-chip"
                                                    >
                                                        <v-icon size="x-small" start>{{ mdiLock }}</v-icon>
                                                        {{ t('protectedRoom') }}
                                                    </v-chip>
                                                    <div class="room-entry__state" :class="room.isActive ? 'room-entry__state--active' : 'room-entry__state--idle'">
                                                        <span class="room-entry__state-dot"></span>
                                                        {{ room.isActive ? t('active') : t('inactive') }}
                                                    </div>
                                                </div>
                                            </div>
                                        </v-list-item-title>
                                        <v-list-item-subtitle class="room-entry__meta">
                                            {{ room.deviceCount || 0 }} {{ t('devices') }} · {{ t('messages') }} {{ room.messageCount || 0 }}
                                        </v-list-item-subtitle>
                                        <v-list-item-subtitle class="room-entry__activity">
                                            {{ t('lastActive') }} · {{ formatTime(room.lastActive) }}
                                        </v-list-item-subtitle>
                                        <template v-slot:append>
                                            <v-btn
                                                icon
                                                size="small"
                                                @click.stop="toggleFavoriteRoom(room.name)"
                                                :color="room.isFavorite ? 'error' : ''"
                                            >
                                                <v-icon size="small">
                                                    {{ room.isFavorite ? mdiHeart : mdiHeartOutline }}
                                                </v-icon>
                                            </v-btn>
                                        </template>
                                    </v-list-item>
                                </v-list>
                            </section>
                        </div>
                    </div>
                </aside>
            </div>
        </v-main>

        <v-dialog v-model="colorDialog" max-width="300">
            <v-card>
                <v-card-title>{{ t('selectThemeColor') }}</v-card-title>
                <v-card-text>
                    <v-color-picker v-if="isDark" v-model="theme.themes.value.dark.colors.primary" show-swatches hide-inputs></v-color-picker>
                    <v-color-picker v-else v-model="theme.themes.value.light.colors.primary" show-swatches hide-inputs></v-color-picker>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn color="primary" variant="text" @click="colorDialog = false">{{ t('ok') }}</v-btn>
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

        <v-bottom-sheet v-model="roomSheet" scrollable max-width="820">
            <v-card class="room-browser" :class="{ 'room-browser--dark': isDark }">
                <v-card-title class="d-flex align-center room-browser__header">
                    <v-icon start>{{ mdiViewGrid }}</v-icon>
                    {{ t('roomList') }}
                    <v-chip class="ml-2" size="small" :variant="'outlined'">{{ availableRooms.length }} {{ t('rooms') }}</v-chip>
                    <v-spacer></v-spacer>
                    <v-btn icon density="comfortable" variant="text" @click="roomSheet = false">
                        <v-icon>{{ mdiClose }}</v-icon>
                    </v-btn>
                </v-card-title>

                <v-divider></v-divider>

                <v-card-text class="room-browser__body">
                    <div class="room-browser__toolbar">
                        <v-text-field
                            v-model="roomSearch"
                            :placeholder="t('searchRooms')"
                            :prepend-inner-icon="mdiMagnify"
                            variant="outlined"
                            density="compact"
                            clearable
                            hide-details
                            class="room-browser__search"
                        ></v-text-field>
                        <v-btn
                            :variant="'outlined'"
                            color="primary"
                            class="room-browser__manual-action"
                            @click="roomSheet = false; ws.roomInput = ws.room; ws.roomDialog = true"
                        >
                            {{ t('enterRoom') }}
                        </v-btn>
                    </div>

                    <div class="room-browser__summary">
                        <v-chip size="small" :variant="'outlined'" color="primary">{{ getRoomDisplayName({ name: ws.room }) }}</v-chip>
                        <v-chip size="small" :variant="'outlined'">{{ favoriteRooms.length }} {{ t('favoriteRoomsLabel') }}</v-chip>
                        <v-chip size="small" :variant="'outlined'">{{ activeRooms.length }} {{ t('activeRoomsLabel') }}</v-chip>
                    </div>

                    <div v-if="roomsLoading" class="text-center py-4">
                        <v-progress-circular indeterminate color="primary"></v-progress-circular>
                        <div class="mt-2">{{ t('loadingRooms') }}</div>
                    </div>

                    <div v-else-if="filteredRooms.length === 0" class="text-center py-8">
                        <v-icon size="64" color="grey-lighten-1">{{ mdiHomeOutline }}</v-icon>
                        <div class="mt-2 text-grey">{{ t('noRoomsFound') }}</div>
                    </div>

                    <div v-else class="room-browser__sections">
                        <section v-if="currentRoomEntry" class="room-group">
                            <div class="room-group__label">{{ t('currentRoomLabel') }}</div>
                            <v-list class="room-list" density="compact">
                                <v-list-item
                                    class="room-entry room-entry--current"
                                    @click="switchRoom(currentRoomEntry.name)"
                                >
                                    <template v-slot:prepend>
                                        <v-avatar size="42" class="room-entry__avatar room-entry__avatar--current">
                                            <v-icon color="primary">{{ currentRoomEntry.name === '' ? mdiHomeOutline : mdiHome }}</v-icon>
                                        </v-avatar>
                                    </template>
                                    <v-list-item-title>
                                        <div class="room-entry__title-row">
                                            <div class="room-entry__name">{{ getRoomDisplayName(currentRoomEntry) }}</div>
                                            <div class="room-entry__badges">
                                                <v-chip
                                                    v-if="currentRoomEntry.isProtected"
                                                    size="x-small"
                                                    :variant="'outlined'"
                                                    class="room-entry__security-chip"
                                                >
                                                    <v-icon size="x-small" start>{{ mdiLock }}</v-icon>
                                                    {{ t('protectedRoom') }}
                                                </v-chip>
                                                <div class="room-entry__state" :class="room.isActive ? 'room-entry__state--active' : 'room-entry__state--idle'">
                                                    <span class="room-entry__state-dot"></span>
                                                    {{ room.isActive ? t('active') : t('inactive') }}
                                                </div>
                                            </div>
                                        </div>
                                    </v-list-item-title>
                                    <v-list-item-subtitle class="room-entry__meta">
                                        {{ currentRoomEntry.deviceCount || 0 }} {{ t('devices') }} · {{ t('messages') }} {{ currentRoomEntry.messageCount || 0 }}
                                    </v-list-item-subtitle>
                                    <v-list-item-subtitle class="room-entry__activity">
                                        {{ t('lastActive') }} · {{ formatTime(currentRoomEntry.lastActive) }}
                                    </v-list-item-subtitle>
                                    <template v-slot:append>
                                        <v-btn
                                            icon
                                            size="small"
                                            @click.stop="toggleFavoriteRoom(currentRoomEntry.name)"
                                            :color="currentRoomEntry.isFavorite ? 'error' : ''"
                                        >
                                            <v-icon size="small">
                                                {{ currentRoomEntry.isFavorite ? mdiHeart : mdiHeartOutline }}
                                            </v-icon>
                                        </v-btn>
                                    </template>
                                </v-list-item>
                            </v-list>
                        </section>

                        <section
                            v-for="group in roomGroups"
                            :key="group.key"
                            class="room-group"
                        >
                            <div class="room-group__label">{{ group.title }}</div>
                            <v-list class="room-list" density="compact">
                                <v-list-item
                                    v-for="room in group.rooms"
                                    :key="room.name"
                                    class="room-entry"
                                    :class="{ 'room-entry--active': room.isActive }"
                                    @click="switchRoom(room.name)"
                                >
                                    <template v-slot:prepend>
                                        <v-avatar size="42" class="room-entry__avatar">
                                            <v-icon :color="room.isActive ? 'success' : 'primary'">
                                                {{ room.name === '' ? mdiHomeOutline : mdiHome }}
                                            </v-icon>
                                        </v-avatar>
                                    </template>
                                    <v-list-item-title>
                                        <div class="room-entry__title-row">
                                            <div class="room-entry__name">{{ getRoomDisplayName(room) }}</div>
                                            <div class="room-entry__badges">
                                                <v-chip
                                                    v-if="room.isProtected"
                                                    size="x-small"
                                                    :variant="'outlined'"
                                                    class="room-entry__security-chip"
                                                >
                                                    <v-icon size="x-small" start>{{ mdiLock }}</v-icon>
                                                    {{ t('protectedRoom') }}
                                                </v-chip>
                                                <div class="room-entry__state" :class="room.isActive ? 'room-entry__state--active' : 'room-entry__state--idle'">
                                                    <span class="room-entry__state-dot"></span>
                                                    {{ room.isActive ? t('active') : t('inactive') }}
                                                </div>
                                            </div>
                                        </div>
                                    </v-list-item-title>
                                    <v-list-item-subtitle class="room-entry__meta">
                                        {{ room.deviceCount || 0 }} {{ t('devices') }} · {{ t('messages') }} {{ room.messageCount || 0 }}
                                    </v-list-item-subtitle>
                                    <v-list-item-subtitle class="room-entry__activity">
                                        {{ t('lastActive') }} · {{ formatTime(room.lastActive) }}
                                    </v-list-item-subtitle>
                                    <template v-slot:append>
                                        <v-btn
                                            icon
                                            size="small"
                                            @click.stop="toggleFavoriteRoom(room.name)"
                                            :color="room.isFavorite ? 'error' : ''"
                                        >
                                            <v-icon size="small">
                                                {{ room.isFavorite ? mdiHeart : mdiHeartOutline }}
                                            </v-icon>
                                        </v-btn>
                                    </template>
                                </v-list-item>
                            </v-list>
                        </section>
                    </div>
                </v-card-text>
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

.app-shell__bar {
    box-shadow: 0 14px 34px rgba(15, 23, 42, 0.18) !important;
}

.app-shell--dark .app-shell__bar {
    box-shadow: 0 14px 34px rgba(2, 6, 23, 0.42) !important;
}

.app-shell__main {
    background: transparent;
}

.app-shell__workspace {
    display: flex;
    align-items: flex-start;
    gap: 20px;
    min-height: calc(100vh - 64px);
    padding: 16px 20px 24px;
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

.v-navigation-drawer :deep(.v-navigation-drawer__border) {
    pointer-events: none;
}

.v-alert {
    top: 64px;
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

.room-browser__header {
    padding-bottom: 12px;
}

.room-browser__header--dock {
    padding: 16px 18px 12px;
    border-bottom: 1px solid rgba(148, 163, 184, 0.18);
}

.room-title__lock-icon {
    margin: 0 4px 2px 0;
    vertical-align: middle;
    opacity: 0.92;
}

.room-browser__title-wrap {
    min-width: 0;
}

.room-browser__body {
    max-height: 68vh;
    padding-top: 20px;
}

.room-browser__body--dock {
    max-height: calc(100vh - 164px);
    overflow: auto;
    padding: 16px 18px 20px;
}

.room-browser__toolbar {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
}

.room-browser__search {
    flex: 1;
}

.room-browser__manual-action {
    flex: 0 0 auto;
}

.room-browser__summary {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
}

.room-group {
    margin-bottom: 8px;
}

.room-group__label {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: rgba(0, 0, 0, 0.54);
    padding: 8px 16px 4px;
    font-weight: 600;
}

.room-list {
    padding-bottom: 8px;
}

.room-entry {
    cursor: pointer;
}

.room-entry--current .room-entry__avatar--current {
    border: 2px solid var(--v-primary-base);
}

.room-entry__name {
    font-weight: 500;
}

.room-entry__badges {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
}

.room-entry__security-chip {
    display: flex;
    align-items: center;
}

.room-entry__state {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 0.75rem;
}

.room-entry__state--active {
    color: success;
}

.room-entry__state--idle {
    color: grey;
}

.room-entry__state-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: currentColor;
}

.room-entry__meta {
    font-size: 0.75rem;
}

.room-entry__activity {
    font-size: 0.75rem;
}

@media (min-width: 1264px) {
    .room-browser--dock {
        position: fixed;
        bottom: 0;
        inset-inline-end: 0;
        width: 400px;
        max-height: 70vh;
        border-radius: 20px 0 0 0;
        box-shadow: 0 -4px 24px rgba(0, 0, 0, 0.12);
        z-index: 100;
    }

    .room-browser--dock-left {
        right: auto;
        left: 0;
        border-radius: 0 0 0 20px;
        inset-inline-end: auto;
        inset-inline-start: 0;
    }

    .room-browser--dock .room-browser__header--dock {
        border-radius: 20px 20px 0 0;
    }

    .room-browser--dock .room-browser__body--dock {
        max-height: calc(70vh - 64px);
    }
}
</style>