// Tests de los hallazgos 752455 y 1499b7 de historia-en-el-tablero (ronda 2
// del test-writer) sobre el verbo `hoom item backfill [--dry-run] [--json]`:
// un directorio en el lugar del item se lista como que no califica, con su
// motivo, igual en --dry-run y en la corrida real, y no se crea nada; un
// primer encabezado de spec con un caracter de control o de direccion no
// crea un item con ese titulo ni lo imprime crudo. Escritos desde el spec
// .hoom/specs/historia-en-el-tablero.md y los textos de los hallazgos, sin
// leer la implementacion. Repos git reales en t.TempDir(). Sin ningun CLI de
// IA. Todo identificador de este archivo lleva el prefijo h2.
package itemcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
)

// h2TituloAcentos es un titulo normal que tiene que seguir calificando:
// acentos (precompuestos y con un acento combinante), enie, CJK, hebreo (sin
// caracteres de direccion), emojis, una secuencia con ZWJ (U+200D), un
// selector de variacion (U+FE0F) y una bandera.
const h2TituloAcentos = "Café, ñandú y acción de Jose\u0301: 日本語 שלום 🚀 👩\u200d💻 ✔\ufe0f 🇦🇷"

// h2EsHostil: controles C0 (salvo \t y \n), DEL, controles C1, y los de
// embebido, anulacion y aislamiento de direccion (U+202A..U+202E,
// U+2066..U+2069).
func h2EsHostil(r rune) bool {
	switch {
	case r < 0x20:
		return r != '\t' && r != '\n'
	case r >= 0x7f && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e:
		return true
	case r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

func h2PrimeraHostil(s string) (rune, bool) {
	for _, r := range s {
		if h2EsHostil(r) {
			return r, true
		}
	}
	return 0, false
}

func h2DiceTitulo(motivo string) bool {
	m := strings.ToLower(motivo)
	return strings.Contains(m, "encabezado") || strings.Contains(m, "titulo") || strings.Contains(m, "título")
}

// h2Cand busca slug en cs.
func h2Cand(cs []boardcmd.Candidata, slug string) (boardcmd.Candidata, bool) {
	for _, c := range cs {
		if c.Slug == slug {
			return c, true
		}
	}
	return boardcmd.Candidata{}, false
}

// h2Tiene dice si slug esta en ss.
func h2Tiene(ss []string, slug string) bool {
	for _, s := range ss {
		if s == slug {
			return true
		}
	}
	return false
}

// h2Listadas son las tareas que backfill lista como creadas (o que crearia)
// o que no califican: las sin item.
func h2Listadas(res BackfillResult) []string {
	out := append(htCandSlugs(res.Creados), htCandSlugs(res.NoCalifican)...)
	sort.Strings(out)
	return out
}

// h2Fmt es el resultado entero, para los mensajes.
func h2Fmt(res BackfillResult) string {
	raw, _ := json.Marshal(res)
	return fmt.Sprintf("%+q", string(raw))
}

// ---------------------------------------------------------------------------
// 752455: un directorio en el lugar del item.
// ---------------------------------------------------------------------------

// h2ProyectoConDirectorio arma un proyecto con cuatro tareas con spec y su
// verde aprobado e integrado: "precios" tiene un DIRECTORIO (con un archivo,
// commiteado: como llegaria por git) en .hoom/items/precios.yaml, "vacia" un
// directorio vacio en .hoom/items/vacia.yaml, "control" nada y "con-item" un
// item de verdad.
func h2ProyectoConDirectorio(t *testing.T) string {
	t.Helper()
	root := htRepo(t, h1BlockOn)
	slugs := []string{"precios", "vacia", "control", "con-item"}
	for _, s := range slugs {
		htSpec(t, root, s, "# Spec: "+s)
		htAprobar(t, root, s)
	}
	htCommitear(t, root, "specs")
	for _, s := range slugs {
		htVeredicto(t, root, s, htT0, htGatesVerdes(), true)
	}
	icEscribir(t, root, ".hoom/items/precios.yaml/nota.txt", "no soy un item\n")
	icEscribir(t, root, ".hoom/items/con-item.yaml", icItemYAML("Con item", "2026-09-02T10:00:00Z", ""))
	htCommitear(t, root, "veredictos, un item y un directorio en el lugar de otro")
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "items", "vacia.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// h2NoCalificaPorDirectorio exige que, en res, slug no califique con un
// motivo que dice que en el lugar del item hay algo que no es un item, y que
// no salga ni como creada (o que crearia) ni como "ya tiene item".
func h2NoCalificaPorDirectorio(t *testing.T, ca string, res BackfillResult, slug string) boardcmd.Candidata {
	t.Helper()
	if h2Tiene(res.ConItem, slug) {
		t.Fatalf("%s: con un directorio en .hoom/items/%s.yaml la tarea no \"ya tiene item\": con_item %v\n%s",
			ca, slug, res.ConItem, h2Fmt(res))
	}
	if _, ok := h2Cand(res.Creados, slug); ok {
		t.Fatalf("%s: con un directorio en .hoom/items/%s.yaml backfill no crea (ni crearia) su item\n%s", ca, slug, h2Fmt(res))
	}
	c, ok := h2Cand(res.NoCalifican, slug)
	if !ok {
		t.Fatalf("%s: %s sale entre las que no califican, con su motivo\n%s", ca, slug, h2Fmt(res))
	}
	if c.Historico != "" || c.ConItem || !strings.Contains(strings.ToLower(c.Motivo), "item") ||
		strings.Contains(c.Motivo, htMotivoVeredicto) {
		t.Fatalf("%s: el motivo de %s dice que en el lugar del item hay algo que no es un item (su verde si esta): %+v", ca, slug, c)
	}
	return c
}

// CA-446, CA-453, hallazgo 752455: con un DIRECTORIO en .hoom/items/<slug>.yaml
// (con un archivo adentro, o vacio) para una tarea que por lo demas califica,
// `hoom item backfill --dry-run` y `hoom item backfill` la listan como que NO
// califica, con un motivo que dice que en el lugar del item hay algo que no
// es un item; no la listan como "ya tiene item" ("intacto"), no crean nada
// ahi, y las dos salidas (dry-run y real; resultado, JSON y texto) dicen lo
// mismo de ella. Exit 0. Los controles: "control" se crea, "con-item" sale
// intacto. Lo que backfill lista es el evidencia-sin-item del doctor.
func TestCA446_Hallazgo752455_BackfillDirectorioEnLugarDelItem(t *testing.T) {
	root := h2ProyectoConDirectorio(t)
	dirs := []string{"precios", "vacia"}
	antes := htFoto(t, root)
	head := cbHead(t, root)
	nada := func(t *testing.T, ca, que string) {
		t.Helper()
		htSinCambios(t, ca, que, antes, htFoto(t, root))
		if h := cbHead(t, root); h != head {
			t.Fatalf("%s: %s no commitea: HEAD %s -> %s", ca, que, head, h)
		}
	}

	// --dry-run: el resultado y el texto
	motivos := map[string]boardcmd.Candidata{}
	lineasDry := map[string]string{}
	t.Run("CA-446 --dry-run", func(t *testing.T) {
		dry, err := Backfill(root, "main", "high", true, time.Now().UTC())
		if err != nil {
			t.Fatalf("CA-446 (752455): backfill --dry-run sale bien aunque una tarea no califique: %v", err)
		}
		nada(t, "CA-446", "backfill --dry-run")
		if doc := htDoctorSinItem(t, root); !reflect.DeepEqual(h2Listadas(dry), doc) {
			t.Fatalf("CA-446: --dry-run lista cada tarea de evidencia-sin-item:\ndoctor:   %v\nbackfill: %v", doc, h2Listadas(dry))
		}
		if _, ok := h2Cand(dry.Creados, "control"); !ok || !h2Tiene(dry.ConItem, "con-item") {
			t.Fatalf("CA-446 (control): --dry-run crearia control y deja intacto con-item\n%s", h2Fmt(dry))
		}
		for _, s := range dirs {
			motivos[s] = h2NoCalificaPorDirectorio(t, "CA-446 (752455, --dry-run)", dry, s)
		}
		txtDry, _, err := htExec(t, root, "backfill", "--dry-run")
		if err != nil {
			t.Fatalf("CA-446 (752455): 'hoom item backfill --dry-run': %v", err)
		}
		nada(t, "CA-446", "'hoom item backfill --dry-run'")
		for _, s := range dirs {
			l := htLineaDe(t, "CA-446 (752455)", htLineas(txtDry), s)
			if !strings.Contains(l, "no crea") || !strings.Contains(l, motivos[s].Motivo) || strings.Contains(l, "intacto") ||
				strings.Contains(l, "crearia") {
				t.Fatalf("CA-446 (752455): la linea de %s en --dry-run dice \"no crea\" y su motivo (%q), nunca \"intacto\" ni \"crearia\": %q",
					s, motivos[s].Motivo, l)
			}
			lineasDry[s] = l
		}
	})

	// la corrida real (--json y texto): lo mismo de ellas que --dry-run, y
	// solo se crea control
	t.Run("CA-453 la corrida real", func(t *testing.T) {
		out, _, err := htExec(t, root, "backfill", "--json")
		if err != nil {
			t.Fatalf("CA-453 (752455): 'hoom item backfill --json' sale bien aunque una tarea no califique: %v", err)
		}
		var real BackfillResult
		if err := json.Unmarshal([]byte(out), &real); err != nil || real.DryRun {
			t.Fatalf("CA-453: --json sin --dry-run es JSON con dry_run false: %v\n%s", err, out)
		}
		creados, modificados, borrados := htDiff(antes, htFoto(t, root))
		if want := []string{filepath.Join(".hoom", "items", "control.yaml")}; !reflect.DeepEqual(creados, want) ||
			len(modificados)+len(borrados) > 0 {
			t.Fatalf("CA-453 (752455): backfill solo crea el item de control; nada dentro ni en lugar de los directorios: "+
				"creo %v, modifico %v, borro %v", creados, modificados, borrados)
		}
		if _, ok := h2Cand(real.Creados, "control"); !ok || !h2Tiene(real.ConItem, "con-item") {
			t.Fatalf("CA-447 (control): backfill crea control y deja intacto con-item\n%s", h2Fmt(real))
		}
		for _, s := range dirs {
			c := h2NoCalificaPorDirectorio(t, "CA-453 (752455, real)", real, s)
			if w, ok := motivos[s]; ok && !reflect.DeepEqual(c, w) {
				t.Fatalf("CA-446 (752455): --dry-run y la corrida real dicen lo mismo de %s:\ndry-run: %+v\nreal:    %+v", s, w, c)
			}
		}

		antes := htFoto(t, root)
		txtReal, _, err := htExec(t, root, "backfill")
		if err != nil {
			t.Fatalf("CA-453 (752455): 'hoom item backfill': %v", err)
		}
		htSinCambios(t, "CA-453", "un segundo 'hoom item backfill'", antes, htFoto(t, root))
		for _, s := range dirs {
			l := htLineaDe(t, "CA-453 (752455)", htLineas(txtReal), s)
			if w, ok := lineasDry[s]; ok && l != w {
				t.Fatalf("CA-446 (752455): la linea de %s es la misma con y sin --dry-run:\ndry-run: %q\nreal:    %q", s, w, l)
			}
			if strings.Contains(l, "intacto") || !strings.Contains(l, "no crea") {
				t.Fatalf("CA-453 (752455): la linea de %s dice \"no crea\", nunca \"intacto\": %q", s, l)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// 1499b7: un primer encabezado con un caracter de control o de direccion.
// ---------------------------------------------------------------------------

// h2Hostiles son los encabezados hostiles de este archivo: los del hallazgo
// (ESC con una secuencia de color, \r, NUL, U+202E) y tres de la misma clase
// (DEL, el CSI de 8 bits U+009B, el aislamiento U+2067). La propiedad sobre
// todas las runas de la clase esta en internal/boardcmd/historia_hallazgos2_test.go.
var h2Hostiles = map[string]string{
	"hostil-esc":  "# Spec: Precios \x1b[31mrojo\x1b[0m",
	"hostil-cr":   "# Spec: Precios por region\rFalso titulo",
	"hostil-nul":  "# Spec: Pre\x00cios",
	"hostil-rlo":  "# Spec: Precios \u202eoloR",
	"hostil-del":  "# Spec: Pre\x7fcios",
	"hostil-csi8": "# Spec: Precios \u009b31mrojo",
	"hostil-rli":  "# Spec: Precios \u2067aislado\u2069",
}

// CA-446, CA-447, hallazgo 1499b7: si el primer encabezado del spec de una
// tarea que califica trae un caracter de control o de direccion, backfill
// (--dry-run y real) no la hace calificar y su motivo habla del encabezado o
// del titulo; no se crea ningun item con ese titulo (ni con el titulo
// degradado al slug), y nada de eso se imprime crudo: ni el texto de
// --dry-run ni el de la corrida real traen un caracter de control o de
// direccion. El resto sigue igual (protecciones contra un arreglo demasiado
// amplio): un titulo con acentos y emojis, un spec con fin de linea CRLF y
// una tarea de correccion sin spec se crean con su titulo exacto.
func TestCA447_Hallazgo1499b7_BackfillEncabezadoConControlNoCrea(t *testing.T) {
	root := htRepo(t, h1BlockOn)
	for s, h := range h2Hostiles {
		htSpec(t, root, s, h)
	}
	htSpec(t, root, "precios", "# Spec: Precios por region")
	htSpec(t, root, "acentos", "# Spec: "+h2TituloAcentos)
	htSpec(t, root, "crlf", "# Spec: Con CRLF")
	crlf := filepath.Join(root, ".hoom", "specs", "crlf.md")
	raw, err := os.ReadFile(crlf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crlf, []byte(strings.ReplaceAll(string(raw), "\n", "\r\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	htCommitear(t, root, "specs")
	for s := range h2Hostiles {
		htVeredicto(t, root, s, htT0, htGatesVerdes(), true)
	}
	esp := map[string]htCaso{}
	for s, titulo := range map[string]string{"precios": "Precios por region", "acentos": h2TituloAcentos, "crlf": "Con CRLF"} {
		v := htVeredicto(t, root, s, htT0, htGatesVerdes(), true)
		esp[s] = htCaso{historico: "veredicto " + v.ID, tipo: "feature", titulo: titulo}
	}
	htCerrar(t, root, htHallazgo(t, root, "correccion", "high"), finding.StatusCorrected)
	esp["correccion"] = htCaso{historico: item.HistoricoHallazgos, tipo: "bug", titulo: "correccion"}
	htCommitear(t, root, "veredictos y hallazgos")
	todas := []string{}
	for s := range h2Hostiles {
		todas = append(todas, s)
	}
	for s := range esp {
		todas = append(todas, s)
	}
	sort.Strings(todas)
	if doc := htDoctorSinItem(t, root); !reflect.DeepEqual(doc, todas) {
		t.Fatalf("precondicion: el doctor da evidencia-sin-item de %v, dio %v", todas, doc)
	}

	// exige que las tareas sin encabezado hostil sigan calificando igual
	protecciones := func(t *testing.T, ca string, res BackfillResult) {
		t.Helper()
		for s, w := range esp {
			c, ok := h2Cand(res.Creados, s)
			if !ok || c.Historico != w.historico || c.Tipo != w.tipo || c.Titulo != w.titulo || c.Motivo != "" {
				t.Fatalf("%s: %s sigue calificando con historico %q, tipo %q y titulo %+q; fue %+q (no califican: %+q)",
					ca, s, w.historico, w.tipo, w.titulo, fmt.Sprintf("%+v", c), fmt.Sprintf("%+v", res.NoCalifican))
			}
		}
	}
	// exige lo de cada tarea en un resultado de backfill
	revisar := func(t *testing.T, ca string, res BackfillResult) {
		t.Helper()
		if got := h2Listadas(res); !reflect.DeepEqual(got, todas) || len(res.ConItem) > 0 {
			t.Fatalf("%s: backfill lista cada tarea de evidencia-sin-item %v; listo %v y con item %v", ca, todas, got, res.ConItem)
		}
		protecciones(t, ca, res)
		var mal []string
		for s, h := range h2Hostiles {
			c, ok := h2Cand(res.NoCalifican, s)
			_, motivoHostil := h2PrimeraHostil(c.Motivo)
			if !ok || c.Historico != "" || c.Motivo == "" || !h2DiceTitulo(c.Motivo) || strings.Contains(c.Motivo, htMotivoVeredicto) ||
				motivoHostil {
				if cr, creado := h2Cand(res.Creados, s); creado {
					c = cr
				}
				mal = append(mal, fmt.Sprintf("  %s (%+q): %+q", s, h, fmt.Sprintf("%+v", c)))
			}
		}
		if len(mal) > 0 {
			sort.Strings(mal)
			t.Fatalf("%s: con un caracter de control o de direccion en el primer encabezado del spec la tarea no califica y su "+
				"motivo habla del encabezado o del titulo, sin el caracter crudo; %d de %d no:\n%s",
				ca, len(mal), len(h2Hostiles), strings.Join(mal, "\n"))
		}
	}
	// exige que un texto de backfill no traiga nada crudo
	crudo := func(t *testing.T, ca, out string) {
		t.Helper()
		if r, ok := h2PrimeraHostil(out); ok {
			t.Fatalf("%s: la salida de backfill no imprime crudo un caracter de control o de direccion (U+%04X):\n%+q", ca, r, out)
		}
		for s := range h2Hostiles {
			if l := htLineaDe(t, ca, htLineas(out), s); !strings.Contains(l, "no crea") {
				t.Fatalf("%s: la linea de %s dice \"no crea\": %+q", ca, s, l)
			}
		}
	}

	antes := htFoto(t, root)
	t.Run("CA-447 protecciones: acentos, emojis, CRLF y sin spec siguen calificando", func(t *testing.T) {
		res, err := Backfill(root, "main", "high", true, time.Now().UTC())
		if err != nil {
			t.Fatalf("CA-447: backfill --dry-run: %v", err)
		}
		htSinCambios(t, "CA-447", "backfill --dry-run", antes, htFoto(t, root))
		protecciones(t, "CA-447 (--dry-run)", res)
	})
	t.Run("CA-446 --dry-run", func(t *testing.T) {
		res, err := Backfill(root, "main", "high", true, time.Now().UTC())
		if err != nil {
			t.Fatalf("CA-446 (1499b7): backfill --dry-run sale bien aunque una tarea no califique: %v", err)
		}
		htSinCambios(t, "CA-446", "backfill --dry-run", antes, htFoto(t, root))
		revisar(t, "CA-446 (1499b7, --dry-run)", res)
		out, _, err := htExec(t, root, "backfill", "--dry-run")
		if err != nil {
			t.Fatalf("CA-446 (1499b7): 'hoom item backfill --dry-run': %v", err)
		}
		htSinCambios(t, "CA-446", "'hoom item backfill --dry-run'", antes, htFoto(t, root))
		crudo(t, "CA-446 (1499b7, --dry-run)", out)
	})

	t.Run("CA-447 la corrida real", func(t *testing.T) {
		out, _, err := htExec(t, root, "backfill", "--json")
		if err != nil {
			t.Fatalf("CA-447 (1499b7): 'hoom item backfill --json' sale bien aunque una tarea no califique: %v", err)
		}
		var res BackfillResult
		if err := json.Unmarshal([]byte(out), &res); err != nil || res.DryRun {
			t.Fatalf("CA-447: --json sin --dry-run es JSON con dry_run false: %v\n%s", err, out)
		}
		revisar(t, "CA-447 (1499b7, real)", res)

		creados, modificados, borrados := htDiff(antes, htFoto(t, root))
		want := []string{}
		// el proyecto no tenia .hoom/items/: aparece la carpeta y los items
		// de las que califican (como en TestCA447_BackfillTareasHostilesNoCreanNada)
		if carpeta := filepath.Join(".hoom", "items") + "/"; antes[carpeta] == "" {
			want = append(want, carpeta)
		}
		for s := range esp {
			want = append(want, filepath.Join(".hoom", "items", s+".yaml"))
		}
		sort.Strings(want)
		if !reflect.DeepEqual(creados, want) || len(modificados)+len(borrados) > 0 {
			t.Fatalf("CA-447 (1499b7): backfill crea solo los items de las tareas sin encabezado hostil %v; creo %v, modifico %v, borro %v",
				want, creados, modificados, borrados)
		}
		for s, w := range esp {
			if got := h1Titulo(t, root, s); got != w.titulo {
				t.Fatalf("CA-447: el item de %s lleva su titulo exacto %+q; fue %+q", s, w.titulo, got)
			}
		}
		es, err := os.ReadDir(filepath.Join(root, ".hoom", "items"))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			b, err := os.ReadFile(filepath.Join(root, ".hoom", "items", e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := yaml.Unmarshal(b, &m); err != nil {
				t.Fatalf("CA-447: el item %s es YAML: %v", e.Name(), err)
			}
			titulo, _ := m["titulo"].(string)
			if r, ok := h2PrimeraHostil(titulo); ok {
				t.Fatalf("CA-447 (1499b7): ningun item se crea con un titulo con U+%04X: %s %+q", r, e.Name(), titulo)
			}
		}

		antes2 := htFoto(t, root)
		txt, _, err := htExec(t, root, "backfill")
		if err != nil {
			t.Fatalf("CA-447 (1499b7): 'hoom item backfill' otra vez: %v", err)
		}
		htSinCambios(t, "CA-447", "un segundo 'hoom item backfill'", antes2, htFoto(t, root))
		crudo(t, "CA-447 (1499b7, real)", txt)
	})
}
