import { describe, it } from "node:test";
import assert from "node:assert";
import type { AnyAgentTool, OpenClawPluginApi, OpenClawPluginToolContext, PluginLogger } from "openclaw/plugin-sdk/plugin-entry";
import type { PluginRuntime } from "openclaw/plugin-sdk/plugin-runtime";
import pluginEntry from "./index.ts";

type RegisteredToolOptions = Parameters<OpenClawPluginApi["registerTool"]>[1];
type TestPluginApiInput = Partial<OpenClawPluginApi>;

function createTestPluginApi(api: TestPluginApiInput): OpenClawPluginApi {
  const logger: PluginLogger = {
    info() {},
    warn() {},
    error() {},
    debug() {},
  };

  return {
    id: "pinchtab",
    name: "PinchTab",
    source: "test",
    registrationMode: "full",
    config: {},
    runtime: {} as PluginRuntime,
    logger,
    registerTool() {},
    registerHook() {},
    registerHttpRoute() {},
    registerChannel() {},
    registerGatewayMethod() {},
    registerCli() {},
    registerReload() {},
    registerNodeHostCommand() {},
    registerSecurityAuditCollector() {},
    registerService() {},
    registerGatewayDiscoveryService() {},
    registerCliBackend() {},
    registerTextTransforms() {},
    registerConfigMigration() {},
    registerMigrationProvider() {},
    registerAutoEnableProbe() {},
    registerProvider() {},
    registerSpeechProvider() {},
    registerRealtimeTranscriptionProvider() {},
    registerRealtimeVoiceProvider() {},
    registerMediaUnderstandingProvider() {},
    registerImageGenerationProvider() {},
    registerMusicGenerationProvider() {},
    registerVideoGenerationProvider() {},
    registerWebFetchProvider() {},
    registerWebSearchProvider() {},
    registerInteractiveHandler() {},
    onConversationBindingResolved() {},
    registerCommand() {},
    registerContextEngine() {},
    registerCompactionProvider() {},
    registerAgentHarness() {},
    registerCodexAppServerExtensionFactory() {},
    registerAgentToolResultMiddleware() {},
    registerDetachedTaskRuntime() {},
    registerSessionExtension() {},
    enqueueNextTurnInjection: async (injection) => ({
      enqueued: false,
      id: "",
      sessionKey: injection.sessionKey,
    }),
    registerTrustedToolPolicy() {},
    registerToolMetadata() {},
    registerControlUiDescriptor() {},
    registerRuntimeLifecycle() {},
    registerAgentEventSubscription() {},
    setRunContext: () => false,
    getRunContext: () => undefined,
    clearRunContext() {},
    registerSessionSchedulerJob: () => undefined,
    registerMemoryCapability() {},
    registerMemoryPromptSection() {},
    registerMemoryPromptSupplement() {},
    registerMemoryCorpusSupplement() {},
    registerMemoryFlushPlan() {},
    registerMemoryRuntime() {},
    registerMemoryEmbeddingProvider() {},
    resolvePath(input) {
      return input;
    },
    on() {},
    ...api,
  };
}

const testToolContext: OpenClawPluginToolContext = {
  agentId: "main",
  sessionId: "session-1",
  sessionKey: "chat:main",
};

describe("OpenClaw plugin API contract", () => {
  it("registers tools through the official OpenClaw plugin API", () => {
    const registered: Array<{ tool: AnyAgentTool; opts?: RegisteredToolOptions }> = [];
    const api = createTestPluginApi({
      id: "pinchtab",
      name: "Pinchtab",
      pluginConfig: { registerBrowserTool: true },
      config: {
        plugins: {
          entries: {
            pinchtab: { config: { registerBrowserTool: true } },
          },
        },
      },
      registerTool(tool, opts) {
        const resolved = typeof tool === "function" ? tool(testToolContext) : tool;
        assert.ok(resolved);
        registered.push({ tool: resolved as AnyAgentTool, opts });
      },
    });

    pluginEntry.register(api);

    assert.deepStrictEqual(
      registered.map(({ tool }) => tool.name).sort(),
      ["browser", "pinchtab"],
    );
    for (const { tool, opts } of registered) {
      assert.strictEqual(typeof tool.label, "string");
      assert.strictEqual(typeof tool.description, "string");
      assert.strictEqual(typeof tool.execute, "function");
      assert.strictEqual((tool.parameters as { type?: string }).type, "object");
      assert.deepStrictEqual(opts, { optional: true });
    }
  });

  it("honors pluginConfig when suppressing the compatibility browser tool", () => {
    const names: string[] = [];
    const api = createTestPluginApi({
      id: "pinchtab",
      name: "Pinchtab",
      pluginConfig: { registerBrowserTool: false },
      registerTool(tool) {
        const resolved = typeof tool === "function" ? tool(testToolContext) : tool;
        assert.ok(resolved);
        names.push((resolved as AnyAgentTool).name);
      },
    });

    pluginEntry.register(api);

    assert.deepStrictEqual(names, ["pinchtab"]);
  });
});
