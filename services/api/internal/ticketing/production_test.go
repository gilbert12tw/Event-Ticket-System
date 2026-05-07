package ticketing

import (
	"strings"
	"testing"
	"time"
)

func TestValidEventTransitionRejectsIllegalRollback(t *testing.T) {
	if !validEventTransition(EventStatusDraft, EventStatusPublished) {
		t.Fatal("draft should publish")
	}
	if validEventTransition(EventStatusPublished, EventStatusDraft) {
		t.Fatal("published event must not roll back to draft")
	}
	if !validEventTransition(EventStatusClosed, EventStatusArchived) {
		t.Fatal("closed event should archive")
	}
}

func TestNormalizeTagsTrimsDeduplicatesAndDropsEmpty(t *testing.T) {
	got := normalizeTags([]string{" Family ", "", "family", "Engineering"})
	want := []string{"Family", "Engineering"}
	if len(got) != len(want) {
		t.Fatalf("tags = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tags[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if joined := joinTags(got); joined != "Family,Engineering" {
		t.Fatalf("joined tags = %q", joined)
	}
	if split := splitTags("Family, Engineering, family"); len(split) != 2 || split[0] != "Family" || split[1] != "Engineering" {
		t.Fatalf("split tags = %#v", split)
	}
}

func TestEffectiveAuditQueryDefaultsAndCapsLimit(t *testing.T) {
	if got := effectiveAuditQuery(nil); got.Limit != 100 {
		t.Fatalf("default limit = %d", got.Limit)
	}
	q := effectiveAuditQuery([]AuditLogQuery{{Limit: 1000, Cursor: time.Now()}})
	if q.Limit != 200 {
		t.Fatalf("capped limit = %d", q.Limit)
	}
}

func TestParseAuditCursorSupportsStableTieBreaker(t *testing.T) {
	cursorTime := time.Date(2026, 5, 7, 8, 9, 10, 123, time.UTC)
	gotTime, gotID := parseAuditCursor(cursorTime.Format(time.RFC3339Nano) + "|aud_2")
	if !gotTime.Equal(cursorTime) || gotID != "aud_2" {
		t.Fatalf("cursor = (%s, %q), want (%s, %q)", gotTime.Format(time.RFC3339Nano), gotID, cursorTime.Format(time.RFC3339Nano), "aud_2")
	}
}

func TestParseAuditCursorStrictRequiresStableTieBreaker(t *testing.T) {
	cursorTime := time.Date(2026, 5, 7, 8, 9, 10, 123, time.UTC)
	if _, _, err := ParseAuditCursorStrict(cursorTime.Format(time.RFC3339)); err == nil {
		t.Fatal("expected timestamp-only cursor to be rejected")
	}
	gotTime, gotID, err := ParseAuditCursorStrict(cursorTime.Format(time.RFC3339Nano) + "|aud_2")
	if err != nil {
		t.Fatal(err)
	}
	if !gotTime.Equal(cursorTime) || gotID != "aud_2" {
		t.Fatalf("cursor = (%s, %q), want (%s, %q)", gotTime.Format(time.RFC3339Nano), gotID, cursorTime.Format(time.RFC3339Nano), "aud_2")
	}
}

func TestRedactAuditMetadataMasksSensitiveValues(t *testing.T) {
	raw := `{"signed_token":"secret","employee_id":"E1001","nested":{"session":"cookie","full_name":"Ariel Chen"},"reason":"ok"}`
	got := redactAuditMetadata(raw)
	for _, leaked := range []string{"secret", "cookie", "Ariel Chen", "E1001"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("metadata leaked %q in %s", leaked, got)
		}
	}
	if !strings.Contains(got, `"reason":"ok"`) {
		t.Fatalf("metadata redacted non-sensitive value: %s", got)
	}
}
