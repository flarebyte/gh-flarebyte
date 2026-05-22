// purpose: Provide low-level value extraction helpers for the CUE-like config parser.
// responsibilities: Read object/string/bool/number/list fields and repository labels from raw config text.
// architecture notes: Helpers intentionally use deterministic string/regex parsing to avoid a heavy parser dependency.
package config

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func extractOptionalObjectBlock(raw, field string) (string, bool) {
	block, err := extractObjectBlock(raw, field)
	if err != nil {
		return "", false
	}
	return block, true
}

func extractObjectBlock(raw, field string) (string, error) {
	open := strings.Index(raw, field+":")
	if open == -1 {
		return "", fmt.Errorf("missing required object field %s", field)
	}
	brace := strings.Index(raw[open:], "{")
	if brace == -1 {
		return "", fmt.Errorf("malformed object field %s", field)
	}
	start := open + brace + 1
	end, ok := findMatchingBrace(raw, start-1)
	if !ok || end <= start {
		return "", fmt.Errorf("malformed object field %s", field)
	}
	return raw[start:end], nil
}

func extractStringField(raw, field string) (string, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*"([^"]+)"`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return "", fmt.Errorf("missing required string field %s", field)
	}
	return m[1], nil
}

func extractOptionalStringField(raw, field string) string {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*"([^"]+)"`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func extractBoolField(raw, field string) (bool, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*(true|false)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return false, fmt.Errorf("missing required bool field %s", field)
	}
	return m[1] == "true", nil
}

func extractOptionalBoolField(raw, field string, fallback bool) bool {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*(true|false)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return fallback
	}
	return m[1] == "true"
}

func extractOptionalBoolFieldRaw(raw, field string) (string, bool) {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*(true|false)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

func extractOptionalNumberPointerField(raw, field string) *float64 {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*([0-9]+(?:\.[0-9]+)?)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return nil
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	return &v
}

func extractOptionalBoolFieldWithPresence(raw, field string) (bool, bool) {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*(true|false)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return false, false
	}
	return m[1] == "true", true
}

func extractStringListBlock(raw, field string) ([]string, error) {
	open := strings.Index(raw, field+": [")
	if open == -1 {
		return nil, fmt.Errorf("missing required list field %s", field)
	}
	start := open + len(field+": [")
	end := strings.Index(raw[start:], "]")
	if end == -1 {
		return nil, fmt.Errorf("malformed list field %s", field)
	}
	block := raw[start : start+end]
	matches := quotedStringPattern.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("list field %s has no entries", field)
	}
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values, nil
}

func extractOptionalStringListBlock(raw, field string) []string {
	open := strings.Index(raw, field+": [")
	if open == -1 {
		return nil
	}
	start := open + len(field+": [")
	end := strings.Index(raw[start:], "]")
	if end == -1 {
		return nil
	}
	block := raw[start : start+end]
	matches := quotedStringPattern.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		return nil
	}
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values
}

func extractLabels(raw string) ([]LabelConfig, error) {
	open := strings.Index(raw, "labels: [")
	if open == -1 {
		return nil, errors.New("missing required list field labels")
	}
	start := open + len("labels: [")
	end := strings.Index(raw[start:], "]")
	if end == -1 {
		return nil, errors.New("malformed list field labels")
	}
	block := raw[start : start+end]
	matches := labelObjectPattern.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		return nil, errors.New("list field labels has no entries")
	}
	labels := make([]LabelConfig, 0, len(matches))
	for _, match := range matches {
		item := match[1]
		if err := assertOnlyAllowedFields("repository.labels item", item, map[string]struct{}{
			"name":        {},
			"color":       {},
			"description": {},
		}); err != nil {
			return nil, err
		}
		name, err := extractStringField(item, "name")
		if err != nil {
			return nil, fmt.Errorf("invalid repository.labels entry: %w", err)
		}
		color, err := extractStringField(item, "color")
		if err != nil {
			return nil, fmt.Errorf("invalid repository.labels entry: %w", err)
		}
		description := extractOptionalStringField(item, "description")
		labels = append(labels, LabelConfig{
			Name:        name,
			Color:       color,
			Description: description,
		})
	}
	return labels, nil
}
