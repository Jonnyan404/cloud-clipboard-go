<template>
    <v-card
        class="unified-composer"
        :class="{ 'unified-composer--dark': isDark, 'unified-composer--dragover': dragover }"
        variant="outlined"
        @dragenter.prevent="dragover = true"
        @dragover.prevent="dragover = true"
        @dragleave.prevent="handleDragLeave"
        @drop.prevent="handleDrop"
    >
        <div class="unified-composer__body pa-1 pa-md-3">
            <div
                class="unified-composer__inputs"
                :class="{ 'unified-composer__inputs--files-first': isFilePrimary }"
            >
                <div v-if="app.display.composerText" class="unified-composer__textblock">
                    <!-- `/` 模板菜单：行首打 `/` 弹出。放在文本区**上方**、走正常流 ——
                         这块是底部停靠的，多出来的高度往上长，输入框位置不动。
                         不用绝对定位：浮动元素一旦祖先有 overflow 就会被静默裁掉（踩过）。 -->
                    <composer-slash-menu v-if="slashMenu" :items="SLASH_TEMPLATES" @pick="insertSlashTemplate" />
                    <v-btn
                        icon
                        size="small"
                        density="comfortable"
                        variant="text"
                        color="grey-darken-1"
                        class="unified-composer__fullscreen-btn"
                        @click="toggleTextFullscreen"
                    >
                        <v-icon>{{ textFullscreen ? mdiFullscreenExit : mdiFullscreen }}</v-icon>
                    </v-btn>
                    <v-textarea
                        ref="textarea"
                        :key="app.composerPrimary"
                        v-model="app.send.text"
                        variant="solo"
                        flat
                        density="compact"
                        :rows="composerRows"
                        :placeholder="textareaPlaceholder"
                        hide-details
                        class="unified-composer__textarea"
                        :class="{ 'unified-composer__textarea--secondary': isFilePrimary }"
                        @keydown.ctrl.enter.prevent="onSendShortcut"
                        @keydown.meta.enter.prevent="onSendShortcut"
                        @keydown="onTextareaKeydown"
                        @input="onTextareaInput"
                        @compositionend="onTextareaInput"
                    ></v-textarea>
                </div>

                <div v-if="app.display.composerText && app.display.composerUpload" class="unified-composer__divider">
                    <div class="unified-composer__divider-line"></div>
                    <span class="unified-composer__limit text-caption text-medium-emphasis">{{ textLimitLabel }}</span>
                    <div class="unified-composer__divider-line unified-composer__divider-line--short"></div>
                    <v-tooltip v-if="app.display.composerSwap" location="top">
                        <template v-slot:activator="{ props }">
                            <v-btn
                                icon
                                size="small"
                                density="comfortable"
                                variant="text"
                                class="unified-composer__swapbtn"
                                v-bind="props"
                                @click="app.toggleComposerPrimary"
                            >
                                <v-icon>{{ mdiSwapVertical }}</v-icon>
                            </v-btn>
                        </template>
                        <span>{{ isFilePrimary ? t('textIsPrimaryTip') : t('fileIsPrimaryTip') }}</span>
                    </v-tooltip>
                    <div class="unified-composer__divider-line unified-composer__divider-line--short"></div>
                    <span class="unified-composer__limit text-caption text-medium-emphasis">{{ fileLimitLabel }}</span>
                    <div class="unified-composer__divider-line"></div>
                </div>

                <div v-if="app.display.composerUpload" class="unified-composer__fileblock">
                    <div
                        class="unified-composer__dropzone"
                        :class="{ 'unified-composer__dropzone--primary': isFilePrimary }"
                        @click="openFilePicker"
                    >
                        <v-icon :size="isFilePrimary ? 40 : 20" class="mr-2">{{ mdiCloudUpload }}</v-icon>
                        <span>{{ t(mobile ? 'addFilesShort' : 'addFiles', { keys: pasteKey }) }}</span>
                    </div>
                    <div v-if="app.send.files.length" class="unified-composer__attachments px-1 pt-2">
                        <v-chip
                            v-for="(file, index) in app.send.files"
                            :key="file.name + file.size + index"
                            closable
                            :variant="'outlined'"
                            size="small"
                            class="mr-2 mb-2"
                            @click:close="removeFile(index)"
                        >
                            {{ file.name }} · {{ prettyFileSize(file.size) }}
                        </v-chip>
                    </div>
                </div>
            </div>

            <div v-if="progress" class="px-1 px-md-3 pb-2">
                <small class="d-block text-right text-medium-emphasis mb-1">
                    {{ prettyFileSize(Math.min(uploadedSize, fileSize)) }} / {{ prettyFileSize(fileSize) }}
                </small>
                <v-progress-linear :value="uploadProgress * 100"></v-progress-linear>
            </div>
        </div>

        <div class="unified-composer__footer pt-1">
                <div class="unified-composer__footer-icons">
                    <v-tooltip v-if="app.display.composerDevice" location="top">
                        <template v-slot:activator="{ props }">
                            <v-btn
                                variant="text"
                                size="small"
                                density="comfortable"
                                color="grey-darken-1"
                                v-bind="props"
                                class="unified-composer__device"
                                @click="goDeviceList"
                            >
                                <span class="unified-composer__device-full">
                                    <span class="unified-composer__devicestat"><v-icon size="small" class="mr-1">{{ mdiLaptop }}</v-icon>{{ deviceStats.desktop }}</span>
                                    <span class="unified-composer__devicestat"><v-icon size="small" class="mr-1">{{ mdiCellphone }}</v-icon>{{ deviceStats.mobile }}</span>
                                    <span class="unified-composer__devicestat"><v-icon size="small" class="mr-1">{{ mdiDevices }}</v-icon>{{ deviceStats.other }}</span>
                                </span>
                            </v-btn>
                        </template>
                        <span>{{ t('connectedTotal', { count: deviceTotal }) }}</span>
                    </v-tooltip>
                    <div v-if="app.display.composerReward" class="unified-composer__footer-reward">
                        <v-tooltip location="top">
                            <template v-slot:activator="{ props }">
                                <v-btn
                                    icon
                                    density="comfortable"
                                    variant="text"
                                    size="small"
                                    v-bind="props"
                                    @click="rewardDialog = true"
                                >
                                    <v-icon class="unified-composer__reward-icon">{{ mdiCurrencyCny }}</v-icon>
                                </v-btn>
                            </template>
                            <span>{{ t('reward') }}</span>
                        </v-tooltip>
                    </div>
                    <div class="unified-composer__footer-main">
                        <v-tooltip v-if="app.display.composerPalette" location="top">
                            <template v-slot:activator="{ props }">
                                <v-btn
                                    icon
                                    density="comfortable"
                                    variant="text"
                                    size="small"
                                    color="grey-darken-1"
                                    v-bind="props"
                                    @click="colorDialog = true"
                                >
                                    <v-icon>{{ mdiPaletteSwatch }}</v-icon>
                                </v-btn>
                            </template>
                            <span>{{ t('traditionalColors') }}</span>
                        </v-tooltip>
                        <v-tooltip v-if="app.display.composerShortcuts" location="top">
                            <template v-slot:activator="{ props }">
                                <v-btn
                                    icon
                                    density="comfortable"
                                    variant="text"
                                    size="small"
                                    color="grey-darken-1"
                                    v-bind="props"
                                    @click="shortcutsDialog = true"
                                >
                                    <v-icon>{{ mdiFlash }}</v-icon>
                                </v-btn>
                            </template>
                            <span>{{ t('shortcuts') }}</span>
                        </v-tooltip>
                        <v-tooltip v-if="app.display.composerTheme" location="top">
                            <template v-slot:activator="{ props }">
                                <v-btn
                                    icon
                                    density="comfortable"
                                    variant="text"
                                    size="small"
                                    color="grey-darken-1"
                                    v-bind="props"
                                    @click="toggleDark"
                                >
                                    <v-icon>{{ isDark ? mdiWhiteBalanceSunny : mdiWeatherNight }}</v-icon>
                                </v-btn>
                            </template>
                            <span>{{ t('toggleDarkMode') }}</span>
                        </v-tooltip>
                    </div>
                </div>

                <v-btn
                    variant="flat"
                    color="primary"
                    class="unified-composer__send"
                    :disabled="sendDisabled"
                    @click="sendAll"
                 v-if="canSend">
                    <v-icon start size="small">{{ mdiSend }}</v-icon>
                    {{ t('send') }}
                </v-btn>
            </div>

        <input
            ref="selectFile"
            type="file"
            class="d-none"
            multiple
            @change="handleSelectFiles(Array.from($event.target.files))"
        >
    </v-card>

    <v-dialog v-model="deviceDialog" max-width="480" scrollable>
        <v-card>
            <v-card-title class="d-flex align-center">
                <v-icon start>{{ mdiDevices }}</v-icon>
                {{ t('connectedDevices') }}
                <v-spacer></v-spacer>
                <v-btn icon variant="text" @click="deviceDialog = false">
                    <v-icon>{{ mdiClose }}</v-icon>
                </v-btn>
            </v-card-title>
            <v-divider></v-divider>
            <v-card-text class="pa-2">
                <template v-if="!ws.websocket">
                    <p class="pa-2 text-medium-emphasis">{{ t('notConnectedToServer') }}</p>
                </template>
                <template v-else-if="app.device.length === 0">
                    <p class="pa-2 text-medium-emphasis">{{ t('noDevicesConnected') }}</p>
                </template>
                <template v-else>
                    <p class="pa-2 text-caption text-medium-emphasis">
                        {{ t('devicesConnected', { count: app.device.length, desktop: desktopDeviceCount, mobile: mobileDeviceCount }) }}
                    </p>
                    <v-list rounded two-line density="compact">
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
                            <v-list-item-title>{{ item.name || deviceTypeLabel(item) }}</v-list-item-title>
                            <v-list-item-subtitle>{{item.os}} ({{item.browser}})</v-list-item-subtitle>
                        </v-list-item>
                    </v-list>
                </template>
            </v-card-text>
        </v-card>
    </v-dialog>

    <v-dialog v-model="textFullscreen" fullscreen>
        <v-card class="d-flex flex-column fill-height pts-fullscreen-card">
            <v-toolbar elevation="1">
                <v-btn icon variant="text" @click="textFullscreen = false">
                    <v-icon>{{ mdiArrowLeft }}</v-icon>
                </v-btn>
                <v-toolbar-title>{{ t('enterTextToSend') }}</v-toolbar-title>
                <v-spacer></v-spacer>
                <v-tooltip location="bottom">
                    <template v-slot:activator="{ props }">
                        <v-btn
                            icon
                            variant="text"
                            v-bind="props"
                            :color="app.fullscreenSendClose ? 'primary' : 'grey-darken-1'"
                            @click="app.toggleFullscreenSendClose"
                        >
                            <v-icon>{{ app.fullscreenSendClose ? mdiChevronDownCircle : mdiWindowRestore }}</v-icon>
                        </v-btn>
                    </template>
                    <span>{{ app.fullscreenSendClose ? t('fullscreenCloseAfterSendOn') : t('fullscreenCloseAfterSendOff') }}</span>
                </v-tooltip>
                <v-btn
                    variant="flat"
                    color="primary"
                    :disabled="sendDisabled"
                    @click="sendAll"
                 v-if="canSend">
                    <v-icon start size="small">{{ mdiSend }}</v-icon>
                    {{ t('send') }}
                </v-btn>
            </v-toolbar>
            <div class="pts-fullscreen-body flex-grow-1">
                <composer-slash-menu v-if="slashMenu" :items="SLASH_TEMPLATES" @pick="insertSlashTemplate" />
                <v-textarea
                    v-model="app.send.text"
                    variant="solo"
                    flat
                    hide-details
                    no-resize
                    class="pts-fullscreen-textarea"
                    :placeholder="textareaPlaceholder"
                    @keydown.ctrl.enter.prevent="onSendShortcut"
                    @keydown.meta.enter.prevent="onSendShortcut"
                    @keydown="onTextareaKeydown"
                    @input="onTextareaInput"
                    @compositionend="onTextareaInput"
                ></v-textarea>
                <small class="d-flex justify-center pa-2 text-medium-emphasis">{{ textLimitLabel }}</small>
            </div>
        </v-card>
    </v-dialog>

    <v-dialog v-model="rewardDialog" max-width="420">
        <v-card>
            <v-card-title class="text-h6 d-flex align-center">
                <v-icon class="mr-2 unified-composer__reward-icon">{{ mdiCurrencyCny }}</v-icon>
                {{ t('rewardTitle') }}
                <v-spacer></v-spacer>
                <v-btn icon density="comfortable" variant="text" size="small" @click="rewardDialog = false">
                    <v-icon>{{ mdiClose }}</v-icon>
                </v-btn>
            </v-card-title>
            <v-divider></v-divider>
            <v-card-text class="text-center pa-4">
                <div class="text-body-2 font-weight-medium unified-composer__reward-section-title mb-2">{{ t('supportSectionTitle') }}</div>
                <v-row class="unified-composer__reward-row" dense>
                    <v-col class="text-center">
                        <div class="unified-composer__reward-label">微信</div>
                        <img src="/reward-wechat.png" alt="WeChat Reward QR" class="unified-composer__reward-qr" />
                    </v-col>
                    <v-col class="text-center">
                        <div class="unified-composer__reward-label">支付宝</div>
                        <img src="/reward-alipay.png" alt="Alipay Reward QR" class="unified-composer__reward-qr" />
                    </v-col>
                </v-row>
                <div class="text-body-2 text-medium-emphasis mt-3 unified-composer__warm-text">{{ t('rewardHint') }}</div>
                <v-btn class="mt-3" color="#ff5f5f" variant="tonal" block
                       href="https://ko-fi.com/jonnyan404"
                       target="_blank" rel="noopener">
                    <v-icon start>{{ mdiCoffee }}</v-icon>
                    <span>Buy Me a Coffee</span>
                    <v-icon end size="16">{{ mdiOpenInNew }}</v-icon>
                </v-btn>
                <v-divider class="my-4"></v-divider>
                <div class="unified-composer__warm-box">
                    <div class="text-body-2 text-medium-emphasis unified-composer__warm-text">{{ t('cloudPromoHint') }}</div>
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
        </v-card>
    </v-dialog>

    <traditional-color-dialog v-model="colorDialog"></traditional-color-dialog>
    <shortcuts-dialog v-model="shortcutsDialog"></shortcuts-dialog>
