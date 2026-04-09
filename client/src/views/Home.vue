<template>
    <v-container fluid class="home-minimal pa-3 pa-md-5" :class="{ 'home-minimal--dark': $vuetify.theme.dark }">
        <div class="home-minimal__shell mx-auto">
            <v-card class="composer-dock composer-dock--top px-3 px-md-4 py-3 mb-4" :class="{ 'surface-card--dark': $vuetify.theme.dark }" outlined>
                <unified-composer ref="composer" @show-qr="pageQrDialogVisible = true"></unified-composer>
            </v-card>

            <v-card class="timeline-panel" :class="{ 'surface-card--dark': $vuetify.theme.dark }" outlined>
                <div class="timeline-panel__body px-3 px-md-5 py-4">
                    <div v-if="$root.received.length" class="timeline-panel__stream">
                        <v-fade-transition group>
                            <div
                                v-for="item in $root.received"
                                :key="item.id"
                                class="timeline-panel__item"
                                :class="{ 'timeline-panel__item--first': item === $root.received[0] }"
                            >
                                <v-chip
                                    v-if="item === $root.received[0]"
                                    x-small
                                    outlined
                                    color="primary"
                                    class="timeline-panel__count-chip timeline-panel__count-chip--overlay"
                                >
                                    {{ $root.received.length }} {{ $t('messages') }}
                                </v-chip>
                                <component
                                    :is="item.type === 'text' ? 'received-text' : 'received-file'"
                                    :meta="item"
                                />
                            </div>
                        </v-fade-transition>
                    </div>

                    <v-sheet
                        v-if="!$root.received.length"
                        class="empty-timeline py-10 px-6 text-center"
                        :class="{ 'empty-timeline--dark': $vuetify.theme.dark }"
                        rounded="lg"
                        color="transparent"
                    >
                        <v-icon size="42" color="primary">{{ mdiTimeline }}</v-icon>
                        <div class="text-h6 font-weight-medium mt-4 mb-2">{{ $t('emptyTimelineTitle') }}</div>
                        <div class="body-2 text--secondary mb-4">{{ $t('timelineEmptySubtitle') }}</div>
                        <v-btn small depressed color="primary" @click="focusComposer('text')">
                            {{ $t('quickSend') }}
                        </v-btn>
                    </v-sheet>

                    <div v-else class="text-center caption text--secondary pt-2">{{ $t('alreadyAtBottom') }}</div>
                </div>
            </v-card>
        </div>

        <v-dialog v-model="pageQrDialogVisible" max-width="250">
            <v-card>
                <v-card-title class="headline justify-center">{{ $t('scanToAccessPage') }}</v-card-title>
                <v-card-text class="text-center pa-4">
                    <qrcode-vue :value="currentPageUrl" :size="200" level="H" />
                    <div class="text-caption mt-2" style="word-break: break-all;">{{ currentPageUrl }}</div>
                </v-card-text>
                <v-card-actions>
                    <v-spacer></v-spacer>
                    <v-btn color="primary" text @click="pageQrDialogVisible = false">{{ $t('close') }}</v-btn>
                </v-card-actions>
            </v-card>
        </v-dialog>
    </v-container>
</template>

<script>
import QrcodeVue from 'qrcode.vue';
import UnifiedComposer from '@/components/UnifiedComposer.vue';
import ReceivedText from '@/components/received-item/Text.vue';
import ReceivedFile from '@/components/received-item/File.vue';
import {
    mdiLanConnect,
    mdiLanPending,
    mdiLanDisconnect,
    mdiTimeline,
} from '@mdi/js';

export default {
    name: 'home',
    components: {
        QrcodeVue,
        UnifiedComposer,
        ReceivedText,
        ReceivedFile,
    },
    data() {
        return {
            pageQrDialogVisible: false,
            mdiLanConnect,
            mdiLanPending,
            mdiLanDisconnect,
            mdiTimeline,
        };
    },
    computed: {
        activeRoomName() {
            return this.$root.room || this.$t('publicRoom');
        },
        currentPageUrl() {
            return window.location.href;
        },
    },
    methods: {
        focusComposer(type) {
            this.$nextTick(() => {
                if (this.$refs.composer && typeof this.$refs.composer.focus === 'function') {
                    this.$refs.composer.focus(type);
                }
            });
        },
    },
    beforeRouteEnter(to, from, next) {
        next(vm => vm.$root.room = to.query.room || '');
    },
    beforeRouteUpdate(to, from, next) {
        this.$root.room = to.query.room || '';
        next();
    },
}
</script>

<style scoped>
.home-minimal {
    background: transparent;
    min-height: calc(100vh - 64px);
}

.home-minimal--dark {
    color: rgba(226, 232, 240, 0.96);
}

.home-minimal__shell {
    max-width: 980px;
}

.surface-card--dark,
.timeline-panel,
.composer-dock {
    border-radius: 20px;
    border-color: rgba(148, 163, 184, 0.24) !important;
    box-shadow: 0 12px 30px rgba(15, 23, 42, 0.05);
    background: rgba(255, 255, 255, 0.92);
    transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.surface-card--dark {
    border-color: rgba(71, 85, 105, 0.72) !important;
    box-shadow: 0 18px 36px rgba(2, 6, 23, 0.36);
    background: rgba(15, 23, 42, 0.92);
}

.timeline-panel {
    overflow: hidden;
}

.timeline-panel__body {
    min-height: 24rem;
}

.timeline-panel__stream {
    position: relative;
}

.timeline-panel__item {
    position: relative;
}

.timeline-panel__item--first {
    padding-top: 10px;
}

.timeline-panel__count-chip {
    height: 22px;
    padding: 0 6px;
    border-radius: 999px;
}

.timeline-panel__count-chip--overlay {
    position: absolute;
    top: 0;
    left: 50%;
    transform: translateX(-50%);
    z-index: 1;
    backdrop-filter: blur(8px);
    background: rgba(255, 255, 255, 0.78);
}

.home-minimal--dark .timeline-panel__count-chip--overlay {
    background: rgba(15, 23, 42, 0.78);
}

.empty-timeline {
    border: 1px dashed rgba(148, 163, 184, 0.35);
    background: rgba(248, 250, 252, 0.68) !important;
}

.empty-timeline--dark {
    border-color: rgba(71, 85, 105, 0.72);
    background: rgba(15, 23, 42, 0.42) !important;
}

.composer-dock {
    border-radius: 18px;
}

.composer-dock--top {
    position: sticky;
    top: calc(64px + 0.5rem);
    z-index: 2;
    box-shadow: 0 10px 24px rgba(15, 23, 42, 0.05);
}

@media (max-width: 1263px) {
    .composer-dock--top {
        top: calc(56px + 0.5rem);
    }
}

@media (max-width: 960px) {
    .home-minimal {
        min-height: calc(100vh - 56px);
    }

    .timeline-panel__stream {
        padding-left: 0;
    }

    .timeline-panel__count-chip--overlay {
        left: 50%;
    }
}
</style>