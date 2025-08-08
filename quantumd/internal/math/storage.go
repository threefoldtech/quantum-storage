package math

import "fmt"

func ComputeBackendSize(totalDesiredBytes, expectedShards, minimalShards int64) (int64, error) {
	if totalDesiredBytes <= 0 {
		return 0, fmt.Errorf("totalDesiredBytes must be > 0")
	}
	if expectedShards <= 0 {
		return 0, fmt.Errorf("expectedShards must be > 0")
	}
	if minimalShards <= 0 {
		return 0, fmt.Errorf("minimalShards must be > 0")
	}
	if minimalShards > expectedShards {
		return 0, fmt.Errorf("minimalShards cannot exceed expectedShards")
	}

	overheadMultiplier := expectedShards / minimalShards
	perBackend := (totalDesiredBytes / expectedShards) * overheadMultiplier
	return perBackend, nil
}

func ComputeTotalStorage(backendSizeBytes, expectedShards, minimalShards int64) (int64, error) {
	if backendSizeBytes <= 0 {
		return 0, fmt.Errorf("backendSizeBytes must be > 0")
	}
	if expectedShards <= 0 {
		return 0, fmt.Errorf("expectedShards must be > 0")
	}
	if minimalShards <= 0 {
		return 0, fmt.Errorf("minimalShards must be > 0")
	}
	if minimalShards > expectedShards {
		return 0, fmt.Errorf("minimalShards cannot exceed expectedShards")
	}

	// To reverse the calculation, we need to account for the overhead multiplier.
	// The formula for backend size is:
	// backend = (total / expected) * (expected / minimal)
	// backend = total / minimal
	// So, total = backend * minimal
	return backendSizeBytes * minimalShards, nil
}
