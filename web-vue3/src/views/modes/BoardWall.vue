<script setup>
// 看板模式：固定三列（待办 / 进行中 / 已完成），卡片就是剪贴板条目本身。
//
// 设计上刻意做成**最小实现**：
//   · 列固定三个，不做用户自建（自建列要连带「列本身的增删改」，那是另一层数据）；
//   · 卡片就是普通条目，列只是条目上的一个字段 —— 所以看板不是「另一份数据」，
//     别的模式看到的还是同一批内容，只是不按列摆；
//   · 不引入「列内顺序」，拖动只改「在哪一列」。
//
// 拖放用 HTML5 原生事件（桌面），**手机上拖不动** —— 所以每张卡片另有「移到」菜单，
// 一套机制覆盖两种设备，而不是给触屏再写一套拖动。
import { computed, ref } from 'vue';
import axios from 'axios';
import { useAppStore } from '@/store/app';
import { useWebSocketStore } from '@/store/websocket';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { toast } from '@/plugins/toast';
import PageToolbar from '@/components/PageToolbar.vue';
import StickyComposer from '@/components/sticky/StickyComposer.vue';
import BoardCardBody from '@/components/board/BoardCardBody.vue';
import ShareLinkButton from '@/components/ShareLinkButton.vue';
import { copyTextToClipboard, deviceLabel, errorMessage, formatTimestamp, isImageName, prettyFileSize, updateEntryColumn } from '@/util.js';

// 与服务端 normalizeBoardColumn 同一份取值 —— 空串也算待办（服务端就是这么归一的）。
const COLUMNS = [
    { key: 'todo', labelKey: 'boardColumnTodo', icon: 'mdi-tray-arrow-down' },
    { key: 'doing', labelKey: 'boardColumnDoing', icon: 'mdi-progress-clock' },
    { key: 'done', labelKey: 'boardColumnDone', icon: 'mdi-check-circle-outline' },
];

const app = useAppStore();
const ws = useWebSocketStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);
const { t } = useI18n();

const dragging = ref(null);
const dragOverColumn = ref('');
const savingId = ref('');
// 点卡片打开的详情弹窗。看板卡片以前点不开 —— 于是分享 / 复制 / 删除在这个模式里
// 一个入口都没有，只能切回别的模式去操作同一条内容。
const detailItem = ref(null);

const columnOf = (item) => (item?.column === 'doing' || item?.column === 'done' ? item.column : 'todo');
const itemsIn = (key) => app.received.filter((item) => columnOf(item) === key);
const historyUsageLabel = computed(() => `${app.received.length}/${Number(app.config?.server?.history || 0)}`);

// 文件条目**没有正文**（FileReceive 只有 Name/Size/Cache/...），卡片上只能显示文件名。
// 文本条目的正文交给 BoardCardBody —— 那边要按条目持有 md / 打勾的状态。
const fileName = (item) => item.name || 'file';

function cardMeta(item) {
    const parts = [];
    if (app.display.timestamp && item.timestamp) {
        parts.push(formatTimestamp(item.timestamp));
    }
    if (app.display.device && item.senderDevice) {
        const device = deviceLabel(item.senderDevice);
        if (device) {
            parts.push(device);
        }
    }
    if (app.display.ip && item.senderIP) {
        parts.push(item.senderIP);
    }
    if (item.type === 'file' && item.size) {
        parts.push(prettyFileSize(item.size));
    }
    return parts.join(' · ');
}

async function moveTo(item, column) {
    if (columnOf(item) === column || savingId.value === item.id) {
        return;
    }
    // 乐观更新：先动界面，失败再退回去。
    // 不回退的话「服务端没存上」会显示成成功，刷新才发现卡片又回去了。
    const previous = item.column;
    item.column = column;
    savingId.value = item.id;
    try {
        await updateEntryColumn(item.id, ws.room, column);
    } catch (error) {
        console.error('挪动看板卡片失败:', error);
        item.column = previous;
        toast(t('boardMoveFailed'));
    } finally {
        savingId.value = '';
    }
}

function onDragStart(item, event) {
    dragging.value = item;
    dragOverColumn.value = '';
    if (event?.dataTransfer) {
        event.dataTransfer.effectAllowed = 'move';
        // Firefox 不设 data 就不触发 drop
        event.dataTransfer.setData('text/plain', String(item.id));
    }
}

function onDrop(column) {
    const item = dragging.value;
    dragging.value = null;
    dragOverColumn.value = '';
    if (item) {
        moveTo(item, column);
    }
}

