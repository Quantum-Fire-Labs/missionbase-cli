import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import { basename } from "node:path";

type RemoteMessage = { id: string | number; body: string };
type PollResponse = { messages?: RemoteMessage[] };

type BridgeState = {
  externalId: string;
  registered: boolean;
  stopped: boolean;
  timer?: ReturnType<typeof setTimeout>;
  controller: AbortController;
  pendingMessage?: RemoteMessage;
  pendingResponse?: string;
};

const STATUS_KEY = "missionbase-bridge";
const POLL_INTERVAL_MS = 5000;
const MAX_RESPONSE_CHARACTERS = 100_000;

export default function missionbaseBridge(pi: ExtensionAPI) {
  let state: BridgeState | undefined;

  const token = requiredEnvironment("MISSIONBASE_COMPUTER_TOKEN");
  delete process.env.MISSIONBASE_COMPUTER_TOKEN;
  requiredEnvironment("MISSIONBASE_COMPUTER_ID");
  const baseUrl = requiredEnvironment("MISSIONBASE_BASE_URL").replace(/\/+$/, "");
  const agentId = requiredIntegerEnvironment("MISSIONBASE_AGENT_ID");
  const cliVersion = requiredEnvironment("MISSIONBASE_CLI_VERSION");

  const request = async (path: string, init: RequestInit, signal?: AbortSignal) => {
    const response = await fetch(`${baseUrl}${path}`, {
      ...init,
      headers: {
        Accept: "application/json",
        Authorization: `Bearer ${token}`,
        ...(init.body === undefined ? {} : { "Content-Type": "application/json" }),
        ...init.headers,
      },
      signal,
    });
    if (!response.ok) throw new Error(`Missionbase request failed (HTTP ${response.status})`);
    return response;
  };

  const sessionPath = (externalId: string) =>
    `/api/v1/computer/sessions/${encodeURIComponent(externalId)}`;

  const currentModel = (ctx: ExtensionContext) => ctx.model?.id ?? "";

  const register = async (ctx: ExtensionContext, current: BridgeState) => {
    const [repositoryResult, branchResult] = await Promise.all([
      pi.exec("git", ["rev-parse", "--show-toplevel"], { timeout: 3000 }),
      pi.exec("git", ["branch", "--show-current"], { timeout: 3000 }),
    ]);
    const repositoryRoot = repositoryResult.code === 0 ? repositoryResult.stdout.trim() : "";
    await request(
      "/api/v1/computer/sessions",
      {
        method: "POST",
        body: JSON.stringify({
          external_id: current.externalId,
          agent_id: agentId,
          cwd: ctx.cwd,
          session_file: ctx.sessionManager.getSessionFile() ?? null,
          repository: basename(repositoryRoot || ctx.cwd),
          branch: branchResult.code === 0 ? branchResult.stdout.trim() : "",
          model: currentModel(ctx),
          cli_version: cliVersion,
        }),
      },
      current.controller.signal,
    );
    current.registered = true;
    ctx.ui.setStatus(STATUS_KEY, "Missionbase: connected");
  };

  const flushResponse = async (ctx: ExtensionContext, current: BridgeState) => {
    const message = current.pendingMessage;
    const body = current.pendingResponse;
    if (!message || !body) return;
    await request(
      `${sessionPath(current.externalId)}/messages/${encodeURIComponent(String(message.id))}/response`,
      { method: "POST", body: JSON.stringify({ body }) },
      current.controller.signal,
    );
    current.pendingMessage = undefined;
    current.pendingResponse = undefined;
    ctx.ui.setStatus(STATUS_KEY, "Missionbase: connected");
  };

  const poll = async (ctx: ExtensionContext, current: BridgeState) => {
    if (!current.registered) await register(ctx, current);
    await flushResponse(ctx, current);
    const response = await request(
      sessionPath(current.externalId),
      {
        method: "PATCH",
        body: JSON.stringify({ status: "active", model: currentModel(ctx) }),
      },
      current.controller.signal,
    );
    const payload = (await response.json()) as PollResponse;
    if (!current.stopped && !current.pendingMessage && ctx.isIdle()) {
      const message = payload.messages?.find(
        (candidate) => typeof candidate.body === "string" && candidate.body.trim() !== "",
      );
      if (message) {
        current.pendingMessage = message;
        pi.sendUserMessage(message.body);
        await request(
          `${sessionPath(current.externalId)}/messages/${encodeURIComponent(String(message.id))}/delivery`,
          { method: "POST" },
          current.controller.signal,
        );
      }
    }
  };

  const schedulePoll = (ctx: ExtensionContext, current: BridgeState) => {
    if (current.stopped) return;
    current.timer = setTimeout(async () => {
      try {
        await poll(ctx, current);
      } catch (error) {
        if (!current.stopped && !current.controller.signal.aborted) {
          ctx.ui.setStatus(STATUS_KEY, "Missionbase: reconnecting");
        }
      } finally {
        schedulePoll(ctx, current);
      }
    }, POLL_INTERVAL_MS);
  };

  pi.on("session_start", async (_event, ctx) => {
    const current: BridgeState = {
      externalId: ctx.sessionManager.getSessionId(),
      registered: false,
      stopped: false,
      controller: new AbortController(),
    };
    state = current;
    ctx.ui.setStatus(STATUS_KEY, "Missionbase: connecting");
    try {
      await poll(ctx, current);
    } catch (error) {
      if (!current.controller.signal.aborted) {
        ctx.ui.setStatus(STATUS_KEY, "Missionbase: reconnecting");
      }
    }
    schedulePoll(ctx, current);
  });

  pi.on("agent_settled", async (_event, ctx) => {
    const current = state;
    const message = current?.pendingMessage;
    if (!current || !message || current.stopped) return;

    const body = latestAssistantText(ctx);
    if (!body) return;
    current.pendingResponse = body.slice(0, MAX_RESPONSE_CHARACTERS);
    try {
      await flushResponse(ctx, current);
    } catch (error) {
      if (!current.stopped && !current.controller.signal.aborted) {
        ctx.ui.setStatus(STATUS_KEY, "Missionbase: response pending");
      }
    }
  });

  pi.on("session_shutdown", async (_event, ctx) => {
    const current = state;
    state = undefined;
    if (!current) return;

    current.stopped = true;
    if (current.timer) clearTimeout(current.timer);
    current.controller.abort();
    ctx.ui.setStatus(STATUS_KEY, undefined);
    if (!current.registered) return;

    const shutdownController = new AbortController();
    const timeout = setTimeout(() => shutdownController.abort(), 2000);
    try {
      await request(sessionPath(current.externalId), { method: "DELETE" }, shutdownController.signal);
    } catch (error) {
      // Best effort: shutdown must not be blocked by a bridge/network failure.
    } finally {
      clearTimeout(timeout);
    }
  });
}

function requiredEnvironment(name: string): string {
  const value = process.env[name]?.trim();
  if (!value) throw new Error(`${name} is required`);
  return value;
}

function requiredIntegerEnvironment(name: string): number {
  const value = Number(requiredEnvironment(name));
  if (!Number.isInteger(value) || value <= 0) throw new Error(`${name} must be a positive integer`);
  return value;
}

function latestAssistantText(ctx: ExtensionContext): string {
  const entries = ctx.sessionManager.getBranch();
  for (let index = entries.length - 1; index >= 0; index--) {
    const entry = entries[index];
    if (entry.type !== "message" || entry.message.role !== "assistant") continue;
    return entry.message.content
      .filter((block) => block.type === "text")
      .map((block) => block.text)
      .join("\n")
      .trim();
  }
  return "";
}
