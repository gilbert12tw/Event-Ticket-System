package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireDestructiveOptIn(t *testing.T) {
	t.Setenv(destructiveOptInEnv, "")
	err := requireDestructiveOptIn()
	require.Error(t, err)
	assert.Contains(t, err.Error(), destructiveOptInEnv)

	t.Setenv(destructiveOptInEnv, "1")
	require.NoError(t, requireDestructiveOptIn())
}

func TestValidateExperimentShape(t *testing.T) {
	require.NoError(t, validateExperimentShape(200, 10))
	require.NoError(t, validateExperimentShape(10, 10))

	require.Error(t, validateExperimentShape(0, 10))
	require.Error(t, validateExperimentShape(5, 10))
}

func TestValidateExperimentOutcome(t *testing.T) {
	require.NoError(t, validateExperimentOutcome(200, 10, 10, 190, 0, 10))

	cases := []struct {
		name          string
		confirmed     int
		waitlisted    int
		bookingErrors int
		dbConfirmed   int
	}{
		{name: "booking errors", confirmed: 10, waitlisted: 190, bookingErrors: 1, dbConfirmed: 10},
		{name: "confirmed mismatch", confirmed: 9, waitlisted: 191, bookingErrors: 0, dbConfirmed: 9},
		{name: "database mismatch", confirmed: 10, waitlisted: 190, bookingErrors: 0, dbConfirmed: 11},
		{name: "waitlist mismatch", confirmed: 10, waitlisted: 189, bookingErrors: 0, dbConfirmed: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExperimentOutcome(200, 10, tc.confirmed, tc.waitlisted, tc.bookingErrors, tc.dbConfirmed)
			require.Error(t, err)
		})
	}
}
