package ticketing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactNotificationDeliveryErrorRemovesRecipientPII(t *testing.T) {
	got := redactNotificationDeliveryError("  550 rejected e1001@cets.local for E1001 with token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJtYWlsIn0.signature  ", "E1001")

	assert.Equal(t, "550 rejected [redacted email] for E100**** with token [redacted token]", got)
}

func TestRedactNotificationDeliveryErrorRemovesEmailBodyFragments(t *testing.T) {
	got := redactNotificationDeliveryError("smtp rejected message_body=Private venue body for E1001; retry later", "E1001")

	assert.Equal(t, "smtp rejected message_body=[redacted email body]", got)
	assert.NotContains(t, got, "Private venue body")
	assert.NotContains(t, got, "E1001")
}

func TestRedactNotificationDeliveryErrorHandlesEmptyContext(t *testing.T) {
	assert.Empty(t, redactNotificationDeliveryError(" ", "E1001"))
	assert.Equal(t, "smtp unavailable", redactNotificationDeliveryError("smtp unavailable", ""))
}
