package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hsk-coder/clawbrain/internal/ollama"
	"github.com/hsk-coder/clawbrain/internal/redis"
	"github.com/hsk-coder/clawbrain/internal/store"
	"github.com/hsk-coder/clawbrain/internal/sync"
)

// Global connection settings, set by parseGlobals.
var (
	globalHost      = "localhost"
	globalPort      = 6334
	globalOllamaURL = "http://localhost:11434"
	globalModel     = "all-minilm"
	globalRedisHost = "localhost"
	globalRedisPort = 6379
)

func init() {
	// Environment variables override defaults (before flags override both).
	if v := os.Getenv("CLAWBRAIN_HOST"); v != "" {
		globalHost = v
	}
	if v := os.Getenv("CLAWBRAIN_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &globalPort)
	}
	if v := os.Getenv("CLAWBRAIN_OLLAMA_URL"); v != "" {
		globalOllamaURL = v
	}
	if v := os.Getenv("CLAWBRAIN_MODEL"); v != "" {
		globalModel = v
	}
	if v := os.Getenv("CLAWBRAIN_REDIS_HOST"); v != "" {
		globalRedisHost = v
	}
	if v := os.Getenv("CLAWBRAIN_REDIS_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &globalRedisPort)
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
	case "check":
		runCheck()
	case "sync":
		runSync(args[1:])
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
		case "--redis-host":
			if i+1 < len(args) {
				globalRedisHost = args[i+1]
				i++
			}
		case "--redis-port":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &globalRedisPort)
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
	fmt.Fprintln(os.Stderr, "  --port         Qdrant gRPC port (default: 6334, env: CLAWBRAIN_PORT)")
	fmt.Fprintln(os.Stderr, "  --ollama-url   Ollama base URL (default: http://localhost:11434, env: CLAWBRAIN_OLLAMA_URL)")
	fmt.Fprintln(os.Stderr, "  --model        Embedding model (default: all-minilm, env: CLAWBRAIN_MODEL)")
	fmt.Fprintln(os.Stderr, "  --redis-host   Redis host (default: localhost, env: CLAWBRAIN_REDIS_HOST)")
	fmt.Fprintln(os.Stderr, "  --redis-port   Redis port (default: 6379, env: CLAWBRAIN_REDIS_PORT)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  add            Store a memory (--text 'your text here')")
	fmt.Fprintln(os.Stderr, "  get            Fetch a memory by ID (--id <uuid>)")
	fmt.Fprintln(os.Stderr, "  search         Search memories (--query 'search text')")
	fmt.Fprintln(os.Stderr, "  forget         Remove stale memories")
	fmt.Fprintln(os.Stderr, "  sync           Ingest markdown files into memory")
	fmt.Fprintln(os.Stderr, "  check          Verify Qdrant and Ollama connectivity")
}

func runGet(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	id := fs.String("id", "", "UUID of the memory to fetch (required)")
	fs.Parse(args)

	if *id == "" {
		fmt.Fprintln(os.Stderr, "Error: --id is required")
		fs.Usage()
		os.Exit(1)
	}

	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	result, err := s.Get(ctx, *id)
	if err != nil {
		exitJSON("error", err.Error())
	}

	if result == nil {
		exitJSON("error", fmt.Sprintf("memory %s not found", *id))
	}

	outputJSON(map[string]any{
		"status":  "ok",
		"id":      result.ID,
		"payload": result.Payload,
	})
}

// dedupThreshold is the minimum similarity score at which an existing memory
// is considered a duplicate of the incoming text. When a match is found at or
// above this threshold, the old memory is deleted and its created_at is
// preserved on the new one — effectively "merging" by letting the newer text
// replace the older version while keeping its origin timestamp.
const dedupThreshold float32 = 0.92

func runAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	text := fs.String("text", "", "Text to store as a memory (default mode)")
	payloadJSON := fs.String("payload", "", "Additional metadata as JSON object")
	vectorJSON := fs.String("vector", "", "Embedding vector as JSON array (advanced, overrides text mode)")
	id := fs.String("id", "", "UUID for the point (auto-generated if omitted)")
	pinned := fs.Bool("pinned", false, "Pin this memory to prevent automatic forgetting")
	noMerge := fs.Bool("no-merge", false, "Skip deduplication — store without checking for similar memories")
	fs.Parse(args)

	// Parse optional payload
	var payload map[string]any
	if *payloadJSON != "" {
		if err := json.Unmarshal([]byte(*payloadJSON), &payload); err != nil {
			exitJSON("error", fmt.Sprintf("invalid payload JSON: %v", err))
		}
	} else {
		payload = make(map[string]any)
	}

	if *pinned {
		payload["pinned"] = true
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

		// Require text field in payload — a memory without text is a ghost
		// that pollutes retrieval results with no displayable content.
		t, ok := payload["text"]
		if !ok || t == nil {
			exitJSON("error", "payload must contain a non-empty \"text\" field")
		}
		if s, isStr := t.(string); !isStr || s == "" {
			exitJSON("error", "payload must contain a non-empty \"text\" field")
		}

		// Dedup: search for similar memories and merge if found
		var merged []store.Result
		if !*noMerge {
			merged = dedupAndDelete(ctx, s, vector)
		}
		if len(merged) > 0 {
			if ca := oldestCreatedAt(merged); ca != "" {
				payload["created_at"] = ca
			}
		}

		pointID, err := s.Add(ctx, *id, vector, payload)
		if err != nil {
			exitJSON("error", err.Error())
		}

		result := map[string]any{
			"status": "ok",
			"id":     pointID,
		}
		if len(merged) > 0 {
			result["merged_ids"] = mergedIDs(merged)
			// Backward compat: merged_id is the first (most similar) duplicate
			result["merged_id"] = merged[0].ID
		}
		outputJSON(result)
	} else if *text != "" {
		// Default text mode: embed via Ollama, then store
		oc := ollama.New(globalOllamaURL)
		vector, err := oc.Embed(ctx, globalModel, *text)
		if err != nil {
			exitJSON("error", fmt.Sprintf("embedding failed: %v", err))
		}

		// Store the original text in payload so it can be returned on retrieval
		payload["text"] = *text

		// Dedup: search for similar memories and merge if found
		var merged []store.Result
		if !*noMerge {
			merged = dedupAndDelete(ctx, s, vector)
		}
		if len(merged) > 0 {
			if ca := oldestCreatedAt(merged); ca != "" {
				payload["created_at"] = ca
			}
		}

		pointID, err := s.Add(ctx, *id, vector, payload)
		if err != nil {
			exitJSON("error", err.Error())
		}

		result := map[string]any{
			"status": "ok",
			"id":     pointID,
		}
		if len(merged) > 0 {
			result["merged_ids"] = mergedIDs(merged)
			// Backward compat: merged_id is the first (most similar) duplicate
			result["merged_id"] = merged[0].ID
		}
		outputJSON(result)
	} else {
		fmt.Fprintln(os.Stderr, "Error: --text is required (or --vector for advanced mode)")
		fs.Usage()
		os.Exit(1)
	}
}

// dedupAndDelete looks for all existing memories above the dedup threshold.
// It deletes every duplicate found and returns the full list so the caller can
// preserve the oldest created_at. Returns nil when no duplicates are found.
func dedupAndDelete(ctx context.Context, s *store.Store, vector []float32) []store.Result {
	similar, err := s.FindSimilar(ctx, vector, dedupThreshold, 64)
	if err != nil {
		// Non-fatal: if dedup search fails, just proceed with a normal add.
		return nil
	}
	if len(similar) == 0 {
		return nil
	}

	// Delete all old duplicates, but never delete pinned memories.
	var deleted []store.Result
	for _, old := range similar {
		if pinned, ok := old.Payload["pinned"].(bool); ok && pinned {
			// Pinned memories are immune to automatic deletion, including dedup.
			continue
		}
		if err := s.Delete(ctx, old.ID); err != nil {
			// Non-fatal: skip this one, keep trying the rest.
			continue
		}
		deleted = append(deleted, old)
	}
	if len(deleted) == 0 {
		return nil
	}

	return deleted
}

// oldestCreatedAt returns the earliest created_at timestamp from a set of
// merged results. Returns "" if no valid created_at is found.
func oldestCreatedAt(results []store.Result) string {
	oldest := ""
	for _, r := range results {
		if ca, ok := r.Payload["created_at"].(string); ok {
			if oldest == "" || ca < oldest {
				oldest = ca
			}
		}
	}
	return oldest
}

