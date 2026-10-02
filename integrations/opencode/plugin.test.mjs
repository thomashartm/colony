// schema = 1
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, readFile, rm, chmod } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { Motley } from "./motley.ts";

// withPlugin loads the plugin as a member with a reporter that appends each
// payload to a log, restoring the environment afterwards.
async function withPlugin(body) {
  const dir = await mkdtemp(join(tmpdir(), "motley-plugin-"));
  const oldPath = process.env.PATH;
  const oldMember = process.env.MOTLEY_MEMBER;
  const log = join(dir, "events.jsonl");
  try {
    delete process.env.MOTLEY_MEMBER;
    assert.deepEqual(await Motley({}), {});
    const reporter = join(dir, "motley");
    await writeFile(reporter, `#!${process.execPath}\nimport fs from 'node:fs';let text='';process.stdin.on('data',b=>text+=b);process.stdin.on('end',()=>fs.appendFileSync(${JSON.stringify(log)},text+'\\n'));\n`);
    await chmod(reporter, 0o755);
    process.env.PATH = dir;
    process.env.MOTLEY_MEMBER = "fixture";
    const client = { session: { get: async ({ path }) => ({ data: { id: path.id, ...(path.id === "child" ? { parentID: "root" } : {}) } }) } };
    const hooks = await Motley({ client });
    const emit = (type, properties) => hooks.event({ event: { type, properties } });
    const reported = async (count) => {
      let events = [];
      for (let i = 0; i < 100; i++) {
        await delay(25);
        try { events = (await readFile(log, "utf8")).trim().split("\n").map(JSON.parse); } catch {}
        if (events.length === count) break;
      }
      return events;
    };
    await body({ hooks, emit, reported, reporter });
  } finally {
    process.env.PATH = oldPath;
    if (oldMember === undefined) delete process.env.MOTLEY_MEMBER; else process.env.MOTLEY_MEMBER = oldMember;
    await rm(dir, { recursive: true, force: true });
  }
}

test("plugin reports in order, ignores subagents, contains reporter failures", () => withPlugin(async ({ hooks, emit, reported, reporter }) => {
  // First prompt can be in a resumed session: bind via the SDK, never guess a child is the root.
  await hooks["chat.message"]({ sessionID: "root" }, { parts: [{ type: "text", text: "Plan it" }] });
  await emit("session.created", { info: { id: "child", parentID: "root" } });
  await emit("session.idle", { sessionID: "child" });
  await emit("permission.asked", { sessionID: "root", id: "p1", permission: "bash", patterns: ["git push"] });
  await emit("session.status", { sessionID: "root", status: { type: "busy" } });
  await emit("permission.replied", { sessionID: "root", requestID: "p1" });
  await emit("question.asked", { sessionID: "root", id: "q1", questions: [{ question: "Which?", options: [] }] });
  await emit("question.replied", { sessionID: "root", requestID: "q1" });
  await emit("message.updated", { info: { sessionID: "root", id: "answer", role: "assistant" } });
  await emit("message.part.updated", { part: { sessionID: "root", messageID: "answer", type: "text", text: "Finished" } });
  await emit("session.idle", { sessionID: "root" });
  const events = await reported(7);
  assert.deepEqual(events.map(e => e.type), ["session.created", "chat.message", "permission.asked", "permission.replied", "question.asked", "question.replied", "session.idle"]);
  assert.equal(events.at(-1).properties.lastAssistantMessage, "Finished");
  assert.equal(events[0].properties.info.id, "root");
  await rm(reporter);
  // Missing reporter must neither reject nor keep a plugin callback waiting.
  const start = Date.now();
  await emit("session.error", { sessionID: "root", error: { name: "UnknownError" } });
  assert.ok(Date.now() - start < 100);
  await delay(100);
}));

// Recorded live from OpenCode 1.18.21: an abort sends session.error, then
// idle, and idle again after the partial reply's last text update.
test("plugin marks idle after an abort as interrupted until the next prompt", () => withPlugin(async ({ hooks, emit, reported }) => {
  await hooks["chat.message"]({ sessionID: "root" }, { parts: [{ type: "text", text: "Write an essay" }] });
  await emit("message.updated", { info: { sessionID: "root", id: "essay", role: "assistant" } });
  await emit("session.error", { sessionID: "root", error: { name: "MessageAbortedError", data: { message: "Aborted" } } });
  await emit("session.status", { sessionID: "root", status: { type: "idle" } });
  await emit("session.idle", { sessionID: "root" });
  await emit("message.part.updated", { part: { sessionID: "root", messageID: "essay", type: "text", text: "Partial" } });
  await emit("session.idle", { sessionID: "root" });
  await hooks["chat.message"]({ sessionID: "root" }, { parts: [{ type: "text", text: "Try again" }] });
  await emit("session.idle", { sessionID: "root" });
  const events = await reported(8);
  assert.deepEqual(events.map(e => [e.type, e.properties.interrupted]), [
    ["session.created", undefined], ["chat.message", undefined], ["session.error", undefined],
    ["session.status", true], ["session.idle", true], ["session.idle", true],
    ["chat.message", undefined], ["session.idle", undefined],
  ]);
}));
