package ollama

import (
	"context"
	"testing"
	"time"
)

const (
	testURL   = "http://localhost:11434"
	testModel = "all-minilm"
)

// skipIfNoOllama skips the test if Ollama is not reachable.
func skipIfNoOllama(t *testing.T) *Client {
	t.Helper()
	c := New(testURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Health(ctx); err != nil {
		t.Skipf("Ollama not reachable at %s: %v", testURL, err)
	}
	return c
}

func TestHealth(t *testing.T) {
	c := skipIfNoOllama(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Health(ctx); err != nil {
		t.Fatalf("Health failed: %v", err)
	}
}

func TestEmbed(t *testing.T) {
	c := skipIfNoOllama(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec, err := c.Embed(ctx, testModel, "the user prefers dark mode")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(vec) == 0 {
		t.Fatal("expected non-empty vector")
	}

	// all-minilm produces 384-dimensional vectors
	if len(vec) != 384 {
		t.Errorf("expected 384 dimensions, got %d", len(vec))
	}

	// Sanity check: values should be finite and non-zero
	allZero := true
	for _, v := range vec {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("vector is all zeros")
	}
}

func TestEmbedDeterministic(t *testing.T) {
	c := skipIfNoOllama(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	text := "embedding models are deterministic"

	vec1, err := c.Embed(ctx, testModel, text)
	if err != nil {
		t.Fatalf("first Embed failed: %v", err)
	}

	vec2, err := c.Embed(ctx, testModel, text)
	if err != nil {
		t.Fatalf("second Embed failed: %v", err)
	}

	if len(vec1) != len(vec2) {
		t.Fatalf("dimension mismatch: %d vs %d", len(vec1), len(vec2))
	}

	for i := range vec1 {
		if vec1[i] != vec2[i] {
			t.Fatalf("vectors differ at index %d: %f vs %f", i, vec1[i], vec2[i])
		}
	}
}

func TestEmbedEmptyText(t *testing.T) {
	c := skipIfNoOllama(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Empty text should still return a vector (Ollama handles it)
	vec, err := c.Embed(ctx, testModel, "")
	if err != nil {
		// Some models may error on empty input — that's acceptable
		t.Logf("Embed with empty text returned error (acceptable): %v", err)
		return
	}

	if len(vec) == 0 {
		t.Error("expected non-empty vector even for empty text")
	}
}

func TestEmbedBadModel(t *testing.T) {
	c := skipIfNoOllama(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := c.Embed(ctx, "nonexistent-model-xyz", "test")
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
}
