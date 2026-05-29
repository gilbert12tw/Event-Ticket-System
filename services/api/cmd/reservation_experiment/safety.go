package main

import (
	"errors"
	"fmt"
	"os"
)

const destructiveOptInEnv = "EXPERIMENT_ALLOW_DESTRUCTIVE"

func requireDestructiveOptIn() error {
	if os.Getenv(destructiveOptInEnv) == "1" {
		return nil
	}
	return fmt.Errorf("%s=1 is required because this harness truncates database tables and deletes Redis reservation keys", destructiveOptInEnv)
}

func validateExperimentShape(vus, capacity int) error {
	if vus <= 0 || capacity <= 0 {
		return errors.New("EXPERIMENT_VUS and EXPERIMENT_CAPACITY must be positive")
	}
	if vus < capacity {
		return errors.New("EXPERIMENT_VUS must be greater than or equal to EXPERIMENT_CAPACITY")
	}
	return nil
}

func validateExperimentOutcome(vus, capacity, confirmed, waitlisted, bookingErrors, dbConfirmed int) error {
	if bookingErrors != 0 {
		return fmt.Errorf("experiment produced %d booking errors", bookingErrors)
	}
	if confirmed != capacity {
		return fmt.Errorf("confirmed bookings=%d, want capacity=%d", confirmed, capacity)
	}
	if dbConfirmed != capacity {
		return fmt.Errorf("database confirmed count=%d, want capacity=%d", dbConfirmed, capacity)
	}
	expectedWaitlisted := vus - capacity
	if waitlisted != expectedWaitlisted {
		return fmt.Errorf("waitlisted bookings=%d, want %d", waitlisted, expectedWaitlisted)
	}
	return nil
}
