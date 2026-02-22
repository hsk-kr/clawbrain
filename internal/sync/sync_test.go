package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChunk_SmallText(t *testing.T) {
	text := "This is a short text."
	chunks := Chunk(text, 1600, 320)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("expected %q, got %q", text, chunks[0])
	}
}

func TestChunk_EmptyText(t *testing.T) {
	chunks := Chunk("", 1600, 320)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestChunk_WhitespaceOnly(t *testing.T) {
	chunks := Chunk("   \n\n  \t  ", 1600, 320)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for whitespace-only text, got %d", len(chunks))
	}
}

func TestChunk_ExactSize(t *testing.T) {
	// Text exactly at chunk size should return 1 chunk
	text := strings.Repeat("a", 1600)
	chunks := Chunk(text, 1600, 320)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestChunk_SplitsLargeText(t *testing.T) {
	// Build text with paragraph breaks
	paragraphs := make([]string, 10)
	for i := range paragraphs {
		paragraphs[i] = strings.Repeat("word ", 80) // ~400 chars each
	}
	text := strings.Join(paragraphs, "\n\n")

	chunks := Chunk(text, 1600, 320)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// All chunks should be non-empty
	for i, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			t.Errorf("chunk %d is empty", i)
		}
	}

	// No chunk should exceed size by more than a reasonable margin
	// (exact boundary might vary due to paragraph-aware splitting)
	for i, chunk := range chunks {
		if len(chunk) > 2000 { // generous margin
			t.Errorf("chunk %d is too large: %d chars", i, len(chunk))
		}
	}
}

func TestChunk_OverlapPresent(t *testing.T) {
	// Create text that will produce multiple chunks
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 100)
	chunks := Chunk(text, 400, 100)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// Check that consecutive chunks have overlapping content
	for i := 1; i < len(chunks); i++ {
		prev := chunks[i-1]
		curr := chunks[i]
		// The end of the previous chunk should share some content with
		// the beginning of the current chunk (overlap)
		prevEnd := prev[len(prev)/2:] // second half of prev
		if !hasOverlap(prevEnd, curr) {
			t.Errorf("chunks %d and %d appear to have no overlap", i-1, i)
		}
	}
}

