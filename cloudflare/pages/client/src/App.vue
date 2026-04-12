<template>
    <v-app class="app-shell" :class="{ 'app-shell--dark': $vuetify.theme.dark }">
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

                <v-list-item link @click="colorDialog = true; drawer=false;">
                    <v-list-item-action>
                        <v-icon>{{mdiPalette}}</v-icon>
                    </v-list-item-action>
                    <v-list-item-content>
                        <v-list-item-title>{{ $t('changeThemeColor') }}</v-list-item-title>
                    </v-list-item-content>
                </v-list-item>

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
                    <v-list-item-icon>
                         <v-icon>{{ mdiClockOutline }}</v-icon>
                    </v-list-item-icon>
                    <v-list-item-content @click="$root.showTimestamp = !$root.showTimestamp" style="cursor: pointer;">
                        <v-list-item-title>{{ $t('showTimestamp') }}</v-list-item-title>
                    </v-list-item-content>
                    <v-list-item-action>
                        <v-switch v-model="$root.showTimestamp" color="primary" class="ma-0 pa-0" hide-details></v-switch>
                    </v-list-item-action>
                </v-list-item>

                <v-list-item>
                    <v-list-item-icon>
                         <v-icon>{{ mdiDevices }}</v-icon>
                    </v-list-item-icon>
                    <v-list-item-content @click="$root.showDeviceInfo = !$root.showDeviceInfo" style="cursor: pointer;">
                        <v-list-item-title>{{ $t('showDeviceInfo') }}</v-list-item-title>
                    </v-list-item-content>
                    <v-list-item-action>
                        <v-switch v-model="$root.showDeviceInfo" color="primary" class="ma-0 pa-0" hide-details></v-switch>
                    </v-list-item-action>
                </v-list-item>

                <v-list-item>
                    <v-list-item-icon>
                         <v-icon>{{ mdiIpNetworkOutline }}</v-icon>
                    </v-list-item-icon>
                    <v-list-item-content @click="$root.showSenderIP = !$root.showSenderIP" style="cursor: pointer;">
                        <v-list-item-title>{{ $t('showSenderIP') }}</v-list-item-title>
                    </v-list-item-content>
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
            flat
            class="app-shell__bar"
        >
            <v-app-bar-nav-icon @click.stop="drawer = !drawer" />
            <v-toolbar-title @click="goHome" style="cursor: pointer;">
                {{ $t('cloudClipboard') }}
                <span class="d-none d-sm-inline" v-if="$root.room">
                    （<v-icon
                        v-if="currentRoomEntry && currentRoomEntry.isProtected"
                        x-small
                        class="room-title__lock-icon"
                    >{{ mdiLock }}</v-icon>
                    {{ $t('room') }}：
                    <abbr :title="$t('copyRoomName')" style="cursor:pointer" @click.stop="copyRoomName($root.room)">{{$root.room}}</abbr>）
                </span>
            </v-toolbar-title>
            <v-spacer></v-spacer>

            <v-tooltip left v-if="$root.config && $root.config.server && $root.config.server.roomList">
                <template v-slot:activator="{ on }">
                    <v-btn icon v-on="on" @click="openRoomBrowser()">
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

        <v-main class="app-shell__main">
            <div
                class="app-shell__workspace"
                :class="{
                    'app-shell__workspace--dock-right': isDesktopRoomDockEnabled && roomDockSide === 'right',
                    'app-shell__workspace--dock-left': isDesktopRoomDockEnabled && roomDockSide === 'left'
                }"
            >
                <div class="app-shell__content">
                    <template v-if="$route.meta.keepAlive">
                        <keep-alive><router-view /></keep-alive>
                    </template>
                    <router-view v-else />
                </div>

                <aside
                    v-if="isDesktopRoomDockVisible"
                    class="room-browser room-browser--dock"
                    :class="[
                        { 'room-browser--dark': $vuetify.theme.dark },
                        roomDockSide === 'left' ? 'room-browser--dock-left' : 'room-browser--dock-right'
                    ]"
                >
                    <div class="room-browser__header room-browser__header--dock d-flex align-center">
                        <div class="d-flex align-center room-browser__title-wrap">
                            <v-icon left>{{ mdiViewGrid }}</v-icon>
                            <span>{{ $t('roomList') }}</span>
                            <v-chip class="ml-2" small outlined>{{ availableRooms.length }} {{ $t('rooms') }}</v-chip>
                        </div>
                        <div class="d-flex align-center">
                            <v-tooltip left>
                                <template v-slot:activator="{ on }">
                                    <v-btn icon v-on="on" @click="toggleRoomDockSide()">
                                        <v-icon>{{ roomDockSide === 'right' ? mdiChevronLeft : mdiChevronRight }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ roomDockSide === 'right' ? $t('dockLeft') : $t('dockRight') }}</span>
                            </v-tooltip>
                            <v-tooltip bottom>
                                <template v-slot:activator="{ on }">
                                    <v-btn icon small v-on="on" @click="hideDesktopRoomDock()">
                                        <v-icon>{{ mdiClose }}</v-icon>
                                    </v-btn>
                                </template>
                                <span>{{ $t('hideRoomBrowser') }}</span>
                            </v-tooltip>
                        </div>
                    </div>

                    <div class="room-browser__body room-browser__body--dock">
                        <div class="room-browser__toolbar">
                            <v-text-field
                                v-model="roomSearch"
                                :placeholder="$t('searchRooms')"
                                :prepend-inner-icon="mdiMagnify"
                                outlined
                                dense
                                clearable
                                hide-details
                                class="room-browser__search"
                            ></v-text-field>
                        </div>

                        <div class="room-browser__summary">
                            <v-chip small outlined color="primary">{{ getRoomDisplayName({ name: $root.room }) }}</v-chip>
                            <v-chip small outlined>{{ favoriteRooms.length }} {{ $t('favoriteRoomsLabel') }}</v-chip>
                            <v-chip small outlined>{{ activeRooms.length }} {{ $t('activeRoomsLabel') }}</v-chip>
                        </div>

                        <div v-if="roomsLoading" class="text-center py-4">
                            <v-progress-circular indeterminate color="primary"></v-progress-circular>
                            <div class="mt-2">{{ $t('loadingRooms') }}</div>
                        </div>

                        <div v-else-if="filteredRooms.length === 0" class="text-center py-8">
                            <v-icon size="64" color="grey lighten-1">{{ mdiHomeOutline }}</v-icon>
                            <div class="mt-2 grey--text">{{ $t('noRoomsFound') }}</div>
                        </div>

                        <div v-else class="room-browser__sections">
                            <section v-if="currentRoomEntry" class="room-group">
                                <div class="room-group__label">{{ $t('currentRoomLabel') }}</div>
                                <v-list class="room-list" dense>
                                    <v-list-item
                                        class="room-entry room-entry--current"
                                        @click="switchRoom(currentRoomEntry.name)"
                                    >
                                        <v-list-item-avatar size="42" class="room-entry__avatar room-entry__avatar--current">
                                            <v-icon color="primary">{{ currentRoomEntry.name === '' ? mdiHomeOutline : mdiHome }}</v-icon>
                                        </v-list-item-avatar>
                                        <v-list-item-content>
                                            <div class="room-entry__title-row">
                                                <div class="room-entry__name">{{ getRoomDisplayName(currentRoomEntry) }}</div>
                                                <div class="room-entry__badges">
                                                    <v-chip
                                                        v-if="currentRoomEntry.isProtected"
                                                        x-small
                                                        outlined
                                                        class="room-entry__security-chip"
                                                    >
                                                        <v-icon x-small left>{{ mdiLock }}</v-icon>
                                                        {{ $t('protectedRoom') }}
                                                    </v-chip>
                                                    <v-chip x-small color="primary" text-color="white">{{ $t('currentRoomShortLabel') }}</v-chip>
                                                </div>
                                            </div>
                                            <v-list-item-subtitle class="room-entry__meta">
                                                {{ currentRoomEntry.deviceCount || 0 }} {{ $t('devices') }} · {{ $t('messages') }} {{ currentRoomEntry.messageCount || 0 }}
                                            </v-list-item-subtitle>
                                            <v-list-item-subtitle class="room-entry__activity">
                                                {{ $t('lastActive') }} · {{ formatTime(currentRoomEntry.lastActive) }}
                                            </v-list-item-subtitle>
                                        </v-list-item-content>
                                        <v-list-item-action>
                                            <v-btn
                                                icon
                                                small
                                                @click.stop="toggleFavoriteRoom(currentRoomEntry.name)"
                                                :color="currentRoomEntry.isFavorite ? 'error' : ''"
                                            >
                                                <v-icon small>
                                                    {{ currentRoomEntry.isFavorite ? mdiHeart : mdiHeartOutline }}
                                                </v-icon>
                                            </v-btn>
                                        </v-list-item-action>
                                    </v-list-item>
                                </v-list>
                            </section>

                            <section
                                v-for="group in sidebarRoomGroups"
                                :key="`dock-${group.key}`"
                                class="room-group"
                            >
                                <div class="room-group__label">{{ group.title }}</div>
                                <v-list class="room-list" dense>
                                    <v-list-item
                                        v-for="room in group.rooms"
                                        :key="room.name"
                                        class="room-entry"
                                        :class="{ 'room-entry--active': room.isActive }"
                                        @click="switchRoom(room.name)"
                                    >
                                        <v-list-item-avatar size="42" class="room-entry__avatar">
                                            <v-icon :color="room.isActive ? 'success' : 'primary'">
                                                {{ room.name === '' ? mdiHomeOutline : mdiHome }}
                                            </v-icon>
                                        </v-list-item-avatar>
                                        <v-list-item-content>
                                            <div class="room-entry__title-row">
                                                <div class="room-entry__name">{{ getRoomDisplayName(room) }}</div>
                                                <div class="room-entry__badges">
                                                    <v-chip
                                                        v-if="room.isProtected"
                                                        x-small
                                                        outlined
                                                        class="room-entry__security-chip"
                                                    >
                                                        <v-icon x-small left>{{ mdiLock }}</v-icon>
                                                        {{ $t('protectedRoom') }}
                                                    </v-chip>
                                                    <div class="room-entry__state" :class="room.isActive ? 'room-entry__state--active' : 'room-entry__state--idle'">
                                                        <span class="room-entry__state-dot"></span>
                                                        {{ room.isActive ? $t('active') : $t('inactive') }}
                                                    </div>
                                                </div>
                                            </div>
                                            <v-list-item-subtitle class="room-entry__meta">
                                                {{ room.deviceCount || 0 }} {{ $t('devices') }} · {{ $t('messages') }} {{ room.messageCount || 0 }}
                                            </v-list-item-subtitle>
                                            <v-list-item-subtitle class="room-entry__activity">
                                                {{ $t('lastActive') }} · {{ formatTime(room.lastActive) }}
                                            </v-list-item-subtitle>
                                        </v-list-item-content>
                                        <v-list-item-action>
                                            <v-btn
                                                icon
                                                small
                                                @click.stop="toggleFavoriteRoom(room.name)"
                                                :color="room.isFavorite ? 'error' : ''"
                                            >
                                                <v-icon small>
                                                    {{ room.isFavorite ? mdiHeart : mdiHeartOutline }}
                                                </v-icon>
                                            </v-btn>
                                        </v-list-item-action>
                                    </v-list-item>
                                </v-list>
                            </section>
                        </div>
                    </div>
                </aside>
            </div>
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
                    <p class="text-caption text--secondary mb-3">
                        {{ $t('room') }}: {{ getRoomDisplayName({ name: $root.authPendingRoom || $root.room }) }}
                    </p>
                    <v-text-field
                        v-model="$root.authCode"
                        :label="$t('password')"
                        :loading="$root.authDialogLoading"
                        :disabled="$root.authDialogLoading"
                        :error-messages="$root.authCodeError ? [$root.authCodeError] : []"
                        hide-details="auto"
                        @input="$root.authCodeError = ''"
                        @keyup.enter="$root.submitAuthCodeForPendingRoom()"
                        autofocus
                    ></v-text-field>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn
                        color="primary darken-1"
                        text
                        :loading="$root.authDialogLoading"
                        @click="$root.submitAuthCodeForPendingRoom()"
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
                        @keyup.enter="submitRoomChange()"
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
                        @click="submitRoomChange()"
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

        <v-bottom-sheet v-model="roomSheet" scrollable max-width="820">
            <v-card class="room-browser" :class="{ 'room-browser--dark': $vuetify.theme.dark }">
                <v-card-title class="d-flex align-center room-browser__header">
                    <v-icon left>{{ mdiViewGrid }}</v-icon>
                    {{ $t('roomList') }}
                    <v-chip class="ml-2" small outlined>{{ availableRooms.length }} {{ $t('rooms') }}</v-chip>
                    <v-spacer></v-spacer>
                    <v-btn icon @click="roomSheet = false">
                        <v-icon>{{ mdiClose }}</v-icon>
                    </v-btn>
                </v-card-title>

                <v-divider></v-divider>

                <v-card-text class="room-browser__body">
                    <div class="room-browser__toolbar">
                        <v-text-field
                            v-model="roomSearch"
                            :placeholder="$t('searchRooms')"
                            :prepend-inner-icon="mdiMagnify"
                            outlined
                            dense
                            clearable
                            hide-details
                            class="room-browser__search"
                        ></v-text-field>
                        <v-btn
                            outlined
                            color="primary"
                            class="room-browser__manual-action"
                            @click="roomSheet = false; $root.roomInput = $root.room; $root.roomDialog = true"
                        >
                            {{ $t('enterRoom') }}
                        </v-btn>
                    </div>

                    <div class="room-browser__summary">
                        <v-chip small outlined color="primary">{{ getRoomDisplayName({ name: $root.room }) }}</v-chip>
                        <v-chip small outlined>{{ favoriteRooms.length }} {{ $t('favoriteRoomsLabel') }}</v-chip>
                        <v-chip small outlined>{{ activeRooms.length }} {{ $t('activeRoomsLabel') }}</v-chip>
                    </div>

                    <div v-if="roomsLoading" class="text-center py-4">
                        <v-progress-circular indeterminate color="primary"></v-progress-circular>
                        <div class="mt-2">{{ $t('loadingRooms') }}</div>
                    </div>

                    <div v-else-if="filteredRooms.length === 0" class="text-center py-8">
                        <v-icon size="64" color="grey lighten-1">{{ mdiHomeOutline }}</v-icon>
                        <div class="mt-2 grey--text">{{ $t('noRoomsFound') }}</div>
                    </div>

                    <div v-else class="room-browser__sections">
                        <section v-if="currentRoomEntry" class="room-group">
                            <div class="room-group__label">{{ $t('currentRoomLabel') }}</div>
                            <v-list class="room-list" dense>
                                <v-list-item
                                    class="room-entry room-entry--current"
                                    @click="switchRoom(currentRoomEntry.name)"
                                >
                                    <v-list-item-avatar size="42" class="room-entry__avatar room-entry__avatar--current">
                                        <v-icon color="primary">{{ currentRoomEntry.name === '' ? mdiHomeOutline : mdiHome }}</v-icon>
                                    </v-list-item-avatar>
                                    <v-list-item-content>
                                        <div class="room-entry__title-row">
                                            <div class="room-entry__name">{{ getRoomDisplayName(currentRoomEntry) }}</div>
                                            <div class="room-entry__badges">
                                                <v-chip
                                                    v-if="currentRoomEntry.isProtected"
                                                    x-small
                                                    outlined
                                                    class="room-entry__security-chip"
                                                >
                                                    <v-icon x-small left>{{ mdiLock }}</v-icon>
                                                    {{ $t('protectedRoom') }}
                                                </v-chip>
                                                <v-chip x-small color="primary" text-color="white">{{ $t('currentRoomShortLabel') }}</v-chip>
                                            </div>
                                        </div>
                                        <v-list-item-subtitle class="room-entry__meta">
                                            {{ currentRoomEntry.deviceCount || 0 }} {{ $t('devices') }} · {{ $t('messages') }} {{ currentRoomEntry.messageCount || 0 }}
                                        </v-list-item-subtitle>
                                        <v-list-item-subtitle class="room-entry__activity">
                                            {{ $t('lastActive') }} · {{ formatTime(currentRoomEntry.lastActive) }}
                                        </v-list-item-subtitle>
                                    </v-list-item-content>
                                    <v-list-item-action>
                                        <v-btn
                                            icon
                                            small
                                            @click.stop="toggleFavoriteRoom(currentRoomEntry.name)"
                                            :color="currentRoomEntry.isFavorite ? 'error' : ''"
                                        >
                                            <v-icon small>
                                                {{ currentRoomEntry.isFavorite ? mdiHeart : mdiHeartOutline }}
                                            </v-icon>
                                        </v-btn>
                                    </v-list-item-action>
                                </v-list-item>
                            </v-list>
                        </section>

                        <section
                            v-for="group in roomGroups"
                            :key="group.key"
                            class="room-group"
                        >
                            <div class="room-group__label">{{ group.title }}</div>
                            <v-list class="room-list" dense>
                                <v-list-item
                                    v-for="room in group.rooms"
                                    :key="room.name"
                                    class="room-entry"
                                    :class="{ 'room-entry--active': room.isActive }"
                                    @click="switchRoom(room.name)"
                                >
                                    <v-list-item-avatar size="42" class="room-entry__avatar">
                                        <v-icon :color="room.isActive ? 'success' : 'primary'">
                                            {{ room.name === '' ? mdiHomeOutline : mdiHome }}
                                        </v-icon>
                                    </v-list-item-avatar>
                                    <v-list-item-content>
                                        <div class="room-entry__title-row">
                                            <div class="room-entry__name">{{ getRoomDisplayName(room) }}</div>
                                            <div class="room-entry__badges">
                                                <v-chip
                                                    v-if="room.isProtected"
                                                    x-small
                                                    outlined
                                                    class="room-entry__security-chip"
                                                >
                                                    <v-icon x-small left>{{ mdiLock }}</v-icon>
                                                    {{ $t('protectedRoom') }}
                                                </v-chip>
                                                <div class="room-entry__state" :class="room.isActive ? 'room-entry__state--active' : 'room-entry__state--idle'">
                                                    <span class="room-entry__state-dot"></span>
                                                    {{ room.isActive ? $t('active') : $t('inactive') }}
                                                </div>
                                            </div>
                                        </div>
                                        <v-list-item-subtitle class="room-entry__meta">
                                            {{ room.deviceCount || 0 }} {{ $t('devices') }} · {{ $t('messages') }} {{ room.messageCount || 0 }}
                                        </v-list-item-subtitle>
                                        <v-list-item-subtitle class="room-entry__activity">
                                            {{ $t('lastActive') }} · {{ formatTime(room.lastActive) }}
                                        </v-list-item-subtitle>
                                    </v-list-item-content>
                                    <v-list-item-action>
                                        <v-btn
                                            icon
                                            small
                                            @click.stop="toggleFavoriteRoom(room.name)"
                                            :color="room.isFavorite ? 'error' : ''"
                                        >
                                            <v-icon small>
                                                {{ room.isFavorite ? mdiHeart : mdiHeartOutline }}
                                            </v-icon>
                                        </v-btn>
                                    </v-list-item-action>
                                </v-list-item>
                            </v-list>
                        </section>
                    </div>
                </v-card-text>
            </v-card>
        </v-bottom-sheet>

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

