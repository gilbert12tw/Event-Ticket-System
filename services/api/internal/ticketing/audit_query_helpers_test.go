package ticketing

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAuditCursorHandlesSupportedFormats(t *testing.T) {
	cursorTime := time.Date(2026, 5, 21, 9, 10, 11, 123000000, time.UTC)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(cursorTime.Format(time.RFC3339Nano) + "| aud_9 "))

	tests := []struct {
		name     string
		raw      string
		wantTime time.Time
		wantID   string
	}{
		{name: "blank"},
		{name: "timestamp", raw: cursorTime.Format(time.RFC3339), wantTime: cursorTime.Truncate(time.Second)},
		{name: "timestamp with id", raw: " " + cursorTime.Format(time.RFC3339Nano) + " | aud_7 ", wantTime: cursorTime, wantID: "aud_7"},
		{name: "encoded timestamp with id", raw: encoded, wantTime: cursorTime, wantID: "aud_9"},
		{name: "invalid", raw: "not-a-cursor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTime, gotID := ParseAuditCursor(tt.raw)

			assert.True(t, gotTime.Equal(tt.wantTime), "time mismatch: got %s want %s", gotTime, tt.wantTime)
			assert.Equal(t, tt.wantID, gotID)
		})
	}
}

func TestParseAuditCursorStrictAllowsBlankAndRequiresID(t *testing.T) {
	cursorTime := time.Date(2026, 5, 21, 9, 10, 11, 123000000, time.UTC)

	gotTime, gotID, err := ParseAuditCursorStrict(" ")
	require.NoError(t, err)
	assert.True(t, gotTime.IsZero())
	assert.Empty(t, gotID)

	_, _, err = ParseAuditCursorStrict(cursorTime.Format(time.RFC3339Nano))
	require.Error(t, err)

	gotTime, gotID, err = ParseAuditCursorStrict(cursorTime.Format(time.RFC3339Nano) + "|aud_2")
	require.NoError(t, err)
	assert.True(t, gotTime.Equal(cursorTime))
	assert.Equal(t, "aud_2", gotID)
}

func TestAppendAuditFiltersBuildStableSQLFragments(t *testing.T) {
	parts := []string{"true"}
	args := []interface{}{"existing"}
	cursorTime := time.Date(2026, 5, 21, 9, 10, 11, 0, time.UTC)

	appendAuditFilter(&parts, &args, "actor_id", " hr-1 ")
	appendAuditFilter(&parts, &args, "role", " ")
	appendAuditTimeFilter(&parts, &args, "created_at", ">=", cursorTime)
	appendAuditTimeFilter(&parts, &args, "created_at", "<=", time.Time{})
	appendAuditCursorFilter(&parts, &args, cursorTime, " aud_8 ")

	require.Equal(t, []string{
		"true",
		"actor_id = $2",
		"created_at >= $3",
		"(created_at < $4 OR (created_at = $4 AND audit_id < $5))",
	}, parts)
	require.Len(t, args, 5)
	assert.Equal(t, "hr-1", args[1])
	assert.Equal(t, cursorTime, args[2])
	assert.Equal(t, cursorTime, args[3])
	assert.Equal(t, "aud_8", args[4])
}

func TestAppendAuditCursorFilterSupportsTimestampOnlyCursor(t *testing.T) {
	parts := []string{}
	args := []interface{}{}
	cursorTime := time.Date(2026, 5, 21, 9, 10, 11, 0, time.UTC)

	appendAuditCursorFilter(&parts, &args, time.Time{}, "aud_ignored")
	appendAuditCursorFilter(&parts, &args, cursorTime, " ")

	assert.Equal(t, []string{"created_at < $1"}, parts)
	assert.Equal(t, []interface{}{cursorTime}, args)
}

func TestRedactAuditMetadataHandlesLooseTextAndArrays(t *testing.T) {
	assert.Equal(t, "[redacted audit metadata]", redactAuditMetadata("token=secret"))
	assert.Equal(t, "plain operational note", redactAuditMetadata("plain operational note"))

	got := redactAuditMetadata(`[{"phone":"0912","reason":"ok"}]`)
	assert.NotContains(t, got, "0912")
	assert.Contains(t, got, `"phone":"[redacted]"`)
	assert.Contains(t, got, `"reason":"ok"`)
}
