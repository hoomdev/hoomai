package gittest

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMain saca del entorno de este binario las variables de git que trae
// quien corre go test y que cambian lo que un fixture planta o lee: las de
// quitaAislarGit (seQuitaEnAislarGit), con valor o vacias. Los controles de
// los tests de este paquete corren git con el entorno del proceso antes de
// aislar nada, y con un GIT_DIR heredado el git init del control de
// aislar_test.go no creaba control/.git (hallazgos 20261029T193536_20cb80 y
// 20261029T193832_7dbb81); con un GIT_CONFIG_PARAMETERS o un GIT_TRACE
// heredados fallaba igual el control de
// TestHallazgo30abfc_EntornoSoloNoLeeLaConfiguracionDelQueCorreLosTests
// (gittest_test.go), que lee con ese entorno lo que el test planta.
//
// No llama a AislarGit, que es lo que se prueba aca: no fija la config
// global ni la del sistema. Los tests de este paquete plantan la suya con
// t.Setenv (HOME, XDG_CONFIG_HOME, GIT_CONFIG_GLOBAL) o corren git con
// EntornoSolo.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		if nombre, _, _ := strings.Cut(kv, "="); seQuitaEnAislarGit(nombre) {
			if err := os.Unsetenv(nombre); err != nil {
				fmt.Fprintln(os.Stderr, "fixture: quitar", nombre, "del entorno:", err)
				os.Exit(2)
			}
		}
	}
	os.Exit(m.Run())
}
