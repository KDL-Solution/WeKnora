import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN.ts'
import ruRU from './locales/ru-RU.ts'
import enUS from './locales/en-US.ts'
import koKR from './locales/ko-KR.ts'

const messages = {
  'zh-CN': zhCN,
  'en-US': enUS,
  'ru-RU': ruRU,
  'ko-KR': koKR
}

const defaultLocale = 'ko-KR'
const storedLocale = localStorage.getItem('locale')
const localeExplicitlySelected = localStorage.getItem('localeExplicitlySelected') === 'true'
const savedLocale = storedLocale && (localeExplicitlySelected || storedLocale !== 'zh-CN') ? storedLocale : defaultLocale
localStorage.setItem('locale', savedLocale)
console.log('i18n initialized with locale:', savedLocale)

const i18n = createI18n({
  legacy: false,
  locale: savedLocale,
  fallbackLocale: 'ko-KR',
  globalInjection: true,
  // Some translations intentionally embed `<strong>` markup (e.g. agent step summaries).
  // We render them via v-html with our own sanitization, so silence vue-i18n's HTML warning
  // to avoid flooding the console and slowing renders during history loads.
  warnHtmlMessage: false,
  messages
})

export default i18n
