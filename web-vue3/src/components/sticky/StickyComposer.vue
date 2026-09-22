<script setup>import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import { errorMessage, getClientId, prettyFileSize } from '@/util.js';
import ComposerSlashMenu from '@/components/ComposerSlashMenu.vue';

const props = defineProps({
    variant: { type: String, default: 'sticky' },
    // 多行写作输入区：随内容长高 + 带 `/` markdown 模板菜单。
    // 只有看板开 —— 它是唯一一个「用户会在这里写任务清单 / 表格」的模式。
    // 别的模式不开：终端里行首的 `/` 是路径（`/usr/local/bin`），弹菜单只会碍事。
    multiline: { type: Boolean, default: false },
});
const emit = defineEmits(['sent']);

const app = useAppStore();
const ws = useWebSocketStore();
const clientId = getClientId();
const { t } = useI18n();
const textarea = ref(null);
const selectFile = ref(null);
const sending = ref(false);
const uploadedSizes = ref([]);

// 平台判断**只用于显示**（⌘ 还是 Ctrl），不再参与键盘逻辑 —— 见 onKeydown。
const isApplePlatform = /mac|iphone|ipad|ipod/i.test(navigator.userAgent || '');
// 触摸设备没有硬件键盘，键盘提示是纯噪音。用 `pointer: coarse` 而不是屏宽：
// 宽屏触屏（iPad 横屏、触屏笔记本）同样没有快捷键，按宽度判断会漏掉它们。
const isTouchOnly = typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia('(pointer: coarse)').matches
    : false;
const sendShortcutLabel = computed(() => t('sendShortcutTip', {
    keys: isApplePlatform ? '⌘+Enter' : 'Ctrl+Enter',
}));
const placeholder = computed(() => {
    // 发送键统一成「主修饰键 + Enter」之后，这句话对**所有模式**都成立，
    // 所以统一拼在占位符里（标准模式本来就是这么做的）。
    const withSendHint = (text) => (isTouchOnly ? text : `${text} ${sendShortcutLabel.value}`);
    // 看板：这里最常写的就是任务清单 / 表格，而「/」模板是最不容易被发现的功能，
    // 直接写进占位符（标准模式也是这么做的）。
    if (props.variant === 'board') {
        return withSendHint(t('composerSlashHint'));
    }
    // 巨幕用中性那句：它的输入框也不是「写一张便签」。
    if (props.variant === 'mega') {
        return withSendHint(t('composerPlaceholder'));
    }
    if (props.variant === 'terminal') {
        return withSendHint(t('terminalPlaceholder'));
    }
    if (props.variant === 'workbench') {
        return withSendHint(t('workbenchInputHint'));
    }
    if (props.variant === 'chat') {
        return withSendHint(t('chatPlaceholder'));
    }
    return withSendHint(t('stickyNewNote'));
});
const fileSize = computed(() => app.send.files.length ? app.send.files.reduce((acc, cur) => acc += cur.size, 0) : 0);
const uploadedSize = computed(() => uploadedSizes.value.length ? uploadedSizes.value.reduce((acc, cur) => acc += cur, 0) : 0);
const uploadProgress = computed(() => Math.min(fileSize.value !== 0 ? (uploadedSize.value / fileSize.value) : 0, 1));
const sendDisabled = computed(() => !ws.websocket || sending.value || (!app.send.text && !app.send.files.length) || app.send.text.length > app.config.text.limit);

function focus() {
    nextTick(() => {
        if (textarea.value && typeof textarea.value.focus === 'function') {
            textarea.value.focus();
        }
    });
}
defineExpose({ focus, addFiles });
function addFiles(fileList) {
    handleSelectFiles(fileList);
}

function openFilePicker() {
    // 上传开关关掉时这个 input 会被 v-if 摘掉，ref 就是 null —— 不能裸点
    selectFile.value?.click();
}

