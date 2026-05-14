package ticketing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringFromPayloadCoercesStringValues(t *testing.T) {
	got := stringFromPayload(map[string]interface{}{"employee_id": " E1001 "}, "employee_id")
	require.Equal(t, "E1001", got)
}

func TestNotificationPreferencesUsesNormalizedCategories(t *testing.T) {
	prefs := NotificationPreferences{OptedOutCategories: normalizeTags([]string{"Family", "family", "Sports"})}
	require.Equal(t, "Family,Sports", joinTags(prefs.OptedOutCategories))
}
