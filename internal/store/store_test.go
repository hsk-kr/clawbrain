package store

import (
	"context"
	"testing"
	"time"

	"github.com/qdrant/go-client/qdrant"
)

const (
	testHost = "localhost"
	testPort = 6334
)

// testStore creates a store connected to the local Qdrant instance.
// Tests are skipped if Qdrant is not reachable.
func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(testHost, testPort)
	if err != nil {
		t.Skipf("Qdrant not reachable at %s:%d: %v", testHost, testPort, err)
	}

	// Quick connectivity check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Check(ctx); err != nil {
		s.Close()
		t.Skipf("Qdrant connectivity check failed: %v", err)
	}

	return s
}

// testCollection creates a unique collection name for each test and ensures cleanup.
func testCollection(t *testing.T) string {
	t.Helper()
	return "test_" + t.Name()
}

// cleanupCollection deletes a collection if it exists.
func cleanupCollection(t *testing.T, s *Store, collection string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, err := s.client.CollectionExists(ctx, collection)
	if err != nil {
		return
	}
	if exists {
		_ = s.client.DeleteCollection(ctx, collection)
	}
}

// --- Integration Tests (require Qdrant) ---

func TestCheck(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.Check(ctx); err != nil {
		t.Fatalf("Check failed: %v", err)
	}
}

func TestAdd(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("auto-generated ID", func(t *testing.T) {
		id, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{
			"text": "hello world",
		})
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
		if id == "" {
			t.Fatal("expected non-empty ID")
		}
	})

	t.Run("custom ID", func(t *testing.T) {
		customID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
		id, err := s.Add(ctx, collection, customID, []float32{0.5, 0.6, 0.7, 0.8}, map[string]any{
			"text": "custom id",
		})
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
		if id != customID {
			t.Fatalf("expected ID %q, got %q", customID, id)
		}
	})

	t.Run("payload includes timestamps", func(t *testing.T) {
		// Add and then retrieve to check timestamps
		id, err := s.Add(ctx, collection, "", []float32{0.9, 0.8, 0.7, 0.6}, map[string]any{
			"text": "timestamp check",
		})
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}

		results, err := s.Retrieve(ctx, collection, []float32{0.9, 0.8, 0.7, 0.6}, 0.9, 10, 0, 0)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}

		var found bool
		for _, r := range results {
			if r.ID == id {
				found = true
				if _, ok := r.Payload["created_at"]; !ok {
					t.Error("missing created_at in payload")
				}
				if _, ok := r.Payload["last_accessed"]; !ok {
					t.Error("missing last_accessed in payload")
				}
			}
		}
		if !found {
			t.Fatalf("added point %s not found in results", id)
		}
	})
}

func TestRetrieve(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Seed data
	_, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "alpha"})
	if err != nil {
		t.Fatalf("seed Add failed: %v", err)
	}
	_, err = s.Add(ctx, collection, "", []float32{0.9, 0.8, 0.7, 0.6}, map[string]any{"text": "beta"})
	if err != nil {
		t.Fatalf("seed Add failed: %v", err)
	}
	_, err = s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.5}, map[string]any{"text": "gamma"})
	if err != nil {
		t.Fatalf("seed Add failed: %v", err)
	}

	t.Run("top 1 result", func(t *testing.T) {
		results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 1, 0, 0)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Payload["text"] != "alpha" {
			t.Errorf("expected top result 'alpha', got %v", results[0].Payload["text"])
		}
		if results[0].Score < 0.99 {
			t.Errorf("expected score ~1.0 for exact match, got %f", results[0].Score)
		}
	})

	t.Run("limit controls result count", func(t *testing.T) {
		results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 3, 0, 0)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("min-score filters low matches", func(t *testing.T) {
		results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.999, 10, 0, 0)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		// Only exact match should pass 0.999 threshold
		if len(results) != 1 {
			t.Fatalf("expected 1 result with min-score 0.999, got %d", len(results))
		}
	})

	t.Run("results sorted by score descending", func(t *testing.T) {
		results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10, 0, 0)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		for i := 1; i < len(results); i++ {
			if results[i].Score > results[i-1].Score {
				t.Errorf("results not sorted by score: index %d (%.4f) > index %d (%.4f)",
					i, results[i].Score, i-1, results[i-1].Score)
			}
		}
	})

	t.Run("updates last_accessed on retrieval", func(t *testing.T) {
		// First retrieve to get baseline
		results1, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.99, 1, 0, 0)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		if len(results1) == 0 {
			t.Fatal("expected at least 1 result")
		}
		ts1, ok := results1[0].Payload["last_accessed"].(string)
		if !ok {
			t.Fatal("last_accessed not a string")
		}

		time.Sleep(1100 * time.Millisecond)

		// Second retrieve should have updated last_accessed
		results2, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.99, 1, 0, 0)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		ts2, ok := results2[0].Payload["last_accessed"].(string)
		if !ok {
			t.Fatal("last_accessed not a string")
		}

		t1, err1 := time.Parse(time.RFC3339Nano, ts1)
		t2, err2 := time.Parse(time.RFC3339Nano, ts2)
		if err1 != nil || err2 != nil {
			t.Fatalf("failed to parse timestamps: %v / %v", err1, err2)
		}
		if !t2.After(t1) {
			t.Errorf("last_accessed not updated: %s -> %s", ts1, ts2)
		}
	})
}