// ── 详情弹窗的动作 ────────────────────────────────────────────────────
// 只放三样：复制正文（文本）、分享（复用那个共用组件）、删除。
// **没有下载**：文件条目的分享页本身就带下载按钮，看板不是文件管理器，
// 再挂一个下载会多出一套 ensureFileShareLinks 的复制品。
async function copyItemText(item) {
    try {
        await copyTextToClipboard(item?.content || '');
        toast(t('copySuccess'));
    } catch {
        toast(t('copyFailedGeneral'));
    }
}

async function deleteItem(item) {
    try {
        await axios.delete(`revoke/${item.id}`, {
            params: new URLSearchParams([['room', ws.room]]),
        });
        detailItem.value = null;
        toast(t('deleteSuccessText', { name: item.name || '' }));
    } catch (error) {
        const errMsg = errorMessage(error);
        toast(errMsg ? t('deleteFailedMessageMsg', { msg: errMsg }) : t('deleteFailedMessage'));
    }
}
</script>

<template>
    <div class="board-wall" :class="{ 'board-wall--dark': isDark }">
        <PageToolbar variant="board"></PageToolbar>

        <div class="board-wall__shell">
            <div class="board-wall__head">
                <span class="board-wall__title">{{ t('uiModeBoard') }}</span>
                <span class="board-wall__count">{{ historyUsageLabel }}</span>
            </div>

            <div class="board-wall__columns">
                <section
                    v-for="column in COLUMNS"
                    :key="column.key"
                    class="board-wall__column"
                    :class="{ 'board-wall__column--over': dragOverColumn === column.key }"
                    @dragover.prevent="dragOverColumn = column.key"
                    @dragleave="dragOverColumn = dragOverColumn === column.key ? '' : dragOverColumn"
                    @drop.prevent="onDrop(column.key)"
                >
                    <header class="board-wall__column-head">
                        <v-icon size="16" class="me-1">{{ column.icon }}</v-icon>
                        <span class="board-wall__column-title">{{ t(column.labelKey) }}</span>
                        <span class="board-wall__column-count">{{ itemsIn(column.key).length }}</span>
                    </header>

                    <div class="board-wall__list">
                        <article
                            v-for="item in itemsIn(column.key)"
                            :key="item.id"
                            class="board-wall__card"
                            :class="{ 'board-wall__card--saving': savingId === item.id }"
                            draggable="true"
                            role="button"
                            tabindex="0"
                            @click="detailItem = item"
                            @keydown.enter.prevent="detailItem = item"
                            @dragstart="onDragStart(item, $event)"
                            @dragend="dragging = null"
                        >
                            <div class="board-wall__card-top">
                                <v-icon v-if="item.type === 'file'" size="14" class="me-1">
                                    {{ isImageName(item.name) ? 'mdi-image-outline' : 'mdi-file-outline' }}
                                </v-icon>
                                <!-- 文件没有正文，卡片上就一个文件名；文本交给 BoardCardBody。
                                     不往它身上挂 board-wall__card-text：两边特异性相同，
                                     谁生效取决于打包顺序，字号和行数限制会打架。 -->
                                <span v-if="item.type === 'file'" class="board-wall__card-text">{{ fileName(item) }}</span>
                                <board-card-body v-else :meta="item"></board-card-body>
                            </div>
                            <div class="board-wall__card-meta">{{ cardMeta(item) }}</div>

                            <!-- 手机上拖不动，所以「移到哪一列」得有个点得到的入口 -->
                            <v-menu location="bottom end">
                                <template v-slot:activator="{ props }">
                                    <v-btn
                                        v-bind="props"
                                        icon
                                        density="compact"
                                        size="x-small"
                                        variant="text"
                                        class="board-wall__card-move"
                                        :title="t('boardMoveTo')"
                                        @click.stop
                                    >
                                        <v-icon size="14">mdi-dots-vertical</v-icon>
                                    </v-btn>
                                </template>
                                <v-list density="compact">
                                    <v-list-item
                                        v-for="target in COLUMNS"
                                        :key="target.key"
                                        :disabled="target.key === columnOf(item)"
                                        @click="moveTo(item, target.key)"
                                    >
                                        <v-list-item-title>
                                            <v-icon size="16" class="me-2">{{ target.icon }}</v-icon>{{ t(target.labelKey) }}
                                        </v-list-item-title>
                                    </v-list-item>
                                </v-list>
                            </v-menu>
                        </article>
                    </div>
                </section>
            </div>

            <!-- 空看板**不再另外挂一块空态文案**：三列本身就是「这里是空的」最好的说明，
                 而且原来那块用的是时间流的文案（「时间流还没有内容」），在按列摆的界面上
                 名词就是错的。两块又都是 flex: 1，还会互相挤。 -->
            <div class="board-wall__composer">
                <!-- 这一行是看板特有的：说清楚「在这里写会变成一张卡片、落在哪一列」。
                     键盘提示**不在这里** —— 发送键统一成「主修饰键 + Enter」之后它对所有
                     模式都成立，已经拼进输入框的占位符里（见 StickyComposer 的 placeholder）。 -->
                <div class="board-wall__composer-head">
                    <span>{{ t('boardNewCardIn', { column: t('boardColumnTodo') }) }}</span>
                </div>
                <!-- 复用便签那套发送组件（改发送区只需改一处），但**必须换皮肤**：
                     它的默认皮肤是米黄便签纸 + 虚线边框，直接套上来就是「看板底部挂了
                     一个便签模式的发送窗」。board 皮肤的颜色由这里给的变量决定。
                     multiline：看板是用来写任务清单 / 表格的，输入框要能长高、且带 / 模板菜单。 -->
                <sticky-composer variant="board" multiline></sticky-composer>
            </div>
        </div>

        <!-- 卡片详情。看板是唯一一个「卡片点不开」的模式，于是分享 / 复制 / 删除
             在这里没有入口。这里只补最小的三样，不做别的模式那种大阅读器。
             ⚠️ 弹窗被 teleport 出应用子树，主题类必须挂在自己身上 —— 挂在 .board-wall
             上的话这层拿不到（见仓库里那条覆盖层约定）。 -->
        <v-dialog v-model="detailItem" max-width="480">
            <div v-if="detailItem" class="board-wall__reader" :class="{ 'board-wall__reader--dark': isDark }">
                <div class="board-wall__reader-head">
                    <span class="board-wall__reader-type">{{ detailItem.type === 'file' ? 'FILE' : 'TEXT' }}</span>
                    <span class="board-wall__reader-time">{{ formatTimestamp(detailItem.timestamp) }}</span>
                    <button type="button" class="board-wall__reader-close" :aria-label="t('close')" @click="detailItem = null">
                        <v-icon size="16">mdi-close</v-icon>
                    </button>
                </div>

                <div class="board-wall__reader-body">
                    <div v-if="detailItem.type === 'file'" class="board-wall__reader-file">
                        <v-icon size="18" class="me-1">
                            {{ isImageName(detailItem.name) ? 'mdi-image-outline' : 'mdi-file-outline' }}
                        </v-icon>
                        <span class="board-wall__reader-name">{{ fileName(detailItem) }}</span>
                        <span class="board-wall__reader-size">{{ prettyFileSize(detailItem.size || 0) }}</span>
                    </div>
                    <board-card-body v-else :meta="detailItem"></board-card-body>
                </div>

                <div class="board-wall__reader-actions">
                    <v-btn v-if="detailItem.type !== 'file'" variant="text" size="small" @click="copyItemText(detailItem)">
                        <v-icon start size="small">mdi-content-copy</v-icon>{{ t('copyText') }}
                    </v-btn>
                    <share-link-button :meta="detailItem" :icon-only="false" />
                    <v-spacer></v-spacer>
                    <v-btn variant="text" size="small" class="board-wall__reader-delete" @click="deleteItem(detailItem)">
                        <v-icon start size="small">mdi-close-circle-outline</v-icon>{{ t('delete') }}
                    </v-btn>
                </div>
            </div>
        </v-dialog>
    </div>
