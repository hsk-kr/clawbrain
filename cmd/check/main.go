package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/qdrant/go-client/qdrant"
)

func main() {
	// Connect to Qdrant on gRPC port
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: "localhost",
		Port: 6334,
	})
	if err != nil {
		log.Fatalf("Failed to connect to Qdrant: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "clawbrain_test"

	// Delete collection if it already exists (idempotent runs)
	exists, err := client.CollectionExists(ctx, collectionName)
	if err != nil {
		log.Fatalf("Failed to check collection: %v", err)
	}
	if exists {
		err = client.DeleteCollection(ctx, collectionName)
		if err != nil {
			log.Fatalf("Failed to delete existing collection: %v", err)
		}
	}

	// Create collection — 4-dim vectors with cosine distance
	err = client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     4,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		log.Fatalf("Failed to create collection: %v", err)
	}
	fmt.Println("Created collection:", collectionName)

	// Upsert vectors with payload
	wait := true
	_, err = client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewIDNum(1),
				Vectors: qdrant.NewVectors(0.05, 0.61, 0.76, 0.74),
				Payload: qdrant.NewValueMap(map[string]any{
					"agent": "alpha",
					"topic": "navigation",
				}),
			},
			{
				Id:      qdrant.NewIDNum(2),
				Vectors: qdrant.NewVectors(0.19, 0.81, 0.75, 0.11),
				Payload: qdrant.NewValueMap(map[string]any{
					"agent": "beta",
					"topic": "planning",
				}),
			},
			{
				Id:      qdrant.NewIDNum(3),
				Vectors: qdrant.NewVectors(0.36, 0.55, 0.47, 0.94),
				Payload: qdrant.NewValueMap(map[string]any{
					"agent": "alpha",
					"topic": "memory",
				}),
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to upsert points: %v", err)
	}
	fmt.Println("Upserted 3 vectors")

	// Query — find most similar to a test vector
	results, err := client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(0.1, 0.6, 0.8, 0.7),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		log.Fatalf("Failed to query: %v", err)
	}

	fmt.Println("\nQuery results:")
	for _, point := range results {
		fmt.Printf("  ID: %v | Score: %.4f | Payload: %v\n", point.Id, point.Score, point.Payload)
	}

	// Cleanup
	err = client.DeleteCollection(ctx, collectionName)
	if err != nil {
		log.Fatalf("Failed to cleanup: %v", err)
	}
	fmt.Println("\nCleaned up test collection")
	fmt.Println("ClawBrain stack verified — all systems go.")
}
