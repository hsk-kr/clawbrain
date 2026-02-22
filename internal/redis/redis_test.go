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

func TestConnectionError(t *testing.T) {
	// Connect to a port that's definitely not Redis
	_, err := New("localhost", 1)
	if err == nil {
		t.Fatal("expected connection error to port 1")
	}
}
