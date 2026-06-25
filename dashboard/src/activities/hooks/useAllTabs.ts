import { useEffect, useState } from "react";
import * as api from "../../services/api";
import type { InstanceTab } from "../../types";

// Shared tabs loader for the activity surfaces: fetches all tabs and re-fetches
// when refreshKey changes. Pass a refresh nonce to keep tab filters fresh after a
// refresh / instance churn; omit it for mount-only loading.
export function useAllTabs(refreshKey?: number): InstanceTab[] {
  const [tabs, setTabs] = useState<InstanceTab[]>([]);

  useEffect(() => {
    let cancelled = false;
    void api
      .fetchAllTabs()
      .then((response) => {
        if (!cancelled) {
          setTabs(response);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setTabs([]);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [refreshKey]);

  return tabs;
}
