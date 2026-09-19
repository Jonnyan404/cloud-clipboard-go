<script setup>
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useAppStore } from '@/store/app';
import QrcodeVue from 'qrcode.vue';
import { buildCleanAbsoluteRouteUrl, copyTextToClipboard } from '@/util.js';

const model = defineModel({ type: Boolean, default: false });
const { t } = useI18n();
const app = useAppStore();

const tab = ref('apple');

// 文件名与 shortcuts/apple/ 下的产物一一对应（由 scripts/sync-shortcuts.mjs 拷进 public）。
// 改捷径文件名时要同步这里。
const APPLE_SHORTCUTS = [
    { file: 'Cloud-Clipboard-Send-Text.shortcut', nameKey: 'scSendText', descKey: 'scSendTextDesc' },
    { file: 'Cloud-Clipboard-Send-File.shortcut', nameKey: 'scSendFile', descKey: 'scSendFileDesc' },
    { file: 'Cloud-Clipboard-Receive.shortcut', nameKey: 'scReceive', descKey: 'scReceiveDesc' },
    { file: 'Cloud-Clipboard-Receive-By-ID.shortcut', nameKey: 'scReceiveById', descKey: 'scReceiveByIdDesc' },
];

// HTTP Shortcuts 是第三方 App（本仓库只提供导入包）。三个官方渠道都在，任选其一 ——
// 改地址时记得三个一起看，别只留一个能用的。
const ANDROID_APP_LINKS = [
    { icon: 'mdi-google-play', label: 'Google Play', url: 'https://play.google.com/store/apps/details?id=ch.rmy.android.http_shortcuts' },
    { icon: 'mdi-android', label: 'F-Droid', url: 'https://f-droid.org/packages/ch.rmy.android.http_shortcuts/' },
    { icon: 'mdi-github', label: 'GitHub', url: 'https://github.com/Waboodoo/HTTP-Shortcuts/releases' },
];

const prefix = computed(() => app?.config?.server?.prefix || '');
const appleUrl = (file) => buildCleanAbsoluteRouteUrl(`shortcuts/apple/${file}`, prefix.value);
const androidUrl = () => buildCleanAbsoluteRouteUrl('shortcuts/android/shortcuts.zip', prefix.value);

// 二维码单独一个小对话框：手机扫码直接下载到设备，比在手机上敲地址省事。
const qrVisible = ref(false);
const qrUrl = ref('');
function showQr(url) {
    qrUrl.value = url;
    qrVisible.value = true;
}
</script>

