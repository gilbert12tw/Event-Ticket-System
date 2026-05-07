package ticketing

import "testing"

func TestStringFromPayloadCoercesStringValues(t *testing.T) {
	got := stringFromPayload(map[string]interface{}{"employee_id": " E1001 "}, "employee_id")
	if got != "E1001" {
		t.Fatalf("employee id = %q", got)
	}
}

func TestNotificationPreferencesUsesNormalizedCategories(t *testing.T) {
	prefs := NotificationPreferences{OptedOutCategories: normalizeTags([]string{"Family", "family", "Sports"})}
	if joined := joinTags(prefs.OptedOutCategories); joined != "Family,Sports" {
		t.Fatalf("categories = %q", joined)
	}
}
