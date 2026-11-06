// Tests adversariales de hoom siguiente (CA-507), hoom aprobar (CA-508),
// hoom integrar (CA-509), hoom guardar (CA-510) y hoom reanudar (CA-511),
// del spec verbos-de-la-tarjeta.
package tarjetacmd_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/itemcmd"
)

// ===========================================================================
// CA-507: hoom siguiente
// ===========================================================================

// CA-507: con la accion principal de rol habilitada, es hoom pedir <rol>.
func TestCA507_RolHabilitadoEsPedir(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-507", root, "precios", "")
	s := vdtCorrer(t, "CA-507", vdtOpts{dir: root}, "siguiente", "precios", "-m", "Implementa el spec", "--effort", "high")
	if got := vdtPrimeraLinea(s.out); got != "tarjeta: precios (argumento)" {
		t.Fatalf("CA-507: primero la tarjeta: %q\n%s", got, s)
	}
	want := `→ hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude --effort high "Implementa el spec"`
	if got := vdtFlechaDe(t, "CA-507", s.out); got != want {
		t.Fatalf("CA-507: siguiente lanza lo mismo que hoom pedir writer:\n quiero %q\n fue    %q", want, got)
	}
	args := vdtUnaLlamada(t, "CA-507", "siguiente", f, "claude")
	if v, _ := vdtSigue(args, "--effort"); v != "high" {
		t.Fatalf("CA-507: los flags de lanzamiento llegan al rol: %q", args)
	}
}

// CA-507: en Tu aprobacion y Tu aceptacion no firma: dice el comando y el
// plain, exit 0.
func TestCA507_NuncaApruebaNiIntegra(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	wtA := vdtTuAprobacion(t, "CA-507", root, "por-aprobar", "- CA-1: uno.\n- CA-2: dos.")
	vdtTuAceptacion(t, "CA-507", root, "por-integrar")
	for _, c := range []struct{ slug, verbo string }{{"por-aprobar", "aprobar"}, {"por-integrar", "integrar"}} {
		card := vdtCarta(t, root, c.slug)
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "CA-507", vdtOpts{dir: root, terminal: true, in: "s\n"}, "siguiente", c.slug)
		vdtExit(t, "CA-507 ("+c.slug+")", s, 0)
		vdtDiceOut(t, "CA-507", s, "te toca: hoom "+c.verbo)
		vdtDiceOut(t, "CA-507", s, card.Plain)
		vdtSinEfectos(t, "CA-507", "siguiente en "+card.ColumnName, antes, vdtFoto(t, root))
	}
	if st, _, _ := approval.Status(wtA, filepath.Join(wtA, ".hoom", "specs", "por-aprobar.md")); st == approval.StatusApproved {
		t.Fatalf("CA-507: siguiente nunca aprueba")
	}
	if it, _ := item.Load(root, "por-integrar"); it.HechoEn != nil {
		t.Fatalf("CA-507: siguiente nunca integra")
	}
	f.ninguna(t, "CA-507", "siguiente en una columna humana")
}

// CA-507: con un rol trabajando, exit 1 con el texto del contrato.
func TestCA507_RolTrabajando(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtEnCurso(t, "CA-507", root, "precios")
	s := vdtCorrer(t, "CA-507", vdtOpts{dir: root}, "siguiente", "precios", "-m", "Implementa")
	vdtExit(t, "CA-507", s, 1)
	vdtDiceErr(t, "CA-507", s, "el writer esta trabajando (run, paso 3 de 5): espera a que termine")
	f.ninguna(t, "CA-507", "siguiente con un rol trabajando")
}

// CA-507: interrumpida, no elige: dice hoom reanudar y hoom pedir <rol>,
// exit 0.
func TestCA507_InterrumpidaNoElige(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtInterrumpida(t, "CA-507", root, "precios", true)
	antes := vdtFoto(t, root)
	s := vdtCorrer(t, "CA-507", vdtOpts{dir: root}, "siguiente", "precios")
	vdtExit(t, "CA-507", s, 0)
	if !strings.Contains(s.out, "hoom reanudar") || !strings.Contains(s.out, "hoom pedir writer") {
		t.Fatalf("CA-507: interrumpida dice hoom reanudar y hoom pedir writer:\n%s", s)
	}
	vdtSinEfectos(t, "CA-507", "siguiente interrumpida", antes, vdtFoto(t, root))
	f.ninguna(t, "CA-507", "siguiente interrumpida")
}

