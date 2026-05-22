package ticketing

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateEventRequestUnmarshalSupportsLegacyAliases(t *testing.T) {
	opensAt := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	closesAt := time.Date(2026, 5, 10, 18, 0, 0, 0, time.UTC)
	payload := `{
		"title": "Legacy Event",
		"registration_opens_at": "2026-05-01T09:00:00Z",
		"registration_closes_at": "2026-05-10T18:00:00Z",
		"capacity": 25,
		"eligibility_rule": {
			"department": "Engineering",
			"site": "Taipei HQ",
			"min_grade": 5,
			"employment_status": "active"
		}
	}`

	var got CreateEventRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &got))

	assert.Equal(t, "Legacy Event", got.Title)
	assert.True(t, opensAt.Equal(got.RegistrationStart))
	assert.True(t, closesAt.Equal(got.RegistrationClose))
	assert.Equal(t, 25, got.Capacity)
	assert.Equal(t, RuleInput{
		Department:       "Engineering",
		Site:             "Taipei HQ",
		MinGrade:         5,
		EmploymentStatus: "active",
	}, got.Rule)
}

func TestCreateEventRequestUnmarshalIgnoresNullCapacity(t *testing.T) {
	var got CreateEventRequest

	require.NoError(t, json.Unmarshal([]byte(`{"capacity": null}`), &got))
	assert.Zero(t, got.Capacity)
}

func TestCreateEventRequestUnmarshalRejectsInvalidCapacity(t *testing.T) {
	var got CreateEventRequest

	require.Error(t, json.Unmarshal([]byte(`{"capacity": "many"}`), &got))
}

func TestUpdateEventRequestUnmarshalTracksCapacityPresence(t *testing.T) {
	opensAt := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	closesAt := time.Date(2026, 6, 5, 18, 0, 0, 0, time.UTC)
	payload := `{
		"registration_opens_at": "2026-06-01T09:00:00Z",
		"registration_closes_at": "2026-06-05T18:00:00Z",
		"capacity": 12
	}`

	var got UpdateEventRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &got))

	require.NotNil(t, got.RegistrationStart)
	require.NotNil(t, got.RegistrationClose)
	require.NotNil(t, got.Capacity)
	assert.True(t, opensAt.Equal(*got.RegistrationStart))
	assert.True(t, closesAt.Equal(*got.RegistrationClose))
	assert.Equal(t, 12, *got.Capacity)
	assert.True(t, got.capacitySet)
}

func TestUpdateEventRequestUnmarshalDistinguishesNullAndAbsentCapacity(t *testing.T) {
	var nullCapacity UpdateEventRequest
	require.NoError(t, json.Unmarshal([]byte(`{"capacity": null}`), &nullCapacity))
	assert.True(t, nullCapacity.capacitySet)
	assert.Nil(t, nullCapacity.Capacity)

	var absentCapacity UpdateEventRequest
	require.NoError(t, json.Unmarshal([]byte(`{"title": "No capacity edit"}`), &absentCapacity))
	assert.False(t, absentCapacity.capacitySet)
	assert.Nil(t, absentCapacity.Capacity)
}

func TestUpdateEventRequestUnmarshalRejectsInvalidCapacity(t *testing.T) {
	var got UpdateEventRequest

	require.Error(t, json.Unmarshal([]byte(`{"capacity": "many"}`), &got))
}
