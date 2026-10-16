// Tests de regresion del hallazgo af25ed de la review delta 1 de las
// correcciones de C3 (.hoom/specs/acciones-desde-la-tarjeta.md, CA-346 y su
// enmienda 1), escritos desde el contrato y el hallazgo, sin leer la
// implementacion: "si al repositorio le falta el contenido de HEAD de una
// ruta, se niega sin tocar nada". Vale tambien para el nombre viejo de un
// rename (git mv), que es una ruta a restaurar aunque ya no este en el disco
// ni en el indice: antes de tocar nada se comprueba que el repositorio tenga
// el contenido de HEAD de CADA ruta a restaurar.
//
// Reusa h3Tarea, h3Rel, h3Foto, h3Cambios, h3SinObjeto y h3Evidencia
// (hallazgos_c3_test.go) y cbGit, cbEscribir, cbLeer, cbMismos y cbSlug
// (discard_test.go).
package taskcmd

import (
	"regexp"
	"strings"
	"testing"
)

// Contenidos unicos: el blob de a.go (en HEAD) y el de b.go no los comparte
// ningun otro archivo del fixture, asi que quitar el de a.go no le quita el
// contenido a nadie mas.
const (
	afA        = "package app\n\n// af25ed: contenido de a.go en HEAD\n"
	afB        = "package app\n\n// af25ed: contenido de b.go en HEAD\n"
	afBTrabajo = "package app\n\n// af25ed: trabajo sin guardar en b.go\n"
)

// afNombraA dice si el mensaje nombra la ruta a.go (sola o como
// .hoom/worktrees/precios/a.go), no como parte de otro nombre (a2.go,
// data.go).
var afNombraA = regexp.MustCompile(`(^|[^A-Za-z0-9_.-])a\.go($|[^A-Za-z0-9_.-])`)

// afTarea arma la tarea con a.go y b.go en HEAD, hace `git mv a.go a2.go` y
// modifica b.go (sin agregarlo al indice) en el espacio de trabajo.
func afTarea(t *testing.T) (root, wt string) {
	t.Helper()
	root, wt = h3Tarea(t, map[string]string{"a.go": afA, "b.go": afB}, nil)
	cbGit(t, wt, "mv", "a.go", "a2.go")
	cbEscribir(t, wt, "b.go", afBTrabajo)
	return root, wt
}

// afDescartable es lo que la tarjeta muestra como descartable (Discardable
// tal cual), que es lo que manda el Studio. Tiene que traer a2.go y b.go (y,
// si la tarjeta lo muestra, el nombre viejo a.go).
func afDescartable(t *testing.T, root string) []string {
	t.Helper()
	vio, err := Discardable(root, cbSlug)
	if err != nil {
		t.Fatalf("CA-346: fixture: Discardable con el rename y b.go modificado: %v", err)
	}
	tiene := map[string]bool{}
	for _, p := range vio {
		tiene[p] = true
	}
	if !tiene[h3Rel("a2.go")] || !tiene[h3Rel("b.go")] {
		t.Fatalf("CA-346: fixture: lo descartable trae el destino del rename (a2.go) y b.go: %q", vio)
	}
	if !tiene[h3Rel("a.go")] {
		t.Logf("aviso: Discardable no muestra el origen del rename (a.go): %q", vio)
	}
	return vio
}

