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

// iOS 的二维码必须用 Shortcuts 的**导入 URL 方案**，不能直接放 .shortcut 的下载地址 ——
// 后者扫出来只是「下载了一个文件」，还得自己进「文件」App 找到再点开。
// 官方格式（Apple《使用 URL 方案导入快捷指令》）：
//   shortcuts://import-shortcut?url=<指向 .shortcut 的 URL>&name=<可选>&silent=<可选>
// url 参数要**整体百分号编码**：它自带 `://`，还可能带 `?auth=` 之类，
// 不编码的话其中的 `&` 会把外层查询串拆断。
// 不传 name，让 iOS 用文件名当快捷指令名（传了就是重命名，多语言下还会各叫各的）。
const appleImportUrl = (file) => `shortcuts://import-shortcut?url=${encodeURIComponent(appleUrl(file))}`;

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
                            <div class="shortcuts-dialog__apple-row">
                                <div class="shortcuts-dialog__apple-main">
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
                                    </div>
                                </div>
                                <!-- 二维码直接摊在卡片里：扫码是这条路最主要的用法，
                                     藏在弹窗里等于每次都要多点一下才看到。
                                     码下面再写一行小字 —— 顶部那句提示会随内容滚走，
                                     而且 4 个码各配一句才不会认错。 -->
                                <div class="shortcuts-dialog__apple-qr">
                                    <qrcode-vue :value="appleImportUrl(sc.file)" :size="116" level="M" />
                                    <div class="text-caption text-medium-emphasis mt-1">{{ t('scScanToImport') }}</div>
                                </div>
                            </div>
                        </v-card>
                        <v-alert type="info" variant="tonal" density="comfortable" class="mt-2">
                            <div class="text-caption">{{ t('scAppleSteps') }}</div>
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
                            <div class="text-caption" style="white-space: pre-line;">{{ t('scAndroidSteps') }}</div>
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

/* Apple 卡片：左边名字/说明/按钮，右边二维码 */
.shortcuts-dialog__apple-row {
    display: flex;
    align-items: flex-start;
    gap: 16px;
}

.shortcuts-dialog__apple-main {
    flex: 1;
    /* 不加这条，flex 子项的最小宽度会取内容宽度，窄屏上二维码会把文字挤出去 */
    min-width: 0;
}

/* 二维码自带白底，并且这块白底同时充当**静区**（码四周必须留白，
   少了静区扫码成功率会明显掉）。深色主题下也必须保留 ——
   qrcode.vue 默认 foreground 是纯黑，落在深底上根本扫不出来。
   宽度写死 128 = 116 的码 + 左右各 6 的内边距：不写死的话，
   码下面那行小字会把盒子撑宽（中文比码宽），窄屏上左列就被挤没了。 */
.shortcuts-dialog__apple-qr {
    flex: none;
    width: 128px;
    box-sizing: border-box;
    padding: 6px;
    background: #fff;
    border-radius: 8px;
    /* canvas 是 inline 元素，行高会多垫几像素出来 */
    line-height: 0;
    text-align: center;
}

/* 上面把 line-height 归零了，码下面那行小字得自己恢复回来，否则会被压扁。
   这句中文比码宽，锁在一行会横向溢出，让它折成两行。 */
.shortcuts-dialog__apple-qr .text-caption {
    line-height: 1.4;
}
</style>
