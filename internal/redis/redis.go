// Package redis provides a minimal Redis client using the RESP protocol.
// It supports only the commands needed by ClawBrain's sync feature:
// SET, EXISTS, and SET with EX (TTL). No external dependencies.
package redis

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"time"
)

// Client is a minimal Redis client.
type Client struct {
	conn net.Conn
	rd   *bufio.Reader
}

// New connects to a Redis server and returns a Client.
func New(host string, port int) (*Client, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to redis at %s: %w", addr, err)
	}
	return &Client{conn: conn, rd: bufio.NewReader(conn)}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Ping checks connectivity by sending a PING command.
func (c *Client) Ping() error {
	if err := c.sendCommand("PING"); err != nil {
		return err
	}
	_, err := c.readLine()
	return err
}

// Set stores a key with a value and no expiry.
func (c *Client) Set(key, value string) error {
	if err := c.sendCommand("SET", key, value); err != nil {
		return err
	}
	_, err := c.readLine()
	return err
}

// SetWithTTL stores a key with a value and a TTL in seconds.
func (c *Client) SetWithTTL(key, value string, ttlSeconds int) error {
	if err := c.sendCommand("SET", key, value, "EX", strconv.Itoa(ttlSeconds)); err != nil {
		return err
	}
	_, err := c.readLine()
	return err
}

// Exists returns true if the key exists in Redis.
func (c *Client) Exists(key string) (bool, error) {
	if err := c.sendCommand("EXISTS", key); err != nil {
		return false, err
	}
	line, err := c.readLine()
	if err != nil {
		return false, err
	}
	// RESP integer reply: ":1\r\n" or ":0\r\n"
	if len(line) >= 2 && line[0] == ':' {
		return line[1] == '1', nil
	}
	return false, fmt.Errorf("unexpected EXISTS reply: %q", line)
}

// sendCommand writes a RESP array command to the connection.
func (c *Client) sendCommand(args ...string) error {
	// RESP array: *<count>\r\n followed by $<len>\r\n<data>\r\n for each arg
	buf := fmt.Sprintf("*%d\r\n", len(args))
	for _, arg := range args {
		buf += fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg)
	}
	_, err := c.conn.Write([]byte(buf))
	return err
}

// readLine reads a single RESP line from the connection.
func (c *Client) readLine() (string, error) {
	line, err := c.rd.ReadString('\n')
	if err != nil {
		return "", err
	}
	// Strip trailing \r\n
	if len(line) >= 2 && line[len(line)-2] == '\r' {
		line = line[:len(line)-2]
	}
	// Check for RESP errors
	if len(line) > 0 && line[0] == '-' {
		return "", fmt.Errorf("redis error: %s", line[1:])
	}
	return line, nil
}
