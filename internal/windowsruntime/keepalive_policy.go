package windowsruntime

import "errors"

func validateKeepaliveSeconds(seconds *int) error {
	if seconds == nil {
		return nil
	}
	if *seconds < 0 {
		return errors.New("keepalive must be zero (disabled) or a positive number of seconds")
	}
	if int64(*seconds) > maxIdleTimeoutSeconds {
		return errors.New("keepalive is too large")
	}
	return nil
}

func explicitKeepaliveDisabled(profile Profile) bool {
	return profile.KeepaliveSeconds != nil && *profile.KeepaliveSeconds == 0
}
