// El catalogo de modelos (spec tablero-con-paneles, "El catalogo de
// modelos"; CA-572 y CA-569): el catalogo de internal/modelos es exactamente
// el de las tablas del spec (los modelos de claude y de codex, sus
// esfuerzos, sus lineas, su orden y la version de la CLI contra la que se
// verifico), las lineas sin acentos, y Modelos junta el catalogo con los
// models.<provider> de hoom.yaml. Las tablas se leen del spec (CA-572,
// hallazgo b3e4c7: antes eran una copia a mano).
package modelos_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/modelos"
)

// ttSpec es el spec, relativo a la raiz del modulo (dos arriba de este
// paquete).
var ttSpec = filepath.Join("..", "..", ".hoom", "specs", "tablero-con-paneles.md")

// ttDelSpec es lo que dice la seccion "El catálogo de modelos" del spec: un
// Catalogo por cada "**<provider>** (<version>)." con los modelos de la
// tabla "| Modelo | Línea |" que le sigue, los esfuerzos de cada provider
// en el orden del parrafo "**Esfuerzos.**" (con "los mismos" = los del
// provider anterior) con las lineas de la tabla "| Esfuerzo | Línea |", y la
// fecha de "verificado el <fecha>" (CA-572, hallazgo b3e4c7).
func ttDelSpec(t *testing.T) ([]modelos.Catalogo, string) {
	t.Helper()
	raw, err := os.ReadFile(ttSpec)
	if err != nil {
		t.Fatalf("CA-572: no se pudo leer el spec %s: %v", ttSpec, err)
	}
	texto := string(raw)
	i := strings.Index(texto, "### El catálogo de modelos")
	if i < 0 {
		t.Fatalf("CA-572 (guarda): el spec tiene la seccion 'El catálogo de modelos'")
	}
	sec := texto[i+len("### El catálogo de modelos"):]
	if j := strings.Index(sec, "\n### "); j >= 0 {
		sec = sec[:j]
	}
	fecha := regexp.MustCompile(`verificado el (\d{4}-\d{2}-\d{2})`).FindStringSubmatch(sec)
	if fecha == nil {
		t.Fatalf("CA-572 (guarda): la seccion dice 'verificado el <fecha>'")
	}
	cabeza := regexp.MustCompile(`^\*\*([a-z]+)\*\* \(([^)]+)\)\.`)
	fila := regexp.MustCompile("^\\| `([^`]+)` \\| (.*) \\|$")
	var cats []modelos.Catalogo
	lineas := map[string]string{}
	var tabla string // "modelo" o "esfuerzo": la tabla en la que estamos
	var esfuerzos strings.Builder
	enEsfuerzos := false
	for _, l := range strings.Split(sec, "\n") {
		l = strings.TrimRight(l, "\r")
		switch {
		case cabeza.MatchString(l):
			m := cabeza.FindStringSubmatch(l)
			cats = append(cats, modelos.Catalogo{Provider: m[1], Version: m[2]})
			tabla = ""
		case strings.HasPrefix(l, "**Esfuerzos.**"):
			enEsfuerzos = true
			esfuerzos.WriteString(strings.TrimPrefix(l, "**Esfuerzos.**"))
		case enEsfuerzos && l != "" && !strings.HasPrefix(l, "|"):
			esfuerzos.WriteString(" " + l)
		case l == "| Modelo | Línea |":
			tabla, enEsfuerzos = "modelo", false
		case l == "| Esfuerzo | Línea |":
			tabla, enEsfuerzos = "esfuerzo", false
		case fila.MatchString(l):
			m := fila.FindStringSubmatch(l)
			switch {
			case tabla == "modelo" && len(cats) > 0:
				c := &cats[len(cats)-1]
				c.Modelos = append(c.Modelos, modelos.Opcion{Nombre: m[1], Linea: m[2]})
			case tabla == "esfuerzo":
				lineas[m[1]] = m[2]
			}
		case l == "" && tabla != "":
			tabla = ""
		}
	}
	// "claude (`--effort`): `low`, ... y `max`. codex (`...`): los mismos y `ultra`."
	sinParentesis := regexp.MustCompile(`\([^)]*\)`).ReplaceAllString(esfuerzos.String(), "")
	nombre := regexp.MustCompile("`([^`]+)`")
	var anteriores []string
	for _, m := range regexp.MustCompile(`([a-z]+)\s*:([^.]*)\.`).FindAllStringSubmatch(sinParentesis, -1) {
		var nombres []string
		if strings.Contains(m[2], "los mismos") {
			nombres = append(nombres, anteriores...)
		}
		for _, n := range nombre.FindAllStringSubmatch(m[2], -1) {
			nombres = append(nombres, n[1])
		}
		for k := range cats {
			if cats[k].Provider != m[1] {
				continue
			}
			for _, n := range nombres {
				linea, ok := lineas[n]
				if !ok {
					t.Fatalf("CA-572 (guarda): el esfuerzo %q de %s tiene su linea en la tabla del spec", n, m[1])
				}
				cats[k].Esfuerzos = append(cats[k].Esfuerzos, modelos.Opcion{Nombre: n, Linea: linea})
			}
		}
		anteriores = nombres
	}
	// guarda: lo que el spec dice hoy, para que el lector no quede vacio
	if len(cats) != 2 || cats[0].Provider != "claude" || cats[1].Provider != "codex" ||
		len(cats[0].Modelos) != 8 || len(cats[1].Modelos) != 8 || len(cats[0].Esfuerzos) != 5 || len(cats[1].Esfuerzos) != 6 {
		t.Fatalf("CA-572 (guarda): el spec tiene claude y codex con 8 modelos cada uno y 5 y 6 esfuerzos: %#v", cats)
	}
	return cats, fecha[1]
}