function handlePaste(event) {
    if (!(event && event.clipboardData)) {
        return;
    }
    const files = [];
    for (const item of Array.from(event.clipboardData.items || [])) {
        if (item.kind === 'file') {
            const file = item.getAsFile();
            if (file) {
                files.push(file);
            }
        }
    }
    if (!files.length) {
        for (const file of Array.from(event.clipboardData.files || [])) {
            if (file) {
                files.push(file);
            }
        }
    }
    if (files.length) {
        event.preventDefault();
        handleSelectFiles(files);
    }
}

onMounted(() => {
    document.addEventListener('paste', handlePaste);
});
onBeforeUnmount(() => {
    document.removeEventListener('paste', handlePaste);
});

function handleSelectFiles(fileList) {
    const files = Array.from(fileList || []);
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

function removeFile(index) {
    app.send.files.splice(index, 1);
}

// ── 多行写作（看板）：`/` 模板菜单 + 输入框随内容长高 ─────────────────────
// 逻辑跟标准模式（UnifiedComposer）是同一套：只在**行首**打 `/` 才弹，正文里的 `/`
// （路径、日期、`a/b`）不管。模板正文是中性占位符，不放进 i18n —— 它们是要被改写的骨架。
const SLASH_TEMPLATES = [
    { key: 'filterTaskList', icon: 'mdi-checkbox-marked-outline', text: '- [ ] \n- [ ] \n- [ ] ' },
    { key: 'filterTable', icon: 'mdi-table', text: '| A | B |\n| --- | --- |\n|  |  |' },
];
const slashMenu = ref(false);
let slashEl = null;

function insertSlashTemplate(tpl) {
    const el = slashEl || textarea.value;
    const text = app.send.text || '';
    const pos = el && typeof el.selectionStart === 'number' ? el.selectionStart : text.length;
    // 连同刚打的那个 `/` 一起换掉（如果它还在光标前）
    const head = text.slice(0, pos).replace(/\/$/, '');
    const tail = text.slice(pos);
    app.send.text = head + tpl.text + tail;
    slashMenu.value = false;
    nextTick(() => {
        const caret = head.length + tpl.text.length;
        el?.focus?.();
        el?.setSelectionRange?.(caret, caret);
    });
}

function onKeydown(event) {
    if (props.multiline) {
        if (event.key === 'Escape' && slashMenu.value) {
            slashMenu.value = false;
            event.stopPropagation();
            return;
        }
        if (event.key === '/') {
            const el = event.target;
            const pos = typeof el?.selectionStart === 'number' ? el.selectionStart : 0;
            const lineStart = (app.send.text || '').lastIndexOf('\n', Math.max(0, pos - 1)) + 1;
            // 只认「本行到目前为止只有空白」
            if (!(pos > lineStart && /\S/.test((app.send.text || '').slice(lineStart, pos)))) {
                slashEl = el;
                slashMenu.value = true;
            }
        }
    }
    // 发送约定：**主修饰键 + Enter**（Mac 是 ⌘，其余平台是 Ctrl），跨平台、跨模式一套。
    // 回车本身永远是换行 —— 那是 textarea 的默认行为，不用拦。
    //
    // 为什么不再按平台分叉：
    //   · 标准模式（UnifiedComposer）本来就是 ⌘/Ctrl+Enter。非 Mac 上「回车即发」
    //     等于**同一个 app、同一个用户，换台机器行为就变** —— 而跨设备正是这个项目
    //     存在的意义，按平台分叉恰好和它作对；
    //   · 失败模式不对称：按错键只是多出一个换行（无声、可撤销）；而「回车即发」按错
    //     是把半条消息发出去了（不可撤销）。
    // 看板的 `/` 模板一插就是三行骨架，回车即发的话那三行根本没法改 —— 现在这条规则
    // 对所有模式都成立，不用再给看板开例外。
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
        sendAll();
    }
}

