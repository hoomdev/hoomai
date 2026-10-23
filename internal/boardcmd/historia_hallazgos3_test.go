// Tests de la delta 1 de la review de historia-en-el-tablero (ronda 3 del
// test-writer) sobre el tablero: Candidatas y la derivacion de las tarjetas
// historicas. Valores no confiables (la severidad de un hallazgo, el nombre
// de un archivo de hallazgo ilegible) nunca llegan crudos al motivo ni al
// doctor (763d5f, feeb49); el titulo nunca sale de un spec fuera del
// espacio de trabajo (e8b795); y una historica cuya tarea tiene un espacio
// de trabajo que no se puede usar no se sostiene por lo que hay en root
// (f448aa). Escritos desde el spec .hoom/specs/historia-en-el-tablero.md
// (enmienda 1) y los textos de los hallazgos, sin leer la implementacion.
// Repos git reales en t.TempDir(). Todo identificador de este archivo lleva
// el prefijo h3.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
)

const (
	// h3SevHostil es una severidad con un hipervinculo OSC 8 (ESC ... BEL),
	// un retorno de carro y un salto de linea que falsifica una linea.
	h3SevHostil = "\x1b]8;;http://x\x07hihg\r\nFALSO  inyectado  creado"
	// h3Falso es lo que una linea falsificada diria al principio.
	h3Falso = "FALSO"
	// h3Secreto es el titulo del spec que vive FUERA del espacio de trabajo.
	h3Secreto = "SECRETO-DE-AFUERA"
)

// h3NombresHostiles son nombres de archivos de hallazgo ilegibles con ESC
// (una secuencia de color) y con un salto de linea.
var h3NombresHostiles = []string{
	"20261023T000000_a\x1b[31mROJO\x1b[0m.json",
	"20261023T000000_b\nFALSO  inyectado  creado.json",
}

