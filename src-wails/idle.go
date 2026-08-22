package main

import "time"

func shouldIdleStop(pinned bool, active int, idleStopMin int, lastAccess, now time.Time) bool {
	if pinned || active > 0 || idleStopMin <= 0 || lastAccess.IsZero() {
		return false
	}
	return !lastAccess.Add(time.Duration(idleStopMin) * time.Minute).After(now)
}

func idleUntilUnix(pinned bool, idleStopMin int, lastAccess time.Time) *int64 {
	if pinned || idleStopMin <= 0 || lastAccess.IsZero() {
		return nil
	}
	until := lastAccess.Add(time.Duration(idleStopMin) * time.Minute).Unix()
	return &until
}

func clampIdleStopMin(v int) int {
	if v < 0 {
		return 0
	}
	if v > 24*60 {
		return 24 * 60
	}
	return v
}