async function sendText() {
    if (!app.send.text) {
        return;
    }
    await axios.post(
        'text',
        app.send.text,
        {
            params: new URLSearchParams([['room', ws.room], ['client', clientId]]),
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
    sending.value = true;
    await Promise.all(app.send.files.map(async (file, index) => {
        if (file.size < chunkSize) {
            const formData = new FormData;
            formData.set('file', file);
            await axios.postForm('upload', formData, {
                params: new URLSearchParams([['room', ws.room], ['client', clientId]]),
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
            params: new URLSearchParams([['room', ws.room], ['client', clientId]]),
        });
    }));
    app.send.files.splice(0);
}

// 同 UnifiedComposer：两边都关掉就没有可发的东西，藏起发送按钮。
const canSend = computed(() => Boolean(app.display.composerText || app.display.composerUpload));

async function sendAll() {
    if (sendDisabled.value) {
        return;
    }
    try {
        await sendText();
        await sendFiles();
        toast(t('sendSuccess'));
        focus();
    } catch (error) {
        const errMsg = errorMessage(error);
        if (errMsg) {
            toast(t('sendFailedMsg', { msg: errMsg }));
        } else {
            toast(t('sendFailed'));
        }
    } finally {
        sending.value = false;
        slashMenu.value = false;
        emit('sent');
    }
}
</script>

<template>
    <!-- 文本区与上传区都关掉 = 这个模式只想接收。
         此时**不要留空**，把这块位置改成搜索框 —— 不能发的时候，搜索才是这里的主动作。
         （搜索条本身只在标准模式，所以这里自带一个。） -->
    <div v-if="!app.display.composerText && !app.display.composerUpload" class="sticky-composer sticky-composer--search">
        <v-text-field
            :model-value="app.searchQuery"
            density="compact"
            variant="solo"
            rounded="pill"
            flat
            hide-details
            clearable
            prepend-inner-icon="mdi-magnify"
            :placeholder="t('searchPlaceholder')"
            @update:model-value="app.setSearchQuery"
        ></v-text-field>
    </div>

    <div v-else class="sticky-composer" :class="`sticky-composer--${props.variant}`">
        <div v-if="app.display.composerUpload && app.send.files.length" class="sticky-composer__files">
            <span
                v-for="(file, index) in app.send.files"
                :key="file.name + index"
                class="sticky-composer__file"
            >{{ file.name }} <b class="sticky-composer__closer" @click="removeFile(index)">✕</b></span>
            <span v-if="sending" class="sticky-composer__progress">{{ Math.round(uploadProgress * 100) }}%</span>
        </div>
        <!-- 「/」模板菜单走**正常文档流**、不绝对定位：祖先只要有一个 overflow，
             浮动元素就会被静默裁掉（标准模式那边踩过，DOM 在、就是看不见）。 -->
        <composer-slash-menu
            v-if="slashMenu"
            :items="SLASH_TEMPLATES"
            @pick="insertSlashTemplate"
        />
        <div class="sticky-composer__row">
            <button v-if="app.display.composerUpload" type="button" class="sticky-composer__attach" title="📎" @click="openFilePicker">➕</button>
            <textarea
                v-if="app.display.composerText"
                ref="textarea"
                v-model="app.send.text"
                class="sticky-composer__area"
                rows="1"
                :placeholder="placeholder"
                @keydown="onKeydown"
            ></textarea>
<template v-if="canSend">
            <button
                v-if="props.variant === 'mega'"
                type="button"
                class="sticky-composer__go sticky-composer__go--mega"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('send') }}</button>
            <button
                v-else-if="props.variant === 'terminal'"
                type="button"
                class="sticky-composer__go sticky-composer__go--terminal"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('terminalEnter') }}</button>
            <button
                v-else-if="props.variant === 'workbench'"
                type="button"
                class="sticky-composer__go sticky-composer__go--workbench"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('send') }}</button>
            <button
                v-else-if="props.variant === 'chat'"
                type="button"
                class="sticky-composer__go sticky-composer__go--chat"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('send') }}</button>
            <button
                v-else-if="props.variant === 'board'"
                type="button"
                class="sticky-composer__go sticky-composer__go--board"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('send') }}</button>
            <button
                v-else
                type="button"
                class="sticky-composer__go"
                :disabled="sendDisabled"
                @click="sendAll"
            >{{ t('stickyStick') }}</button>
            </template>
            <input
                v-if="app.display.composerUpload"
                ref="selectFile"
                type="file"
                multiple
                class="d-none"
                @change="handleSelectFiles(Array.from($event.target.files)); $event.target.value = ''"
            >
        </div>
    </div>
