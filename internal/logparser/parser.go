package logparser

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/buffer"
)

// ParseLine parses a raw log line (with optional k8s RFC3339 timestamp prefix)
// into a buffer.Line. JSON logs are detected automatically.
func ParseLine(raw string) buffer.Line {
	ts, text := splitK8sTimestamp(raw)

	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		if line, ok := parseJSON(text, ts); ok {
			line.RawText = raw
			return line
		}
	}

	return buffer.Line{
		Timestamp: ts,
		Text:      text,
		RawText:   raw,
		Level:     buffer.ParseLevel(text),
	}
}

// IsContinuation returns true if the line is a continuation of the previous
// log entry (stack trace line, wrapped output, etc.).
func IsContinuation(raw string) bool {
	if len(raw) == 0 {
		return false
	}
	// k8s-prefixed lines start with a digit (RFC3339 year)
	if raw[0] >= '0' && raw[0] <= '9' {
		return false
	}
	return raw[0] == '\t' || raw[0] == ' '
}

// splitK8sTimestamp splits the optional k8s RFC3339 timestamp prefix from the text.
func splitK8sTimestamp(line string) (time.Time, string) {
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 2 {
		if ts, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
			return ts, parts[1]
		}
	}
	return time.Now(), line
}

var (
	levelFields = []string{"level", "severity", "lvl"}
	msgFields   = []string{"msg", "message", "text", "body"}
	tsFields    = []string{"time", "timestamp", "ts", "@timestamp"}
	traceFields = []string{"traceId", "trace_id", "traceid", "requestId", "request_id", "reqId", "spanId", "span_id", "X-Request-ID"}
)

func parseJSON(text string, fallbackTs time.Time) (buffer.Line, bool) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return buffer.Line{}, false
	}

	level := extractStr(raw, levelFields)
	msg := extractStr(raw, msgFields)
	tsStr := extractStr(raw, tsFields)

	ts := fallbackTs
	if tsStr != "" {
		ts = parseTS(tsStr, fallbackTs)
	} else if f, ok := extractFloat(raw, tsFields); ok {
		sec := int64(f)
		ts = time.Unix(sec, int64((f-float64(sec))*1e9))
	}

	var parts []string
	if msg != "" {
		parts = append(parts, msg)
	}
	for _, f := range traceFields {
		if v := extractStr(raw, []string{f}); v != "" {
			parts = append(parts, f+"="+v)
			break
		}
	}
	if len(parts) == 0 {
		return buffer.Line{}, false
	}

	// Remaining fields (exclude standard ones already consumed)
	used := usedSet(levelFields, msgFields, tsFields, traceFields)
	fields := make(map[string]string, len(raw))
	for k, v := range raw {
		if used[k] {
			continue
		}
		switch val := v.(type) {
		case string:
			fields[k] = val
		case float64:
			fields[k] = strconv.FormatFloat(val, 'f', -1, 64)
		default:
			b, _ := json.Marshal(v)
			fields[k] = string(b)
		}
	}

	return buffer.Line{
		Timestamp: ts,
		Text:      strings.Join(parts, " · "),
		Level:     buffer.ParseLevelString(level),
		Fields:    fields,
	}, true
}

func extractStr(m map[string]interface{}, keys []string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func extractFloat(m map[string]interface{}, keys []string) (float64, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if f, ok := v.(float64); ok {
				return f, true
			}
		}
	}
	return 0, false
}

func parseTS(s string, fallback time.Time) time.Time {
	for _, f := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return fallback
}

func usedSet(lists ...[]string) map[string]bool {
	m := make(map[string]bool)
	for _, list := range lists {
		for _, f := range list {
			m[f] = true
		}
	}
	return m
}
