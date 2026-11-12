// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md
// (CA-648) sobre `hoom aprobar <slug>`: firmar desde la terminal no lanza
// ningun trabajo ni corre la vuelta del piloto, y agrega la linea
// `la tarjeta tiene piloto automatico: sigue con: hoom piloto <slug>` solo
// cuando la tarjeta tiene `auto: hasta-humano` en su item (leido como el
// riesgo: el del checkout principal cuando lo tiene). CLIs falsos en un PATH
// minimo, nunca uno real.
package tarjetacmd_test

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

const (
	aetConAuto  = "presupuesto_usd: 5\nauto: hasta-humano\n"
	aetSinAuto  = "presupuesto_usd: 5\n"
	aetCriterio = "- CA-948: todavia nadie lo cita."
)

func aetAviso(slug string) string {
	return "la tarjeta tiene piloto automatico: sigue con: hoom piloto " + slug
}

// aetCarta: item (con extra) guardado en el proyecto, tarea abierta y un spec
// completo commiteado en el espacio de trabajo, sin firma (Tu aprobacion).
func aetCarta(t *testing.T, root, slug, extra string) string {
	t.Helper()
	vdtItem(t, root, slug, extra)
	wt := vdtTarea(t, root, slug)
	vdtEscribir(t, wt, vdtSpecRel(slug), vdtSpecTexto(aetCriterio))
	vdtCommit(t, wt, "spec sin aprobar")
	vdtColumna(t, "CA-648", root, slug, boardcmd.ColTuAprobacion)
	return wt
}

func aetTieneLinea(out, linea string) bool {
	for _, l := range vdtLineas(out) {
		if strings.TrimSpace(l) == linea {
			return true
		}
	}
	return false
}

// aetNadaLanzado: ningun CLI corrio, no hay sobres ni runs, y la vuelta del
// piloto no guardo nada en la rama de la tarea.
func aetNadaLanzado(t *testing.T, ca, que, root, wt, headTarea string, f *vdtFalsos) {
	t.Helper()
	f.ninguna(t, ca, que)
	if recs := envelope.List(root); len(recs) != 0 {
		t.Fatalf("%s: %s no lanza ningun trabajo: sobres %+v", ca, que, recs)
	}
	if ms := runcmd.Metas(root); len(ms) != 0 {
		t.Fatalf("%s: %s no corre nada: runs %+v", ca, que, ms)
	}
	if h := vdtGit(t, wt, "rev-parse", "HEAD"); h != headTarea {
		t.Fatalf("%s: %s no corre la vuelta del piloto (ni su guardado): HEAD de la tarea %s -> %s", ca, que, headTarea, h)
	}
}

// CA-648: con auto, `hoom aprobar <slug> --si` firma como hoy (exit 0, la
// linea de aprobado, la tarjeta pasa al test-writer), agrega el aviso del
// piloto y no lanza nada aunque haya un CLI con tope.
func TestCA648_AprobarConAutoAvisaYNoLanza(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	f.cli(t, "codex", "")
	root := vdtProyecto(t, "")
	const slug = "firma-con-piloto"
	wt := aetCarta(t, root, slug, aetConAuto)
	head := vdtGit(t, wt, "rev-parse", "HEAD")

	s := vdtCorrer(t, "CA-648", vdtOpts{dir: root}, "aprobar", slug, "--si")
	vdtExit(t, "CA-648", s, 0)
	if m := vdtAprobadoRe.FindStringSubmatch(s.out); m == nil || m[1] != slug {
		t.Fatalf("CA-648: firma como hoy, con su linea de aprobado:\n%s", s)
	}
	if !aetTieneLinea(s.out, aetAviso(slug)) {
		t.Fatalf("CA-648: con auto agrega la linea %q:\n%s", aetAviso(slug), s)
	}
	if strings.Count(s.out+s.err, "piloto automatico") != 1 {
		t.Fatalf("CA-648: aprobar no imprime lineas del piloto (no decide): solo el aviso:\n%s", s)
	}
	if len(vdtAprobaciones(t, wt)) != 1 {
		t.Fatalf("CA-648: la firma queda en el espacio de trabajo: %v", vdtAprobaciones(t, wt))
	}
	vdtColumna(t, "CA-648", root, slug, boardcmd.ColTestWriter)
	aetNadaLanzado(t, "CA-648", "hoom aprobar", root, wt, head, f)
}

