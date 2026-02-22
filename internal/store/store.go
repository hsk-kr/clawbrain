package store

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// collectionName is the single Qdrant collection used for all memories.
const collectionName = "memories"

// Store wraps the Qdrant client and provides memory operations.
type Store struct {
	client *qdrant.Client
}

// Result represents a single retrieval result.
type Result struct {
	ID      string         `json:"id"`
	Score   float32        `json:"score"`
	Payload map[string]any `json:"payload"`
}

// New creates a new Store connected to Qdrant.
func New(host string, port int) (*Store, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: host,
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to qdrant: %w", err)
	}
	return &Store{client: client}, nil
}

// Close closes the underlying Qdrant connection.
func (s *Store) Close() error {
	return s.client.Close()
}

// ensureCollection creates the memories collection if it doesn't exist.
func (s *Store) ensureCollection(ctx context.Context, vectorSize uint64) error {
	exists, err := s.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}
	if exists {
		return nil
	}

	err = s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     vectorSize,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

// Add stores a vector with its payload.
// It auto-adds created_at and last_accessed timestamps to the payload.
// If id is empty, a UUID is generated.
func (s *Store) Add(ctx context.Context, id string, vector []float32, payload map[string]any) (string, error) {
	if err := s.ensureCollection(ctx, uint64(len(vector))); err != nil {
		return "", err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Only set created_at if not already present (e.g. preserved from a merged memory)
	if _, exists := payload["created_at"]; !exists {
		payload["created_at"] = now
	}
	payload["last_accessed"] = now

	if id == "" {
		id = uuid.New().String()
	}
	pointID := qdrant.NewIDUUID(id)

	wait := true
	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points: []*qdrant.PointStruct{
			{
				Id:      pointID,
				Vectors: qdrant.NewVectors(vector...),
				Payload: qdrant.NewValueMap(payload),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("upsert: %w", err)
	}

	return id, nil
}

// Retrieve queries memories and returns the top matches.
// It updates last_accessed on all returned points.
// Ranking is pure cosine similarity.
func (s *Store) Retrieve(ctx context.Context, vector []float32, minScore float32, limit uint64) ([]Result, error) {
	// Guard: return empty results gracefully when the collection doesn't exist
	// yet (e.g. no memories have been stored). Matches the behavior of Get,
	// FindSimilar, and every other read method in this package.
	exists, err := s.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("check collection exists: %w", err)
	}
	if !exists {
		return nil, nil
	}

	query := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(vector...),
		WithPayload:    qdrant.NewWithPayload(true),
		ScoreThreshold: &minScore,
		Limit:          &limit,
	}

	results, err := s.client.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	out := make([]Result, 0, len(results))

	for _, point := range results {
		s.updateLastAccessed(ctx, point.Id, nowStr)

		out = append(out, Result{
			ID:      pointIDToString(point.Id),
			Score:   point.Score,
			Payload: valueMapToGoMap(point.Payload),
		})
	}

	return out, nil
}

// Get retrieves a single point by its UUID.
// Returns nil if the point is not found. Updates last_accessed on retrieval.
func (s *Store) Get(ctx context.Context, id string) (*Result, error) {
	exists, err := s.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return nil, nil
	}

	points, err := s.client.Get(ctx, &qdrant.GetPoints{
		CollectionName: collectionName,
		Ids:            []*qdrant.PointId{qdrant.NewIDUUID(id)},
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	})
	if err != nil {
		return nil, fmt.Errorf("get point: %w", err)
	}

	if len(points) == 0 {
		return nil, nil
	}

	point := points[0]

	// Update last_accessed
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	s.updateLastAccessed(ctx, point.Id, nowStr)

	return &Result{
		ID:      pointIDToString(point.Id),
		Score:   0,
		Payload: valueMapToGoMap(point.Payload),
	}, nil
}

// Forget deletes memories not accessed within the given TTL.
// Returns the number of memories deleted.
func (s *Store) Forget(ctx context.Context, ttl time.Duration) (int, error) {
	// Check if collection exists first
	exists, err := s.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return 0, fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return 0, nil
	}

	cutoff := time.Now().UTC().Add(-ttl)

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewDatetimeRange("last_accessed", &qdrant.DatetimeRange{
				Lt: timestamppb.New(cutoff),
			}),
		},
		MustNot: []*qdrant.Condition{
			qdrant.NewMatchBool("pinned", true),
		},
	}

	// Scroll to find all stale points
	pointIDs, err := s.scrollPointIDs(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("scroll stale points: %w", err)
	}

	if len(pointIDs) == 0 {
		return 0, nil
	}

	// Delete them
	wait := true
	_, err = s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: pointIDs,
				},
			},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("delete stale points: %w", err)
	}

	return len(pointIDs), nil
}

// Delete removes a single memory by its UUID.
// Returns nil if the point doesn't exist or the collection doesn't exist.
func (s *Store) Delete(ctx context.Context, id string) error {
	exists, err := s.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return nil
	}

	wait := true
	_, err = s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: []*qdrant.PointId{qdrant.NewIDUUID(id)},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("delete point: %w", err)
	}
	return nil
}

// FindSimilar searches for memories similar to the given vector above a score threshold.
// Unlike Retrieve, it does NOT update last_accessed on returned points.
// This is intended for internal dedup checks before insertion.
func (s *Store) FindSimilar(ctx context.Context, vector []float32, threshold float32, limit uint64) ([]Result, error) {
	exists, err := s.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return nil, nil
	}

	query := &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(vector...),
		WithPayload:    qdrant.NewWithPayload(true),
		ScoreThreshold: &threshold,
		Limit:          &limit,
	}

	results, err := s.client.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	out := make([]Result, 0, len(results))
	for _, point := range results {
		out = append(out, Result{
			ID:      pointIDToString(point.Id),
			Score:   point.Score,
			Payload: valueMapToGoMap(point.Payload),
		})
	}
	return out, nil
}

