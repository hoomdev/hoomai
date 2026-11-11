// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// taskcmd: el tope de pendientes en Ready (CA-633) y la linea de pendientes
// de `hoom task done` (CA-631). Escritos desde el spec y las firmas
// publicas, sin leer la implementacion.
package taskcmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const crSlug = "precios"

func crTope(slug string, m, n int) string {
	return fmt.Sprintf("aceptar %q dejaria %d medium abiertos en el proyecto (tope %d): corrige o refuta alguno (hoom finding list --open) o sube findings.max_pendientes", slug, m, n)
}

// crHallazgo registra un hallazgo en dir (sin commitear).
func crHallazgo(t *testing.T, dir, sev, task string) finding.Finding {
	t.Helper()
	h := "huella"
	f, err := finding.Register(dir, "main", finding.Draft{Severity: sev, Lens: "risk", File: "app.go",
		Description: "hallazgo " + sev, Author: "reviewer@test", Task: task, Huella: &h})
	if err != nil {
		t.Fatalf("fixture: Register: %v", err)
	}
	return f
}

// crLista arma la tarea con el hoom.yaml de su arbol (findings extra), los
// hallazgos que arma poner, y un verde commiteado: con todo en orden salvo
// el tope, Done la cerraria.
func crLista(t *testing.T, findingsYAML string, poner func(wt string)) (root, wt string) {
	t.Helper()
	root, wt = rdTarea(t, crSlug)
	rdEscribir(t, wt, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+findingsYAML)
	rdEscribir(t, wt, "precios.go", "package app\n")
	if poner != nil {
		poner(wt)
	}
	rdCommit(t, wt, "codigo, hoom.yaml y hallazgos")
	rdVeredicto(t, wt, time.Now().UTC(), verdict.StatusPass, false, "")
	rdCommit(t, wt, "verde")
	return root, wt
}

func crReadyTope(t *testing.T, ca, root string, m, n int) {
	t.Helper()
	err := Ready(root, crSlug, "main")
	var re *ReadyError
	if err == nil || !errors.As(err, &re) || re.Kind != ReadyTopePendientes || err.Error() != crTope(crSlug, m, n) {
		t.Fatalf("%s: Ready falla con %q y kind %q: %v (%+v)", ca, crTope(crSlug, m, n), ReadyTopePendientes, err, re)
	}
	if derr := Done(root, crSlug, "main", false); derr == nil || derr.Error() != err.Error() {
		t.Fatalf("%s: task done rechaza con el mismo mensaje: %v", ca, derr)
	}
}

func crReadyOK(t *testing.T, ca, root string) {
	t.Helper()
	if err := Ready(root, crSlug, "main"); err != nil {
		t.Fatalf("%s: Ready pasa: %v", ca, err)
	}
}

// CA-633: con max_pendientes n, la tarea con un medium propio y el arbol con
// mas de n medium abiertos (de cualquier tarea o sin tarea) falla con el
// texto del contrato.
func TestCA633_ReadyFallaSobreElTope(t *testing.T) {
	root, _ := crLista(t, "findings:\n  block_on: high\n  max_pendientes: 1\n", func(wt string) {
		crHallazgo(t, wt, "medium", crSlug)
		crHallazgo(t, wt, "medium", "otra")
		crHallazgo(t, wt, "medium", "")
		crHallazgo(t, wt, "low", crSlug) // un low no cuenta
	})
	crReadyTope(t, "CA-633", root, 3, 1)
	// --force lo salta
	if err := Done(root, crSlug, "main", true); err != nil {
		t.Fatalf("CA-633: --force salta el tope: %v", err)
	}
}

// CA-633 (caso limite: max_pendientes: 0): ninguna tarjeta con un medium
// abierto propio se puede aceptar.
func TestCA633_TopeCero(t *testing.T) {
	root, _ := crLista(t, "findings:\n  block_on: high\n  max_pendientes: 0\n", func(wt string) {
		crHallazgo(t, wt, "medium", crSlug)
	})
	crReadyTope(t, "CA-633", root, 1, 0)
}

