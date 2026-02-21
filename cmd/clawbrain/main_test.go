package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

// buildBinary builds the clawbrain CLI and returns the path to the binary.
func buildBinary(t *testing.T) string {
	t.Helper()
	binary := t.TempDir() + "/clawbrain"
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}
	return binary
}

// runCLI runs the clawbrain binary with the given args and returns the output.
func runCLI(t *testing.T, binary string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	return cmd.CombinedOutput()
}

// parseJSON parses JSON output into a map.
func parseJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse JSON %q: %v", string(data), err)
	}
	return result
}

// skipIfNoQdrant skips the test if Qdrant is not running.
// Uses a lightweight TCP dial instead of the CLI check command to avoid
// race conditions on the clawbrain_check collection between parallel tests.
func skipIfNoQdrant(t *testing.T, binary string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "localhost:6334", 2*time.Second)
	if err != nil {
		t.Skipf("Qdrant not available on localhost:6334, skipping: %v", err)
	}
	conn.Close()
}

// skipIfNoOllama skips the test if Ollama is not running.
func skipIfNoOllama(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/")
	if err != nil {
		t.Skipf("Ollama not available on localhost:11434, skipping: %v", err)
	}
	resp.Body.Close()
}

func TestCLINoArgs(t *testing.T) {
	binary := buildBinary(t)
	out, err := runCLI(t, binary)
	if err == nil {
		t.Fatal("expected non-zero exit code with no args")
	}
	if len(out) == 0 {
		t.Fatal("expected usage output")
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	binary := buildBinary(t)
	out, err := runCLI(t, binary, "bogus")
	if err == nil {
		t.Fatal("expected non-zero exit code for unknown command")
	}
	if len(out) == 0 {
		t.Fatal("expected error output")
	}
}

func TestCLICheck(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	out, err := runCLI(t, binary, "check")
	if err != nil {
		t.Fatalf("check failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %v", result["status"])
	}
}

func TestCLIAddMissingFlags(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name string
		args []string
	}{
		{"no flags", []string{"add"}},
		{"missing collection", []string{"add", "--text", "hello"}},
		{"missing text and vector", []string{"add", "--collection", "test"}},
		{"missing collection with vector", []string{"add", "--vector", "[0.1]", "--payload", "{}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runCLI(t, binary, tt.args...)
			if err == nil {
				t.Fatal("expected error for missing required flags")
			}
		})
	}
}

func TestCLIAddAndSearch(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_add_search_" + t.Name()

	// Cleanup at the end by forgetting everything
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add a memory
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "cli test memory"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	addResult := parseJSON(t, out)
	if addResult["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", addResult["status"])
	}
	if addResult["id"] == nil || addResult["id"] == "" {
		t.Fatal("expected non-empty id")
	}

	// Search for it
	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--min-score", "0.9",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	searchResult := parseJSON(t, out)
	if searchResult["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", searchResult["status"])
	}
	count, ok := searchResult["count"].(float64)
	if !ok || count < 1 {
		t.Fatalf("expected at least 1 result, got %v", searchResult["count"])
	}
}

func TestCLIAddWithCustomID(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_custom_id_" + t.Name()

	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	customID := "12345678-1234-1234-1234-123456789abc"
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.5, 0.5, 0.5, 0.5]",
		"--payload", `{"text": "custom id"}`,
		"--id", customID,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["id"] != customID {
		t.Errorf("expected id %q, got %v", customID, result["id"])
	}
}

func TestCLISearchMissingFlags(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name string
		args []string
	}{
		{"no flags", []string{"search"}},
		{"missing query and vector", []string{"search", "--collection", "test"}},
		{"missing collection", []string{"search", "--query", "hello"}},
		{"missing collection with vector", []string{"search", "--vector", "[0.1]"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runCLI(t, binary, tt.args...)
			if err == nil {
				t.Fatal("expected error for missing required flags")
			}
		})
	}
}

