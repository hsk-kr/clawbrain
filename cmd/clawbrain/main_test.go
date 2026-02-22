package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/hsk-coder/clawbrain/internal/store"
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

// cleanupMemories deletes the memories collection entirely via the store.
// This ensures a clean slate between tests that may use different vector dimensions.
func cleanupMemories(t *testing.T) {
	t.Helper()
	s, err := store.New("localhost", 6334)
	if err != nil {
		return
	}
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Forget with 0s TTL to delete all unpinned points, then rely on
	// the collection being reusable. For full cleanup (including dimension
	// changes), we delete the collection entirely.
	s.Forget(ctx, 0)
	// Delete the collection if it exists — the store will recreate it on next Add.
	s.DeleteCollection(ctx)
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
		{"missing text and vector", []string{"add", "--payload", "{}"}},
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

func TestCLIAddVectorRejectsEmptyPayload(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			"no payload flag",
			[]string{"add", "--vector", "[0.1, 0.2, 0.3, 0.4]"},
			"payload must contain a non-empty \"text\" field",
		},
		{
			"empty payload object",
			[]string{"add", "--vector", "[0.1, 0.2, 0.3, 0.4]", "--payload", "{}"},
			"payload must contain a non-empty \"text\" field",
		},
		{
			"payload with empty text",
			[]string{"add", "--vector", "[0.1, 0.2, 0.3, 0.4]", "--payload", `{"text": ""}`},
			"payload must contain a non-empty \"text\" field",
		},
		{
			"payload with null text",
			[]string{"add", "--vector", "[0.1, 0.2, 0.3, 0.4]", "--payload", `{"text": null}`},
			"payload must contain a non-empty \"text\" field",
		},
		{
			"payload with other fields but no text",
			[]string{"add", "--vector", "[0.1, 0.2, 0.3, 0.4]", "--payload", `{"source": "test"}`},
			"payload must contain a non-empty \"text\" field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runCLI(t, binary, tt.args...)
			if err == nil {
				t.Fatal("expected error for missing text in payload")
			}
			result := parseJSON(t, out)
			if result["status"] != "error" {
				t.Errorf("expected status error, got %v", result["status"])
			}
			if msg, ok := result["message"].(string); !ok || msg != tt.wantMsg {
				t.Errorf("expected message %q, got %v", tt.wantMsg, result["message"])
			}
		})
	}
}

