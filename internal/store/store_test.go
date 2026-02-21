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

// cleanupMemories deletes the memories collection if it exists.
func cleanupMemories(t *testing.T, s *Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.DeleteCollection(ctx)
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
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("auto-generated ID", func(t *testing.T) {
		id, err := s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{
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
		id, err := s.Add(ctx, customID, []float32{0.5, 0.6, 0.7, 0.8}, map[string]any{
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
		id, err := s.Add(ctx, "", []float32{0.9, 0.8, 0.7, 0.6}, map[string]any{
			"text": "timestamp check",
		})
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}

		results, err := s.Retrieve(ctx, []float32{0.9, 0.8, 0.7, 0.6}, 0.9, 10)
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
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Seed data
	_, err := s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "alpha"})
	if err != nil {
		t.Fatalf("seed Add failed: %v", err)
	}
	_, err = s.Add(ctx, "", []float32{0.9, 0.8, 0.7, 0.6}, map[string]any{"text": "beta"})
	if err != nil {
		t.Fatalf("seed Add failed: %v", err)
	}
	_, err = s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.5}, map[string]any{"text": "gamma"})
	if err != nil {
		t.Fatalf("seed Add failed: %v", err)
	}

	t.Run("top 1 result", func(t *testing.T) {
		results, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 1)
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
		results, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 3)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("min-score filters low matches", func(t *testing.T) {
		results, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.999, 10)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		// Only exact match should pass 0.999 threshold
		if len(results) != 1 {
			t.Fatalf("expected 1 result with min-score 0.999, got %d", len(results))
		}
	})

	t.Run("results sorted by score descending", func(t *testing.T) {
		results, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10)
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
		results1, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.99, 1)
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
		results2, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.99, 1)
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
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add and then forget to create an empty collection
	_, err := s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "temp"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	_, err = s.Forget(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}

	results, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10)
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
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add two memories
	_, err := s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "old"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	_, err = s.Add(ctx, "", []float32{0.5, 0.6, 0.7, 0.8}, map[string]any{"text": "also old"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	t.Run("no deletions when within TTL", func(t *testing.T) {
		deleted, err := s.Forget(ctx, 1*time.Hour)
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

		deleted, err := s.Forget(ctx, 1*time.Second)
		if err != nil {
			t.Fatalf("Forget failed: %v", err)
		}
		if deleted != 2 {
			t.Fatalf("expected 2 deletions, got %d", deleted)
		}

		// Verify they're gone
		results, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10)
		if err != nil {
			t.Fatalf("Retrieve failed: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("expected 0 results after forget, got %d", len(results))
		}
	})

	t.Run("no error on nonexistent collection", func(t *testing.T) {
		// Delete the collection to simulate nonexistence
		cleanupMemories(t, s)
		deleted, err := s.Forget(ctx, 1*time.Hour)
		if err != nil {
			t.Fatalf("Forget on nonexistent collection should not error: %v", err)
		}
		if deleted != 0 {
			t.Fatalf("expected 0 deletions, got %d", deleted)
		}
	})
}

func TestForgetPreservesRecentlyAccessed(t *testing.T) {
	s := testStore(t)
	defer s.Close()
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add two memories
	_, err := s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "will be accessed"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	_, err = s.Add(ctx, "", []float32{0.9, 0.8, 0.7, 0.6}, map[string]any{"text": "will be forgotten"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Wait a bit
	time.Sleep(1100 * time.Millisecond)

	// Access only the first one (exact match query)
	_, err = s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.99, 1)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	// Now forget with a short TTL — only the un-accessed one should be deleted
	deleted, err := s.Forget(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deletion (the un-accessed memory), got %d", deleted)
	}

	// The accessed one should still be there
	results, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 10)
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

func TestAddUpsertBehavior(t *testing.T) {
	s := testStore(t)
	defer s.Close()
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fixedID := "11111111-2222-3333-4444-555555555555"

	// Add with a fixed ID
	_, err := s.Add(ctx, fixedID, []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{
		"text": "original",
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Upsert same ID with different payload
	_, err = s.Add(ctx, fixedID, []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{
		"text": "updated",
	})
	if err != nil {
		t.Fatalf("Add (upsert) failed: %v", err)
	}

	// Retrieve — should only get one result with the updated payload
	results, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.99, 10)
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

func TestForgetIdempotent(t *testing.T) {
	s := testStore(t)
	defer s.Close()
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "temp"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Forget everything
	deleted1, err := s.Forget(ctx, 0)
	if err != nil {
		t.Fatalf("First forget failed: %v", err)
	}
	if deleted1 != 1 {
		t.Fatalf("expected 1 deletion, got %d", deleted1)
	}

	// Forget again — should delete 0 (already gone)
	deleted2, err := s.Forget(ctx, 0)
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
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "fresh"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Forget with 1 year TTL — nothing should be deleted
	deleted, err := s.Forget(ctx, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deletions with large TTL, got %d", deleted)
	}

	// Memory should still be there
	results, err := s.Retrieve(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 0.0, 1)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after large-TTL forget, got %d", len(results))
	}
}

func TestGet(t *testing.T) {
	s := testStore(t)
	defer s.Close()
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add a memory with a known ID
	fixedID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	_, err := s.Add(ctx, fixedID, []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{
		"text": "get test memory",
		"tag":  "important",
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	t.Run("fetches existing memory by ID", func(t *testing.T) {
		result, err := s.Get(ctx, fixedID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if result.ID != fixedID {
			t.Errorf("expected ID %q, got %q", fixedID, result.ID)
		}
		if result.Payload["text"] != "get test memory" {
			t.Errorf("expected text 'get test memory', got %v", result.Payload["text"])
		}
		if result.Payload["tag"] != "important" {
			t.Errorf("expected tag 'important', got %v", result.Payload["tag"])
		}
		if result.Payload["created_at"] == nil {
			t.Error("missing created_at in payload")
		}
		if result.Payload["last_accessed"] == nil {
			t.Error("missing last_accessed in payload")
		}
	})

	t.Run("returns nil for nonexistent ID", func(t *testing.T) {
		result, err := s.Get(ctx, "00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if result != nil {
			t.Fatalf("expected nil for nonexistent ID, got %+v", result)
		}
	})

	t.Run("returns nil when collection does not exist", func(t *testing.T) {
		// Delete the collection to simulate nonexistence
		cleanupMemories(t, s)
		result, err := s.Get(ctx, fixedID)
		if err != nil {
			t.Fatalf("Get should not error on nonexistent collection: %v", err)
		}
		if result != nil {
			t.Fatalf("expected nil when collection does not exist, got %+v", result)
		}
	})
}

func TestGetUpdatesLastAccessed(t *testing.T) {
	s := testStore(t)
	defer s.Close()
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fixedID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	_, err := s.Add(ctx, fixedID, []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{
		"text": "last accessed test",
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// First get
	result1, err := s.Get(ctx, fixedID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	ts1, ok := result1.Payload["last_accessed"].(string)
	if !ok {
		t.Fatal("last_accessed not a string")
	}

	time.Sleep(1100 * time.Millisecond)

	// Second get — last_accessed should be updated
	result2, err := s.Get(ctx, fixedID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	ts2, ok := result2.Payload["last_accessed"].(string)
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
}

func TestCount(t *testing.T) {
	s := testStore(t)
	defer s.Close()
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Count before adding anything (collection may not exist)
	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 memories initially, got %d", count)
	}

	// Add some memories
	_, err = s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{"text": "one"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	_, err = s.Add(ctx, "", []float32{0.5, 0.6, 0.7, 0.8}, map[string]any{"text": "two"})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	count, err = s.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 memories, got %d", count)
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

func TestForgetSkipsPinnedMemories(t *testing.T) {
	s := testStore(t)
	defer s.Close()
	defer cleanupMemories(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add a pinned memory
	pinnedID, err := s.Add(ctx, "", []float32{0.1, 0.2, 0.3, 0.4}, map[string]any{
		"text":   "important pinned memory",
		"pinned": true,
	})
	if err != nil {
		t.Fatalf("Add pinned failed: %v", err)
	}

	// Add an unpinned memory
	_, err = s.Add(ctx, "", []float32{0.5, 0.6, 0.7, 0.8}, map[string]any{
		"text": "ephemeral memory",
	})
	if err != nil {
		t.Fatalf("Add unpinned failed: %v", err)
	}

	// Wait so both memories become stale
	time.Sleep(1100 * time.Millisecond)

	// Forget with a short TTL — only the unpinned one should be deleted
	deleted, err := s.Forget(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deletion (the unpinned memory), got %d", deleted)
	}

	// The pinned one should still be there
	result, err := s.Get(ctx, pinnedID)
	if err != nil {
		t.Fatalf("Get pinned memory failed: %v", err)
	}
	if result.Payload["text"] != "important pinned memory" {
		t.Errorf("expected pinned memory text, got %v", result.Payload["text"])
	}
}
