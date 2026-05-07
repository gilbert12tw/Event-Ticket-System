package ticketing

import (
	"strings"
	"testing"
	"time"
)

func TestSignerRoundTripAndHash(t *testing.T) {
	signer := NewSigner("test-secret")
	claims := TicketClaims{
		TicketID:   "tkt_1",
		EventID:    "evt_1",
		EmployeeID: "E1001",
	}

	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	got, err := signer.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if got.TicketID != claims.TicketID || got.EventID != claims.EventID || got.EmployeeID != claims.EmployeeID {
		t.Fatalf("claims = %+v", got)
	}
	if signer.HashToken(token) == "" || signer.HashToken(token) == token {
		t.Fatal("expected non-empty token hash")
	}
}

func TestSignerRejectsTampering(t *testing.T) {
	signer := NewSigner("test-secret")
	token, err := signer.Sign(TicketClaims{TicketID: "tkt_1", EventID: "evt_1", EmployeeID: "E1001"})
	if err != nil {
		t.Fatal(err)
	}

	tampered := strings.Replace(token, "E", "F", 1)
	if tampered == token {
		tampered = token + "x"
	}

	if _, err := signer.Verify(tampered); err == nil {
		t.Fatal("expected tampered token error")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	got, err := signer.VerifyOfflinePackage(token)
	if err != nil {
		t.Fatal(err)
	}
	if got.BatchID != claims.BatchID || got.EventID != claims.EventID || got.DeviceID != claims.DeviceID || got.StaffID != claims.StaffID || !got.ValidUntil.Equal(claims.ValidUntil) {
		t.Fatalf("claims = %+v", got)
	}
}