// ttCatalogoDelSpec es la entrada del provider p en el spec.
func ttCatalogoDelSpec(t *testing.T, p string) modelos.Catalogo {
	t.Helper()
	cats, _ := ttDelSpec(t)
	for _, c := range cats {
		if c.Provider == p {
			return c
		}
	}
	t.Fatalf("CA-572 (guarda): el spec tiene la entrada de %s", p)
	return modelos.Catalogo{}
}

// CA-572: Catalogos es exactamente el de las tablas del spec, en el orden
// claude, codex, con la version de cada CLI, y Verificado es la fecha del
// spec (2026-11-09). (CA-572, hallazgo b3e4c7: leido del spec.)
func TestCA572_ElCatalogoEsElDelSpec(t *testing.T) {
	quiere, fecha := ttDelSpec(t)
	if got := modelos.Catalogos(); !reflect.DeepEqual(got, quiere) {
		t.Fatalf("CA-572: el catalogo es el de las tablas del spec:\n got %#v\nwant %#v", got, quiere)
	}
	if fecha != "2026-11-09" {
		t.Fatalf("CA-572 (guarda): el spec dice que el catalogo se verifico el 2026-11-09: %q", fecha)
	}
	if v := modelos.Verificado; v != fecha {
		t.Fatalf("CA-572: el catalogo se verifico el %s: %q", fecha, v)
	}
}

// CA-572: las lineas van sin acentos (ASCII), iguales en UTF-8 y en ASCII; y
// Catalogos da una copia.
func TestCA572_SinAcentosYUnaCopia(t *testing.T) {
	for _, c := range modelos.Catalogos() {
		for _, o := range append(append([]modelos.Opcion{}, c.Modelos...), c.Esfuerzos...) {
			for _, r := range o.Linea + o.Nombre {
				if r > 127 {
					t.Fatalf("CA-572: la linea de %s/%s va sin acentos: %q", c.Provider, o.Nombre, o.Linea)
				}
			}
		}
	}
	a := modelos.Catalogos()
	a[0].Modelos[0].Nombre = "cambiado"
	a[0].Version = "otra"
	a[1].Esfuerzos = nil
	if b := modelos.Catalogos(); b[0].Modelos[0].Nombre != "fable" || b[0].Version != "Claude Code 2.1.319" || len(b[1].Esfuerzos) != 6 {
		t.Fatalf("CA-572: cambiar el resultado de Catalogos no cambia el catalogo: %#v", b[0])
	}
	c, _ := modelos.De("claude")
	c.Modelos[1].Linea = "otra"
	if d, _ := modelos.De("claude"); d.Modelos[1].Linea != "para trabajo complejo" {
		t.Fatalf("CA-572: De tampoco comparte el catalogo")
	}
}

