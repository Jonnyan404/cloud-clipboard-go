import Vue from 'vue';
import Vuetify, { VSnackbar, VBtn, VIcon, VSlider, VChip } from 'vuetify/lib';
import VuetifyToast from 'vuetify-toast-snackbar';

const vuetify = new Vuetify({
    icons: {
        iconfont: 'mdiSvg',
    },
});


Vue.use(Vuetify, {
    components: {
        VSnackbar,
        VBtn,
        VIcon,
        VSlider,
        VChip,
    }
});
Vue.use(VuetifyToast, {
    $vuetify: vuetify.framework,
    x: '',
    color: '',
    closeText: '关闭',
    closeColor: 'accent',
});

export default vuetify;