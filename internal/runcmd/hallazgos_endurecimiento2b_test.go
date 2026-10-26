// Tests de regresion de la segunda ronda de endurecimiento-2 en runcmd,
// sobre .hoom/specs/evidencia-en-disco.md (CA-442) y el doctor del tablero
// (hallazgo d0d331). Escritos desde el texto del hallazgo y el contrato de la
// correccion, SIN LEER LA IMPLEMENTACION. Reusan e2Valido, e2Roto, e2Sidecar,
// e2MetaJSON e e2IDs (hallazgos_endurecimiento2_test.go); los helpers de
// este archivo llevan el prefijo e2b.
//
//   - 20261026T20*_be6933 (high; correccion incompleta de 64323b): MetasErr
//     daba error por un sidecar que no era JSON o no tenia id, pero un JSON
//     valido con el id de su archivo y un campo de otro tipo (por ejemplo
//     "created_at": 7) no se podia leer como meta y se omitia sin error: el
//     doctor certificaba "sin problemas" sin haber leido ese sidecar. Ahora es
//     un error, y lo legible se sigue devolviendo. Un meta entero de otro id,
//     o con un estado que hoom no escribe, se sigue salteando (los guardas de
//     la primera ronda, en hallazgos_endurecimiento2_test.go).
package runcmd

import (
	"encoding/json"
	"testing"
)

// e2bConCampo es el JSON de un meta de hoom con id e2Roto y estado "done", y
// el campo campo reemplazado por valor (JSON crudo).
func e2bConCampo(t *testing.T, campo, valor string) []byte {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(e2MetaJSON(t, e2Roto, "done"), &m); err != nil {
		t.Fatal(err)
	}
	m[campo] = json.RawMessage(valor)
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// CA-442 (hallazgo be6933): con un sidecar legible y, al lado,
// .hoom/runs/<id>.meta.json que es JSON valido, con el id de su archivo y un
// estado que hoom escribe, pero con un campo de otro tipo (created_at
// numero o texto que no es una fecha, ended_at texto, exit_code y pid texto,
// usage numero, isolated texto), MetasErr devuelve error (no pudo leer ese
// meta: el doctor no puede certificar que no hay problemas) y, aun asi,
// devuelve el meta legible. Metas (best-effort) sigue sin error y trae el
// legible.
func TestHallazgo_be6933_CA442_UnMetaConUnCampoDeOtroTipoEsUnErrorDeMetasErr(t *testing.T) {
	const ca = "CA-442 (hallazgo be6933, meta con un campo de otro tipo)"
	for _, cs := range []struct{ campo, valor string }{
		{"created_at", `7`},
		{"created_at", `"no es una fecha"`},
		{"ended_at", `"ayer"`},
		{"exit_code", `"cero"`},
		{"pid", `"abc"`},
		{"usage", `5`},
		{"isolated", `"si"`},
	} {
		t.Run(cs.campo+"="+cs.valor, func(t *testing.T) {
			root := t.TempDir()
			e2Sidecar(t, root, e2Valido+".meta.json", e2MetaJSON(t, e2Valido, "done"))
			e2Sidecar(t, root, e2Roto+".meta.json", e2bConCampo(t, cs.campo, cs.valor))

			ms, err := MetasErr(root)
			if err == nil {
				t.Errorf("%s: .hoom/runs/%s.meta.json es JSON valido con el id de su archivo y \"%s\": %s (de otro tipo): no se puede leer como meta, y MetasErr devuelve error, no %v sin error",
					ca, e2Roto, cs.campo, cs.valor, e2IDs(ms))
			}
			if ids := e2IDs(ms); len(ids) != 1 || ids[0] != e2Valido {
				t.Errorf("%s: con \"%s\": %s al lado, MetasErr devuelve igual el meta legible (%s) y solo ese: %v", ca, cs.campo, cs.valor, e2Valido, ids)
			}
			if ids := e2IDs(Metas(root)); len(ids) != 1 || ids[0] != e2Valido {
				t.Errorf("%s: con \"%s\": %s al lado, Metas (best-effort) trae el legible y solo ese: %v", ca, cs.campo, cs.valor, ids)
			}
		})
	}
}

// CA-442 (hallazgo be6933, guardas): un meta de hoom con un campo que esta
// version no conoce (lo agrega una version mas nueva), o con un campo
// opcional en null, se lee: MetasErr lo trae junto al legible y sin error.
func TestHallazgo_be6933_CA442_UnMetaConUnCampoNuevoONuloSeLee(t *testing.T) {
	const ca = "CA-442 (hallazgo be6933, guardas: campo nuevo o nulo)"
	for _, cs := range []struct{ campo, valor string }{
		{"campo_de_una_version_nueva", `{"lo": "que sea"}`},
		{"usage", `null`},
	} {
		t.Run(cs.campo+"="+cs.valor, func(t *testing.T) {
			root := t.TempDir()
			e2Sidecar(t, root, e2Valido+".meta.json", e2MetaJSON(t, e2Valido, "done"))
			e2Sidecar(t, root, e2Roto+".meta.json", e2bConCampo(t, cs.campo, cs.valor))
			ms, err := MetasErr(root)
			if ids := e2IDs(ms); err != nil || len(ids) != 2 {
				t.Errorf("%s: con \"%s\": %s, el meta se lee: MetasErr trae los dos sin error: %v %v", ca, cs.campo, cs.valor, ids, err)
			}
		})
	}
}
