<script setup>
// 分享记录：这个房间**最近分享过什么**、每条被打开了几次。
//
// 为什么需要它：分享 token 是无状态签名，服务端不留痕迹 —— 建完链接之后
// 「我上次分享的那条还在吗 / 有人看过没有」以前是没有任何地方能回答的。
// 现在每次签发都会在服务端留下一条记录（Go 侧 share-log.json、Worker 侧 D1）。
//
// 两件刻意为之的事，别当成 bug 改掉：
//   1. **列表里没有链接本身**（不回 token）。token 是 bearer 凭据，把列表做成
//      「可以再抄一遍链接」的入口，等于让任何能读这个房间记录的人取用别人的分享。
//      要重新发链接就在内容上再点一次分享 —— 那会走一遍正常的房间鉴权。
//   2. **鉴权和签发分享完全一致**（房间凭据）。房间没设密码时，这个列表是公开可读的
//      —— 和「谁都能在这个房间发内容 / 建分享」是同一件事，不是新的暴露面。
//      放敏感内容请给房间设密码，那时列表同样需要凭据。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useWebSocketStore } from '@/store/websocket';
import { errorMessage, fetchShareRecords, formatTimestamp, prettyFileSize } from '@/util.js';

const mdiAlertCircleOutline = 'mdi-alert-circle-outline';
const mdiClose = 'mdi-close';
const mdiFileOutline = 'mdi-file-outline';
const mdiLockOutline = 'mdi-lock-outline';
const mdiQrcodeScan = 'mdi-qrcode-scan';
const mdiRefresh = 'mdi-refresh';
const mdiShareVariant = 'mdi-share-variant';
const mdiTextBoxOutline = 'mdi-text-box-outline';

const props = defineProps({
    modelValue: {
        type: Boolean,
        default: false,
    },
});

const emit = defineEmits(['update:modelValue']);

const ws = useWebSocketStore();
const { t } = useI18n();

const visible = computed({
    get: () => props.modelValue,
    set: (value) => emit('update:modelValue', value),
});

const loading = ref(false);
const error = ref('');
const records = ref([]);
const total = ref(0);

function recordTitle(record) {
    const name = String(record?.name || '').trim();
    if (name) {
        return name;
    }
    return record?.kind === 'file' ? t('shareHistoryFile') : t('shareHistoryText');
}

function recordSubtitle(record) {
    const parts = [];
    if (record?.createdAt) {
        parts.push(formatTimestamp(record.createdAt));
    }
    if (record?.kind === 'file' && record?.size > 0) {
        parts.push(prettyFileSize(Number(record.size)));
    }
    if (record?.maxUses > 0) {
        parts.push(t('shareHistoryUsed', { used: Number(record.used || 0), max: Number(record.maxUses) }));
    }
    return parts.join(' · ');
}

// 一次取多少条。
//
// ⚠️ 这个数字受服务端两道闸门夹着，改之前先看：单次请求的上限是 **200**
// （Go 的 maxShareListLimit / Worker 的 MAX_SHARE_LIST_LIMIT，传再大也会被夹），
// 而这份日志服务端最多只留 **500** 条（maxShareLogRecords / MAX_SHARE_LOG_ROWS，
// 超出丢最旧、已过期的优先丢）。
//
// 取 100 是折中：比原来的 50 多一倍、够翻，又不至于让对话框里滚不完。
// **不做分页** —— 服务端一共就 500 条上限，翻页的收益抵不上多出来的那套状态。
// 被截断时列表底部有「已显示 N / 共 M 条」兜底（见模板里的 shareHistoryMore）。
const SHARE_HISTORY_LIMIT = 100;

async function load() {
    loading.value = true;
    error.value = '';
    try {
        const data = await fetchShareRecords({ room: ws.room, limit: SHARE_HISTORY_LIMIT });
        records.value = Array.isArray(data?.records) ? data.records : [];
        total.value = Number(data?.total || records.value.length);
    } catch (err) {
        console.error('读取分享记录失败:', err);
        records.value = [];
        total.value = 0;
        error.value = errorMessage(err) || t('shareHistoryFailed');
    } finally {
        loading.value = false;
    }
}