</template>

<style scoped>
.board-wall {
    background: #f4f6fb;
    height: 100vh;
    height: 100dvh;
    display: flex;
    flex-direction: column;
    color: #1f2937;
    /* 发送区（StickyComposer 的 board 皮肤）要跟看板面板同一种材质，所以颜色从这里给：
       那个组件不认识「看板」，也不该认识 —— 它只认这几个变量。 */
    --board-panel-bg: #fff;
    --board-hairline: rgba(148, 163, 184, 0.32);
    --board-hint: #a8b1bd;
}

.board-wall--dark {
    background: #0f172a;
    color: #e2e8f0;
    --board-panel-bg: rgba(15, 23, 42, 0.9);
    --board-hairline: rgba(71, 85, 105, 0.6);
    --board-hint: #64748b;
}

.board-wall__shell {
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: 1200px;
    margin: 0 auto;
    padding: 10px 12px 0;
    display: flex;
    flex-direction: column;
}

.board-wall__head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    padding: 0 2px 8px;
    font-size: 12px;
    color: #64748b;
}

.board-wall--dark .board-wall__head {
    color: #94a3b8;
}

.board-wall__title {
    font-weight: 500;
}

/* 三列等宽；窄屏时每列至少留出 78vw，于是能横向滑、下一列露一角提示可滑 */
.board-wall__columns {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    /* ⚠️ 行高必须显式写成 minmax(0, 1fr)。默认是 auto —— 由内容撑开，
       待办列里卡片一多，这一行就比容器还高，多出来的部分被下面的 overflow-y: hidden
       直接裁掉：表现为「列里滚不动、最下面的卡片永远看不见」。 */
    grid-template-rows: minmax(0, 1fr);
    gap: 10px;
    overflow-x: auto;
    overflow-y: hidden;
    padding-bottom: 6px;
}

