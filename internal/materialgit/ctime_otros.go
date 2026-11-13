//go:build !linux && !darwin

package materialgit

import "io/fs"

// CtimeDe is the status change time of fi: this system's stat is not read
// for it, so there is none.
func CtimeDe(fs.FileInfo) (int64, bool) {
	return 0, false
}
