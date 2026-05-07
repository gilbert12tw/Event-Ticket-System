package ticketing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Signer struct {
	secret []byte
}

type TicketClaims struct {
	TicketID   string `json:"ticket_id"`
	EventID    string `json:"event_id"`
	EmployeeID string `json:"employee_id"`
}

type OfflinePackageClaims struct {
	BatchID    string    `json:"batch_id"`
	EventID    string    `json:"event_id"`
	DeviceID   string    `json:"device_id"`
	StaffID    string    `json:"staff_id"`
	ValidUntil time.Time `json:"valid_until"`
}

func NewSigner(secret string) Signer {
	return Signer{secret: []byte(secret)}
}

func (s Signer) Sign(claims TicketClaims) (string, error) {
	return s.signJSON(claims)
}

func (s Signer) SignOfflinePackage(claims OfflinePackageClaims) (string, error) {
	return s.signJSON(claims)
}

func (s Signer) VerifyOfflinePackage(token string) (OfflinePackageClaims, error) {
	var claims OfflinePackageClaims
	if err := s.verifyJSON(token, &claims); err != nil {
		return claims, err
	}
	if claims.BatchID == "" || claims.EventID == "" || claims.DeviceID == "" || claims.StaffID == "" || claims.ValidUntil.IsZero() {
		return claims, errors.New("offline package claims are incomplete")
	}
	return claims, nil
}

func (s Signer) signJSON(value interface{}) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.signature(payloadPart)
	return payloadPart + "." + signature, nil
}

func (s Signer) Verify(token string) (TicketClaims, error) {
	var claims TicketClaims
	if err := s.verifyJSON(token, &claims); err != nil {
		return claims, err
	}
	if claims.TicketID == "" || claims.EventID == "" || claims.EmployeeID == "" {
		return claims, errors.New("token claims are incomplete")
	}
	return claims, nil
}

func (s Signer) verifyJSON(token string, value interface{}) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return errors.New("invalid token format")
	}
	expected := s.signature(parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, value)
}

func (s Signer) HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s Signer) signature(payloadPart string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payloadPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
