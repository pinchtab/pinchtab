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
      "wss://pinchtab.com/instances/inst_123/proxy/screencast?tabId=tab_456&quality=40&maxWidth=800&fps=1",
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

  it("renders streamed frames with createImageBitmap when available", async () => {
    const drawImage = vi.fn();
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage,
    } as unknown as CanvasRenderingContext2D);

    const bitmap = {
      width: 640,
      height: 360,
      close: vi.fn(),
    } as unknown as ImageBitmap;
    const createImageBitmapMock = vi.fn().mockResolvedValue(bitmap);
    vi.stubGlobal("createImageBitmap", createImageBitmapMock);

    render(
      <ScreencastTile
        instanceId="inst_123"
        tabId="tab_456"
        label="Example"
        url="https://pinchtab.com"
      />,
    );

    await waitFor(() => expect(webSocketMock).toHaveBeenCalledTimes(1));

    const socket = webSocketInstances[0] as {
      onmessage?: (event: MessageEvent<ArrayBuffer>) => void;
    };
    await act(async () => {
      socket.onmessage?.({
        data: new Uint8Array([1, 2, 3]).buffer,
      } as MessageEvent<ArrayBuffer>);
    });

    await waitFor(() => expect(drawImage).toHaveBeenCalledWith(bitmap, 0, 0));
    expect(createImageBitmapMock).toHaveBeenCalledTimes(1);
    expect(bitmap.close).toHaveBeenCalledTimes(1);
  });

  it("keeps rendering after a frame decode failure", async () => {
    const drawImage = vi.fn();
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage,
    } as unknown as CanvasRenderingContext2D);

    const bitmap = {
      width: 800,
      height: 600,
      close: vi.fn(),
    } as unknown as ImageBitmap;
    const createImageBitmapMock = vi
      .fn()
      .mockRejectedValueOnce(new Error("decode failed"))
      .mockResolvedValueOnce(bitmap);
    vi.stubGlobal("createImageBitmap", createImageBitmapMock);
    vi.spyOn(console, "error").mockImplementation(() => {});

    render(
      <ScreencastTile
        instanceId="inst_123"
        tabId="tab_456"
        label="Example"
        url="https://pinchtab.com"
      />,
    );

    await waitFor(() => expect(webSocketMock).toHaveBeenCalledTimes(1));

    const socket = webSocketInstances[0] as {
      onmessage?: (event: MessageEvent<ArrayBuffer>) => void;
    };
    await act(async () => {
      socket.onmessage?.({
        data: new Uint8Array([1, 2, 3]).buffer,
      } as MessageEvent<ArrayBuffer>);
      socket.onmessage?.({
        data: new Uint8Array([4, 5, 6]).buffer,
      } as MessageEvent<ArrayBuffer>);
    });

    await waitFor(() => expect(drawImage).toHaveBeenCalledWith(bitmap, 0, 0));
    expect(createImageBitmapMock).toHaveBeenCalledTimes(2);
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
