//go:build darwin || linux

// Test de la enmienda 1 de historia-en-el-tablero (hallazgo de decision
// 142d20, que reemplaza a d03734 y a5e1d2 y cubre 1e8b0c, 039b75 y a7d0b1):
// backfill nunca borra un item. Si un alta falla a mitad del lote, termina
// con error, los items que esa corrida ya creo quedan (completos y validos),
// la salida los lista (texto, y --json con "error"), y volver a correr
// backfill completa el resto sin tocarlos. Reemplaza al test de d03734 de la
// ronda 1, cuyo contrato ("no queda nada") ya no vale.
//
// El fallo se provoca con RLIMIT_FSIZE (por eso solo darwin y linux): Go
// ignora SIGXFSZ, asi que una escritura que pasa el tope falla con EFBIG
// ("file too large"), un error de E/S de verdad como el del disco lleno. El
// limite es de TODO el proceso, asi que nunca se baja en el de 'go test':
// el test se re-ejecuta a si mismo en un proceso hijo, que baja el limite,
// corre backfill y reporta por stdout; el padre mira el disco. Escrito desde
// el spec, sin leer la implementacion. Todo identificador de este archivo
// lleva el prefijo h3.
package itemcmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/item"
)

const (
	// h3Tope es el limite de tamano de archivo del hijo: el item de a-uno
	// entra, el de b-dos (su titulo ocupa 16 KiB) no.
	h3Tope = 8 * 1024
	// h3EnvRoot y h3EnvModo marcan el modo hijo: el proyecto y que correr
	// (backfill | texto | json).
	h3EnvRoot = "HOOM_TEST_H3_142D20_ROOT"
	h3EnvModo = "HOOM_TEST_H3_142D20_MODO"
	// h3Marca precede la linea JSON con la que el hijo reporta.
	h3Marca = "H3-142D20-RESULTADO: "
	// h3Test142d20 es el nombre exacto del test (el hijo se corre con el).
	h3Test142d20 = "TestCA447_Hallazgo142d20_FallaElSegundoQuedanLosCreados"
)

// h3Reporte es lo que el hijo dice de su corrida.
type h3Reporte struct {
	Err    string         `json:"err"`    // el error de Backfill/Execute; "" = salio bien
	Res    BackfillResult `json:"res"`    // el resultado de Backfill (modo backfill)
	Stdout string         `json:"stdout"` // la salida de Execute (modos texto y json)
}

// h3ConTopeDeArchivo corre f con el limite blando de RLIMIT_FSIZE en tope
// bytes (el duro no se toca) y lo restaura despues, aunque f entre en panico.
// Solo se llama en el proceso hijo.
func h3ConTopeDeArchivo(t *testing.T, tope uint64, f func()) {
	t.Helper()
	var antes syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
		t.Fatal(err)
	}
	if antes.Cur < tope {
		t.Fatalf("el limite de tamano de archivo del proceso (%d) ya es menor que %d", antes.Cur, tope)
	}
	nuevo := antes
	nuevo.Cur = tope
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &nuevo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
			t.Fatalf("no pude restaurar RLIMIT_FSIZE: %v", err)
		}
	}()
	f()
}

// h3Hijo es el proceso hijo: con el tope puesto corre backfill sobre root
// (por Backfill, o por el comando en texto o --json), restaura el tope y
// reporta por stdout.
func h3Hijo(t *testing.T, root, modo string) {
	var rep h3Reporte
	h3ConTopeDeArchivo(t, h3Tope, func() {
		var err error
		args := []string{"backfill"}
		switch modo {
		case "backfill":
			rep.Res, err = Backfill(root, "main", "high", false, time.Now().UTC())
		case "json":
			args = append(args, "--json")
			fallthrough
		case "texto":
			req, perr := Parse(args)
			if perr != nil {
				err = fmt.Errorf("parse: %w", perr)
				break
			}
			var out, errb bytes.Buffer
			err = Execute(root, "main", "high", req, &out, &errb)
			rep.Stdout = out.String()
		default:
			err = fmt.Errorf("modo hijo desconocido %q", modo)
		}
		if err != nil {
			rep.Err = err.Error()
		}
	})
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s%s\n", h3Marca, raw)
}