.v-navigation-drawer >>> .v-navigation-drawer__border {
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
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 18px;
}

.room-browser__sections {
    display: grid;
    gap: 18px;
}

.room-browser--dock {
    position: sticky;
    top: 80px;
    flex: 0 0 332px;
    width: 332px;
    max-height: calc(100vh - 100px);
    border-radius: 24px;
    border: 1px solid rgba(148, 163, 184, 0.18);
    box-shadow: 0 24px 60px rgba(15, 23, 42, 0.14);
    overflow: hidden;
}

.room-group {
    display: grid;
    gap: 10px;
}

.room-group__label {
    font-size: 0.8rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: rgba(100, 116, 139, 0.95);
}

.app-shell--dark .room-group__label {
    color: rgba(148, 163, 184, 0.9);
}

.room-list {
    padding: 0;
    background: transparent !important;
}

.room-entry {
    border: 1px solid rgba(148, 163, 184, 0.22);
    border-radius: 18px;
    margin-bottom: 10px;
    padding: 10px 8px;
    background: rgba(248, 250, 252, 0.72);
    transition: transform 0.18s ease, border-color 0.18s ease, box-shadow 0.18s ease, background-color 0.18s ease;
}

.room-entry:hover {
    transform: translateY(-1px);
    border-color: rgba(59, 130, 246, 0.3);
    box-shadow: 0 12px 24px rgba(15, 23, 42, 0.06);
}

