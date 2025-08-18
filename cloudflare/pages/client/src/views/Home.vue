<template>
    <v-container>
        <v-row>
            <!-- Left Column: Sending Area & QR Code (Hidden on small screens) -->
            <v-col cols="12" md="4" class="hidden-sm-and-down">
                <send-text ref="sendTextDesktop"></send-text> <!-- Added ref -->
                <v-divider class="my-4"></v-divider>
                <send-file ref="sendFileDesktop"></send-file> <!-- Added ref -->
                <v-divider class="my-4"></v-divider> <!-- Divider before QR Code -->
                <!-- Page QR Code Section -->
                <v-card outlined class="mt-4 pa-4 text-center">
                    <v-subheader class="justify-center">{{ $t('scanToAccessPage') }}</v-subheader>
                    <qrcode-vue :value="currentPageUrl" :size="150" level="H" />
                    <div class="text-caption mt-2" style="word-break: break-all;">{{ currentPageUrl }}</div>
                </v-card>
                <!-- End Page QR Code Section -->
            </v-col>

            <!-- Right Column: Receiving Area -->
            <v-col cols="12" md="8">
                <v-fade-transition group>
                    <component
                        v-for="item in $root.received"
                        :key="item.id"
                        :is="item.type === 'text' ? 'received-text' : 'received-file'"
                        :meta="item"
                    />
                </v-fade-transition>
                <div class="text-center caption text--secondary py-2">{{ $root.received.length ? $t('alreadyAtBottom') : $t('emptyHere') }}</div>
            </v-col>
        </v-row>

        <!-- Speed Dial for Mobile -->
        <v-speed-dial
            v-model="fab"
            bottom
            right
            fixed
            direction="top"
            transition="scale-transition"
            class="hidden-md-and-up"
            style="transform:translateY(-64px)"
        >
            <template v-slot:activator>
                <v-btn
                    v-model="fab"
                    fab
                    dark
                    color="primary"
                >
                    <v-icon v-if="fab">{{ mdiClose }}</v-icon> <!-- Change icon when open -->
                    <v-icon v-else>{{ mdiPlus }}</v-icon>
                </v-btn>
            </template>
            <!-- Show Page QR Code Button -->
            <v-tooltip left>
                 <template v-slot:activator="{ on }">
                    <v-btn fab dark small color="indigo" v-on="on" @click="pageQrDialogVisible = true">
                        <v-icon>{{ mdiQrcode }}</v-icon>
                    </v-btn>
                 </template>
                 <span>{{ $t('showQrCode') }}</span>
            </v-tooltip>
            <!-- Send File Button -->
            <v-tooltip left>
                 <template v-slot:activator="{ on }">
                    <v-btn fab dark small color="green" v-on="on" @click="openDialog('file')">
                        <v-icon>{{ mdiFileDocumentOutline }}</v-icon>
                    </v-btn>
                 </template>
                 <span>{{ $t('sendFile') }}</span>
            </v-tooltip>
            <!-- Send Text Button -->
            <v-tooltip left>
                 <template v-slot:activator="{ on }">
                    <v-btn fab dark small color="red" v-on="on" @click="openDialog('text')">
                        <v-icon>{{ mdiText }}</v-icon>
                    </v-btn>
                 </template>
                 <span>{{ $t('sendText') }}</span>
            </v-tooltip>
        </v-speed-dial>

        <!-- Fullscreen Dialog for Mobile Sending -->
        <v-dialog
            v-model="dialog"
            fullscreen
            hide-overlay
            transition="dialog-bottom-transition"
            scrollable
        >
            <v-card>
                <v-toolbar dark color="primary" class="flex-grow-0">
                    <v-btn icon @click="closeDialog">
                        <v-icon>{{mdiClose}}</v-icon>
                    </v-btn>
                    <v-toolbar-title v-if="mode === 'text'">{{ $t('sendText') }}</v-toolbar-title>
                    <v-toolbar-title v-if="mode === 'file'">{{ $t('sendFile') }}</v-toolbar-title>
                    <v-spacer></v-spacer>
                    <v-tooltip bottom>
                        <template v-slot:activator="{ on }">
                            <v-btn icon v-on="on" @click="reconnect">
                                <v-icon v-if="$root.websocket">{{mdiLanConnect}}</v-icon>
                                <v-icon v-else-if="$root.websocketConnecting">{{mdiLanPending}}</v-icon>
                                <v-icon v-else>{{mdiLanDisconnect}}</v-icon>
                            </v-btn>
                        </template>
                        <span v-if="$root.websocket">{{ $t('connected') }}</span>
                        <span v-else-if="$root.websocketConnecting">{{ $t('connecting') }}</span>
                        <span v-else>{{ $t('disconnected') }}</span>
                    </v-tooltip>
                </v-toolbar>
                <v-card-text class="px-4">
                    <div class="my-4">
                        <send-text ref="sendTextDialog" v-if="mode === 'text'"></send-text> <!-- Changed ref -->
                        <send-file ref="sendFileDialog" v-if="mode === 'file'"></send-file> <!-- Changed ref -->
                    </div>
                </v-card-text>
            </v-card>
        </v-dialog>

        <!-- Page QR Code Dialog -->
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
import SendText from '@/components/SendText.vue';
import SendFile from '@/components/SendFile.vue';
import ReceivedText from '@/components/received-item/Text.vue';
import ReceivedFile from '@/components/received-item/File.vue';
import {
    mdiPlus,
    mdiFileDocumentOutline,
    mdiText,
    mdiClose,
    mdiQrcode,
    mdiLanConnect,
    mdiLanPending,
    mdiLanDisconnect,
} from '@mdi/js';