// CA-633: pasa cuando no pasa de n.
func TestCA633_NoPasaDelTope(t *testing.T) {
	root, _ := crLista(t, "findings:\n  block_on: high\n  max_pendientes: 2\n", func(wt string) {
		crHallazgo(t, wt, "medium", crSlug)
		crHallazgo(t, wt, "medium", "otra")
	})
	crReadyOK(t, "CA-633", root)
	if err := Done(root, crSlug, "main", false); err != nil {
		t.Fatalf("CA-633: Done cierra: %v", err)
	}
}

// CA-633: una tarea sin medium abiertos propios pasa aunque el arbol este
// sobre el tope (es lo que deja aceptar una limpieza); un medium propio
// cerrado no es abierto.
func TestCA633_SinMediumPropioPasa(t *testing.T) {
	root, _ := crLista(t, "findings:\n  block_on: high\n  max_pendientes: 0\n", func(wt string) {
		crHallazgo(t, wt, "medium", "otra")
		crHallazgo(t, wt, "medium", "")
		crHallazgo(t, wt, "low", crSlug)
		cerrado := crHallazgo(t, wt, "medium", crSlug)
		if _, err := finding.Resolve(wt, cerrado.ID, "corregido", "el commit abc lo arregla", "Ana"); err != nil {
			t.Fatal(err)
		}
	})
	crReadyOK(t, "CA-633", root)
}

// CA-633: sin la clave no hay tope.
func TestCA633_SinLaClaveNoHayTope(t *testing.T) {
	root, _ := crLista(t, "findings:\n  block_on: high\n", func(wt string) {
		for i := 0; i < 5; i++ {
			crHallazgo(t, wt, "medium", crSlug)
		}
	})
	crReadyOK(t, "CA-633", root)
	root, _ = crLista(t, "", func(wt string) {
		crHallazgo(t, wt, "medium", crSlug)
	})
	crReadyOK(t, "CA-633 (sin findings)", root)
}

// CA-633: el tope lee el arbol de la tarea, no el checkout principal: los
// medium del proyecto que no estan en el arbol de la tarea no cuentan, y el
// hoom.yaml del proyecto sin la clave no quita el del arbol.
func TestCA633_LeeElArbolDeLaTarea(t *testing.T) {
	root, _ := crLista(t, "findings:\n  block_on: high\n  max_pendientes: 1\n", func(wt string) {
		crHallazgo(t, wt, "medium", crSlug)
	})
	// medium en el checkout principal: no estan en el arbol de la tarea
	crHallazgo(t, root, "medium", "otra")
	crHallazgo(t, root, "medium", "")
	crReadyOK(t, "CA-633", root)
}

// CA-633: el tope es el ultimo chequeo: un rojo se dice antes.
func TestCA633_ElTopeVaAlFinal(t *testing.T) {
	root, wt := crLista(t, "findings:\n  block_on: high\n  max_pendientes: 0\n", func(wt string) {
		crHallazgo(t, wt, "medium", crSlug)
	})
	rojo := rdVeredicto(t, wt, time.Now().UTC().Add(time.Second), verdict.StatusFail, false, "")
	rdCommit(t, wt, "rojo")
	err := Ready(root, crSlug, "main")
	var re *ReadyError
	if err == nil || !errors.As(err, &re) || re.Kind != ReadyRojo || !strings.Contains(err.Error(), rojo.ID) {
		t.Fatalf("CA-633: con un rojo, el error es el del rojo: %v", err)
	}
}

// ---------------------------------------------------------------- CA-631 (binario)

// crTareaConHallazgos es tdTareaLista con los hallazgos que poner registra en
// el worktree antes del verify (con block_on high un medium y un low no lo
// ponen rojo).
func crTareaConHallazgos(t *testing.T, bin, slug string, poner func(wt string)) (root, wt string) {
	t.Helper()
	root, wt = tdTareaLista(t, bin, slug)
	if poner != nil {
		poner(wt)
		rdCommit(t, wt, "hallazgos")
	}
	return root, wt
}

