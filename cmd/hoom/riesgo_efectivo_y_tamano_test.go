// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md por el
// binario real (CA-776, CA-781): `hoom verify` imprime `riesgo:` y `tamano:`
// debajo de `cambio:`, el aviso de 400 lineas de producto solo con efectivo
// normal, el de mayor que el declarado con el slug de la tarea, y nunca el
// viejo `cambio grande (>400 lineas)`; con una seccion `rutas` invalida falla
// como cualquier error del manifiesto, sin correr gates ni escribir un
// veredicto. Escritos desde el spec, las aclaraciones de la ronda 0 y las
// firmas publicas, sin leer la implementacion.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func retyLineas(n int, marca string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(marca + " " + strings.Repeat("x", i%5) + "\n")
	}
	return b.String()
}

// retyRama es veeRepo con una rama tarea que agrega archivos (ruta: lineas),
// commiteados, y con la rama ya activa.
func retyRama(t *testing.T, extra string, archivos map[string]int) string {
	t.Helper()
	dir := veeRepo(t, extra, veeGate("test", "true"))
	qcGit(t, dir, "checkout", "-q", "-b", "tarea")
	for ruta, n := range archivos {
		veeEscribir(t, dir, ruta, retyLineas(n, "l"), 0o644)
	}
	qcGit(t, dir, "add", "-A")
	qcGit(t, dir, "commit", "-q", "-m", "la tarea")
	return dir
}

// retyLinea es el indice de la primera linea que, sin los blancos de los
// bordes, empieza con p; -1 si ninguna.
func retyLinea(out, p string) int {
	for i, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), p) {
			return i
		}
	}
	return -1
}

const (
	retyViejo  = "cambio grande (>400 lineas)"
	retyAviso4 = "! mas de 400 lineas de producto: la review exige las 4 lentes"
)

// CA-781: sin tarea y con una ruta de riesgo, riesgo alto con el motivo, el
// tamano en tres numeros, y ningun aviso de 400 lineas (ni el viejo).
func TestCA781_VerifyConRutaDeRiesgo(t *testing.T) {
	const ca = "CA-781"
	veeSaltar(t)
	bin := qcHoom(t)
	dir := retyRama(t, "", map[string]int{"internal/auth/login.go": 500, "internal/auth/login_test.go": 20})
	r := qcCorrer(t, bin, dir, nil, "verify")
	if r.exit != 0 {
		t.Fatalf("%s: hoom verify verde: %d\n%s", ca, r.exit, r.todo())
	}
	c := retyLinea(r.stdout, "cambio:")
	ri := retyLinea(r.stdout, "riesgo: alto (el cambio toca internal/auth/login.go: riesgo alto)")
	ta := retyLinea(r.stdout, "tamano: producto 500 · pruebas 20 · hoom ")
	if ri < 0 || ta < 0 || ri > ta || (c >= 0 && c > ri) {
		t.Fatalf("%s: riesgo y tamano debajo de cambio: (%d, %d, %d)\n%s", ca, c, ri, ta, r.stdout)
	}
	if strings.Contains(r.todo(), retyViejo) || strings.Contains(r.todo(), retyAviso4) {
		t.Fatalf("%s: con efectivo alto no sale ningun aviso de 400 lineas:\n%s", ca, r.todo())
	}
	if strings.Contains(r.todo(), "es mayor que el declarado") {
		t.Fatalf("%s: sin tarea no hay aviso de mayor que el declarado:\n%s", ca, r.todo())
	}
}

// CA-781 (calibrado por CAL-2): sin tarea y sin ruta de riesgo, 450 lineas de
// producto mantienen riesgo normal sin aviso de 4 lentes; 3000 de pruebas y 10
// de producto, tampoco.
func TestCA781_VerifyAvisoDeProducto(t *testing.T) {
	const ca = "CA-781"
	veeSaltar(t)
	bin := qcHoom(t)
	dir := retyRama(t, "", map[string]int{"precios.go": 450})
	r := qcCorrer(t, bin, dir, nil, "verify")
	if r.exit != 0 || retyLinea(r.stdout, "riesgo: normal (sin tarjeta: riesgo normal)") < 0 ||
		retyLinea(r.stdout, "tamano: producto 450 · pruebas 0 · hoom ") < 0 {
		t.Fatalf("%s: 450 de producto: riesgo normal y tamano: %d\n%s", ca, r.exit, r.todo())
	}
	if strings.Contains(r.todo(), retyViejo) || strings.Contains(r.todo(), retyAviso4) {
		t.Fatalf("%s: en riesgo normal ya no sale el aviso de 4 lentes ni el viejo:\n%s", ca, r.todo())
	}
	dir = retyRama(t, "", map[string]int{"precios.go": 10, "precios_test.go": 3000})
	r = qcCorrer(t, bin, dir, nil, "verify")
	if r.exit != 0 || retyLinea(r.stdout, "tamano: producto 10 · pruebas 3000 · hoom ") < 0 {
		t.Fatalf("%s: 10 de producto y 3000 de pruebas: %d\n%s", ca, r.exit, r.todo())
	}
	if strings.Contains(r.todo(), retyViejo) || strings.Contains(r.todo(), retyAviso4) {
		t.Fatalf("%s: las pruebas no dan aviso de tamano:\n%s", ca, r.todo())
	}
}

