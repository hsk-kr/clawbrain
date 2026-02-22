package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hsk-coder/clawbrain/internal/ollama"
	"github.com/hsk-coder/clawbrain/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds the connection settings for Qdrant and Ollama.
type Config struct {
	QdrantHost string
	QdrantPort int
	OllamaURL  string
	Model      string
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadConfig() Config {
	port := 6334
	if v := os.Getenv("CLAWBRAIN_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &port)
	}
	return Config{
		QdrantHost: envOr("CLAWBRAIN_HOST", "localhost"),
		QdrantPort: port,
		OllamaURL:  envOr("CLAWBRAIN_OLLAMA_URL", "http://localhost:11434"),
		Model:      envOr("CLAWBRAIN_MODEL", "all-minilm"),
	}
}

// --- Tool input types ---

type AddInput struct {
	Text    string         `json:"text" jsonschema:"The text to store as a memory"`
	Payload map[string]any `json:"payload,omitempty" jsonschema:"Additional metadata as a JSON object"`
	ID      string         `json:"id,omitempty" jsonschema:"UUID for the memory (auto-generated if omitted)"`
	Pinned  bool           `json:"pinned,omitempty" jsonschema:"Pin this memory to prevent automatic forgetting"`
}

type GetInput struct {
	ID string `json:"id" jsonschema:"UUID of the memory to fetch"`
}

type SearchInput struct {
	Query    string  `json:"query" jsonschema:"Text to search for (semantic search)"`
	Limit    int     `json:"limit,omitempty" jsonschema:"Maximum number of results (default 1)"`
	MinScore float64 `json:"min_score,omitempty" jsonschema:"Minimum similarity score threshold (default 0.0)"`
}

type ForgetInput struct {
	TTL string `json:"ttl,omitempty" jsonschema:"Duration string - memories not accessed within this window are deleted (default 720h)"`
}

type CheckInput struct{}

// --- Helpers ---

func errResult(msg string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}, nil, nil
}

func textResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}

func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return errResult(fmt.Sprintf("json marshal: %v", err))
	}
	return textResult(string(data))
}

func confidence(results []store.Result) string {
	if len(results) == 0 {
		return "none"
	}
	top := results[0].Score
	switch {
	case top >= 0.7:
		return "high"
	case top >= 0.4:
		return "medium"
	default:
		return "low"
	}
}

// --- Tool handlers ---

func handleAdd(cfg Config) func(context.Context, *mcp.CallToolRequest, AddInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, any, error) {
		if input.Text == "" {
			return errResult("text is required")
		}

		s, err := store.New(cfg.QdrantHost, cfg.QdrantPort)
		if err != nil {
			return errResult(fmt.Sprintf("connect to qdrant: %v", err))
		}
		defer s.Close()

		oc := ollama.New(cfg.OllamaURL)
		vector, err := oc.Embed(ctx, cfg.Model, input.Text)
		if err != nil {
			return errResult(fmt.Sprintf("embedding failed: %v", err))
		}

		payload := input.Payload
		if payload == nil {
			payload = make(map[string]any)
		}
		payload["text"] = input.Text
		if input.Pinned {
			payload["pinned"] = true
		}

		pointID, err := s.Add(ctx, input.ID, vector, payload)
		if err != nil {
			return errResult(fmt.Sprintf("store failed: %v", err))
		}

		return jsonResult(map[string]any{
			"status": "ok",
			"id":     pointID,
		})
	}
}

func handleGet(cfg Config) func(context.Context, *mcp.CallToolRequest, GetInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, any, error) {
		if input.ID == "" {
			return errResult("id is required")
		}

		s, err := store.New(cfg.QdrantHost, cfg.QdrantPort)
		if err != nil {
			return errResult(fmt.Sprintf("connect to qdrant: %v", err))
		}
		defer s.Close()

		result, err := s.Get(ctx, input.ID)
		if err != nil {
			return errResult(fmt.Sprintf("get failed: %v", err))
		}
		if result == nil {
			return errResult(fmt.Sprintf("memory %s not found", input.ID))
		}

		return jsonResult(map[string]any{
			"status":  "ok",
			"id":      result.ID,
			"payload": result.Payload,
		})
	}
}

