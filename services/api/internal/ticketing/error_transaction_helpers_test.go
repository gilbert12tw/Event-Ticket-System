package ticketing

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestAppErrorHelpersExposeStatusAndMessage(t *testing.T) {
	err := notImplemented("future workflow")

	assert.Equal(t, "future workflow", err.Error())
	assert.Equal(t, 501, ErrorStatus(err))
	assert.Equal(t, "future workflow", ErrorMessage(err))
}

func TestErrorHelpersFallBackForGenericErrors(t *testing.T) {
	err := errors.New("database unavailable")

	assert.Equal(t, 500, ErrorStatus(err))
	assert.Equal(t, "internal server error", ErrorMessage(err))
}

func TestIsUniqueViolationMatchesWrappedPgError(t *testing.T) {
	err := fmt.Errorf("insert registration: %w", &pgconn.PgError{Code: "23505"})

	assert.True(t, isUniqueViolation(err))
	assert.False(t, isUniqueViolation(&pgconn.PgError{Code: "23503"}))
	assert.False(t, isUniqueViolation(errors.New("constraint failed")))
}