// h3CorrerHijo re-ejecuta este test en un proceso hijo sobre root y devuelve
// su reporte. El proceso de 'go test' nunca baja su limite.
func h3CorrerHijo(t *testing.T, root, modo string) h3Reporte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+h3Test142d20+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), h3EnvRoot+"="+root, h3EnvModo+"="+modo)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var rep *h3Reporte
	sc := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if l, ok := strings.CutPrefix(sc.Text(), h3Marca); ok {
			var r h3Reporte
			if jerr := json.Unmarshal([]byte(l), &r); jerr != nil {
				t.Fatalf("el reporte del hijo es JSON: %v\n%s", jerr, l)
			}
			rep = &r
		}
	}
	if rep == nil || err != nil {
		t.Fatalf("el proceso hijo (%s) corre y reporta: %v\nstdout:\n%s\nstderr:\n%s", modo, err, out.String(), errb.String())
	}
	return *rep
}

// h3Lote arma el proyecto: tres tareas califican, en orden a-uno (sin spec,
// item chico), b-dos (con spec cuyo encabezado ocupa 16 KiB: su item no
// entra en h3Tope) y c-tres (sin spec); ya-estaba tiene item. Devuelve root
// y el contenido de ya-estaba.
func h3Lote(t *testing.T) (string, string) {
	t.Helper()
	root := htRepo(t, h1BlockOn)
	largo := strings.Repeat("g", 16*1024)
	htSpec(t, root, "b-dos", "# Spec: "+largo)
	htAprobar(t, root, "b-dos")
	for _, s := range []string{"a-uno", "c-tres", "ya-estaba"} {
		htHallazgo(t, root, s, "low")
	}
	yaEstaba := icItemYAML("Ya estaba", "2026-09-02T10:00:00Z", "")
	icEscribir(t, root, ".hoom/items/ya-estaba.yaml", yaEstaba)
	htCommitear(t, root, "spec, hallazgos e item")
	htVeredicto(t, root, "b-dos", htT0, htGatesVerdes(), true)
	htCommitear(t, root, "verde de b-dos")

	pre, err := Backfill(root, "main", "high", true, time.Now().UTC())
	if err != nil {
		t.Fatalf("precondicion: backfill --dry-run: %v", err)
	}
	if got := htCandSlugs(pre.Creados); !reflect.DeepEqual(got, []string{"a-uno", "b-dos", "c-tres"}) ||
		!reflect.DeepEqual(pre.ConItem, []string{"ya-estaba"}) {
		t.Fatalf("precondicion: crearia a-uno, b-dos y c-tres, y ya-estaba tiene item: %+v", pre)
	}
	for _, c := range pre.Creados {
		if c.Slug == "b-dos" && c.Titulo != largo {
			t.Fatalf("precondicion: el titulo de b-dos es su encabezado de 16 KiB (su item no entra en %d bytes): %d bytes", h3Tope, len(c.Titulo))
		}
	}
	return root, yaEstaba
}

// h3Casos es lo que cada item creado tiene que llevar.
var h3Casos = map[string]htCaso{
	"a-uno":  {historico: item.HistoricoHallazgos, tipo: "bug", titulo: "a-uno"},
	"c-tres": {historico: item.HistoricoHallazgos, tipo: "bug", titulo: "c-tres"},
}

