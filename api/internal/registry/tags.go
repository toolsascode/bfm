package registry

import (
	"fmt"
	"strings"
)

// ParseTagFilter parses API request tag strings (each key=value). Keys are normalized to
// lowercase; duplicate keys in the request use last-wins semantics.
func ParseTagFilter(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]string)
	for i, s := range raw {
		k, v, err := parseOneTag(s)
		if err != nil {
			return nil, fmt.Errorf("tags[%d]: %w", i, err)
		}
		out[k] = v
	}
	return out, nil
}

func parseOneTag(s string) (key, value string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", fmt.Errorf("empty tag")
	}
	eq := strings.Index(s, "=")
	if eq <= 0 {
		return "", "", fmt.Errorf("tag must be key=value")
	}
	k := strings.TrimSpace(s[:eq])
	val := strings.TrimSpace(s[eq+1:])
	if k == "" {
		return "", "", fmt.Errorf("empty key")
	}
	return strings.ToLower(k), val, nil
}

// TagMapFromScriptTags builds a map from migration-declared tags; malformed entries are skipped.
func TagMapFromScriptTags(raw []string) map[string]string {
	out := make(map[string]string)
	for _, s := range raw {
		k, v, err := parseOneTag(s)
		if err != nil {
			continue
		}
		out[k] = v
	}
	return out
}

// MatchesTagFilter reports whether migrationTags satisfies required (AND: every key=value in required must match).
func MatchesTagFilter(migrationTags []string, required map[string]string) bool {
	if len(required) == 0 {
		return true
	}
	m := TagMapFromScriptTags(migrationTags)
	if len(m) == 0 {
		return false
	}
	for k, v := range required {
		if m[k] != v {
			return false
		}
	}
	return true
}
