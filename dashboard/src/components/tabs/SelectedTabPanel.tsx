import type { InstanceTab } from "../../generated/types";

interface Props {
  selectedTab: InstanceTab | null;
}

export default function SelectedTabPanel({ selectedTab }: Props) {
  return (
    <div className="flex min-h-[12rem] flex-1 flex-col rounded-xl border border-border-subtle bg-white/[0.02] p-4">
      {selectedTab ? (
        <>
          <div className="dashboard-section-label mb-1">Selected Tab</div>
          <h5 className="truncate text-base font-semibold text-text-primary">
            {selectedTab.title || "Untitled"}
          </h5>
          <div className="mt-4 space-y-4">
            <div>
              <div className="dashboard-section-title mb-1 text-[0.68rem]">
                URL
              </div>
              <div className="break-all text-sm text-text-secondary">
                {selectedTab.url}
              </div>
            </div>
          </div>
        </>
      ) : (
        <div className="flex flex-1 items-center justify-center text-sm text-text-muted">
          Select a tab to view details
        </div>
      )}
    </div>
  );
}