// Hallazgo af25ed, CA-346 (enmienda 1): en el espacio de trabajo, HEAD tiene
// a.go y b.go; `git mv a.go a2.go`, b.go modificado, y al repositorio le
// falta el contenido de HEAD de a.go (su blob suelto se quito del almacen de
// objetos). Descartar con exactamente lo que muestra la tarjeta se niega con
// un error que nombra a.go y no toca nada: b.go conserva su trabajo sin
// guardar, a2.go sigue ahi, y el status, el indice (modo, objeto y ruta de
// cada entrada) y el disco (tambien la evidencia bajo .hoom/) quedan
// identicos.
func TestHallazgo_af25ed_CA346_RenameSinContenidoDeHEADSeNiegaSinTocarNada(t *testing.T) {
	root, wt := afTarea(t)
	// h3SinObjeto exige que el blob este suelto (no empaquetado): lo borra
	// del almacen y comprueba que git cat-file -e HEAD:a.go ya no lo
	// encuentra en ningun lado.
	h3SinObjeto(t, root, wt, "a.go")
	vio := afDescartable(t, root)
	antes := h3Foto(t, wt)

	res, err := Discard(root, cbSlug, vio)

	despues := h3Foto(t, wt)
	bGot, bOk := cbLeer(t, wt, "b.go")
	a2Got, a2Ok := cbLeer(t, wt, "a2.go")
	if err == nil {
		t.Fatalf("hallazgo af25ed (CA-346): al repositorio le falta el contenido de HEAD de a.go (origen del rename): Descartar se niega con error; devolvio exito: restored %q removed %q\nb.go quedo %q (existe: %v), a2.go existe: %v\ncambios (- antes, + despues):\n%s", res.Restored, res.Removed, bGot, bOk, a2Ok, h3Cambios(antes, despues))
	}
	if !afNombraA.MatchString(err.Error()) {
		t.Errorf("hallazgo af25ed (CA-346): la negativa nombra la ruta cuyo contenido de HEAD falta (a.go): %v", err)
	}
	if !bOk || bGot != afBTrabajo {
		t.Errorf("hallazgo af25ed (CA-346): se niega SIN TOCAR NADA: b.go conserva su trabajo sin guardar %q, quedo %q (existe: %v) (error: %v)", afBTrabajo, bGot, bOk, err)
	}
	if !a2Ok || a2Got != afA {
		t.Errorf("hallazgo af25ed (CA-346): se niega SIN TOCAR NADA: a2.go (destino del rename) sigue ahi con %q, quedo %q (existe: %v) (error: %v)", afA, a2Got, a2Ok, err)
	}
	if despues != antes {
		t.Fatalf("hallazgo af25ed (CA-346): se niega sin tocar nada: el status, el indice y el disco quedan identicos (- antes, + despues):\n%s\n(error: %v)", h3Cambios(antes, despues), err)
	}
	h3Evidencia(t, "af25ed", wt)
}

// Hallazgo af25ed, CA-346 (guarda): el mismo caso con el repositorio sano
// (el blob de a.go esta) se descarta entero con lo que muestra la tarjeta:
// a.go vuelve a como esta en HEAD, a2.go desaparece, b.go vuelve a HEAD,
// fuera de .hoom/ no queda nada sin guardar y la evidencia sigue igual.
func TestHallazgo_af25ed_CA346_GuardaRenameConRepoSanoSeDescarta(t *testing.T) {
	root, wt := afTarea(t)
	vio := afDescartable(t, root)

	res, err := Discard(root, cbSlug, vio)
	if err != nil {
		t.Fatalf("hallazgo af25ed (CA-346): guarda: con el repositorio sano Descartar el rename y b.go funciona: %v", err)
	}
	if got, ok := cbLeer(t, wt, "a.go"); !ok || got != afA {
		t.Fatalf("hallazgo af25ed (CA-346): guarda: a.go vuelve a como esta en HEAD: %q (existe: %v)", got, ok)
	}
	if _, ok := cbLeer(t, wt, "a2.go"); ok {
		t.Fatal("hallazgo af25ed (CA-346): guarda: a2.go no esta en HEAD y desaparece")
	}
	if got, ok := cbLeer(t, wt, "b.go"); !ok || got != afB {
		t.Fatalf("hallazgo af25ed (CA-346): guarda: b.go vuelve a como esta en HEAD: %q (existe: %v)", got, ok)
	}
	for _, l := range strings.Split(cbGit(t, wt, "status", "--porcelain", "--untracked-files=all"), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(strings.TrimSpace(l[2:]), ".hoom/") {
			t.Fatalf("hallazgo af25ed (CA-346): guarda: despues de descartar, lo unico sin guardar es la evidencia: %q", l)
		}
	}
	if !cbMismos(res.Restored, []string{h3Rel("a.go"), h3Rel("b.go")}) || !cbMismos(res.Removed, []string{h3Rel("a2.go")}) {
		t.Fatalf("hallazgo af25ed (CA-346): guarda: el resultado dice lo que hizo: a.go y b.go (en HEAD) en restored, a2.go (no esta en HEAD) en removed: restored %q removed %q", res.Restored, res.Removed)
	}
	h3Evidencia(t, "af25ed", wt)
}