// 每次打开都重新拉：这份数据是「别人打开过没有」的答案，缓存着显示等于骗人。
watch(visible, (open) => {
    if (open) {
        load();
    }
});
</script>

<template>
    <v-dialog v-model="visible" max-width="520">
        <v-card>
            <v-card-title class="d-flex align-center">
                <v-icon start size="20">{{ mdiShareVariant }}</v-icon>
                <span class="flex-grow-1">{{ t('shareHistory') }}</span>
                <v-btn icon variant="text" @click="visible = false">
                    <v-icon>{{ mdiClose }}</v-icon>
                </v-btn>
            </v-card-title>
            <v-divider></v-divider>
            <v-card-text style="max-height: 62vh; overflow-y: auto;">
                <div class="text-body-2 text-medium-emphasis mb-3">{{ t('shareHistoryHint') }}</div>

                <div v-if="loading" class="d-flex justify-center py-8">
                    <v-progress-circular indeterminate size="28" width="3" color="primary" />
                </div>

                <div v-else-if="error" class="share-history__state">
                    <v-icon size="30" color="error">{{ mdiAlertCircleOutline }}</v-icon>
                    <span class="text-body-2">{{ error }}</span>
                </div>

                <div v-else-if="!records.length" class="share-history__state">
                    <v-icon size="30" class="text-medium-emphasis">{{ mdiShareVariant }}</v-icon>
                    <span class="text-body-2 text-medium-emphasis">{{ t('shareHistoryEmpty') }}</span>
                </div>

                <v-list v-else density="comfortable" class="pa-0">
                    <v-list-item v-for="record in records" :key="record.jti" class="px-0">
                        <template v-slot:prepend>
                            <v-icon :color="record.expired ? 'grey' : 'primary'">
                                {{ record.kind === 'file' ? mdiFileOutline : mdiTextBoxOutline }}
                            </v-icon>
                        </template>
                        <v-list-item-title class="share-history__title">
                            {{ recordTitle(record) }}
                        </v-list-item-title>
                        <v-list-item-subtitle class="share-history__subtitle">
                            {{ recordSubtitle(record) }}
                        </v-list-item-subtitle>
                        <template v-slot:append>
                            <div class="share-history__badges">
                                <v-chip size="x-small" variant="tonal" label>
                                    {{ t('shareHistoryOpened', { count: Number(record.visits || 0) }) }}
                                </v-chip>
                                <v-chip v-if="record.scans > 0" size="x-small" variant="tonal" label :prepend-icon="mdiQrcodeScan">
                                    {{ t('shareHistoryScanned', { count: Number(record.scans) }) }}
                                </v-chip>
                                <v-chip v-if="record.expired" size="x-small" variant="tonal" color="grey" label>
                                    {{ t('shareHistoryExpired') }}
                                </v-chip>
                                <v-icon v-if="record.password" size="14" class="text-medium-emphasis">{{ mdiLockOutline }}</v-icon>
                            </div>
                        </template>
                    </v-list-item>
                </v-list>

                <div v-if="total > records.length" class="text-caption text-medium-emphasis mt-2">
                    {{ t('shareHistoryMore', { shown: records.length, total }) }}
                </div>
                <div class="text-caption text-medium-emphasis mt-3">{{ t('shareHistoryPrivacyHint') }}</div>
            </v-card-text>
            <v-card-actions>
                <v-spacer></v-spacer>
                <v-btn variant="text" :prepend-icon="mdiRefresh" :loading="loading" @click="load">
                    {{ t('shareHistoryRefresh') }}
                </v-btn>
                <v-btn variant="text" @click="visible = false">{{ t('close') }}</v-btn>
            </v-card-actions>
        </v-card>
    </v-dialog>
</template>

<style scoped>
/* ⚠️ 弹窗一律 teleport 出应用子树 —— 样式必须挂在自己身上（见仓库里的覆盖层约定）。 */
.share-history__state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 32px 8px;
    text-align: center;
}

.share-history__title {
    font-size: 0.875rem;
    word-break: break-all;
}

.share-history__subtitle {
    font-size: 0.75rem;
    opacity: 0.8;
}

.share-history__badges {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 4px;
}
</style>
