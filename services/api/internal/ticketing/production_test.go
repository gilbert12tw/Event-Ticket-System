package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidEventTransitionRejectsIllegalRollback(t *testing.T) {
	assert.True(t, validEventTransition(EventStatusDraft, EventStatusPublished), "draft should publish")
	assert.False(t, validEventTransition(EventStatusPublished, EventStatusDraft), "published event must not roll back to draft")
	assert.True(t, validEventTransition(EventStatusClosed, EventStatusArchived), "closed event should archive")
}

func TestNormalizeTagsTrimsDeduplicatesAndDropsEmpty(t *testing.T) {
	got := normalizeTags([]string{" Family ", "", "family", "Engineering"})
	want := []string{"Family", "Engineering"}
	require.Equal(t, want, got)
	assert.Equal(t, "Family,Engineering", joinTags(got))
	split := splitTags("Family, Engineering, family")
	require.Len(t, split, 2)
	assert.Equal(t, "Family", split[0])
	assert.Equal(t, "Engineering", split[1])
}

func TestEffectiveAuditQueryDefaultsAndCapsLimit(t *testing.T) {
	assert.Equal(t, 100, effectiveAuditQuery(nil).Limit)
	q := effectiveAuditQuery([]AuditLogQuery{{Limit: 1000, Cursor: time.Now()}})
	assert.Equal(t, 200, q.Limit)
}

func TestParseAuditCursorSupportsStableTieBreaker(t *testing.T) {
	cursorTime := time.Date(2026, 5, 7, 8, 9, 10, 123, time.UTC)
	gotTime, gotID := parseAuditCursor(cursorTime.Format(time.RFC3339Nano) + "|aud_2")
	assert.True(t, gotTime.Equal(cursorTime))
	assert.Equal(t, "aud_2", gotID)
}

func TestParseAuditCursorStrictRequiresStableTieBreaker(t *testing.T) {
	cursorTime := time.Date(2026, 5, 7, 8, 9, 10, 123, time.UTC)
	_, _, err := ParseAuditCursorStrict(cursorTime.Format(time.RFC3339))
	require.Error(t, err, "expected timestamp-only cursor to be rejected")
	gotTime, gotID, err := ParseAuditCursorStrict(cursorTime.Format(time.RFC3339Nano) + "|aud_2")
	require.NoError(t, err)
	assert.True(t, gotTime.Equal(cursorTime))
	assert.Equal(t, "aud_2", gotID)
}

func TestRedactAuditMetadataMasksSensitiveValues(t *testing.T) {
	raw := `{"signed_token":"secret","employee_id":"E1001","nested":{"session":"cookie","full_name":"Ariel Chen"},"reason":"ok"}`
	got := redactAuditMetadata(raw)
	for _, leaked := range []string{"secret", "cookie", "Ariel Chen", "E1001"} {
		assert.NotContains(t, got, leaked, "metadata leaked %q", leaked)
	}
	assert.Contains(t, got, `"reason":"ok"`, "metadata redacted non-sensitive value")
}
