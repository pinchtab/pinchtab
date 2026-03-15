import type { Instance, InstanceTab } from "../../generated/types";
import ScreencastTile from "../screencast/ScreencastTile";
import { EmptyView } from "../molecules";

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
  return (
    <div className="h-full overflow-y-auto">
      {isRunning && instance ? (
        tabs.length === 0 ? (
          <EmptyView message="No tabs open" />
        ) : (
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {tabs.map((tab) => (
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
