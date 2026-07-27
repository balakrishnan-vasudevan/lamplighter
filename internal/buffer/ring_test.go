package buffer_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/buffer"
)

func ln(text string) buffer.Line {
	return buffer.Line{Timestamp: time.Now(), Text: text, Level: buffer.LevelInfo}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := buffer.New(5)
	if got := rb.Read(10); len(got) != 0 {
		t.Errorf("expected empty, got %d lines", len(got))
	}
}

func TestRingBuffer_BelowCapacity(t *testing.T) {
	rb := buffer.New(5)
	rb.Write(ln("a"))
	rb.Write(ln("b"))
	got := rb.Read(10)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Text != "a" || got[1].Text != "b" {
		t.Errorf("wrong order: %v %v", got[0].Text, got[1].Text)
	}
}

func TestRingBuffer_AtCapacity(t *testing.T) {
	rb := buffer.New(3)
	rb.Write(ln("a"))
	rb.Write(ln("b"))
	rb.Write(ln("c"))
	got := rb.Read(3)
	if len(got) != 3 || got[0].Text != "a" || got[2].Text != "c" {
		t.Errorf("unexpected: %v", got)
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := buffer.New(3)
	rb.Write(ln("a"))
	rb.Write(ln("b"))
	rb.Write(ln("c"))
	rb.Write(ln("d")) // overwrites "a"
	got := rb.Read(3)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0].Text != "b" || got[1].Text != "c" || got[2].Text != "d" {
		t.Errorf("expected b,c,d got %v,%v,%v", got[0].Text, got[1].Text, got[2].Text)
	}
}

func TestRingBuffer_ReadFewerThanStored(t *testing.T) {
	rb := buffer.New(5)
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		rb.Write(ln(s))
	}
	got := rb.Read(3)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	// last 3: c, d, e
	if got[0].Text != "c" || got[1].Text != "d" || got[2].Text != "e" {
		t.Errorf("expected c,d,e got %v,%v,%v", got[0].Text, got[1].Text, got[2].Text)
	}
}

func TestRingBuffer_MultipleOverflows(t *testing.T) {
	rb := buffer.New(3)
	for i := 0; i < 10; i++ {
		rb.Write(ln(string(rune('a' + i))))
	}
	got := rb.Read(3)
	// last 3 written: h, i, j
	if got[0].Text != "h" || got[1].Text != "i" || got[2].Text != "j" {
		t.Errorf("expected h,i,j got %v,%v,%v", got[0].Text, got[1].Text, got[2].Text)
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		text string
		want buffer.LogLevel
	}{
		{"2024-01-01 ERROR something failed", buffer.LevelError},
		{"fatal: connection refused", buffer.LevelError},
		{"CRITICAL: disk full", buffer.LevelError},
		{"WARN high memory usage", buffer.LevelWarn},
		{"INFO server started on :8080", buffer.LevelInfo},
		{"just a plain log line", buffer.LevelInfo},
	}
	for _, c := range cases {
		if got := buffer.ParseLevel(c.text); got != c.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestReadBefore(t *testing.T) {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	rb := buffer.New(10)
	for i := 0; i < 5; i++ {
		rb.Write(buffer.Line{Timestamp: base.Add(time.Duration(i) * time.Second), Text: fmt.Sprintf("%d", i)})
	}

	// All lines at or before T=base+4s
	got := rb.ReadBefore(base.Add(4*time.Second), 10)
	if len(got) != 5 {
		t.Fatalf("expected 5, got %d", len(got))
	}

	// Lines at or before T=base+2s → should return 0,1,2
	got = rb.ReadBefore(base.Add(2*time.Second), 10)
	if len(got) != 3 || got[0].Text != "0" || got[2].Text != "2" {
		t.Errorf("unexpected: %v", got)
	}

	// Limit n
	got = rb.ReadBefore(base.Add(4*time.Second), 2)
	if len(got) != 2 || got[0].Text != "3" || got[1].Text != "4" {
		t.Errorf("limit n: expected 3,4 got %v", got)
	}

	// Before all lines
	got = rb.ReadBefore(base.Add(-1*time.Second), 10)
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestRingBuffer_Len(t *testing.T) {
	rb := buffer.New(3)
	if rb.Len() != 0 {
		t.Errorf("expected 0")
	}
	rb.Write(ln("a"))
	rb.Write(ln("b"))
	if rb.Len() != 2 {
		t.Errorf("expected 2, got %d", rb.Len())
	}
	rb.Write(ln("c"))
	rb.Write(ln("d"))
	if rb.Len() != 3 {
		t.Errorf("expected 3 (capped), got %d", rb.Len())
	}
}
