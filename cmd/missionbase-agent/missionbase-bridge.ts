import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import { basename } from "node:path";

type RemoteMessage = { id: string | number; body: string };
type PollResponse = { messages?: RemoteMessage[] };
type TranscriptEntry = {
  external_id: string;
  parent_external_id: string | null;
  role: "user" | "assistant" | "tool" | "bash" | "summary";
  body: string;
  metadata: Record<string, unknown>;
  occurred_at: string;
};
type SessionEntry = {
  type: string;
  id: string;
  parentId: string | null;
  timestamp: string;
  message?: Record<string, any>;
  summary?: string;
  customType?: string;
  data?: Record<string, unknown>;
};
type BridgeState = {
  externalId: string;
  registered: boolean;
  stopped: boolean;
  transcriptFingerprint?: string;
  syncPromise?: Promise<void>;
  timer?: ReturnType<typeof setTimeout>;
  controller: AbortController;
};

const STATUS_KEY = "missionbase-bridge";
const REMOTE_MESSAGE_MARKER = "missionbase-remote-message";
const POLL_INTERVAL_MS = 5000;
const MAX_ENTRY_CHARACTERS = 200_000;
const MAX_TOOL_ARGUMENT_CHARACTERS = 4_000;

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

  const syncTranscript = async (ctx: ExtensionContext, current: BridgeState) => {
    if (current.syncPromise) return current.syncPromise;

    current.syncPromise = (async () => {
      const entries = transcriptEntries(ctx);
      const fingerprint = JSON.stringify(entries);
      if (fingerprint === current.transcriptFingerprint) return;

      await request(
        `${sessionPath(current.externalId)}/transcript`,
        { method: "PUT", body: JSON.stringify({ entries }) },
        current.controller.signal,
      );
      current.transcriptFingerprint = fingerprint;
    })();

    try {
      await current.syncPromise;
    } finally {
      current.syncPromise = undefined;
    }
  };

  const deliverRemoteMessage = async (ctx: ExtensionContext, current: BridgeState, message: RemoteMessage) => {
    const deliveryState = remoteMessageDeliveryState(ctx, message.id);
    if (deliveryState !== "delivered" && !ctx.isIdle()) return;

    if (deliveryState === "absent") {
      ctx.sessionManager.appendCustomEntry(REMOTE_MESSAGE_MARKER, { messageId: String(message.id) });
    }
    if (deliveryState !== "delivered") {
      pi.sendUserMessage(message.body);
      if (remoteMessageDeliveryState(ctx, message.id) !== "delivered") {
        throw new Error("Remote message was not persisted by Pi");
      }
    }

    await request(
      `${sessionPath(current.externalId)}/messages/${encodeURIComponent(String(message.id))}/delivery`,
      { method: "POST" },
      current.controller.signal,
    );
  };

  const poll = async (ctx: ExtensionContext, current: BridgeState) => {
    if (!current.registered) await register(ctx, current);
    await syncTranscript(ctx, current);
    const response = await request(
      sessionPath(current.externalId),
      {
        method: "PATCH",
        body: JSON.stringify({ status: "active", model: currentModel(ctx) }),
      },
      current.controller.signal,
    );
    const payload = (await response.json()) as PollResponse;
    const message = payload.messages?.find(
      (candidate) => typeof candidate.body === "string" && candidate.body.trim() !== "",
    );
    if (!current.stopped && message) await deliverRemoteMessage(ctx, current, message);
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
    if (!current || current.stopped || !current.registered) return;

    try {
      await syncTranscript(ctx, current);
    } catch (error) {
      if (!current.stopped && !current.controller.signal.aborted) {
        ctx.ui.setStatus(STATUS_KEY, "Missionbase: transcript pending");
      }
    }
  });

  pi.on("session_shutdown", async (_event, ctx) => {
    const current = state;
    state = undefined;
    if (!current) return;

    current.stopped = true;
    if (current.timer) clearTimeout(current.timer);
    try {
      if (current.registered) await syncTranscript(ctx, current);
    } catch (error) {
      // Best effort: the next resume will synchronize the full active branch.
    }
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

function transcriptEntries(ctx: ExtensionContext): TranscriptEntry[] {
  return (ctx.sessionManager.getBranch() as SessionEntry[])
    .map(sanitizeEntry)
    .filter((entry): entry is TranscriptEntry => entry !== undefined);
}

function sanitizeEntry(entry: SessionEntry): TranscriptEntry | undefined {
  if (entry.type === "compaction" || entry.type === "branch_summary") {
    return transcriptEntry(entry, "summary", entry.summary ?? "", {});
  }
  if (entry.type !== "message" || !entry.message) return;

  const message = entry.message;
  switch (message.role) {
    case "user":
      return transcriptEntry(entry, "user", visibleText(message.content), {});
    case "assistant": {
      const content = Array.isArray(message.content) ? message.content : [];
      const tools = content
        .filter((block: any) => block?.type === "toolCall")
        .slice(0, 20)
        .map((block: any) => ({
          id: String(block.id ?? ""),
          name: String(block.name ?? "tool"),
          arguments: limitedJson(block.arguments),
        }));
      return transcriptEntry(entry, "assistant", visibleText(content), tools.length > 0 ? { tools } : {});
    }
    case "toolResult":
      return transcriptEntry(entry, "tool", visibleText(message.content), {
        tool_call_id: String(message.toolCallId ?? ""),
        tool_name: String(message.toolName ?? "Tool result"),
        is_error: Boolean(message.isError),
      });
    case "bashExecution":
      return transcriptEntry(entry, "bash", String(message.output ?? ""), {
        command: String(message.command ?? "Shell command").slice(0, MAX_TOOL_ARGUMENT_CHARACTERS),
        exit_code: message.exitCode ?? null,
        cancelled: Boolean(message.cancelled),
        truncated: Boolean(message.truncated),
      });
    case "branchSummary":
    case "compactionSummary":
      return transcriptEntry(entry, "summary", String(message.summary ?? ""), {});
    default:
      return;
  }
}

function transcriptEntry(
  entry: SessionEntry,
  role: TranscriptEntry["role"],
  body: string,
  metadata: Record<string, unknown>,
): TranscriptEntry | undefined {
  const limitedBody = body.trim().slice(0, MAX_ENTRY_CHARACTERS);
  if (!limitedBody && Object.keys(metadata).length === 0) return;
  return {
    external_id: entry.id,
    parent_external_id: entry.parentId,
    role,
    body: limitedBody,
    metadata,
    occurred_at: entry.timestamp,
  };
}

function visibleText(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .flatMap((block: any) => {
      if (block?.type === "text") return [String(block.text ?? "")];
      if (block?.type === "image") return [`[Image: ${String(block.mimeType ?? "attachment")}]`];
      return [];
    })
    .join("\n")
    .trim();
}

function limitedJson(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}).slice(0, MAX_TOOL_ARGUMENT_CHARACTERS);
  } catch (error) {
    return "[unserializable arguments]";
  }
}

function remoteMessageDeliveryState(ctx: ExtensionContext, messageId: string | number): "absent" | "marked" | "delivered" {
  const entries = ctx.sessionManager.getEntries() as SessionEntry[];
  const markerIndex = entries.findIndex(
    (entry) => entry.type === "custom" && entry.customType === REMOTE_MESSAGE_MARKER && entry.data?.messageId === String(messageId),
  );
  if (markerIndex < 0) return "absent";
  return entries.slice(markerIndex + 1).some((entry) => entry.type === "message" && entry.message?.role === "user")
    ? "delivered"
    : "marked";
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
