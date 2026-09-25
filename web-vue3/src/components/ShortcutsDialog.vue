<script setup>
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import QrcodeVue from 'qrcode.vue';
import { buildAppUrl, copyTextToClipboard } from '@/util.js';

const model = defineModel({ type: Boolean, default: false });
const { t } = useI18n();

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

// ⚠️★ 这三个地址**不能**用 `app.config.server.prefix`（2026-09-25 修，Issue #23 的同一类）。
//
// `prefix` 只能从 WebSocket 握手的 `config` 事件拿到，而**本组件的 `onMounted` 在应用挂载
// 时就跑了** —— `<shortcuts-dialog>` 在 `UnifiedComposer` 的模板里是**无条件渲染**的
// （`v-dialog` 的懒渲染只延后**内容**，不延后组件本身）。那时候 config 还没到、prefix 是空串，
// 于是 `meta.json` 被请求成 `/shortcuts/meta.json`，而 `/clip` 部署下正确地址是
// `/clip/shortcuts/meta.json` → **404**，还被下面的 catch 吞掉，
// 症状只是「那行『更新于』日期不见了」—— 极难查。
//
// 改用 `buildAppUrl()`（相对 `document.baseURI` 推导的应用基准目录，页面加载时就有），
// 与 `store/websocket.js` 的 `getWebSocketEndpoint()` 同一套办法。
const appleUrl = (file) => buildAppUrl(`shortcuts/apple/${file}`);
const androidUrl = () => buildAppUrl('shortcuts/android/shortcuts.zip');

// iOS 的二维码必须用 Shortcuts 的**导入 URL 方案**，不能直接放 .shortcut 的下载地址 ——
// 后者扫出来只是「下载了一个文件」，还得自己进「文件」App 找到再点开。
// 官方格式（Apple《使用 URL 方案导入快捷指令》）：
//   shortcuts://import-shortcut?url=<指向 .shortcut 的 URL>&name=<可选>&silent=<可选>
// url 参数要**整体百分号编码**：它自带 `://`，还可能带 `?auth=` 之类，
// 不编码的话其中的 `&` 会把外层查询串拆断。
// 不传 name，让 iOS 用文件名当快捷指令名（传了就是重命名，多语言下还会各叫各的）。
const appleImportUrl = (file) => `shortcuts://import-shortcut?url=${encodeURIComponent(appleUrl(file))}`;

// 产物的「更新于」日期。`meta.json` 由 scripts/sync-shortcuts.mjs 在 dev/build 时生成，
// 取的是 shortcuts/apple 与 shortcuts/android **最后一次提交的日期** —— 不是最新 commit，
// 免得无关提交也让日期往后跳（那会让旁边那句「请更新」变成没人看的噪音）。
// 拿不到就整个不显示这一行：提醒照常显示，下载照常可用。
// ⚠️ 这次 fetch 在 `onMounted` 里、**只发一次**，所以地址更不能依赖 `config`（见上面 `appleUrl` 那段）。
const meta = ref({ apple: null, android: null });
const activeDate = computed(() => (tab.value === 'apple' ? meta.value.apple : meta.value.android));
onMounted(async () => {
    try {
        const url = buildAppUrl('shortcuts/meta.json');
        const data = await (await fetch(url)).json();
        meta.value = { apple: data?.apple ?? null, android: data?.android ?? null };
    } catch {
        // 老版本产物里没有这个文件，或者离线 —— 两种都不该影响这个弹窗
    }
});

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

            <!-- 日期 + 「旧版请重新导入」。两个 tab 共用这一条，日期跟着当前 tab 走。
                 旧版的请求格式即将下线（见 docs/api.md），届时老版本会直接不可用，
                 所以这句要显眼、常驻，而不是塞在某个 tab 的角落里。 -->
            <div class="shortcuts-dialog__notice">
                <v-icon size="16" class="shortcuts-dialog__notice-icon">mdi-alert-circle-outline</v-icon>
                <span class="shortcuts-dialog__notice-text">{{ t('scOutdatedNotice') }}</span>
                <span v-if="activeDate" class="shortcuts-dialog__notice-date">{{ t('scUpdatedAt', { date: activeDate }) }}</span>
            </div>

            <v-tabs-window v-model="tab">
                <v-tabs-window-item value="apple">
                    <v-card-text class="shortcuts-dialog__body">
                        <!-- 平台图标先亮出来：这几条捷径 Mac 和 iPhone / iPad 用的是同一份文件，
                             不写清楚的话 Mac 用户会以为这是手机专用。
                             （上面那句 shortcutsHint 是两个 tab 共用的，不能在这里改文案。） -->
                        <div class="text-caption text-medium-emphasis mb-3 d-flex align-center flex-wrap">
                            <v-icon size="16" class="mr-1">mdi-apple</v-icon>
                            <v-icon size="16" class="mr-1">mdi-laptop</v-icon>
                            <v-icon size="16" class="mr-3">mdi-cellphone</v-icon>
                            <span>{{ t('shortcutsHint') }}</span>
                        </div>
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
/* 日期 + 「旧版请重新导入」。用「警告色的一圈淡底」而不是 v-alert：
   这是常驻提示，v-alert 的体积会把两个 tab 的内容都往下挤。 */
.shortcuts-dialog__notice {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: 4px 8px;
    padding: 8px 16px;
    font-size: 12px;
    line-height: 1.5;
    color: rgb(var(--v-theme-on-surface));
    background: rgba(var(--v-theme-warning), 0.12);
    border-bottom: 1px solid rgba(var(--v-theme-warning), 0.3);
}

.shortcuts-dialog__notice-icon {
    color: rgb(var(--v-theme-warning));
    align-self: center;
}

.shortcuts-dialog__notice-text {
    flex: 1;
    min-width: 12em;
}

.shortcuts-dialog__notice-date {
    flex-shrink: 0;
    opacity: 0.7;
    font-variant-numeric: tabular-nums;
}

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
