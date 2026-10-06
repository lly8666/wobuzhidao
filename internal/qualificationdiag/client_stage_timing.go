package qualificationdiag

import (
	"errors"
	"os"
)

// ClientStageTimingEnabled validates the private qualification opt-in before
// startup mutates any network state. It is not a user configuration parameter.
func ClientStageTimingEnabled(diagnosticPath string) (bool, error) {
	switch os.Getenv("WBD_QUALIFICATION_CLIENT_STAGE_TIMING") {
	case "", "0":
		return false, nil
	case "1":
		if diagnosticPath == "" {
			return false, errors.New("qualification client stage timing requires diagnostic-jsonl")
		}
		return true, nil
	default:
		return false, errors.New("WBD_QUALIFICATION_CLIENT_STAGE_TIMING must be empty, 0 or 1")
	}
}
