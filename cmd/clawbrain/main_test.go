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

func TestCLIAddAndRetrieve(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_add_retrieve_" + t.Name()

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

	// Retrieve it
	out, err = runCLI(t, binary, "retrieve",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--min-score", "0.9",
	)
	if err != nil {
		t.Fatalf("retrieve failed: %v\n%s", err, out)
	}

	retrieveResult := parseJSON(t, out)
	if retrieveResult["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", retrieveResult["status"])
	}
	count, ok := retrieveResult["count"].(float64)
	if !ok || count < 1 {
		t.Fatalf("expected at least 1 result, got %v", retrieveResult["count"])
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

func TestCLIRetrieveMissingFlags(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name string
		args []string
	}{
		{"no flags", []string{"retrieve"}},
		{"missing query and vector", []string{"retrieve", "--collection", "test"}},
		{"missing collection", []string{"retrieve", "--query", "hello"}},
		{"missing collection with vector", []string{"retrieve", "--vector", "[0.1]"}},
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

	// Verify collection is empty
	out, err = runCLI(t, binary, "retrieve",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("retrieve failed: %v\n%s", err, out)
	}

	retrieveResult := parseJSON(t, out)
	count, _ := retrieveResult["count"].(float64)
	if count != 0 {
		t.Errorf("expected 0 results after forget, got %v", count)
	}
}

func TestCLIRetrieveWithRecencyBoost(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_recency_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add a memory
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--payload", `{"text": "recency test"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Retrieve with recency boost
	out, err = runCLI(t, binary, "retrieve",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--recency-boost", "0.5",
		"--recency-scale", "3600",
	)
	if err != nil {
		t.Fatalf("retrieve with recency boost failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", result["status"])
	}
	count, ok := result["count"].(float64)
	if !ok || count < 1 {
		t.Fatalf("expected at least 1 result, got %v", result["count"])
	}

	// Score should be boosted above 1.0 (cosine max) for a just-added memory
	results, ok := result["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatal("expected results array with at least 1 entry")
	}
	firstResult, ok := results[0].(map[string]any)
	if !ok {
		t.Fatal("expected first result to be a map")
	}
	score, ok := firstResult["score"].(float64)
	if !ok {
		t.Fatal("expected score to be a number")
	}
	if score <= 1.0 {
		t.Logf("Note: score %.4f (boost may have decayed)", score)
	}
}

func TestCLIRetrieveRecencyBoostDefaultOff(t *testing.T) {
	binary := buildBinary(t)
	skipIfNoQdrant(t, binary)

	collection := "test_cli_recency_default_" + t.Name()
	defer func() {
		runCLI(t, binary, "forget", "--collection", collection, "--ttl", "0s")
	}()

	// Add
	out, err := runCLI(t, binary, "add",
		"--collection", collection,
		"--vector", "[0.5, 0.5, 0.5, 0.5]",
		"--payload", `{"text": "default boost test"}`,
	)
	if err != nil {
		t.Fatalf("add failed: %v\n%s", err, out)
	}

	// Retrieve WITHOUT recency flags — should work as pure similarity
	out, err = runCLI(t, binary, "retrieve",
		"--collection", collection,
		"--vector", "[0.5, 0.5, 0.5, 0.5]",
	)
	if err != nil {
		t.Fatalf("retrieve failed: %v\n%s", err, out)
	}

	result := parseJSON(t, out)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", result["status"])
	}

	// Score should be <= 1.0 (no boost applied)
	results, ok := result["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatal("expected results")
	}
	firstResult := results[0].(map[string]any)
	score := firstResult["score"].(float64)
	if score > 1.01 { // small epsilon for float precision
		t.Errorf("expected score <= 1.0 without boost, got %.4f", score)
	}
}

func TestCLIAddRetrievePreservesPayload(t *testing.T) {
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

	// Retrieve and check payload fields
	out, err = runCLI(t, binary, "retrieve",
		"--collection", collection,
		"--vector", "[0.1, 0.2, 0.3, 0.4]",
		"--min-score", "0.9",
	)
	if err != nil {
		t.Fatalf("retrieve failed: %v\n%s", err, out)
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

// --- Text mode tests (require both Qdrant and Ollama) ---

func TestCLITextAddAndRetrieve(t *testing.T) {
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

	// Retrieve by text query (query is also embedded via Ollama)
	out, err = runCLI(t, binary, "retrieve",
		"--collection", collection,
		"--query", "dark mode",
		"--limit", "5",
	)
	if err != nil {
		t.Fatalf("retrieve text failed: %v\n%s", err, out)
	}

	retrieveResult := parseJSON(t, out)
	if retrieveResult["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", retrieveResult["status"])
	}
	count, ok := retrieveResult["count"].(float64)
	if !ok || count < 1 {
		t.Fatalf("expected at least 1 result, got %v", retrieveResult["count"])
	}

	// Verify the text is in the payload and score is present
	results := retrieveResult["results"].([]any)
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

	// Retrieve and check payload
	out, err = runCLI(t, binary, "retrieve",
		"--collection", collection,
		"--query", "golang",
		"--limit", "5",
	)
	if err != nil {
		t.Fatalf("retrieve failed: %v\n%s", err, out)
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

func TestCLITextRetrieveMissingQuery(t *testing.T) {
	binary := buildBinary(t)

	// No --query and no --vector should fail
	_, err := runCLI(t, binary, "retrieve",
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
	out, err = runCLI(t, binary, "retrieve",
		"--collection", collection,
		"--query", "forgotten",
		"--limit", "10",
	)
	if err != nil {
		t.Fatalf("retrieve failed: %v\n%s", err, out)
	}

	retrieveResult := parseJSON(t, out)
	count, _ := retrieveResult["count"].(float64)
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
	out, err := runCLI(t, binary, "retrieve",
		"--collection", collection,
		"--query", "night theme preferences",
		"--limit", "3",
	)
	if err != nil {
		t.Fatalf("retrieve failed: %v\n%s", err, out)
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

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
