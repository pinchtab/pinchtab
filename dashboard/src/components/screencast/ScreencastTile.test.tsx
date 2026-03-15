import { render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ScreencastTile from "../screencast/ScreencastTile";

const webSocketMock = vi.fn(function MockWebSocket(
  this: Record<string, unknown>,
) {
  this.close = vi.fn();
});

describe("ScreencastTile", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal(
      "location",
      new URL("https://pinchtab.com/dashboard/profiles"),
    );
    window.localStorage.setItem("pinchtab.auth.token", "secret-token");
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage: vi.fn(),
    } as unknown as CanvasRenderingContext2D);
    vi.stubGlobal("WebSocket", webSocketMock);
  });

  afterEach(() => {
    window.localStorage.clear();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("connects through the same-origin screencast proxy on secure deployments", async () => {
    render(
      <ScreencastTile
        instanceId="inst_123"
        tabId="tab_456"
        label="Example"
        url="https://pinchtab.com"
      />,
    );

    await waitFor(() => expect(webSocketMock).toHaveBeenCalledTimes(1));

    expect(webSocketMock).toHaveBeenCalledWith(
      "wss://pinchtab.com/instances/inst_123/proxy/screencast?tabId=tab_456&quality=30&maxWidth=800&fps=1&token=secret-token",
    );
  });
});
