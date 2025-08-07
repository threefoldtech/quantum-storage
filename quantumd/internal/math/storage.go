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
