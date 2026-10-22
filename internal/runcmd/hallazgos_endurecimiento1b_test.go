// Tests de regresion de la segunda ronda de la tarea endurecimiento-1 en
// runcmd. Escritos desde el texto de cada hallazgo, las specs y el contrato
// que promete la correccion, SIN LEER LA IMPLEMENTACION: runs de verdad
// (Manager.Start) con el provider falso de la primera ronda (e1Proveedor:
// una linea que no es un evento JSON sale como text con la linea en el
// detail) y sidecars escritos a mano. Reusan los helpers e1* de
// hallazgos_endurecimiento1_test.go y dlg* de delegaciones_helpers_test.go;
// los de este archivo llevan el prefijo e1b.
//
//   - 20261022T202056_9e7983 y 20261022T203213_a2b180 (risk, resilience):
//     MetasErr omitia en silencio un sidecar <id>.meta.json que no podia
//     leer: con .hoom/runs listable, devolvia exito y el doctor certificaba
//     sin haber leido toda la evidencia. Ahora MetasErr devuelve error (y los
//     sidecars legibles igual); Metas sigue salteandolo sin error (CA-159,
//     CA-369).
//   - 20261022T202805_b7aba5 (reliability): el .jsonl se escribia con el
//     evento serializado (encoding/json cambia el UTF-8 invalido por U+FFFD)
//     pero en memoria quedaba el evento original con los bytes invalidos: lo
//     que el run tenia en memoria (Manager.Events) no era lo que ReadEvents
//     leia del .jsonl. Memoria y disco tienen exactamente los mismos eventos
//     (CA-362).
package runcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- 9e7983 / a2b180

// CA-159, CA-369 (hallazgos 9e7983 y a2b180, MetasErr): con .hoom/runs
// listable y el sidecar <id>.meta.json de un run ilegible (000) al lado de
// otro legible, MetasErr devuelve error (no pudo leer toda la evidencia) y,
// aun asi, trae el sidecar legible. Metas (best-effort) sigue sin error:
// trae el legible y saltea el ilegible. Guarda: con los dos legibles,
// MetasErr trae los dos sin error.
func TestHallazgo_9e7983_a2b180_CA159_MetasErrDiceQueUnSidecarNoSePudoLeer(t *testing.T) {
	const ca = "CA-159 (hallazgos 9e7983 y a2b180, MetasErr con un sidecar ilegible)"
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("sin permisos de unix (o como root) un chmod no quita la lectura")
	}
	const ilegible = "20261022T100700_e1b0c3"
	root := t.TempDir()
	runs := filepath.Join(root, ".hoom", "runs")
	e1Run(t, runs, e1IDPropio, "propio", "trabajo propio")
	e1Run(t, runs, ilegible, "ilegible", "trabajo ilegible")
	if ms, err := MetasErr(root); err != nil || len(ms) != 2 {
		t.Fatalf("%s: guarda: con los dos sidecars legibles, MetasErr trae los dos sin error: %v %v", ca, e1IDs(ms), err)
	}

	sidecar := filepath.Join(runs, ilegible+".meta.json")
	if err := os.Chmod(sidecar, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sidecar, 0o644) })
	if _, err := os.ReadFile(sidecar); err == nil {
		t.Skip("este sistema deja leer un archivo sin permiso de lectura")
	}
	if _, err := os.ReadDir(runs); err != nil {
		t.Fatalf("fixture: .hoom/runs sigue listable: %v", err)
	}

	ms, err := MetasErr(root)
	if err == nil {
		t.Errorf("%s: con el sidecar %s ilegible (000), MetasErr devuelve error: no pudo leer toda la evidencia; dio %v sin error",
			ca, ilegible, e1IDs(ms))
	}
	if len(ms) != 1 || ms[0].ID != e1IDPropio {
		t.Errorf("%s: MetasErr trae igual el sidecar legible (%s) y solo ese: %v", ca, e1IDPropio, e1IDs(ms))
	}
	if ls := Metas(root); len(ls) != 1 || ls[0].ID != e1IDPropio {
		t.Errorf("%s: Metas (best-effort) saltea el ilegible y trae el legible: %v", ca, e1IDs(ls))
	}
}

