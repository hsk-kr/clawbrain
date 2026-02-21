package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hsk-coder/clawbrain/internal/ollama"
	"github.com/hsk-coder/clawbrain/internal/store"
)

// Global connection settings, set by parseGlobals.
var (
	globalHost      = "localhost"
	globalPort      = 6334
	globalOllamaURL = "http://localhost:11434"
	globalModel     = "all-minilm"
)

func init() {
	// Environment variables override defaults (before flags override both).
	if v := os.Getenv("CLAWBRAIN_OLLAMA_URL"); v != "" {
		globalOllamaURL = v
	}
	if v := os.Getenv("CLAWBRAIN_MODEL"); v != "" {
		globalModel = v
	}
}

func main() {
	args := parseGlobals(os.Args[1:])

	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]

	switch command {
	case "add":
		runAdd(args[1:])
	case "get":
		runGet(args[1:])
	case "search":
		runSearch(args[1:])
	case "forget":
		runForget(args[1:])
	case "collections":
		runCollections()
	case "check":
		runCheck()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

// parseGlobals extracts --host, --port, --ollama-url, and --model from the
// argument list and returns the remaining arguments (command + subcommand flags).
func parseGlobals(args []string) []string {
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			if i+1 < len(args) {
				globalHost = args[i+1]
				i++
			}
		case "--port":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &globalPort)
				i++
			}
		case "--ollama-url":
			if i+1 < len(args) {
				globalOllamaURL = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				globalModel = args[i+1]
				i++
			}
		default:
			remaining = append(remaining, args[i])
		}
	}
	return remaining
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: clawbrain [--host HOST] [--port PORT] [--ollama-url URL] [--model MODEL] <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Global flags:")
	fmt.Fprintln(os.Stderr, "  --host         Qdrant host (default: localhost, env: CLAWBRAIN_HOST)")
	fmt.Fprintln(os.Stderr, "  --port         Qdrant gRPC port (default: 6334)")
	fmt.Fprintln(os.Stderr, "  --ollama-url   Ollama base URL (default: http://localhost:11434, env: CLAWBRAIN_OLLAMA_URL)")
	fmt.Fprintln(os.Stderr, "  --model        Embedding model (default: all-minilm, env: CLAWBRAIN_MODEL)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  add            Store a memory (--text 'your text here')")
	fmt.Fprintln(os.Stderr, "  get            Fetch a memory by ID (--id <uuid>)")
	fmt.Fprintln(os.Stderr, "  search         Search memories (--query 'search text')")
	fmt.Fprintln(os.Stderr, "  forget         Remove stale memories")
	fmt.Fprintln(os.Stderr, "  collections    List all collections")
	fmt.Fprintln(os.Stderr, "  check          Verify Qdrant and Ollama connectivity")
}

func runGet(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	collection := fs.String("collection", "", "Collection to fetch from (required)")
	id := fs.String("id", "", "UUID of the memory to fetch (required)")
	fs.Parse(args)

	if *collection == "" {
		fmt.Fprintln(os.Stderr, "Error: --collection is required")
		fs.Usage()
		os.Exit(1)
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "Error: --id is required")
		fs.Usage()
		os.Exit(1)
	}

	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	result, err := s.Get(ctx, *collection, *id)
	if err != nil {
		exitJSON("error", err.Error())
	}

	if result == nil {
		exitJSON("error", fmt.Sprintf("memory %s not found in collection %s", *id, *collection))
	}

	outputJSON(map[string]any{
		"status":  "ok",
		"id":      result.ID,
		"payload": result.Payload,
	})
}

func runAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	collection := fs.String("collection", "", "Target collection name (required)")
	text := fs.String("text", "", "Text to store as a memory (default mode)")
	payloadJSON := fs.String("payload", "", "Additional metadata as JSON object")
	vectorJSON := fs.String("vector", "", "Embedding vector as JSON array (advanced, overrides text mode)")
	id := fs.String("id", "", "UUID for the point (auto-generated if omitted)")
	fs.Parse(args)

	if *collection == "" {
		fmt.Fprintln(os.Stderr, "Error: --collection is required")
		fs.Usage()
		os.Exit(1)
	}

	// Parse optional payload
	var payload map[string]any
	if *payloadJSON != "" {
		if err := json.Unmarshal([]byte(*payloadJSON), &payload); err != nil {
			exitJSON("error", fmt.Sprintf("invalid payload JSON: %v", err))
		}
	} else {
		payload = make(map[string]any)
	}

	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	if *vectorJSON != "" {
		// Advanced vector mode: user provides their own embedding
		var vector []float32
		if err := json.Unmarshal([]byte(*vectorJSON), &vector); err != nil {
			exitJSON("error", fmt.Sprintf("invalid vector JSON: %v", err))
		}

		pointID, err := s.Add(ctx, *collection, *id, vector, payload)
		if err != nil {
			exitJSON("error", err.Error())
		}

		outputJSON(map[string]any{
			"status":     "ok",
			"id":         pointID,
			"collection": *collection,
		})
	} else if *text != "" {
		// Default text mode: embed via Ollama, then store
		oc := ollama.New(globalOllamaURL)
		vector, err := oc.Embed(ctx, globalModel, *text)
		if err != nil {
			exitJSON("error", fmt.Sprintf("embedding failed: %v", err))
		}

		// Store the original text in payload so it can be returned on retrieval
		payload["text"] = *text

		pointID, err := s.Add(ctx, *collection, *id, vector, payload)
		if err != nil {
			exitJSON("error", err.Error())
		}

		outputJSON(map[string]any{
			"status":     "ok",
			"id":         pointID,
			"collection": *collection,
		})
	} else {
		fmt.Fprintln(os.Stderr, "Error: --text is required (or --vector for advanced mode)")
		fs.Usage()
		os.Exit(1)
	}
}

