package ticketing

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func parseAuditLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func effectiveAuditQuery(query []AuditLogQuery) AuditLogQuery {
	if len(query) == 0 {
		return AuditLogQuery{Limit: 100}
	}
	query[0].Limit = parseAuditLimit(query[0].Limit)
	return query[0]
}

func appendAuditFilter(parts *[]string, args *[]interface{}, column string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	*args = append(*args, value)
	*parts = append(*parts, fmt.Sprintf("%s = $%d", column, len(*args)))
}

func appendAuditTimeFilter(parts *[]string, args *[]interface{}, column string, op string, value time.Time) {
	if value.IsZero() {
		return
	}
	*args = append(*args, value)
	*parts = append(*parts, fmt.Sprintf("%s %s $%d", column, op, len(*args)))
}

func parseAuditCursor(raw string) (time.Time, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, ""
	}
	if parsed, id, ok := parseDelimitedCursor(raw); ok {
		return parsed, id
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, ""
	}
	return parsed, ""
}

func parseDelimitedCursor(raw string) (time.Time, string, bool) {
	if strings.Contains(raw, "|") {
		parts := strings.SplitN(raw, "|", 2)
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(parts[0]))
		if err != nil {
			return time.Time{}, "", false
		}
		return parsed, strings.TrimSpace(parts[1]), true
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err == nil && strings.Contains(string(decoded), "|") {
		return parseDelimitedCursor(string(decoded))
	}
	return time.Time{}, "", false
}

func ParseAuditCursor(raw string) (time.Time, string) {
	return parseAuditCursor(raw)
}

func ParseAuditCursorStrict(raw string) (time.Time, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, "", nil
	}
	cursor, cursorID := parseAuditCursor(raw)
	if cursor.IsZero() || strings.TrimSpace(cursorID) == "" {
		return time.Time{}, "", fmt.Errorf("invalid audit cursor")
	}
	return cursor, cursorID, nil
}

func appendAuditCursorFilter(parts *[]string, args *[]interface{}, cursor time.Time, cursorID string) {
	if cursor.IsZero() {
		return
	}
	*args = append(*args, cursor)
	cursorIndex := len(*args)
	cursorID = strings.TrimSpace(cursorID)
	if cursorID == "" {
		*parts = append(*parts, fmt.Sprintf("created_at < $%d", cursorIndex))
		return
	}
	*args = append(*args, cursorID)
	*parts = append(*parts, fmt.Sprintf("(created_at < $%d OR (created_at = $%d AND audit_id < $%d))", cursorIndex, cursorIndex, len(*args)))
}

func redactAuditMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	var value interface{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return redactLooseAuditText(raw)
	}
	redacted, err := json.Marshal(redactAuditValue(value))
	if err != nil {
		return "[redacted audit metadata]"
	}
	return string(redacted)
}

func redactAuditValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case []interface{}:
		for i := range typed {
			typed[i] = redactAuditValue(typed[i])
		}
		return typed
	case map[string]interface{}:
		for key, raw := range typed {
			if sensitiveAuditKey(key) {
				typed[key] = "[redacted]"
				continue
			}
			typed[key] = redactAuditValue(raw)
		}
		return typed
	default:
		return value
	}
}

func sensitiveAuditKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{"signed_token", "qr_payload", "session", "token", "email", "full_name", "phone", "employee_id", "actor_id", "staff_id", "requested_by"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func redactLooseAuditText(raw string) string {
	lowered := strings.ToLower(raw)
	for _, marker := range []string{"signed_token", "qr_payload", "session", "token"} {
		if strings.Contains(lowered, marker) {
			return "[redacted audit metadata]"
		}
	}
	return raw
}
