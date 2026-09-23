// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-352): la regla de oro. No existe un endpoint para mover una tarjeta ni
// para escribir su columna: ninguna ruta lo nombra, ningun struct del
// paquete recibe una columna, y un POST con token a CADA ruta del Studio con
// una columna en el cuerpo deja la tarjeta donde la pone la evidencia.
package servecmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// atRutasDeHoy son los patrones que Handler registraba antes de C3.
var atRutasDeHoy = []string{
	"GET /",
	"GET /api/status",
	"GET /api/verdicts",
	"GET /api/verdicts/{id}",
	"GET /api/tasks",
	"GET /api/report",
	"GET /api/files",
	"GET /api/findings",
	"GET /api/context",
	"GET /api/specs",
	"GET /api/specs/{name}",
	"GET /api/board",
	"GET /api/board/{slug}",
	"GET /api/providers",
	"GET /api/runs",
	"GET /api/envelopes",
	"GET /api/runs/{id}",
	"GET /api/runs/{id}/stage",
	"POST /api/runs",
	"POST /api/runs/{id}/input",
	"POST /api/runs/{id}/cancel",
	"POST /api/verify",
	"POST /api/tasks",
	"POST /api/tasks/{slug}/done",
	"POST /api/intake",
	"POST /api/specs/{name}/approve",
	"POST /api/specs/{name}/review",
}

// atRutasNuevas son las de C3.
var atRutasNuevas = []string{
	"POST /api/board/{slug}/launch",
	"POST /api/board/{slug}/save",
	"POST /api/board/{slug}/discard",
	"POST /api/board/{slug}/session",
	"GET /api/board/{slug}/terminal",
}

// Las claves que ninguna ruta acepta.
var atClavesDeColumna = map[string]bool{"column": true, "columna": true, "to": true, "from": true, "destino": true, "destination": true}

const atCuerpoColumna = `{"column": "hecho", "columna": "hecho"}`

// atRuta separa "METODO /ruta" y sustituye los comodines: {slug} y {name}
// por la tarjeta de fixture, {id} por algo que no existe.
func atRuta(patron, slug string) (string, string) {
	metodo, ruta, _ := strings.Cut(patron, " ")
	ruta = strings.NewReplacer("{slug}", slug, "{name}", slug, "{id}", "no-existe-xyz", "{$}", "").Replace(ruta)
	return metodo, ruta
}

// CA-352: Routes() lista cada ruta que registra Handler (las de hoy y las
// cinco nuevas, sin repetir), cada una esta de verdad registrada (el mux no
// la contesta con su 404 ni con 405), ningun patron del codigo del paquete
// queda fuera de la lista, y ninguna contiene column, columna, move ni mover.
func TestCA352_RutasSinColumna(t *testing.T) {
	rutas := Routes()
	if len(rutas) == 0 {
		t.Fatal("CA-352: servecmd.Routes() lista las rutas que registra Handler: esta vacia")
	}
	vistas := map[string]bool{}
	for _, r := range rutas {
		if vistas[r] {
			t.Fatalf("CA-352: Routes() repite %q", r)
		}
		vistas[r] = true
		if m := regexp.MustCompile(`(?i)column|columna|move|mover`).FindString(r); m != "" {
			t.Fatalf("CA-352: ningun patron de ruta nombra una columna ni un movimiento: %q contiene %q", r, m)
		}
	}
	for _, quiero := range append(append([]string{}, atRutasDeHoy...), atRutasNuevas...) {
		if !vistas[quiero] {
			t.Fatalf("CA-352: Routes() lista %q: %q", quiero, rutas)
		}
	}

	// ningun patron escrito en el codigo del paquete se registra por fuera
	// del ayudante que alimenta Routes()
	fset := token.NewFileSet()
	patron := regexp.MustCompile(`^(GET|HEAD|POST|PUT|PATCH|DELETE|OPTIONS) /`)
	for _, f := range atFuentesDelPaquete(t) {
		archivo, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(archivo, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err == nil && patron.MatchString(s) && !vistas[s] {
				t.Errorf("CA-352: %s registra %q y Routes() no lo lista", fset.Position(lit.Pos()), s)
			}
			return true
		})
	}

	// cada ruta listada esta registrada de verdad
	atPATH(t, nil) // ningun CLI de IA ni tmux de la maquina
	dir := tbProyecto(t)
	s := newServer(t, dir)
	for _, r := range rutas {
		metodo, ruta := atRuta(r, "no-existe")
		if metodo != http.MethodGet {
			continue // los POST se prueban en TestCA352_UnPostConColumnaNoMueveLaTarjeta
		}
		rec := atPedir(t, "CA-352", s, metodo, ruta, "", "")
		if rec.Code == http.StatusMethodNotAllowed || rec.Body.String() == "404 page not found\n" {
			t.Fatalf("CA-352: %q esta en Routes() pero Handler no la registra: %d %s", r, rec.Code, rec.Body.String())
		}
	}
}

func atFuentesDelPaquete(t *testing.T) []string {
	t.Helper()
	todos, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var fuentes []string
	for _, f := range todos {
		if !strings.HasSuffix(f, "_test.go") {
			fuentes = append(fuentes, f)
		}
	}
	if len(fuentes) == 0 {
		t.Fatal("CA-352: no encontre el codigo del paquete servecmd")
	}
	return fuentes
}