func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	collection := fs.String("collection", "", "Collection to search (required)")
	query := fs.String("query", "", "Text to search for (default mode)")
	vectorJSON := fs.String("vector", "", "Query embedding as JSON array (advanced, overrides text mode)")
	minScore := fs.Float64("min-score", 0.0, "Minimum similarity score threshold")
	limit := fs.Uint64("limit", 1, "Maximum number of results")
	recencyBoost := fs.Float64("recency-boost", 0.0, "Recency boost weight (0 = off)")
	recencyScale := fs.Float64("recency-scale", 3600, "Seconds until recency boost decays to half strength")
	fs.Parse(args)

	if *collection == "" {
		fmt.Fprintln(os.Stderr, "Error: --collection is required")
		fs.Usage()
		os.Exit(1)
	}

	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	if *vectorJSON != "" {
		// Advanced vector mode
		var vector []float32
		if err := json.Unmarshal([]byte(*vectorJSON), &vector); err != nil {
			exitJSON("error", fmt.Sprintf("invalid vector JSON: %v", err))
		}

		results, err := s.Retrieve(ctx, *collection, vector, float32(*minScore), *limit, float32(*recencyBoost), float32(*recencyScale))
		if err != nil {
			exitJSON("error", err.Error())
		}

		outputJSON(map[string]any{
			"status":     "ok",
			"results":    results,
			"count":      len(results),
			"confidence": confidence(results),
		})
	} else if *query != "" {
		// Default text mode: embed query via Ollama, then search
		oc := ollama.New(globalOllamaURL)
		vector, err := oc.Embed(ctx, globalModel, *query)
		if err != nil {
			exitJSON("error", fmt.Sprintf("embedding failed: %v", err))
		}

		results, err := s.Retrieve(ctx, *collection, vector, float32(*minScore), *limit, float32(*recencyBoost), float32(*recencyScale))
		if err != nil {
			exitJSON("error", err.Error())
		}

		outputJSON(map[string]any{
			"status":     "ok",
			"results":    results,
			"count":      len(results),
			"confidence": confidence(results),
		})
	} else {
		fmt.Fprintln(os.Stderr, "Error: --query is required (or --vector for advanced mode)")
		fs.Usage()
		os.Exit(1)
	}
}

func runForget(args []string) {
	fs := flag.NewFlagSet("forget", flag.ExitOnError)
	collection := fs.String("collection", "", "Collection to prune (required)")
	ttlStr := fs.String("ttl", "720h", "Duration — memories not accessed within this window are deleted")
	fs.Parse(args)

	if *collection == "" {
		fmt.Fprintln(os.Stderr, "Error: --collection is required")
		fs.Usage()
		os.Exit(1)
	}

	ttl, err := time.ParseDuration(*ttlStr)
	if err != nil {
		exitJSON("error", fmt.Sprintf("invalid TTL: %v", err))
	}

	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	deleted, err := s.Forget(ctx, *collection, ttl)
	if err != nil {
		exitJSON("error", err.Error())
	}

	outputJSON(map[string]any{
		"status":     "ok",
		"deleted":    deleted,
		"collection": *collection,
		"ttl":        ttlStr,
	})
}

func runCollections() {
	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	names, err := s.Collections(ctx)
	if err != nil {
		exitJSON("error", err.Error())
	}

	outputJSON(map[string]any{
		"status":      "ok",
		"collections": names,
		"count":       len(names),
	})
}

func runCheck() {
	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	// Check Qdrant
	if err := s.Check(ctx); err != nil {
		exitJSON("error", fmt.Sprintf("qdrant: %v", err))
	}

	// Check Ollama
	oc := ollama.New(globalOllamaURL)
	if err := oc.Health(ctx); err != nil {
		exitJSON("error", fmt.Sprintf("ollama: %v", err))
	}

	outputJSON(map[string]any{
		"status":  "ok",
		"message": "Qdrant and Ollama verified",
	})
}

// confidence returns a confidence label based on the top result score.
// This helps agents quickly assess whether the results are trustworthy
// without needing to interpret raw similarity scores.
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

// connect creates a store connection and a context with timeout.
// The caller should defer both s.Close() and cancel().
func connect() (*store.Store, context.Context, context.CancelFunc) {
	s, err := store.New(globalHost, globalPort)
	if err != nil {
		exitJSON("error", err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	return s, ctx, cancel
}

// outputJSON marshals the value and prints it to stdout.
func outputJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"status":"error","message":"json marshal: %v"}`, err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// exitJSON outputs an error as JSON and exits with code 1.
func exitJSON(status string, message string) {
	outputJSON(map[string]any{
		"status":  status,
		"message": message,
	})
	os.Exit(1)
}
