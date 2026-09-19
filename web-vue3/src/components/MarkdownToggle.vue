<script setup>
import { useI18n } from 'vue-i18n';

// 预览框右上角那对「原文 / Markdown」切换图标。
//
// 浮动定位（absolute）由组件自己带，所以调用方的预览容器必须是 position: relative ——
// 这样图标不占布局、直接叠在内容右上角。
defineProps({
    mode: {
        type: String,
        default: 'raw',
    },
});
const emit = defineEmits(['update:mode']);
const { t } = useI18n();

const mdiCodeTags = 'mdi-code-tags';
const mdiLanguageMarkdown = 'mdi-language-markdown';
</script>

<template>
    <div class="md-toggle">
        <v-btn-group density="compact" variant="text" divided>
            <v-tooltip :text="t('rawText')" location="top">
                <template v-slot:activator="{ props: activatorProps }">
                    <v-btn
                        v-bind="activatorProps"
                        size="small"
                        :color="mode === 'raw' ? 'primary' : undefined"
                        @click="emit('update:mode', 'raw')"
                    >
                        <v-icon size="18">{{ mdiCodeTags }}</v-icon>
                    </v-btn>
                </template>
            </v-tooltip>
            <v-tooltip :text="t('renderMarkdown')" location="top">
                <template v-slot:activator="{ props: activatorProps }">
                    <v-btn
                        v-bind="activatorProps"
                        size="small"
                        :color="mode === 'md' ? 'primary' : undefined"
                        @click="emit('update:mode', 'md')"
                    >
                        <v-icon size="18">{{ mdiLanguageMarkdown }}</v-icon>
                    </v-btn>
                </template>
            </v-tooltip>
        </v-btn-group>
    </div>
</template>

<style scoped>
/* 浮在预览框右上角：不占布局，内容该多高还多高。
   底色用 surface 的半透明 —— 否则浮在标题或代码块上会看不清（明暗主题都成立）。 */
.md-toggle {
    position: absolute;
    top: 0;
    right: 0;
    z-index: 2;
    background: rgba(var(--v-theme-surface), 0.85);
    border-radius: 8px;
    backdrop-filter: blur(2px);
}
</style>