</template>

<script setup>import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useDisplay } from 'vuetify';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { toast } from '@/plugins/toast';
import { errorMessage, prettyFileSize } from '@/util.js';
import TraditionalColorDialog from '@/components/TraditionalColorDialog.vue';
import ComposerSlashMenu from '@/components/ComposerSlashMenu.vue';
import { SLASH_TEMPLATES, resolveSlashText, slashMenuShouldOpen, slashMenuShouldStay, slashPendingAt, stripTrailingSlash } from '@/slash-template.js';
import ShortcutsDialog from '@/components/ShortcutsDialog.vue';

const mdiPalette = 'mdi-palette';
const mdiPaletteSwatch = 'mdi-palette-swatch';
const mdiFlash = 'mdi-flash';
const mdiSend = 'mdi-send';
const mdiLaptop = 'mdi-laptop';
const mdiCellphone = 'mdi-cellphone';
const mdiDevices = 'mdi-devices';
const mdiCurrencyCny = 'mdi-currency-cny';
const mdiCoffee = 'mdi-coffee';
const mdiOpenInNew = 'mdi-open-in-new';
const mdiWhiteBalanceSunny = 'mdi-white-balance-sunny';
const mdiWeatherNight = 'mdi-weather-night';
const mdiClose = 'mdi-close';
const mdiAndroid = 'mdi-android';
const mdiApple = 'mdi-apple';
const mdiAppleIos = 'mdi-apple-ios';
const mdiLinux = 'mdi-linux';
const mdiMicrosoftWindows = 'mdi-microsoft-windows';
const mdiTabletCellphone = 'mdi-tablet-cellphone';
const mdiSwapVertical = 'mdi-swap-vertical';
const mdiCloudUpload = 'mdi-cloud-upload-outline';
const mdiFullscreen = 'mdi-fullscreen';
const mdiFullscreenExit = 'mdi-fullscreen-exit';
const mdiArrowLeft = 'mdi-arrow-left';
const mdiChevronDownCircle = 'mdi-chevron-down-circle';
const mdiWindowRestore = 'mdi-window-restore';


