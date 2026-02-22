// Package sync provides file-to-memory synchronization for ClawBrain.
// It reads markdown files, chunks them, and adds new content to ClawBrain
// while skipping already-ingested content tracked via Redis.
package sync

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Default chunking parameters (character-based approximation of tokens).
// ~1600 chars ≈ ~400 tokens, ~320 chars ≈ ~80 tokens overlap.
const (
	DefaultChunkSize    = 1600
	DefaultChunkOverlap = 320
)

// redisKeyPrefix is prepended to all sync tracking keys in Redis.
const redisKeyPrefix = "sync:"

// memoryMDTTL is the TTL for MEMORY.md entries in Redis (7 days).
const memoryMDTTL = 7 * 24 * 60 * 60 // 604800 seconds

// ContentHash returns the SHA-256 hex digest of the given content.
// Used to detect whether a file has changed since last sync.
func ContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("%x", h)
}

// datePattern matches filenames containing YYYY-MM-DD.
var datePattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// FileResult holds the sync outcome for a single file.
type FileResult struct {
	File    string `json:"file"`
	Added   int    `json:"added"`
	Skipped int    `json:"skipped"`
	Reason  string `json:"reason,omitempty"`
}

// Chunk splits text into overlapping chunks of approximately the given size.
// It prefers splitting at paragraph boundaries (double newline), then
// sentence boundaries, then falls back to hard character splits.
func Chunk(text string, size, overlap int) []string {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return nil
	}
	if len(text) <= size {
		return []string{text}
	}

	var chunks []string
	start := 0
	for start < len(text) {
		prevStart := start
		end := start + size
		if end >= len(text) {
			chunks = append(chunks, strings.TrimSpace(text[start:]))
			break
		}

		// Try to find a paragraph boundary (double newline) near the end
		splitAt := findSplit(text, start, end, "\n\n")
		if splitAt == -1 {
			// Try sentence boundary (. followed by space or newline)
			splitAt = findSentenceSplit(text, start, end)
		}
		if splitAt == -1 {
			// Try single newline
			splitAt = findSplit(text, start, end, "\n")
		}
		if splitAt == -1 {
			// Hard split at size
			splitAt = end
		}

		chunk := strings.TrimSpace(text[start:splitAt])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}

		// Next chunk starts with overlap from the end of the current chunk
		start = splitAt - overlap
		if start < 0 {
			start = 0
		}
		// Prevent infinite loop: always advance past the previous start
		if start <= prevStart {
			start = prevStart + size
		}
	}

	return chunks
}

// findSplit looks for the last occurrence of sep in text[start:end],
// preferring a split in the last 25% of the window to maximize chunk size.
// Returns the position right after the separator, or -1 if not found.
func findSplit(text string, start, end int, sep string) int {
	// Search in the last 25% of the chunk for a natural break
	searchFrom := start + (end-start)*3/4
	window := text[searchFrom:end]
	idx := strings.LastIndex(window, sep)
	if idx != -1 {
		return searchFrom + idx + len(sep)
	}
	return -1
}

// findSentenceSplit looks for the last sentence-ending punctuation
// followed by whitespace in the last 25% of the chunk.
func findSentenceSplit(text string, start, end int) int {
	searchFrom := start + (end-start)*3/4
	window := text[searchFrom:end]
	// Look for ". " or ".\n" or "! " or "? " patterns
	best := -1
	for i := len(window) - 2; i >= 0; i-- {
		if (window[i] == '.' || window[i] == '!' || window[i] == '?') &&
			i+1 < len(window) && (window[i+1] == ' ' || window[i+1] == '\n') {
			best = searchFrom + i + 1
			break
		}
	}
	return best
}