func handleSearch(cfg Config) func(context.Context, *mcp.CallToolRequest, SearchInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, any, error) {
		if input.Query == "" {
			return errResult("query is required")
		}

		limit := uint64(input.Limit)
		if limit == 0 {
			limit = 1
		}

		s, err := store.New(cfg.QdrantHost, cfg.QdrantPort)
		if err != nil {
			return errResult(fmt.Sprintf("connect to qdrant: %v", err))
		}
		defer s.Close()

		oc := ollama.New(cfg.OllamaURL)
		vector, err := oc.Embed(ctx, cfg.Model, input.Query)
		if err != nil {
			return errResult(fmt.Sprintf("embedding failed: %v", err))
		}

		results, err := s.Retrieve(ctx, vector, float32(input.MinScore), limit)
		if err != nil {
			return errResult(fmt.Sprintf("search failed: %v", err))
		}

		return jsonResult(map[string]any{
			"status":     "ok",
			"results":    results,
			"returned":   len(results),
			"confidence": confidence(results),
		})
	}
}

func handleForget(cfg Config) func(context.Context, *mcp.CallToolRequest, ForgetInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input ForgetInput) (*mcp.CallToolResult, any, error) {
		ttlStr := input.TTL
		if ttlStr == "" {
			ttlStr = "720h"
		}

		ttl, err := time.ParseDuration(ttlStr)
		if err != nil {
			return errResult(fmt.Sprintf("invalid TTL: %v", err))
		}

		s, err := store.New(cfg.QdrantHost, cfg.QdrantPort)
		if err != nil {
			return errResult(fmt.Sprintf("connect to qdrant: %v", err))
		}
		defer s.Close()

		deleted, err := s.Forget(ctx, ttl)
		if err != nil {
			return errResult(fmt.Sprintf("forget failed: %v", err))
		}

		return jsonResult(map[string]any{
			"status":  "ok",
			"deleted": deleted,
			"ttl":     ttlStr,
		})
	}
}

func handleCheck(cfg Config) func(context.Context, *mcp.CallToolRequest, CheckInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input CheckInput) (*mcp.CallToolResult, any, error) {
		s, err := store.New(cfg.QdrantHost, cfg.QdrantPort)
		if err != nil {
			return errResult(fmt.Sprintf("connect to qdrant: %v", err))
		}
		defer s.Close()

		if err := s.Check(ctx); err != nil {
			return errResult(fmt.Sprintf("qdrant: %v", err))
		}

		oc := ollama.New(cfg.OllamaURL)
		if err := oc.Health(ctx); err != nil {
			return errResult(fmt.Sprintf("ollama: %v", err))
		}

		return jsonResult(map[string]any{
			"status":  "ok",
			"message": "Qdrant and Ollama verified",
		})
	}
}

func main() {
	cfg := loadConfig()

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "clawbrain",
			Version: "1.0.0",
		},
		nil,
	)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add",
		Description: "Store a memory. Text is embedded via Ollama and stored in the vector database. Returns the memory's UUID.",
	}, handleAdd(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get",
		Description: "Fetch a single memory by its UUID. Returns the full payload including text and metadata.",
	}, handleGet(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search memories by semantic similarity. Your query is embedded and compared against stored memories. Returns ranked results with similarity scores and a confidence level (high/medium/low/none). Call this multiple times with different or refined queries to deepen recall. If confidence is 'low' or 'none', rephrase your query or try a different angle before giving up. Increase the limit to 3-5 for broader context per search.",
	}, handleSearch(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "forget",
		Description: "Prune stale memories. Deletes memories not accessed within the TTL window. Pinned memories are never deleted. Returns the count of deleted memories.",
	}, handleForget(cfg))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check",
		Description: "Verify connectivity to Qdrant (vector database) and Ollama (embedding model). Run this to confirm the memory system is operational.",
	}, handleCheck(cfg))

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
