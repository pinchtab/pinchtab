import type { Instance, InstanceTab } from "../generated/types";
import ScreencastTile from "../components/screencast/ScreencastTile";
import { EmptyView } from "../components/molecules";

interface Props {
  instance?: Instance;
  tabs: InstanceTab[];
  isRunning: boolean;
}

export default function ProfileLiveViewPanel({
  instance,
  tabs,
  isRunning,
}: Props) {
  const sortedTabs = [...tabs].sort((a, b) => a.id.localeCompare(b.id));

  return (
    <div className="h-full overflow-y-auto">
      {isRunning && instance ? (
        tabs.length === 0 ? (
          <EmptyView message="No tabs open" />
        ) : (
          <div className="p-4 grid grid-cols-1 gap-4 lg:grid-cols-2">
            {sortedTabs.map((tab) => (
              <div key={tab.id} className="aspect-video">
                <ScreencastTile
                  instanceId={instance.id}
                  tabId={tab.id}
                  label={tab.title?.slice(0, 20) || tab.id.slice(0, 8)}
                  url={tab.url}
                />
              </div>
            ))}
          </div>
        )
      ) : (
        <EmptyView message="Instance not running. Start the profile to see live view." />
      )}
    </div>
  );
}
