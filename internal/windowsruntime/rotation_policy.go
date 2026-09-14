package windowsruntime

import (
	"errors"
	"fmt"
	"time"
)

const (
	DefaultLaneRotationMinSeconds = 30 * 60
	DefaultLaneRotationMaxSeconds = 60 * 60
	minConfigurableLaneRotation   = 10 * time.Second
)

func laneRotationSeconds(profile Profile) (int, int) {
	return profile.LaneRotationMinSeconds, profile.LaneRotationMaxSeconds
}

func automaticLaneRotationEnabled(profile Profile) bool {
	minSeconds, maxSeconds := laneRotationSeconds(profile)
	return minSeconds > 0 && maxSeconds > 0
}

func validateLaneRotationProfile(profile Profile) error {
	minSeconds, maxSeconds := laneRotationSeconds(profile)
	if minSeconds == 0 && maxSeconds == 0 {
		return nil
	}
	if minSeconds == 0 || maxSeconds == 0 {
		return errors.New("lane rotation minimum and maximum must both be zero to disable automatic rotation, or both be positive")
	}
	if minSeconds < int(minConfigurableLaneRotation/time.Second) {
		return fmt.Errorf("lane rotation minimum must be at least %d seconds", int(minConfigurableLaneRotation/time.Second))
	}
	if maxSeconds < minSeconds {
		return errors.New("lane rotation maximum must be greater than or equal to the minimum")
	}
	maxDurationSeconds := int64((1<<63 - 1) / int64(time.Second))
	if int64(maxSeconds) > maxDurationSeconds {
		return errors.New("lane rotation maximum is too large")
	}
	return nil
}

func laneRotationBounds(profile Profile) (time.Duration, time.Duration) {
	minSeconds, maxSeconds := laneRotationSeconds(profile)
	return time.Duration(minSeconds) * time.Second, time.Duration(maxSeconds) * time.Second
}