.room-entry--current {
    border-color: rgba(59, 130, 246, 0.38);
    background: rgba(239, 246, 255, 0.92);
}

.app-shell--dark .room-entry {
    border-color: rgba(71, 85, 105, 0.78);
    background: rgba(15, 23, 42, 0.7);
}

.app-shell--dark .room-entry--current {
    border-color: rgba(96, 165, 250, 0.52);
    background: rgba(30, 41, 59, 0.92);
}

.room-entry__avatar {
    border-radius: 14px;
    background: rgba(255, 255, 255, 0.7);
}

.app-shell--dark .room-entry__avatar {
    background: rgba(30, 41, 59, 0.8);
}

.room-entry__avatar--current {
    background: rgba(219, 234, 254, 0.9);
}

.app-shell--dark .room-entry__avatar--current {
    background: rgba(30, 64, 175, 0.24);
}

.room-entry__title-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 4px;
}

.room-entry__badges {
    display: inline-flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    flex-wrap: wrap;
}

.room-entry__name {
    font-size: 0.98rem;
    font-weight: 700;
    color: rgba(15, 23, 42, 0.96);
    word-break: break-word;
}

.app-shell--dark .room-entry__name {
    color: rgba(226, 232, 240, 0.96);
}

.room-entry__security-chip {
    color: #b45309 !important;
    border-color: rgba(217, 119, 6, 0.32) !important;
    background: rgba(245, 158, 11, 0.08) !important;
}

