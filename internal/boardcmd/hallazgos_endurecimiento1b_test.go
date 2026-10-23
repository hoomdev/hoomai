// Tests de regresion de la segunda ronda de la tarea endurecimiento-1 en
// boardcmd, sobre .hoom/specs/historia-doctor-y-cinta.md (CA-369) y
// .hoom/specs/evidencia-en-disco.md (CA-442). Escritos desde el spec, el
// texto de cada hallazgo y el contrato que promete la correccion, SIN LEER LA
// IMPLEMENTACION: repos git reales en t.TempDir() y evidencia escrita con las
// APIs publicas (verdict.Write, finding.Register) o a mano. Reusan los
// helpers bd*, hi*, dr*, c4v2Enlace y los e1* de la primera ronda
// (hallazgos_endurecimiento1_test.go); los de este archivo llevan el prefijo
// e1b. Los casos con un FIFO estan en hallazgos_endurecimiento1b_unix_test.go.
//
//   - 20261022T202055_53c2af (high, risk): el tablero y el doctor leian los
//     registros de .hoom/reviews (y de .hoom/verdicts y .hoom/findings)
//     siguiendo enlaces simbolicos y sin exigir un archivo regular: un
//     registro enlazado al de otro proyecto se usaba como propio. Un
//     registro que es un enlace no se usa; y un archivo de evidencia que no
//     se pudo leer o que no es un registro valido hace que Doctor devuelva
//     error (no certifica lo que no leyo) y que Build lo lleve en Warnings
//     (CA-369, CA-442).
//   - 20261022T202657_d69d7e y 20261022T203213_8c1ed6 (reliability,
//     resilience): Doctor solo sondeaba las fuentes del arbol raiz; un
//     espacio de trabajo de una tarea sin item con .hoom/reviews ilegible
//     dejaba al doctor decir "sin problemas de coherencia". Ahora es un error
//     de Doctor (CA-369).
//   - 20261022T202056_9e7983 y 20261022T203213_a2b180 (risk, resilience):
//     con .hoom/envelopes (o .hoom/runs) listable pero un registro ilegible,
//     Doctor no agregaba ninguna falla y podia callar un sobre huerfano.
//     Ahora es un error de Doctor (CA-369, CA-202).
package boardcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------- helpers

// e1bIDReview es el id del registro de review de la tarjeta precios.
const e1bIDReview = "20261022T110000_e1b0a1"

// e1bFuente es una fuente de evidencia que el tablero y el doctor leen
// registro por registro.
type e1bFuente struct {
	dir string // relativo al proyecto
	// crear escribe en el proyecto un registro valido de la tarjeta precios
	// y devuelve su ruta absoluta y su id.
	crear func(t *testing.T, proyecto string) (ruta, id string)
	// usado dice si la evidencia de la tarjeta tomo ese registro.
	usado func(ev Evidence, id string) bool
}

