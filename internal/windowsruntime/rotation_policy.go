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
	minSeconds := profile.LaneRotationMinSeconds
	maxSeconds := profile.LaneRotationMaxSeconds
	if minSeconds == 0 {
		minSeconds = DefaultLaneRotationMinSeconds
	}
	if maxSeconds == 0 {
		maxSeconds = DefaultLaneRotationMaxSeconds
	}
	return minSeconds, maxSeconds
}

func validateLaneRotationProfile(profile Profile) error {
	minSeconds, maxSeconds := laneRotationSeconds(profile)
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