// CA-507 y caso limite: la accion principal deshabilitada imprime su why y,
// si guardar esta habilitada, hoom guardar; exit 1.
func TestCA507_PrincipalDeshabilitada(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")

	// Test-writer con el spec sin guardar: why y hoom guardar
	wt := vdtTestWriter(t, "CA-507", root, "precios", "")
	vdtEscribir(t, wt, vdtSpecRel("precios"), vdtSpecTexto("- CA-902: todavia nadie lo cita, otra version."))
	vdtAprobar(t, wt, "precios")
	c := vdtColumna(t, "CA-507", root, "precios", boardcmd.ColTestWriter)
	if g := vdtAccion(t, "CA-507", c, boardcmd.ActGuardar); !g.Enabled {
		t.Fatalf("CA-507: fixture: guardar habilitada con el spec sin guardar: %+v", g)
	}
	s := vdtCorrer(t, "CA-507", vdtOpts{dir: root}, "siguiente", "precios", "-m", "Escribi los tests")
	vdtExit(t, "CA-507", s, 1)
	vdtDiceErr(t, "CA-507", s, boardcmd.WhySpecSinGuardar)
	if !strings.Contains(s.out+s.err, "hoom guardar") {
		t.Fatalf("CA-507: con guardar habilitada sugiere hoom guardar:\n%s", s)
	}

	// Writer con el presupuesto agotado y nada sin guardar: why, sin guardar
	wt2 := vdtWriter(t, "CA-507", root, "agotada", "presupuesto_usd: 1\n")
	vdtSidecar(t, root, vdtGastoMeta("agotada", wt2, 2))
	c = vdtCarta(t, root, "agotada")
	a := vdtAccion(t, "CA-507", c, boardcmd.ActPedirWriter)
	for _, g := range c.Actions {
		if g.ID == boardcmd.ActGuardar && g.Enabled {
			t.Fatalf("CA-507: fixture: nada sin guardar en agotada: %+v", g)
		}
	}
	s = vdtCorrer(t, "CA-507", vdtOpts{dir: root}, "siguiente", "agotada", "-m", "Implementa")
	vdtExit(t, "CA-507", s, 1)
	vdtDiceErr(t, "CA-507", s, a.Why)
	if strings.Contains(s.out+s.err, "hoom guardar") {
		t.Fatalf("CA-507: sin guardar habilitada no sugiere hoom guardar:\n%s", s)
	}
	f.ninguna(t, "CA-507", "siguiente con la principal deshabilitada")
}

// CA-507: en Hecho, exit 0 con el texto del contrato.
func TestCA507_Hecho(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtHecho(t, "CA-507", root, "precios")
	s := vdtCorrer(t, "CA-507", vdtOpts{dir: root}, "siguiente", "precios")
	vdtExit(t, "CA-507", s, 0)
	vdtDiceOut(t, "CA-507", s, "la tarjeta ya esta en Hecho")
	f.ninguna(t, "CA-507", "siguiente en Hecho")
}

// CA-507: un flag de lanzamiento cuando la accion principal no lanza un rol
// es exit 2 con el flag como se escribio, sin efectos.
func TestCA507_FlagsDeLanzamientoSinRol(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtTuAprobacion(t, "CA-507", root, "por-aprobar", "- CA-1: uno.")
	vdtTuAceptacion(t, "CA-507", root, "por-integrar")
	casos := []struct {
		slug, verbo string
		flag        []string
	}{
		{"por-aprobar", "aprobar", []string{"--model", "opus"}},
		{"por-aprobar", "aprobar", []string{"-m", "aprueba"}},
		{"por-aprobar", "aprobar", []string{"--effort", "high"}},
		{"por-aprobar", "aprobar", []string{"--budget-usd", "2"}},
		{"por-aprobar", "aprobar", []string{"--timeout", "45m"}},
		{"por-aprobar", "aprobar", []string{"--provider", "claude"}},
		{"por-integrar", "integrar", []string{"--model", "opus"}},
		{"por-integrar", "integrar", []string{"-m", "integra"}},
	}
	for _, c := range casos {
		antes := vdtFoto(t, root)
		args := append([]string{c.slug}, c.flag...)
		s := vdtCorrer(t, "CA-507", vdtOpts{dir: root}, "siguiente", args...)
		vdtExit(t, "CA-507 (siguiente "+strings.Join(args, " ")+")", s, 2)
		want := c.flag[0] + " solo sirve cuando hoom siguiente lanza un rol; ahora te toca hoom " + c.verbo
		if !strings.Contains(s.err, want) {
			t.Fatalf("CA-507: siguiente %q dice %q:\n%s", args, want, s)
		}
		vdtSinEfectos(t, "CA-507", "un flag de lanzamiento sin rol", antes, vdtFoto(t, root))
	}
	f.ninguna(t, "CA-507", "un flag de lanzamiento sin rol")
}