export default {
    name: 'home',
    components: {
        QrcodeVue,
        SendText,
        SendFile,
        ReceivedText,
        ReceivedFile,
    },
    data() {
        return {
            fab: false,
            dialog: false,
            mode: null,
            pageQrDialogVisible: false, // <-- Add dialog visibility flag
            mdiPlus,
            mdiFileDocumentOutline,
            mdiText,
            mdiClose,
            mdiQrcode,
            mdiLanConnect,
            mdiLanPending,
            mdiLanDisconnect,
        };
    },
    computed: {
        currentPageUrl() {
            const currentUrl = window.location.href;
            if (this.$root.room) {
                return currentUrl;
            }
            else {
                return currentUrl;
            }
        }
    },
    methods: {
        reconnect() {
            if (!this.$root.websocket && !this.$root.websocketConnecting) {
                this.$root.retry = 0;
                this.$root.connect();
            }
        },
        openDialog(type) {
            this.mode = type;
            this.dialog = true;
            this.$nextTick(() => {
                 setTimeout(() => {
                    if (type === 'text' && this.$refs.sendTextDialog) {
                        this.$refs.sendTextDialog.focus();
                    } else if (type === 'file' && this.$refs.sendFileDialog) {
                         if (typeof this.$refs.sendFileDialog.focus === 'function') {
                             this.$refs.sendFileDialog.focus();
                         }
                    }
                }, 300);
            });
        },
        closeDialog() {
            this.dialog = false;
        },
        handlePopState(event) {
            if (this.dialog && (!event.state || !event.state.dialogOpen)) {
                this.closeDialog();
            }
        }
    },
    watch: {
        dialog(newval, oldval) {
            if (newval && !oldval) {
                history.pushState({ dialogOpen: true }, null);
                window.addEventListener('popstate', this.handlePopState);
            } else if (!newval && oldval) {
                window.removeEventListener('popstate', this.handlePopState);
                if (history.state && history.state.dialogOpen) {
                     history.back();
                }
            }
        },
         '$root.received': function() {
            this.$nextTick(() => {
                const scrollThreshold = 200;
                if (document.documentElement.scrollHeight - window.innerHeight - window.scrollY < scrollThreshold) {
                     window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
                }
            });
        },
    },
    beforeDestroy() { // Added beforeDestroy for cleanup
        window.removeEventListener('popstate', this.handlePopState);
    },
    beforeRouteEnter(to, from, next) { // Kept route hooks
        next(vm => vm.$root.room = to.query.room || '');
    },
    beforeRouteUpdate(to, from, next) { // Kept route hooks
        this.$root.room = to.query.room || '';
        next();
    },
}
</script>