func e1bFuentes() []e1bFuente {
	return []e1bFuente{
		{".hoom/reviews", func(t *testing.T, p string) (string, string) {
			r := reviewcmd.Record{ID: e1bIDReview, CreatedAt: e1Ahora.Add(-time.Hour), Task: bdSlug,
				Spec: ".hoom/specs/" + bdSlug + ".md", Fingerprint: "h", VerdictID: "v", Verdict: "green",
				Lenses: []string{"risk"}, Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}}
			return bdReviewDisco(t, p, r), r.ID
		}, func(ev Evidence, id string) bool {
			for _, r := range ev.Reviews {
				if r.ID == id {
					return true
				}
			}
			return false
		}},
		{".hoom/verdicts", func(t *testing.T, p string) (string, string) {
			v := &verdict.Verdict{Project: "demo", CreatedAt: e1Ahora.Add(-time.Hour), Spec: ".hoom/specs/" + bdSlug + ".md",
				Git: gitx.Snapshot(p, "main"), Gates: hiGates("test", "pass")}
			v.Finalize()
			ruta, err := verdict.Write(p, v)
			if err != nil {
				t.Fatalf("fixture: verdict.Write: %v", err)
			}
			if v.ID == "" {
				t.Fatalf("fixture: el veredicto escrito tiene id")
			}
			return filepath.Join(p, ".hoom", "verdicts", filepath.Base(ruta)), v.ID
		}, func(ev Evidence, id string) bool {
			if ev.Verdict != nil && ev.Verdict.ID == id {
				return true
			}
			for _, g := range ev.GreenVerdicts {
				if g == id {
					return true
				}
			}
			return false
		}},
		{".hoom/findings", func(t *testing.T, p string) (string, string) {
			id := hiHallazgo(t, p, "high", bdSlug)
			return filepath.Join(p, ".hoom", "findings", id+".json"), id
		}, func(ev Evidence, id string) bool {
			for _, f := range ev.Findings {
				if f.ID == id {
					return true
				}
			}
			return false
		}},
	}
}

// e1bProyecto es un repo con el item precios commiteado.
func e1bProyecto(t *testing.T) string {
	t.Helper()
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdCommitear(t, root, "alta del item")
	return root
}

// e1bNombra dice si algun aviso nuevo nombra el registro (su id) o su
// directorio.
func e1bNombra(nuevos []string, id, dir string) bool {
	for _, w := range nuevos {
		if strings.Contains(w, id) || strings.Contains(w, filepath.Base(dir)) {
			return true
		}
	}
	return false
}

// e1bTableroAvisa exige que Build se arme igual y que sus Warnings ganen un
// aviso que nombra el registro o su directorio.
func e1bTableroAvisa(t *testing.T, ca, root string, antes []string, id, dir, que string) {
	t.Helper()
	b, err := Build(root, "main", "high", e1Ahora)
	if err != nil {
		t.Fatalf("%s: el tablero no falla por %s: Build se arma igual: %v", ca, que, err)
	}
	nuevos := e1Nuevos(antes, b.Warnings)
	if !e1bNombra(nuevos, id, dir) {
		t.Errorf("%s: con %s, el tablero lo lleva en Warnings: un aviso nuevo que nombra %s o %s; avisos nuevos: %q (antes %q)",
			ca, que, id, filepath.Base(dir), nuevos, antes)
	}
}

// e1bDoctorFalla exige que Doctor devuelva error y no un reporte.
func e1bDoctorFalla(t *testing.T, ca, root, que string) {
	t.Helper()
	if r, err := Doctor(root, "main", "high", e1Ahora); err == nil {
		t.Errorf("%s: con %s, Doctor devuelve error: no certifica evidencia que no pudo leer; dio un reporte de %d problemas %v",
			ca, que, len(r.Problems), drIDs(r.Problems))
	}
}

// e1bDoctorSano exige el reporte sin error (guarda).
func e1bDoctorSano(t *testing.T, ca, root, que string) DoctorReport {
	t.Helper()
	r, err := Doctor(root, "main", "high", e1Ahora)
	if err != nil {
		t.Fatalf("%s: guarda: %s, Doctor da un reporte sin error: %v", ca, que, err)
	}
	return r
}

// e1bAvisos son los Warnings de Build (guarda: se arma).
func e1bAvisos(t *testing.T, ca, root string) []string {
	t.Helper()
	b, err := Build(root, "main", "high", e1Ahora)
	if err != nil {
		t.Fatalf("%s: guarda: Build arma el tablero: %v", ca, err)
	}
	return b.Warnings
}

// e1bSinPermisos saltea donde un chmod no quita la lectura.
func e1bSinPermisos(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("corre como root: un chmod no le quita la lectura, el test no probaria nada")
	}
}

// ---------------------------------------------------------------- 53c2af