// CA-631: `hoom task done` imprime los pendientes de la tarea, despues de la
// linea de cierre.
func TestCA631_TaskDoneImprimeLosPendientes(t *testing.T) {
	bin := tdBinario(t)
	root, _ := crTareaConHallazgos(t, bin, "precios", func(wt string) {
		crHallazgo(t, wt, "medium", "precios")
		crHallazgo(t, wt, "low", "precios")
		crHallazgo(t, wt, "low", "precios")
		crHallazgo(t, wt, "medium", "otra") // de otra tarea: no es pendiente de esta
	})
	rdEscribir(t, root, ".hoom/items/precios.yaml", tdItemYAML("Precios", ""))
	r := tdCorrer(t, bin, root, "task", "done", "precios")
	tdOK(t, "CA-631", r, "task", "done", "precios")
	want := "pendientes: 1 medium, 2 low abiertos (hoom finding list --open)"
	if !strings.Contains(r.stdout, want) {
		t.Fatalf("CA-631: task done imprime %q:\n%s", want, r.stdout)
	}
	if strings.Contains(r.stdout, "sin pendientes") {
		t.Fatalf("CA-631: con pendientes no dice `sin pendientes`:\n%s", r.stdout)
	}
	crDespuesDelCierre(t, r.stdout, want)
}

// CA-631: solo low: nombra solo las severidades presentes.
func TestCA631_SoloLow(t *testing.T) {
	bin := tdBinario(t)
	root, _ := crTareaConHallazgos(t, bin, "precios", func(wt string) {
		crHallazgo(t, wt, "low", "precios")
	})
	r := tdCorrer(t, bin, root, "task", "done", "precios")
	tdOK(t, "CA-631", r, "task", "done", "precios")
	if !strings.Contains(r.stdout, "pendientes: 1 low abiertos (hoom finding list --open)") || strings.Contains(r.stdout, "0 medium") {
		t.Fatalf("CA-631: %s", r.stdout)
	}
}

// CA-631: sin pendientes imprime `sin pendientes`; un hallazgo cerrado no es
// pendiente.
func TestCA631_SinPendientes(t *testing.T) {
	bin := tdBinario(t)
	root, _ := crTareaConHallazgos(t, bin, "precios", func(wt string) {
		f := crHallazgo(t, wt, "medium", "precios")
		if _, err := finding.Resolve(wt, f.ID, "refutado", "el test TestX lo tumba", "Ana"); err != nil {
			t.Fatal(err)
		}
	})
	r := tdCorrer(t, bin, root, "task", "done", "precios")
	tdOK(t, "CA-631", r, "task", "done", "precios")
	if !crTieneLinea(r.stdout, "sin pendientes") || strings.Contains(r.stdout, "abiertos (hoom finding list --open)") {
		t.Fatalf("CA-631: sin pendientes imprime `sin pendientes`:\n%s", r.stdout)
	}
	crDespuesDelCierre(t, r.stdout, "sin pendientes")
}

func crTieneLinea(out, l string) bool {
	for _, x := range strings.Split(out, "\n") {
		if strings.TrimSpace(x) == l {
			return true
		}
	}
	return false
}

// crDespuesDelCierre exige que la linea de pendientes sea una sola y no la
// primera: va justo despues de la linea de cierre.
func crDespuesDelCierre(t *testing.T, out, linea string) {
	t.Helper()
	lineas := strings.Split(strings.TrimRight(out, "\n"), "\n")
	n, en := 0, -1
	for i, l := range lineas {
		if strings.Contains(l, linea) {
			n++
			en = i
		}
	}
	if n != 1 || en < 1 {
		t.Fatalf("CA-631: la linea %q sale una vez, despues de la linea de cierre:\n%s", linea, out)
	}
}
