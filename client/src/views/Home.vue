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
                    <v-icon>{{mdiPlus}}</v-icon>
                </v-btn>
            </template>
            <v-btn fab dark small color="primary" @click="openDialog('file')">
                <v-icon>{{mdiFileDocumentOutline}}</v-icon>
            </v-btn>
            <v-btn fab dark small color="primary" @click="openDialog('text')">
                <v-icon>{{mdiText}}</v-icon>
            </v-btn>
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
                </v-toolbar>
                <v-card-text class="px-4">
                    <div class="my-4">
                        <send-text ref="sendTextDialog" v-if="mode === 'text'"></send-text> <!-- Changed ref -->
                        <send-file ref="sendFileDialog" v-if="mode === 'file'"></send-file> <!-- Changed ref -->
                    </div>
                </v-card-text>
            </v-card>
        </v-dialog>
    </v-container>
</template>

<script>
import QrcodeVue from 'qrcode.vue'; // Import QR Code
import SendText from '@/components/SendText.vue';
import SendFile from '@/components/SendFile.vue';
import ReceivedText from '@/components/received-item/Text.vue';
import ReceivedFile from '@/components/received-item/File.vue';
import {
    mdiPlus,
    mdiFileDocumentOutline,
    mdiText,
    mdiClose,
} from '@mdi/js';

export default {
    name: 'home', // Added name
    components: {
        QrcodeVue, // Register QR Code
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
            mdiPlus,
            mdiFileDocumentOutline,
            mdiText,
            mdiClose,
        };
    },
    computed: {
        currentPageUrl() {
            const origin = window.location.origin; // e.g., http://localhost:8080
            const path = this.$route.fullPath;     // e.g., /?room=abc or /

            // Check if the application is likely using hash mode by looking at the current URL
            const usingHash = window.location.href.includes('#');

            if (usingHash) {
                // Construct URL with hash: origin + /# + path
                // Ensure path starts with '/'
                const formattedPath = path.startsWith('/') ? path : '/' + path;
                return origin + '/#' + formattedPath;
                // Example: http://localhost:8080/#/?room=abc
            } else {
                // Assume history mode: origin + path
                // Ensure path starts with '/'
                const formattedPath = path.startsWith('/') ? path : '/' + path;
                return origin + formattedPath;
                // Example: http://localhost:8080/?room=abc
            }
        }
    },
    methods: {
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
        // Removed custom setTimeout
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
    // Removed created hook, handlePopState defined in methods
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