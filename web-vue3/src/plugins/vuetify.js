import { createVuetify } from 'vuetify';
import { aliases, mdi } from 'vuetify/iconsets/mdi';
import * as components from 'vuetify/components';
import * as directives from 'vuetify/directives';
import 'vuetify/styles';
import '@mdi/font/css/materialdesignicons.css';

export default createVuetify({
    components,
    directives,
    icons: {
        defaultSet: 'mdi',
        aliases,
        sets: { mdi },
    },
    theme: {
        defaultTheme: 'light',
        themes: {
            light: { colors: { primary: '#1e88e5', secondary: '#424242', accent: '#82b1ff' } },
            dark: { colors: { primary: '#1e88e5', secondary: '#424242', accent: '#82b1ff' } },
        },
    },
});
