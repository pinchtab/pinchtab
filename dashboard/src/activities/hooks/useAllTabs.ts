import { useAsyncResource } from "../../hooks/useAsyncResource";
import * as api from "../../services/api";
import type { InstanceTab } from "../../types";

export function useAllTabs(refreshKey?: number): InstanceTab[] {
  const { data, error } = useAsyncResource(api.fetchAllTabs, [refreshKey], {
    initial: [] as InstanceTab[],
  });
  return error ? [] : data;
}
