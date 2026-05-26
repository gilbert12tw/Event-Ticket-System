package ticketing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactNotificationDeliveryErrorRemovesRecipientPII(t *testing.T) {
	got := redactNotificationDeliveryError("  550 rejected e1001@cets.local for E1001  ", "E1001")

	assert.Equal(t, "550 rejected [redacted email] for E100****", got)
}

func TestRedactNotificationDeliveryErrorHandlesEmptyContext(t *testing.T) {
	assert.Empty(t, redactNotificationDeliveryError(" ", "E1001"))
	assert.Equal(t, "smtp unavailable", redactNotificationDeliveryError("smtp unavailable", ""))
}
