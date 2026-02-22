import { Client } from "@modelcontextprotocol/sdk/client";
// @ts-expect-error -- subpath resolves at runtime via jiti; tsc cannot follow the "./*" wildcard export
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio";
import { Type } from "@sinclair/typebox";

let mcpClient: Client | null = null;
let transport: InstanceType<typeof StdioClientTransport> | null = null;

async function getClient(
  mcpBinary: string,
  env: Record<string, string>,
): Promise<Client> {
  if (mcpClient) return mcpClient;

  transport = new StdioClientTransport({
    command: mcpBinary,
    env: { ...process.env, ...env } as Record<string, string>,
  });

  mcpClient = new Client({
    name: "openclaw-clawbrain",
    version: "1.0.0",
  });

  await mcpClient.connect(transport);
  return mcpClient;
}

function resolveConfig(api: any): {
  mcpBinary: string;
  env: Record<string, string>;
} {
  const pluginConfig =
    api.config?.plugins?.entries?.clawbrain?.config ?? {};
  return {
    mcpBinary: pluginConfig.mcpBinary || "clawbrain-mcp",
    env: pluginConfig.env ?? {},
  };
}

async function callTool(
  api: any,
  toolName: string,
  args: Record<string, unknown>,
): Promise<string> {
  const { mcpBinary, env } = resolveConfig(api);
  const client = await getClient(mcpBinary, env);
  const result = await client.callTool({ name: toolName, arguments: args });

  const textParts = (result.content as Array<{ type: string; text?: string }>)
    .filter((c) => c.type === "text" && c.text)
    .map((c) => c.text!);

  return textParts.join("\n");
}

export default function register(api: any) {
  // --- memory_add ---
  api.registerTool({
    name: "memory_add",
    description:
      "Store a memory. Text is embedded and stored in the vector database. Returns the memory's UUID. Use pinned for memories that should never expire.",
    parameters: Type.Object({
      text: Type.String({ description: "The text to store as a memory" }),
      payload: Type.Optional(
        Type.Record(Type.String(), Type.Any(), {
          description: "Additional metadata as a JSON object",
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
    async execute(_id: string, params: any) {
      const text = await callTool(api, "add", params);
      return { content: [{ type: "text", text }] };
    },
  });

  // --- memory_search ---
  api.registerTool({
    name: "memory_search",
    description:
      "Search memories by semantic similarity. Returns ranked results with similarity scores and a confidence level (high/medium/low/none). Call multiple times with different queries to deepen recall -- if confidence is low or none, rephrase and try again.",
    parameters: Type.Object({
      query: Type.String({
        description: "Text to search for (semantic search)",
      }),
      limit: Type.Optional(
        Type.Integer({
          description:
            "Maximum number of results (default 1). Use 3-5 for broader context.",
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
    async execute(_id: string, params: any) {
      const text = await callTool(api, "search", params);
      return { content: [{ type: "text", text }] };
    },
  });

  // --- memory_get ---
  api.registerTool({
    name: "memory_get",
    description:
      "Fetch a single memory by its UUID. Returns the full payload including text and metadata.",
    parameters: Type.Object({
      id: Type.String({ description: "UUID of the memory to fetch" }),
    }),
    async execute(_id: string, params: any) {
      const text = await callTool(api, "get", params);
      return { content: [{ type: "text", text }] };
    },
  });

  // --- memory_forget ---
  api.registerTool(
    {
      name: "memory_forget",
      description:
        "Prune stale memories. Deletes memories not accessed within the TTL window. Pinned memories are never deleted.",
      parameters: Type.Object({
        ttl: Type.Optional(
          Type.String({
            description:
              'Duration string (e.g. "720h" for 30 days). Memories not accessed within this window are deleted.',
          }),
        ),
      }),
      async execute(_id: string, params: any) {
        const text = await callTool(api, "forget", params);
        return { content: [{ type: "text", text }] };
      },
    },
    { optional: true },
  );

  // --- memory_check ---
  api.registerTool({
    name: "memory_check",
    description:
      "Verify connectivity to Qdrant (vector database) and Ollama (embedding model). Run this to confirm the memory system is operational.",
    parameters: Type.Object({}),
    async execute(_id: string, _params: any) {
      const text = await callTool(api, "check", {});
      return { content: [{ type: "text", text }] };
    },
  });

  // Clean up MCP connection on gateway shutdown
  api.registerService({
    id: "clawbrain-mcp",
    start: () => api.logger?.info?.("ClawBrain MCP plugin loaded"),
    stop: async () => {
      if (transport) {
        await transport.close();
        transport = null;
        mcpClient = null;
      }
    },
  });
}