// CA-648: sin auto (con o sin presupuesto) no hay aviso; la salida es la de
// hoy y tampoco se lanza nada.
func TestCA648_AprobarSinAutoNoAvisa(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	for i, extra := range []string{aetSinAuto, ""} {
		slug := "firma-a-mano-" + itoa(i)
		wt := aetCarta(t, root, slug, extra)
		head := vdtGit(t, wt, "rev-parse", "HEAD")
		s := vdtCorrer(t, "CA-648", vdtOpts{dir: root}, "aprobar", slug, "--si")
		vdtExit(t, "CA-648", s, 0)
		if strings.Contains(s.out+s.err, "piloto automatico") || strings.Contains(s.out+s.err, "hoom piloto") {
			t.Fatalf("CA-648: sin auto: hasta-humano (%q) no hay aviso del piloto:\n%s", extra, s)
		}
		aetNadaLanzado(t, "CA-648", "hoom aprobar sin auto", root, wt, head, f)
	}
}

// CA-648 y caso limite "el item cambiado con hoom item set sin guardar": el
// aviso lee el item como el riesgo, del checkout principal cuando lo tiene.
// auto puesto en el principal sin guardar avisa; auto solo en la copia del
// worktree, con el item del principal sin auto, no.
func TestCA648_ElAvisoLeeElItemDelPrincipal(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")

	const enPrincipal = "auto-sin-guardar"
	wt := aetCarta(t, root, enPrincipal, aetSinAuto)
	head := vdtGit(t, wt, "rev-parse", "HEAD")
	vdtEscribir(t, root, ".hoom/items/"+enPrincipal+".yaml", "titulo: Tarjeta "+enPrincipal+"\ntipo: feature\nprioridad: media\n"+
		"pedido: el pedido de "+enPrincipal+"\ncreado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n"+aetConAuto)
	s := vdtCorrer(t, "CA-648", vdtOpts{dir: root}, "aprobar", enPrincipal, "--si")
	vdtExit(t, "CA-648", s, 0)
	if !aetTieneLinea(s.out, aetAviso(enPrincipal)) {
		t.Fatalf("CA-648: auto en el item del principal, sin guardar, cuenta para el aviso:\n%s", s)
	}
	aetNadaLanzado(t, "CA-648", "hoom aprobar", root, wt, head, f)

	const enTarea = "auto-solo-en-la-tarea"
	wt2 := aetCarta(t, root, enTarea, aetSinAuto)
	vdtEscribir(t, wt2, ".hoom/items/"+enTarea+".yaml", "titulo: Tarjeta "+enTarea+"\ntipo: feature\nprioridad: media\n"+
		"pedido: el pedido de "+enTarea+"\ncreado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n"+aetConAuto)
	vdtCommit(t, wt2, "auto solo en la copia de la tarea")
	head2 := vdtGit(t, wt2, "rev-parse", "HEAD")
	s = vdtCorrer(t, "CA-648", vdtOpts{dir: root}, "aprobar", enTarea, "--si")
	vdtExit(t, "CA-648", s, 0)
	if strings.Contains(s.out+s.err, "piloto automatico") {
		t.Fatalf("CA-648: el item del principal (sin auto) manda sobre la copia de la tarea: no hay aviso:\n%s", s)
	}
	aetNadaLanzado(t, "CA-648", "hoom aprobar", root, wt2, head2, f)
}

// CA-648: desde el espacio de trabajo de la tarjeta, sin pasar el slug,
// tambien avisa (la tarjeta es la deducida) y no lanza nada.
func TestCA648_AprobarDesdeElWorktreeAvisa(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	const slug = "firma-desde-adentro"
	wt := aetCarta(t, root, slug, aetConAuto)
	head := vdtGit(t, wt, "rev-parse", "HEAD")
	s := vdtCorrer(t, "CA-648", vdtOpts{dir: wt}, "aprobar", "--si")
	vdtExit(t, "CA-648", s, 0)
	if !aetTieneLinea(s.out, aetAviso(slug)) {
		t.Fatalf("CA-648: desde el worktree tambien avisa %q:\n%s", aetAviso(slug), s)
	}
	aetNadaLanzado(t, "CA-648", "hoom aprobar desde el worktree", root, wt, head, f)
}