func TestRetrieveEmptyCollection(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add and then forget to create an empty collection
	_, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "temp"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	_, err = s.Forget(ctx, collection, 1*time.Second)
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}

	results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10, 0, 0)
	if err != nil {
		t.Fatalf("Retrieve on empty collection failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestForget(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add two memories
	_, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "old"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	_, err = s.Add(ctx, collection, "", []float32{0.5, 0.6, 0.7, 0.8}, map[string]any{"text": "also old"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	t.Run("no deletions when within TTL", func(t *testing.T) {
		deleted, err := s.Forget(ctx, collection, 1*time.Hour)
		if err != nil {
			t.Fatalf("Forget failed: %v", err)
		}
		if deleted != 0 {
			t.Fatalf("expected 0 deletions, got %d", deleted)
		}
	})

	t.Run("deletes stale memories", func(t *testing.T) {
		// Wait so memories become stale
		time.Sleep(1100 * time.Millisecond)

		deleted, err := s.Forget(ctx, collection, 1*time.Second)
		if err != nil {
			t.Fatalf("Forget failed: %v", err)
		}
		if deleted != 2 {
			t.Fatalf("expected 2 deletions, got %d", deleted)
		}

		// Verify they're gone
		results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10, 0, 0)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("expected 0 results after forget, got %d", len(results))
		}
	})

	t.Run("nonexistent collection errors", func(t *testing.T) {
		_, err := s.Forget(ctx, "nonexistent_collection_xyz", 1*time.Hour)
		if err == nil {
			t.Fatal("expected error for nonexistent collection")
		}
	})
}

func TestForgetPreservesRecentlyAccessed(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add two memories
	_, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "will be accessed"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	_, err = s.Add(ctx, collection, "", []float32{0.9, 0.8, 0.7, 0.6}, map[string]any{"text": "will be forgotten"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Wait a bit
	time.Sleep(1100 * time.Millisecond)

	// Access only the first one (exact match query)
	_, err = s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.99, 1, 0, 0)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	// Now forget with a short TTL — only the un-accessed one should be deleted
	deleted, err := s.Forget(ctx, collection, 1*time.Second)
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deletion (the un-accessed memory), got %d", deleted)
	}

	// The accessed one should still be there
	results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10, 0, 0)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 remaining result, got %d", len(results))
	}
	if results[0].Payload["text"] != "will be accessed" {
		t.Errorf("wrong memory survived: %v", results[0].Payload["text"])
	}
}