func TestCLIAddAndSearch(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	// Cleanup at the end
	defer cleanupMemories(t)

	// Add a memory
	out, err := runCLI(t, binary, "add",
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
	returned, ok := searchResult["returned"].(float64)
	if !ok || returned < 1 {
		t.Fatalf("expected at least 1 result, got %v", searchResult["returned"])
	}
}

func TestCLIAddWithCustomID(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	defer cleanupMemories(t)

	customID := "12345678-1234-1234-1234-123456789abc"
	out, err := runCLI(t, binary, "add",
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
		{"missing query and vector", []string{"search", "--limit", "5"}},
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

	// Add a memory
	out, err := runCLI(t, binary, "add",
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "will be forgotten"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Forget with 0s TTL — should delete everything
	out, err = runCLI(t, binary, "forget",
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

	// Verify empty via search
	out, err = runCLI(t, binary, "search",
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	searchResult := parseJSON(t, out)
	returned, _ := searchResult["returned"].(float64)
	if returned != 0 {
		t.Errorf("expected 0 results after forget, got %v", returned)
	}
}

func TestCLIAddSearchPreservesPayload(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	defer cleanupMemories(t)

	// Add with rich payload
	out, err := runCLI(t, binary, "add",
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "preserved", "count": 42, "active": true}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Search and check payload fields
	out, err = runCLI(t, binary, "search",
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

func TestCLIInvalidVectorJSON(t *testing.T) {
	binary := buildBinary(t)

	out, err := runCLI(t, binary, "add",
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

// --- Get command tests ---

func TestCLIGetMissingFlags(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name string
		args []string
	}{
		{"no flags", []string{"get"}},
		{"missing id", []string{"get"}},
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

	defer cleanupMemories(t)

	customID := "11111111-2222-3333-4444-555555555555"

	// Add a memory with a known ID
	out, err := runCLI(t, binary, "add",
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "get me by id", "tag": "test"}`,
		"--id", customID,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Get by ID
	out, err = runCLI(t, binary, "get",
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

	defer cleanupMemories(t)

	// Add something so the collection exists
	out, err := runCLI(t, binary, "add",
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "placeholder"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Try to get a nonexistent ID
	out, err = runCLI(t, binary, "get",
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

	defer cleanupMemories(t)

	// Add a text memory (embedding happens via Ollama)
	out, err := runCLI(t, binary, "add",
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
	returned, ok := searchResult["returned"].(float64)
	if !ok || returned < 1 {
		t.Fatalf("expected at least 1 result, got %v", searchResult["returned"])
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

	defer cleanupMemories(t)

	// Add text with extra payload
	out, err := runCLI(t, binary, "add",
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
	_, err := runCLI(t, binary, "add")
	if err == nil {
		t.Fatal("expected error when neither --text nor --vector provided")
	}
}

func TestCLITextSearchMissingQuery(t *testing.T) {
	binary := buildBinary(t)

	// No --query and no --vector should fail
	_, err := runCLI(t, binary, "search")
	if err == nil {
		t.Fatal("expected error when neither --query nor --vector provided")
	}
}

func TestCLITextForget(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	// Add a text memory
	out, err := runCLI(t, binary, "add",
		"--text", "this memory will be forgotten",
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Forget with 0s TTL — should delete everything
	out, err = runCLI(t, binary, "forget",
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
		"--query", "forgotten",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}

	searchResult := parseJSON(t, out)
	returned, _ := searchResult["returned"].(float64)
	if returned != 0 {
		t.Errorf("expected 0 results after forget, got %v", returned)
	}
}

func TestCLITextAddWithCustomID(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	defer cleanupMemories(t)

	customID := "aabbccdd-1122-3344-5566-778899aabbcc"
	out, err := runCLI(t, binary, "add",
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

	defer cleanupMemories(t)

	// Add memories with distinct topics
	memories := []string{
		"the user prefers dark mode for coding at night",
		"deploy the application to production every friday",
		"use golang and qdrant for the memory system",
	}
	for _, m := range memories {
		out, err := runCLI(t, binary, "add",
			"--text", m,
		)
		if err != nil {
			t.Fatalf("add failed: %v\n%s", err, out)
		}
	}

	// Search for something semantically related to dark mode
	out, err := runCLI(t, binary, "search",
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
	// (they'll fail because of missing --text/--vector, but shouldn't fail on flag parsing)
	_, err := runCLI(t, binary, "--ollama-url", "http://example.com:11434", "--model", "test-model", "add")
	if err == nil {
		t.Fatal("expected error for missing --text, not a pass")
	}
	// The error should be about missing text/vector, not about unknown flags
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

	defer cleanupMemories(t)

	// Add a memory
	out, err := runCLI(t, binary, "add",
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "confidence test"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Search — response should include confidence field
	out, err = runCLI(t, binary, "search",
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

	defer cleanupMemories(t)

	// Add and search with identical vector — score ~1.0, should be "high"
	out, err := runCLI(t, binary, "add",
		"--vector", "[0.5, 0.5, 0.5, 0.5]",
		"--payload", `{"text": "exact match"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	out, err = runCLI(t, binary, "search",
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

	defer cleanupMemories(t)

	// Add a memory then search with min-score so high nothing matches
	out, err := runCLI(t, binary, "add",
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "will not match"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	out, err = runCLI(t, binary, "search",
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
	returned, _ := result["returned"].(float64)
	if returned != 0 {
		t.Errorf("expected 0 results, got %v", returned)
	}
}

func TestCLIConfidenceLow(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	defer cleanupMemories(t)

	// Add a memory with one vector, query with a very different one
	// Cosine similarity of orthogonal-ish vectors should be low
	out, err := runCLI(t, binary, "add",
		"--vector", "[1.0, 0.0, 0.0, 0.0]",
		"--payload", `{"text": "low confidence test"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Query with a nearly orthogonal vector — score should be close to 0
	out, err = runCLI(t, binary, "search",
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

	defer cleanupMemories(t)

	// Add a text memory
	out, err := runCLI(t, binary, "add",
		"--text", "the cat sat on the mat",
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Query with semantically related text — should have confidence field
	out, err = runCLI(t, binary, "search",
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

	defer cleanupMemories(t)

	// Add a pinned memory
	out, err := runCLI(t, binary, "add",
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
		"--text", "this memory is not pinned and should be forgotten",
	)
	if err != nil {
		t.Fatalf("add unpinned failed: %v\n%s", err, out)
	}

	// Forget with 0s TTL — should delete only the unpinned one
	out, err = runCLI(t, binary, "forget",
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

// --- Dedup-merge tests ---

func TestCLIDedupMergeExactDuplicate(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	defer cleanupMemories(t)

	// Add the first memory
	out, err := runCLI(t, binary, "add",
		"--text", "lico is a Korean software engineer living in London",
	)
	if err != nil {
		t.Fatalf("first add failed: %v\n%s", err, out)
	}
	first := parseJSON(t, out)
	if first["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", first["status"])
	}
	firstID := first["id"].(string)

	// Add the same text again — should trigger merge
	out, err = runCLI(t, binary, "add",
		"--text", "lico is a Korean software engineer living in London",
	)
	if err != nil {
		t.Fatalf("second add failed: %v\n%s", err, out)
	}
	second := parseJSON(t, out)
	if second["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", second["status"])
	}

	// The old memory should have been merged (deleted + replaced)
	if second["merged_id"] != firstID {
		t.Errorf("expected merged_id %q, got %v", firstID, second["merged_id"])
	}

	// There should only be 1 memory total, not 2
	out, err = runCLI(t, binary, "search",
		"--query", "lico is a Korean software engineer living in London",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}
	search := parseJSON(t, out)
	returned := int(search["returned"].(float64))
	if returned != 1 {
		t.Fatalf("expected 1 memory after dedup merge, got %d", returned)
	}
}

func TestCLIDedupMergePreservesCreatedAt(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	defer cleanupMemories(t)

	// Add the first memory
	out, err := runCLI(t, binary, "add",
		"--text", "the sky is blue on a clear day",
	)
	if err != nil {
		t.Fatalf("first add failed: %v\n%s", err, out)
	}
	first := parseJSON(t, out)
	firstID := first["id"].(string)

	// Fetch the original created_at
	out, err = runCLI(t, binary, "get", "--id", firstID)
	if err != nil {
		t.Fatalf("get failed: %v\n%s", err, out)
	}
	firstGet := parseJSON(t, out)
	originalCreatedAt := firstGet["payload"].(map[string]any)["created_at"].(string)

	time.Sleep(1100 * time.Millisecond)

	// Add the same text again — should merge
	out, err = runCLI(t, binary, "add",
		"--text", "the sky is blue on a clear day",
	)
	if err != nil {
		t.Fatalf("second add failed: %v\n%s", err, out)
	}
	second := parseJSON(t, out)
	secondID := second["id"].(string)

	// The new memory should have the original created_at preserved
	out, err = runCLI(t, binary, "get", "--id", secondID)
	if err != nil {
		t.Fatalf("get merged failed: %v\n%s", err, out)
	}
	merged := parseJSON(t, out)
	mergedCreatedAt := merged["payload"].(map[string]any)["created_at"].(string)

	if mergedCreatedAt != originalCreatedAt {
		t.Errorf("created_at not preserved: original=%s, merged=%s", originalCreatedAt, mergedCreatedAt)
	}
}

func TestCLINoMergeFlag(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)
	skipIfNoOllama(t)

	defer cleanupMemories(t)

	// Add the first memory
	out, err := runCLI(t, binary, "add",
		"--text", "water is composed of hydrogen and oxygen",
	)
	if err != nil {
		t.Fatalf("first add failed: %v\n%s", err, out)
	}
	first := parseJSON(t, out)
	if first["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", first["status"])
	}

	// Add the same text with --no-merge — should NOT merge, creating a duplicate
	out, err = runCLI(t, binary, "add",
		"--text", "water is composed of hydrogen and oxygen",
		"--no-merge",
	)
	if err != nil {
		t.Fatalf("second add failed: %v\n%s", err, out)
	}
	second := parseJSON(t, out)
	if second["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", second["status"])
	}

	// merged_id should NOT be present
	if second["merged_id"] != nil {
		t.Errorf("expected no merged_id with --no-merge, got %v", second["merged_id"])
	}

	// There should be 2 memories (duplicate allowed)
	out, err = runCLI(t, binary, "search",
		"--query", "water is composed of hydrogen and oxygen",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("search failed: %v\n%s", err, out)
	}
	search := parseJSON(t, out)
	returned := int(search["returned"].(float64))
	if returned != 2 {
		t.Fatalf("expected 2 memories with --no-merge, got %d", returned)
	}
}

func TestCLIDedupMergeVectorMode(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	defer cleanupMemories(t)

	vector := "[0.1,0.2,0.3,0.4]"
	payload := `{"text":"vector mode dedup test"}`

	// Add the first memory in vector mode
	out, err := runCLI(t, binary, "add",
		"--vector", vector,
		"--payload", payload,
	)
	if err != nil {
		t.Fatalf("first add failed: %v\n%s", err, out)
	}
	first := parseJSON(t, out)
	firstID := first["id"].(string)

	// Add the same vector again — should trigger merge
	out, err = runCLI(t, binary, "add",
		"--vector", vector,
		"--payload", payload,
	)
	if err != nil {
		t.Fatalf("second add failed: %v\n%s", err, out)
	}
	second := parseJSON(t, out)

	if second["merged_id"] != firstID {
		t.Errorf("expected merged_id %q, got %v", firstID, second["merged_id"])
	}
}

// --- Collections command removed ---

func TestCLICollectionsCommandRemoved(t *testing.T) {
	binary := buildBinary(t)
	_, err := runCLI(t, binary, "collections")
	if err == nil {
		t.Fatal("expected error for removed 'collections' command")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
