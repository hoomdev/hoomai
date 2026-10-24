// Tests de regresion de la tarea endurecimiento-2 en runcmd, sobre
// .hoom/specs/evidencia-en-disco.md (CA-442) y el doctor del tablero
// (hallazgo d0d331). Escritos desde el texto del hallazgo y el contrato de la
// correccion, SIN LEER LA IMPLEMENTACION. Los helpers de este archivo llevan
// el prefijo e2.
//
//   - 20261024T141937_64323b (high): MetasErr omitia sin error un sidecar con
//     nombre de hoom (<id>.meta.json) que no era un meta (JSON roto o
//     truncado, sin id), y el doctor certificaba "sin problemas" sin haber
//     leido la telemetria de ese run. Ahora es un error, y los sidecars
//     legibles se siguen devolviendo. Un meta entero de otro id, o con un
//     estado que hoom no escribe, no es de hoom y se saltea sin error; un
//     nombre que no es el de un sidecar de hoom se ignora.
package runcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	e2Valido = "20261024T110000_a1b2c3" // un run de hoom, legible
	e2Roto   = "20261024T120001_feed01" // el nombre del sidecar que no es un meta
	// e2NoID no es un id de run de hoom (tiene un espacio). Ojo: la regla de
	// ids de hoom acepta nombres cortos como "notas" o "x"; por eso un
	// "notas.meta.json" SI tiene nombre de sidecar de hoom.
	e2NoID = "notas sueltas"
)

// e2Sidecar escribe .hoom/runs/<nombre> con contenido tal cual.
func e2Sidecar(t *testing.T, root, nombre string, contenido []byte) {
	t.Helper()
	p := filepath.Join(root, ".hoom", "runs", nombre)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, contenido, 0o644); err != nil {
		t.Fatal(err)
	}
}

// e2MetaJSON es el JSON de un meta de hoom con ese id y estado.
func e2MetaJSON(t *testing.T, id, estado string) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(Meta{ID: id, Provider: "claude", Role: "writer", Task: "endurecimiento-2", Dir: "/x",
		CreatedAt: time.Date(2026, 10, 24, 11, 0, 0, 0, time.UTC), Status: estado}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// e2IDs son los ids de los metas, en orden.
func e2IDs(ms []Meta) []string {
	out := []string{}
	for _, m := range ms {
		out = append(out, m.ID)
	}
	sort.Strings(out)
	return out
}

// CA-442 (hallazgo 64323b): con un sidecar legible y, al lado,
// .hoom/runs/<id>.meta.json con nombre de hoom que NO es un meta (no es JSON,
// esta truncado, esta vacio, es un JSON que no es un objeto, no tiene id o
// tiene el id vacio), MetasErr devuelve error (no pudo leer toda la
// telemetria: el doctor no puede certificar que no hay problemas) y, aun asi,
// devuelve el meta legible. Metas (best-effort) sigue sin error y trae el
// legible.
func TestHallazgo_64323b_CA442_UnSidecarDeHoomQueNoEsUnMetaEsUnErrorDeMetasErr(t *testing.T) {
	const ca = "CA-442 (hallazgo 64323b, sidecar de hoom que no es un meta)"
	entero := e2MetaJSON(t, e2Roto, "done")
	var sinID map[string]any
	if err := json.Unmarshal(entero, &sinID); err != nil {
		t.Fatal(err)
	}
	delete(sinID, "id")
	rawSinID, err := json.Marshal(sinID)
	if err != nil {
		t.Fatal(err)
	}
	for _, cs := range []struct {
		nombre    string
		contenido []byte
	}{
		{"no es JSON", []byte("{esto tampoco es json")},
		{"JSON truncado", entero[:len(entero)/2]},
		{"vacio", []byte{}},
		{"JSON que no es un objeto", []byte(`["no", "es", "un", "meta"]`)},
		{"sin id", rawSinID},
		{"id vacio", e2MetaJSON(t, "", "done")},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			root := t.TempDir()
			e2Sidecar(t, root, e2Valido+".meta.json", e2MetaJSON(t, e2Valido, "done"))
			if ms, err := MetasErr(root); err != nil || len(ms) != 1 {
				t.Fatalf("%s: guarda: con un solo sidecar legible, MetasErr lo trae sin error: %v %v", ca, e2IDs(ms), err)
			}
			e2Sidecar(t, root, e2Roto+".meta.json", cs.contenido)

			ms, err := MetasErr(root)
			if err == nil {
				t.Errorf("%s: .hoom/runs/%s.meta.json (%s) tiene nombre de sidecar de hoom y no es un meta: MetasErr devuelve error, no %v sin error",
					ca, e2Roto, cs.nombre, e2IDs(ms))
			}
			if ids := e2IDs(ms); len(ids) != 1 || ids[0] != e2Valido {
				t.Errorf("%s: con %s al lado, MetasErr devuelve igual el meta legible (%s) y solo ese: %v", ca, cs.nombre, e2Valido, ids)
			}
			if ids := e2IDs(Metas(root)); len(ids) != 1 || ids[0] != e2Valido {
				t.Errorf("%s: con %s al lado, Metas (best-effort) trae el legible y solo ese: %v", ca, cs.nombre, ids)
			}
		})
	}
}

