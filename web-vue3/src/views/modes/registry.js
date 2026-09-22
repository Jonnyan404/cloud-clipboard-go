import DefaultMode from './DefaultMode.vue';
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
        key: 'chat',
        labelKey: 'uiModeChat',
        icon: 'mdi-chat-outline',
        component: ChatWall,
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
    },
    {
        key: 'workbench',
        labelKey: 'uiModeWorkbench',
        icon: 'mdi-view-column-outline',
        component: WorkbenchWall,
    },
    {
        key: 'terminal',
        labelKey: 'uiModeTerminal',
        icon: 'mdi-console-line',
        component: TerminalWall,
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