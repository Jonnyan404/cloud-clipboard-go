import DefaultMode from './DefaultMode.vue';
import StickyWall from './StickyWall.vue';

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
];

export function resolveModeComponent(key) {
    return MODES.find(mode => mode.key === key)?.component || DefaultMode;
}