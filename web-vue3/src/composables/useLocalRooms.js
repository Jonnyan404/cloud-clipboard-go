import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useWebSocketStore } from '@/store/websocket';
import { toast } from '@/plugins/toast';

// 本地管理的房间列表：**不依赖服务端**。
//
// 为什么要有这一套：服务端把 `server.roomList` 关掉时 `/rooms` 什么都不返回，
// 于是「房间侧栏」那一套（App.vue 的 roomGroups → RoomList）整个不可用 ——
// 但用户明明可以自己记住几个房间名、在它们之间切。工作台最早自己做了这件事，
// 现在抽出来共用：**两份本地房间列表比一份更难解释**（同一个用户会问「我加的房间去哪了」）。
//
// ⚠️ localStorage 键从 `workbenchRooms` 改成了 `localRooms`，**旧键要读一次做迁移** ——
// 直接换键会让老用户本地那份列表凭空消失。
const STORAGE_KEY = 'localRooms';
const LEGACY_STORAGE_KEY = 'workbenchRooms';

function readStoredRooms() {
    for (const key of [STORAGE_KEY, LEGACY_STORAGE_KEY]) {
        try {
            const parsed = JSON.parse(localStorage.getItem(key) || 'null');
            if (Array.isArray(parsed) && parsed.length) {
                return parsed;
            }
        } catch {
            // 坏值就当没有，继续试下一个键
        }
    }
    return [];
}

export function useLocalRooms() {
    const ws = useWebSocketStore();
    const { t } = useI18n();

    const rooms = [];
    for (const room of readStoredRooms()) {
        const normalized = ws.normalizeRoomName(room);
        if (!rooms.includes(normalized)) {
            rooms.push(normalized);
        }
    }
    // 空串代表「公共房间」，永远排第一 —— 它是默认房间，不能从列表里消失。
    if (!rooms.includes('')) {
        rooms.unshift('');
    }
    const localRooms = ref(rooms);

    function persist() {
        try {
            localStorage.setItem(STORAGE_KEY, JSON.stringify(localRooms.value));
        } catch {
            // 存不下就算了，内存里仍然生效
        }
    }

    // 建房间 = 加进本地列表 + 切过去。返回是否成功（调用方据此决定关不关弹窗）。
    function createRoom(rawName) {
        const name = ws.normalizeRoomName(rawName);
        if (!name) {
            toast(t('workbenchRoomNameInvalid'));
            return false;
        }
        if (!localRooms.value.includes(name)) {
            localRooms.value.push(name);
            persist();
        }
        ws.switchRoom(name);
        toast(t('workbenchRoomCreated', { room: name }));
        return true;
    }

    // 只从本地列表里摘掉，**不删服务端的任何东西** —— 这里管的是「我记住哪几个房间」。
    function removeRoom(room) {
        const index = localRooms.value.indexOf(room);
        if (index === -1) {
            return;
        }
        localRooms.value.splice(index, 1);
        persist();
        // 删掉的正是当前房间 → 回公共房间，不然会停在一个已经从列表里消失的房间里。
        if (ws.room === room) {
            ws.switchRoom('');
        }
    }

    function switchToRoom(room) {
        ws.switchRoom(room);
    }

    return { localRooms, createRoom, removeRoom, switchToRoom };
}