</template>

<style scoped>
/* 搜索态：去掉便签那圈虚线外框 —— 它不再是「写点什么」的地方，是个搜索框。 */
/* ⚠️ 用**两个类**写，不要只写 .sticky-composer--search：
   它和基础规则 .sticky-composer 权重相同，而基础规则在文件里更靠后，
   于是米黄底 (#fffbe8) 和那圈虚线边框照样生效 —— 表现为「五个非标准模式的
   搜索框全长成便签样」。两个类权重更高，跟位置无关。 */
.sticky-composer.sticky-composer--search {
    background: transparent;
    border: none;
    box-shadow: none;
    padding: 0;
    /* 颜色回到模式本身，不要沿用便签 composer 的褐字（搜索框底色是从 currentColor 推的） */
    color: inherit;
}
/* 搜索框底色跟着模式走。各模式没有统一的颜色 token（终端有 --tw-*，便签/聊天是写死的），
   所以用 currentColor 混一层浅底：深色模式文字浅 → 得到浅底；浅色模式文字深 → 得到深一点的底。
   一处规则适配六套皮肤，不用每套各写一份。 */
.sticky-composer--search :deep(.v-field) {
    background: color-mix(in srgb, currentColor 8%, transparent);
    /* ⚠️ `color: inherit` 必须加在 .v-field 上，不能只加在 .v-field__input 上：
       color-mix 里的 currentColor 取的是**元素自己**的颜色。只改 input 的话，
       .v-field 仍是 Vuetify 给的颜色，于是便签/终端这类自带皮肤的模式的底色
       全都算成黑色 —— 看着像「没适配」。 */
    color: inherit;
}

.sticky-composer--search :deep(.v-field__input),
.sticky-composer--search :deep(.v-field__prepend-inner .v-icon),
.sticky-composer--search :deep(.v-field__clearable .v-icon) {
    color: inherit;
}


.sticky-composer {
    background: #fffbe8;
    border: 1.5px dashed #d5c49a;
    border-radius: 12px;
    padding: 10px 13px;
    box-shadow: 0 2px 6px rgba(68, 64, 42, 0.08);
}

.sticky-composer__files {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 8px;
}

.sticky-composer__file {
    font-size: 11px;
    background: rgba(213, 196, 154, 0.28);
    color: #6f6548;
    border-radius: 6px;
    padding: 3px 8px;
}

.sticky-composer__closer {
    font-weight: 700;
    cursor: pointer;
    margin-left: 4px;
    opacity: 0.6;
}

.sticky-composer__progress {
    font-size: 11px;
    font-weight: 700;
    color: #0a0d24;
}

.sticky-composer__row {
    display: flex;
    align-items: center;
    gap: 9px;
}

.sticky-composer__attach {
    font-size: 16px;
    background: none;
    border: none;
    cursor: pointer;
    padding: 0;
    line-height: 1;
    color: #b8ae9a;
}

.sticky-composer__area {
    flex: 1;
    min-width: 0;
    resize: none;
    border: none;
    outline: none;
    background: transparent;
    font-size: 12px;
    line-height: 1.5;
    color: #444034;
    font-family: inherit;
    padding: 6px 0;
}

.sticky-composer__area::placeholder {
    color: #b8ae9a;
}

.sticky-composer__go {
    background: #d97706;
    color: #fff;
    border: none;
    border-radius: 8px;
    font-size: 12px;
    padding: 6px 14px;
    font-weight: 650;
    cursor: pointer;
    flex-shrink: 0;
}