// CA-442, CA-369 (hallazgo 53c2af): un registro de .hoom/reviews,
// .hoom/verdicts o .hoom/findings con nombre de hoom que es un enlace
// simbolico al registro valido (de la misma tarjeta, con el mismo nombre)
// de OTRO proyecto no se usa: la evidencia de la tarjeta (Gather) no lo
// toma, Doctor devuelve error (no es un registro valido de este proyecto) y
// el tablero (Build) se arma igual y lo lleva en Warnings. Guarda en el
// mismo fixture: con una copia REGULAR del mismo archivo en ese lugar, la
// tarjeta si lo toma y Doctor no falla (el enlace es lo unico que cambia).
func TestHallazgo_53c2af_CA442_RegistroEnlazadoAOtroProyectoNoSeUsa(t *testing.T) {
	for _, f := range e1bFuentes() {
		t.Run(f.dir, func(t *testing.T) {
			ca := "CA-442 (hallazgo 53c2af, " + f.dir + " enlazado)"
			otro := bdRepo(t, "")
			origen, id := f.crear(t, otro)
			if fi, err := os.Lstat(origen); err != nil || !fi.Mode().IsRegular() {
				t.Fatalf("fixture: el registro del otro proyecto es un archivo regular: %v %v", fi, err)
			}
			raw, err := os.ReadFile(origen)
			if err != nil {
				t.Fatal(err)
			}

			root := e1bProyecto(t)
			lugar := filepath.Join(root, f.dir, filepath.Base(origen))
			if err := os.MkdirAll(filepath.Dir(lugar), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(lugar, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			if !f.usado(Gather(root, "main", "high", bdItem(bdSlug), e1Ahora), id) {
				t.Fatalf("%s: guarda: con una copia regular del registro en %s, la tarjeta lo toma", ca, f.dir)
			}
			e1bDoctorSano(t, ca, root, "con la copia regular")
			antes := e1bAvisos(t, ca, root)

			if err := os.Remove(lugar); err != nil {
				t.Fatal(err)
			}
			c4v2Enlace(t, origen, lugar)
			if f.usado(Gather(root, "main", "high", bdItem(bdSlug), e1Ahora), id) {
				t.Errorf("%s: un registro que es un enlace simbolico al de otro proyecto no se usa: la tarjeta tomo %s", ca, id)
			}
			e1bDoctorFalla(t, ca, root, "el registro "+id+" como enlace a otro proyecto")
			e1bTableroAvisa(t, ca, root, antes, id, f.dir, "el registro "+id+" como enlace a otro proyecto")
		})
	}
}

// CA-369, CA-442 (hallazgo 53c2af): un archivo de evidencia de .hoom/reviews,
// .hoom/verdicts o .hoom/findings con nombre de hoom que no se pudo leer
// (000) o que no es un registro valido (no es JSON) hace que Doctor
// devuelva error: el doctor certifica la coherencia de toda la evidencia y
// no puede hacerlo sobre un registro que no leyo. El tablero (Build) se
// arma igual y lo lleva en Warnings (un aviso nuevo que lo nombra). Guarda:
// con el registro valido y legible, Doctor da su reporte sin error.
func TestHallazgo_53c2af_CA369_RegistroIlegibleOInvalidoEsUnErrorDelDoctor(t *testing.T) {
	for _, f := range e1bFuentes() {
		for _, variante := range []string{"ilegible", "invalido"} {
			t.Run(f.dir+"/"+variante, func(t *testing.T) {
				ca := "CA-369 (hallazgo 53c2af, registro " + variante + " en " + f.dir + ")"
				root := e1bProyecto(t)
				ruta, id := f.crear(t, root)
				e1bDoctorSano(t, ca, root, "con el registro valido y legible")
				antes := e1bAvisos(t, ca, root)

				que := ""
				switch variante {
				case "ilegible":
					e1bSinPermisos(t)
					if err := os.Chmod(ruta, 0); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.Chmod(ruta, 0o644) })
					if raw, err := os.ReadFile(ruta); err == nil {
						t.Skipf("este sistema deja leer un archivo sin permiso de lectura (%d bytes)", len(raw))
					}
					que = "el registro " + id + " ilegible (000)"
				case "invalido":
					if err := os.WriteFile(ruta, []byte("esto no es un registro de hoom\n"), 0o644); err != nil {
						t.Fatal(err)
					}
					que = "el registro " + id + " que no es un registro valido"
				}
				e1bDoctorFalla(t, ca, root, que)
				e1bTableroAvisa(t, ca, root, antes, id, f.dir, que)
			})
		}
	}
}