const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();
const { mobile } = useDisplay();
const isFilePrimary = computed(() => app.composerPrimary === 'files');
const composerRows = computed(() => isFilePrimary.value ? 1 : 3);
const deviceDialog = ref(false);
const rewardDialog = ref(false);
const colorDialog = ref(false);
const shortcutsDialog = ref(false);
const textFullscreen = ref(false);
function toggleTextFullscreen() {
    textFullscreen.value = !textFullscreen.value;
}
const deviceStats = computed(() => {
    const list = app.device || [];
    const desktop = list.filter(d => d.type === 'desktop').length;
    const mobile = list.filter(d => d.type === 'smartphone' || d.type === 'mobile' || d.type === 'tablet').length;
    const other = list.length - desktop - mobile;
    return { desktop, mobile, other };
});
const deviceTotal = computed(() => deviceStats.value.desktop + deviceStats.value.mobile + deviceStats.value.other);
const desktopDeviceCount = computed(() => app.device.filter(e => e.type === 'desktop').length);
const mobileDeviceCount = computed(() => app.device.filter(e => (e.type === 'smartphone' || e.type === 'tablet')).length);
// 设备没自报名字时的兜底标题（服务端对未声明的名字会 omitempty 掉，所以这条路径真的会走到）。
//
// ⚠️ 这个函数在模板里被调用，但一直**没有定义**。JS 的 || 短路让它只在
// 「有设备、且至少一台没名字」时才被求值 —— 那时整块列表渲染抛 TypeError，
// 弹窗直接挂不上，表现就是「点了没反应」，而控制台之外看不出任何异常。
function deviceTypeLabel(item) {
    if (item.type === 'desktop') {
        return t('desktopDevice');
    }
    if (item.type === 'smartphone' || item.type === 'mobile' || item.type === 'tablet') {
        return t('mobileDevice');
    }
    return t('otherDevice');
}
function goDeviceList() {
    deviceDialog.value = true;
}
function toggleDark() {
    app.dark = app.useDark ? 'disable' : 'enable';
}
defineExpose({ focus, openFilePicker });
const progress = ref(false);
const dragover = ref(false);
const uploadedSizes = ref([]);
const isMac = /mac|iphone|ipad|ipod/i.test(navigator.userAgent || '');
const textarea = ref(null);
const selectFile = ref(null);
const fileSize = computed(() => app.send.files.length ? app.send.files.reduce((acc, cur) => acc += cur.size, 0) : 0);
const uploadedSize = computed(() => uploadedSizes.value.length ? uploadedSizes.value.reduce((acc, cur) => acc += cur, 0) : 0);
const uploadProgress = computed(() => Math.min(fileSize.value !== 0 ? (uploadedSize.value / fileSize.value) : 0, 1));
const sendDisabled = computed(() => !ws.websocket || progress.value || (!app.send.text && !app.send.files.length) || app.send.text.length > app.config.text.limit);
const pasteKey = isMac ? '⌘+V' : 'Ctrl+V';
const sendShortcutLabel = computed(() => t('sendShortcutTip', {
    keys: isMac ? '⌘+Enter' : 'Ctrl+Enter',
}));
const textareaPlaceholder = computed(() => {
    // 「/」模板是这套输入区里最不容易被发现的功能，直接写在占位符里。
    const hint = t('composerSlashHint');
    return mobile.value ? hint : `${hint} ${sendShortcutLabel.value}`;
});
// 文本区和上传区都被关掉时，发送按钮没有任何东西可发 —— 藏起来，
// 而不是留一个点了没反应的按钮。（两边都关 = 这个模式只想接收。）
const canSend = computed(() => Boolean(app.display.composerText || app.display.composerUpload));

