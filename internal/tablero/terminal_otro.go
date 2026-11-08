//go:build !unix && !windows

package tablero

import (
	"os"
	"time"
)

func habilitarVT(*os.File) func() { return func() {} }

func hayEntrada(*os.File, time.Duration) (bool, error) { return true, nil }

func avisosDeTamano() (<-chan os.Signal, func()) { return nil, func() {} }
