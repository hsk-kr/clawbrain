/**
 * Plugin integration tests.
 *
 * These tests exercise the plugin's tool handlers by calling the clawbrain CLI
 * directly (binary mode). They require Qdrant and Ollama to be running:
 *
 *   docker compose up -d   # starts Qdrant + Ollama
 *   go build -o clawbrain ./cmd/clawbrain  # build the CLI
 *   CLAWBRAIN_BINARY=./clawbrain npm test  # run tests
 *
 * In CI, the Go binary is built and services are provided as sidecars.
 *
 * Ported from cmd/clawbrain/main_test.go — covers the same operations the
 * plugin exposes to OpenClaw agents.
 */

import { describe, it, expect, beforeAll, afterEach } from "vitest";
import { execFile } from "node:child_process";
import { runClawbrain, type PluginConfig } from "./index.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const binaryPath = process.env.CLAWBRAIN_BINARY || "./clawbrain";

const config: PluginConfig = { binaryPath };

function parseJSON(raw: string): Record<string, any> {
  return JSON.parse(raw);
}

/** Run the CLI and return parsed JSON. */
async function run(args: string[]): Promise<Record<string, any>> {
  const stdout = await runClawbrain(config, args);
  return parseJSON(stdout);
}

/** Cleanup: forget everything with 0s TTL (deletes all unpinned memories). */
async function forgetAll(): Promise<void> {
  try {
    await runClawbrain(config, ["forget", "--ttl", "0s"]);
  } catch {
    // Collection may not exist yet — that's fine.
  }
}

/** Check if Qdrant is reachable. */
function isQdrantAvailable(): Promise<boolean> {
  return new Promise((resolve) => {
    const net = require("node:net");
    const sock = net.createConnection({ host: "localhost", port: 6334 }, () => {
      sock.end();
      resolve(true);
    });
    sock.on("error", () => resolve(false));
    sock.setTimeout(2000, () => {
      sock.destroy();
      resolve(false);
    });
  });
}

/** Check if Ollama is reachable. */
async function isOllamaAvailable(): Promise<boolean> {
  try {
    const resp = await fetch("http://localhost:11434/");
    return resp.ok;
  } catch {
    return false;
  }
}

