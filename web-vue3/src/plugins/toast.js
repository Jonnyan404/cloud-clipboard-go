import { reactive } from 'vue';

export const toastState = reactive({
    visible: false,
    text: '',
    color: '',
    timeout: 3000,
});

let showClose = false;
let dismissable = false;
let forever = false;

export function toast(text, options = {}) {
    toastState.text = text;
    toastState.color = options.color || '';
    toastState.timeout = options.timeout ?? (options.forever || options.dismissable ? -1 : 3000);
    toastState.visible = true;
    showClose = options.showClose ?? false;
    dismissable = options.dismissable ?? false;
    forever = options.forever ?? false;
}

toast.error = (text, options = {}) => {
    toast(text, { ...options, color: 'error' });
};
toast.success = (text, options = {}) => {
    toast(text, { ...options, color: 'success' });
};
toast.info = (text, options = {}) => {
    toast(text, { ...options, color: 'info' });
};

export { showClose, dismissable, forever };
