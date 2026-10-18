import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./locales/en.json";

/**
 * Dashboard i18n bootstrap.
 *
 * The dashboard ships English only, but every user-visible string goes through
 * `t()` so a translation can be added without touching components. Deliberately
 * absent: language detection, a language switcher, lazy language chunks and any
 * persistence — one language, one statically imported bundle.
 */
export const resources = {
  en: { translation: en },
} as const;

void i18n.use(initReactI18next).init({
  resources,
  lng: "en",
  fallbackLng: "en",
  supportedLngs: ["en"],
  defaultNS: "translation",
  // `initAsync: false` makes the initialisation synchronous, so the first
  // render already has its translations and never flashes a bare key.
  initAsync: false,
  interpolation: {
    // React escapes interpolated values itself.
    escapeValue: false,
  },
});

export default i18n;
