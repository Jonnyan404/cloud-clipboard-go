<template>
    <v-app>
        <v-navigation-drawer
            v-model="drawer"
            temporary
            app
        >
            <v-list>
                <v-list-item link :href="`#/?room=${$root.room}`">
                    <v-list-item-action>
                        <v-icon>{{mdiContentPaste}}</v-icon>
                    </v-list-item-action>
                    <v-list-item-content>
                        <v-list-item-title>{{ $t('clipboard') }}</v-list-item-title>
                    </v-list-item-content>
                </v-list-item>
                <v-list-item link href="#/device">
                    <v-list-item-action>
                        <v-icon>{{mdiDevices}}</v-icon>
                    </v-list-item-action>
                    <v-list-item-content>
                        <v-list-item-title>{{ $t('deviceList') }}</v-list-item-title>
                    </v-list-item-content>
                </v-list-item>
                <v-menu
                    offset-x
                    transition="slide-x-transition"
                    open-on-click
                    open-on-hover
                    :close-on-content-click="false"
                >
                    <template v-slot:activator="{on}">
                        <v-list-item link v-on="on">
                            <v-list-item-action>
                                <v-icon>{{mdiBrightness4}}</v-icon>
                            </v-list-item-action>
                            <v-list-item-content>
                                <v-list-item-title>{{ $t('darkMode') }}</v-list-item-title>
                            </v-list-item-content>
                        </v-list-item>
                    </template>
                    <v-list two-line>
                        <v-list-item-group v-model="$root.dark" color="primary" mandatory>
                            <v-list-item link value="time">
                                <v-list-item-content>
                                    <v-list-item-title>{{ $t('switchByTime') }}</v-list-item-title>
                                    <v-list-item-subtitle>{{ $t('switchByTimeDesc') }}</v-list-item-subtitle>
                                </v-list-item-content>
                            </v-list-item>
                            <v-list-item link value="prefer">
                                <v-list-item-content>
                                    <v-list-item-title>{{ $t('switchBySystem') }}</v-list-item-title>
                                    <v-list-item-subtitle><code>prefers-color-scheme</code> {{ $t('switchBySystemDesc') }}</v-list-item-subtitle>
                                </v-list-item-content>
                            </v-list-item>
                            <v-list-item link value="enable">
                                <v-list-item-content>
                                    <v-list-item-title>{{ $t('keepEnabled') }}</v-list-item-title>
                                </v-list-item-content>
                            </v-list-item>
                            <v-list-item link value="disable">
                                <v-list-item-content>
                                    <v-list-item-title>{{ $t('keepDisabled') }}</v-list-item-title>
                                </v-list-item-content>
                            </v-list-item>
                        </v-list-item-group>
                    </v-list>
                </v-menu>

                <!-- customize primary color -->
                <v-list-item link @click="colorDialog = true; drawer=false;">
                    <v-list-item-action>
                        <v-icon>{{mdiPalette}}</v-icon>
                    </v-list-item-action>
                    <v-list-item-content>
                        <v-list-item-title>{{ $t('changeThemeColor') }}</v-list-item-title>
                    </v-list-item-content>
                </v-list-item>

                <!-- Language Switcher -->
                <v-menu
                    offset-x
                    transition="slide-x-transition"
                >
                    <template v-slot:activator="{ on }">
                        <v-list-item link v-on="on">
                            <v-list-item-action>
                                <v-icon>{{mdiTranslate}}</v-icon>
                            </v-list-item-action>
                            <v-list-item-content>
                                <v-list-item-title>{{ $t('language') }}</v-list-item-title>
                                <v-list-item-subtitle>{{ currentLanguageName }}</v-list-item-subtitle>
                            </v-list-item-content>
                        </v-list-item>
                    </template>
                    <v-list>
                        <v-list-item @click="changeLocale('zh')">
                            <v-list-item-title>简体中文</v-list-item-title>
                        </v-list-item>
                        <v-list-item @click="changeLocale('zh-TW')">
                            <v-list-item-title>繁體中文</v-list-item-title>
                        </v-list-item>
                        <v-list-item @click="changeLocale('en')">
                            <v-list-item-title>English</v-list-item-title>
                        </v-list-item>
                        <v-list-item @click="changeLocale('ja')">
                            <v-list-item-title>日本語</v-list-item-title>
                        </v-list-item>
                    </v-list>
                </v-menu>

                <v-divider></v-divider>
                <v-subheader>{{ $t('displaySettings') }}</v-subheader>

                <v-list-item>
                    <!-- Icon on the left -->
                    <v-list-item-icon>
                         <v-icon>{{ mdiClockOutline }}</v-icon>
                    </v-list-item-icon>
                    <!-- Content in the middle -->
                    <v-list-item-content @click="$root.showTimestamp = !$root.showTimestamp" style="cursor: pointer;">
                        <v-list-item-title>{{ $t('showTimestamp') }}</v-list-item-title>
                    </v-list-item-content>
                    <!-- Action (Switch) on the right -->
                    <v-list-item-action>
                        <v-switch v-model="$root.showTimestamp" color="primary" class="ma-0 pa-0" hide-details></v-switch>
                    </v-list-item-action>
                </v-list-item>

                <v-list-item>
                    <!-- Icon on the left -->
                    <v-list-item-icon>
                         <v-icon>{{ mdiDevices }}</v-icon>
                    </v-list-item-icon>
                    <!-- Content in the middle -->
                    <v-list-item-content @click="$root.showDeviceInfo = !$root.showDeviceInfo" style="cursor: pointer;">
                        <v-list-item-title>{{ $t('showDeviceInfo') }}</v-list-item-title>
                    </v-list-item-content>
                    <!-- Action (Switch) on the right -->
                    <v-list-item-action>
                        <v-switch v-model="$root.showDeviceInfo" color="primary" class="ma-0 pa-0" hide-details></v-switch>
                    </v-list-item-action>
                </v-list-item>

                <v-list-item>
                    <!-- Icon on the left -->
                    <v-list-item-icon>
                         <v-icon>{{ mdiIpNetworkOutline }}</v-icon>
                    </v-list-item-icon>
                    <!-- Content in the middle -->
                    <v-list-item-content @click="$root.showSenderIP = !$root.showSenderIP" style="cursor: pointer;">
                        <v-list-item-title>{{ $t('showSenderIP') }}</v-list-item-title>
                    </v-list-item-content>
                    <!-- Action (Switch) on the right -->
                    <v-list-item-action>
                        <v-switch v-model="$root.showSenderIP" color="primary" class="ma-0 pa-0" hide-details></v-switch>
                    </v-list-item-action>
                </v-list-item>

                 <v-divider></v-divider>

                <v-list-item link href="#/about">
                    <v-list-item-action>
                        <v-icon>{{mdiInformation}}</v-icon>
                    </v-list-item-action>
                    <v-list-item-content>
                        <v-list-item-title>{{ $t('about') }}</v-list-item-title>
                    </v-list-item-content>
                </v-list-item>
            </v-list>
        </v-navigation-drawer>

        <v-app-bar
            app
            color="primary"
            dark
        >
            <v-app-bar-nav-icon @click.stop="drawer = !drawer" />
            <v-toolbar-title @click="goHome" style="cursor: pointer;">
                {{ $t('cloudClipboard') }}<span class="d-none d-sm-inline" v-if="$root.room">（{{ $t('room') }}：<abbr :title="$t('copyRoomName')" style="cursor:pointer" @click.stop="copyRoomName($root.room)">{{$root.room}}</abbr>）</span>
            </v-toolbar-title>
            <v-spacer></v-spacer>
            
            <!-- Room List Button (only show if roomList is enabled) -->
            <v-tooltip left v-if="$root.config && $root.config.server && $root.config.server.roomList">
                <template v-slot:activator="{ on }">
                    <v-btn icon v-on="on" @click="roomSheet = true; fetchRoomList();">
                        <v-badge
                            :content="availableRooms.length"
                            :value="availableRooms.length > 0"
                            color="accent"
                            overlap
                        >
                            <v-icon>{{mdiViewGrid}}</v-icon>
                        </v-badge>
                    </v-btn>
                </template>
                <span>{{ $t('roomList') }} ({{ availableRooms.length }})</span>
            </v-tooltip>

            <v-tooltip left>
                <template v-slot:activator="{ on }">
                    <v-btn icon v-on="on" @click="clearAllDialog = true">
                        <v-icon>{{mdiNotificationClearAll}}</v-icon>
                    </v-btn>
                </template>
                <span>{{ $t('clearClipboard') }}</span>
            </v-tooltip>
            <v-tooltip left>
                <template v-slot:activator="{ on }">
                    <v-btn icon v-on="on" @click="$root.roomInput = $root.room; $root.roomDialog = true">
                        <v-icon>{{mdiBulletinBoard}}</v-icon>
                    </v-btn>
                </template>
                <span>{{ $t('enterRoom') }}</span>
            </v-tooltip>
            <v-tooltip left>
                <template v-slot:activator="{ on }">
                    <v-btn icon v-on="on" @click="if (!$root.websocket && !$root.websocketConnecting) {$root.retry = 0; $root.connect();}">
                        <v-icon v-if="$root.websocket">{{mdiLanConnect}}</v-icon>
                        <v-icon v-else-if="$root.websocketConnecting">{{mdiLanPending}}</v-icon>
                        <v-icon v-else>{{mdiLanDisconnect}}</v-icon>
                    </v-btn>
                </template>
                <span v-if="$root.websocket">{{ $t('connected') }}</span>
                <span v-else-if="$root.websocketConnecting">{{ $t('connecting') }}</span>
                <span v-else>{{ $t('disconnected') }}</span>
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
            {{ $t('clipboardClearedRefresh') }}
        </v-alert>

        <v-main>
            <template v-if="$route.meta.keepAlive">
                <keep-alive><router-view /></keep-alive>
            </template>
            <router-view v-else />
        </v-main>

        <v-dialog v-model="colorDialog" max-width="300" hide-overlay>
            <v-card>
                <v-card-title>{{ $t('selectThemeColor') }}</v-card-title>
                <v-card-text>
                    <v-color-picker v-if="$vuetify.theme.dark" v-model="$vuetify.theme.themes.dark.primary " show-swatches hide-inputs></v-color-picker>
                    <v-color-picker v-else                     v-model="$vuetify.theme.themes.light.primary" show-swatches hide-inputs></v-color-picker>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn color="primary" text @click="colorDialog = false">{{ $t('ok') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>

        <v-dialog v-model="$root.authCodeDialog" persistent max-width="360">
            <v-card>
                <v-card-title class="headline">{{ $t('authRequired') }}</v-card-title>
                <v-card-text>
                    <p>{{ $t('authPrompt') }}</p>
                    <v-text-field v-model="$root.authCode" :label="$t('password')"
                    @keyup.enter="$root.authCodeDialog = false; $root.connect();"
                    autofocus
                    ></v-text-field>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn
                        color="primary darken-1"
                        text
                        @click="
                            $root.authCodeDialog = false;
                            $root.connect();
                        "
                    >{{ $t('submit') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>

        <v-dialog v-model="$root.roomDialog" persistent max-width="360">
            <v-card>
                <v-card-title class="headline">{{ $t('clipboardRoom') }}</v-card-title>
                <v-card-text>
                    <p>{{ $t('roomPrompt1') }}</p>
                    <p>{{ $t('roomPrompt2') }}</p>
                    <v-text-field
                        v-model="$root.roomInput"
                        :label="$t('roomName')"
                        :append-icon="mdiDiceMultiple"
                        @click:append="$root.roomInput = ['reimu', 'marisa', 'rumia', 'cirno', 'meiling', 'patchouli', 'sakuya', 'remilia', 'flandre', 'letty', 'chen', 'lyrica', 'lunasa', 'merlin', 'youmu', 'yuyuko', 'ran', 'yukari', 'suika', 'mystia', 'keine', 'tewi', 'reisen', 'eirin', 'kaguya', 'mokou'][Math.floor(Math.random() * 26)] + '-' + Math.random().toString(16).substring(2, 6)"
                        @keyup.enter="$router.push({ path: '/', query: { room: $root.roomInput }}); $root.roomDialog = false;"
                        autofocus
                    ></v-text-field>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn
                        color="primary darken-1"
                        text
                        @click="$root.roomDialog = false"
                    >{{ $t('cancel') }}</v-btn>
                    <v-btn
                        color="primary darken-1"
                        text
                        @click="
                            $router.push({ path: '/', query: { room: $root.roomInput }});
                            $root.roomDialog = false;
                        "
                    >{{ $t('enterRoom') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>

        <v-dialog v-model="clearAllDialog" max-width="360">
            <v-card>
                <v-card-title class="headline">{{ $t('clearClipboardConfirmTitle') }}</v-card-title>
                <v-card-text>
                    <p>{{ $t('clearClipboardConfirmText') }}</p>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn
                        color="primary darken-1"
                        text
                        @click="clearAllDialog = false"
                    >{{ $t('cancel') }}</v-btn>
                    <v-btn
                        color="primary darken-1"
                        text
                        @click="clearAllDialog = false; clearAll(); clipboardClearedMessageVisible = true;"  
                    >{{ $t('ok') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>

        <!-- Room List Bottom Sheet -->
        <v-bottom-sheet v-model="roomSheet" scrollable max-width="800">
            <v-card>
                <v-card-title class="d-flex align-center">
                    <v-icon left>{{ mdiViewGrid }}</v-icon>
                    {{ $t('roomList') }}
                    <v-chip class="ml-2" small outlined>{{ availableRooms.length }} {{ $t('rooms') }}</v-chip>
                    <v-spacer></v-spacer>
                    <!-- 移除刷新按钮 -->
                    <v-btn icon @click="roomSheet = false">
                        <v-icon>{{ mdiClose }}</v-icon>
                    </v-btn>
                </v-card-title>
                
                <v-divider></v-divider>
                
                <v-card-text style="max-height: 60vh;">
                    <!-- Search Box -->
                    <v-text-field
                        v-model="roomSearch"
                        :placeholder="$t('searchRooms')"
                        :prepend-inner-icon="mdiMagnify"
                        outlined
                        dense
                        clearable
                        class="mb-4"
                    ></v-text-field>
                    
                    <!-- Loading State -->
                    <div v-if="roomsLoading" class="text-center py-4">
                        <v-progress-circular indeterminate color="primary"></v-progress-circular>
                        <div class="mt-2">{{ $t('loadingRooms') }}</div>
                    </div>
                    
                    <!-- Empty State -->
                    <div v-else-if="filteredRooms.length === 0" class="text-center py-8">
                        <v-icon size="64" color="grey lighten-1">{{ mdiHomeOutline }}</v-icon>
                        <div class="mt-2 grey--text">{{ $t('noRoomsFound') }}</div>
                    </div>
                    
                    <!-- Room Grid -->
                    <v-row v-else>
                        <v-col
                            v-for="room in filteredRooms"
                            :key="room.name"
                            cols="12"
                            sm="6"
                            md="4"
                        >
                            <v-card
                                outlined
                                hover
                                :color="$root.room === room.name ? 'primary' : ''"
                                :dark="$root.room === room.name"
                                @click="switchRoom(room.name)"
                                style="cursor: pointer; transition: all 0.3s ease;"
                                :elevation="$root.room === room.name ? 4 : 0"
                            >
                                <v-card-text>
                                    <div class="d-flex justify-space-between align-center mb-3">
                                        <v-icon :color="$root.room === room.name ? 'white' : 'primary'">
                                            {{ room.name === '' ? mdiHomeOutline : mdiHome }}
                                        </v-icon>
                                        <v-chip
                                            x-small
                                            :color="room.isActive ? 'success' : 'grey'"
                                            :text-color="room.isActive ? 'white' : 'grey darken-2'"
                                        >
                                            {{ room.isActive ? $t('active') : $t('inactive') }}
                                        </v-chip>
                                    </div>
                                    
                                    <div class="subtitle-1 font-weight-bold mb-2" style="word-break: break-word;">
                                        {{ room.name || $t('publicRoom') }}
                                    </div>
                                    
                                    <div class="caption mb-3" :class="$root.room === room.name ? 'white--text' : 'grey--text'">
                                        {{ $t('messages') }}: {{ room.messageCount }}<br>
                                        {{ $t('lastActive') }}: {{ formatTime(room.lastActive) }}
                                    </div>
                                    
                                    <div class="d-flex justify-space-between align-center">
                                        <v-chip 
                                            x-small 
                                            :outlined="$root.room !== room.name"
                                            :color="$root.room === room.name ? 'white' : 'primary'"
                                            :text-color="$root.room === room.name ? 'primary' : 'white'"
                                        >
                                            {{ room.deviceCount }} {{ $t('devices') }}
                                        </v-chip>
                                        <v-btn
                                            x-small
                                            icon
                                            @click.stop="toggleFavoriteRoom(room.name)"
                                            :color="$root.room === room.name ? 'white' : 'grey'"
                                        >
                                            <v-icon small>
                                                {{ room.isFavorite ? mdiHeart : mdiHeartOutline }}
                                            </v-icon>
                                        </v-btn>
                                    </div>
                                </v-card-text>
                            </v-card>
                        </v-col>
                    </v-row>
                </v-card-text>
            </v-card>
        </v-bottom-sheet>

    </v-app>
</template>

<style scoped>
.v-navigation-drawer >>> .v-navigation-drawer__border {
    pointer-events: none;
}

/* Ensure alert is above main content but below app bar */
.v-alert {
    /* Adjust top value if your app bar height is different */
    top: 64px; /* Default Vuetify app bar height */
    z-index: 5; /* Ensure it's above v-main but below v-app-bar */
}
</style>

<script>
import {
    mdiContentPaste,
    mdiDevices,
    mdiInformation,
    mdiLanConnect,
    mdiLanDisconnect,
    mdiLanPending,
    mdiBrightness4,
    mdiBulletinBoard,
    mdiDiceMultiple,
    mdiPalette,
    mdiNotificationClearAll,
    mdiTranslate,
    mdiClockOutline,
    mdiIpNetworkOutline,
    mdiViewGrid,
    mdiClose,
    mdiMagnify,
    mdiHome,
    mdiHomeOutline,
    mdiHeart,
    mdiHeartOutline,
} from '@mdi/js';

export default {
    data() {
        return {
            drawer: false,
            colorDialog: false,
            clearAllDialog: false,
            clipboardClearedMessageVisible: false,
            roomSheet: false,
            roomSearch: '',
            availableRooms: [],
            roomsLoading: false,
            // 图标
            mdiContentPaste,
            mdiDevices,
            mdiInformation,
            mdiLanConnect,
            mdiLanDisconnect,
            mdiLanPending,
            mdiBrightness4,
            mdiBulletinBoard,
            mdiDiceMultiple,
            mdiPalette,
            mdiNotificationClearAll,
            mdiTranslate,
            mdiClockOutline,
            mdiIpNetworkOutline,
            mdiViewGrid,
            mdiClose,
            mdiMagnify,
            mdiHome,
            mdiHomeOutline,
            mdiHeart,
            mdiHeartOutline,
            navigator,
        };
    },
    computed: {
        currentLanguageName() {
            switch (this.$i18n.locale) {
                case 'zh': return '简体中文';
                case 'zh-TW': return '繁體中文';
                case 'ja': return '日本語';
                case 'en':
                default: return 'English';
            }
        },
        filteredRooms() {
            let rooms = this.availableRooms;
            
            // 先按搜索条件过滤
            if (this.roomSearch) {
                rooms = rooms.filter(room => 
                    (room.name || this.$t('publicRoom')).toLowerCase().includes(this.roomSearch.toLowerCase())
                );
            }
            
            // 按收藏状态排序：收藏的房间在前，未收藏的在后
            // 在每个分组内保持原有顺序（可能是按活跃度或其他后端逻辑排序的）
            return rooms.sort((a, b) => {
                // 如果收藏状态不同，收藏的排在前面
                if (a.isFavorite !== b.isFavorite) {
                    return b.isFavorite - a.isFavorite; // true = 1, false = 0，所以收藏的(1)排在前面
                }
                // 如果收藏状态相同，保持原有顺序（返回0表示不改变相对位置）
                return 0;
            });
        },
    },
    methods: {
        async clearAll() {
            // Set message visible immediately on confirmation
            // this.clipboardClearedMessageVisible = true; // Moved to button click for immediate feedback

            try {
                const files = this.$root.received.filter(e => e.type === 'file');
                await this.$http.delete('revoke/all', {
                    params: { room: this.$root.room },
                });
                // No need to delete individual files if revoke/all works correctly
                // for (const file of files) {
                //     await this.$http.delete(`file/${file.cache}`);
                // }
            } catch (error) {
                console.log(error);
                // Hide the generic success message if there's an error
                this.clipboardClearedMessageVisible = false;
                if (error.response && error.response.data.msg) {
                    this.$toast(this.$t('clearClipboardFailedMsg', { msg: error.response.data.msg }));
                } else {
                    this.$toast(this.$t('clearClipboardFailed'));
                }
            }
        },
        copyRoomName(roomName) {
            if (navigator.clipboard && window.isSecureContext) {
                navigator.clipboard.writeText(roomName)
                    .then(() => this.$toast(this.$t('copiedRoomName', { room: roomName })))
                    .catch(err => this.$toast(this.$t('copyFailed', { err: err })));
            } else {
                // 兼容旧浏览器或非安全上下文
                try {
                    const textArea = document.createElement("textarea");
                    textArea.value = roomName;
                    textArea.style.position = "absolute";
                    textArea.style.left = "-9999px";
                    document.body.appendChild(textArea);
                    textArea.select();
                    document.execCommand('copy');
                    document.body.removeChild(textArea);
                    this.$toast(this.$t('copiedRoomName', { room: roomName }));
                } catch (err) {
                    this.$toast(this.$t('copyFailed', { err: err }));
                }
            }
        },
        changeLocale(locale) {
            if (this.$i18n.locale !== locale) {
                this.$i18n.locale = locale;
                localStorage.setItem('locale', locale); // 保存用户选择
            }
        },
        // Add goHome method
        goHome() {
            console.log('goHome triggered. Current route:', this.$route.fullPath); // Log full path for debugging
            // Navigate to '/' if the current path is not '/' OR if there are query parameters
            if (this.$route.path !== '/' || Object.keys(this.$route.query).length > 0) {
                 console.log('Navigating to / (Public Room)');
                 this.$router.push('/'); // Navigate to the root path, clearing query parameters
            } else {
                 console.log('Already on public room (/), not navigating.');
            }
        },

        // 简化的房间列表获取方法 - 添加更严格的检查
        async fetchRoomList() {
            // 添加更严格的配置检查
            if (!this.$root.config || 
                !this.$root.config.server || 
                !this.$root.config.server.roomList) {
                console.log('房间列表功能未启用或配置未完成加载');
                return;
            }
            
            // 如果正在加载中，避免重复请求
            if (this.roomsLoading) {
                console.log('房间列表正在加载中，跳过重复请求');
                return;
            }
            
            this.roomsLoading = true;
            console.log('获取房间列表');
            
            try {
                const response = await this.$http.get('rooms');
                const favoriteRooms = this.getFavoriteRooms();
                this.availableRooms = response.data.rooms.map(room => ({
                    ...room,
                    isFavorite: favoriteRooms.includes(room.name)
                }));
                console.log(`房间列表更新成功，共 ${this.availableRooms.length} 个房间`);
            } catch (error) {
                console.error('Failed to fetch room list:', error);
                this.$toast(this.$t('failedToLoadRooms'));
            } finally {
                this.roomsLoading = false;
            }
        },

        // 切换房间
        switchRoom(roomName) {
            this.roomSheet = false;
            if (roomName === '') {
                // 公共房间
                this.$router.push('/');
            } else {
                this.$router.push({ path: '/', query: { room: roomName } });
            }
        },

        // 获取收藏房间列表
        getFavoriteRooms() {
            try {
                return JSON.parse(localStorage.getItem('favoriteRooms') || '[]');
            } catch {
                return [];
            }
        },

        // 切换房间收藏状态
        toggleFavoriteRoom(roomName) {
            const favorites = this.getFavoriteRooms();
            const index = favorites.indexOf(roomName);
            
            if (index > -1) {
                favorites.splice(index, 1);
                this.$toast(this.$t('removedFromFavorites', { room: roomName || this.$t('publicRoom') }));
            } else {
                favorites.push(roomName);
                this.$toast(this.$t('addedToFavorites', { room: roomName || this.$t('publicRoom') }));
            }
            
            localStorage.setItem('favoriteRooms', JSON.stringify(favorites));
            
            // 更新当前房间列表的收藏状态
            const room = this.availableRooms.find(r => r.name === roomName);
            if (room) {
                room.isFavorite = !room.isFavorite;
            }
            
            // 注意：由于使用了计算属性 filteredRooms，收藏状态改变后会自动重新排序
        },

        // 修复格式化时间方法
        formatTime(timestamp) {
            if (!timestamp || timestamp === 0) return this.$t('never');
            
            const now = Math.floor(Date.now() / 1000); // 当前时间（秒）
            const messageTime = timestamp; // 后端返回的已经是秒
            const diff = now - messageTime;
            
            // 如果时间戳是未来时间（可能是错误数据），返回 "刚刚"
            if (diff < 0) {
                return this.$t('justNow');
            }
            
            if (diff < 60) { // 1分钟内
                return this.$t('justNow');
            } else if (diff < 3600) { // 1小时内
                return this.$t('minutesAgo', { minutes: Math.floor(diff / 60) });
            } else if (diff < 86400) { // 24小时内
                return this.$t('hoursAgo', { hours: Math.floor(diff / 3600) });
            } else {
                return this.$t('daysAgo', { days: Math.floor(diff / 86400) });
            }
        },

        // 移除自动刷新相关方法
        // startRoomRefresh() - 已删除
        // stopRoomRefresh() - 已删除
    },
    mounted() {
        // primary color <==> localStorage
        const darkPrimary = localStorage.getItem('darkPrimary');
        const lightPrimary = localStorage.getItem('lightPrimary');
        if (darkPrimary) {
            this.$vuetify.theme.themes.dark.primary = darkPrimary;
        }
        if (lightPrimary) {
            this.$vuetify.theme.themes.light.primary = lightPrimary;
        }

        // theme colors ==> localStorage
        this.$watch('$vuetify.theme.themes.dark.primary', (newVal) => {
            localStorage.setItem('darkPrimary', newVal);
        });
        this.$watch('$vuetify.theme.themes.light.primary', (newVal) => {
            localStorage.setItem('lightPrimary', newVal);
        });

        console.log('App.vue mounted - 房间列表将在用户点击时获取');
    },
    watch: {
        '$route'() {
            this.clipboardClearedMessageVisible = false;
        }
    }
};
</script>
