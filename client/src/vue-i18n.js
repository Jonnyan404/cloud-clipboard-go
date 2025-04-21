import Vue from 'vue';
import VueI18n from 'vue-i18n';
import en from './locales/en.json';
import zh from './locales/zh.json';
import zhTW from './locales/zh-TW.json';
import ja from './locales/ja.json';

Vue.use(VueI18n);

const messages = {
  en,
  zh,
  'zh-TW': zhTW, // 添加繁体中文
  ja,           // 添加日文
};

// 检测浏览器语言或默认使用中文
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
  // 默认为英文
  return 'en';
}

const i18n = new VueI18n({
  locale: localStorage.getItem('locale') || getBrowserLocale(), // 优先从localStorage读取，否则根据浏览器判断
  fallbackLocale: 'en', // 回退语言
  messages, // 语言包
});

export default i18n;