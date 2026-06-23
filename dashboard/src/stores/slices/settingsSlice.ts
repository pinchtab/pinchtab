import type { StateCreator } from "zustand";
import type { Settings } from "../../generated/types";
import type { AppState } from "../useAppStore";

const defaultSettings: Settings = {
  screencast: { fps: 1, quality: 40, maxWidth: 800 },
  stealth: "light",
  browser: { blockImages: false, blockMedia: false, noAnimations: false },
  monitoring: { memoryMetrics: false, pollInterval: 30 },
  agents: { reasoningMode: "tool_calls" },
};

const SETTINGS_KEY = "pinchtab_settings";

function loadSettings(): Settings {
  try {
    const saved = localStorage.getItem(SETTINGS_KEY);
    if (saved) {
      return { ...defaultSettings, ...JSON.parse(saved) };
    }
  } catch {
    // ignore parse errors
  }
  return defaultSettings;
}

function saveSettings(settings: Settings) {
  try {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
  } catch {
    // ignore storage errors
  }
}

export interface SettingsSlice {
  settings: Settings;
  setSettings: (settings: Settings) => void;
}

export const createSettingsSlice: StateCreator<
  AppState,
  [],
  [],
  SettingsSlice
> = (set) => ({
  settings: loadSettings(),
  setSettings: (settings) => {
    saveSettings(settings);
    set({ settings });
  },
});
