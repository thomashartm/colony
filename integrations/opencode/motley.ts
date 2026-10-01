// schema = 1
import { spawn } from "node:child_process";

// Reporter failures are contained here. No shell, agent output or model calls.
function report(event) {
  return new Promise((resolve) => {
    let child;
    try {
      child = spawn("motley", ["report", "--agent", "opencode"], {
        stdio: ["pipe", "ignore", "ignore"],
      });
      const timer = setTimeout(() => { child.kill("SIGKILL"); resolve(); }, 500);
      const done = () => { clearTimeout(timer); resolve(); };
      child.on("error", done);
      child.on("close", done);
      child.stdin.on("error", () => {});
      child.stdin.end(JSON.stringify(event));
    } catch { if (child) child.kill(); resolve(); }
  });
}

export const Motley = async ({ client }) => {
  if (!process.env.MOTLEY_MEMBER) return {};
  let rootID = "";
  let assistantID = "";
  let lastAssistantMessage = "";
  const children = new Set();
  const waiting = new Set();
  const queue = [];
  let running = false;

  async function processEvent(event) {
    const p = event.properties || {};
    const id = p.sessionID || p.info?.sessionID || p.info?.id || p.part?.sessionID;
    if (!id || children.has(id)) return;
    if (event.type === "session.created" && p.info?.parentID) { children.add(id); return; }
    // Resolve resumed sessions once, keeping child sessions out of the member's status.
    if (!rootID || (event.type === "chat.message" && id !== rootID)) {
      let info = event.type === "session.created" ? p.info : null;
      if (!info) {
        const response = await client.session.get({ path: { id }, signal: AbortSignal.timeout(500) });
        info = response.data;
      }
      if (!info) return;
      if (info.parentID) { children.add(id); return; }
      rootID = id;
      assistantID = "";
      lastAssistantMessage = "";
      waiting.clear();
      await report({ type: "session.created", properties: { info: { id } } });
    }
    if (id !== rootID || event.type === "session.created") return;
    if (event.type === "chat.message") {
      assistantID = "";
      lastAssistantMessage = "";
      waiting.clear();
    }
    if (event.type === "message.updated") {
      if (p.info.role === "assistant" && p.info.id !== assistantID) {
        assistantID = p.info.id;
        lastAssistantMessage = "";
      }
      return;
    }
    if (event.type === "message.part.updated") {
      if (p.part.type === "text" && p.part.messageID === assistantID) {
        lastAssistantMessage = p.part.text.slice(-1536);
      }
      return;
    }
    if (event.type.endsWith(".asked")) waiting.add(p.id);
    if (event.type.endsWith(".replied") || event.type === "question.rejected") {
      waiting.delete(p.requestID);
      if (waiting.size) return;
    }
    if (event.type === "session.status" && p.status.type !== "idle" && waiting.size) return;
    if (event.type === "session.idle" || (event.type === "session.status" && p.status.type === "idle")) {
      waiting.clear();
      event = { ...event, properties: { ...p, lastAssistantMessage } };
    }
    await report(event);
  }

  // Return to OpenCode immediately; serialize reports so status changes stay ordered.
  async function drain() {
    if (running) return;
    running = true;
    try {
      while (queue.length) {
        try { await processEvent(queue.shift()); } catch { /* fail open */ }
      }
    } finally { running = false; }
  }
  function enqueue(event) {
    if (queue.length >= 64) queue.shift();
    queue.push(event);
    void drain();
  }
  const supported = new Set([
    "session.created", "session.deleted", "session.status", "session.idle", "session.error",
    "permission.asked", "permission.replied", "question.asked", "question.replied", "question.rejected",
    "message.updated", "message.part.updated",
  ]);
  return {
    event: async ({ event }) => { if (supported.has(event.type)) enqueue(event); },
    "chat.message": async (input, output) => enqueue({
      type: "chat.message",
      properties: { sessionID: input.sessionID, prompt: output.parts.filter(p => p.type === "text").map(p => p.text).join("\n").slice(0, 1536) },
    }),
  };
};
