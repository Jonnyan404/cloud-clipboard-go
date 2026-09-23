import DefaultMode from './DefaultMode.vue';
import GlanceWall from './GlanceWall.vue';
import BenchWall from './BenchWall.vue';
import StickyWall from './StickyWall.vue';
import MegaWall from './MegaWall.vue';
import TerminalWall from './TerminalWall.vue';
import WorkbenchWall from './WorkbenchWall.vue';
import ChatWall from './ChatWall.vue';
import BoardWall from './BoardWall.vue';

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
        //
        // ⚠️ 中文名是「动作台」而不是「动作工作台」：已有的 workbench 模式中文就叫「工作台」
        // （它即将下架，但还在菜单里），两个「工作台」并排会让人分不清谁是谁。
        key: 'bench',
        labelKey: 'uiModeBench',
        icon: 'mdi-auto-fix',
        component: BenchWall,
    },
    {
        key: 'chat',
        labelKey: 'uiModeChat',
        icon: 'mdi-chat-outline',
        component: ChatWall,
        // ⚠️ 即将下架：**只影响菜单里的分组，功能一切照旧** —— 仍然可选、地址参数照旧、
        // 组件照常渲染。标记而不是直接删除，是因为 localStorage 和书签里可能还存着
        // `?mode=chat`，删掉会让它们静默落到兜底模式（用户只会看到「我的模式没了」）。
        //
        // 消费点有两处，都读这个字段，别各写一份判断：
        //   · PageToolbar 的模式下拉框 —— 折到「即将下架」分组，用一条分割线隔开
        //   · App.vue 个性化面板里的模式按钮组 —— 同样弱化显示
        deprecated: true,
    },
    {
        key: 'sticky',
        labelKey: 'uiModeSticky',
        icon: 'mdi-pin-outline',
        component: StickyWall,
    },
    {
        key: 'mega',
        labelKey: 'uiModeMega',
        icon: 'mdi-newspaper-variant-outline',
        component: MegaWall,
        deprecated: true, // 理由见上面 chat 那一项
    },
    {
        key: 'workbench',
        labelKey: 'uiModeWorkbench',
        icon: 'mdi-view-column-outline',
        component: WorkbenchWall,
        deprecated: true,
    },
    {
        key: 'terminal',
        labelKey: 'uiModeTerminal',
        icon: 'mdi-console-line',
        component: TerminalWall,
        deprecated: true,
    },
    {
        // 看板放在最后：它是最新加的一个，而且和上面几个的定位不太一样 ——
        // 那几个都是「同一份内容的不同排版」，看板多了一层「这条在哪一列」的状态。
        key: 'board',
        labelKey: 'uiModeBoard',
        icon: 'mdi-view-column-outline',
        component: BoardWall,
    },
];

export function resolveModeComponent(key) {
    return MODES.find(mode => mode.key === key)?.component || DefaultMode;
}