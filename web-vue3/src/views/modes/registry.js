import DefaultMode from './DefaultMode.vue';
import GlanceWall from './GlanceWall.vue';
import BenchWall from './BenchWall.vue';
import StickyWall from './StickyWall.vue';
import BoardWall from './BoardWall.vue';

/**
 * ⚠️★ **四个模式已退役并真删**（2026-09-26）：`chat` / `mega` / `workbench` / `terminal`。
 *
 * 它们原来是 `deprecated: true` —— 只标记不删，理由是「localStorage 和书签里可能还存着
 * `?mode=chat`，删掉会让它们**静默**落到兜底模式」。
 *
 * ⚠️ **那条理由已经不成立了**（Jonny 2026-09-26：「统统删除，不要考虑老用户，没有老用户」）。
 * 所以**没有退役映射表、没有兼容分支** —— 旧的 `?mode=chat` 就是会落到 `default`（兜底）。
 * 别为了「优雅降级」再把那张表加回来：这个项目**没有老用户**，
 * 而兼容垫片是**永久成本**（它会一直躺在代码里，让下一个人以为还有人依赖它）。
 */

export const MODES = [
    {
        key: 'default',
        labelKey: 'uiModeDefault',
        // 原来用 mdi-view-dashboard-outline（2×2 宫格）：那画的是「网格」，
        // 而这个模式是**竖着排的时间流卡片**，宫格更像巨型/工作台那种多栏布局，指错了。
        // view-stream 是一列竖着堆的块，跟实际长相一致。
        icon: 'mdi-view-stream-outline',
        component: DefaultMode,
    },
    {
        // 速览紧跟标准模式：**同一批条目的同一个视图**，区别只在「发不发东西」——
        // 标准模式发送区占了大半屏，速览没有发送区、把搜索和分类提到最前。
        // 挨着放是因为用户会来回比这两个（而不是因为它是最新加的）。
        key: 'glance',
        labelKey: 'uiModeGlance',
        icon: 'mdi-magnify-scan',
        component: GlanceWall,
    },
    {
        // 动作台也是主从两栏，所以挨着速览放 —— 区别在右边那一栏的**性质**：
        // 速览是「读」（渲染好的完整预览，只读），动作台是「加工」（可叠多步的动作链 + 实时结果）。
        key: 'bench',
        labelKey: 'uiModeBench',
        icon: 'mdi-auto-fix',
        component: BenchWall,
    },
    {
        key: 'sticky',
        labelKey: 'uiModeSticky',
        icon: 'mdi-pin-outline',
        component: StickyWall,
    },
    {
        // 看板放在最后：它和上面几个的定位不太一样 ——
        // 那几个都是「同一份内容的不同排版」，看板多了一层「这条在哪一列」的状态。
        key: 'board',
        labelKey: 'uiModeBoard',
        icon: 'mdi-view-column-outline',
        component: BoardWall,
    },
];

/**
 * 把模式 key 解析成组件。认不出的 key（含已退役那四个）落到 `DefaultMode`。
 *
 * ⚠️ **不要**为退役的 key 加映射 —— 见上面那段注释（这个项目没有老用户）。
 */
export function resolveModeComponent(key) {
    return MODES.find(mode => mode.key === key)?.component || DefaultMode;
}
