package ticketing

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignerRoundTripAndHash(t *testing.T) {
	signer := NewSigner("test-secret")
	claims := TicketClaims{
		TicketID:   "tkt_1",
		EventID:    "evt_1",
		EmployeeID: "E1001",
	}

	token, err := signer.Sign(claims)
	require.NoError(t, err)
	got, err := signer.Verify(token)
	require.NoError(t, err)
	assert.Equal(t, claims.TicketID, got.TicketID)
	assert.Equal(t, claims.EventID, got.EventID)
	assert.Equal(t, claims.EmployeeID, got.EmployeeID)
	hash := signer.HashToken(token)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, token, hash)
}

func TestSignerRejectsTampering(t *testing.T) {
	signer := NewSigner("test-secret")
	token, err := signer.Sign(TicketClaims{TicketID: "tkt_1", EventID: "evt_1", EmployeeID: "E1001"})
	require.NoError(t, err)

	tampered := strings.Replace(token, "E", "F", 1)
	if tampered == token {
		tampered = token + "x"
	}

	_, err = signer.Verify(tampered)
	require.Error(t, err, "expected tampered token error")
}

func TestSignerOfflinePackageRoundTrip(t *testing.T) {
	signer := NewSigner("test-secret")
	claims := OfflinePackageClaims{
		BatchID:    "off_1",
		EventID:    "evt_1",
		DeviceID:   "gate-1",
		StaffID:    "staff-1",
		ValidUntil: time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC),
	}

	token, err := signer.SignOfflinePackage(claims)
	require.NoError(t, err)
	got, err := signer.VerifyOfflinePackage(token)
	require.NoError(t, err)
	assert.Equal(t, claims.BatchID, got.BatchID)
	assert.Equal(t, claims.EventID, got.EventID)
	assert.Equal(t, claims.DeviceID, got.DeviceID)
	assert.Equal(t, claims.StaffID, got.StaffID)
	assert.True(t, got.ValidUntil.Equal(claims.ValidUntil), "ValidUntil mismatch: got %v want %v", got.ValidUntil, claims.ValidUntil)
}