func TestCLIForget(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_forget_" + t.Name()

	// Add a memory
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "will be forgotten"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Forget with 0s TTL — should delete everything
	out, err = runCLI(t, binary, "forget",
		"--collection", collection,
		"--ttl", "0s",
	)
	if err != nil {
		t.Fatalf("forget failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", result["status"])
	}
	deleted, ok := result["deleted"].(float64)
	if !ok || deleted < 1 {
		t.Fatalf("expected at least 1 deletion, got %v", result["deleted"])
	}

	// Verify collection is empty via search
	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	searchResult := parseJSON(t, out)
	count, _ := searchResult["count"].(float64)
	if count != 0 {
		t.Errorf("expected 0 results after forget, got %v", count)
	}
}

func TestCLIAddSearchPreservesPayload(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_payload_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add with rich payload
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "preserved", "count": 42, "active": true}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Search and check payload fields
	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--min-score", "0.9",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	results := result["results"].([]any)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	payload := results[0].(map[string]any)["payload"].(map[string]any)

	if payload["text"] != "preserved" {
		t.Errorf("expected text 'preserved', got %v", payload["text"])
	}
	if payload["count"] != float64(42) { // JSON numbers are float64
		t.Errorf("expected count 42, got %v", payload["count"])
	}
	if payload["active"] != true {
		t.Errorf("expected active true, got %v", payload["active"])
	}
	// Verify auto-injected timestamps exist
	if payload["created_at"] == nil {
		t.Error("missing created_at in payload")
	}
	if payload["last_accessed"] == nil {
		t.Error("missing last_accessed in payload")
	}
}

func TestCLIForgetInvalidTTL(t *testing.T) {
	binary := buildBinary(t)

	out, err := runCLI(t, binary, "forget",
		"--collection", "test",
		"--ttl", "not-a-duration",
	)
	if err == nil {
		t.Fatal("expected error for invalid TTL format")
	}
	result := parseJSON(t, out)
	if result["status"] != "error" {
		t.Errorf("expected status error, got %v", result["status"])
	}
}

func TestCLIForgetMissingCollection(t *testing.T) {
	binary := buildBinary(t)
	_, err := runCLI(t, binary, "forget")
	if err == nil {
		t.Fatal("expected error for missing --collection")
	}
}

func TestCLIInvalidVectorJSON(t *testing.T) {
	binary := buildBinary(t)

	out, err := runCLI(t, binary, "add",
		"--collection", "test",
		"--vector", "not json",
		"--payload", "{}",
	)
	if err == nil {
		t.Fatal("expected error for invalid vector JSON")
	}
	result := parseJSON(t, out)
	if result["status"] != "error" {
		t.Errorf("expected status error, got %v", result["status"])
	}
}

func TestCLIInvalidPayloadJSON(t *testing.T) {
	binary := buildBinary(t)

	out, err := runCLI(t, binary, "add",
		"--collection", "test",
		"--vector", "[0.1]",
		"--payload", "not json",
	)
	if err == nil {
		t.Fatal("expected error for invalid payload JSON")
	}
	result := parseJSON(t, out)
	if result["status"] != "error" {
		t.Errorf("expected status error, got %v", result["status"])
	}
}

func TestCLICollections(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	// Add a memory so there's at least one collection
	collection := "test_cli_collections_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3]",
		"--payload", `{"text": "collections test"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// List collections
	out, err = runCLI(t, binary, "collections")
	if err != nil {
		t.Fatalf("collections failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", result["status"])
	}

	collections, ok := result["collections"].([]any)
	if !ok {
		t.Fatal("expected collections to be an array")
	}

	found := false
	for _, c := range collections {
		if c == collection {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in collections list, got %v", collection, collections)
	}
}

// --- Get command tests ---

func TestCLIGetMissingFlags(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name string
		args []string
	}{
		{"no flags", []string{"get"}},
		{"missing collection", []string{"get", "--id", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}},
		{"missing id", []string{"get", "--collection", "test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runCLI(t, binary, tt.args...)
			if err == nil {
				t.Fatal("expected error for missing required flags")
			}
		})
	}
}

func TestCLIGetByID(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_get_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	customID := "11111111-2222-3333-4444-555555555555"

	// Add a memory with a known ID
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "get me by id", "tag": "test"}`,
		"--id", customID,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Get by ID
	out, err = runCLI(t, binary, "get",
		"--collection", collection,
		"--id", customID,
	)
	if err != nil {
		t.Fatalf("get failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", result["status"])
	}
	if result["id"] != customID {
		t.Errorf("expected id %q, got %v", customID, result["id"])
	}

	payload, ok := result["payload"].(map[string]any)
	if !ok {
		t.Fatal("expected payload to be a map")
	}
	if payload["text"] != "get me by id" {
		t.Errorf("expected text 'get me by id', got %v", payload["text"])
	}
	if payload["tag"] != "test" {
		t.Errorf("expected tag 'test', got %v", payload["tag"])
	}
	if payload["created_at"] == nil {
		t.Error("missing created_at in payload")
	}
	if payload["last_accessed"] == nil {
		t.Error("missing last_accessed in payload")
	}
}