.board-wall__column {
    min-width: 0;
    /* 同理：grid 项默认 min-height: auto，不会缩到内容以下，列内的 overflow-y 就永远不会触发 */
    min-height: 0;
    display: flex;
    flex-direction: column;
    background: rgba(255, 255, 255, 0.72);
    border: 1px solid rgba(148, 163, 184, 0.32);
    border-radius: 12px;
    padding: 6px;
    transition: border-color 0.15s, background-color 0.15s;
}

.board-wall--dark .board-wall__column {
    background: rgba(30, 41, 59, 0.62);
    border-color: rgba(71, 85, 105, 0.6);
}

.board-wall__column--over {
    border-color: rgb(var(--v-theme-primary));
    background: rgba(148, 163, 184, 0.18);
}

.board-wall__column-head {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 4px 8px;
    font-size: 12px;
    font-weight: 500;
}

.board-wall__column-count {
    margin-left: auto;
    font-weight: 400;
    color: #64748b;
}

.board-wall__list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 6px;
}

.board-wall__card {
    position: relative;
    border-radius: 8px;
    border: 1px solid rgba(148, 163, 184, 0.32);
    background: #fff;
    padding: 7px 26px 7px 9px;
    cursor: grab;
}

.board-wall--dark .board-wall__card {
    background: rgba(15, 23, 42, 0.9);
    border-color: rgba(71, 85, 105, 0.6);
}

.board-wall__card--saving {
    opacity: 0.6;
}

.board-wall__card-text {
    display: -webkit-box;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
    white-space: pre-wrap;
    word-break: break-word;
    font-size: 12.5px;
    line-height: 1.5;
}

.board-wall__card-meta {
    margin-top: 4px;
    font-size: 10.5px;
    color: #64748b;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.board-wall--dark .board-wall__card-meta {
    color: #94a3b8;
}

.board-wall__card-move {
    position: absolute;
    top: 2px;
    right: 2px;
}

.board-wall__composer {
    flex-shrink: 0;
    padding-bottom: env(safe-area-inset-bottom);
}

.board-wall__composer-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
    padding: 0 2px 6px;
    font-size: 11px;
    color: #64748b;
}

.board-wall--dark .board-wall__composer-head {
    color: #94a3b8;
}

/* ── 卡片详情弹窗 ──────────────────────────────────────────────────────
   弹窗被 teleport 出去，所以这一套颜色必须自包含：不能靠 .board-wall / --board-* 变量
   （那些挂在应用子树里，这层拿不到）。明暗两套写在这里。 */
.board-wall__reader {
    background: #fff;
    color: #1f2937;
    border: 1px solid rgba(148, 163, 184, 0.32);
    border-radius: 12px;
    padding: 12px 14px 10px;
    display: flex;
    flex-direction: column;
    gap: 10px;
}

.board-wall__reader--dark {
    background: #0f172a;
    color: #e2e8f0;
    border-color: rgba(71, 85, 105, 0.6);
}

.board-wall__reader-head {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 11px;
    letter-spacing: 0.04em;
    color: #64748b;
}

.board-wall__reader--dark .board-wall__reader-head {
    color: #94a3b8;
}

.board-wall__reader-time {
    flex: 1;
    letter-spacing: normal;
}

.board-wall__reader-close {
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
    color: inherit;
    line-height: 1;
}

.board-wall__reader-body {
    font-size: 12.5px;
    line-height: 1.55;
}

/* 详情里要能看全：卡片上的 4 行截断在这里放开，改成整体限高 + 滚动。
   ⚠️ 用 :deep() 是因为 .board-card-body 的截断写在子组件的 scoped 样式里；
   这里的选择器带两个类 + 属性，特异性压得过它，不靠打包顺序。 */
.board-wall__reader-body :deep(.board-card-body) {
    display: block;
    -webkit-line-clamp: unset;
    max-height: 46vh;
    overflow-y: auto;
}

.board-wall__reader-file {
    display: flex;
    align-items: center;
    gap: 6px;
    word-break: break-all;
}

.board-wall__reader-size {
    margin-left: auto;
    flex-shrink: 0;
    color: #64748b;
}

.board-wall__reader--dark .board-wall__reader-size {
    color: #94a3b8;
}

.board-wall__reader-actions {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
}

.board-wall__reader-delete {
    color: #dc2626;
}

.board-wall__reader--dark .board-wall__reader-delete {
    color: #f87171;
}

@media (max-width: 768px) {
    .board-wall__columns {
        grid-template-columns: repeat(3, minmax(78vw, 1fr));
    }
}
</style>
