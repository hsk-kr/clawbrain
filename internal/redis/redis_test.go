package redis

import (
	"net"
	"testing"
	"time"
)

func skipIfNoRedis(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "localhost:6379", 2*time.Second)
	if err != nil {
		t.Skipf("Redis not available on localhost:6379, skipping: %v", err)
	}
	conn.Close()
}

func TestPing(t *testing.T) {
	skipIfNoRedis(t)
	c, err := New("localhost", 6379)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestSetAndExists(t *testing.T) {
	skipIfNoRedis(t)
	c, err := New("localhost", 6379)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	key := "clawbrain_test:set_exists"

	// Clean up
	c.sendCommand("DEL", key)
	c.readLine()

	// Key should not exist
	exists, err := c.Exists(key)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected key to not exist before SET")
	}

	// Set it
	if err := c.Set(key, "1"); err != nil {
		t.Fatal(err)
	}

	// Should exist now
	exists, err = c.Exists(key)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected key to exist after SET")
	}

	// Clean up
	c.sendCommand("DEL", key)
	c.readLine()
}

func TestSetWithTTL(t *testing.T) {
	skipIfNoRedis(t)
	c, err := New("localhost", 6379)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	key := "clawbrain_test:set_ttl"

	// Clean up
	c.sendCommand("DEL", key)
	c.readLine()

	// Set with 2-second TTL
	if err := c.SetWithTTL(key, "1", 2); err != nil {
		t.Fatal(err)
	}

	// Should exist immediately
	exists, err := c.Exists(key)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected key to exist immediately after SetWithTTL")
	}

	// Wait for TTL to expire
	time.Sleep(3 * time.Second)

	// Should be gone
	exists, err = c.Exists(key)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected key to expire after TTL")
	}
}

func TestGetAndSet(t *testing.T) {
	skipIfNoRedis(t)
	c, err := New("localhost", 6379)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	key := "clawbrain_test:get_set"

	// Clean up
	c.sendCommand("DEL", key)
	c.readLine()

	// Get non-existent key
	val, found, err := c.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected key to not be found before SET")
	}
	if val != "" {
		t.Fatalf("expected empty value for missing key, got %q", val)
	}

	// Set it
	if err := c.Set(key, "abc123"); err != nil {
		t.Fatal(err)
	}

	// Get should return the value
	val, found, err = c.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected key to be found after SET")
	}
	if val != "abc123" {
		t.Fatalf("expected value %q, got %q", "abc123", val)
	}

	// Update value
	if err := c.Set(key, "xyz789"); err != nil {
		t.Fatal(err)
	}

	val, found, err = c.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || val != "xyz789" {
		t.Fatalf("expected updated value %q, got found=%v val=%q", "xyz789", found, val)
	}

	// Clean up
	c.sendCommand("DEL", key)
	c.readLine()
}

func TestConnectionError(t *testing.T) {
	// Connect to a port that's definitely not Redis
	_, err := New("localhost", 1)
	if err == nil {
		t.Fatal("expected connection error to port 1")
	}
}
