import DefaultMode from './DefaultMode.vue';
import StickyWall from './StickyWall.vue';
import MegaWall from './MegaWall.vue';
import TerminalWall from './TerminalWall.vue';

export const MODES = [
    {
        key: 'default',
        labelKey: 'uiModeDefault',
        icon: 'mdi-view-dashboard-outline',
        component: DefaultMode,
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
        key: 'terminal',
        labelKey: 'uiModeTerminal',
        icon: 'mdi-console-line',
        component: TerminalWall,
    },
];

export function resolveModeComponent(key) {
    return MODES.find(mode => mode.key === key)?.component || DefaultMode;
}