func hasOverlap(a, b string) bool {
	// Check if any substring of reasonable length from a appears at the start of b
	minOverlap := 20
	if len(a) < minOverlap || len(b) < minOverlap {
		return true // too small to check meaningfully
	}
	for i := 0; i <= len(a)-minOverlap; i++ {
		sub := a[i : i+minOverlap]
		if strings.Contains(b[:min(len(b), len(a))], sub) {
			return true
		}
	}
	return false
}

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"  hello  world  ", "hello world"},
		{"line1\n\nline2", "line1\n\nline2"},     // paragraph break preserved
		{"line1\n\n\n\nline2", "line1\n\nline2"}, // excessive newlines collapsed to 2
		{"tabs\t\there", "tabs here"},            // tabs collapsed
		{"already normal", "already normal"},
		{"  ", ""},
		{"# Title\n\nParagraph", "# Title\n\nParagraph"}, // markdown structure preserved
		{"a\nb\nc", "a\nb\nc"},                           // single newlines preserved
	}
	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsMemoryMD(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"MEMORY.md", true},
		{"memory.md", true},
		{"Memory.md", true},
		{"/path/to/MEMORY.md", true},
		{"/path/to/memory.md", true},
		{"memory/2024-01-15.md", false},
		{"notes.md", false},
		{"MEMORY.txt", false},
	}
	for _, tt := range tests {
		got := IsMemoryMD(tt.path)
		if got != tt.want {
			t.Errorf("IsMemoryMD(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsTodayDailyFile(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	tests := []struct {
		path string
		want bool
	}{
		{today + ".md", true},
		{"memory/" + today + ".md", true},
		{yesterday + ".md", false},
		{"memory/" + yesterday + ".md", false},
		{"MEMORY.md", false},
		{"notes.md", false},
		{"2024-13-45.md", false}, // invalid date still matches regex, but not today
	}
	for _, tt := range tests {
		got := IsTodayDailyFile(tt.path)
		if got != tt.want {
			t.Errorf("IsTodayDailyFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestRedisKey(t *testing.T) {
	got := RedisKey("/workspace/MEMORY.md")
	want := "sync:/workspace/MEMORY.md"
	if got != want {
		t.Errorf("RedisKey() = %q, want %q", got, want)
	}
}

func TestDiscoverFiles_ExplicitFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.md")
	os.WriteFile(f, []byte("hello"), 0644)

	files, err := DiscoverFiles(dir, []string{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestDiscoverFiles_ExplicitDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "one.md"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "two.md"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "three.txt"), []byte("c"), 0644) // not .md

	files, err := DiscoverFiles(dir, nil, []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 .md files, got %d: %v", len(files), files)
	}
}

func TestDiscoverFiles_DefaultLayout(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("curated"), 0644)
	memDir := filepath.Join(dir, "memory")
	os.Mkdir(memDir, 0755)
	os.WriteFile(filepath.Join(memDir, "2024-01-15.md"), []byte("daily"), 0644)
	os.WriteFile(filepath.Join(memDir, "2024-01-16.md"), []byte("daily"), 0644)

	files, err := DiscoverFiles(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files (MEMORY.md + 2 daily), got %d: %v", len(files), files)
	}
}

func TestDiscoverFiles_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.md")
	os.WriteFile(f, []byte("hello"), 0644)

	// Pass the same file twice
	files, err := DiscoverFiles(dir, []string{f, f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file (deduped), got %d", len(files))
	}
}

func TestDiscoverFiles_MissingFilesSilentlySkipped(t *testing.T) {
	dir := t.TempDir()
	files, err := DiscoverFiles(dir, []string{"/nonexistent/file.md"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestContentHash(t *testing.T) {
	// Deterministic: same input always gives same hash
	h1 := ContentHash([]byte("hello world"))
	h2 := ContentHash([]byte("hello world"))
	if h1 != h2 {
		t.Errorf("expected same hash for same input, got %q and %q", h1, h2)
	}

	// Different input gives different hash
	h3 := ContentHash([]byte("goodbye world"))
	if h1 == h3 {
		t.Errorf("expected different hashes for different inputs")
	}

	// SHA-256 produces 64 hex characters
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex digest, got %d chars: %q", len(h1), h1)
	}

	// Empty content still produces a valid hash
	h4 := ContentHash([]byte(""))
	if len(h4) != 64 {
		t.Errorf("expected 64-char hex digest for empty input, got %d chars", len(h4))
	}
}

func TestLoadIgnorePatterns(t *testing.T) {
	t.Run("file exists with patterns", func(t *testing.T) {
		dir := t.TempDir()
		content := "*.log\n# comment\n\nmemory/scratch.md\n  \ntemp-*\n"
		os.WriteFile(filepath.Join(dir, ".clawbrain-ignore"), []byte(content), 0644)

		patterns := LoadIgnorePatterns(dir)
		expected := []string{"*.log", "memory/scratch.md", "temp-*"}
		if len(patterns) != len(expected) {
			t.Fatalf("expected %d patterns, got %d: %v", len(expected), len(patterns), patterns)
		}
		for i, p := range patterns {
			if p != expected[i] {
				t.Errorf("pattern[%d] = %q, want %q", i, p, expected[i])
			}
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		patterns := LoadIgnorePatterns(dir)
		if patterns != nil {
			t.Errorf("expected nil for missing file, got %v", patterns)
		}
	})

	t.Run("file is empty", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".clawbrain-ignore"), []byte(""), 0644)
		patterns := LoadIgnorePatterns(dir)
		if len(patterns) != 0 {
			t.Errorf("expected 0 patterns for empty file, got %d", len(patterns))
		}
	})

	t.Run("comments and blank lines only", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".clawbrain-ignore"), []byte("# comment\n\n# another\n"), 0644)
		patterns := LoadIgnorePatterns(dir)
		if len(patterns) != 0 {
			t.Errorf("expected 0 patterns, got %d: %v", len(patterns), patterns)
		}
	})
}

func TestIsIgnored(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		patterns []string
		want     bool
	}{
		{
			name:     "match base filename with wildcard",
			filePath: "/workspace/memory/notes.log",
			patterns: []string{"*.log"},
			want:     true,
		},
		{
			name:     "match exact base filename",
			filePath: "/workspace/scratch.md",
			patterns: []string{"scratch.md"},
			want:     true,
		},
		{
			name:     "no match",
			filePath: "/workspace/memory/notes.md",
			patterns: []string{"*.log", "scratch.md"},
			want:     false,
		},
		{
			name:     "match directory-relative pattern via suffix",
			filePath: "/workspace/memory/scratch.md",
			patterns: []string{"memory/scratch.md"},
			want:     true,
		},
		{
			name:     "directory pattern does not match wrong dir",
			filePath: "/workspace/other/scratch.md",
			patterns: []string{"memory/scratch.md"},
			want:     false,
		},
		{
			name:     "empty patterns",
			filePath: "/workspace/notes.md",
			patterns: nil,
			want:     false,
		},
		{
			name:     "multiple patterns first matches",
			filePath: "/workspace/temp-file.md",
			patterns: []string{"*.log", "temp-*"},
			want:     true,
		},
		{
			name:     "prefix pattern with wildcard",
			filePath: "/workspace/memory/2024-01-15.md",
			patterns: []string{"2024-*.md"},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsIgnored(tt.filePath, tt.patterns)
			if got != tt.want {
				t.Errorf("IsIgnored(%q, %v) = %v, want %v", tt.filePath, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestDiscoverFiles_EmptyResult(t *testing.T) {
	dir := t.TempDir()
	files, err := DiscoverFiles(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}