// CA-442 (hallazgo 64323b, guardas del contrato de siempre): no son un error
// de MetasErr, y no aparecen: un meta ENTERO cuyo id no es el de su nombre
// (con forma de id o sin ella), un meta entero con un estado que hoom no
// escribe, un archivo cuyo nombre no es el de un sidecar de hoom (aunque no
// sea JSON; un id de run de hoom puede ser tan corto como "notas", asi que
// el caso usa uno con un espacio) y la narracion <id>.jsonl de un run, sea lo que sea su contenido.
// El meta legible sigue saliendo.
func TestHallazgo_64323b_CA442_LoQueNoEsDeHoomSeSalteaSinError(t *testing.T) {
	const ca = "CA-442 (hallazgo 64323b, lo que no es de hoom)"
	// guarda: e2NoID no es un id de run de hoom: un meta entero con ese id y
	// ese nombre no sale de MetasErr
	guarda := t.TempDir()
	e2Sidecar(t, guarda, e2NoID+".meta.json", e2MetaJSON(t, e2NoID, "done"))
	if ms, err := MetasErr(guarda); err != nil || len(ms) != 0 {
		t.Fatalf("%s: fixture: %q no es un id de run de hoom: un meta entero con ese id no sale de MetasErr: %v %v", ca, e2NoID, e2IDs(ms), err)
	}
	for _, cs := range []struct {
		nombre, archivo string
		contenido       []byte
	}{
		{"meta entero de otro id", e2Roto + ".meta.json", e2MetaJSON(t, "20261024T120002_otro01", "done")},
		{"meta entero con id sin forma de id", e2Roto + ".meta.json", e2MetaJSON(t, "../../VICTIMA", "done")},
		{"meta entero con un estado que hoom no escribe", e2Roto + ".meta.json", e2MetaJSON(t, e2Roto, "inventado")},
		{"nombre que no es id de un run (con un espacio), no JSON", e2NoID + ".meta.json", []byte("{no es json")},
		{"nombre que no es de un sidecar, otra extension", e2Roto + ".meta.json.bak", []byte("{no es json")},
		{"temporal de una escritura", "." + e2Roto + ".meta.json.tmp-123", []byte("{no es")},
		{"narracion del run", e2Roto + ".jsonl", []byte("{no es un meta\nni una linea entera")},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			root := t.TempDir()
			e2Sidecar(t, root, e2Valido+".meta.json", e2MetaJSON(t, e2Valido, "done"))
			e2Sidecar(t, root, cs.archivo, cs.contenido)
			ms, err := MetasErr(root)
			if err != nil {
				t.Errorf("%s: .hoom/runs/%s (%s) no es de hoom: MetasErr no da error por el: %v", ca, cs.archivo, cs.nombre, err)
			}
			if ids := e2IDs(ms); len(ids) != 1 || ids[0] != e2Valido {
				t.Errorf("%s: con %s al lado, MetasErr trae el meta legible y solo ese: %v", ca, cs.nombre, ids)
			}
			for _, m := range ms {
				if strings.Contains(m.ID, "VICTIMA") || m.Status == "inventado" {
					t.Errorf("%s: MetasErr no trae lo que no es de hoom: %+v", ca, m)
				}
			}
		})
	}
}
