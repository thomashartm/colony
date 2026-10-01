// schema = 1
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, readFile, rm, chmod } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { Motley } from "./motley.ts";

test("plugin reports in order, ignores subagents, contains reporter failures", async () => {
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
    let events = [];
    for (let i = 0; i < 100; i++) {
      await delay(25);
      try { events = (await readFile(log, "utf8")).trim().split("\n").map(JSON.parse); } catch {}
      if (events.length === 7) break;
    }
    assert.deepEqual(events.map(e => e.type), ["session.created", "chat.message", "permission.asked", "permission.replied", "question.asked", "question.replied", "session.idle"]);
    assert.equal(events.at(-1).properties.lastAssistantMessage, "Finished");
    assert.equal(events[0].properties.info.id, "root");
    await rm(reporter);
    // Missing reporter must neither reject nor keep a plugin callback waiting.
    const start = Date.now();
    await emit("session.error", { sessionID: "root", error: { name: "UnknownError" } });
    assert.ok(Date.now() - start < 100);
    await delay(100);
  } finally {
    process.env.PATH = oldPath;
    if (oldMember === undefined) delete process.env.MOTLEY_MEMBER; else process.env.MOTLEY_MEMBER = oldMember;
    await rm(dir, { recursive: true, force: true });
  }
});
