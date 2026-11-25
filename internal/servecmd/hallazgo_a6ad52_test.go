//go:build !windows

// Regresion de los hallazgos 20261125T182624_a6ad52 y 20261125T183322_94b657
// (high), por el Studio: servecmd.New carga el manifiesto una vez y POST
// /api/verify lo reusa. Arrancado con el hoom.yaml del arbol cambiado a A
// (static y mutation con aplica_en: [app/**]) sobre B en la base (sin
// aplica_en), si el hoom.yaml vuelve a ser B y el cambio es solo
// resources/js/app.js, static y mutation quedaban no_aplica. Si el hoom.yaml
// ya no tiene los bytes con los que se cargo, ante la duda el gate corre
// (CA-812).
package servecmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ha6aVerify hace POST /api/verify en s, con su token, y devuelve el
// veredicto de un 200.
func ha6aVerify(t *testing.T, ca string, s *Server) *verdict.Verdict {
	t.Helper()
	rec := doPOST(t, s, "/api/verify", s.Token(), []byte(`{}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: POST /api/verify responde 200: %d %s", ca, rec.Code, rec.Body.String())
	}
	var v verdict.Verdict
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("%s: el cuerpo de un 200 es el veredicto: %v\n%s", ca, err, rec.Body.String())
	}
	return &v
}

func TestHallazgoA6ad52_CA812_ElStudioConElHoomYamlCambiadoCorreLosGates(t *testing.T) {
	const ca = "CA-812"
	root := gprProyecto(t)
	raw, err := os.ReadFile(filepath.Join(root, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	a := string(raw)
	b := strings.ReplaceAll(a, "    aplica_en: [\"app/**\"]\n", "")
	if b == a {
		t.Fatalf("%s: fixture: B es A sin aplica_en:\n%s", ca, a)
	}
	tbEscribir(t, root, manifest.FileName, b)
	gprCommit(t, root, "B en la base")
	gprGit(t, root, "checkout", "-q", "-b", "feature")
	tbEscribir(t, root, manifest.FileName, a)

	s := newServer(t, root)
	gprGit(t, root, "checkout", "-q", "--", manifest.FileName)
	tbEscribir(t, root, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, root, "solo el js")

	v := ha6aVerify(t, ca, s)
	for _, g := range v.Gates {
		if (g.Name == "static" || g.Name == "mutation") && g.Status != verdict.StatusPass {
			t.Fatalf("%s: con el hoom.yaml cambiado desde que el Studio lo cargo, %s corre: %+v", ca, g.Name, g)
		}
	}
	if v.Summary.NoAplica != 0 || v.Verdict != "green" {
		t.Fatalf("%s: ningun no_aplica y verde: %s %+v", ca, v.Verdict, v.Summary)
	}
}

// El control: el mismo Server, con el hoom.yaml que cargo intacto y solo
// resources/js/app.js cambiado despues de arrancar, deja static y mutation
// en no_aplica.
func TestHallazgoA6ad52_CA810_ElStudioConElHoomYamlQueCargoNoAplica(t *testing.T) {
	const ca = "CA-810"
	root := gprProyecto(t)
	gprGit(t, root, "checkout", "-q", "-b", "feature")
	s := newServer(t, root)
	tbEscribir(t, root, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, root, "solo el js")

	v := ha6aVerify(t, ca, s)
	if v.Summary.NoAplica != 2 || v.Verdict != "green" {
		t.Fatalf("%s: static y mutation en no_aplica: %s %+v", ca, v.Verdict, v.Summary)
	}
	for _, g := range v.Gates {
		if (g.Name == "static" || g.Name == "mutation") && g.Status != verdict.StatusNoAplica {
			t.Fatalf("%s: con el hoom.yaml que el Studio cargo, %s no aplica: %+v", ca, g.Name, g)
		}
	}
}