// h3Despues mira el disco despues de una corrida que fallo en b-dos: no se
// borro ni modifico nada; a-uno quedo, completo y valido; b-dos no existe
// (ni a medias, ni un temporal); ya-estaba intacto. Devuelve los slugs de
// los items nuevos.
func h3Despues(t *testing.T, ca, root, yaEstaba string, antes map[string]string, desde, hasta time.Time) []string {
	t.Helper()
	creados, modificados, borrados := htDiff(antes, htFoto(t, root))
	if len(modificados)+len(borrados) > 0 {
		t.Fatalf("%s: backfill nunca borra ni reescribe: modifico %v, borro %v", ca, modificados, borrados)
	}
	if _, err := os.Stat(item.Path(root, "a-uno")); err != nil {
		t.Fatalf("%s: el item de a-uno, creado antes del fallo, QUEDA (backfill nunca borra un item): %v (creo %v)", ca, err, creados)
	}
	htItemCreado(t, ca, root, "a-uno", h3Casos["a-uno"], desde, hasta)
	if _, err := os.Stat(item.Path(root, "b-dos")); !os.IsNotExist(err) {
		t.Fatalf("%s: el item de b-dos, que no se pudo escribir, no queda (ni a medias): %v", ca, err)
	}
	if raw, err := os.ReadFile(item.Path(root, "ya-estaba")); err != nil || string(raw) != yaEstaba {
		t.Fatalf("%s: el item que ya existia sigue intacto: %v\n%s", ca, err, raw)
	}
	var nuevos []string
	pref := filepath.Join(".hoom", "items") + string(filepath.Separator)
	for _, p := range creados {
		if strings.HasPrefix(p, pref) && strings.HasPrefix(filepath.Base(p), ".") {
			t.Fatalf("%s: nada a medias por item: no queda un temporal en .hoom/items: %s (creo %v)", ca, p, creados)
		}
		if strings.HasPrefix(p, pref) && strings.HasSuffix(p, ".yaml") {
			s := strings.TrimSuffix(filepath.Base(p), ".yaml")
			if w, ok := h3Casos[s]; ok {
				htItemCreado(t, ca, root, s, w, desde, hasta)
			}
			nuevos = append(nuevos, s)
		}
	}
	sort.Strings(nuevos)
	return nuevos
}

