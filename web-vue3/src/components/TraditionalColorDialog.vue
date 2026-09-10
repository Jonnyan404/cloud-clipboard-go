<template>
    <v-dialog v-model="visible" max-width="380" scrollable>
<v-card class="cc-color-dialog">
                <v-card-title>{{ t('traditionalColors') }}</v-card-title>
                <v-card-text class="pt-0">
                    <div class="cc-color-dialog__divider-text">
                        742
                    </div>

                <v-expansion-panels
                    v-model="openGroups"
                    multiple
                    flat
                    class="cc-color-dialog__panels"
                >
                    <v-expansion-panel
                        v-for="group in traditionalColorGroups"
                        :key="group.name"
                        :value="group.name"
                    >
                        <v-expansion-panel-title class="cc-color-dialog__group-title">
                            {{ t(group.name) }}
                            <span class="cc-color-dialog__group-count">{{ group.colors.length }}</span>
                        </v-expansion-panel-title>
                        <v-expansion-panel-text>
                            <div class="cc-color-dialog__swatches">
                                <button
                                    v-for="c in group.colors"
                                    :key="c.hex"
                                    type="button"
                                    class="cc-color-dialog__swatch"
                                    :class="{ 'cc-color-dialog__swatch--active': c.hex.toLowerCase() === currentPrimary.toLowerCase() }"
                                    :style="{ background: c.hex }"
                                    :aria-label="c.name + ' ' + c.hex"
                                    :title="c.name + ' ' + c.hex"
                                    @click="setTraditionalColor(c.hex)"
                                ></button>
                            </div>
                        </v-expansion-panel-text>
                    </v-expansion-panel>
                </v-expansion-panels>
            </v-card-text>
            <v-card-actions>
                <v-spacer></v-spacer>
                <v-btn color="primary" variant="text" @click="visible = false">{{ t('ok') }}</v-btn>
            </v-card-actions>
        </v-card>
    </v-dialog>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useTheme } from 'vuetify';
import { useI18n } from 'vue-i18n';
import { traditionalColorGroups } from '@/data/traditionalColors';

const visible = defineModel({ type: Boolean, default: false });

const theme = useTheme();
const { t } = useI18n();
const isDark = computed(() => theme.current.value?.dark ?? false);
const currentPrimary = computed(() => isDark.value ? theme.themes.value.dark.colors.primary : theme.themes.value.light.colors.primary);
const openGroups = ref(traditionalColorGroups.map(g => g.name));

function setTraditionalColor(hex) {
    if (isDark.value) {
        theme.themes.value.dark.colors.primary = hex;
    } else {
        theme.themes.value.light.colors.primary = hex;
    }
}
</script>

<style scoped>
.cc-color-dialog__divider-text {
    margin: 16px 0 10px;
    padding-top: 12px;
    border-top: 1px solid rgba(var(--v-theme-on-background), 0.12);
    font-size: 13px;
    font-weight: 500;
    color: rgb(var(--v-theme-on-background));
    opacity: 0.75;
}

.cc-color-dialog__panels :deep(.v-expansion-panel) {
    background: transparent;
    border-bottom: 1px solid rgba(var(--v-theme-on-background), 0.1);
}

.cc-color-dialog__panels :deep(.v-expansion-panel-title) {
    min-height: 40px;
    padding: 8px 0;
}

.cc-color-dialog__group-title {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: rgb(var(--v-theme-on-background));
    opacity: 0.85;
}

.cc-color-dialog__group-count {
    font-size: 11px;
    color: rgb(var(--v-theme-on-background));
    opacity: 0.45;
}

.cc-color-dialog__panels :deep(.v-expansion-panel-text__wrapper) {
    padding: 8px 0 0;
}

:deep(.cc-color-dialog .v-card-text) {
    max-height: 64vh;
    overflow-y: auto;
}

.cc-color-dialog__swatches {
    display: grid;
    grid-template-columns: repeat(10, 1fr);
    gap: 4px;
    padding: 2px 0 10px;
}

.cc-color-dialog__swatch {
    aspect-ratio: 1;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    padding: 0;
    outline: 2px solid transparent;
    outline-offset: 1px;
    transition: outline-color 0.15s ease;
}

.cc-color-dialog__swatch:hover {
    outline-color: rgb(var(--v-theme-on-background));
}

.cc-color-dialog__swatch--active {
    outline-color: rgb(var(--v-theme-primary) / 1) !important;
    outline-width: 3px;
}
</style>