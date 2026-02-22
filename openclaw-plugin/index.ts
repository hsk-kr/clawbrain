import { execFile } from "node:child_process";
import { Type } from "@sinclair/typebox";

// ---------------------------------------------------------------------------
// Execution layer — runs clawbrain CLI via Docker or directly
// ---------------------------------------------------------------------------

interface PluginConfig {
  composePath?: string;
  serviceName?: string;
  binaryPath?: string;
}

function resolveConfig(api: any): PluginConfig {
  const cfg = api.config?.plugins?.entries?.clawbrain?.config ?? {};
  return {
    composePath: cfg.composePath,
    serviceName: cfg.serviceName || "clawbrain",
    binaryPath: cfg.binaryPath,
  };
}

function execPromise(
  cmd: string,
  args: string[],
): Promise<{ stdout: string; stderr: string }> {
  return new Promise((resolve, reject) => {
    execFile(cmd, args, { maxBuffer: 10 * 1024 * 1024 }, (err, stdout, stderr) => {
      if (err) {
        // CLI returns JSON errors on stdout with exit code 0 in some cases,
        // but real failures (binary not found, docker not running) come here.
        // If we got stdout, treat it as a result anyway (the CLI writes JSON
        // errors to stdout with exit code 1).
        if (stdout && stdout.trim()) {
          resolve({ stdout: stdout.trim(), stderr: stderr?.trim() ?? "" });
          return;
        }
        reject(new Error(stderr?.trim() || err.message));
        return;
      }
      resolve({ stdout: stdout.trim(), stderr: stderr?.trim() ?? "" });
    });
  });
}

/**
 * Run a clawbrain CLI command and return the raw JSON string.
 *
 * Two modes:
 * - Binary mode (binaryPath set): runs the binary directly
 * - Docker mode (default): docker compose exec -T <service> clawbrain ...
 */
async function runClawbrain(
  config: PluginConfig,
  args: string[],
): Promise<string> {
  if (config.binaryPath) {
    const { stdout } = await execPromise(config.binaryPath, args);
    return stdout;
  }

  const composeArgs: string[] = [];
  if (config.composePath) {
    composeArgs.push("-f", `${config.composePath}/docker-compose.yml`);
  }
  composeArgs.push("exec", "-T", config.serviceName!, "clawbrain", ...args);

  const { stdout } = await execPromise("docker", ["compose", ...composeArgs]);
  return stdout;
}

// ---------------------------------------------------------------------------
// Tool result helpers
// ---------------------------------------------------------------------------

function textResult(text: string) {
  return { content: [{ type: "text" as const, text }] };
}

function errResult(msg: string) {
  return { content: [{ type: "text" as const, text: JSON.stringify({ status: "error", message: msg }) }] };
}

// ---------------------------------------------------------------------------
// Plugin registration
// ---------------------------------------------------------------------------

export default function register(api: any) {
  const config = resolveConfig(api);

  // --- memory_add -----------------------------------------------------------
  api.registerTool({
    name: "memory_add",
    description:
      "Store a memory. Text is embedded via Ollama and stored in the vector database. Returns the memory's UUID.",
    parameters: Type.Object({
      text: Type.String({ description: "The text to store as a memory" }),
      payload: Type.Optional(
        Type.String({
          description: "Additional metadata as a JSON string (e.g. '{\"source\": \"chat\"}')",
        }),
      ),
      id: Type.Optional(
        Type.String({
          description: "UUID for the memory (auto-generated if omitted)",
        }),
      ),
      pinned: Type.Optional(
        Type.Boolean({
          description: "Pin this memory to prevent automatic forgetting",
        }),
      ),
    }),
    async execute(_id: string, params: { text: string; payload?: string; id?: string; pinned?: boolean }) {
      try {
        const args = ["add", "--text", params.text];
        if (params.payload) {
          args.push("--payload", params.payload);
        }
        if (params.id) {
          args.push("--id", params.id);
        }
        if (params.pinned) {
          args.push("--pinned");
        }
        const stdout = await runClawbrain(config, args);
        return textResult(stdout);
      } catch (e: any) {
        return errResult(e.message);
      }
    },
  });

  // --- memory_search --------------------------------------------------------
  api.registerTool({
    name: "memory_search",
    description:
      "Search memories by semantic similarity. Your query is embedded and compared against stored memories. Returns ranked results with similarity scores and a confidence level (high/medium/low/none). Call this multiple times with different or refined queries to deepen recall. If confidence is 'low' or 'none', rephrase your query or try a different angle before giving up. Increase the limit to 3-5 for broader context per search.",
    parameters: Type.Object({
      query: Type.String({
        description: "Text to search for (semantic search)",
      }),
      limit: Type.Optional(
        Type.Integer({
          description: "Maximum number of results (default 1). Use 3-5 for broader context.",
          minimum: 1,
        }),
      ),
      min_score: Type.Optional(
        Type.Number({
          description: "Minimum similarity score threshold (default 0.0)",
          minimum: 0,
          maximum: 1,
        }),
      ),
    }),
    async execute(_id: string, params: { query: string; limit?: number; min_score?: number }) {
      try {
        const args = ["search", "--query", params.query];
        if (params.limit !== undefined) {
          args.push("--limit", String(params.limit));
        }
        if (params.min_score !== undefined) {
          args.push("--min-score", String(params.min_score));
        }
        const stdout = await runClawbrain(config, args);
        return textResult(stdout);
      } catch (e: any) {
        return errResult(e.message);
      }
    },
  });

  // --- memory_get -----------------------------------------------------------
  api.registerTool({
    name: "memory_get",
    description:
      "Fetch a single memory by its UUID. Returns the full payload including text and metadata.",
    parameters: Type.Object({
      id: Type.String({ description: "UUID of the memory to fetch" }),
    }),
    async execute(_id: string, params: { id: string }) {
      try {
        const stdout = await runClawbrain(config, ["get", "--id", params.id]);
        return textResult(stdout);
      } catch (e: any) {
        return errResult(e.message);
      }
    },
  });

  // --- memory_forget --------------------------------------------------------
  api.registerTool(
    {
      name: "memory_forget",
      description:
        "Prune stale memories. Deletes memories not accessed within the TTL window. Pinned memories are never deleted. Returns the count of deleted memories.",
      parameters: Type.Object({
        ttl: Type.Optional(
          Type.String({
            description:
              'Duration string (e.g. "720h" for 30 days). Memories not accessed within this window are deleted.',
          }),
        ),
      }),
      async execute(_id: string, params: { ttl?: string }) {
        try {
          const args = ["forget"];
          if (params.ttl) {
            args.push("--ttl", params.ttl);
          }
          const stdout = await runClawbrain(config, args);
          return textResult(stdout);
        } catch (e: any) {
          return errResult(e.message);
        }
      },
    },
    { optional: true },
  );

  // --- memory_check ---------------------------------------------------------
  api.registerTool({
    name: "memory_check",
    description:
      "Verify connectivity to Qdrant (vector database) and Ollama (embedding model). Run this to confirm the memory system is operational.",
    parameters: Type.Object({}),
    async execute() {
      try {
        const stdout = await runClawbrain(config, ["check"]);
        return textResult(stdout);
      } catch (e: any) {
        return errResult(e.message);
      }
    },
  });
}

// ---------------------------------------------------------------------------
// Export internals for testing
// ---------------------------------------------------------------------------
export { runClawbrain, resolveConfig, type PluginConfig };