<template>
    <v-dialog v-model="model" max-width="560" scrollable>
        <v-card class="shortcuts-dialog">
            <v-card-title class="d-flex align-center">
                <v-icon class="mr-2">mdi-flash</v-icon>{{ t('shortcuts') }}
            </v-card-title>
            <v-tabs v-model="tab" density="comfortable" color="primary">
                <v-tab value="apple">{{ t('shortcutsApple') }}</v-tab>
                <v-tab value="android">{{ t('shortcutsAndroid') }}</v-tab>
            </v-tabs>
            <v-divider></v-divider>

            <v-tabs-window v-model="tab">
                <v-tabs-window-item value="apple">
                    <v-card-text class="shortcuts-dialog__body">
                        <div class="text-caption text-medium-emphasis mb-3">{{ t('shortcutsHint') }}</div>
                        <v-card
                            v-for="sc in APPLE_SHORTCUTS"
                            :key="sc.file"
                            variant="outlined"
                            class="shortcuts-dialog__item mb-3 pa-3"
                        >
                            <div class="d-flex align-center mb-1">
                                <v-icon size="small" class="mr-2">mdi-apple</v-icon>
                                <span class="text-subtitle-2">{{ t(sc.nameKey) }}</span>
                            </div>
                            <div class="text-caption text-medium-emphasis mb-3">{{ t(sc.descKey) }}</div>
                            <div class="d-flex flex-wrap" style="gap: 6px;">
                                <v-btn size="small" variant="tonal" color="primary" :href="appleUrl(sc.file)" download>
                                    <v-icon start size="16">mdi-download</v-icon>{{ t('scDownload') }}
                                </v-btn>
                                <v-btn size="small" variant="text" @click="copyTextToClipboard(appleUrl(sc.file), 'copySuccess')">
                                    <v-icon start size="16">mdi-link-variant</v-icon>{{ t('scCopyLink') }}
                                </v-btn>
                                <v-btn size="small" variant="text" @click="showQr(appleUrl(sc.file))">
                                    <v-icon start size="16">mdi-qrcode</v-icon>{{ t('scShowQr') }}
                                </v-btn>
                            </div>
                        </v-card>
                        <v-alert type="info" variant="tonal" density="comfortable" class="mt-2">
                            <div class="text-caption">{{ t('scAppleSteps') }}</div>
                            <div class="text-caption mt-2 d-flex align-start">
                                <v-icon size="14" class="mr-1 mt-1">mdi-qrcode-scan</v-icon>
                                <span>{{ t('scAppleQrHint') }}</span>
                            </div>
                        </v-alert>
                    </v-card-text>
                </v-tabs-window-item>

                <v-tabs-window-item value="android">
                    <v-card-text class="shortcuts-dialog__body">
                        <div class="text-caption text-medium-emphasis mb-3">{{ t('shortcutsHint') }}</div>
                        <v-card variant="outlined" class="shortcuts-dialog__item mb-3 pa-3">
                            <div class="d-flex align-center mb-1">
                                <v-icon size="small" class="mr-2">mdi-android</v-icon>
                                <span class="text-subtitle-2">{{ t('scAndroidPackage') }}</span>
                            </div>
                            <div class="d-flex flex-wrap mt-3" style="gap: 6px;">
                                <v-btn size="small" variant="tonal" color="primary" :href="androidUrl()" download>
                                    <v-icon start size="16">mdi-download</v-icon>{{ t('scDownload') }}
                                </v-btn>
                                <v-btn size="small" variant="text" @click="copyTextToClipboard(androidUrl(), 'copySuccess')">
                                    <v-icon start size="16">mdi-link-variant</v-icon>{{ t('scCopyLink') }}
                                </v-btn>
                                <v-btn size="small" variant="text" @click="showQr(androidUrl())">
                                    <v-icon start size="16">mdi-qrcode</v-icon>{{ t('scShowQr') }}
                                </v-btn>
                            </div>
                        </v-card>
                        <v-card variant="outlined" class="shortcuts-dialog__item pa-3">
                            <div class="text-caption text-medium-emphasis mb-2">{{ t('scAndroidAppHint') }}</div>
                            <div class="d-flex flex-wrap" style="gap: 6px;">
                                <v-btn
                                    v-for="link in ANDROID_APP_LINKS"
                                    :key="link.url"
                                    size="small"
                                    variant="text"
                                    :href="link.url"
                                    target="_blank"
                                    rel="noopener"
                                >
                                    <v-icon start size="16">{{ link.icon }}</v-icon>{{ link.label }}
                                </v-btn>
                            </div>
                        </v-card>
                        <v-alert type="info" variant="tonal" density="comfortable" class="mt-3">
                            <div class="text-caption">{{ t('scAndroidSteps') }}</div>
                        </v-alert>
                    </v-card-text>
                </v-tabs-window-item>
            </v-tabs-window>
        </v-card>

        <v-dialog v-model="qrVisible" max-width="320">
            <v-card class="pa-4 text-center">
                <qrcode-vue :value="qrUrl" :size="240" level="M" />
                <div class="text-caption mt-3">{{ t('scQrHint') }}</div>
                <div class="text-caption text-medium-emphasis mt-1" style="word-break: break-all;">{{ qrUrl }}</div>
            </v-card>
        </v-dialog>
    </v-dialog>
</template>

<style scoped>
.shortcuts-dialog__body {
    max-height: 62vh;
    overflow-y: auto;
}

/* 卡片在暗色主题下用 on-surface 的透明度做描边，跟其他面板保持一致 */
.shortcuts-dialog__item {
    border-color: rgba(var(--v-theme-on-surface), 0.12);
}
</style>