// ===========================================================================
// CA-508: hoom aprobar
// ===========================================================================

func vdtTextoAprobar(titulo string, n int, firma string) string {
	cr := "criterios"
	if n == 1 {
		cr = "criterio"
	}
	return "Vas a aprobar el spec de \"" + titulo + "\" (" + itoa(n) + " " + cr + ") con tu identidad: " + firma +
		". Si el spec cambia despues, la aprobacion deja de valer."
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func vdtAprobaciones(t *testing.T, dir string) []string {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(dir, ".hoom", "approvals"))
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

var vdtAprobadoRe = regexp.MustCompile(`aprobado: \.hoom/specs/([a-z0-9-]+)\.md \(sha256 ([0-9a-f]{8})\)`)

// CA-508: con --si aprueba en el espacio de trabajo de la tarjeta, con la
// identidad de su arbol, e imprime el contrato.
func TestCA508_AprobarConSi(t *testing.T) {
	root := vdtProyecto(t, "")
	wt := vdtTuAprobacion(t, "CA-508", root, "precios", "- CA-1: uno.\n- CA-2: dos.")
	vdtGit(t, root, "config", "extensions.worktreeConfig", "true")
	vdtGit(t, wt, "config", "--worktree", "user.name", "Persona del Worktree")
	vdtGit(t, wt, "config", "--worktree", "user.email", "wt@hoom.dev")
	firma := gitx.Identity(wt)

	s := vdtCorrer(t, "CA-508", vdtOpts{dir: root}, "aprobar", "precios", "--si")
	vdtExit(t, "CA-508", s, 0)
	m := vdtAprobadoRe.FindStringSubmatch(s.out)
	if m == nil || m[1] != "precios" {
		t.Fatalf("CA-508: imprime `aprobado: .hoom/specs/precios.md (sha256 <8>)`:\n%s", s)
	}
	ap := vdtAprobaciones(t, wt)
	if len(ap) != 1 || len(vdtAprobaciones(t, root)) != 0 {
		t.Fatalf("CA-508: la aprobacion vive en el espacio de trabajo, no en el proyecto: wt=%v raiz=%v", ap, vdtAprobaciones(t, root))
	}
	raw, err := os.ReadFile(filepath.Join(wt, ".hoom", "approvals", ap[0]))
	if err != nil {
		t.Fatal(err)
	}
	var rec approval.Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rec.SHA256, m[2]) || rec.ApprovedBy != firma {
		t.Fatalf("CA-508: el sha256 impreso es el de la aprobacion (%s) y firma la identidad del arbol (%s): %+v", m[2], firma, rec)
	}
	vdtColumna(t, "CA-508", root, "precios", boardcmd.ColTestWriter)
}