// ---------------------------------------------------------------- d69d7e / 8c1ed6

// e1bHuerfana es el slug de la tarea sin item.
const e1bHuerfana = "huerfana"

// e1bWorktreeSinItem arma el proyecto con el espacio de trabajo de una tarea
// sin item (.hoom/worktrees/huerfana, repo git valido en su rama) y deja en
// el lo que arma la fuente.
func e1bWorktreeSinItem(t *testing.T, f e1Fuente) (root, wt string) {
	t.Helper()
	root = bdRepo(t, "findings:\n  block_on: high\n")
	wt = bdWorktree(t, root, e1bHuerfana)
	if _, err := os.Stat(filepath.Join(root, ".hoom", "items", e1bHuerfana+".yaml")); err == nil {
		t.Fatalf("fixture: la tarea %s no tiene item", e1bHuerfana)
	}
	f.armar(t, wt)
	if fi, err := os.Stat(filepath.Join(wt, f.dir)); err != nil || !fi.IsDir() {
		t.Fatalf("fixture: %s existe en el espacio de trabajo sin item: %v %v", f.dir, fi, err)
	}
	return root, wt
}

// CA-369 (hallazgos d69d7e y 8c1ed6): el doctor recorre tambien los espacios
// de trabajo de tareas sin item. Si el de huerfana (.hoom/worktrees/huerfana,
// un repo git valido) tiene .hoom/reviews que existe y no se puede listar
// (000), Doctor devuelve error: no certifica "sin problemas de coherencia"
// sobre reviews que no pudo listar. Guarda: legible, el mismo fixture da un
// reporte sin error.
func TestHallazgo_d69d7e_8c1ed6_CA369_ReviewsIlegiblesEnUnWorktreeSinItemSonUnErrorDelDoctor(t *testing.T) {
	const ca = "CA-369 (hallazgos d69d7e y 8c1ed6, .hoom/reviews ilegible en un worktree sin item)"
	var reviews e1Fuente
	for _, f := range e1Fuentes() {
		if f.dir == ".hoom/reviews" {
			reviews = f
		}
	}
	if reviews.armar == nil {
		t.Fatal("fixture: e1Fuentes trae .hoom/reviews")
	}
	root, wt := e1bWorktreeSinItem(t, reviews)
	e1bDoctorSano(t, ca, root, "con .hoom/reviews legible en el worktree sin item")
	e1SinLectura(t, filepath.Join(wt, reviews.dir))
	e1bDoctorFalla(t, ca, root, ".hoom/worktrees/"+e1bHuerfana+"/.hoom/reviews ilegible")
}

// CA-369 (hallazgo d69d7e: "debe sondearse cada place descubierto"): lo
// mismo para las otras fuentes que el doctor lee de un espacio de trabajo
// sin item (.hoom/envelopes, .hoom/runs, .hoom/verdicts, .hoom/findings):
// si existe y no se puede listar (000), Doctor devuelve error. Guarda:
// legible, el mismo fixture da un reporte sin error.
func TestHallazgo_d69d7e_CA369_FuentesIlegiblesEnUnWorktreeSinItemSonUnErrorDelDoctor(t *testing.T) {
	for _, f := range e1Fuentes() {
		if f.dir == ".hoom/reviews" {
			continue
		}
		t.Run(f.dir, func(t *testing.T) {
			ca := "CA-369 (hallazgo d69d7e, " + f.dir + " ilegible en un worktree sin item)"
			root, wt := e1bWorktreeSinItem(t, f)
			e1bDoctorSano(t, ca, root, "con "+f.dir+" legible en el worktree sin item")
			e1SinLectura(t, filepath.Join(wt, f.dir))
			e1bDoctorFalla(t, ca, root, ".hoom/worktrees/"+e1bHuerfana+"/"+f.dir+" ilegible")
		})
	}
}

