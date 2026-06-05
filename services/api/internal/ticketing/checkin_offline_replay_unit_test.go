package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfflineReplayRowResultCoversAcceptedDuplicateAndConflictBranches(t *testing.T) {
	scannedAt := time.Date(2026, 6, 3, 10, 30, 0, 123456000, time.UTC)
	req := OfflineCheckinSyncRequest{EventID: "evt-fallback"}
	acceptedCheckin := CheckinResponse{
		CheckinID:   "chk-1",
		TicketID:    "tic-1",
		EventID:     "evt-1",
		EmployeeID:  "E1001",
		Status:      "checked_in",
		ReasonCode:  "stale",
		Duplicate:   true,
		ScannedAt:   scannedAt,
		Holder:      TicketHolder{DisplayName: "Ada Lovelace", Department: "Engineering", City: "Taipei"},
		FamilyCount: 1,
	}

	accepted, err := offlineReplayRow{
		ticketID:     "tic-1",
		status:       offlineScanStatusAccepted,
		checkin:      acceptedCheckin,
		checkinFound: true,
	}.result(req, TicketClaims{})
	require.NoError(t, err)
	assert.Equal(t, "accepted", accepted.ReasonCode)
	assert.False(t, accepted.Duplicate)
	assert.Equal(t, "chk-1", accepted.CheckinID)

	firstScannedAt := scannedAt.Add(-time.Minute)
	duplicate, err := offlineReplayRow{
		ticketID:       "tic-1",
		status:         offlineScanStatusDuplicate,
		checkin:        acceptedCheckin,
		checkinFound:   true,
		firstScannedBy: "staff-1",
		firstScannedAt: firstScannedAt,
	}.result(req, TicketClaims{})
	require.NoError(t, err)
	assert.True(t, duplicate.Duplicate)
	assert.Equal(t, "duplicate_scan", duplicate.ReasonCode)
	assert.Equal(t, offlineConflictRedeemed, duplicate.ConflictReason)
	assert.Equal(t, "staff-1", duplicate.FirstScannedBy)
	assert.Equal(t, firstScannedAt, duplicate.FirstScannedAt)

	conflict, err := offlineReplayRow{
		ticketID:       "tic-2",
		status:         offlineScanStatusConflict,
		conflictReason: offlineConflictExpired,
		scannedAt:      scannedAt,
		ticketFound:    true,
		ticketEventID:  "evt-2",
		ticketEmployee: "E1002",
		ticketHolder:   TicketHolder{DisplayName: "Grace Hopper", Department: "Engineering", City: "Taipei"},
		ticketFamily:   2,
	}.result(req, TicketClaims{})
	require.NoError(t, err)
	assert.Equal(t, offlineScanStatusConflict, conflict.Status)
	assert.Equal(t, "offline_conflict", conflict.ReasonCode)
	assert.Equal(t, offlineConflictExpired, conflict.ConflictReason)
	assert.Equal(t, "evt-2", conflict.EventID)
	assert.Equal(t, 2, conflict.FamilyCount)
}

func TestOfflineReplayRowResultRejectsInconsistentReplayState(t *testing.T) {
	_, err := offlineReplayRow{ticketID: "tic-1", status: offlineScanStatusAccepted}.result(OfflineCheckinSyncRequest{}, TicketClaims{})
	require.ErrorContains(t, err, "accepted scan is missing check-in record")

	_, err = offlineReplayRow{ticketID: "tic-1", status: offlineScanStatusDuplicate}.result(OfflineCheckinSyncRequest{}, TicketClaims{})
	require.ErrorContains(t, err, "duplicate scan is missing first check-in record")

	_, err = offlineReplayRow{ticketID: "tic-1", status: offlineScanStatusConflict}.result(OfflineCheckinSyncRequest{}, TicketClaims{})
	require.ErrorContains(t, err, "ticket snapshot is missing")
}

func TestOfflineReplayRowResultFallsBackToClaimsForUnknownTicketConflict(t *testing.T) {
	scannedAt := time.Date(2026, 6, 3, 11, 0, 0, 0, time.UTC)

	result, err := offlineReplayRow{
		status:         offlineScanStatusConflict,
		conflictReason: offlineConflictNotFound,
		scannedAt:      scannedAt,
	}.result(
		OfflineCheckinSyncRequest{EventID: "evt-request"},
		TicketClaims{TicketID: "tic-404", EmployeeID: "E404"},
	)

	require.NoError(t, err)
	assert.Equal(t, "tic-404", result.TicketID)
	assert.Equal(t, "evt-request", result.EventID)
	assert.Equal(t, "E404", result.EmployeeID)
	assert.Equal(t, offlineScanStatusConflict, result.Status)
	assert.Equal(t, offlineConflictNotFound, result.ConflictReason)
	assert.Equal(t, scannedAt, result.ScannedAt)
}

func TestOfflineReplayKeyUsesPostgresMicrosecondPrecision(t *testing.T) {
	scannedAt := time.Date(2026, 6, 3, 10, 30, 0, 123456789, time.FixedZone("TST", 8*60*60))

	assert.Equal(t, "hash-1|2026-06-03T02:30:00.123456Z", offlineReplayKey("hash-1", scannedAt))
}