// CA-781 y aclaracion 11: en el worktree de una tarea que declara normal y
// toca una ruta de riesgo, el aviso de mayor que el declarado nombra el slug.
func TestCA781_VerifyEnLaTareaAvisaMayorQueDeclarado(t *testing.T) {
	const ca = "CA-781"
	veeSaltar(t)
	bin := qcHoom(t)
	dir := veeRepo(t, "", veeGate("test", "true"))
	veeEscribir(t, dir, ".hoom/items/precios.yaml", "titulo: Precios\ntipo: feature\nprioridad: media\n"+
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-11-20T10:00:00Z\n", 0o644)
	qcGit(t, dir, "add", "-A")
	qcGit(t, dir, "commit", "-q", "-m", "item")
	wt := filepath.Join(dir, ".hoom", "worktrees", "precios")
	qcGit(t, dir, "worktree", "add", "-q", "-b", "hoom/precios", wt)
	veeEscribir(t, wt, "internal/auth/token.go", retyLineas(30, "t"), 0o644)
	r := qcCorrer(t, bin, wt, nil, "verify")
	if r.exit != 0 {
		t.Fatalf("%s: hoom verify verde en la tarea: %d\n%s", ca, r.exit, r.todo())
	}
	for _, l := range []string{
		"riesgo: alto (el cambio toca internal/auth/token.go: riesgo alto)",
		"tamano: producto 30 · pruebas 0 · hoom ",
		"! el riesgo efectivo (alto) es mayor que el declarado (normal): hoom item set precios --riesgo alto lo sube",
	} {
		if retyLinea(r.stdout, l) < 0 {
			t.Fatalf("%s: falta %q:\n%s", ca, l, r.stdout)
		}
	}
	if retyLinea(r.stdout, "riesgo: ") > retyLinea(r.stdout, "! el riesgo efectivo") {
		t.Fatalf("%s: el aviso va despues de las lineas:\n%s", ca, r.stdout)
	}
}

// CA-776: una seccion `rutas` invalida hace fallar `hoom verify` como
// cualquier error del manifiesto: el texto exacto, sin correr gates ni
// escribir un veredicto.
func TestCA776_VerifyConRutasInvalidas(t *testing.T) {
	const ca = "CA-776"
	veeSaltar(t)
	bin := qcHoom(t)
	for _, k := range []struct{ rutas, want string }{
		{"rutas:\n  de_riesgo: \"internal/auth\"\n", "rutas.de_riesgo: tiene que ser una lista de patrones"},
		{"rutas:\n  generadas: [\"[x\"]\n", `rutas.generadas: patron invalido "[x"`},
		{"rutas:\n  de_bajo_riesgo: [\"a//b\"]\n", `rutas.de_bajo_riesgo: patron invalido "a//b"`},
		{"rutas:\n  de_riesgos: []\n", `rutas: clave desconocida "de_riesgos" (validas: de_riesgo, de_bajo_riesgo, generadas)`},
	} {
		dir := veeRepo(t, k.rutas, veeGate("test", "touch corrio"))
		r := qcCorrer(t, bin, dir, nil, "verify")
		if r.exit == 0 || !strings.Contains(r.stderr, k.want) {
			t.Fatalf("%s: %q: verify falla con %q: %d\n%s", ca, k.rutas, k.want, r.exit, r.todo())
		}
		if _, err := os.Stat(filepath.Join(dir, "corrio")); err == nil {
			t.Fatalf("%s: %q: no corre ningun gate", ca, k.rutas)
		}
		if es, _ := os.ReadDir(filepath.Join(dir, ".hoom", "verdicts")); len(es) != 0 {
			t.Fatalf("%s: %q: no escribe un veredicto: %v", ca, k.rutas, es)
		}
	}
	// rutas: null cuenta como sin la seccion
	dir := veeRepo(t, "rutas: null\n", veeGate("test", "true"))
	if r := qcCorrer(t, bin, dir, nil, "verify"); r.exit != 0 {
		t.Fatalf("%s: rutas: null es sin la seccion: %d\n%s", ca, r.exit, r.todo())
	}
}
