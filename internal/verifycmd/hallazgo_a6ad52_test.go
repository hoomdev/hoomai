//go:build !windows

// Regresion de los hallazgos 20261125T182624_a6ad52 y 20261125T183322_94b657
// (high), en Run: el Studio carga el manifiesto una vez (manifest.Load) y lo
// reusa en cada verify. Con B commiteado en la base (static sin aplica_en) y
// el hoom.yaml del arbol cambiado a A (static con aplica_en: [app/**]) al
// cargar, si despues el hoom.yaml vuelve a ser B (un archivo regular, igual a
// la base) y el cambio es solo resources/js/app.js, el Lstat de 8c4016 no ve
// nada y static quedaba no_aplica, juzgado con un aplica_en que el proyecto
// ya no tiene. Si el hoom.yaml de m.Dir ya no tiene los bytes con los que se
// cargo, ante la duda el gate corre (CA-812).
package verifycmd_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

const ha6aCabecera = "schema: hoom/v1\nproject: demo\nbase_branch: main\n"

// ha6aGates es la seccion gates: test sin aplica_en, y static, que deja
// marca/static-cmd; conAplicaEn le pone aplica_en: [app/**] (A), sin, aplica
// siempre (B).
func ha6aGates(marca string, conAplicaEn bool) string {
	s := "gates:\n  test:\n    required: true\n    cmd: \"true\"\n" +
		"  static:\n    required: true\n    cmd: " + strconv.Quote(gprToque(marca, "static-cmd", false)) + "\n"
	if conAplicaEn {
		s += "    aplica_en: [\"app/**\"]\n"
	}
	return s
}

// ha6aSoloElJS cambia solo resources/js/app.js, en un commit.
func ha6aSoloElJS(t *testing.T, root string) {
	t.Helper()
	gprEscribir(t, root, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, root, "solo el js")
}

func TestHallazgoA6ad52_CA812_ElHoomYamlCambiadoDesdeQueSeCargoElGateCorre(t *testing.T) {
	const ca = "CA-812"
	casos := []struct {
		nombre  string
		volverB func(t *testing.T, root, b string)
	}{
		{"restaurado con git checkout", func(t *testing.T, root, _ string) {
			gprGit(t, root, "checkout", "-q", "--", manifest.FileName)
		}},
		{"reescrito con los bytes de la base", func(t *testing.T, root, b string) {
			gprEscribir(t, root, manifest.FileName, b)
		}},
		{"reemplazado con un rename, como guarda un editor", func(t *testing.T, root, b string) {
			tmp := filepath.Join(root, manifest.FileName+".tmp")
			if err := os.WriteFile(tmp, []byte(b), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(tmp, filepath.Join(root, manifest.FileName)); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			marca := t.TempDir()
			b := ha6aCabecera + ha6aGates(marca, false)
			root := gprRepo(t, ha6aGates(marca, false), map[string]string{"app/Models/User.php": "<?php\n"})
			gprEscribir(t, root, manifest.FileName, ha6aCabecera+ha6aGates(marca, true))
			m, err := manifest.Load(root, nil)
			if err != nil {
				t.Fatalf("%s: fixture: Load con A: %v", ca, err)
			}
			if g := m.Gates["static"]; len(g.AplicaEn) != 1 {
				t.Fatalf("%s: fixture: el manifiesto cargado es A: %+v", ca, g)
			}
			c.volverB(t, root, b)
			if raw, err := os.ReadFile(filepath.Join(root, manifest.FileName)); err != nil || string(raw) != b {
				t.Fatalf("%s: fixture: el hoom.yaml es B, igual a la base: %v\n%s", ca, err, raw)
			}
			ha6aSoloElJS(t, root)
			v, _, err := verifycmd.Run(m, verifycmd.Options{})
			if err != nil {
				t.Fatalf("%s (%s): verify corre: %v", ca, c.nombre, err)
			}
			g := gprGate(t, ca, v, "static")
			if g.Status != verdict.StatusPass || !gprHay(marca, "static-cmd") || v.Summary.NoAplica != 0 {
				t.Fatalf("%s (%s): con el hoom.yaml cambiado desde que se cargo, static corre, no queda no_aplica: %+v %+v", ca, c.nombre, g, v.Summary)
			}
		})
	}
}

// El control: con A commiteado en la base y el hoom.yaml con los mismos
// bytes con los que se cargo (sin tocar, o reescrito igual: cuenta el
// contenido, no la fecha), y solo resources/js/app.js cambiado, static sigue
// en no_aplica.
func TestHallazgoA6ad52_CA810_ConElHoomYamlQueSeCargoStaticNoAplica(t *testing.T) {
	const ca = "CA-810"
	for _, reescrito := range []bool{false, true} {
		nombre := "sin tocar"
		if reescrito {
			nombre = "reescrito con los mismos bytes"
		}
		t.Run(nombre, func(t *testing.T) {
			marca := t.TempDir()
			root := gprRepo(t, ha6aGates(marca, true), map[string]string{"app/Models/User.php": "<?php\n"})
			m, err := manifest.Load(root, nil)
			if err != nil {
				t.Fatalf("%s: fixture: Load con A: %v", ca, err)
			}
			if reescrito {
				gprEscribir(t, root, manifest.FileName, ha6aCabecera+ha6aGates(marca, true))
			}
			ha6aSoloElJS(t, root)
			v, _, err := verifycmd.Run(m, verifycmd.Options{})
			if err != nil {
				t.Fatalf("%s (%s): verify corre: %v", ca, nombre, err)
			}
			if g := gprGate(t, ca, v, "static"); g.Status != verdict.StatusNoAplica || gprHay(marca, "static-cmd") {
				t.Fatalf("%s (%s): con el hoom.yaml que se cargo y solo el js cambiado, static no aplica: %+v", ca, nombre, g)
			}
		})
	}
}

// Un Manifest armado en codigo, sin manifest.Load, no dice con que bytes se
// juzga: aunque m.Dir tenga un hoom.yaml regular igual a la base, ante la
// duda el gate corre.
func TestHallazgoA6ad52_CA812_UnManifestSinCargarCorreTodo(t *testing.T) {
	const ca = "CA-812"
	marca := t.TempDir()
	root := gprRepo(t, ha6aGates(marca, true), map[string]string{"app/Models/User.php": "<?php\n"})
	ha6aSoloElJS(t, root)
	req := true
	test, static := "true", gprToque(marca, "static-cmd", false)
	m := &manifest.Manifest{Schema: "hoom/v1", Project: "demo", BaseBranch: "main", Dir: root,
		Gates: map[string]manifest.Gate{
			"test":   {Required: &req, Cmd: &test},
			"static": {Required: &req, Cmd: &static, AplicaEn: []string{"app/**"}},
		}}
	v, _, err := verifycmd.Run(m, verifycmd.Options{})
	if err != nil {
		t.Fatalf("%s: verify corre: %v", ca, err)
	}
	g := gprGate(t, ca, v, "static")
	if g.Status != verdict.StatusPass || !gprHay(marca, "static-cmd") || v.Summary.NoAplica != 0 {
		t.Fatalf("%s: sin la huella de manifest.Load, static corre, no queda no_aplica: %+v %+v", ca, g, v.Summary)
	}
}
