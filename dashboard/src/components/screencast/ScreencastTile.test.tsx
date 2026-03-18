import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "../../services/api";
import ScreencastTile from "../screencast/ScreencastTile";

const webSocketInstances: Record<string, unknown>[] = [];
const webSocketMock = vi.fn(function MockWebSocket(
  this: Record<string, unknown>,
) {
  this.close = vi.fn();
  webSocketInstances.push(this);
});

describe("ScreencastTile", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    webSocketInstances.length = 0;
    vi.stubGlobal(
      "location",
      new URL("https://pinchtab.com/dashboard/profiles"),
    );
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage: vi.fn(),
    } as unknown as CanvasRenderingContext2D);
    vi.stubGlobal("WebSocket", webSocketMock);
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      writable: true,
      value: vi.fn(() => "blob:preview"),
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      writable: true,
      value: vi.fn(),
    });
  });

  afterEach(() => {
    vi.useRealTimers();
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
      "wss://pinchtab.com/instances/inst_123/proxy/screencast?tabId=tab_456&quality=30&maxWidth=800&fps=1",
    );
  });

  it("shows a static preview when no screencast frame arrives", async () => {
    vi.useFakeTimers();
    vi.spyOn(api, "fetchTabScreenshot").mockResolvedValue(new Blob(["frame"]));

    render(
      <ScreencastTile
        instanceId="inst_123"
        tabId="tab_456"
        label="Example"
        url="https://pinchtab.com"
      />,
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });

    expect(api.fetchTabScreenshot).toHaveBeenCalledWith("tab_456", "png");
    expect(screen.getByAltText("Tab preview")).toHaveAttribute(
      "src",
      "blob:preview",
    );
  });

  it("reconnects when retry is clicked after the socket closes", async () => {
    render(
      <ScreencastTile
        instanceId="inst_123"
        tabId="tab_456"
        label="Example"
        url="https://pinchtab.com"
      />,
    );

    await waitFor(() => expect(webSocketMock).toHaveBeenCalledTimes(1));

    const firstSocket = webSocketInstances[0] as {
      onclose?: (event: CloseEvent) => void;
    };
    await act(async () => {
      firstSocket.onclose?.({} as CloseEvent);
    });

    fireEvent.click(screen.getByRole("button", { name: "Retry connection" }));

    await waitFor(() => expect(webSocketMock).toHaveBeenCalledTimes(2));
  });
});