// DeleteCollection deletes the memories collection entirely.
// Used for testing and full resets. Returns nil if the collection doesn't exist.
func (s *Store) DeleteCollection(ctx context.Context) error {
	exists, err := s.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return nil
	}
	return s.client.DeleteCollection(ctx, collectionName)
}

// Count returns the approximate number of memories stored.
func (s *Store) Count(ctx context.Context) (uint64, error) {
	exists, err := s.client.CollectionExists(ctx, collectionName)
	if err != nil {
		return 0, fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return 0, nil
	}

	count, err := s.client.Count(ctx, &qdrant.CountPoints{
		CollectionName: collectionName,
	})
	if err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return count, nil
}

// Check runs an end-to-end connectivity check against Qdrant.
func (s *Store) Check(ctx context.Context) error {
	checkCollection := "clawbrain_check"

	// Cleanup any leftover
	exists, err := s.client.CollectionExists(ctx, checkCollection)
	if err != nil {
		return fmt.Errorf("check collection exists: %w", err)
	}
	if exists {
		if err := s.client.DeleteCollection(ctx, checkCollection); err != nil {
			return fmt.Errorf("cleanup leftover collection: %w", err)
		}
	}

	// Create
	err = s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: checkCollection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     4,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("create test collection: %w", err)
	}

	// Upsert
	wait := true
	_, err = s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: checkCollection,
		Wait:           &wait,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewIDNum(1),
				Vectors: qdrant.NewVectors(0.1, 0.2, 0.3, 0.4),
				Payload: qdrant.NewValueMap(map[string]any{"test": true}),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("upsert test vector: %w", err)
	}

	// Query
	results, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: checkCollection,
		Query:          qdrant.NewQuery(0.1, 0.2, 0.3, 0.4),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return fmt.Errorf("query test vector: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("query returned no results")
	}

	// Cleanup
	if err := s.client.DeleteCollection(ctx, checkCollection); err != nil {
		return fmt.Errorf("cleanup test collection: %w", err)
	}

	return nil
}

// updateLastAccessed sets the last_accessed payload field on a point.
// Errors are logged but not propagated — a failed timestamp update should
// not cause a retrieval to fail.
func (s *Store) updateLastAccessed(ctx context.Context, id *qdrant.PointId, timestamp string) {
	wait := true
	_, err := s.client.SetPayload(ctx, &qdrant.SetPayloadPoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Payload: qdrant.NewValueMap(map[string]any{
			"last_accessed": timestamp, // RFC3339Nano for sub-second precision
		}),
		PointsSelector: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: []*qdrant.PointId{id},
				},
			},
		},
	})
	if err != nil {
		log.Printf("warning: failed to update last_accessed on %v: %v", pointIDToString(id), err)
	}
}

// scrollPointIDs scrolls through memories with a filter and returns all matching point IDs.
func (s *Store) scrollPointIDs(ctx context.Context, filter *qdrant.Filter) ([]*qdrant.PointId, error) {
	var allIDs []*qdrant.PointId
	var offset *qdrant.PointId
	limit := uint32(100)

	for {
		points, nextOffset, err := s.client.ScrollAndOffset(ctx, &qdrant.ScrollPoints{
			CollectionName: collectionName,
			Filter:         filter,
			Limit:          &limit,
			Offset:         offset,
			WithPayload:    qdrant.NewWithPayload(false),
			WithVectors:    qdrant.NewWithVectors(false),
		})
		if err != nil {
			return nil, err
		}

		for _, point := range points {
			allIDs = append(allIDs, point.Id)
		}

		if nextOffset == nil {
			break
		}
		offset = nextOffset
	}

	return allIDs, nil
}

// pointIDToString converts a Qdrant PointId to its string representation.
func pointIDToString(id *qdrant.PointId) string {
	switch v := id.GetPointIdOptions().(type) {
	case *qdrant.PointId_Uuid:
		return v.Uuid
	case *qdrant.PointId_Num:
		return strconv.FormatUint(v.Num, 10)
	default:
		return ""
	}
}

// valueMapToGoMap converts Qdrant's map[string]*Value to a plain Go map.
func valueMapToGoMap(m map[string]*qdrant.Value) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = valueToGo(v)
	}
	return out
}

// valueToGo converts a single Qdrant Value to a Go value.
func valueToGo(v *qdrant.Value) any {
	if v == nil {
		return nil
	}
	switch kind := v.GetKind().(type) {
	case *qdrant.Value_NullValue:
		return nil
	case *qdrant.Value_BoolValue:
		return kind.BoolValue
	case *qdrant.Value_IntegerValue:
		return kind.IntegerValue
	case *qdrant.Value_DoubleValue:
		return kind.DoubleValue
	case *qdrant.Value_StringValue:
		return kind.StringValue
	case *qdrant.Value_ListValue:
		items := kind.ListValue.GetValues()
		list := make([]any, len(items))
		for i, item := range items {
			list[i] = valueToGo(item)
		}
		return list
	case *qdrant.Value_StructValue:
		fields := kind.StructValue.GetFields()
		obj := make(map[string]any, len(fields))
		for fk, fv := range fields {
			obj[fk] = valueToGo(fv)
		}
		return obj
	default:
		return nil
	}
}
