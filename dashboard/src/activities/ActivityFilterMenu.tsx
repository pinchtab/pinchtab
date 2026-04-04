import { useState, type ChangeEvent } from "react";
import { Button, Input } from "../components/atoms";
import type { Profile, Instance, InstanceTab } from "../types";
import type { ActivityFilters } from "./types";
import { actionOptions } from "./helpers";

interface Props {
  filters: ActivityFilters;
  profileOptions: Profile[];
  instanceOptions: Instance[];
  tabOptions: InstanceTab[];
  loading: boolean;
  showAgentFilter?: boolean;
  onClear: () => void;
  onRefresh: () => void;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
  onProfileChange: (value: string) => void;
  onInstanceChange: (value: string) => void;
}

function FilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: Array<{ value: string; label: string }>;
  onChange: (event: ChangeEvent<HTMLSelectElement>) => void;
}) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="dashboard-section-title text-[0.68rem]">{label}</span>
      <select
        aria-label={label}
        value={value}
        onChange={onChange}
        className="rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-primary transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
      >
        {options.map((option) => (
          <option key={option.value || "all"} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

export default function ActivityFilterMenu({
  filters,
  profileOptions,
  instanceOptions,
  tabOptions,
  loading,
  showAgentFilter = true,
  onClear,
  onRefresh,
  onFilterChange,
  onProfileChange,
  onInstanceChange,
}: Props) {
  const [showAdvanced, setShowAdvanced] = useState(false);

  return (
    <>
      <div className="flex-1 space-y-4 overflow-auto p-4">
        <div className="space-y-3">
          <FilterSelect
            label="Profile"
            value={filters.profileName}
            options={[
              { value: "", label: "Any profile" },
              ...profileOptions.map((profile) => ({
                value: profile.name,
                label: profile.name,
              })),
            ]}
            onChange={(event) => onProfileChange(event.target.value)}
          />
          <FilterSelect
            label="Tab"
            value={filters.tabId}
            options={[
              { value: "", label: "Any tab" },
              ...tabOptions.map((tab) => ({
                value: tab.id,
                label: `${tab.title || tab.url || tab.id} · ${tab.id}`,
              })),
            ]}
            onChange={(event) => onFilterChange("tabId", event.target.value)}
          />
        </div>

        <div className="space-y-3 border-t border-border-subtle pt-4">
          {showAgentFilter && (
            <Input
              label="Agent"
              placeholder="cli, mcp, custom"
              value={filters.agentId}
              onChange={(event) =>
                onFilterChange("agentId", event.target.value)
              }
            />
          )}
          <FilterSelect
            label="Action"
            value={filters.action}
            options={[
              { value: "", label: "Any action" },
              ...actionOptions
                .filter(Boolean)
                .map((option) => ({ value: option, label: option })),
            ]}
            onChange={(event) => onFilterChange("action", event.target.value)}
          />
        </div>

        <div className="border-t border-border-subtle pt-4">
          <button
            type="button"
            className="flex w-full items-center justify-between text-left"
            onClick={() => setShowAdvanced((current) => !current)}
            aria-expanded={showAdvanced}
            aria-controls="activity-advanced-filters"
          >
            <span className="dashboard-section-title text-[0.68rem]">
              Advanced filters
            </span>
            <span className="text-[0.68rem] uppercase tracking-[0.16em] text-text-muted">
              {showAdvanced ? "Hide" : "Show"}
            </span>
          </button>

          {showAdvanced && (
            <div id="activity-advanced-filters" className="mt-3 space-y-3">
              <FilterSelect
                label="Instance"
                value={filters.instanceId}
                options={[
                  { value: "", label: "Any instance" },
                  ...instanceOptions.map((instance) => ({
                    value: instance.id,
                    label: `${instance.profileName} · ${instance.id}`,
                  })),
                ]}
                onChange={(event) => onInstanceChange(event.target.value)}
              />
              <Input
                label="Session"
                placeholder="session_xxx"
                value={filters.sessionId}
                onChange={(event) =>
                  onFilterChange("sessionId", event.target.value)
                }
              />
              <Input
                label="Path prefix"
                placeholder="/tabs/ or /instances/"
                value={filters.pathPrefix}
                onChange={(event) =>
                  onFilterChange("pathPrefix", event.target.value)
                }
              />
              <Input
                label="Age (seconds)"
                placeholder="3600"
                value={filters.ageSec}
                onChange={(event) =>
                  onFilterChange("ageSec", event.target.value)
                }
              />
              <Input
                label="Limit"
                placeholder="200"
                value={filters.limit}
                onChange={(event) =>
                  onFilterChange("limit", event.target.value)
                }
              />
            </div>
          )}
        </div>
      </div>

      <div className="flex gap-2 border-t border-border-subtle p-4">
        <Button
          variant="secondary"
          size="sm"
          onClick={onClear}
          disabled={loading}
          className="flex-1"
        >
          Clear
        </Button>
        <Button
          variant="primary"
          size="sm"
          onClick={onRefresh}
          loading={loading}
          className="flex-1"
        >
          Refresh
        </Button>
      </div>
    </>
  );
}