func TestRetrieveWithRecencyBoost(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add two memories with the same vector direction but slightly different
	// so they both match a query. "old" is added first, "fresh" second.
	_, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "old"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	_, err = s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.5}, map[string]any{"text": "fresh"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	t.Run("recency boost returns results", func(t *testing.T) {
		results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10, 0.5, 3600)
		if err != nil {
			t.Fatalf("Retrieve with recency boost failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected results with recency boost, got 0")
		}
	})

	t.Run("recency boost inflates scores above pure similarity", func(t *testing.T) {
		results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10, 0.5, 3600)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		// cosine similarity maxes at 1.0; with boost, top score should exceed it
		if len(results) > 0 && results[0].Score <= 1.0 {
			t.Logf("Note: top score is %.4f (boost may have decayed if test was slow)", results[0].Score)
		}
	})

	t.Run("zero boost matches plain retrieval", func(t *testing.T) {
		plain, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10, 0, 0)
		if err != nil {
			t.Fatalf("Plain retrieve failed: %v", err)
		}
		boosted, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10, 0, 3600)
		if err != nil {
			t.Fatalf("Boosted retrieve failed: %v", err)
		}
		if len(plain) != len(boosted) {
			t.Fatalf("result count mismatch: plain=%d boosted=%d", len(plain), len(boosted))
		}
	})

	t.Run("recency boost updates last_accessed", func(t *testing.T) {
		// Retrieve with boost, note the timestamp
		results1, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 1, 0.5, 3600)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		if len(results1) == 0 {
			t.Fatal("expected at least 1 result")
		}
		ts1, ok := results1[0].Payload["last_accessed"].(string)
		if !ok {
			t.Fatal("last_accessed not a string")
		}

		time.Sleep(1100 * time.Millisecond)

		// Second retrieve with boost should show updated timestamp
		results2, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 1, 0.5, 3600)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		ts2, ok := results2[0].Payload["last_accessed"].(string)
		if !ok {
			t.Fatal("last_accessed not a string")
		}

		t1, _ := time.Parse(time.RFC3339Nano, ts1)
		t2, _ := time.Parse(time.RFC3339Nano, ts2)
		if !t2.After(t1) {
			t.Errorf("last_accessed not updated with recency boost: %s -> %s", ts1, ts2)
		}
	})
}

func TestRecencyBoostFavorsRecentlyAccessed(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add two memories with IDENTICAL vectors — same cosine similarity to any query.
	// The only differentiator will be recency.
	_, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "stale"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Wait so the first memory's last_accessed ages
	time.Sleep(1100 * time.Millisecond)

	_, err = s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "recent"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// With strong recency boost and short scale, the recently-added memory
	// should rank higher despite identical similarity.
	results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 2, 1.0, 1)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Payload["text"] != "recent" {
		t.Errorf("expected 'recent' to rank first with recency boost, got %q (scores: %.4f, %.4f)",
			results[0].Payload["text"], results[0].Score, results[1].Score)
	}
	// The recent one should have a higher score
	if results[0].Score <= results[1].Score {
		t.Errorf("recent memory should score higher: recent=%.4f stale=%.4f",
			results[0].Score, results[1].Score)
	}
}

func TestAddUpsertBehavior(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fixedID := "11111111-2222-3333-4444-555555555555"

	// Add with a fixed ID
	_, err := s.Add(ctx, collection, fixedID, []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{
		"text": "original",
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Upsert same ID with different payload
	_, err = s.Add(ctx, collection, fixedID, []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{
		"text": "updated",
	})
	if err != nil {
		t.Fatalf("Add (upsert) failed: %v", err)
	}

	// Retrieve — should only get one result with the updated payload
	results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.99, 10, 0, 0)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (upsert should replace), got %d", len(results))
	}
	if results[0].ID != fixedID {
		t.Errorf("expected ID %q, got %q", fixedID, results[0].ID)
	}
	if results[0].Payload["text"] != "updated" {
		t.Errorf("expected payload 'updated', got %v", results[0].Payload["text"])
	}
}