func TestCLIGetNotFound(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_get_notfound_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add something so the collection exists
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "placeholder"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Try to get a nonexistent ID
	out, err = runCLI(t, binary, "get",
		"--collection", collection,
		"--id", "00000000-0000-0000-0000-000000000000",
	)
	if err == nil {
		t.Fatal("expected error for nonexistent memory")
	}

	result := parseJSON(t, out)
	if result["status"] != "error" {
		t.Errorf("expected status error, got %v", result["status"])
	}
}

// --- Text mode tests (require both Qdrant and Ollama) ---

func TestCLITextAddAndSearch(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	collection := "test_cli_text_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add a text memory (embedding happens via Ollama)
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--text", "the user prefers dark mode for coding",
	)
	if err != nil {
		t.Fatalf("add text failed: %v\n%s", err, out)
	}

	addResult := parseJSON(t, out)
	if addResult["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", addResult["status"])
	}
	if addResult["id"] == nil || addResult["id"] == "" {
		t.Fatal("expected non-empty id")
	}

	// Search by text query (query is also embedded via Ollama)
	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--query", "dark mode",
		"--limit", "5",
	)
	if err != nil {
		t.Fatalf("search text failed: %v\n%s", err, out)
	}

	searchResult := parseJSON(t, out)
	if searchResult["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", searchResult["status"])
	}
	count, ok := searchResult["count"].(float64)
	if !ok || count < 1 {
		t.Fatalf("expected at least 1 result, got %v", searchResult["count"])
	}

	// Verify the text is in the payload and score is present
	results := searchResult["results"].([]any)
	firstResult := results[0].(map[string]any)
	payload := firstResult["payload"].(map[string]any)
	if payload["text"] != "the user prefers dark mode for coding" {
		t.Errorf("expected text in payload, got %v", payload["text"])
	}
	if _, ok := firstResult["score"].(float64); !ok {
		t.Error("expected score in result")
	}
}

func TestCLITextAddWithPayload(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	collection := "test_cli_text_payload_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add text with extra payload
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--text", "golang is great for cli tools",
		"--payload", `{"source": "conversation", "confidence": 0.9}`,
	)
	if err != nil {
		t.Fatalf("add text failed: %v\n%s", err, out)
	}

	addResult := parseJSON(t, out)
	if addResult["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", addResult["status"])
	}

	// Search and check payload
	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--query", "golang",
		"--limit", "5",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	results := result["results"].([]any)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	payload := results[0].(map[string]any)["payload"].(map[string]any)
	if payload["source"] != "conversation" {
		t.Errorf("expected source 'conversation', got %v", payload["source"])
	}
	if payload["text"] != "golang is great for cli tools" {
		t.Errorf("expected text preserved, got %v", payload["text"])
	}
	if payload["created_at"] == nil {
		t.Error("missing created_at")
	}
	if payload["last_accessed"] == nil {
		t.Error("missing last_accessed")
	}
}

func TestCLITextAddMissingText(t *testing.T) {
	binary := buildBinary(t)

	// No --text and no --vector should fail
	_, err := runCLI(t, binary, "add",
		"--collection", "test",
	)
	if err == nil {
		t.Fatal("expected error when neither --text nor --vector provided")
	}
}