.sticky-composer__go:disabled {
    opacity: 0.55;
    cursor: not-allowed;
}

.sticky-composer__go--mega {
    background: #111827;
    color: #fff;
    border-radius: 10px;
    font-size: 13px;
    padding: 7px 18px;
    font-weight: 700;
}

.sticky-composer--mega {
    background: #fff;
    border: 1px solid #e8ebf0;
    border-radius: 14px;
    padding: 12px 15px;
    box-shadow: none;
}

.sticky-composer--mega .sticky-composer__row {
    gap: 10px;
}

.sticky-composer--mega .sticky-composer__attach {
    font-size: 16px;
    color: #aab2bd;
}

.sticky-composer--mega .sticky-composer__area {
    font-size: 13px;
    color: #1f2937;
    padding: 4px 0;
}

.sticky-composer--mega .sticky-composer__area::placeholder {
    color: #c6ccd4;
}

.sticky-composer--mega .sticky-composer__file {
    background: #f1f5f9;
    color: #475569;
}

.sticky-composer--mega .sticky-composer__progress {
    color: #1f2937;
}

.sticky-composer--terminal {
    background: #161b22;
    border: 1px solid #30363d;
    border-radius: 8px;
    padding: 9px 11px;
    box-shadow: none;
    font-family: 'SF Mono', 'Menlo', 'Consolas', monospace;
}

.sticky-composer--terminal .sticky-composer__row {
    gap: 8px;
}

.sticky-composer--terminal .sticky-composer__attach {
    font-size: 13px;
    color: #8b949e;
}

.sticky-composer--terminal .sticky-composer__area {
    font-family: inherit;
    font-size: 12px;
    color: #c9d1d9;
    padding: 4px 0;
}

.sticky-composer--terminal .sticky-composer__area::placeholder {
    color: #6e7681;
}

.sticky-composer--terminal .sticky-composer__file {
    background: #21262d;
    color: #c9d1d9;
}

.sticky-composer--terminal .sticky-composer__progress {
    color: #58a6ff;
}

.sticky-composer__go--terminal {
    background: #238636;
    color: #fff;
    border-radius: 6px;
    font-size: 11px;
    padding: 5px 12px;
    font-weight: 700;
    font-family: 'SF Mono', 'Menlo', 'Consolas', monospace;
}

.sticky-composer--workbench {
    background: #fff;
    border: 1px solid #dfe4ea;
    border-radius: 10px;
    padding: 11px 13px;
    box-shadow: 0 1px 3px rgba(30, 45, 62, 0.06);
}

.sticky-composer--workbench .sticky-composer__row {
    gap: 9px;
}

.sticky-composer--workbench .sticky-composer__attach {
    font-size: 15px;
    color: #9ca3af;
}

.sticky-composer--workbench .sticky-composer__area {
    font-size: 12px;
    color: #1a2332;
    padding: 4px 0;
}

.sticky-composer--workbench .sticky-composer__area::placeholder {
    color: #aab2bd;
}

.sticky-composer--workbench .sticky-composer__file {
    background: #f1f5f9;
    color: #475569;
}

.sticky-composer--workbench .sticky-composer__progress {
    color: #1a2332;
}

.sticky-composer__go--workbench {
    background: #1e88e5;
    border-radius: 9px;
    font-size: 12px;
    padding: 8px 15px;
    font-weight: 650;
}

.sticky-composer--chat {
    background: #fff;
    border: 1px solid #dfe4ea;
    border-radius: 14px;
    padding: 11px 14px;
    box-shadow: 0 1px 3px rgba(30, 45, 62, 0.05);
}

.sticky-composer--chat .sticky-composer__attach {
    font-size: 15px;
    color: #9ca3af;
}

.sticky-composer--chat .sticky-composer__area {
    font-size: 12px;
    color: #111827;
    padding: 4px 0;
}

.sticky-composer--chat .sticky-composer__area::placeholder {
    color: #aab2bd;
}

