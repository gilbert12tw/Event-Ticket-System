package ticketing

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readJSONMap(t *testing.T, service *Service, ctx context.Context, query string, args ...interface{}) map[string]interface{} {
	t.Helper()
	var raw string
	require.NoError(t, service.db.QueryRow(ctx, query, args...).Scan(&raw))
	got := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(raw), &got))
	return got
}

func assertNoSensitiveJSONValues(t *testing.T, metadata map[string]interface{}, values ...string) {
	t.Helper()
	raw := fmt.Sprint(metadata)
	for _, value := range values {
		assert.NotContains(t, raw, value)
	}
	assert.NotContains(t, raw, "signed_token")
	assert.NotContains(t, raw, "qr_payload")
	assert.NotContains(t, raw, "@")
}
