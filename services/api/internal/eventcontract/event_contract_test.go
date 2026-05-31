package eventcontract_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"event-ticket-system/internal/eventcontract"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testdataDirName = "testdata"

func loadFixture(t *testing.T, eventType string) map[string]any {
	t.Helper()
	path := filepath.Join(testdataDirName, eventType+".json")
	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "missing fixture %s", path)
	var env map[string]any
	require.NoErrorf(t, json.Unmarshal(data, &env), "fixture %s not valid JSON", path)
	return env
}

func TestRegistryHasFixtureForEveryEntry(t *testing.T) {
	entries, err := os.ReadDir(testdataDirName)
	require.NoError(t, err)
	have := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		have[strings.TrimSuffix(name, ".json")] = true
	}
	want := map[string]bool{}
	for _, et := range eventcontract.Registry {
		want[et] = true
		assert.Truef(t, have[et], "registry entry %s has no fixture under %s/", et, testdataDirName)
	}
	for fixtureType := range have {
		assert.Truef(t, want[fixtureType], "orphan fixture %s.json (not in registry)", fixtureType)
	}
}

func TestFixtureEnvelopeShape(t *testing.T) {
	for _, et := range eventcontract.Registry {
		et := et
		t.Run(et, func(t *testing.T) {
			env := loadFixture(t, et)
			for _, k := range eventcontract.RequiredEnvelopeKeys {
				assert.Containsf(t, env, k, "envelope missing key %s", k)
			}
			assert.EqualValuesf(t, 2, env["schema_version"], "schema_version must be 2")
			assert.Equalf(t, et, env["event_type"], "event_type must match filename")

			occurred, ok := env["occurred_at"].(string)
			require.Truef(t, ok, "occurred_at must be string")
			require.Truef(t, strings.HasSuffix(occurred, "Z"), "occurred_at must end with Z (UTC)")
			_, err := time.Parse(time.RFC3339, occurred)
			require.NoErrorf(t, err, "occurred_at must be RFC3339")

			idem, _ := env["idempotency_key"].(string)
			require.NotEmptyf(t, idem, "idempotency_key must be non-empty")
			part, _ := env["partition_key"].(string)
			require.NotEmptyf(t, part, "partition_key must be non-empty")

			_, ok = env["payload"].(map[string]any)
			require.Truef(t, ok, "payload must be object")
		})
	}
}

func TestFixtureNoForbiddenKeys(t *testing.T) {
	for _, et := range eventcontract.Registry {
		et := et
		t.Run(et, func(t *testing.T) {
			env := loadFixture(t, et)
			require.NoError(t, walkRejectKeys(env, ""))
		})
	}
}

func TestFixtureNoForbiddenPatterns(t *testing.T) {
	for _, et := range eventcontract.Registry {
		et := et
		t.Run(et, func(t *testing.T) {
			env := loadFixture(t, et)
			body, err := json.Marshal(env)
			require.NoError(t, err)
			assert.Falsef(t, eventcontract.EmailPattern.Match(body), "fixture %s contains email-shaped substring: %s", et, body)
			assert.Falsef(t, eventcontract.JWTPattern.Match(body), "fixture %s contains JWT-shaped substring", et)
		})
	}
}

// validateEnvelope is the contract validator under test. It is intentionally
// in-package (not exported) — the spec is fixture-driven, not Go-typed.
func validateEnvelope(env map[string]any) error {
	for _, k := range eventcontract.RequiredEnvelopeKeys {
		if _, ok := env[k]; !ok {
			return fmt.Errorf("missing %s", k)
		}
	}
	if v, _ := env["schema_version"].(float64); v != 2 {
		return fmt.Errorf("schema_version must be 2, got %v", env["schema_version"])
	}
	if err := walkRejectKeys(env, ""); err != nil {
		return err
	}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if eventcontract.EmailPattern.Match(body) {
		return fmt.Errorf("forbidden email pattern present")
	}
	if eventcontract.JWTPattern.Match(body) {
		return fmt.Errorf("forbidden JWT pattern present")
	}
	return nil
}

func walkRejectKeys(node any, path string) error {
	switch v := node.(type) {
	case map[string]any:
		return walkRejectMapKeys(v, path)
	case []any:
		return walkRejectSliceKeys(v, path)
	}
	return nil
}

func walkRejectMapKeys(node map[string]any, path string) error {
	keys := sortedMapKeys(node)
	for _, key := range keys {
		if _, banned := eventcontract.ForbiddenKeys[key]; banned {
			return fmt.Errorf("forbidden key %q at %s/%s", key, path, key)
		}
		if err := walkRejectKeys(node[key], path+"/"+key); err != nil {
			return err
		}
	}
	return nil
}

func walkRejectSliceKeys(node []any, path string) error {
	for i, item := range node {
		if err := walkRejectKeys(item, fmt.Sprintf("%s/[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func sortedMapKeys(node map[string]any) []string {
	keys := make([]string, 0, len(node))
	for key := range node {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestValidatorRejectsMissingSchemaVersion(t *testing.T) {
	env := loadFixture(t, "registration.confirmed.v2")
	delete(env, "schema_version")
	err := validateEnvelope(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing schema_version")
}

func TestValidatorRejectsWrongSchemaVersion(t *testing.T) {
	env := loadFixture(t, "registration.confirmed.v2")
	env["schema_version"] = float64(1)
	err := validateEnvelope(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema_version must be 2")
}

func mutablePayload(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	payload, ok := env["payload"].(map[string]any)
	require.Truef(t, ok, "payload must be object")
	return payload
}

func TestValidatorRejectsForbiddenEmail(t *testing.T) {
	env := loadFixture(t, "notification.requested.v2")
	mutablePayload(t, env)["nickname"] = "ops-rotation+contact@example.com"
	err := validateEnvelope(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email pattern")
}

func TestValidatorRejectsForbiddenJWT(t *testing.T) {
	env := loadFixture(t, "ticket.issued.v2")
	mutablePayload(t, env)["audit_blob"] = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4ifQ.dummysig01"
	err := validateEnvelope(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JWT pattern")
}

func TestValidatorRejectsForbiddenKey(t *testing.T) {
	env := loadFixture(t, "notification.requested.v2")
	mutablePayload(t, env)["recipient_email"] = "redacted"
	err := validateEnvelope(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden key")
}
