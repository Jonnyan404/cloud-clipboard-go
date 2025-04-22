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
                    <v-text-field v-model="$root.authCode" :label="$t('password')"></v-text-field>
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
    mdiTranslate, // 添加图标
    mdiClockOutline,      // Add icon
    mdiIpNetworkOutline,  // Add icon
} from '@mdi/js';

export default {
    data() {
        return {
            drawer: false,
            colorDialog: false,
            clearAllDialog: false,
            clipboardClearedMessageVisible: false, // Add this line
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
            mdiTranslate, // 添加图标
            mdiClockOutline,      // Add icon
            mdiIpNetworkOutline,  // Add icon
            navigator, // 使 navigator 可用
        };
    },
    computed: {
        currentLanguageName() {
            // 根据当前语言环境返回名称
            switch (this.$i18n.locale) {
                case 'zh': return '简体中文';
                case 'zh-TW': return '繁體中文';
                case 'ja': return '日本語';
                case 'en':
                default: return 'English';
            }
        }
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
        }
    },
    mounted() {
        // primary color <==> localStorage
        // theme colors <== localStorage
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
    },
    watch: {
        // Add watcher to hide the message when route changes
        '$route'() {
            this.clipboardClearedMessageVisible = false;
        }
    }
};
</script>