// ---------------------------------------------------------------- b7aba5

// e1bMarca esta en el texto de la linea con UTF-8 invalido.
const e1bMarca = "e1b-binario"

// e1bMismos exige que la memoria y el disco tengan exactamente los mismos
// eventos, en el mismo orden, y que alguno traiga la marca.
func e1bMismos(t *testing.T, ca string, mem, disco []Event) {
	t.Helper()
	if e1Indice(mem, e1bMarca) < 0 {
		t.Fatalf("%s: fixture: el run narra la linea con UTF-8 invalido (detail con %q):\n%s", ca, e1bMarca, e1Resumen(mem))
	}
	if len(mem) != len(disco) {
		t.Fatalf("%s: memoria y disco tienen los mismos eventos: %d en memoria (Manager.Events), %d en el .jsonl (ReadEvents)\nmemoria:\n%sdisco:\n%s",
			ca, len(mem), len(disco), e1Resumen(mem), e1Resumen(disco))
	}
	for i := range mem {
		if !dlgMismoEvento(mem[i], disco[i]) {
			t.Errorf("%s: el evento %d en memoria (Manager.Events) es exactamente el que ReadEvents lee del .jsonl:\n  memoria %q\n  disco   %q",
				ca, i, mem[i].Detail, disco[i].Detail)
		}
	}
}

// CA-362 (hallazgo b7aba5): el provider emite por stdout una linea con UTF-8
// invalido (los bytes 0xff 0xfe dentro de un texto) que el normalizador
// pasa tal cual al detail de un evento text. Lo que el run deja en memoria
// (Manager.Events) es exactamente lo que ReadEvents lee del .jsonl, evento
// por evento: un consumidor que relee el .jsonl ve el mismo detail que vio
// el run en vivo.
func TestHallazgo_b7aba5_CA362_UTF8InvalidoEnStdoutMemoriaYDiscoIguales(t *testing.T) {
	const ca = "CA-362 (hallazgo b7aba5, UTF-8 invalido por stdout)"
	script := "printf '" + e1bMarca + " antes \\377\\376 despues\\n'\nexit 0\n"
	root, id, fin, _, mem := e1Correr(t, script, time.Minute)
	if fin.Status == StatusRunning {
		t.Fatalf("%s: fixture: el run del provider falso cierra: %+v", ca, fin)
	}
	i := e1Indice(mem, e1bMarca)
	if i >= 0 && !strings.Contains(mem[i].Detail, "antes") {
		t.Fatalf("%s: fixture: el detail trae la linea: %q", ca, mem[i].Detail)
	}
	disco, err := ReadEvents(root, id)
	if err != nil {
		t.Fatalf("%s: ReadEvents lee la narracion del run: %v", ca, err)
	}
	e1bMismos(t, ca, mem, disco)
}

// CA-362 (hallazgo b7aba5, stderr): lo mismo con la linea de UTF-8
// invalido por stderr, que hoom tambien narra: memoria y disco iguales.
func TestHallazgo_b7aba5_CA362_UTF8InvalidoEnStderrMemoriaYDiscoIguales(t *testing.T) {
	const ca = "CA-362 (hallazgo b7aba5, UTF-8 invalido por stderr)"
	script := "printf '" + e1bMarca + " antes \\377\\376 despues\\n' >&2\nexit 0\n"
	root, id, fin, _, mem := e1Correr(t, script, time.Minute)
	if fin.Status == StatusRunning {
		t.Fatalf("%s: fixture: el run del provider falso cierra: %+v", ca, fin)
	}
	disco, err := ReadEvents(root, id)
	if err != nil {
		t.Fatalf("%s: ReadEvents lee la narracion del run: %v", ca, err)
	}
	e1bMismos(t, ca, mem, disco)
}