// mergedIDs extracts the point IDs from a set of merged results.
func mergedIDs(results []store.Result) []string {
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	return ids
}

// multiFlag implements flag.Value to allow repeated flags (e.g. --file a --file b).
type multiFlag []string

func (f *multiFlag) String() string { return strings.Join(*f, ", ") }
func (f *multiFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func runSync(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	var files multiFlag
	var dirs multiFlag
	var excludes multiFlag
	fs.Var(&files, "file", "Path to a markdown file to ingest (repeatable)")
	fs.Var(&dirs, "dir", "Path to a directory of markdown files (repeatable)")
	fs.Var(&excludes, "exclude", "Glob pattern to exclude from sync (repeatable)")
	basePath := fs.String("base", ".", "Base path for default file discovery (env: CLAWBRAIN_WORKSPACE)")
	fs.Parse(args)

	// Environment variable override for base path
	if v := os.Getenv("CLAWBRAIN_WORKSPACE"); v != "" && *basePath == "." {
		*basePath = v
	}

	// Connect to services. Sync is a batch operation that may process many
	// files and chunks, so use a much longer timeout than the default 30s.
	s, err := store.New(globalHost, globalPort)
	if err != nil {
		exitJSON("error", err.Error())
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	oc := ollama.New(globalOllamaURL)

	rc, err := redis.New(globalRedisHost, globalRedisPort)
	if err != nil {
		exitJSON("error", fmt.Sprintf("redis: %v", err))
	}
	defer rc.Close()

	// Discover files
	discovered, err := sync.DiscoverFiles(*basePath, files, dirs)
	if err != nil {
		exitJSON("error", fmt.Sprintf("discover files: %v", err))
	}

	// Load ignore patterns: .clawbrain-ignore file + --exclude flags
	ignorePatterns := sync.LoadIgnorePatterns(*basePath)
	ignorePatterns = append(ignorePatterns, excludes...)

	if len(discovered) == 0 {
		outputJSON(map[string]any{
			"status":  "ok",
			"files":   0,
			"added":   0,
			"skipped": 0,
			"results": []any{},
		})
		return
	}

	totalAdded := 0
	totalSkipped := 0
	var results []sync.FileResult

	for _, filePath := range discovered {
		// Check ignore patterns
		if sync.IsIgnored(filePath, ignorePatterns) {
			fr := sync.FileResult{
				File:    filePath,
				Skipped: 1,
				Reason:  "excluded by ignore pattern",
			}
			results = append(results, fr)
			totalSkipped++
			continue
		}

		// Skip today's daily file — it's still being written
		if sync.IsTodayDailyFile(filePath) {
			fr := sync.FileResult{
				File:    filePath,
				Skipped: 1,
				Reason:  "today's daily file, still growing",
			}
			results = append(results, fr)
			totalSkipped++
			continue
		}

		redisKey := sync.RedisKey(filePath)
		isMemoryMD := sync.IsMemoryMD(filePath)

		// For non-MEMORY.md files, check Redis first (cheap) before reading
		// the file. These files are immutable — a simple existence check suffices.
		if !isMemoryMD {
			exists, err := rc.Exists(redisKey)
			if err != nil {
				exists = false
			}
			if exists {
				fr := sync.FileResult{
					File:    filePath,
					Skipped: 1,
					Reason:  "already synced",
				}
				results = append(results, fr)
				totalSkipped++
				continue
			}
		}

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			fr := sync.FileResult{
				File:   filePath,
				Reason: fmt.Sprintf("read error: %v", err),
			}
			results = append(results, fr)
			continue
		}

		text := string(content)
		if strings.TrimSpace(text) == "" {
			fr := sync.FileResult{
				File:    filePath,
				Skipped: 1,
				Reason:  "empty file",
			}
			results = append(results, fr)
			totalSkipped++
			continue
		}

		// For MEMORY.md: compare content hash — re-sync only if file changed.
		var contentHash string
		if isMemoryMD {
			contentHash = sync.ContentHash(content)
			storedHash, found, err := rc.Get(redisKey)
			if err == nil && found && storedHash == contentHash {
				fr := sync.FileResult{
					File:    filePath,
					Skipped: 1,
					Reason:  "already synced (unchanged)",
				}
				results = append(results, fr)
				totalSkipped++
				continue
			}
		}

		// Chunk the file
		chunks := sync.Chunk(text, sync.DefaultChunkSize, sync.DefaultChunkOverlap)
		added := 0

		for i, chunk := range chunks {
			normalized := sync.NormalizeText(chunk)
			if normalized == "" {
				continue
			}

			// Embed via Ollama
			vector, err := oc.Embed(ctx, globalModel, normalized)
			if err != nil {
				// Non-fatal per chunk: log and continue
				log.Printf("sync: embed failed for %s chunk %d: %v", filePath, i, err)
				continue
			}

			// Add to store with source metadata
			payload := map[string]any{
				"text":        normalized,
				"source":      filePath,
				"chunk_index": i,
			}

			// Run dedup before adding (same as regular add)
			merged := dedupAndDelete(ctx, s, vector)
			if len(merged) > 0 {
				if ca := oldestCreatedAt(merged); ca != "" {
					payload["created_at"] = ca
				}
			}

			_, err = s.Add(ctx, "", vector, payload)
			if err != nil {
				log.Printf("sync: store failed for %s chunk %d: %v", filePath, i, err)
				continue
			}
			added++
		}

		// Only mark file as processed in Redis if at least one chunk
		// was successfully stored. If all chunks failed (e.g. Ollama
		// was down), leave the file unmarked so it gets retried next run.
		if added > 0 {
			if isMemoryMD {
				// Store the content hash so we can detect changes next run.
				// Use a 7-day TTL as a safety net — even if the file hasn't
				// changed, it will be re-synced after a week. This catches
				// edge cases like hash collisions or corrupted state.
				rc.SetWithTTL(redisKey, contentHash, sync.MemoryMDTTLSeconds())
			} else {
				rc.Set(redisKey, "1")
			}
		}

		fr := sync.FileResult{
			File:  filePath,
			Added: added,
		}
		results = append(results, fr)
		totalAdded += added
	}

	outputJSON(map[string]any{
		"status":  "ok",
		"files":   len(discovered),
		"added":   totalAdded,
		"skipped": totalSkipped,
		"results": results,
	})
}

func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	query := fs.String("query", "", "Text to search for (default mode)")
	vectorJSON := fs.String("vector", "", "Query embedding as JSON array (advanced, overrides text mode)")
	minScore := fs.Float64("min-score", 0.0, "Minimum similarity score threshold")
	limit := fs.Uint64("limit", 1, "Maximum number of results")
	fs.Parse(args)

	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	if *vectorJSON != "" {
		// Advanced vector mode
		var vector []float32
		if err := json.Unmarshal([]byte(*vectorJSON), &vector); err != nil {
			exitJSON("error", fmt.Sprintf("invalid vector JSON: %v", err))
		}

		results, err := s.Retrieve(ctx, vector, float32(*minScore), *limit)
		if err != nil {
			exitJSON("error", err.Error())
		}

		outputJSON(map[string]any{
			"status":     "ok",
			"results":    results,
			"returned":   len(results),
			"confidence": confidence(results),
		})
	} else if *query != "" {
		// Default text mode: embed query via Ollama, then search
		oc := ollama.New(globalOllamaURL)
		vector, err := oc.Embed(ctx, globalModel, *query)
		if err != nil {
			exitJSON("error", fmt.Sprintf("embedding failed: %v", err))
		}

		results, err := s.Retrieve(ctx, vector, float32(*minScore), *limit)
		if err != nil {
			exitJSON("error", err.Error())
		}

		outputJSON(map[string]any{
			"status":     "ok",
			"results":    results,
			"returned":   len(results),
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
	ttlStr := fs.String("ttl", "720h", "Duration — memories not accessed within this window are deleted")
	fs.Parse(args)

	ttl, err := time.ParseDuration(*ttlStr)
	if err != nil {
		exitJSON("error", fmt.Sprintf("invalid TTL: %v", err))
	}

	s, ctx, cancel := connect()
	defer cancel()
	defer s.Close()

	deleted, err := s.Forget(ctx, ttl)
	if err != nil {
		exitJSON("error", err.Error())
	}

	outputJSON(map[string]any{
		"status":  "ok",
		"deleted": deleted,
		"ttl":     ttlStr,
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
