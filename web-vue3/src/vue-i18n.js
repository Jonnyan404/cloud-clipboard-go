import { createI18n } from 'vue-i18n';
import en from './locales/en.json';
import zh from './locales/zh.json';
import zhTW from './locales/zh-TW.json';
import ja from './locales/ja.json';

const messages = {
  en,
  zh,
  'zh-TW': zhTW,
  ja,
};

function getBrowserLocale() {
  const navigatorLocale = navigator.language || navigator.userLanguage;
  const lang = navigatorLocale.toLowerCase();

  if (lang.startsWith('zh-tw') || lang.startsWith('zh-hk')) {
    return 'zh-TW';
  }
  if (lang.startsWith('zh')) {
    return 'zh';
  }
  if (lang.startsWith('ja')) {
    return 'ja';
  }
  return 'en';
}

const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: localStorage.getItem('locale') || getBrowserLocale(),
  fallbackLocale: 'en',
  messages,
});

export default i18n;