// CA-508: con terminal muestra el texto del dialogo del Studio y pregunta;
// solo s o si (sin mayusculas ni blancos que importen) siguen.
func TestCA508_AprobarPregunta(t *testing.T) {
	root := vdtProyecto(t, "")
	si := []string{"s\n", "si\n", "S\n", " SI \n", "Si"}
	no := []string{"n\n", "\n", "", "sí\n", "yes\n", "no\n", "s i\n", "ss\n"}
	i := 0
	nuevo := func(criterios string) (string, string) {
		i++
		slug := "tarjeta-" + itoa(i)
		return slug, vdtTuAprobacion(t, "CA-508", root, slug, criterios)
	}
	for _, r := range si {
		slug, wt := nuevo("- CA-1: uno.\n- CA-2: dos.")
		firma := vdtAccion(t, "CA-508", vdtCarta(t, root, slug), boardcmd.ActAprobar).Signer
		s := vdtCorrer(t, "CA-508", vdtOpts{dir: root, terminal: true, in: r}, "aprobar", slug)
		vdtExit(t, "CA-508 (respuesta "+r+")", s, 0)
		vdtDiceOut(t, "CA-508", s, vdtTextoAprobar("Tarjeta "+slug, 2, firma))
		vdtDiceOut(t, "CA-508", s, "¿Seguir? [s/N] ")
		if len(vdtAprobaciones(t, wt)) != 1 {
			t.Fatalf("CA-508: la respuesta %q aprueba:\n%s", r, s)
		}
	}
	for _, r := range no {
		slug, _ := nuevo("- CA-1: uno.")
		firma := vdtAccion(t, "CA-508", vdtCarta(t, root, slug), boardcmd.ActAprobar).Signer
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "CA-508", vdtOpts{dir: root, terminal: true, in: r}, "aprobar", slug)
		vdtExit(t, "CA-508 (respuesta "+r+")", s, 1)
		vdtDiceOut(t, "CA-508", s, vdtTextoAprobar("Tarjeta "+slug, 1, firma))
		vdtDiceErr(t, "CA-508", s, "no se firmo nada")
		vdtSinEfectos(t, "CA-508", "la respuesta "+r, antes, vdtFoto(t, root))
	}
}

// CA-508: sin terminal y sin --si es exit 2 sin efectos; fuera de Tu
// aprobacion es exit 1 con el texto del contrato, y eso se mira antes que
// --si.
func TestCA508_AprobarSinTerminalYFueraDeSuColumna(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtTuAprobacion(t, "CA-508", root, "precios", "- CA-1: uno.")
	antes := vdtFoto(t, root)
	s := vdtCorrer(t, "CA-508", vdtOpts{dir: root, in: "s\n"}, "aprobar", "precios")
	vdtExit(t, "CA-508", s, 2)
	if !strings.Contains(s.err, "sin terminal, aprobar necesita --si") {
		t.Fatalf("CA-508: el texto del contrato:\n%s", s)
	}
	vdtSinEfectos(t, "CA-508", "aprobar sin terminal", antes, vdtFoto(t, root))

	vdtWriter(t, "CA-508", root, "en-writer", "")
	c := vdtCarta(t, root, "en-writer")
	for _, args := range [][]string{{"en-writer"}, {"en-writer", "--si"}} {
		antes = vdtFoto(t, root)
		s = vdtCorrer(t, "CA-508", vdtOpts{dir: root}, "aprobar", args...)
		vdtExit(t, "CA-508 (aprobar "+strings.Join(args, " ")+")", s, 1)
		vdtDiceErr(t, "CA-508", s, "en "+c.ColumnName+" no hay nada que aprobar: "+c.Plain)
		vdtSinEfectos(t, "CA-508", "aprobar fuera de su columna", antes, vdtFoto(t, root))
	}
}

// ===========================================================================
// CA-509: hoom integrar
// ===========================================================================

const vdtTextoIntegrar = "Se cierra el trabajo de \"Tarjeta %s\": se comprueba una ultima vez que todo este verde y guardado, y la tarjeta pasa a Hecho. Juntarlo con el proyecto lo haces vos despues."

func vdtIntegrarTexto(slug string) string { return strings.Replace(vdtTextoIntegrar, "%s", slug, 1) }

