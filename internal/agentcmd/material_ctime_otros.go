//go:build !linux && !darwin

package agentcmd

import "io/fs"

// ctimeDe is the status change time of fi: this system's stat is not read
// for it, so there is none.
func ctimeDe(fs.FileInfo) (int64, bool) {
	return 0, false
}
