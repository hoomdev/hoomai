// Tests de los hallazgos de la review de historia-en-el-tablero (registro
// 20261023T132035_0607fa) sobre el verbo `hoom item backfill`: que no crea
// lo que la evidencia no sostiene (hallazgos ilegibles, severidades
// desconocidas, veredictos de espacios ajenos), que un spec que no se puede
// leer no degrada el titulo, y que un git que no puede decir si un commit
// esta integrado es un error y no un motivo. Que un fallo a mitad de la
// escritura no deje nada a medias esta en historia_hallazgos1_unix_test.go.
// Escritos desde el spec .hoom/specs/historia-en-el-tablero.md y los
// textos de los hallazgos, sin leer la implementacion. Repos git reales en
// t.TempDir(). Sin ningun CLI de IA. Todo identificador de este archivo lleva
// el prefijo h1.
package itemcmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
)

const (
	h1BlockOn = "findings:\n  block_on: high\n"
	// h1Ilegible es un archivo de hallazgo que no se puede leer.
	h1Ilegible = "20261023T000000_dead00"
)

// h1Severidad reescribe a mano la severidad del hallazgo id, como llegaria
// por git sin pasar por Register (que la valida).
func h1Severidad(t *testing.T, dir, id, sev string) {
	t.Helper()
	p := filepath.Join(dir, ".hoom", "findings", id+".json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("precondicion: el hallazgo %s vive en %s: %v", id, p, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m["severity"] == nil {
		t.Fatalf("precondicion: el JSON del hallazgo trae \"severity\": %v %s", err, raw)
	}
	m["severity"] = sev
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// h1Backfill corre backfill (sin --dry-run) y exige que salga bien.
func h1Backfill(t *testing.T, ca, root string) BackfillResult {
	t.Helper()
	res, err := Backfill(root, "main", "high", false, time.Now().UTC())
	if err != nil {
		t.Fatalf("%s: backfill sale bien aunque alguna tarea no califique: %v", ca, err)
	}
	return res
}

// h1Creado exige que backfill haya creado el item de slug.
func h1Creado(t *testing.T, ca, root string, res BackfillResult, slug string) {
	t.Helper()
	if !h1Tiene(res.Creados, slug) {
		t.Fatalf("%s: backfill crea %s: creados %v, no califican %+v", ca, slug, htCandSlugs(res.Creados), res.NoCalifican)
	}
	if _, err := os.Stat(item.Path(root, slug)); err != nil {
		t.Fatalf("%s: backfill crea .hoom/items/%s.yaml: %v", ca, slug, err)
	}
}

// h1NoCrea exige que slug no califique, no se cree, y que su motivo diga sub.
func h1NoCrea(t *testing.T, ca, root string, res BackfillResult, slug, sub string) {
	t.Helper()
	if _, err := os.Stat(item.Path(root, slug)); !os.IsNotExist(err) {
		t.Fatalf("%s: backfill no crea .hoom/items/%s.yaml (creados %v): %v", ca, slug, htCandSlugs(res.Creados), err)
	}
	for _, c := range res.NoCalifican {
		if c.Slug == slug {
			if c.Historico != "" || !strings.Contains(c.Motivo, sub) {
				t.Fatalf("%s: %s no califica y su motivo dice %q: %+v", ca, slug, sub, c)
			}
			return
		}
	}
	t.Fatalf("%s: %s sale entre las que no califican, con su motivo: %+v", ca, slug, res.NoCalifican)
}

func h1Tiene(cs []boardcmd.Candidata, slug string) bool {
	for _, c := range cs {
		if c.Slug == slug {
			return true
		}
	}
	return false
}

// h1Items son los nombres en .hoom/items/ (sin leer su contenido: sirve con
// un spec que no se puede leer en el arbol).
func h1Items(t *testing.T, root string) []string {
	t.Helper()
	es, err := os.ReadDir(filepath.Join(root, ".hoom", "items"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// 1d6b30 / 6fdcc5: un hallazgo ilegible.
// ---------------------------------------------------------------------------

// CA-448, CA-450, hallazgos 1d6b30 y 6fdcc5: con un hallazgo ilegible en el
// arbol (tarea y severidad desconocidas; el gate findings_open falla cerrado
// ante el) y un hallazgo legible y cerrado de la tarea, backfill no puede
// demostrar que los bloqueantes de la tarea estan cerrados: no crea su item
// (exit 0) y el motivo nombra el archivo ilegible. Nada se escribe.
func TestCA450_Hallazgo1d6b30_BackfillNoCreaConUnHallazgoIlegible(t *testing.T) {
	root := htRepo(t, h1BlockOn)
	htCerrar(t, root, htHallazgo(t, root, "corr", "high"), finding.StatusCorrected)
	htCommitear(t, root, "hallazgo cerrado")
	pre, err := Backfill(root, "main", "high", true, time.Now().UTC())
	if err != nil || !h1Tiene(pre.Creados, "corr") {
		t.Fatalf("precondicion: sin el ilegible corr califica: %+v %v", pre, err)
	}
	icEscribir(t, root, ".hoom/findings/"+h1Ilegible+".json", "{esto no es un hallazgo")
	htCommitear(t, root, "un hallazgo ilegible")

	antes := htFoto(t, root)
	res := h1Backfill(t, "CA-450 (1d6b30)", root)
	h1NoCrea(t, "CA-450 (1d6b30)", root, res, "corr", h1Ilegible)
	htSinCambios(t, "CA-450 (1d6b30)", "backfill con un hallazgo ilegible", antes, htFoto(t, root))
}

// ---------------------------------------------------------------------------
// eda92d: severidad desconocida.
// ---------------------------------------------------------------------------

// CA-450, hallazgo eda92d: un hallazgo ABIERTO de la tarea con una severidad
// que hoom no conoce ("hihg") cuenta como bloqueante, como en el gate
// findings_open, que falla cerrado ante el: backfill no crea el item y el
// motivo nombra el hallazgo. El control: la tarea de al lado, con sus
// hallazgos cerrados, si se crea.
func TestCA450_Hallazgoeda92d_BackfillSeveridadDesconocidaBloquea(t *testing.T) {
	root := htRepo(t, h1BlockOn)
	htCerrar(t, root, htHallazgo(t, root, "corr", "high"), finding.StatusCorrected)
	htCerrar(t, root, htHallazgo(t, root, "sana", "high"), finding.StatusCorrected)
	raro := htHallazgo(t, root, "corr", "high")
	h1Severidad(t, root, raro, "hihg")
	htCommitear(t, root, "hallazgos")

	res := h1Backfill(t, "CA-450 (eda92d)", root)
	h1Creado(t, "CA-448 (control)", root, res, "sana")
	h1NoCrea(t, "CA-450 (eda92d)", root, res, "corr", raro)
}

// ---------------------------------------------------------------------------
// 552a4e: lo que backfill elige, la derivacion lo sostiene.
// ---------------------------------------------------------------------------

// CA-447, CA-452, hallazgo 552a4e: backfill elige el veredicto en los arboles
// donde la derivacion lo va a volver a buscar. Un verde aprobado e integrado
// del spec de "propia" que solo vive en el espacio de trabajo de OTRA tarea
// ("ajena") no es elegido: "propia" no califica ("sin veredicto verde
// aprobado en la base") y no se crea. Y la propiedad: despues de backfill,
// todo item recien creado se sostiene (Gather) y ninguno da
// historica-sin-sosten. Los controles: "precios" (su verde en root) y
// "enrama" (su verde integrado en su propio espacio) se crean.
func TestCA452_Hallazgo552a4e_LoQueBackfillEligeSeSostiene(t *testing.T) {
	root := htRepo(t, h1BlockOn)
	for _, s := range []string{"propia", "enrama", "precios"} {
		htSpec(t, root, s, "# Spec: "+s)
		htAprobar(t, root, s)
	}
	htCommitear(t, root, "specs")
	htVeredicto(t, root, "precios", htT0, htGatesVerdes(), true)
	htCommitear(t, root, "verde de precios")
	ajena := htWorktree(t, root, "ajena")
	vAjena := htVeredicto(t, ajena, "propia", htT0, htGatesVerdes(), true)
	enrama := htWorktree(t, root, "enrama")
	htVeredicto(t, enrama, "enrama", htT0, htGatesVerdes(), true)

	res := h1Backfill(t, "CA-447 (552a4e)", root)
	h1Creado(t, "CA-447 (control)", root, res, "precios")
	h1Creado(t, "CA-447 (control)", root, res, "enrama")

	t.Run("CA-452 todo lo creado se sostiene", func(t *testing.T) {
		for _, c := range res.Creados {
			it, err := item.Load(root, c.Slug)
			if err != nil {
				t.Fatalf("CA-452: el item creado de %s se lee: %v", c.Slug, err)
			}
			if ev := boardcmd.Gather(root, "main", "high", it, time.Now().UTC()); !ev.HistoricaSostenida {
				t.Fatalf("CA-452 (552a4e): lo que backfill elige, la derivacion lo sostiene: %s (%s) no se sostiene: %q",
					c.Slug, it.Historico, ev.HistoricaMotivo)
			}
		}
		r, err := boardcmd.Doctor(root, "main", "high", time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range r.Problems {
			if p.ID == boardcmd.ProbHistoricaSinSosten {
				t.Fatalf("CA-452 (552a4e): ningun item recien creado da historica-sin-sosten: %+v", p)
			}
		}
	})
	t.Run("CA-447 el verde de un espacio ajeno no se elige", func(t *testing.T) {
		for _, c := range res.Creados {
			if strings.Contains(c.Historico, vAjena.ID) {
				t.Fatalf("CA-447 (552a4e): backfill no elige el veredicto %s, que solo esta en el espacio de otra tarea: %+v", vAjena.ID, c)
			}
		}
		h1NoCrea(t, "CA-447 (552a4e)", root, res, "propia", htMotivoVeredicto)
	})
}

// ---------------------------------------------------------------------------
// b16c61: un spec que existe pero no se puede leer.
// ---------------------------------------------------------------------------

// h1Precios arma un proyecto con una sola tarea que califica: "precios", con
// el spec cuerpo, aprobado, y su verde integrado.
func h1Precios(t *testing.T, cuerpo string) string {
	t.Helper()
	root := htRepo(t, h1BlockOn)
	icEscribir(t, root, ".hoom/specs/precios.md", cuerpo)
	htAprobar(t, root, "precios")
	htCommitear(t, root, "spec")
	htVeredicto(t, root, "precios", htT0, htGatesVerdes(), true)
	htCommitear(t, root, "verde")
	return root
}

// h1CuerpoSpec es un spec valido con ese primer encabezado y, antes, lo que
// se le pase.
func h1CuerpoSpec(antes, encabezado string) string {
	return antes + encabezado + "\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n- CA-1: algo. [verifica: true]\n\n## Decisiones\nx\n\n## Riesgos\nx\n"
}

// h1Titulo lee el titulo del item creado de slug.
func h1Titulo(t *testing.T, root, slug string) string {
	t.Helper()
	raw, err := os.ReadFile(item.Path(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatalf("el item de %s es YAML: %v\n%s", slug, err, raw)
	}
	s, _ := m["titulo"].(string)
	return s
}

// CA-447, hallazgo b16c61: el titulo sale del primer encabezado del spec, o
// del slug cuando NO hay spec. Un spec que existe pero no se puede leer no es
// "no hay spec": backfill no crea un item con el titulo degradado al slug.
//   - permisos 000: termina con error y no crea nada;
//   - una linea de mas de 64 KiB antes del encabezado: termina con error sin
//     crear nada, o lee el encabezado de verdad; nunca el slug;
//   - el control: sin spec (tarea de correccion), el titulo es el slug.
func TestCA447_Hallazgob16c61_SpecIlegibleNoDegradaElTitulo(t *testing.T) {
	t.Run("permisos 000", func(t *testing.T) {
		if os.Geteuid() == 0 || runtime.GOOS == "windows" {
			t.Skip("como root (o en windows) los permisos 000 no impiden leer")
		}
		root := h1Precios(t, h1CuerpoSpec("", "# Spec: Precios por region"))
		pre, err := Backfill(root, "main", "high", true, time.Now().UTC())
		if err != nil || !h1Tiene(pre.Creados, "precios") {
			t.Fatalf("precondicion: con el spec legible precios califica: %+v %v", pre, err)
		}
		spec := filepath.Join(root, ".hoom", "specs", "precios.md")
		if err := os.Chmod(spec, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(spec, 0o644) })
		antes := h1Items(t, root)
		res, err := Backfill(root, "main", "high", false, time.Now().UTC())
		if err == nil {
			titulo := ""
			if _, e := os.Stat(item.Path(root, "precios")); e == nil {
				titulo = h1Titulo(t, root, "precios")
			}
			t.Fatalf("CA-447 (b16c61): con el spec de precios sin permiso de lectura backfill termina con error; salio bien (creados %v, titulo %q)",
				htCandSlugs(res.Creados), titulo)
		}
		if despues := h1Items(t, root); !reflect.DeepEqual(despues, antes) {
			t.Fatalf("CA-447 (b16c61): con el error no se crea ningun item: %v -> %v", antes, despues)
		}
	})
	t.Run("linea de mas de 64 KiB antes del encabezado", func(t *testing.T) {
		larga := strings.Repeat("x", 70*1024) + "\n\n"
		root := h1Precios(t, h1CuerpoSpec(larga, "# Spec: Precios por region"))
		antes := htFoto(t, root)
		res, err := Backfill(root, "main", "high", false, time.Now().UTC())
		if err != nil {
			htSinCambios(t, "CA-447 (b16c61)", "un backfill que no pudo leer el spec", antes, htFoto(t, root))
			return
		}
		if !h1Tiene(res.Creados, "precios") {
			t.Fatalf("CA-447 (b16c61): o termina con error, o crea precios con su titulo: %+v", res)
		}
		if got := h1Titulo(t, root, "precios"); got != "Precios por region" {
			t.Fatalf("CA-447 (b16c61): el titulo de precios es el primer encabezado de su spec (\"Precios por region\"), nunca el slug: %q", got)
		}
	})
	t.Run("control: sin spec el titulo es el slug", func(t *testing.T) {
		root := htRepo(t, h1BlockOn)
		htCerrar(t, root, htHallazgo(t, root, "correccion", "high"), finding.StatusCorrected)
		htCommitear(t, root, "hallazgo")
		res := h1Backfill(t, "CA-448", root)
		h1Creado(t, "CA-448", root, res, "correccion")
		if got := h1Titulo(t, root, "correccion"); got != "correccion" {
			t.Fatalf("CA-448: sin spec el titulo es el slug: %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// 3098f2: git no puede decir si el commit esta integrado.
// ---------------------------------------------------------------------------

// h1GitFalso pone en el PATH un git que falla (exit 128) en cualquier
// 'merge-base --is-ancestor' y deja pasar todo lo demas al git real. Devuelve
// el archivo donde anota cada invocacion.
func h1GitFalso(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("el git falso es un script de sh")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "invocaciones")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> '" + log + "'\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = \"--is-ancestor\" ]; then\n" +
		"    echo 'fatal: git falso: no se puede comprobar' >&2\n" +
		"    exit 128\n" +
		"  fi\n" +
		"done\n" +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// CA-447, CA-449, hallazgo 3098f2: "integrado en la base" es una pregunta a
// git. Si git no puede contestarla (falla con un codigo distinto de 0 y 1, o
// la base no existe), backfill termina con error y no clasifica ni escribe
// nada: no vale el motivo "sin veredicto verde aprobado en la base" con
// exit 0, que es una decision de negocio sobre algo que no se pudo comprobar.
func TestCA449_Hallazgo3098f2_GitSinRespuestaEsError(t *testing.T) {
	nuevo := func(t *testing.T) string {
		t.Helper()
		root := h1Precios(t, h1CuerpoSpec("", "# Spec: Precios por region"))
		pre, err := Backfill(root, "main", "high", true, time.Now().UTC())
		if err != nil || !h1Tiene(pre.Creados, "precios") {
			t.Fatalf("precondicion: con git sano y la base main, precios califica: %+v %v", pre, err)
		}
		return root
	}
	t.Run("una base que no existe", func(t *testing.T) {
		root := nuevo(t)
		antes := htFoto(t, root)
		res, err := Backfill(root, "no-existe", "high", false, time.Now().UTC())
		if err == nil {
			t.Fatalf("CA-449 (3098f2): con una base que no existe backfill termina con error; salio bien: %+v", res)
		}
		htSinCambios(t, "CA-449 (3098f2)", "backfill con una base que no existe", antes, htFoto(t, root))
	})
	t.Run("git falla en merge-base --is-ancestor", func(t *testing.T) {
		root := nuevo(t)
		antes := htFoto(t, root)
		log := h1GitFalso(t)
		res, err := Backfill(root, "main", "high", false, time.Now().UTC())
		raw, _ := os.ReadFile(log)
		if !strings.Contains(string(raw), "--is-ancestor") {
			t.Fatalf("precondicion: backfill pregunta a git si el commit esta integrado (merge-base --is-ancestor); el git falso vio:\n%s", raw)
		}
		if err == nil {
			t.Fatalf("CA-449 (3098f2): si git no puede decir si el commit es ancestro de la base, backfill termina con error; salio bien: %+v", res)
		}
		htSinCambios(t, "CA-449 (3098f2)", "backfill con git sin respuesta", antes, htFoto(t, root))
	})
}