func vdtPunta(t *testing.T, root, slug string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-parse", "hoom/"+slug).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// CA-509: integrar es hoom task done: la tarjeta pasa a Hecho con hecho_en y
// commit_final, e imprime el contrato.
func TestCA509_IntegrarConSi(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtTuAceptacion(t, "CA-509", root, "precios")
	punta := vdtPunta(t, root, "precios")
	s := vdtCorrer(t, "CA-509", vdtOpts{dir: root}, "integrar", "precios", "--si")
	vdtExit(t, "CA-509", s, 0)
	vdtDiceOut(t, "CA-509", s, "listo: la tarjeta pasa a Hecho")
	vdtDiceOut(t, "CA-509", s, "para juntarla: git merge --no-ff hoom/precios")
	it, err := item.Load(root, "precios")
	if err != nil || it.HechoEn == nil || it.CommitFinal != punta {
		t.Fatalf("CA-509: el item gana hecho_en y commit_final = %s: %+v %v", punta, it, err)
	}
	vdtColumna(t, "CA-509", root, "precios", boardcmd.ColHecho)
}

// CA-509: la misma confirmacion que aprobar, con su texto.
func TestCA509_IntegrarPregunta(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtTuAceptacion(t, "CA-509", root, "no-va")
	vdtTuAceptacion(t, "CA-509", root, "si-va")

	antes := vdtFoto(t, root)
	s := vdtCorrer(t, "CA-509", vdtOpts{dir: root, terminal: true, in: "n\n"}, "integrar", "no-va")
	vdtExit(t, "CA-509", s, 1)
	vdtDiceOut(t, "CA-509", s, vdtIntegrarTexto("no-va"))
	vdtDiceOut(t, "CA-509", s, "¿Seguir? [s/N] ")
	vdtDiceErr(t, "CA-509", s, "no se firmo nada")
	vdtSinEfectos(t, "CA-509", "integrar con n", antes, vdtFoto(t, root))

	s = vdtCorrer(t, "CA-509", vdtOpts{dir: root, terminal: true, in: "si\n"}, "integrar", "si-va")
	vdtExit(t, "CA-509", s, 0)
	vdtDiceOut(t, "CA-509", s, vdtIntegrarTexto("si-va"))
	vdtDiceOut(t, "CA-509", s, "para juntarla: git merge --no-ff hoom/si-va")
	vdtColumna(t, "CA-509", root, "si-va", boardcmd.ColHecho)
}

// CA-509: sin terminal y sin --si es exit 2; fuera de Tu aceptacion, exit 1
// con el texto del contrato.
func TestCA509_IntegrarSinTerminalYFueraDeSuColumna(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtTuAceptacion(t, "CA-509", root, "precios")
	antes := vdtFoto(t, root)
	s := vdtCorrer(t, "CA-509", vdtOpts{dir: root}, "integrar", "precios")
	vdtExit(t, "CA-509", s, 2)
	if !strings.Contains(s.err, "sin terminal, integrar necesita --si") {
		t.Fatalf("CA-509: el texto del contrato:\n%s", s)
	}
	vdtSinEfectos(t, "CA-509", "integrar sin terminal", antes, vdtFoto(t, root))

	vdtWriter(t, "CA-509", root, "en-writer", "")
	c := vdtCarta(t, root, "en-writer")
	antes = vdtFoto(t, root)
	s = vdtCorrer(t, "CA-509", vdtOpts{dir: root}, "integrar", "en-writer", "--si")
	vdtExit(t, "CA-509", s, 1)
	vdtDiceErr(t, "CA-509", s, "en "+c.ColumnName+" no hay nada que integrar: "+c.Plain)
	vdtSinEfectos(t, "CA-509", "integrar fuera de su columna", antes, vdtFoto(t, root))
}

// ===========================================================================
// CA-510: hoom guardar
// ===========================================================================

// CA-510: commitea las rutas sin guardar con el mensaje de hoom item save,
// las imprime, y sin nada que guardar dice el contrato con exit 0.
func TestCA510_Guardar(t *testing.T) {
	root := vdtProyecto(t, "")
	wt := vdtWriter(t, "CA-510", root, "precios", "")
	vdtEscribir(t, wt, "extra.go", "package app\n\nvar Extra = 1\n")
	vdtEscribir(t, wt, "web/precios.js", "export const p = 1\n")
	a := vdtAccion(t, "CA-510", vdtCarta(t, root, "precios"), boardcmd.ActGuardar)
	if len(a.Paths) == 0 {
		t.Fatalf("CA-510: fixture: guardar tiene rutas: %+v", a)
	}
	s := vdtCorrer(t, "CA-510", vdtOpts{dir: wt}, "guardar")
	vdtExit(t, "CA-510", s, 0)
	for _, p := range a.Paths {
		vdtDiceOut(t, "CA-510", s, p)
	}
	if got := vdtGit(t, wt, "log", "-1", "--format=%s"); got != itemcmd.SaveMessage("precios") {
		t.Fatalf("CA-510: el mensaje es el de hoom item save (%q): %q", itemcmd.SaveMessage("precios"), got)
	}
	if st := vdtGit(t, wt, "status", "--porcelain"); st != "" {
		t.Fatalf("CA-510: no queda nada sin guardar en el espacio: %q", st)
	}
	s = vdtCorrer(t, "CA-510", vdtOpts{dir: wt}, "guardar")
	vdtExit(t, "CA-510", s, 0)
	vdtDiceOut(t, "CA-510", s, "no hay nada sin guardar")
}

// ===========================================================================
// CA-511: hoom reanudar
// ===========================================================================

// CA-511: retoma la sesion del sobre interrumpido con su provider (aunque
// hoom.yaml declare otro), y la → lleva --resume.
func TestCA511_ReanudarConSuProvider(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	f.cli(t, "codex", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    provider: codex\n")
	vdtInterrumpida(t, "CA-511", root, "precios", true)
	s := vdtCorrer(t, "CA-511", vdtOpts{dir: root}, "reanudar", "precios", "-m", "Termina lo que quedo")
	args := vdtUnaLlamada(t, "CA-511", "reanudar", f, "claude")
	if v, ok := vdtSigue(args, "--resume"); !ok || v != vdtSesion {
		t.Fatalf("CA-511: reanudar retoma la sesion (--resume %s): %q\n%s", vdtSesion, args, s)
	}
	if len(f.llamadas(t, "codex")) != 0 {
		t.Fatalf("CA-511: reanudar no elige otro provider")
	}
	fl := vdtFlechaDe(t, "CA-511", s.out)
	if !strings.HasPrefix(fl, "→ hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude") ||
		!strings.Contains(fl, " --resume "+vdtSesion) || !strings.HasSuffix(fl, `"Termina lo que quedo"`) {
		t.Fatalf("CA-511 y CA-503: la → de reanudar lleva --resume: %q", fl)
	}
}

// CA-511: sin -m y con terminal el pedido propuesto pasa por el editor.
func TestCA511_ReanudarPasaPorElEditor(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtInterrumpida(t, "CA-511", root, "precios", true)
	propuesto := vdtAccion(t, "CA-511", vdtCarta(t, root, "precios"), boardcmd.ActReanudar).Pedido
	ed, capt := vdtEditor(t, "editor", "Segui desde el paso 3", 0)
	s := vdtCorrer(t, "CA-511", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed}}, "reanudar", "precios")
	pl := vdtPlantilla(t, capt)
	if !strings.Contains(pl, propuesto) || !strings.Contains(propuesto, "El trabajo anterior se corto en el paso") {
		t.Fatalf("CA-511: la plantilla trae el pedido propuesto de reanudar %q:\n%s", propuesto, pl)
	}
	if p := f.pedidos(t, "claude"); len(p) != 1 || !strings.Contains(p[0], "Segui desde el paso 3") {
		t.Fatalf("CA-511: el pedido sale del editor: %q\n%s", p, s)
	}
}

