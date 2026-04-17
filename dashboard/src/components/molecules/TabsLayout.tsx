import React from "react";
import SidebarPanel from "./SidebarPanel";

export interface TabItem<T extends string> {
  id: T;
  label: string;
  badge?: string | number;
}

interface Props<T extends string> {
  tabs: TabItem<T>[];
  activeTab: T;
  onChange: (id: T) => void;
  children: React.ReactNode;
  className?: string;
  rightSlot?: React.ReactNode;
  footer?: React.ReactNode;
}

export default function TabsLayout<T extends string>({
  tabs,
  activeTab,
  onChange,
  children,
  className = "",
  rightSlot,
  footer,
}: Props<T>) {
  return (
    <SidebarPanel
      className={`h-full ${className}`}
      footer={footer}
      headerPadding="md"
      header={
        <div className="flex items-center gap-1">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={() => onChange(tab.id)}
              className={`rounded px-3 py-1.5 text-xs font-medium transition-colors ${
                activeTab === tab.id
                  ? "bg-bg-hover text-text-primary"
                  : "text-text-muted hover:bg-bg-hover hover:text-text-secondary"
              }`}
            >
              {tab.label}
              {tab.badge !== undefined && (
                <span className="ml-1.5 rounded-full bg-bg-elevated px-1.5 py-0.5 text-[10px] opacity-70">
                  {tab.badge}
                </span>
              )}
            </button>
          ))}
          {rightSlot && (
            <div className="ml-auto min-w-0 shrink">{rightSlot}</div>
          )}
        </div>
      }
    >
      {children}
    </SidebarPanel>
  );
}
