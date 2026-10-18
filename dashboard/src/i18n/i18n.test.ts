import { describe, expect, it } from "vitest";
import i18n, { resources } from "./index";

describe("i18n bootstrap", () => {
  it("initialises synchronously with the english bundle", () => {
    // The dashboard renders on the first tick, so a lazy init would flash raw
    // keys; `initAsync: false` is what keeps this true.
    expect(i18n.isInitialized).toBe(true);
    expect(i18n.t("app.name")).toBe("PinchTab");
  });

  it("ships exactly one language", () => {
    expect(Object.keys(resources)).toEqual(["en"]);
    // i18next always adds its built-in `cimode` pseudo-locale for debugging.
    const realLanguages = (i18n.options.supportedLngs ?? []).filter(
      (lng) => lng !== "cimode",
    );
    expect(realLanguages).toEqual(["en"]);
    expect(i18n.resolvedLanguage).toBe("en");
  });
});