// CA-447 (caso limite de la enmienda 1), hallazgo 142d20 (reemplaza a
// d03734 y a5e1d2; cubre 1e8b0c, 039b75 y a7d0b1): con un tope de 8 KiB por
// archivo, el alta de b-dos falla con un error de E/S despues de crear
// a-uno. Backfill termina con error y NUNCA borra un item: a-uno queda,
// completo y valido; b-dos no queda ni a medias; ya-estaba intacto; y la
// salida lista exactamente los items creados (el resultado, el texto, y el
// --json con "error" no vacio). Volver a correr backfill (sin el tope)
// completa el resto sin tocar los ya creados.
func TestCA447_Hallazgo142d20_FallaElSegundoQuedanLosCreados(t *testing.T) {
	if root := os.Getenv(h3EnvRoot); root != "" {
		h3Hijo(t, root, os.Getenv(h3EnvModo))
		return
	}

	t.Run("Backfill y la segunda corrida", func(t *testing.T) {
		ca := "CA-447 (142d20)"
		root, yaEstaba := h3Lote(t)
		antes := htFoto(t, root)
		desde := time.Now().UTC().Truncate(time.Second)
		rep := h3CorrerHijo(t, root, "backfill")
		hasta := time.Now().UTC().Add(time.Second)
		if rep.Err == "" {
			t.Fatalf("%s: si no puede escribir el item de b-dos, backfill termina con error: %+v", ca, rep.Res)
		}
		nuevos := h3Despues(t, ca, root, yaEstaba, antes, desde, hasta)
		if got := htCandSlugs(rep.Res.Creados); !reflect.DeepEqual(got, nuevos) {
			t.Fatalf("%s: el resultado lista exactamente los items que la corrida creo: en disco %v, listados %v", ca, nuevos, got)
		}

		// la segunda corrida, con el disco de vuelta: completa el resto
		medio := htFoto(t, root)
		res, err := Backfill(root, "main", "high", false, time.Now().UTC())
		if err != nil {
			t.Fatalf("%s: volver a correr backfill completa el resto: %v", ca, err)
		}
		despues := htFoto(t, root)
		for _, s := range nuevos {
			k := filepath.Join(".hoom", "items", s+".yaml")
			if despues[k] != medio[k] {
				t.Fatalf("%s: la segunda corrida no toca %s, creado por la primera (ni contenido, ni modo, ni mtime)", ca, s)
			}
			if !h2Tiene(res.ConItem, s) {
				t.Fatalf("%s: la segunda corrida lista %s como que ya tiene item: %v", ca, s, res.ConItem)
			}
		}
		for _, s := range []string{"a-uno", "b-dos", "c-tres"} {
			if _, err := item.Load(root, s); err != nil {
				t.Fatalf("%s: despues de la segunda corrida el item de %s esta, valido: %v", ca, s, err)
			}
		}
		if raw, _ := os.ReadFile(item.Path(root, "ya-estaba")); string(raw) != yaEstaba {
			t.Fatalf("%s: ya-estaba sigue intacto:\n%s", ca, raw)
		}
	})

	t.Run("el texto los lista", func(t *testing.T) {
		ca := "CA-447 (142d20, texto)"
		root, yaEstaba := h3Lote(t)
		antes := htFoto(t, root)
		desde := time.Now().UTC().Truncate(time.Second)
		rep := h3CorrerHijo(t, root, "texto")
		hasta := time.Now().UTC().Add(time.Second)
		if rep.Err == "" {
			t.Fatalf("%s: 'hoom item backfill' termina con error si no puede escribir un item:\n%s", ca, rep.Stdout)
		}
		nuevos := h3Despues(t, ca, root, yaEstaba, antes, desde, hasta)
		for _, s := range nuevos {
			if !h3LineaCon(rep.Stdout, s, "creado") {
				t.Fatalf("%s: la salida de texto lista %s como creado:\n%s", ca, s, rep.Stdout)
			}
		}
		if h3LineaCon(rep.Stdout, "b-dos", "creado") {
			t.Fatalf("%s: la salida de texto no lista b-dos como creado:\n%s", ca, rep.Stdout)
		}
	})

	t.Run("el --json los lista con error", func(t *testing.T) {
		ca := "CA-447 (142d20, --json)"
		root, yaEstaba := h3Lote(t)
		antes := htFoto(t, root)
		desde := time.Now().UTC().Truncate(time.Second)
		rep := h3CorrerHijo(t, root, "json")
		hasta := time.Now().UTC().Add(time.Second)
		if rep.Err == "" {
			t.Fatalf("%s: 'hoom item backfill --json' termina con error si no puede escribir un item:\n%s", ca, rep.Stdout)
		}
		nuevos := h3Despues(t, ca, root, yaEstaba, antes, desde, hasta)
		var js map[string]json.RawMessage
		if err := json.Unmarshal([]byte(rep.Stdout), &js); err != nil {
			t.Fatalf("%s: con el error, --json igual emite el resultado como JSON: %v\n%s", ca, err, rep.Stdout)
		}
		var msg string
		if err := json.Unmarshal(js["error"], &msg); err != nil || strings.TrimSpace(msg) == "" {
			t.Fatalf("%s: --json trae \"error\" no vacio: %v\n%s", ca, err, rep.Stdout)
		}
		var res BackfillResult
		if err := json.Unmarshal([]byte(rep.Stdout), &res); err != nil || res.DryRun {
			t.Fatalf("%s: --json es el BackfillResult de una corrida real: %v\n%s", ca, err, rep.Stdout)
		}
		if got := htCandSlugs(res.Creados); !reflect.DeepEqual(got, nuevos) {
			t.Fatalf("%s: --json lista en \"creados\" exactamente los items que la corrida creo: en disco %v, listados %v", ca, nuevos, got)
		}
	})
}

// h3LineaCon dice si alguna linea de out nombra slug y dice palabra.
func h3LineaCon(out, slug, palabra string) bool {
	for _, l := range strings.Split(out, "\n") {
		for _, campo := range strings.Fields(l) {
			if campo == slug && strings.Contains(l, palabra) {
				return true
			}
		}
	}
	return false
}