.sticky-composer--chat .sticky-composer__file {
    background: #f1f5f9;
    color: #475569;
}

.sticky-composer--chat .sticky-composer__progress {
    color: #111827;
}

.sticky-composer__go--chat {
    background: #1e88e5;
    border-radius: 9px;
    font-size: 12px;
    padding: 8px 15px;
    font-weight: 650;
}

/* ── 看板 ────────────────────────────────────────────────────────────────
   看板的面板是「实心浅底 + 一圈发丝线」，没有便签那张纸的意思，所以这一套皮肤只做
   一件事：把便签的米黄底 + 虚线边框换掉。颜色不写死在这里 —— 看板有自己的明暗两套配色，
   由 .board-wall / .board-wall--dark 给变量。这样这个组件不需要认识「看板」，
   也不需要认识主题。 */
.sticky-composer--board {
    background: var(--board-panel-bg, #fff);
    border: 1px solid var(--board-hairline, rgba(148, 163, 184, 0.32));
    border-radius: 10px;
    padding: 7px 11px;
    box-shadow: none;
    color: inherit;
}

/* 看板的发送区是「新建一张卡片」，不是聊天输入条，所以排布跟另外五个模式不同：
   正文**独占一整行**（要写得下任务清单和表格），动作另起一行（附件在左、发送在右）。

   ⚠️ 之前三样（附件 / 正文 / 发送）挤在同一个 flex 行里、`align-items: center`：
   正文一长高，附件和发送就被甩到框底，看起来像「底部那一行没被用上」。
   根因就是这个 —— 不是间距问题，是它们不该在同一行。
   ⚠️ 改成 grid 之后，每个元素都必须显式写 grid-row / grid-column：
   grid 靠 grid-area 定位，不写的话按 DOM 顺序排，附件会跑到正文上面去。 */
.sticky-composer--board .sticky-composer__row {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    grid-template-rows: auto auto;
    align-items: center;
    gap: 6px 8px;
}

.sticky-composer--board .sticky-composer__area {
    grid-column: 1 / -1;
    grid-row: 1;
    font-size: 12.5px;
    /* 跟随容器（看板给的文字色），不然深色看板里输入的字是黑的 */
    color: inherit;
    padding: 2px 0;
    /* 起步就给五行：一行的框写不出任务清单 / 表格。
       5lh 是五行正文，+8px 是上下 padding（盒子是 border-box，min-height 含 padding）。
       ⚠️ 刻意**只用 CSS 定高**：之前那版用 JS 按内容算高度（el.scrollHeight），
       结果发送按钮那一行看起来被架空 —— 输入区一长高，底下那行就成了一块用不上的空白。
       要调高度只改这一个数，别再引入运行时改高度。 */
    min-height: calc(5lh + 8px);
    max-height: 38vh;
    overflow-y: auto;
}

.sticky-composer--board .sticky-composer__attach {
    grid-column: 1;
    grid-row: 2;
    justify-self: start;
    /* 之前是个裸 ➕ 文本（`font-size: 15px`），看着像误入的字符；
       给它一个按钮的形状，读起来才是「可以点」。 */
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 30px;
    height: 30px;
    border-radius: 8px;
    background: rgba(148, 163, 184, 0.18);
    font-size: 14px;
    color: var(--board-hint, #a8b1bd);
}

.sticky-composer--board .sticky-composer__go {
    grid-column: 2;
    grid-row: 2;
    justify-self: end;
}

.sticky-composer--board .sticky-composer__area::placeholder {
    color: var(--board-hint, #a8b1bd);
}

.sticky-composer--board .sticky-composer__file {
    background: rgba(148, 163, 184, 0.18);
    color: inherit;
}

.sticky-composer--board .sticky-composer__progress {
    color: inherit;
}

.sticky-composer__go--board {
    background: rgb(var(--v-theme-primary));
    color: #fff;
    border-radius: 8px;
    font-size: 12px;
    padding: 6px 14px;
    font-weight: 650;
}
</style>