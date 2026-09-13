<script setup>import { computed } from 'vue';
import { useAppStore } from '@/store/app';
import { useTheme } from 'vuetify';
import { resolveModeComponent } from '@/views/modes/registry.js';

const app = useAppStore();
const theme = useTheme();
const isDark = computed(() => theme.current.value?.dark ?? false);

const viewComponent = computed(() => resolveModeComponent(app.uiMode));
</script>

<template>
    <div class="mode-root" :class="{ 'mode-root--dark': isDark }">
        <component :is="viewComponent"></component>
    </div>
</template>

<style scoped>
.mode-root {
    min-height: 100vh;
}
</style>