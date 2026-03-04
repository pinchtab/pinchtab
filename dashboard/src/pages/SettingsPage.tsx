import { useState, useMemo } from "react";
import { useAppStore } from "../stores/useAppStore";
import { Button, Card } from "../components/atoms";
import { ServerSummary } from "../components/molecules";
import type { Settings } from "../types";

export default function SettingsPage() {
  const { settings, setSettings, serverInfo } = useAppStore();
  const [local, setLocal] = useState<Settings>(settings);

  // Check if settings have changed
  const hasChanges = useMemo(
    () => JSON.stringify(local) !== JSON.stringify(settings),
    [local, settings],
  );

  const handleSave = () => {
    // Settings are persisted to localStorage via setSettings
    setSettings(local);
  };

  const handleReset = () => setLocal(settings);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="sticky top-0 z-10 border-b border-border-subtle bg-bg-surface px-4 py-2">
        <div className="mx-auto flex w-full max-w-5xl items-center justify-end gap-2">
          <Button
            variant="secondary"
            onClick={handleReset}
            disabled={!hasChanges}
          >
            Reset
          </Button>
          <Button variant="primary" onClick={handleSave} disabled={!hasChanges}>
            Apply Settings
          </Button>
        </div>
      </div>

      <div className="mx-auto flex w-full max-w-5xl flex-1 overflow-hidden p-6 gap-8">
        {/* Sidebar Info */}
        <aside className="w-64 flex-shrink-0">
          <ServerSummary />
        </aside>

        {/* Settings Content */}
        <div className="flex-1 space-y-6 overflow-auto pr-2">
          {/* Policies */}
          {serverInfo && (
            <Card className="p-4">
              <h3 className="mb-4 text-sm font-semibold text-text-primary">
                ⚖️ Policies
              </h3>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1">
                  <label className="text-[10px] font-semibold text-text-muted uppercase tracking-tight">
                    Allocation Strategy
                  </label>
                  <div className="flex">
                    <span className="rounded bg-primary/10 px-2 py-0.5 text-xs font-bold text-primary uppercase">
                      {serverInfo.strategy || "none"}
                    </span>
                  </div>
                </div>
                <div className="space-y-1">
                  <label className="text-[10px] font-semibold text-text-muted uppercase tracking-tight">
                    Selection Policy
                  </label>
                  <div className="text-sm text-text-secondary italic">
                    {serverInfo.allocationPolicy || "fcfs"}
                  </div>
                </div>
              </div>
            </Card>
          )}

          {/* Screencast */}
          <Card className="p-4">
            <h3 className="mb-4 text-sm font-semibold text-text-primary">
              📺 Screencast
            </h3>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <label className="text-sm text-text-secondary">Frame Rate</label>
                <div className="flex items-center gap-2">
                  <input
                    type="range"
                    min={1}
                    max={15}
                    value={local.screencast.fps}
                    onChange={(e) =>
                      setLocal({
                        ...local,
                        screencast: { ...local.screencast, fps: +e.target.value },
                      })
                    }
                    className="w-32"
                  />
                  <span className="w-12 text-right text-sm text-text-muted">
                    {local.screencast.fps} fps
                  </span>
                </div>
              </div>
              <div className="flex items-center justify-between">
                <label className="text-sm text-text-secondary">Quality</label>
                <div className="flex items-center gap-2">
                  <input
                    type="range"
                    min={10}
                    max={80}
                    value={local.screencast.quality}
                    onChange={(e) =>
                      setLocal({
                        ...local,
                        screencast: {
                          ...local.screencast,
                          quality: +e.target.value,
                        },
                      })
                    }
                    className="w-32"
                  />
                  <span className="w-12 text-right text-sm text-text-muted">
                    {local.screencast.quality}%
                  </span>
                </div>
              </div>
              <div className="flex items-center justify-between">
                <label className="text-sm text-text-secondary">Max Width</label>
                <select
                  value={local.screencast.maxWidth}
                  onChange={(e) =>
                    setLocal({
                      ...local,
                      screencast: {
                        ...local.screencast,
                        maxWidth: +e.target.value,
                      },
                    })
                  }
                  className="rounded border border-border-default bg-bg-elevated px-2 py-1 text-sm text-text-primary"
                >
                  {[400, 600, 800, 1024, 1280].map((w) => (
                    <option key={w} value={w}>
                      {w}px
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </Card>

          {/* Stealth */}
          <Card className="p-4">
            <h3 className="mb-4 text-sm font-semibold text-text-primary">
              🛡️ Stealth
            </h3>
            <div className="flex items-center justify-between">
              <label className="text-sm text-text-secondary">Level</label>
              <select
                value={local.stealth}
                onChange={(e) =>
                  setLocal({
                    ...local,
                    stealth: e.target.value as "light" | "full",
                  })
                }
                className="rounded border border-border-default bg-bg-elevated px-2 py-1 text-sm text-text-primary"
              >
                <option value="light">Light (default)</option>
                <option value="full">Full (canvas noise, WebGL, fonts)</option>
              </select>
            </div>
          </Card>

          {/* Browser */}
          <Card className="p-4">
            <h3 className="mb-4 text-sm font-semibold text-text-primary">
              🌐 Browser
            </h3>
            <div className="space-y-3">
              {[
                { key: "blockImages", label: "Block Images" },
                { key: "blockMedia", label: "Block Media" },
                { key: "noAnimations", label: "No Animations" },
              ].map(({ key, label }) => (
                <label key={key} className="flex items-center justify-between">
                  <span className="text-sm text-text-secondary">{label}</span>
                  <input
                    type="checkbox"
                    checked={local.browser[key as keyof typeof local.browser]}
                    onChange={(e) =>
                      setLocal({
                        ...local,
                        browser: { ...local.browser, [key]: e.target.checked },
                      })
                    }
                    className="h-4 w-4"
                  />
                </label>
              ))}
            </div>
          </Card>

          {/* Monitoring */}
          <Card className="p-4">
            <h3 className="mb-4 text-sm font-semibold text-text-primary">
              📈 Monitoring
            </h3>
            <div className="space-y-3">
              <label className="flex items-center justify-between">
                <div>
                  <span className="text-sm text-text-secondary">
                    Tab Memory Metrics{" "}
                    <span className="rounded bg-yellow-500/20 px-1 py-0.5 text-xs text-yellow-500">
                      experimental
                    </span>
                  </span>
                  <p className="text-xs text-text-muted">
                    Track JS heap usage per tab via CDP (may cause instability)
                  </p>
                </div>
                <input
                  type="checkbox"
                  checked={local.monitoring?.memoryMetrics ?? false}
                  onChange={(e) =>
                    setLocal({
                      ...local,
                      monitoring: {
                        ...local.monitoring,
                        memoryMetrics: e.target.checked,
                      },
                    })
                  }
                  className="h-4 w-4"
                />
              </label>
              <div className="flex items-center justify-between">
                <div>
                  <span className="text-sm text-text-secondary">
                    Poll Interval
                  </span>
                  <p className="text-xs text-text-muted">
                    How often to fetch tab/memory data
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <input
                    type="range"
                    min={5}
                    max={120}
                    step={5}
                    value={local.monitoring?.pollInterval ?? 30}
                    onChange={(e) =>
                      setLocal({
                        ...local,
                        monitoring: {
                          ...local.monitoring,
                          pollInterval: +e.target.value,
                        },
                      })
                    }
                    className="w-24"
                  />
                  <span className="w-12 text-right text-sm text-text-muted">
                    {local.monitoring?.pollInterval ?? 30}s
                  </span>
                </div>
              </div>
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
