// Tests adversariales del spec .hoom/specs/trinquete-ratchet.md
// (CA-89, CA-91..CA-94, CA-96, CA-97): la base solo se mueve hacia mejor,
// el ruido no la mueve, y aflojar exige razon registrada.
package ratchet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/verdict"
)

func fp(v float64) *float64 { return &v }

func writeFile(t *testing.T, root string, metrics map[string]*Metric) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".hoom"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &File{Schema: Schema, Metrics: metrics}
	if err := f.Save(root); err != nil {
		t.Fatal(err)
	}
}

func reload(t *testing.T, root string) *File {
	t.Helper()
	f, err := Load(root)
	if err != nil || f == nil {
		t.Fatalf("no se pudo recargar la base: %v", err)
	}
	return f
}

func gateOn(t *testing.T, root string) (verdict.GateResult, *File) {
	t.Helper()
	f := reload(t, root)
	res := Gate(root, f)
	return res, reload(t, root)
}

// CA-89: init crea el esqueleto y se niega a pisar uno existente.
func TestCA89_Init(t *testing.T) {
	root := t.TempDir()
	p, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != FileName {
		t.Fatalf("CA-89: ruta inesperada %q", p)
	}
	f := reload(t, root)
	if f.Schema != Schema || len(f.Metrics) != 0 {
		t.Fatalf("CA-89: esqueleto invalido: %+v", f)
	}
	if _, err := Init(root); err == nil || !strings.Contains(err.Error(), "ya existe") {
		t.Fatalf("CA-89: no debe pisar un archivo existente: %v", err)
	}
}

// CA-91: regresion mas alla de la tolerancia = FAIL con metrica, valor,
// base y delta exactos; la base no se mueve.
func TestCA91_RegresionEsRoja(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, map[string]*Metric{
		"cobertura": {Cmd: "echo 75", Direction: "up", Value: fp(80)},
	})
	res, f := gateOn(t, root)
	if res.Status != verdict.StatusFail {
		t.Fatalf("CA-91: esperaba FAIL: %+v", res)
	}
	for _, want := range []string{"cobertura", "75", "80", "-5", "REGRESION"} {
		if !strings.Contains(res.OutputTail, want) {
			t.Fatalf("CA-91: la evidencia debe incluir %q:\n%s", want, res.OutputTail)
		}
	}
	if *f.Metrics["cobertura"].Value != 80 {
		t.Fatalf("CA-91: una regresion jamas mueve la base: %+v", f.Metrics["cobertura"])
	}
}

// CA-92: mejora mas alla de la tolerancia aprieta la base y lo registra.
func TestCA92_MejoraAprieta(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, map[string]*Metric{
		"cobertura": {Cmd: "echo 75", Direction: "up", Value: fp(70)},
	})
	res, f := gateOn(t, root)
	if res.Status != verdict.StatusPass || !strings.Contains(res.OutputTail, "apretada") {
		t.Fatalf("CA-92: esperaba pass con apriete: %+v", res)
	}
	if *f.Metrics["cobertura"].Value != 75 {
		t.Fatalf("CA-92: la base debia quedar en 75: %+v", f.Metrics["cobertura"])
	}
	last := f.History[len(f.History)-1]
	if last.Kind != "tightened" || *last.From != 70 || last.To != 75 {
		t.Fatalf("CA-92: falta el registro tightened 70->75: %+v", last)
	}
}

// CA-93: dentro de la tolerancia no se falla ni se aprieta (anti-ruido).
func TestCA93_ToleranciaNoMueve(t *testing.T) {
	for _, cmd := range []string{"echo 70.5", "echo 69.5"} {
		root := t.TempDir()
		writeFile(t, root, map[string]*Metric{
			"msi": {Cmd: cmd, Direction: "up", Tolerance: 1, Value: fp(70)},
		})
		res, f := gateOn(t, root)
		if res.Status != verdict.StatusPass {
			t.Fatalf("CA-93: %q dentro de tolerancia debe pasar: %+v", cmd, res)
		}
		if *f.Metrics["msi"].Value != 70 || len(f.History) != 0 {
			t.Fatalf("CA-93: %q no debe mover la base ni registrar nada: %+v", cmd, f)
		}
	}
}

// CA-94: direction down invierte la regla: subir es regresion, bajar aprieta.
func TestCA94_DireccionDown(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, map[string]*Metric{
		"deuda": {Cmd: "echo 7", Direction: "down", Value: fp(5)},
	})
	res, _ := gateOn(t, root)
	if res.Status != verdict.StatusFail || !strings.Contains(res.OutputTail, "REGRESION") {
		t.Fatalf("CA-94: subir una metrica down debe ser FAIL: %+v", res)
	}

	root = t.TempDir()
	writeFile(t, root, map[string]*Metric{
		"deuda": {Cmd: "echo 3", Direction: "down", Value: fp(5)},
	})
	res, f := gateOn(t, root)
	if res.Status != verdict.StatusPass || *f.Metrics["deuda"].Value != 3 {
		t.Fatalf("CA-94: bajar una metrica down debe apretar a 3: %+v %+v", res, f.Metrics["deuda"])
	}
}

// CA-96: comando roto = ERROR fail-closed y la base intacta.
func TestCA96_ComandoRoto(t *testing.T) {
	for cmd, want := range map[string]string{
		"exit 3":    "exit 3",
		"echo hola": "no es un numero",
	} {
		root := t.TempDir()
		writeFile(t, root, map[string]*Metric{
			"m": {Cmd: cmd, Direction: "up", Value: fp(50)},
		})
		res, f := gateOn(t, root)
		if res.Status != verdict.StatusError {
			t.Fatalf("CA-96: %q debe ser ERROR fail-closed: %+v", cmd, res)
		}
		if !strings.Contains(res.OutputTail, want) {
			t.Fatalf("CA-96: la evidencia debe incluir %q:\n%s", want, res.OutputTail)
		}
		if *f.Metrics["m"].Value != 50 {
			t.Fatalf("CA-96: un comando roto jamas toca la base: %+v", f.Metrics["m"])
		}
	}
}

// CA-97: lower exige razon, registra loosened, y rechaza "aflojar" hacia mejor.
func TestCA97_LowerConRegistro(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, map[string]*Metric{
		"cobertura": {Cmd: "echo 70", Direction: "up", Value: fp(70)},
	})
	if _, err := Lower(root, "cobertura", 60, "  "); err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("CA-97: sin razon debe negarse: %v", err)
	}
	ch, err := Lower(root, "cobertura", 60, "se elimino el modulo legacy con sus tests")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Kind != "loosened" || *ch.From != 70 || ch.To != 60 {
		t.Fatalf("CA-97: registro loosened incorrecto: %+v", ch)
	}
	f := reload(t, root)
	if *f.Metrics["cobertura"].Value != 60 || f.History[len(f.History)-1].Reason == "" {
		t.Fatalf("CA-97: la base y la razon deben persistir: %+v", f)
	}
	if _, err := Lower(root, "cobertura", 65, "quiero subirla"); err == nil || !strings.Contains(err.Error(), "apretar") {
		t.Fatalf("CA-97: mover hacia mejor no es aflojar: %v", err)
	}
	if _, err := Lower(root, "inexistente", 1, "x"); err == nil {
		t.Fatalf("CA-97: metrica inexistente debe ser error")
	}
}
