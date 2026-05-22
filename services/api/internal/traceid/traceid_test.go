package traceid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReturnsTraceID(t *testing.T) {
	got := New()

	require.True(t, strings.HasPrefix(got, "trc_"))
	assert.Len(t, got, 36)
}

func TestNormalizeAcceptsPrintableASCIIAndRejectsUnsafeValues(t *testing.T) {
	assert.Equal(t, "trace-1", Normalize(" trace-1 "))
	assert.Empty(t, Normalize(""))
	assert.Empty(t, Normalize("trace 1"))
	assert.Empty(t, Normalize("trace\n1"))
	assert.Empty(t, Normalize(strings.Repeat("a", 129)))
	assert.Empty(t, Normalize("trace-é"))
}

func TestEnsurePreservesValidTraceIDAndGeneratesMissingOne(t *testing.T) {
	assert.Equal(t, "trace-1", Ensure(" trace-1 "))

	generated := Ensure(" ")
	require.True(t, strings.HasPrefix(generated, "trc_"))
	assert.Len(t, generated, 36)
}

func TestContextRoundTrip(t *testing.T) {
	ctx := WithContext(context.Background(), " trace-ctx ")

	assert.Equal(t, "trace-ctx", FromContext(ctx))
	assert.Empty(t, FromContext(nil)) //nolint:staticcheck // FromContext explicitly accepts nil for middleware fallbacks.
	assert.Empty(t, FromContext(context.Background()))
}
