package eventcontract

import "regexp"

// Registry mirrors docs/specs/phase2-event-contract-v2.md §4 and
// docs/specs/phase2-ws4-async-notification.md §6 (normative event type registry).
var Registry = []string{
	"registration.confirmed.v2",
	"registration.cancelled.v2",
	"registration.waitlisted.v2",
	"registration.received.v2",
	"registration.promoted.v2",
	"ticket.issued.v2",
	"ticket.revoked.v2",
	"ticket.expired.v2",
	"checkin.recorded.v2",
	"notification.requested.v2",
	"reservation.compensation.release_required.v2",
	"report.export.requested.v2",
	"report.export.completed.v2",
	"report.export.failed.v2",
	"hr_sync.batch.completed.v2",
	"eligibility.impact_review.created.v2",
	"reporting.projection.update_required.v2",
}

// RequiredEnvelopeKeys lists the top-level keys every v2 outbox envelope must carry.
var RequiredEnvelopeKeys = []string{
	"event_id", "event_type", "schema_version", "occurred_at",
	"idempotency_key", "partition_key", "payload",
}

// ForbiddenKeys mirrors §8 of the event contract spec: payloads must not carry PII or secrets.
var ForbiddenKeys = map[string]struct{}{
	"email": {}, "recipient_email": {}, "email_address": {}, "sender_email": {},
	"full_name": {}, "display_name": {}, "given_name": {}, "family_name": {},
	"phone": {}, "phone_number": {}, "mobile": {}, "address": {}, "street": {}, "postal_code": {},
	"qr_token": {}, "signed_token": {}, "provider_token": {}, "bearer_token": {}, "access_token": {},
	"refresh_token": {}, "jwt": {}, "session_token": {}, "qr_payload": {},
	"password": {}, "passcode": {}, "pin": {}, "secret": {}, "credentials": {}, "private_key": {},
	"download_url": {}, "signed_url": {}, "presigned_url": {},
}

// EmailPattern detects an email-shaped substring anywhere inside a payload.
var EmailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// JWTPattern detects a three-segment JWT-shaped substring anywhere inside a payload.
var JWTPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)
