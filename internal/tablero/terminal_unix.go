//go:build !windows

package tablero

import "os"

// habilitarVT does nothing outside Windows: terminals read ANSI already.
func habilitarVT(*os.File) func() { return func() {} }
