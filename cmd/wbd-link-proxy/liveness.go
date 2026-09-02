package main

import "time"

const clientRemoteRXTimeoutWindows = 3

func clientRemoteRXTimeout(keepalive time.Duration) time.Duration {
	if keepalive <= 0 {
		return 0
	}
	const maxDuration = time.Duration(1<<63 - 1)
	if keepalive > maxDuration/clientRemoteRXTimeoutWindows {
		return maxDuration
	}
	return keepalive * clientRemoteRXTimeoutWindows
}

func clientRemoteRXExpired(lastRemoteRX, now time.Time, keepalive time.Duration) bool {
	timeout := clientRemoteRXTimeout(keepalive)
	return timeout > 0 && !now.Before(lastRemoteRX.Add(timeout))
}
