<template>
    <v-container fill-height>
        <v-row justify="center" align="center" class="pa-3">
            <div class="text-center">
                <p class="text-h5 my-4">{{ t('aboutTitle', { version: app.config.version }) }}</p>
                <p>{{ t('aboutDesc1') }}<br>{{ t('aboutDesc2') }}</p>
                <p><a href="https://github.com/Jonnyan404/cloud-clipboard-go" target="_blank"><img src="https://img.shields.io/github/stars/jonnyan404/cloud-clipboard-go.svg?style=social"></a></p>
                <p class="text-medium-emphasis"><small>{{ t('donatePrompt') }}</small></p>
            </div>
        </v-row>
    </v-container>
</template>

<script setup>
import { useAppStore } from '@/store/app';
import { useI18n } from 'vue-i18n';

const app = useAppStore();
const { t } = useI18n();
</script>