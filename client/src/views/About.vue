<template>
    <v-container fill-height>
        <v-row justify="center" align="center" class="pa-3">
            <div class="text-center">
                <p class="headline my-4">{{ $t('aboutTitle', { version: $root.config.version }) }}</p>
                <p>{{ $t('aboutDesc1') }}<br>{{ $t('aboutDesc2') }}</p>
                <p><a href="https://github.com/Jonnyan404/cloud-clipboard-go" target="_blank"><img src="https://img.shields.io/github/stars/TransparentLC/cloud-clipboard.svg?style=social"></a></p>
                <p class="text--secondary"><small>{{ $t('donatePrompt') }}</small></p>
            </div>
        </v-row>
    </v-container>
</template>

<script>
export default {

}
</script>