// NormalizeText normalizes text for consistent comparison:
// trims outer whitespace and collapses runs of 3+ newlines into 2
// (preserving paragraph breaks). Collapses runs of spaces/tabs on
// the same line into a single space. Newlines are preserved so that
// markdown structure (headings, paragraphs) is not lost -- this
// matters for embedding quality.
func NormalizeText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Collapse runs of 3+ newlines into double newline (paragraph break)
	var b strings.Builder
	newlineRun := 0
	spaceRun := false
	for _, r := range s {
		if r == '\n' {
			newlineRun++
			spaceRun = false
			if newlineRun <= 2 {
				b.WriteRune('\n')
			}
		} else if r == ' ' || r == '\t' {
			newlineRun = 0
			if !spaceRun {
				b.WriteRune(' ')
				spaceRun = true
			}
		} else {
			newlineRun = 0
			spaceRun = false
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// RedisKey returns the Redis key for tracking a file's sync state.
func RedisKey(filePath string) string {
	return redisKeyPrefix + filePath
}

// MemoryMDTTLSeconds returns the TTL in seconds for MEMORY.md entries.
func MemoryMDTTLSeconds() int {
	return memoryMDTTL
}

// IsMemoryMD returns true if the filename (case-insensitive) is memory.md.
func IsMemoryMD(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.EqualFold(base, "memory.md")
}

// IsTodayDailyFile returns true if the filename contains today's date (YYYY-MM-DD).
// Today's daily file is still being written and should be skipped.
func IsTodayDailyFile(filePath string) bool {
	base := filepath.Base(filePath)
	match := datePattern.FindString(base)
	if match == "" {
		return false
	}
	today := time.Now().Format("2006-01-02")
	return match == today
}

// LoadIgnorePatterns reads a .clawbrain-ignore file and returns the patterns.
// Returns nil (no error) if the file does not exist.
// Lines starting with # are comments. Empty lines are skipped.
func LoadIgnorePatterns(basePath string) []string {
	ignoreFile := filepath.Join(basePath, ".clawbrain-ignore")
	data, err := os.ReadFile(ignoreFile)
	if err != nil {
		return nil
	}
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// IsIgnored checks whether a file path matches any of the ignore patterns.
// Patterns are matched against the base filename, the full path, and —
// for patterns containing path separators — the path suffix. This handles
// relative patterns like "memory/scratch.md" matching absolute paths like
// "/workspace/memory/scratch.md".
func IsIgnored(filePath string, patterns []string) bool {
	base := filepath.Base(filePath)
	for _, pattern := range patterns {
		// Match against base filename (handles "*.md", "scratch.md")
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Match against full path (handles absolute patterns)
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return true
		}
		// For patterns with path separators, try suffix matching.
		// This allows "memory/scratch.md" to match "/workspace/memory/scratch.md".
		if strings.Contains(pattern, string(filepath.Separator)) {
			// Ensure we match at a path boundary by prepending separator
			suffix := string(filepath.Separator) + pattern
			if strings.HasSuffix(filePath, suffix) {
				return true
			}
		}
	}
	return false
}

// DiscoverFiles finds markdown files to sync based on explicit paths and/or
// the default agent memory layout. Returns a deduplicated list of absolute paths.
func DiscoverFiles(basePath string, files []string, dirs []string) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	addFile := func(path string) error {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if seen[abs] {
			return nil
		}
		// Verify it exists and is a file
		info, err := os.Stat(abs)
		if err != nil {
			// Skip missing files silently
			return nil
		}
		if info.IsDir() {
			return nil
		}
		seen[abs] = true
		result = append(result, abs)
		return nil
	}

	// Explicit files
	for _, f := range files {
		if err := addFile(f); err != nil {
			return nil, err
		}
	}

	// Explicit directories: glob for *.md
	for _, d := range dirs {
		matches, err := filepath.Glob(filepath.Join(d, "*.md"))
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", d, err)
		}
		for _, m := range matches {
			if err := addFile(m); err != nil {
				return nil, err
			}
		}
	}

	// Default discovery if no explicit paths given
	if len(files) == 0 && len(dirs) == 0 {
		// Look for MEMORY.md in basePath. Try the canonical name first,
		// then the lowercase variant. Only add the first one found to
		// avoid duplicates on case-insensitive filesystems (macOS).
		for _, name := range []string{"MEMORY.md", "memory.md"} {
			p := filepath.Join(basePath, name)
			if _, err := os.Stat(p); err == nil {
				if err := addFile(p); err != nil {
					return nil, err
				}
				break // only add the first match
			}
		}
		// Look for memory/*.md
		memDir := filepath.Join(basePath, "memory")
		if info, err := os.Stat(memDir); err == nil && info.IsDir() {
			matches, err := filepath.Glob(filepath.Join(memDir, "*.md"))
			if err != nil {
				return nil, fmt.Errorf("glob memory dir: %w", err)
			}
			for _, m := range matches {
				if err := addFile(m); err != nil {
					return nil, err
				}
			}
		}
	}

	return result, nil
}
