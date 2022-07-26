package segment

import (
	"crypto/sha1" // nolint:gosec
	"fmt"
)

// Hashes the given username to anonymize the data sent to Segment (and subsequently Woopra)
//
// Note: implementation of the hash MUST match with the existing ones of CRW and Web/Dev Console
// so that metrics can be correlated in Woopra
// See https://github.com/che-incubator/che-workspace-telemetry-client/pull/68/files
func hash(username string) string {
	h := sha1.New() // nolint:gosec
	h.Write([]byte(username))
	return fmt.Sprintf("%x", h.Sum(nil))
}
