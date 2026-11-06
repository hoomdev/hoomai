// Tests adversariales de la ronda 1 de la review de verbos-de-la-tarjeta en
// el manifiesto (CA-490): hallazgo c5a19f. Un hoom.yaml con el mismo rol dos
// veces bajo agents: es un hoom.yaml que no se entiende. Antes el segundo
// bloque reemplazaba en silencio al primero (y su write: desaparecia); ahora
// manifest.Load y manifest.ParseAgents (que hoom review usa sobre el
// hoom.yaml de la base) lo rechazan nombrando el rol y la linea de la
// segunda clave.
package manifest

import (
	"strconv"
	"strings"
	"testing"
)

// r1Repetido es el texto del contrato para un rol repetido en la linea n.
func r1Repetido(rol string, n int) string {
	return `agents: el rol "` + rol + `" esta repetido (linea ` + strconv.Itoa(n) + `)`
}

// r1Base son las lineas que foManifiesto pone antes de lo extra: lo extra
// empieza en la linea r1Base+1.
const r1Base = 6

// hallazgo c5a19f, CA-490: manifest.Load rechaza el mismo rol dos veces bajo
// agents:, con la linea de la segunda clave; el primer bloque (con su
// write:) no se pierde en silencio.
func TestHallazgo_c5a19f_CA490_LoadRechazaUnRolRepetido(t *testing.T) {
	casos := []struct {
		nombre, extra, rol string
		linea              int // de la segunda clave, contando desde lo extra (1 = primera linea de lo extra)
	}{
		{"writer repetido, el segundo sin write",
			"agents:\n  writer:\n    write: { allow: [x] }\n  writer:\n    model: m\n", "writer", 4},
		{"test-writer repetido con otro rol en el medio",
			"agents:\n  test-writer:\n    write: { allow: [\"tests/**\"] }\n  writer:\n    effort: high\n  test-writer:\n    timeout: 45m\n", "test-writer", 6},
		{"el mismo bloque dos veces",
			"agents:\n  arquitecto:\n    model: opus\n  arquitecto:\n    model: opus\n", "arquitecto", 4},
		{"la segunda clave entre comillas es la misma clave",
			"agents:\n  writer:\n    write: { allow: [x] }\n  \"writer\":\n    model: m\n", "writer", 4},
	}
	for _, c := range casos {
		_, err := vdtCargar(t, c.extra)
		if err == nil {
			t.Fatalf("hallazgo c5a19f, CA-490 (%s): un rol repetido bajo agents: es un error de carga, y cargo:\n%s", c.nombre, c.extra)
		}
		want := r1Repetido(c.rol, r1Base+c.linea)
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("hallazgo c5a19f, CA-490 (%s): el error dice\n  %q\nfue\n  %q", c.nombre, want, err.Error())
		}
	}
}

// hallazgo c5a19f: ParseAgents (el hoom.yaml de la base de hoom review)
// tambien lo rechaza, con la linea dentro de lo que recibe.
func TestHallazgo_c5a19f_ParseAgentsRechazaUnRolRepetido(t *testing.T) {
	casos := []struct {
		nombre, raw, rol string
		linea            int
	}{
		{"solo la seccion agents",
			"agents:\n  writer:\n    write: { allow: [x] }\n  writer:\n    model: m\n", "writer", 4},
		{"un hoom.yaml entero",
			"schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n" +
				"agents:\n  writer:\n    write: { allow: [\"internal/**\"] }\n  reviewer:\n    write: { allow: [] }\n  writer:\n    timeout: 45m\n" +
				"review:\n  timeout: 60m\n", "writer", 12},
	}
	for _, c := range casos {
		got, err := ParseAgents([]byte(c.raw))
		if err == nil {
			t.Fatalf("hallazgo c5a19f (%s): ParseAgents rechaza un rol repetido, y devolvio %+v", c.nombre, got)
		}
		want := r1Repetido(c.rol, c.linea)
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("hallazgo c5a19f (%s): el error de ParseAgents dice\n  %q\nfue\n  %q", c.nombre, want, err.Error())
		}
	}
}

// hallazgo c5a19f, CA-490: sin repetidos, agents: carga igual que antes (los
// mismos roles en otro orden, el mismo valor en dos roles distintos).
func TestHallazgo_c5a19f_CA490_SinRepetidosCargaIgual(t *testing.T) {
	m, err := vdtCargar(t, "agents:\n  writer:\n    write: { allow: [x] }\n    model: m\n  test-writer:\n    model: m\n    write: { allow: [x] }\n")
	if err != nil {
		t.Fatalf("hallazgo c5a19f, CA-490: dos roles distintos con los mismos valores no son un rol repetido: %v", err)
	}
	if w := m.Agents["writer"]; w.Model != "m" || len(w.Write.Allow) != 1 || w.Write.Allow[0] != "x" {
		t.Fatalf("hallazgo c5a19f, CA-490: agents.writer se lee entero: %+v", w)
	}
	got, err := ParseAgents([]byte("agents:\n  writer:\n    write: { allow: [x] }\n  test-writer:\n    write: { allow: [y] }\n"))
	if err != nil || len(got) != 2 || len(got["writer"].Write.Allow) != 1 || got["writer"].Write.Allow[0] != "x" {
		t.Fatalf("hallazgo c5a19f: ParseAgents sin repetidos devuelve los dos roles: %+v %v", got, err)
	}
}