const textLimitLabel = computed(() => t('composerTextLimit', {
    current: app.send.text.length,
    limit: app.config.text.limit,
}));
const fileLimitLabel = computed(() => t('fileSizeLimit', {
    limit: prettyFileSize(app.config.file.limit),
}));
function focus(type) {
    if (type === 'file') {
        openFilePicker();
        return;
    }
    if (textarea.value && typeof textarea.value.focus === 'function') {
        textarea.value.focus();
    }
}
function openFilePicker() {
    // 上传开关关掉时这个 input 会被 v-if 摘掉，ref 就是 null —— 不能裸点
    selectFile.value?.click();
}

// 「/」快捷方式：在**行首**打 `/` 弹出 markdown 模板菜单。
//
// 为什么限定行首：正文里 `/` 太常见了（路径、日期、`a/b`），到处弹菜单会烦人；
// 行首打 `/` 是个明确的开头动作。缩进过的行（前面只有空白）也算行首。
//
// 判定与模板都在 `slash-template.js`：这个组件和 StickyComposer 共用一份。
const slashMenu = ref(false);
let slashEl = null;

function onTextareaKeydown(e) {
    if (e.key === 'Escape') {
        if (slashMenu.value) {
            slashMenu.value = false;
            e.stopPropagation();
        }
        return;
    }
    if (e.key !== '/') return;
    // 硬件键盘（桌面）：这里的 `/` 还没落进文本，判定点在光标当前位置。
    // 屏幕键盘不保证能走到这里 —— 手机上靠下面的 onTextareaInput。
    if (!slashPendingAt(e.target, app.send.text)) return;
    slashEl = e.target;
    slashMenu.value = true;
}

