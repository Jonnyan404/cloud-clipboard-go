import DefaultMode from './DefaultMode.vue';
import GlanceWall from './GlanceWall.vue';
import BenchWall from './BenchWall.vue';
import StickyWall from './StickyWall.vue';
import BoardWall from './BoardWall.vue';

/**
 * ⚠️★ **已退役的模式**（2026-09-26 真删了，不再是「标记」）。
 *
 * 原来它们是 `deprecated: true` —— 标记而不是删，理由是：
 * 「localStorage 和书签里可能还存着 `?mode=chat`，删掉会让它们**静默**落到兜底模式，
 * 用户只会看到「我的模式没了」」（见 git 历史里 registry.js 那段注释）。
 *
 * 现在真删了，但**那条顾虑照旧成立** —— 所以留这张表：旧 key 显式映射到一个
 * **还在的模式**，让降级是**有意的**，而不是靠 `resolveModeComponent` 的兜底碰巧生效。
 *
 * ⚠️ 别把这张表也删掉：删了之后 `?mode=chat` 会落到 `default`（兜底），
 * 症状和原来一样是「模式没了」，但那时**没人知道它曾经是什么、该换成什么**。
 *
 * 为什么各自映射到那个模式（按「它原来是干什么的」挑最近的）：
 *   · `chat`      → `default`   —— 都是「一列气泡/卡片」的时间流
 *   · `mega`      → `default`   —— 大字版时间流
 *   · `workbench` → `bench`     —— 多栏；动作台是还在的多栏模式
 *   · `terminal`  → `default`   —— 终端风的一列时间流
 */
export const RETIRED_MODES = {
    chat: 'default',
    mega: 'default',
    workbench: 'bench',
    terminal: 'default',
};

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
 * 把模式 key 解析成组件。
 *
 * ⚠️ 退役的 key **先过 `RETIRED_MODES`** —— 见那张表的注释：旧链接要**有意**降级。
 */
export function resolveModeComponent(key) {
    const effective = RETIRED_MODES[key] || key;
    return MODES.find(mode => mode.key === effective)?.component || DefaultMode;
}