// CA-572, CA-569: De da la entrada de claude y de codex; ninguna para gemini,
// opencode ni un provider desconocido. ConModelo son claude y codex.
// RecibeModelo y RecibeEsfuerzo, true solo para ellos.
func TestCA572_CA569_DeYLosQueRecibenModelo(t *testing.T) {
	for _, p := range []string{"claude", "codex"} {
		c, ok := modelos.De(p)
		if !ok || c.Provider != p {
			t.Fatalf("CA-572: De(%q) da su entrada: %v %#v", p, ok, c)
		}
		if !modelos.RecibeModelo(p) || !modelos.RecibeEsfuerzo(p) {
			t.Fatalf("CA-569: hoom le pasa modelo y esfuerzo a %s", p)
		}
	}
	for _, p := range []string{"gemini", "opencode", "no-existe", ""} {
		if _, ok := modelos.De(p); ok {
			t.Fatalf("CA-572: De(%q) no tiene entrada", p)
		}
		if modelos.RecibeModelo(p) || modelos.RecibeEsfuerzo(p) {
			t.Fatalf("CA-569: hoom no le pasa modelo ni esfuerzo a %q", p)
		}
	}
	if got := modelos.ConModelo(); !reflect.DeepEqual(got, []string{"claude", "codex"}) {
		t.Fatalf("CA-572: ConModelo son claude y codex, ordenados: %q", got)
	}
}

// CA-569: Modelos es el catalogo del provider y despues los propios que no
// son del catalogo, en su orden, con su descripcion como linea; un propio con
// el nombre de un modelo del catalogo no lo repite (su descripcion, si no
// esta vacia, le cambia la linea); dos propios con el mismo nombre cuentan una
// vez (el primero); un provider sin entrada da solo los propios; el
// resultado es nuevo.
func TestCA569_Modelos(t *testing.T) {
	propios := []modelos.Propio{
		{Nombre: "gpt-7-sol", Descripcion: "el nuevo de todos los dias"},
		{Nombre: "gpt-6-astra", Descripcion: "el de la casa"},
		{Nombre: "gpt-6-luna"},
		{Nombre: "gpt-7-sol", Descripcion: "repetido"},
		{Nombre: "sin-desc"},
	}
	got := modelos.Modelos("codex", propios)
	quiere := append([]modelos.Opcion{}, ttCatalogoDelSpec(t, "codex").Modelos...)
	quiere[1].Linea = "el de la casa"
	quiere = append(quiere, modelos.Opcion{Nombre: "gpt-7-sol", Linea: "el nuevo de todos los dias"}, modelos.Opcion{Nombre: "sin-desc"})
	if !reflect.DeepEqual(got, quiere) {
		t.Fatalf("CA-569: Modelos de codex con los propios:\n got %#v\nwant %#v", got, quiere)
	}
	if got := modelos.Modelos("claude", nil); !reflect.DeepEqual(got, ttCatalogoDelSpec(t, "claude").Modelos) {
		t.Fatalf("CA-569: sin propios, el catalogo: %#v", got)
	}
	if got := modelos.Modelos("gemini", []modelos.Propio{{Nombre: "x", Descripcion: "y"}}); !reflect.DeepEqual(got, []modelos.Opcion{{Nombre: "x", Linea: "y"}}) {
		t.Fatalf("CA-569: un provider sin entrada da solo los propios: %#v", got)
	}
	if got := modelos.Modelos("gemini", nil); len(got) != 0 {
		t.Fatalf("CA-569: sin entrada y sin propios, nada: %#v", got)
	}
	a := modelos.Modelos("claude", nil)
	a[0].Nombre = "cambiado"
	if b := modelos.Modelos("claude", nil); b[0].Nombre != "fable" {
		t.Fatalf("CA-569: el resultado de Modelos es nuevo")
	}
}