// 手机上 `/` **只有** `input` 事件看得见（原因见 slash-template.js）：这里既负责在刚打完
// 行首 `/` 时把菜单弹出来，也负责继续敲别的（正文里的路径、日期）时收起来。
//
// 顺带补上桌面缺的一半：以前只靠 keydown 弹、没人收 —— 菜单弹出后继续打字不会消失。
function onTextareaInput(e) {
    if (!slashMenu.value) {
        if (!slashMenuShouldOpen(e, app.send.text)) return;
        slashEl = e.target;
        slashMenu.value = true;
        return;
    }
    if (!slashMenuShouldStay(e, app.send.text)) {
        slashMenu.value = false;
    }
}

async function insertSlashTemplate(tpl) {
    const el = slashEl;
    const text = app.send.text || '';
    // ⚠️ 光标位置要在 await **之前**读 —— 要插入的文本可能是动作算出来的（异步），
    // 等回来时光标未必还在原处。
    const pos = el && typeof el.selectionStart === 'number' ? el.selectionStart : text.length;
    // 连同刚打的那个 `/` 一起换掉（如果它还在光标前）
    const head = stripTrailingSlash(text.slice(0, pos));
    const tail = text.slice(pos);
    // 模板项直接给文本；动作项（插入时间 / UUID）在**这一刻**才算 —— 时间是「现在」的
    const insert = await resolveSlashText(tpl);
    app.send.text = head + insert + tail;
    slashMenu.value = false;
    nextTick(() => {
        const caret = head.length + insert.length;
        el?.focus?.();
        el?.setSelectionRange?.(caret, caret);
    });
}
function onSendShortcut() {
    if (!sendDisabled.value) {
        sendAll();
    }
}
function removeFile(index) {
    app.send.files.splice(index, 1);
}
function handleSelectFiles(files) {
    if (!files.length) {
        return;
    }
    if (files.some(file => !file.size)) {
        toast(t('cannotSendEmptyFile'));
        return;
    }
    if (files.some(file => file.size > app.config.file.limit)) {
        toast(t('fileSizeExceeded', { limit: prettyFileSize(app.config.file.limit) }));
        return;
    }
    app.send.files.splice(0);
    app.send.files.push(...files);
}
async function sendText() {
    if (!app.send.text) {
        return;
    }
    await axios.post(
        'text',
        app.send.text,
        {
            params: new URLSearchParams([['room', ws.room]]),
            headers: {
                'Content-Type': 'text/plain',
            },
        },
    );
    app.send.text = '';
}
async function sendFiles() {
    if (!app.send.files.length) {
        return;
    }
    const chunkSize = app.config.file.chunk;
    uploadedSizes.value.splice(0);
    uploadedSizes.value.push(...Array(app.send.files.length).fill(0));
    progress.value = true;
    await Promise.all(app.send.files.map(async (file, index) => {
        if (file.size < chunkSize) {
            const formData = new FormData;
            formData.set('file', file);
            await axios.postForm('upload', formData, {
                params: new URLSearchParams([['room', ws.room]]),
                onUploadProgress: event => uploadedSizes.value[index] = event.loaded,
            });
            return;
        }
        const response = await axios.post('upload/chunk', file.name, {
            headers: { 'Content-Type': 'text/plain' },
            params: new URLSearchParams([['room', ws.room]]),
        });
        const uuid = response.data.result.uuid;
        let uploadedSize = 0;
        while (uploadedSize < file.size) {
            const chunk = file.slice(uploadedSize, uploadedSize + chunkSize);
            await axios.post(`upload/chunk/${uuid}`, chunk, {
                headers: { 'Content-Type': 'application/octet-stream' },
                onUploadProgress: event => uploadedSizes.value[index] = uploadedSize + event.loaded,
            });
            uploadedSize += chunkSize;
        }
        await axios.post(`upload/finish/${uuid}`, null, {
            params: new URLSearchParams([['room', ws.room]]),
        });
    }));
    app.send.files.splice(0);
}
async function sendAll() {
    try {
        if (app.send.text) {
            await sendText();
        }
        if (app.send.files.length) {
            await sendFiles();
        }
        toast(t('sendSuccess'));
        if (app.fullscreenSendClose) {
            textFullscreen.value = false;
        }
        focus();
    } catch (error) {
        const errMsg = errorMessage(error);
        if (errMsg) {
            toast(t('sendFailedMsg', { msg: errMsg }));
        } else {
            toast(t('sendFailed'));
        }
    } finally {
        progress.value = false;
    }
}
function handleDragLeave(event) {
    if (event.currentTarget.contains(event.relatedTarget)) {
        return;
    }
    dragover.value = false;
}
function handleDrop(event) {
    dragover.value = false;
    if (!(event && event.dataTransfer)) {
        return;
    }
    const files = Array.from(event.dataTransfer.files || []);
    if (files.length) {
        handleSelectFiles(files);
    }
}
function handlePaste(event) {
    if (!(event && event.clipboardData)) {
        return;
    }
    const items = Array.from(event.clipboardData.items || []);
    const files = items.filter(item => item.kind === 'file').map(item => item.getAsFile()).filter(Boolean);
    if (files.length) {
        handleSelectFiles(files);
    }
}
onMounted(() => {
    document.addEventListener('paste', handlePaste);
    nextTick(() => {
        focus();
    });
});
onBeforeUnmount(() => {
    document.removeEventListener('paste', handlePaste);
});
</script>

