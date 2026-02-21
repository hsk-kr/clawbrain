package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hsk-coder/clawbrain/internal/store"
)

// Global connection settings, set by parseGlobals.
var (
	globalHost = "localhost"
	globalPort = 6334
)

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
	case "retrieve":
		runRetrieve(args[1:])
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

// parseGlobals extracts --host and --port from the argument list and
// returns the remaining arguments (command + subcommand flags).
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
		default:
			remaining = append(remaining, args[i])
		}
	}
	return remaining
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: clawbrain [--host HOST] [--port PORT] <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Global flags:")
	fmt.Fprintln(os.Stderr, "  --host         Qdrant host (default: localhost)")
	fmt.Fprintln(os.Stderr, "  --port         Qdrant gRPC port (default: 6334)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  add            Store a memory")
	fmt.Fprintln(os.Stderr, "  retrieve       Query similar memories")
	fmt.Fprintln(os.Stderr, "  forget         Remove stale memories")
	fmt.Fprintln(os.Stderr, "  collections    List all collections")
	fmt.Fprintln(os.Stderr, "  check          Verify Qdrant connectivity")
}

func runAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	collection := fs.String("collection", "", "Target collection name (required)")
	vectorJSON := fs.String("vector", "", "Embedding vector as JSON array (required)")
	payloadJSON := fs.String("payload", "", "Metadata as JSON object (required)")
	id := fs.String("id", "", "UUID for the point (auto-generated if omitted)")
	fs.Parse(args)

	if *collection == "" || *vectorJSON == "" || *payloadJSON == "" {
		fmt.Fprintln(os.Stderr, "Error: --collection, --vector, and --payload are required")
		fs.Usage()
		os.Exit(1)
	}

	var vector []float32
	if err := json.Unmarshal([]byte(*vectorJSON), &vector); err != nil {
		exitJSON("error", fmt.Sprintf("invalid vector JSON: %v", err))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(*payloadJSON), &payload); err != nil {
		exitJSON("error", fmt.Sprintf("invalid payload JSON: %v", err))
	}

	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	pointID, err := s.Add(ctx, *collection, *id, vector, payload)
	if err != nil {
		exitJSON("error", err.Error())
	}

	outputJSON(map[string]any{
		"status":     "ok",
		"id":         pointID,
		"collection": *collection,
	})
}

func runRetrieve(args []string) {
	fs := flag.NewFlagSet("retrieve", flag.ExitOnError)
	collection := fs.String("collection", "", "Collection to search (required)")
	vectorJSON := fs.String("vector", "", "Query embedding as JSON array (required)")
	minScore := fs.Float64("min-score", 0.0, "Minimum similarity score threshold")
	limit := fs.Uint64("limit", 1, "Maximum number of results")
	recencyBoost := fs.Float64("recency-boost", 0.0, "Recency boost weight (0.0 = off, higher = stronger short-term memory effect)")
	recencyScale := fs.Float64("recency-scale", 3600, "Seconds until recency boost decays to half strength")
	fs.Parse(args)

	if *collection == "" || *vectorJSON == "" {
		fmt.Fprintln(os.Stderr, "Error: --collection and --vector are required")
		fs.Usage()
		os.Exit(1)
	}

	var vector []float32
	if err := json.Unmarshal([]byte(*vectorJSON), &vector); err != nil {
		exitJSON("error", fmt.Sprintf("invalid vector JSON: %v", err))
	}

	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	results, err := s.Retrieve(ctx, *collection, vector, float32(*minScore), *limit, float32(*recencyBoost), float32(*recencyScale))
	if err != nil {
		exitJSON("error", err.Error())
	}

	outputJSON(map[string]any{
		"status":  "ok",
		"results": results,
		"count":   len(results),
	})
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

	if err := s.Check(ctx); err != nil {
		exitJSON("error", err.Error())
	}

	outputJSON(map[string]any{
		"status":  "ok",
		"message": "ClawBrain stack verified",
	})
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