// ---------------------------------------------------------------- 9e7983 / a2b180

// CA-369, CA-202 (hallazgos 9e7983 y a2b180): con .hoom/envelopes listable
// pero el registro de un sobre huerfano ilegible (000), Doctor devuelve
// error: no puede certificar que no hay sobres huerfanos sin haber leido
// el unico sobre. Guarda: legible, el doctor ve el sobre huerfano y da su
// reporte sin error.
func TestHallazgo_9e7983_a2b180_CA369_SobreIlegibleEsUnErrorDelDoctor(t *testing.T) {
	const ca = "CA-369 (hallazgos 9e7983 y a2b180, un sobre ilegible)"
	e1bSinPermisos(t)
	root := e1bProyecto(t)
	ruta := bdSobreDisco(t, root, envelope.Record{ID: "20261022T090000_e1b0e1", Role: "writer", Provider: "claude",
		Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: e1Ahora.Add(-2 * time.Hour), UpdatedAt: e1Ahora.Add(-2 * time.Hour), PID: bdPIDMuerto(t)})
	r := e1bDoctorSano(t, ca, root, "con el sobre legible")
	if !strings.Contains(strings.Join(drIDs(r.Problems), " "), ProbSobreHuerfano) {
		t.Fatalf("%s: guarda: legible, el doctor ve el %s: %v", ca, ProbSobreHuerfano, drIDs(r.Problems))
	}
	if err := os.Chmod(ruta, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ruta, 0o644) })
	if _, err := os.ReadFile(ruta); err == nil {
		t.Skip("este sistema deja leer un archivo sin permiso de lectura")
	}
	if _, err := os.ReadDir(filepath.Dir(ruta)); err != nil {
		t.Fatalf("fixture: .hoom/envelopes sigue listable: %v", err)
	}
	e1bDoctorFalla(t, ca, root, "el registro de un sobre ilegible (000) en .hoom/envelopes listable")
}

// CA-369 (hallazgos 9e7983 y a2b180, MetasErr): lo mismo con el sidecar de
// un run: .hoom/runs listable y un <id>.meta.json ilegible (000) hacen que
// Doctor devuelva error. Guarda: legible, reporte sin error.
func TestHallazgo_9e7983_a2b180_CA369_SidecarIlegibleEsUnErrorDelDoctor(t *testing.T) {
	const ca = "CA-369 (hallazgos 9e7983 y a2b180, un sidecar ilegible)"
	e1bSinPermisos(t)
	root := e1bProyecto(t)
	ruta := bdRunDisco(t, root, runcmd.Meta{ID: "20261022T090000_e1b0f2", Provider: "claude", Role: "writer", Task: bdSlug,
		Dir: root, CreatedAt: e1Ahora.Add(-time.Hour), Status: runcmd.StatusDone, EndedAt: e1Ahora.Add(-50 * time.Minute)})
	e1bDoctorSano(t, ca, root, "con el sidecar legible")
	if err := os.Chmod(ruta, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ruta, 0o644) })
	if _, err := os.ReadFile(ruta); err == nil {
		t.Skip("este sistema deja leer un archivo sin permiso de lectura")
	}
	e1bDoctorFalla(t, ca, root, "el sidecar de un run ilegible (000) en .hoom/runs listable")
}
