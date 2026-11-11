// Tests de regresion de la ronda 2 de ciclo-de-review-acotado sobre `hoom
// item set` dentro del worktree de una tarea (CA-617). Principio: la
// comprobacion de Hecho mira el mismo item que se escribe, que es el que se
// lee (el del checkout principal cuando lo tiene); y la salida es la linea
// del contrato, igual que desde el principal. Escritos desde el spec, las
// firmas publicas y el texto de los hallazgos, sin leer la implementacion.
package itemcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const crR2Hecho = "la tarjeta ya esta en Hecho"

func crR2Cerrado() string {
	return icItemYAML("Precios", "2026-09-22T15:04:05Z",
		"hecho_en: 2026-09-23T10:00:00Z\ncommit_final: 3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c\n")
}

func crR2Leer(t *testing.T, dir, slug string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".hoom", "items", slug+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// crR2PrincipalCerrado: tarea iniciada; despues el item del principal se
// cierra (hecho_en, commit_final) y la copia del worktree sigue abierta.
func crR2PrincipalCerrado(t *testing.T) (root, wt string) {
	t.Helper()
	root, wt = crR1Tarea(t, "precios")
	icEscribir(t, root, ".hoom/items/precios.yaml", crR2Cerrado())
	crCommitearItems(t, root)
	return root, wt
}

// crR2WorktreeCerrado es el espejo: el principal abierto y la copia del
// worktree cerrada.
func crR2WorktreeCerrado(t *testing.T) (root, wt string) {
	t.Helper()
	root, wt = crR1Tarea(t, "precios")
	icEscribir(t, wt, ".hoom/items/precios.yaml", crR2Cerrado())
	crCommitearItems(t, wt)
	return root, wt
}

// hallazgo 20261111T212403_822b51 (CA-617): el item del principal esta
// cerrado y la copia del worktree no. itemcmd.Set dentro del worktree se
// niega con `la tarjeta ya esta en Hecho` y ningun item cambia. Espejo: con
// el principal abierto y la copia cerrada, el cambio se hace en el principal.
func TestHallazgo_822b51_SetMiraHechoEnElItemQueEscribe(t *testing.T) {
	root, wt := crR2PrincipalCerrado(t)
	antesP, antesW := crR2Leer(t, root, "precios"), crR2Leer(t, wt, "precios")
	_, err := Set(wt, "main", "high", "precios", "alto", "")
	if err == nil || !strings.Contains(err.Error(), crR2Hecho) {
		t.Fatalf("hallazgo 822b51 (CA-617): con el item del principal cerrado, Set desde el worktree se niega con %q: %v", crR2Hecho, err)
	}
	if crR2Leer(t, root, "precios") != antesP || crR2Leer(t, wt, "precios") != antesW {
		t.Fatal("hallazgo 822b51 (CA-617): la negativa no cambia ningun item")
	}

	root, wt = crR2WorktreeCerrado(t)
	antesW = crR2Leer(t, wt, "precios")
	res, err := Set(wt, "main", "high", "precios", "alto", "")
	if err != nil || res.Antes != "normal" || res.Riesgo != "alto" || !res.Cambio {
		t.Fatalf("hallazgo 822b51 (CA-617, espejo): con el principal abierto el cambio se hace como desde el principal: %+v %v", res, err)
	}
	if !strings.Contains(crR2Leer(t, root, "precios"), "riesgo: alto") {
		t.Fatalf("hallazgo 822b51 (espejo): el item del principal queda con riesgo alto:\n%s", crR2Leer(t, root, "precios"))
	}
	if crR2Leer(t, wt, "precios") != antesW {
		t.Fatal("hallazgo 822b51 (espejo): la copia del worktree no se toca")
	}
}

// hallazgo 20261111T212806_c55dde (CA-617), el mismo defecto por el
// binario: exit 1 con `la tarjeta ya esta en Hecho` sin tocar nada; y el
// espejo sale 0 y escribe en el principal.
func TestHallazgo_c55dde_ItemSetPorElBinarioMiraHechoEnElItemQueEscribe(t *testing.T) {
	bin := ixBinario(t)
	root, wt := crR2PrincipalCerrado(t)
	antesP, antesW := crR2Leer(t, root, "precios"), crR2Leer(t, wt, "precios")
	r := crCorrer(t, bin, wt, "", "item", "set", "precios", "--riesgo", "alto")
	if r.exit != 1 || !strings.Contains(r.stderr, crR2Hecho) {
		t.Fatalf("hallazgo c55dde (CA-617): exit 1 con %q: %d\n%s%s", crR2Hecho, r.exit, r.stdout, r.stderr)
	}
	if crR2Leer(t, root, "precios") != antesP || crR2Leer(t, wt, "precios") != antesW {
		t.Fatal("hallazgo c55dde (CA-617): la negativa no cambia ningun item")
	}

	root, wt = crR2WorktreeCerrado(t)
	antesW = crR2Leer(t, wt, "precios")
	r = crCorrer(t, bin, wt, "", "item", "set", "precios", "--riesgo", "alto")
	if r.exit != 0 || !strings.Contains(crR2Leer(t, root, "precios"), "riesgo: alto") || crR2Leer(t, wt, "precios") != antesW {
		t.Fatalf("hallazgo c55dde (CA-617, espejo): sale 0 y escribe en el principal: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
}

// hallazgo 20261111T212806_321462 (CA-617): dentro del worktree, stdout es
// EXACTAMENTE la linea del contrato y su salto de linea, igual que desde el
// principal (sin agregados como la ruta del proyecto).
func TestHallazgo_321462_LaLineaDesdeElWorktreeEsLaDelContrato(t *testing.T) {
	bin := ixBinario(t)
	const want = "riesgo de precios: normal -> alto - guardalo con: hoom item save precios\n"
	_, wt := crR1Tarea(t, "precios")
	r := crCorrer(t, bin, wt, "", "item", "set", "precios", "--riesgo", "alto")
	if r.exit != 0 || r.stdout != want {
		t.Fatalf("hallazgo 321462 (CA-617): desde el worktree stdout es exactamente %q: %d %q\n%s", want, r.exit, r.stdout, r.stderr)
	}
	root, _ := crR1Tarea(t, "precios")
	r = crCorrer(t, bin, root, "", "item", "set", "precios", "--riesgo", "alto")
	if r.exit != 0 || r.stdout != want {
		t.Fatalf("hallazgo 321462 (CA-617, guarda): desde el principal es la misma linea: %d %q", r.exit, r.stdout)
	}
}