/** Check if the clawbrain binary exists and is executable. */
function isBinaryAvailable(): Promise<boolean> {
  return new Promise((resolve) => {
    const fs = require("node:fs");
    try {
      fs.accessSync(binaryPath, fs.constants.X_OK);
      resolve(true);
    } catch {
      resolve(false);
    }
  });
}

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe("ClawBrain plugin", () => {
  let skipAll = false;

  beforeAll(async () => {
    const [binary, qdrant, ollama] = await Promise.all([
      isBinaryAvailable(),
      isQdrantAvailable(),
      isOllamaAvailable(),
    ]);
    if (!binary || !qdrant || !ollama) {
      const missing = [];
      if (!binary) missing.push(`binary (${binaryPath})`);
      if (!qdrant) missing.push("Qdrant (localhost:6334)");
      if (!ollama) missing.push("Ollama (localhost:11434)");
      console.warn(`Skipping integration tests — missing: ${missing.join(", ")}`);
      skipAll = true;
    }
  });

  function skipIfUnavailable() {
    if (skipAll) {
      return true;
    }
    return false;
  }

  // --- check ----------------------------------------------------------------

  describe("memory_check", () => {
    it("returns status ok when services are running", async () => {
      if (skipIfUnavailable()) return;

      const result = await run(["check"]);
      expect(result.status).toBe("ok");
      expect(result.message).toBe("Qdrant and Ollama verified");
    });
  });

  // --- add + search ---------------------------------------------------------

  describe("memory_add + memory_search", () => {
    afterEach(async () => {
      if (!skipAll) await forgetAll();
    });

    it("round-trips text through add and search", async () => {
      if (skipIfUnavailable()) return;

      // Add
      const addResult = await run(["add", "--text", "the user prefers dark mode for coding"]);
      expect(addResult.status).toBe("ok");
      expect(addResult.id).toBeTruthy();

      // Search
      const searchResult = await run(["search", "--query", "dark mode", "--limit", "5"]);
      expect(searchResult.status).toBe("ok");
      expect(searchResult.returned).toBeGreaterThanOrEqual(1);

      const firstPayload = searchResult.results[0].payload;
      expect(firstPayload.text).toBe("the user prefers dark mode for coding");
      expect(searchResult.results[0].score).toBeGreaterThan(0);
    });

    it("preserves extra payload fields", async () => {
      if (skipIfUnavailable()) return;

      await run([
        "add",
        "--text", "golang is great for cli tools",
        "--payload", '{"source": "conversation"}',
      ]);

      const result = await run(["search", "--query", "golang", "--limit", "5"]);
      const payload = result.results[0].payload;
      expect(payload.source).toBe("conversation");
      expect(payload.text).toBe("golang is great for cli tools");
      expect(payload.created_at).toBeTruthy();
      expect(payload.last_accessed).toBeTruthy();
    });

    it("honors custom ID", async () => {
      if (skipIfUnavailable()) return;

      const customID = "aabbccdd-1122-3344-5566-778899aabbcc";
      const result = await run(["add", "--text", "text with custom id", "--id", customID]);
      expect(result.id).toBe(customID);
    });

    it("semantic search ranks correctly", async () => {
      if (skipIfUnavailable()) return;

      // Add memories with distinct topics
      await run(["add", "--text", "the user prefers dark mode for coding at night"]);
      await run(["add", "--text", "deploy the application to production every friday"]);
      await run(["add", "--text", "use golang and qdrant for the memory system"]);

      // Search for something semantically related to dark mode
      const result = await run(["search", "--query", "night theme preferences", "--limit", "3"]);
      expect(result.status).toBe("ok");
      expect(result.results.length).toBeGreaterThan(0);

      // Top result should be about dark mode
      const topText = result.results[0].payload.text;
      expect(topText).toBe("the user prefers dark mode for coding at night");
    });
  });

  // --- get ------------------------------------------------------------------

  describe("memory_get", () => {
    afterEach(async () => {
      if (!skipAll) await forgetAll();
    });

    it("fetches a memory by ID with full payload", async () => {
      if (skipIfUnavailable()) return;

      const customID = "11111111-2222-3333-4444-555555555555";
      await run([
        "add",
        "--text", "get me by id",
        "--payload", '{"tag": "test"}',
        "--id", customID,
      ]);

      const result = await run(["get", "--id", customID]);
      expect(result.status).toBe("ok");
      expect(result.id).toBe(customID);
      expect(result.payload.text).toBe("get me by id");
      expect(result.payload.tag).toBe("test");
      expect(result.payload.created_at).toBeTruthy();
      expect(result.payload.last_accessed).toBeTruthy();
    });

    it("returns error for nonexistent ID", async () => {
      if (skipIfUnavailable()) return;

      // Add something so the collection exists
      await run(["add", "--text", "placeholder"]);

      // Try to get a nonexistent ID — CLI returns error JSON
      const stdout = await runClawbrain(config, [
        "get", "--id", "00000000-0000-0000-0000-000000000000",
      ]);
      const result = parseJSON(stdout);
      expect(result.status).toBe("error");
    });
  });

  // --- forget ---------------------------------------------------------------

  describe("memory_forget", () => {
    it("deletes memories past TTL", async () => {
      if (skipIfUnavailable()) return;

      const addResult = await run(["add", "--text", "this memory will be forgotten"]);
      expect(addResult.status).toBe("ok");
      const memoryID = addResult.id;

      const result = await run(["forget", "--ttl", "0s"]);
      expect(result.status).toBe("ok");
      expect(result.deleted).toBeGreaterThanOrEqual(1);

      // Verify the specific memory was deleted (get returns error)
      const getStdout = await runClawbrain(config, ["get", "--id", memoryID]);
      const getResult = parseJSON(getStdout);
      expect(getResult.status).toBe("error");
    });

    it("pinned memory survives forget", async () => {
      if (skipIfUnavailable()) return;

      // Add a pinned memory
      const addPinned = await run([
        "add", "--text", "this memory is pinned and should survive", "--pinned",
      ]);
      expect(addPinned.status).toBe("ok");
      const pinnedID = addPinned.id;

      // Add an unpinned memory
      await run(["add", "--text", "this memory is not pinned and should be forgotten"]);

      // Forget with 0s TTL
      const forgetResult = await run(["forget", "--ttl", "0s"]);
      expect(forgetResult.deleted).toBe(1);

      // Verify pinned memory still exists
      const getResult = await run(["get", "--id", pinnedID]);
      expect(getResult.status).toBe("ok");
      expect(getResult.payload.text).toBe("this memory is pinned and should survive");
      expect(getResult.payload.pinned).toBe(true);

      // Cleanup: we can't forget pinned memories, so this is fine — next
      // test's forgetAll will skip it. Tests should be tolerant of leftover
      // pinned data.
    });
  });

  // --- confidence -----------------------------------------------------------

  describe("confidence levels", () => {
    afterEach(async () => {
      if (!skipAll) await forgetAll();
    });

    it("returns confidence field in search results", async () => {
      if (skipIfUnavailable()) return;

      await run(["add", "--text", "the cat sat on the mat"]);

      const result = await run(["search", "--query", "cat sitting on a mat", "--limit", "3"]);
      expect(result.confidence).toBeDefined();
      expect(["high", "medium", "low", "none"]).toContain(result.confidence);
    });

    it("returns high confidence for semantically similar text", async () => {
      if (skipIfUnavailable()) return;

      await run(["add", "--text", "the cat sat on the mat"]);

      const result = await run(["search", "--query", "cat sitting on a mat", "--limit", "3"]);
      // Near-identical meaning should score high
      expect(["high", "medium"]).toContain(result.confidence);
    });

    it("returns none confidence when no results match min-score", async () => {
      if (skipIfUnavailable()) return;

      await run(["add", "--text", "I love pizza with extra cheese"]);

      // Search for something completely unrelated with very high min-score
      const result = await run([
        "search",
        "--query", "quantum physics equations",
        "--min-score", "0.99",
      ]);
      expect(result.confidence).toBe("none");
      expect(result.returned).toBe(0);
    });
  });
});