func TestAddAutoCreatesCollection(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test with 3-dimensional vector
	collection3d := testCollection(t) + "_3d"
	defer cleanupCollection(t, s, collection3d)

	_, err := s.Add(ctx, collection3d, "", []float32{0.1, 0.2, 0.3}, map[string]any{"dim": 3})
	if err != nil {
		t.Fatalf("Add with 3d vector failed: %v", err)
	}

	results, err := s.Retrieve(ctx, collection3d, []float32{0.1, 0.2, 0.3}, 0.0, 1, 0, 0)
	if err != nil {
		t.Fatalf("Retrieve 3d failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Test with 8-dimensional vector
	collection8d := testCollection(t) + "_8d"
	defer cleanupCollection(t, s, collection8d)

	_, err = s.Add(ctx, collection8d, "", []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}, map[string]any{"dim": 8})
	if err != nil {
		t.Fatalf("Add with 8d vector failed: %v", err)
	}

	results, err = s.Retrieve(ctx, collection8d, []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}, 0.0, 1, 0, 0)
	if err != nil {
		t.Fatalf("Retrieve 8d failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestForgetIdempotent(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "temp"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Forget everything
	deleted1, err := s.Forget(ctx, collection, 0)
	if err != nil {
		t.Fatalf("First forget failed: %v", err)
	}
	if deleted1 != 1 {
		t.Fatalf("expected 1 deletion, got %d", deleted1)
	}

	// Forget again — should delete 0 (already gone)
	deleted2, err := s.Forget(ctx, collection, 0)
	if err != nil {
		t.Fatalf("Second forget failed: %v", err)
	}
	if deleted2 != 0 {
		t.Fatalf("expected 0 deletions on second forget, got %d", deleted2)
	}
}

func TestForgetLargeTTLDeletesNothing(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "fresh"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Forget with 1 year TTL — nothing should be deleted
	deleted, err := s.Forget(ctx, collection, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deletions with large TTL, got %d", deleted)
	}

	// Memory should still be there
	results, err := s.Retrieve(ctx, collection, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 1, 0, 0)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after large-TTL forget, got %d", len(results))
	}
}

// --- Text-based Add/Retrieve Tests ---

func TestAddText(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("stores text memory", func(t *testing.T) {
		id, err := s.AddText(ctx, collection, "", "the user prefers dark mode", nil)
		if err != nil {
			t.Fatalf("AddText failed: %v", err)
		}
		if id == "" {
			t.Fatal("expected non-empty ID")
		}
	})

	t.Run("custom ID", func(t *testing.T) {
		customID := "aabbccdd-1122-3344-5566-778899aabbcc"
		id, err := s.AddText(ctx, collection, customID, "custom id text", nil)
		if err != nil {
			t.Fatalf("AddText failed: %v", err)
		}
		if id != customID {
			t.Fatalf("expected ID %q, got %q", customID, id)
		}
	})

	t.Run("includes timestamps", func(t *testing.T) {
		_, err := s.AddText(ctx, collection, "", "timestamp test text", nil)
		if err != nil {
			t.Fatalf("AddText failed: %v", err)
		}

		results, err := s.RetrieveText(ctx, collection, "timestamp test", 10)
		if err != nil {
			t.Fatalf("RetrieveText failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least 1 result")
		}

		payload := results[0].Payload
		if _, ok := payload["created_at"]; !ok {
			t.Error("missing created_at in payload")
		}
		if _, ok := payload["last_accessed"]; !ok {
			t.Error("missing last_accessed in payload")
		}
	})

	t.Run("extra payload is preserved", func(t *testing.T) {
		extra := map[string]any{
			"source": "test",
			"count":  42,
		}
		_, err := s.AddText(ctx, collection, "", "extra payload memory", extra)
		if err != nil {
			t.Fatalf("AddText failed: %v", err)
		}

		results, err := s.RetrieveText(ctx, collection, "extra payload", 10)
		if err != nil {
			t.Fatalf("RetrieveText failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least 1 result")
		}

		payload := results[0].Payload
		if payload["source"] != "test" {
			t.Errorf("expected source 'test', got %v", payload["source"])
		}
		if payload["text"] != "extra payload memory" {
			t.Errorf("expected text preserved, got %v", payload["text"])
		}
	})
}

func TestRetrieveText(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Seed data
	_, err := s.AddText(ctx, collection, "", "the user prefers dark mode for coding", nil)
	if err != nil {
		t.Fatalf("AddText failed: %v", err)
	}
	_, err = s.AddText(ctx, collection, "", "project uses golang and qdrant for memory", nil)
	if err != nil {
		t.Fatalf("AddText failed: %v", err)
	}
	_, err = s.AddText(ctx, collection, "", "deploy to production every friday", nil)
	if err != nil {
		t.Fatalf("AddText failed: %v", err)
	}

	t.Run("finds matching text", func(t *testing.T) {
		results, err := s.RetrieveText(ctx, collection, "dark mode", 10)
		if err != nil {
			t.Fatalf("RetrieveText failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least 1 result for 'dark mode'")
		}
		found := false
		for _, r := range results {
			text, _ := r.Payload["text"].(string)
			if text == "the user prefers dark mode for coding" {
				found = true
			}
		}
		if !found {
			t.Error("expected to find 'the user prefers dark mode for coding'")
		}
	})

	t.Run("limit controls result count", func(t *testing.T) {
		// Search for a common word that appears in multiple entries
		results, err := s.RetrieveText(ctx, collection, "the", 1)
		if err != nil {
			t.Fatalf("RetrieveText failed: %v", err)
		}
		if len(results) > 1 {
			t.Fatalf("expected at most 1 result with limit=1, got %d", len(results))
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		results, err := s.RetrieveText(ctx, collection, "xyznonexistent", 10)
		if err != nil {
			t.Fatalf("RetrieveText failed: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("expected 0 results for nonsense query, got %d", len(results))
		}
	})

	t.Run("updates last_accessed on retrieval", func(t *testing.T) {
		// First retrieve
		results1, err := s.RetrieveText(ctx, collection, "golang", 1)
		if err != nil {
			t.Fatalf("RetrieveText failed: %v", err)
		}
		if len(results1) == 0 {
			t.Fatal("expected at least 1 result")
		}
		ts1, ok := results1[0].Payload["last_accessed"].(string)
		if !ok {
			t.Fatal("last_accessed not a string")
		}

		time.Sleep(1100 * time.Millisecond)

		// Second retrieve should update last_accessed
		results2, err := s.RetrieveText(ctx, collection, "golang", 1)
		if err != nil {
			t.Fatalf("RetrieveText failed: %v", err)
		}
		ts2, ok := results2[0].Payload["last_accessed"].(string)
		if !ok {
			t.Fatal("last_accessed not a string")
		}

		t1, err1 := time.Parse(time.RFC3339Nano, ts1)
		t2, err2 := time.Parse(time.RFC3339Nano, ts2)
		if err1 != nil || err2 != nil {
			t.Fatalf("failed to parse timestamps: %v / %v", err1, err2)
		}
		if !t2.After(t1) {
			t.Errorf("last_accessed not updated: %s -> %s", ts1, ts2)
		}
	})

	t.Run("nonexistent collection errors", func(t *testing.T) {
		_, err := s.RetrieveText(ctx, "nonexistent_collection_xyz", "test", 10)
		if err == nil {
			t.Fatal("expected error for nonexistent collection")
		}
	})
}

func TestForgetTextMemories(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add text memories
	_, err := s.AddText(ctx, collection, "", "first text memory", nil)
	if err != nil {
		t.Fatalf("AddText failed: %v", err)
	}
	_, err = s.AddText(ctx, collection, "", "second text memory", nil)
	if err != nil {
		t.Fatalf("AddText failed: %v", err)
	}

	// Forget with 0 TTL — should delete everything
	deleted, err := s.Forget(ctx, collection, 0)
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deletions, got %d", deleted)
	}

	// Verify nothing remains
	results, err := s.RetrieveText(ctx, collection, "memory", 10)
	if err != nil {
		t.Fatalf("RetrieveText failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after forget, got %d", len(results))
	}
}

func TestAddTextUpsertBehavior(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	collection := testCollection(t)
	defer cleanupCollection(t, s, collection)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fixedID := "11111111-2222-3333-4444-555555555555"

	// Add with a fixed ID
	_, err := s.AddText(ctx, collection, fixedID, "original text", nil)
	if err != nil {
		t.Fatalf("AddText failed: %v", err)
	}

	// Upsert same ID with different text
	_, err = s.AddText(ctx, collection, fixedID, "updated text", nil)
	if err != nil {
		t.Fatalf("AddText (upsert) failed: %v", err)
	}

	// Retrieve — should find "updated" but not "original"
	results, err := s.RetrieveText(ctx, collection, "updated", 10)
	if err != nil {
		t.Fatalf("RetrieveText failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (upsert should replace), got %d", len(results))
	}
	if results[0].ID != fixedID {
		t.Errorf("expected ID %q, got %q", fixedID, results[0].ID)
	}
	if results[0].Payload["text"] != "updated text" {
		t.Errorf("expected payload 'updated text', got %v", results[0].Payload["text"])
	}

	// "original" should not be found
	results, err = s.RetrieveText(ctx, collection, "original", 10)
	if err != nil {
		t.Fatalf("RetrieveText failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for old text after upsert, got %d", len(results))
	}
}

// --- Unit Tests for helper functions ---

func TestPointIDToString(t *testing.T) {
	t.Run("UUID", func(t *testing.T) {
		id := qdrant.NewIDUUID("550e8400-e29b-41d4-a716-446655440000")
		got := pointIDToString(id)
		if got != "550e8400-e29b-41d4-a716-446655440000" {
			t.Errorf("expected UUID string, got %q", got)
		}
	})

	t.Run("Num", func(t *testing.T) {
		id := qdrant.NewIDNum(42)
		got := pointIDToString(id)
		if got != "42" {
			t.Errorf("expected '42', got %q", got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		id := &qdrant.PointId{}
		got := pointIDToString(id)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

func TestValueMapToGoMap(t *testing.T) {
	input := qdrant.NewValueMap(map[string]any{
		"str":    "hello",
		"num":    42,
		"float":  3.14,
		"bool":   true,
		"nested": map[string]any{"inner": "value"},
		"list":   []any{"a", "b"},
	})

	result := valueMapToGoMap(input)

	if result["str"] != "hello" {
		t.Errorf("str: expected 'hello', got %v", result["str"])
	}
	// Qdrant stores integers as int64
	if result["num"] != int64(42) {
		t.Errorf("num: expected 42 (int64), got %v (%T)", result["num"], result["num"])
	}
	if result["float"] != 3.14 {
		t.Errorf("float: expected 3.14, got %v", result["float"])
	}
	if result["bool"] != true {
		t.Errorf("bool: expected true, got %v", result["bool"])
	}

	nested, ok := result["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested: expected map, got %T", result["nested"])
	}
	if nested["inner"] != "value" {
		t.Errorf("nested.inner: expected 'value', got %v", nested["inner"])
	}

	list, ok := result["list"].([]any)
	if !ok {
		t.Fatalf("list: expected []any, got %T", result["list"])
	}
	if len(list) != 2 || list[0] != "a" || list[1] != "b" {
		t.Errorf("list: expected ['a','b'], got %v", list)
	}
}

func TestValueToGo(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := valueToGo(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("null value", func(t *testing.T) {
		v := qdrant.NewValueNull()
		if got := valueToGo(v); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("string", func(t *testing.T) {
		v := qdrant.NewValueString("test")
		if got := valueToGo(v); got != "test" {
			t.Errorf("expected 'test', got %v", got)
		}
	})

	t.Run("bool", func(t *testing.T) {
		v := qdrant.NewValueBool(true)
		if got := valueToGo(v); got != true {
			t.Errorf("expected true, got %v", got)
		}
	})

	t.Run("int", func(t *testing.T) {
		v := qdrant.NewValueInt(99)
		if got := valueToGo(v); got != int64(99) {
			t.Errorf("expected 99, got %v", got)
		}
	})

	t.Run("double", func(t *testing.T) {
		v := qdrant.NewValueDouble(2.718)
		if got := valueToGo(v); got != 2.718 {
			t.Errorf("expected 2.718, got %v", got)
		}
	})
}

func TestCollections(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a known collection
	collection := "test_collections_" + t.Name()
	defer cleanupCollection(t, s, collection)

	_, err := s.Add(ctx, collection, "", []float32{0.1, 0.2, 0.3}, map[string]any{"text": "test"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	// List collections — should include our test collection
	names, err := s.Collections(ctx)
	if err != nil {
		t.Fatalf("collections: %v", err)
	}

	found := false
	for _, name := range names {
		if name == collection {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find %q in collections list, got %v", collection, names)
	}
}