.app-shell--dark .room-entry__security-chip {
    color: #fbbf24 !important;
    border-color: rgba(251, 191, 36, 0.35) !important;
    background: rgba(245, 158, 11, 0.14) !important;
}

.room-entry__meta,
.room-entry__activity {
    color: rgba(100, 116, 139, 0.95) !important;
}

.app-shell--dark .room-entry__meta,
.app-shell--dark .room-entry__activity {
    color: rgba(148, 163, 184, 0.9) !important;
}

.room-entry__state {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    white-space: nowrap;
    font-size: 0.74rem;
    font-weight: 600;
}

.room-entry__state--active {
    color: #15803d;
}

.room-entry__state--idle {
    color: #64748b;
}

.app-shell--dark .room-entry__state--active {
    color: #86efac;
}

.app-shell--dark .room-entry__state--idle {
    color: #94a3b8;
}

.room-entry__state-dot {
    width: 8px;
    height: 8px;
    border-radius: 999px;
    background: currentColor;
    opacity: 0.9;
}

@media (max-width: 600px) {
    .room-browser__toolbar {
        flex-direction: column;
        align-items: stretch;
    }

    .room-entry__title-row {
        align-items: flex-start;
        flex-direction: column;
        gap: 4px;
    }
}

@media (max-width: 1263px) {
    .app-shell__workspace {
        display: block;
        padding: 0;
    }
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
    mdiChevronLeft,
    mdiChevronRight,
    mdiMagnify,
    mdiHome,
    mdiHomeOutline,
    mdiLock,
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
            roomDockVisible: true,
            roomDockSide: 'right',
            availableRooms: [],
            roomsLoading: false,
            roomListRefreshQueued: false,
            roomListRefreshSilent: true,
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
            mdiChevronLeft,
            mdiChevronRight,
            mdiMagnify,
            mdiHome,
            mdiHomeOutline,
            mdiLock,
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
        isDesktopRoomDockEnabled() {
            return !this.$vuetify.breakpoint.mdAndDown && this.$root.config && this.$root.config.server && this.$root.config.server.roomList;
        },
        isDesktopRoomDockVisible() {
            return this.isDesktopRoomDockEnabled && this.roomDockVisible;
        },
        filteredRooms() {
            let rooms = this.availableRooms.slice();
            if (this.roomSearch) {
                rooms = rooms.filter(room =>
                    (room.name || this.$t('publicRoom')).toLowerCase().includes(this.roomSearch.toLowerCase())
                );
            }
            return rooms.sort((a, b) => {
                if (a.isFavorite !== b.isFavorite) {
                    return b.isFavorite - a.isFavorite;
                }
                return 0;
            });
        },
        currentRoomEntry() {
            const currentRoomName = this.$root.room || '';
            return this.filteredRooms.find(room => room.name === currentRoomName) || this.createOptimisticRoom(currentRoomName);
        },
        favoriteRooms() {
            const currentRoomName = this.$root.room || '';
            return this.filteredRooms.filter(room => room.isFavorite && room.name !== currentRoomName);
        },
        activeRooms() {
            const currentRoomName = this.$root.room || '';
            return this.filteredRooms.filter(room => !room.isFavorite && room.isActive && room.name !== currentRoomName);
        },
        otherRooms() {
            const currentRoomName = this.$root.room || '';
            return this.filteredRooms.filter(room => !room.isFavorite && !room.isActive && room.name !== currentRoomName);
        },
        roomGroups() {
            return [
                {
                    key: 'favorites',
                    title: this.$t('favoriteRoomsLabel'),
                    rooms: this.favoriteRooms,
                },
                {
                    key: 'active',
                    title: this.$t('activeRoomsLabel'),
                    rooms: this.activeRooms,
                },
                {
                    key: 'other',
                    title: this.$t('otherRoomsLabel'),
                    rooms: this.otherRooms,
                },
            ].filter(group => group.rooms.length > 0);
        },
        sidebarRoomGroups() {
            return [
                {
                    key: 'favorites',
                    title: this.$t('favoriteRoomsLabel'),
                    rooms: this.favoriteRooms,
                },
                {
                    key: 'other',
                    title: this.$t('otherRoomsLabel'),
                    rooms: this.otherRooms.concat(this.activeRooms),
                },
            ].filter(group => group.rooms.length > 0);
        },
    },
    methods: {
        async submitRoomChange() {
            const roomName = this.$root.roomInput || '';
            this.ensureRoomPresent(roomName);
            this.$root.roomDialog = false;
            await this.$root.navigateToRoom(roomName);
        },
        createOptimisticRoom(roomName = this.$root.room || '') {
            if (roomName === undefined || roomName === null) {
                return null;
            }
            const normalizedRoomName = roomName || '';
            return {
                name: normalizedRoomName,
                isFavorite: this.getFavoriteRooms().includes(normalizedRoomName),
                isProtected: Boolean(this.$root.roomProtectionCache?.[normalizedRoomName]),
                isActive: true,
                messageCount: 0,
                deviceCount: 0,
                lastActive: Math.floor(Date.now() / 1000),
            };
        },
        ensureRoomPresent(roomName = this.$root.room || '') {
            const normalizedRoomName = roomName || '';
            if (this.availableRooms.some(room => room.name === normalizedRoomName)) {
                return;
            }
            this.availableRooms.unshift(this.createOptimisticRoom(normalizedRoomName));
        },
        syncAvailableRooms(rooms) {
            const nextRooms = Array.isArray(rooms) ? rooms : [];
            const existingByName = new Map(this.availableRooms.map(room => [room.name, room]));
            const orderedRooms = nextRooms.map(roomData => {
                const existing = existingByName.get(roomData.name);
                if (existing) {
                    Object.assign(existing, roomData);
                    return existing;
                }
                return roomData;
            });

            const currentRoomName = this.$root.room || '';
            if (!orderedRooms.some(room => room.name === currentRoomName)) {
                orderedRooms.unshift(existingByName.get(currentRoomName) || this.createOptimisticRoom(currentRoomName));
            }

            this.availableRooms.splice(0, this.availableRooms.length, ...orderedRooms);
            this.patchCurrentRoomStats();
        },
        patchCurrentRoomStats() {
            const currentRoomName = this.$root.room || '';
            const currentRoom = this.availableRooms.find(room => room.name === currentRoomName);
            if (!currentRoom) {
                return;
            }

            const localMessageCount = Array.isArray(this.$root.received) ? this.$root.received.length : 0;
            const localDeviceCount = (Array.isArray(this.$root.device) ? this.$root.device.length : 0) + (this.$root.websocket ? 1 : 0);
            const latestMessageTimestamp = localMessageCount > 0 ? Number(this.$root.received[0]?.timestamp || 0) : 0;
            const connectedTimestamp = this.$root.websocket ? Math.floor(Date.now() / 1000) : 0;

            currentRoom.messageCount = localMessageCount;
            currentRoom.deviceCount = localDeviceCount;
            currentRoom.isActive = localDeviceCount > 0;
            currentRoom.lastActive = Math.max(currentRoom.lastActive || 0, latestMessageTimestamp, connectedTimestamp);
        },
        openRoomBrowser() {
            if (this.isDesktopRoomDockEnabled) {
                this.roomDockVisible = true;
                this.persistRoomBrowserPreferences();
                this.ensureRoomPresent();
                this.fetchRoomList({ silent: false });
                return;
            }
            this.roomSheet = true;
            this.ensureRoomPresent();
            this.fetchRoomList({ silent: false });
        },
        hideDesktopRoomDock() {
            this.roomDockVisible = false;
            this.persistRoomBrowserPreferences();
        },
        toggleRoomDockSide() {
            this.roomDockSide = this.roomDockSide === 'right' ? 'left' : 'right';
            this.persistRoomBrowserPreferences();
        },
        persistRoomBrowserPreferences() {
            localStorage.setItem('roomDockVisible', String(this.roomDockVisible));
            localStorage.setItem('roomDockSide', this.roomDockSide);
        },
        restoreRoomBrowserPreferences() {
            const storedVisible = localStorage.getItem('roomDockVisible');
            const storedSide = localStorage.getItem('roomDockSide');
            this.roomDockVisible = storedVisible === null ? true : storedVisible === 'true';
            this.roomDockSide = storedSide === 'left' ? 'left' : 'right';
        },
        async clearAll() {
            try {
                await this.$http.delete('revoke/all', {
                    params: { room: this.$root.room },
                });
            } catch (error) {
                console.log(error);
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
                localStorage.setItem('locale', locale);
            }
        },
        goHome() {
            console.log('goHome triggered. Current route:', this.$route.fullPath);
            if (this.$route.path !== '/' || Object.keys(this.$route.query).length > 0) {
                 console.log('Navigating to / (Public Room)');
                 this.$router.push('/');
            } else {
                 console.log('Already on public room (/), not navigating.');
            }
        },
        async fetchRooms() {
            const candidateTokens = typeof this.$root.getKnownAuthTokens === 'function'
                ? this.$root.getKnownAuthTokens()
                : [];
            const dedupedTokens = Array.from(new Set(candidateTokens.map(token => (token || '').trim()).filter(Boolean)));
            const response = await this.$http.get('rooms', {
                headers: dedupedTokens.length ? {
                    'X-Room-Auth-Tokens': JSON.stringify(dedupedTokens),
                } : undefined,
                __skipRoomAuthHandling: true,
            });
            return Array.isArray(response.data?.rooms) ? response.data.rooms : [];
        },
        async fetchRoomList({ silent = false } = {}) {
            if (!this.$root.config || !this.$root.config.server || !this.$root.config.server.roomList) {
                console.log('房间列表功能未启用或配置未完成加载');
                return;
            }
            if (this.roomsLoading) {
                console.log('房间列表正在加载中，排队等待下一次刷新');
                this.roomListRefreshQueued = true;
                this.roomListRefreshSilent = this.roomListRefreshSilent && silent;
                return;
            }
            this.roomsLoading = true;
            this.roomListRefreshSilent = true;
            console.log('获取房间列表');
            try {
                const rooms = await this.fetchRooms();

                const favoriteRooms = this.getFavoriteRooms();
                this.syncAvailableRooms(rooms.map(room => ({
                    ...room,
                    isFavorite: favoriteRooms.includes(room.name)
                })));
                this.ensureRoomPresent();
                console.log(`房间列表更新成功，共 ${this.availableRooms.length} 个房间`);
            } catch (error) {
                console.error('Failed to fetch room list:', error);
                if (!silent) {
                    this.$toast(this.$t('failedToLoadRooms'));
                }
            } finally {
                this.roomsLoading = false;
                if (this.roomListRefreshQueued) {
                    const nextSilent = this.roomListRefreshSilent;
                    this.roomListRefreshQueued = false;
                    this.roomListRefreshSilent = true;
                    this.fetchRoomList({ silent: nextSilent });
                }
            }
        },
        async switchRoom(roomName) {
            this.roomSheet = false;
            await this.$root.navigateToRoom(roomName);
        },
        getFavoriteRooms() {
            try {
                return JSON.parse(localStorage.getItem('favoriteRooms') || '[]');
            } catch {
                return [];
            }
        },
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
            const room = this.availableRooms.find(r => r.name === roomName);
            if (room) {
                room.isFavorite = !room.isFavorite;
            }
        },
        getRoomDisplayName(room) {
            return room && room.name ? room.name : this.$t('publicRoom');
        },
        formatTime(timestamp) {
            if (!timestamp || timestamp === 0) return this.$t('never');
            const now = Math.floor(Date.now() / 1000);
            const messageTime = timestamp;
            const diff = now - messageTime;
            if (diff < 0) {
                return this.$t('justNow');
            }
            if (diff < 60) {
                return this.$t('justNow');
            } else if (diff < 3600) {
                return this.$t('minutesAgo', { minutes: Math.floor(diff / 60) });
            } else if (diff < 86400) {
                return this.$t('hoursAgo', { hours: Math.floor(diff / 3600) });
            } else {
                return this.$t('daysAgo', { days: Math.floor(diff / 86400) });
            }
        },
    },
    mounted() {
        this.restoreRoomBrowserPreferences();
        const darkPrimary = localStorage.getItem('darkPrimary');
        const lightPrimary = localStorage.getItem('lightPrimary');
        if (darkPrimary) {
            this.$vuetify.theme.themes.dark.primary = darkPrimary;
        }
        if (lightPrimary) {
            this.$vuetify.theme.themes.light.primary = lightPrimary;
        }
        this.$watch('$vuetify.theme.themes.dark.primary', (newVal) => {
            localStorage.setItem('darkPrimary', newVal);
        });
        this.$watch('$vuetify.theme.themes.light.primary', (newVal) => {
            localStorage.setItem('lightPrimary', newVal);
        });
        this.$watch(() => Boolean(this.$root.websocket), (connected) => {
            if (connected && this.$root.config && this.$root.config.server && this.$root.config.server.roomList) {
                this.fetchRoomList({ silent: true });
            }
        });
        this.$watch(() => this.$root.received.length, () => {
            this.patchCurrentRoomStats();
        });
        this.$watch(() => this.$root.device.length, () => {
            this.patchCurrentRoomStats();
        });
        this.$watch('isDesktopRoomDockVisible', (newVal) => {
            if (newVal) {
                this.ensureRoomPresent();
                this.fetchRoomList({ silent: false });
            }
        }, { immediate: true });
        console.log('App.vue mounted - 房间列表将在用户点击时获取');
    },
    watch: {
        '$route'() {
            this.clipboardClearedMessageVisible = false;
            if (this.$root.config && this.$root.config.server && this.$root.config.server.roomList) {
                this.ensureRoomPresent();
                this.patchCurrentRoomStats();
            }
        }
    }
};
</script>