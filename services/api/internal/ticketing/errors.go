package ticketing

import "errors"

type AppError struct {
	Status            int
	Code              string
	Message           string
	RetryAfterSeconds int
}

func (e AppError) Error() string {
	return e.Message
}

func ErrorStatus(err error) int {
	var appErr AppError
	if errors.As(err, &appErr) {
		return appErr.Status
	}
	return 500
}

func ErrorMessage(err error) string {
	var appErr AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return "internal server error"
}

func ErrorCode(err error) string {
	var appErr AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

func ErrorRetryAfterSeconds(err error) (int, bool) {
	var appErr AppError
	if errors.As(err, &appErr) && appErr.RetryAfterSeconds > 0 {
		return appErr.RetryAfterSeconds, true
	}
	return 0, false
}

func badRequest(message string) AppError {
	return AppError{Status: 400, Message: message}
}

func unauthorized(message string) AppError {
	return AppError{Status: 401, Message: message}
}

func forbidden(message string) AppError {
	return AppError{Status: 403, Message: message}
}

func notFound(message string) AppError {
	return AppError{Status: 404, Message: message}
}

func conflict(message string) AppError {
	return AppError{Status: 409, Message: message}
}

func serviceUnavailable(code string, message string) AppError {
	return AppError{Status: 503, Code: code, Message: message}
}

func rateLimited(code string, message string, retryAfterSeconds int) AppError {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 1
	}
	return AppError{Status: 429, Code: code, Message: message, RetryAfterSeconds: retryAfterSeconds}
}

func notImplemented(message string) AppError {
	return AppError{Status: 501, Message: message}
}

// bookingBanned returns a 422 with machine-readable code BOOKING_BANNED.
// Use this only when an active booking_bans row blocks the booking attempt.
func bookingBanned(message string) AppError {
	return AppError{Status: 422, Code: "BOOKING_BANNED", Message: message}
}