// CA-511: sin la accion, exit 1 con el texto del contrato; con la accion
// deshabilitada, exit 1 con su why.
func TestCA511_ReanudarSinAccionODeshabilitada(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-511", root, "en-writer", "")
	c := vdtCarta(t, root, "en-writer")
	s := vdtCorrer(t, "CA-511", vdtOpts{dir: root}, "reanudar", "en-writer", "-m", "sigue")
	vdtExit(t, "CA-511", s, 1)
	vdtDiceErr(t, "CA-511", s, "en "+c.ColumnName+" no hay nada que reanudar: "+c.Plain)

	vdtInterrumpida(t, "CA-511", root, "sin-sesion", false)
	a := vdtAccion(t, "CA-511", vdtCarta(t, root, "sin-sesion"), boardcmd.ActReanudar)
	if a.Enabled || a.Why == "" {
		t.Fatalf("CA-511: fixture: sin id de sesion reanudar esta deshabilitada: %+v", a)
	}
	antes := vdtFoto(t, root)
	s = vdtCorrer(t, "CA-511", vdtOpts{dir: root}, "reanudar", "sin-sesion", "-m", "sigue")
	vdtExit(t, "CA-511", s, 1)
	vdtDiceErr(t, "CA-511", s, a.Why)
	vdtSinEfectos(t, "CA-511", "reanudar deshabilitada", antes, vdtFoto(t, root))
	f.ninguna(t, "CA-511", "reanudar sin accion o deshabilitada")
}
