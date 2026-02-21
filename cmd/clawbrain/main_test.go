package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
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
func skipIfNoQdrant(t *testing.T, binary string) {
	t.Helper()
	out, err := runCLI(t, binary, "check")
	if err != nil {
		t.Skipf("Qdrant not available, skipping: %s", string(out))
	}
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
		{"missing vector", []string{"add", "--collection", "test", "--payload", "{}"}},
		{"missing payload", []string{"add", "--collection", "test", "--vector", "[0.1]"}},
		{"missing collection", []string{"add", "--vector", "[0.1]", "--payload", "{}"}},
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
		{"missing vector", []string{"retrieve", "--collection", "test"}},
		{"missing collection", []string{"retrieve", "--vector", "[0.1]"}},
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

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