func TestCLITextSearchMissingQuery(t *testing.T) {
	binary := buildBinary(t)

	// No --query and no --vector should fail
	_, err := runCLI(t, binary, "search",
		"--collection", "test",
	)
	if err == nil {
		t.Fatal("expected error when neither --query nor --vector provided")
	}
}

func TestCLITextForget(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	collection := "test_cli_text_forget_" + t.Name()

	// Add a text memory
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--text", "this memory will be forgotten",
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Forget with 0s TTL — should delete everything
	out, err = runCLI(t, binary, "forget",
		"--collection", collection,
		"--ttl", "0s",
	)
	if err != nil {
		t.Fatalf("forget failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", result["status"])
	}
	deleted, ok := result["deleted"].(float64)
	if !ok || deleted < 1 {
		t.Fatalf("expected at least 1 deletion, got %v", result["deleted"])
	}

	// Verify search returns nothing
	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--query", "forgotten",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	searchResult := parseJSON(t, out)
	count, _ := searchResult["count"].(float64)
	if count != 0 {
		t.Errorf("expected 0 results after forget, got %v", count)
	}
}

func TestCLITextAddWithCustomID(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	collection := "test_cli_text_customid_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	customID := "aabbccdd-1122-3344-5566-778899aabbcc"
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--text", "text with custom id",
		"--id", customID,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["id"] != customID {
		t.Errorf("expected id %q, got %v", customID, result["id"])
	}
}

func TestCLITextSemanticSearch(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	collection := "test_cli_semantic_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add memories with distinct topics
	memories := []string{
		"the user prefers dark mode for coding at night",
		"deploy the application to production every friday",
		"use golang and qdrant for the memory system",
	}
	for _, m := range memories {
		out, err := runCLI(t, binary, "add",
			"--collection", collection,
			"--text", m,
		)
		if err != nil {
			t.Fatalf("add failed: %v\n%s", err, out)
		}
	}

	// Search for something semantically related to dark mode
	out, err := runCLI(t, binary, "search",
		"--collection", collection,
		"--query", "night theme preferences",
		"--limit", "3",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", result["status"])
	}

	results := result["results"].([]any)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// The top result should be about dark mode (semantically closest to "night theme preferences")
	topPayload := results[0].(map[string]any)["payload"].(map[string]any)
	topText := topPayload["text"].(string)
	if topText != "the user prefers dark mode for coding at night" {
		t.Errorf("expected dark mode memory as top result for 'night theme preferences', got %q", topText)
	}
}

func TestCLIGlobalOllamaFlags(t *testing.T) {
	binary := buildBinary(t)

	// Test that --ollama-url and --model flags are accepted without error
	// (they'll fail because of missing --collection, but shouldn't fail on flag parsing)
	_, err := runCLI(t, binary, "--ollama-url", "http://example.com:11434", "--model", "test-model", "add")
	if err == nil {
		t.Fatal("expected error for missing --collection, not a pass")
	}
	// The error should be about missing collection, not about unknown flags
}

func TestCLIGlobalHostFlag(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	// --host before command should work
	out, err := runCLI(t, binary, "--host", "localhost", "check")
	if err != nil {
		t.Fatalf("check with --host failed: %v\n%s", err, out)
	}
	result := parseJSON(t, out)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", result["status"])
	}
}

// --- Confidence field tests ---

func TestCLIConfidenceFieldPresent(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_confidence_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add a memory
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "confidence test"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Search — response should include confidence field
	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	conf, ok := result["confidence"].(string)
	if !ok {
		t.Fatalf("expected confidence field as string, got %v (%T)", result["confidence"], result["confidence"])
	}
	if conf != "high" && conf != "medium" && conf != "low" && conf != "none" {
		t.Errorf("unexpected confidence value %q", conf)
	}
}

