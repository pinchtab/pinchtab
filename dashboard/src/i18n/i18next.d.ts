import type { resources } from "./index";

/**
 * Makes `t()` keys type-checked: a key that is missing from en.json (or a typo)
 * fails `bun run typecheck` instead of rendering the raw key at runtime.
 */
declare module "i18next" {
  interface CustomTypeOptions {
    defaultNS: "translation";
    resources: (typeof resources)["en"];
  }
}