<style scoped>
.unified-composer {
    border-radius: 22px;
    border-color: rgba(148, 163, 184, 0.22) !important;
    box-shadow: 0 10px 28px rgba(15, 23, 42, 0.06);
    background: rgba(255, 255, 255, 0.96);
    transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
    display: flex;
    flex-direction: column;
    max-height: calc(100vh - 6.5rem);
    max-height: calc(100dvh - 6.5rem);
    min-height: 0;
}

@media (max-width: 1263px) {
    .unified-composer {
        max-height: calc(100vh - 4.5rem);
        max-height: calc(100dvh - 4.5rem);
    }
}

.unified-composer--dark {
    border-color: rgba(71, 85, 105, 0.72) !important;
    box-shadow: 0 18px 36px rgba(2, 6, 23, 0.3);
    background: rgba(15, 23, 42, 0.94);
}

.unified-composer--dragover {
    border-color: var(--v-primary-base, #1976d2) !important;
    box-shadow: 0 0 0 2px var(--v-primary-base, #1976d2);
}

.unified-composer--dragover * {
    pointer-events: none;
}

.unified-composer__textarea :deep(.v-input__slot) {
    box-shadow: none !important;
    border-radius: 16px;
    background: rgba(248, 250, 252, 0.95) !important;
    padding: 0.25rem 0.25rem 0 0.25rem;
}

.unified-composer--dark .unified-composer__textarea :deep(.v-input__slot) {
    background: rgba(30, 41, 59, 0.96) !important;
}

.unified-composer--dark .unified-composer__textarea :deep(textarea),
.unified-composer--dark .unified-composer__limit,
.unified-composer--dark .unified-composer__attachments {
    color: rgba(226, 232, 240, 0.92) !important;
}

.unified-composer__textarea :deep(textarea) {
    resize: none;
    height: 80px;
    max-height: 40vh;
    overflow-y: auto;
}

.unified-composer__textarea--secondary :deep(textarea) {
    resize: none;
    height: 42px;
    max-height: 6rem;
}

.unified-composer__textblock {
    position: relative;
}

.unified-composer__fullscreen-btn {
    position: absolute;
    top: 4px;
    right: 4px;
    z-index: 1;
    background: rgba(255, 255, 255, 0.9);
    border-radius: 50%;
}

.unified-composer__fullscreen-btn :deep(.v-btn__overlay) {
    background: transparent;
}

.unified-composer--dark .unified-composer__fullscreen-btn {
    background: rgba(30, 41, 59, 0.9);
}

.unified-composer__fullscreen-btn :deep(.v-icon),
.unified-composer__fullscreen-btn :deep(.v-btn__content) {
    opacity: 0.55;
}

.unified-composer__textblock:hover .unified-composer__fullscreen-btn :deep(.v-icon) {
    opacity: 1;
}

.pts-fullscreen-body {
    display: flex;
    flex-direction: column;
    padding: 1rem;
    min-height: 0;
}

.pts-fullscreen-textarea {
    flex: 1 1 auto;
    min-height: 0;
}

.pts-fullscreen-textarea :deep(.v-field--solo),
.pts-fullscreen-textarea :deep(.v-field__input),
.pts-fullscreen-textarea :deep(textarea) {
    height: 100% !important;
}

.pts-fullscreen-textarea :deep(.v-field__input) {
    overflow-y: auto;
}

.unified-composer__inputs {
    display: flex;
    flex-direction: column;
}

.unified-composer__inputs--files-first .unified-composer__textblock {
    order: 3;
}

.unified-composer__inputs--files-first .unified-composer__divider {
    order: 2;
}

.unified-composer__inputs--files-first .unified-composer__fileblock {
    order: 1;
}

.unified-composer__textarea--secondary {
    max-height: 3.5rem;
}

.unified-composer__divider {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 0.25rem 0;
}

.unified-composer__limit {
    flex-shrink: 0;
    white-space: nowrap;
}

.unified-composer__divider-line {
    flex: 1;
    min-width: 0;
    height: 1px;
    background: rgba(148, 163, 184, 0.35);
}

.unified-composer__divider-line--short {
    flex: 0 0 1.5rem;
}

.unified-composer--dark .unified-composer__divider-line {
    background: rgba(71, 85, 105, 0.6);
}

.unified-composer__swapbtn {
    flex-shrink: 0;
}

.unified-composer__fileblock {
    min-width: 0;
}

.unified-composer__dropzone {
    display: flex;
    align-items: center;
    justify-content: center;
    border: 1px dashed rgba(148, 163, 184, 0.55);
    border-radius: 16px;
    color: rgba(100, 116, 139, 0.95);
    cursor: pointer;
    min-height: 2.5rem;
    padding: 0.25rem 0.5rem;
    transition: border-color 0.2s ease, background 0.2s ease;
}

.unified-composer--dark .unified-composer__dropzone {
    border-color: rgba(71, 85, 105, 0.65);
    color: rgba(203, 213, 225, 0.85);
}

.unified-composer__dropzone:hover {
    border-color: var(--v-theme-primary);
    background: rgba(99, 102, 241, 0.06);
}

.unified-composer__dropzone--primary {
    min-height: 7rem;
    flex-direction: column;
    gap: 0.25rem;
}

.unified-composer__attachments {
    min-height: 1.5rem;
}

.unified-composer__body {
    flex: 1 1 auto;
    min-height: 0;
    overflow: hidden;
}

.unified-composer__footer {
    flex-shrink: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    align-items: center;
    gap: 0.75rem;
    border-top: 1px solid rgba(226, 232, 240, 0.9);
    padding: 0.25rem 0.25rem 0.25rem 0.25rem;
}

.unified-composer__footer-icons {
    grid-column: 2;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.15rem 0.5rem;
    min-width: 0;
}

@media (min-width: 960px) {
    .unified-composer__footer {
        padding: 0.75rem;
        padding-top: 0.5rem;
    }
}

.unified-composer--dark .unified-composer__footer {
    border-top-color: rgba(71, 85, 105, 0.72);
}

.unified-composer__footer-main {
    min-width: 0;
}

.unified-composer__footer-reward {
    display: flex;
    justify-content: center;
    min-width: 0;
}

.unified-composer__device {
    margin-inline-start: 2px;
    height: 28px;
    padding: 0 6px;
}

.unified-composer__device-full {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    font-variant-numeric: tabular-nums;
}

.unified-composer__devicestat {
    display: inline-flex;
    align-items: center;
}

.unified-composer__shortcut {
    white-space: nowrap;
}

.unified-composer__reward-icon {
    color: #f5b301;
    animation: unified-composer-reward-shine 2s ease-in-out infinite;
}

@keyframes unified-composer-reward-shine {
    0%, 100% {
        filter: brightness(1);
        text-shadow: 0 0 0 rgba(245, 179, 1, 0);
    }
    50% {
        filter: brightness(1.45);
        text-shadow: 0 0 6px rgba(245, 179, 1, 0.85);
    }
}

.unified-composer__reward-row {
    justify-content: center;
}

.unified-composer__reward-label {
    font-size: 12px;
    color: rgba(0, 0, 0, 0.6);
    margin-bottom: 4px;
}

.unified-composer__reward-qr {
    max-width: 150px;
    width: 100%;
    height: 150px;
    object-fit: contain;
    border-radius: 8px;
}

.unified-composer__warm-text {
    white-space: pre-line;
    line-height: 1.7;
}

.unified-composer__warm-box {
    background: rgba(99, 102, 241, 0.06);
    border: 1px solid rgba(148, 163, 184, 0.25);
    border-radius: 10px;
    padding: 0.75rem 1rem;
}

.unified-composer__reward-section-title {
    color: #f5b301;
}

.unified-composer__send {
    justify-self: end;
    flex-shrink: 0;
    white-space: nowrap;
}

@media (max-width: 960px) {
    .unified-composer__footer {
        display: flex;
        flex-wrap: wrap;
        align-items: flex-start;
        gap: 0.5rem 0.75rem;
    }

    .unified-composer__footer-icons {
        flex: 1 1 auto;
        display: flex;
        align-items: center;
        justify-content: center;
        gap: 0.15rem 0.5rem;
        min-width: 0;
    }

    .unified-composer__footer-reward {
        justify-content: flex-start;
    }

    .unified-composer__send {
        flex: 1 0 100%;
        justify-content: center;
    }
}
</style>