// h3EsCrudo: un control C0 (incluidos \n y \t: aca ninguno es legitimo,
// porque cada texto es UNA linea), DEL, un control C1, o un caracter de
// embebido, anulacion o aislamiento de direccion.
func h3EsCrudo(r rune) bool {
	switch {
	case r < 0x20, r >= 0x7f && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// h3SinCrudos exige que s (un texto de una linea para una persona) no traiga
// ningun caracter crudo.
func h3SinCrudos(t *testing.T, ca, que, s string) {
	t.Helper()
	for _, r := range s {
		if h3EsCrudo(r) {
			t.Fatalf("%s: %s no trae caracteres de control crudos (U+%04X): %+q", ca, que, r, s)
		}
	}
}

// h3ProblemaSinCrudos exige lo mismo de cada campo de un problema.
func h3ProblemaSinCrudos(t *testing.T, ca string, p Problem) {
	t.Helper()
	for campo, v := range map[string]string{"What": p.What, "Plain": p.Plain, "Action": p.Action, "Ref": p.Ref} {
		h3SinCrudos(t, ca, fmt.Sprintf("el %s de %s de %s", campo, p.ID, p.Slug), v)
	}
}

// ---------------------------------------------------------------------------
// 763d5f / feeb49: valores no confiables en el motivo y en el doctor.
// ---------------------------------------------------------------------------

// CA-450, CA-452, hallazgos 763d5f y feeb49: un hallazgo abierto de la tarea
// con una severidad que trae ESC/OSC, \r y \n, o un archivo de hallazgo
// ilegible cuyo nombre trae ESC o \n. El fail-closed se conserva (la tarea
// no califica; la historica existente no se sostiene; con los ilegibles, el
// doctor del proyecto falla cerrado y su error los nombra), pero ninguna
// salida trae esos caracteres crudos: ni el motivo de Candidatas, ni el de
// Gather, ni el doctor y el texto de la tarjeta, ni el doctor del proyecto
// (su reporte y su texto, o su error). No pueden emitir secuencias de
// terminal ni falsificar lineas.
func TestCA450_Hallazgo763d5f_ValoresNoConfiablesSinCrudos(t *testing.T) {
	t.Run("763d5f severidad hostil", func(t *testing.T) {
		root := bdRepo(t, htBlockOn)
		for _, s := range []string{"nueva", "hist"} {
			htCerrar(t, root, htHallazgo(t, root, s, "high"), finding.StatusCorrected)
		}
		bdEscribir(t, root, ".hoom/items/hist.yaml", htItemYAML("hist", "bug", item.HistoricoHallazgos))
		bdCommitear(t, root, "hallazgos cerrados e item historico")
		h1SeSostiene(t, "CA-451 (precondicion)", root, "hist")
		for _, s := range []string{"nueva", "hist"} {
			h1Severidad(t, root, htHallazgo(t, root, s, "high"), h3SevHostil)
		}
		h3Comprobar(t, "763d5f", root)
	})
	t.Run("feeb49 nombre de hallazgo ilegible hostil", func(t *testing.T) {
		root := bdRepo(t, htBlockOn)
		for _, s := range []string{"nueva", "hist"} {
			htCerrar(t, root, htHallazgo(t, root, s, "high"), finding.StatusCorrected)
		}
		bdEscribir(t, root, ".hoom/items/hist.yaml", htItemYAML("hist", "bug", item.HistoricoHallazgos))
		bdCommitear(t, root, "hallazgos cerrados e item historico")
		h1SeSostiene(t, "CA-451 (precondicion)", root, "hist")
		for _, n := range h3NombresHostiles {
			bdEscribir(t, root, filepath.Join(".hoom", "findings", n), "{esto no es un hallazgo")
		}
		h3Comprobar(t, "feeb49", root, "20261023T000000_a", "20261023T000000_b")
	})
}

// h3Comprobar: "nueva" (sin item) no califica y su motivo no trae crudos; la
// historica "hist" no se sostiene, y ni Gather, ni el doctor de la tarjeta,
// ni el texto de la tarjeta traen crudos. El doctor del proyecto: con
// archivos ilegibles (ilegibles, las piezas de sus nombres que el error tiene
// que nombrar) falla cerrado (endurecimiento-1, hallazgo d0d331) y su error,
// que tambien es salida humana, no trae crudos; sin ellos da su reporte, con
// UN historica-sin-sosten de hist, y ni sus problemas ni su texto traen
// crudos.
func h3Comprobar(t *testing.T, id, root string, ilegibles ...string) {
	t.Helper()
	ca := "CA-450 (" + id + ")"
	c := htCand(t, ca, htCandidatas(t, ca, root, "high"), "nueva")
	if c.ConItem || c.Historico != "" || c.Motivo == "" {
		t.Fatalf("%s: el fail-closed se conserva: nueva no califica y dice por que: %+v", ca, c)
	}
	h3SinCrudos(t, ca, "el motivo de nueva", c.Motivo)

	ca = "CA-452 (" + id + ")"
	var problemas []Problem
	var buf bytes.Buffer
	if len(ilegibles) > 0 {
		msg := h1DoctorFallaCerrado(t, ca, root, ilegibles...)
		h3SinCrudos(t, ca, "el error del doctor del proyecto", msg)
	} else {
		h1NoSeSostiene(t, ca, root, "hist", "")
		r, err := Doctor(root, "main", "high", htAhora)
		if err != nil {
			t.Fatalf("%s: Doctor: %v", ca, err)
		}
		problemas = append(problemas, r.Problems...)
		RenderDoctor(&buf, r)
		h3TextoLimpio(t, ca, "el texto de 'hoom board doctor'", buf.String())
	}
	card := h1NoSeSostieneEnLaTarjeta(t, ca, root, "hist", "")
	ev := Gather(root, "main", "high", card.Item, htAhora)
	h3SinCrudos(t, ca, "el motivo de Gather de hist", ev.HistoricaMotivo)
	buf.Reset()
	RenderCard(&buf, card)
	h3TextoLimpio(t, ca, "el texto de la tarjeta hist ('hoom item show')", buf.String())
	for _, p := range append(problemas, card.Doctor...) {
		h3ProblemaSinCrudos(t, ca, p)
	}
}

// h3TextoLimpio exige que ninguna linea de un texto de varias lineas traiga
// un caracter crudo (\t se permite).
func h3TextoLimpio(t *testing.T, ca, que, texto string) {
	t.Helper()
	for _, l := range strings.Split(texto, "\n") {
		for _, r := range l {
			if r != '\t' && h3EsCrudo(r) {
				t.Fatalf("%s: %s no trae caracteres de control crudos (U+%04X) en %+q", ca, que, r, l)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// e8b795: el spec del espacio de trabajo por un symlink.
// ---------------------------------------------------------------------------

// CA-447, hallazgo e8b795: en el espacio de trabajo de la tarea "fuga",
// .hoom/specs es un symlink a un directorio FUERA del espacio, con un
// fuga.md cuyo encabezado es conocido; la tarea tiene un verde aprobado e
// integrado atado a .hoom/specs/fuga.md. Candidatas nunca devuelve ese
// titulo (ni nada de ese archivo): puede fallar (error) o tratar la tarea
// sin el titulo de afuera (el slug), pero el titulo de afuera no aparece.
func TestCA447_Hallazgoe8b795_SpecPorSymlinkNoDaTitulo(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	wt := bdWorktree(t, root, "fuga")
	fuera := t.TempDir()
	htSpec(t, fuera, "fuga", "# Spec: "+h3Secreto)
	link := filepath.Join(wt, ".hoom", "specs")
	if err := os.RemoveAll(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(fuera, ".hoom", "specs"), link); err != nil {
		t.Fatal(err)
	}
	htForma(t, htVeredicto(t, wt, wt, "fuga", bdT0, false, bdGatesVerdes(), nil), true, false)

	cs, err := Candidatas(root, "main", "high", htAhora)
	if err != nil {
		if strings.Contains(err.Error(), h3Secreto) {
			t.Fatalf("CA-447 (e8b795): el error no trae el titulo de afuera: %v", err)
		}
		return // aceptable: no lee el spec de afuera
	}
	raw, _ := json.Marshal(cs)
	if strings.Contains(string(raw), h3Secreto) {
		t.Fatalf("CA-447 (e8b795): Candidatas no usa el titulo de un spec fuera del espacio de trabajo (por un symlink en .hoom/specs):\n%s",
			htFmtCands(cs))
	}
}

// ---------------------------------------------------------------------------
// f448aa: el espacio de trabajo de la tarea no se puede usar.
// ---------------------------------------------------------------------------

// CA-452, hallazgo f448aa: una historica cuya tarea tiene su espacio de
// trabajo fuera de su rama (el tablero lo dice en Evidence.WorktreeInusable)
// no se sostiene por la evidencia de root, aunque alli este su testigo
// cerrado o su verde: lo que hay en el espacio no se pudo mirar y puede
// tumbarla. No queda en Hecho y el doctor de la tarjeta (sus insignias, el
// de 'hoom item show') da historica-sin-sosten con su accion. El doctor del
// proyecto, con un espacio fuera de su rama, es un error que envuelve
// runcmd.ErrOtraRama (hallazgo 966474), asi que aca no se lo mira. Por
// hallazgos y por veredicto.
func TestCA452_Hallazgof448aa_EspacioInusableNoSostiene(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	htCerrar(t, root, htHallazgo(t, root, "rama-h", "high"), finding.StatusCorrected)
	htSpec(t, root, "rama-v", "# Spec: rama-v")
	bdAprobar(t, root, "rama-v")
	bdCommitear(t, root, "spec y hallazgo")
	v := htVeredicto(t, root, root, "rama-v", bdT0, false, bdGatesVerdes(), nil)
	htForma(t, v, true, false)
	bdEscribir(t, root, ".hoom/items/rama-h.yaml", htItemYAML("rama-h", "bug", item.HistoricoHallazgos))
	bdEscribir(t, root, ".hoom/items/rama-v.yaml", htItemYAML("rama-v", "feature", item.HistoricoVeredicto(v.ID)))
	bdCommitear(t, root, "verde e items historicos")
	slugs := []string{"rama-h", "rama-v"}
	for _, s := range slugs {
		bdWorktree(t, root, s)
		h1SeSostiene(t, "CA-451 (precondicion: en su rama)", root, s)
	}
	for _, s := range slugs {
		wt := filepath.Join(root, ".hoom", "worktrees", s)
		bdGit(t, wt, "checkout", "-q", "-b", "otra-"+s)
		b := h1Tablero(t, root)
		if ev := Gather(root, "main", "high", bdCard(t, b, s).Item, htAhora); ev.WorktreeInusable == "" {
			t.Fatalf("precondicion: el espacio de %s fuera de su rama no se puede usar (WorktreeInusable): %+v", s, ev.WorktreeInusable)
		}
	}
	b := h1Tablero(t, root)
	for _, s := range slugs {
		t.Run(s, func(t *testing.T) {
			ca := "CA-452 (f448aa)"
			c := bdCard(t, b, s)
			ev := Gather(root, "main", "high", c.Item, htAhora)
			if ev.HistoricaSostenida || ev.HistoricaMotivo == "" {
				t.Fatalf("%s: con su espacio inusable, la evidencia de root no sostiene a %s (%s), y Gather dice por que: sostenida=%v motivo=%q",
					ca, s, c.Item.Historico, ev.HistoricaSostenida, ev.HistoricaMotivo)
			}
			if c.Column == ColHecho || c.Historica {
				t.Fatalf("%s: la historica %s que no se sostiene deja Hecho y la marca; fue %s historica=%v", ca, s, c.Column, c.Historica)
			}
			n := 0
			for _, p := range c.Doctor {
				if p.ID == ProbHistoricaSinSosten {
					n++
					if p.Action == "" || p.What == "" || p.Slug != s {
						t.Fatalf("%s: historica-sin-sosten de %s trae la tarjeta, que y su accion: %+v", ca, s, p)
					}
				}
			}
			if n != 1 {
				t.Fatalf("%s: el doctor de la tarjeta %s da UN historica-sin-sosten: %s", ca, s, drFmt(c.Doctor))
			}
		})
	}
}