// CA-352: ningun struct del paquete servecmd (su codigo, leido con
// go/parser) tiene una etiqueta json column, columna, to, from, destino ni
// destination. encoding/json compara las claves sin distinguir mayusculas, y
// un campo sin etiqueta recibe la clave de su nombre, asi que tambien cuentan
// "Column" y un campo exportado To sin etiqueta.
func TestCA352_NingunStructRecibeUnaColumna(t *testing.T) {
	fset := token.NewFileSet()
	structs := 0
	for _, f := range atFuentesDelPaquete(t) {
		archivo, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(archivo, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			structs++
			for _, campo := range st.Fields.List {
				nombreJSON := ""
				if campo.Tag != nil {
					tag, err := strconv.Unquote(campo.Tag.Value)
					if err != nil {
						t.Fatalf("CA-352: etiqueta ilegible en %s: %v", fset.Position(campo.Pos()), err)
					}
					nombreJSON, _, _ = strings.Cut(reflect.StructTag(tag).Get("json"), ",")
				}
				if nombreJSON == "-" {
					continue
				}
				if nombreJSON != "" {
					if atClavesDeColumna[strings.ToLower(nombreJSON)] {
						t.Errorf("CA-352: %s: un struct del Studio recibe la clave json %q", fset.Position(campo.Pos()), nombreJSON)
					}
					continue
				}
				for _, id := range campo.Names {
					if id.IsExported() && atClavesDeColumna[strings.ToLower(id.Name)] {
						t.Errorf("CA-352: %s: el campo %s sin etiqueta json recibe la clave %q", fset.Position(id.Pos()), id.Name, strings.ToLower(id.Name))
					}
				}
			}
			return true
		})
	}
	if structs == 0 {
		t.Fatal("CA-352: no encontre ningun struct en el paquete: el test no esta leyendo el codigo")
	}
}

// CA-352: un POST con token a CADA ruta POST de Routes() (y a las nuevas,
// aunque Routes() no las liste), con {"column": "hecho", "columna": "hecho"},
// deja la columna de una tarjeta de Tu aprobacion igual: ni aprobada, ni
// con tarea, ni con item cambiado. Los endpoints nuevos (launch, save,
// discard, session) y approve con cuerpo responden 400 a esa clave.
func TestCA352_UnPostConColumnaNoMueveLaTarjeta(t *testing.T) {
	atPATH(t, nil) // ningun CLI de IA ni tmux de la maquina
	dir := tbProyecto(t)
	const slug = "por-firmar"
	tbItem(t, dir, slug, "")
	tbEscribir(t, dir, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-30: algo.", true))
	if c := atTarjeta(t, dir, slug); c.Column != boardcmd.ColTuAprobacion {
		t.Fatalf("CA-352: fixture: la tarjeta esta en tu-aprobacion (CA-271), esta en %s", c.Column)
	}
	itemAntes, err := os.ReadFile(filepath.Join(dir, ".hoom", "items", slug+".yaml"))
	if err != nil {
		t.Fatal(err)
	}

	s := newServer(t, dir)
	t.Cleanup(s.waitLaunches)

	posts := []string{}
	vistos := map[string]bool{}
	for _, r := range append(Routes(), atRutasNuevas...) {
		if strings.HasPrefix(r, "POST ") && !vistos[r] {
			vistos[r] = true
			posts = append(posts, r)
		}
	}
	if !vistos["POST /api/specs/{name}/approve"] {
		posts = append(posts, "POST /api/specs/{name}/approve")
	}
	nuevas := map[string]bool{
		"POST /api/board/{slug}/launch":  true,
		"POST /api/board/{slug}/save":    true,
		"POST /api/board/{slug}/discard": true,
		"POST /api/board/{slug}/session": true,
		"POST /api/specs/{name}/approve": true,
	}

	for _, r := range posts {
		_, ruta := atRuta(r, slug)
		rec := atPedir(t, "CA-352", s, http.MethodPost, ruta, s.Token(), atCuerpoColumna)
		if rec.Body.String() == "404 page not found\n" || rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("CA-352: POST %s no esta registrada: %d", ruta, rec.Code)
		}
		if nuevas[r] && rec.Code != http.StatusBadRequest {
			t.Errorf("CA-352: POST %s con una columna en el cuerpo es 400 (clave desconocida), fue %d: %s", ruta, rec.Code, rec.Body.String())
		}
		if c := atTarjeta(t, dir, slug); c.Column != boardcmd.ColTuAprobacion {
			t.Fatalf("CA-352: POST %s con %s movio la tarjeta a %s: la columna solo la da la evidencia", ruta, atCuerpoColumna, c.Column)
		}
	}

	// y una accion valida con una columna de yapa tampoco pasa
	rec := atPedir(t, "CA-352", s, http.MethodPost, "/api/board/"+slug+"/launch", s.Token(),
		`{"action": "pedir-arquitecto", "provider": "claude", "pedido": "corregi el spec", "column": "review"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("CA-352: launch con una clave column es 400, fue %d: %s", rec.Code, rec.Body.String())
	}
	s.waitLaunches()

	c := atTarjeta(t, dir, slug)
	if c.Column != boardcmd.ColTuAprobacion {
		t.Fatalf("CA-352: la tarjeta sigue en tu-aprobacion, esta en %s", c.Column)
	}
	if st, _, err := approval.Status(dir, filepath.Join(dir, ".hoom", "specs", slug+".md")); err != nil || st == approval.StatusApproved {
		t.Fatalf("CA-352: ningun POST con una columna aprobo el spec: %s %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".hoom", "worktrees", slug)); !os.IsNotExist(err) {
		t.Fatalf("CA-352: ningun POST con una columna creo la tarea: %v", err)
	}
	if despues, err := os.ReadFile(filepath.Join(dir, ".hoom", "items", slug+".yaml")); err != nil || string(despues) != string(itemAntes) {
		t.Fatalf("CA-352: el item no cambia:\nantes:\n%s\ndespues:\n%s", itemAntes, despues)
	}
}
