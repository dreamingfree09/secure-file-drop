package server

import (
	"errors"
	"os"
)

// LoadManagedLinksConfig is off by default. Schema 15 is only selected for
// explicitly qualified self-hosted stacks already using schema 14.
func LoadManagedLinksConfig(resumableEnabled bool) (bool, error) {
	switch os.Getenv("SFD_MANAGED_LINKS_ENABLED") {
	case "", "false", "0":
		return false, nil
	case "true":
		if !resumableEnabled {
			return false, errors.New("managed links require private resumable storage")
		}
		return true, nil
	default:
		return false, errors.New("invalid managed links configuration")
	}
}
