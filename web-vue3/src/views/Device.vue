<template>
    <v-container>
        <v-responsive max-width="640" class="mx-auto">
            <div class="text-primary my-4">{{ t('connectedDevices') }}</div>
            <template v-if="ws.websocket">
                {{ t('devicesConnected', { count: app.device.length, desktop: desktopDeviceCount, mobile: mobileDeviceCount }) }}
                <v-divider class="my-2"></v-divider>
            </template>
            <template v-else>
                {{ t('notConnectedToServer') }}
            </template>

            <v-list rounded two-line>
                <v-list-item v-for="item in app.device" :key="item.id">
                    <template v-slot:prepend>
                        <v-icon v-if="item.type === 'desktop' && item.os.split(' ').shift() === 'Windows'">{{mdiMicrosoftWindows}}</v-icon>
                        <v-icon v-else-if="item.type === 'desktop' && item.os.split(' ').shift() === 'GNU/Linux'">{{mdiLinux}}</v-icon>
                        <v-icon v-else-if="item.type === 'desktop' && item.os.split(' ').shift() === 'Mac'">{{mdiApple}}</v-icon>
                        <v-icon v-else-if="item.type === 'desktop'">{{mdiLaptop}}</v-icon>
                        <v-icon v-else-if="(item.type === 'smartphone' || item.type === 'mobile' || item.type === 'tablet') && item.os.split(' ').shift() === 'Android'">{{mdiAndroid}}</v-icon>
                        <v-icon v-else-if="(item.type === 'smartphone' || item.type === 'mobile' || item.type === 'tablet') && item.os.split(' ').shift() === 'iOS'">{{mdiAppleIos}}</v-icon>
                        <v-icon v-else-if="item.type === 'smartphone' || item.type === 'mobile' || item.type === 'tablet'">{{mdiTabletCellphone}}</v-icon>
                        <v-icon v-else>{{mdiDevices}}</v-icon>
                    </template>
                    <v-list-item-title>{{
                        item.type === 'desktop' ? t('desktopDevice') : (
                            (item.type === 'smartphone' || item.type === 'mobile' || item.type === 'tablet') ? t('mobileDevice') : t('otherDevice')
                        )
                    }}</v-list-item-title>
                    <v-list-item-subtitle>{{item.os}} ({{item.browser}})</v-list-item-subtitle>
                </v-list-item>
            </v-list>
        </v-responsive>
    </v-container>
</template>

<script setup>import { computed } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useI18n } from 'vue-i18n';

const mdiAndroid = 'mdi-android';
const mdiApple = 'mdi-apple';
const mdiAppleIos = 'mdi-apple-ios';
const mdiDevices = 'mdi-devices';
const mdiLaptop = 'mdi-laptop';
const mdiLinux = 'mdi-linux';
const mdiMicrosoftWindows = 'mdi-microsoft-windows';
const mdiTabletCellphone = 'mdi-tablet-cellphone';


const app = useAppStore();
const ws = useWebSocketStore();
const { t } = useI18n();
const desktopDeviceCount = computed(() => app.device.filter(e => e.type === 'desktop').length);
const mobileDeviceCount = computed(() => app.device.filter(e => (e.type === 'smartphone' || e.type === 'tablet')).length);
</script>