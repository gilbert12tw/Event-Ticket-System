package ticketing

import "errors"

type AppError struct {
	Status  int
	Message string
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