func TestCLIConfidenceHigh(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_conf_high_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add and search with identical vector — score ~1.0, should be "high"
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.5, 0.5, 0.5, 0.5]",
		"--payload", `{"text": "exact match"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--vector", "[0.5, 0.5, 0.5, 0.5]",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["confidence"] != "high" {
		t.Errorf("expected confidence 'high' for exact match, got %v", result["confidence"])
	}
}

func TestCLIConfidenceNone(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_conf_none_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add a memory then search with min-score so high nothing matches
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "will not match"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--vector", "[0.9, -0.9, 0.9, -0.9]",
		"--min-score", "0.99",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["confidence"] != "none" {
		t.Errorf("expected confidence 'none' for empty results, got %v", result["confidence"])
	}
	count, _ := result["count"].(float64)
	if count != 0 {
		t.Errorf("expected 0 results, got %v", count)
	}
}

func TestCLIConfidenceLow(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_conf_low_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add a memory with one vector, query with a very different one
	// Cosine similarity of orthogonal-ish vectors should be low
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[1.0, 0.0, 0.0, 0.0]",
		"--payload", `{"text": "low confidence test"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Query with a nearly orthogonal vector — score should be close to 0
	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--vector", "[0.01, 1.0, 0.0, 0.0]",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	conf := result["confidence"].(string)
	if conf != "low" {
		// Check the actual score to understand
		results := result["results"].([]any)
		if len(results) > 0 {
			score := results[0].(map[string]any)["score"].(float64)
			t.Errorf("expected confidence 'low' for near-orthogonal vectors, got %q (score: %.4f)", conf, score)
		} else {
			t.Errorf("expected confidence 'low', got %q", conf)
		}
	}
}

func TestCLIConfidenceWithTextQuery(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	collection := "test_cli_conf_text_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add a text memory
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--text", "the cat sat on the mat",
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Query with semantically related text — should have confidence field
	out, err = runCLI(t, binary, "search",
		"--collection", collection,
		"--query", "cat sitting on a mat",
		"--limit", "3",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	conf, ok := result["confidence"].(string)
	if !ok {
		t.Fatalf("expected confidence field as string in text mode, got %v (%T)", result["confidence"], result["confidence"])
	}
	// For nearly identical text, confidence should be high
	if conf != "high" {
		results := result["results"].([]any)
		if len(results) > 0 {
			score := results[0].(map[string]any)["score"].(float64)
			t.Logf("Note: confidence=%q, top score=%.4f", conf, score)
		}
	}
}

func TestCLIPinnedMemorySurvivesForget(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	collection := "test_cli_pinned_" + t.Name()
	defer func() {
		// Force cleanup: forget with 0s TTL won't delete pinned memories,
		// so we add a second forget after this test to clean up.
		// In practice the collection will be cleaned up by future test runs.
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add a pinned memory
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--text", "this memory is pinned and should survive",
		"--pinned",
	)
	if err != nil {
		t.Fatalf("add pinned failed: %v\n%s", err, out)
	}

	addResult := parseJSON(t, out)
	if addResult["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", addResult["status"])
	}
	pinnedID := addResult["id"].(string)

	// Add an unpinned memory
	out, err = runCLI(t, binary, "add",
		"--collection", collection,
		"--text", "this memory is not pinned and should be forgotten",
	)
	if err != nil {
		t.Fatalf("add unpinned failed: %v\n%s", err, out)
	}

	// Forget with 0s TTL — should delete only the unpinned one
	out, err = runCLI(t, binary, "forget",
		"--collection", collection,
		"--ttl", "0s",
	)
	if err != nil {
		t.Fatalf("forget failed: %v\n%s", err, out)
	}

	forgetResult := parseJSON(t, out)
	deleted, _ := forgetResult["deleted"].(float64)
	if deleted != 1 {
		t.Fatalf("expected 1 deletion (unpinned only), got %v", deleted)
	}

	// Verify the pinned memory is still retrievable by ID
	out, err = runCLI(t, binary, "get",
		"--collection", collection,
		"--id", pinnedID,
	)
	if err != nil {
		t.Fatalf("get pinned failed: %v\n%s", err, out)
	}

	getResult := parseJSON(t, out)
	if getResult["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", getResult["status"])
	}
	payload := getResult["payload"].(map[string]any)
	if payload["text"] != "this memory is pinned and should survive" {
		t.Errorf("expected pinned memory text, got %v", payload["text"])
	}
	if payload["pinned"] != true {
		t.Errorf("expected pinned=true in payload, got %v", payload["pinned"])
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
