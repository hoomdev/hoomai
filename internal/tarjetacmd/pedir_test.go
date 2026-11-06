// Tests adversariales de `hoom pedir` (spec verbos-de-la-tarjeta): los roles
// (CA-501), el pedido y el editor (CA-502), las lineas valores: y → (CA-503),
// el lanzamiento compartido visto desde la terminal (CA-504), el candado
// (CA-505) y el exit code y --json (CA-506).
package tarjetacmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

func vdtUso(costo float64) *providers.Usage { return &providers.Usage{CostUSD: &costo, Turns: 3} }

// vdtGastoMeta es un run terminado del writer de la tarjeta que costo costo.
func vdtGastoMeta(slug, wt string, costo float64) runcmd.Meta {
	return runcmd.Meta{ID: "20260923T060000_gasto" + slug, Provider: "claude", Role: "writer", Task: slug, Dir: wt,
		CreatedAt: time.Now().UTC().Add(-time.Hour), Status: runcmd.StatusDone, EndedAt: time.Now().UTC().Add(-50 * time.Minute),
		Usage: vdtUso(costo)}
}

// vdtNarra es lo que el CLI falso narra por stderr: la narracion del
// provider.
const vdtNarra = "echo NARRACION-UNICA-DEL-CLI >&2"

// vdtValoresDe y vdtFlechaDe devuelven las lineas `valores:` y `→ ` de un
// texto.
func vdtValoresDe(t *testing.T, ca, texto string) string {
	t.Helper()
	l, i := vdtLineaQueEmpieza(texto, "valores: ")
	if i < 0 {
		t.Fatalf("%s: falta la linea `valores: `:\n%s", ca, texto)
	}
	return l
}

func vdtFlechaDe(t *testing.T, ca, texto string) string {
	t.Helper()
	l, i := vdtLineaQueEmpieza(texto, "→ ")
	if i < 0 {
		t.Fatalf("%s: falta la linea `→ `:\n%s", ca, texto)
	}
	return l
}

func vdtSobres(t *testing.T, root string) []envelope.Record {
	t.Helper()
	return envelope.List(root)
}

// ===========================================================================
// CA-501: los roles
// ===========================================================================

// CA-501: otro rol es exit 2 con el texto del contrato, sin efectos.
func TestCA501_RolDesconocidoEsExit2(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-501", root, "precios", "")
	for _, rol := range []string{"scout", "Writer", "orquestador", ""} {
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "CA-501", vdtOpts{dir: root}, "pedir", rol, "precios", "-m", "Implementa")
		vdtExit(t, "CA-501 (rol "+rol+")", s, 2)
		want := `rol desconocido "` + rol + `" (arquitecto, test-writer, writer, reviewer); para otros roles: hoom agent`
		if !strings.Contains(s.err, want) {
			t.Fatalf("CA-501: el rol %q dice %q:\n%s", rol, want, s)
		}
		vdtSinEfectos(t, "CA-501", "pedir "+rol, antes, vdtFoto(t, root))
	}
	f.ninguna(t, "CA-501", "un rol desconocido")
}

// CA-501: la columna sin esa accion es exit 1 con el texto del contrato.
func TestCA501_ColumnaSinLaAccion(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-501", root, "precios", "")
	c := vdtCarta(t, root, "precios")
	for _, rol := range []string{"arquitecto", "test-writer", "reviewer"} {
		antes := vdtFoto(t, root)
		args := []string{rol, "precios"}
		if rol != "reviewer" {
			args = append(args, "-m", "Haz algo concreto con el spec")
		}
		s := vdtCorrer(t, "CA-501", vdtOpts{dir: root}, "pedir", args...)
		vdtExit(t, "CA-501 (pedir "+rol+" en Writer)", s, 1)
		vdtDiceErr(t, "CA-501", s, "en "+c.ColumnName+" no se le pide al "+rol+": "+c.Plain+". Siguiente: hoom siguiente")
		vdtSinEfectos(t, "CA-501", "pedir "+rol+" en Writer", antes, vdtFoto(t, root))
	}
	f.ninguna(t, "CA-501", "una accion que la columna no tiene")
}

// CA-501 y CA-497: la accion deshabilitada es exit 1 con su why.
func TestCA501_AccionDeshabilitadaDiceSuWhy(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	wt := vdtTestWriter(t, "CA-501", root, "precios", "")
	// otra version del spec en disco, aprobada: lo guardado difiere del disco
	vdtEscribir(t, wt, vdtSpecRel("precios"), vdtSpecTexto("- CA-902: todavia nadie lo cita, otra version."))
	vdtAprobar(t, wt, "precios")
	c := vdtColumna(t, "CA-501", root, "precios", boardcmd.ColTestWriter)
	a := vdtAccion(t, "CA-501", c, boardcmd.ActPedirTestWriter)
	if a.Enabled || a.Why != boardcmd.WhySpecSinGuardar {
		t.Fatalf("CA-497: fixture: pedir-test-writer deshabilitada por el spec sin guardar: %+v", a)
	}
	antes := vdtFoto(t, root)
	s := vdtCorrer(t, "CA-501", vdtOpts{dir: root}, "pedir", "test-writer", "precios", "-m", "Escribi los tests")
	vdtExit(t, "CA-501", s, 1)
	vdtDiceErr(t, "CA-501", s, boardcmd.WhySpecSinGuardar)
	vdtSinEfectos(t, "CA-501", "una accion deshabilitada", antes, vdtFoto(t, root))
	f.ninguna(t, "CA-501", "una accion deshabilitada")
}

// CA-501 y CA-504: el presupuesto agotado deshabilita con el mismo why que
// el tablero.
func TestCA501_PresupuestoAgotado(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	wt := vdtWriter(t, "CA-501", root, "precios", "presupuesto_usd: 1\n")
	vdtSidecar(t, root, vdtGastoMeta("precios", wt, 2))
	a := vdtAccion(t, "CA-501", vdtCarta(t, root, "precios"), boardcmd.ActPedirWriter)
	if a.Enabled || a.Why == "" {
		t.Fatalf("CA-501: fixture: el presupuesto agotado deshabilita pedir-writer: %+v", a)
	}
	s := vdtCorrer(t, "CA-501", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa el spec")
	vdtExit(t, "CA-501", s, 1)
	vdtDiceErr(t, "CA-501", s, a.Why)
	f.ninguna(t, "CA-501", "el presupuesto agotado")
}

// ===========================================================================
// CA-502: el pedido y el editor
// ===========================================================================

// CA-502: con -m, ese texto; sin -m y sin terminal, el pedido propuesto tal
// cual, sin abrir el editor.
func TestCA502_PedidoDeMoPropuestoSinTerminal(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-502", root, "precios", "")
	ed, capt := vdtEditor(t, "editor", "lo del editor", 0)
	env := map[string]string{"EDITOR": ed, "VISUAL": ed}

	s := vdtCorrer(t, "CA-502", vdtOpts{dir: root, env: env}, "pedir", "writer", "precios", "-m", "Implementa el spec con cuidado")
	if p := f.pedidos(t, "claude"); len(p) != 1 || !strings.Contains(p[0], "Implementa el spec con cuidado") {
		t.Fatalf("CA-502: con -m el pedido es ese texto: %q\n%s", p, s)
	}
	if vdtEditorLlamado(capt) {
		t.Fatalf("CA-502: con -m no se abre el editor")
	}

	propuesto := vdtAccion(t, "CA-502", vdtCarta(t, root, "precios"), boardcmd.ActPedirWriter).Pedido
	f2 := vdtPATH(t)
	f2.cli(t, "claude", "")
	s = vdtCorrer(t, "CA-502", vdtOpts{dir: root, env: env, terminal: false}, "pedir", "writer", "precios")
	if p := f2.pedidos(t, "claude"); len(p) != 1 || !strings.Contains(p[0], propuesto) {
		t.Fatalf("CA-502: sin -m y sin terminal se usa el pedido propuesto %q: %q\n%s", propuesto, p, s)
	}
	if vdtEditorLlamado(capt) {
		t.Fatalf("CA-502: sin terminal no se abre el editor")
	}
}

// CA-502: sin -m y con terminal, $VISUAL gana a $EDITOR; la plantilla trae
// el pedido propuesto y las lineas # del contrato; se quitan las lineas que
// empiezan con #.
func TestCA502_EditorConPlantillaYSinComentarios(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-502", root, "precios", "")
	propuesto := vdtAccion(t, "CA-502", vdtCarta(t, root, "precios"), boardcmd.ActPedirWriter).Pedido
	visual, captV := vdtEditor(t, "visual", "Implementa el #3 del spec, sin tocar los tests\n# esto se borra\n#y esto tambien", 0)
	editor, captE := vdtEditor(t, "editor", "no soy yo", 0)

	s := vdtCorrer(t, "CA-502", vdtOpts{dir: root, terminal: true, env: map[string]string{"VISUAL": visual, "EDITOR": editor}},
		"pedir", "writer", "precios")
	if vdtEditorLlamado(captE) || !vdtEditorLlamado(captV) {
		t.Fatalf("CA-502: $VISUAL gana a $EDITOR:\n%s", s)
	}
	pl := vdtPlantilla(t, captV)
	if !strings.Contains(pl, propuesto) {
		t.Fatalf("CA-502: la plantilla trae el pedido propuesto %q:\n%s", propuesto, pl)
	}
	var comentarios []string
	for _, l := range strings.Split(pl, "\n") {
		if strings.HasPrefix(l, "#") {
			comentarios = append(comentarios, l)
		}
	}
	tiene := func(que string) bool {
		for _, l := range comentarios {
			if strings.Contains(l, que) {
				return true
			}
		}
		return false
	}
	if !tiene("Las lineas que empiezan con # se ignoran. Si lo dejas vacio no se lanza nada.") ||
		!tiene("precios") || !tiene("writer") || !tiene("claude") {
		t.Fatalf("CA-502: la plantilla trae lineas # con la tarjeta, el rol, los valores y el aviso del contrato:\n%s", pl)
	}
	p := f.pedidos(t, "claude")
	if len(p) != 1 || !strings.Contains(p[0], "Implementa el #3 del spec, sin tocar los tests") ||
		strings.Contains(p[0], "esto se borra") || strings.Contains(p[0], "y esto tambien") {
		t.Fatalf("CA-502: se quitan las lineas que empiezan con # y queda el resto: %q\n%s", p, s)
	}
}

// Caso limite y CA-502: $EDITOR con argumentos (code --wait) se ejecuta por
// el shell, con la ruta del archivo entre comillas aunque tenga espacios; sin
// $VISUAL ni $EDITOR, vi.
func TestCA502_EditorConArgumentosYVi(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-502", root, "precios", "")
	tmp := filepath.Join(t.TempDir(), "con espacio")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	ed, capt := vdtEditor(t, "code", "Implementa desde code", 0)
	s := vdtCorrer(t, "CA-502", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed + " --wait"}},
		"pedir", "writer", "precios")
	args := vdtEditorArgs(t, capt)
	if len(args) != 2 || args[0] != "--wait" {
		t.Fatalf("CA-502: el editor recibe sus argumentos y la ruta como un solo argumento: %q\n%s", args, s)
	}
	vdtPlantilla(t, capt)
	if p := f.pedidos(t, "claude"); len(p) != 1 || !strings.Contains(p[0], "Implementa desde code") {
		t.Fatalf("CA-502: el pedido sale del editor: %q\n%s", p, s)
	}

	// sin $VISUAL ni $EDITOR: vi (el del PATH)
	f2 := vdtPATH(t)
	f2.cli(t, "claude", "")
	vi, captVi := vdtEditor(t, "vi", "Implementa desde vi", 0)
	raw, err := os.ReadFile(vi)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f2.bin, "vi"), raw, 0o755); err != nil {
		t.Fatal(err)
	}
	s = vdtCorrer(t, "CA-502", vdtOpts{dir: root, terminal: true, env: map[string]string{}}, "pedir", "writer", "precios")
	if !vdtEditorLlamado(captVi) {
		t.Fatalf("CA-502: sin $VISUAL ni $EDITOR se abre vi:\n%s", s)
	}
	if p := f2.pedidos(t, "claude"); len(p) != 1 || !strings.Contains(p[0], "Implementa desde vi") {
		t.Fatalf("CA-502: el pedido sale de vi: %q\n%s", p, s)
	}
}

// CA-502: un pedido vacio (solo comentarios y blancos, o -m "") es exit 1
// sin lanzar; un editor que falla tambien.
func TestCA502_PedidoVacioYEditorQueFalla(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-502", root, "precios", "")

	vacio, _ := vdtEditor(t, "vacio", "\n   \n# Implementa todo\n\n", 0)
	antes := vdtFoto(t, root)
	s := vdtCorrer(t, "CA-502", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": vacio}}, "pedir", "writer", "precios")
	vdtExit(t, "CA-502 (editor vacio)", s, 1)
	vdtDiceErr(t, "CA-502", s, "pedido vacio: no se lanzo nada")
	vdtSinEfectos(t, "CA-502", "un pedido vacio", antes, vdtFoto(t, root))

	falla, _ := vdtEditor(t, "falla", "Implementa algo", 3)
	s = vdtCorrer(t, "CA-502", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": falla}}, "pedir", "writer", "precios")
	vdtExit(t, "CA-502 (editor que falla)", s, 1)
	vdtDiceErr(t, "CA-502", s, "el editor salio con error: no se lanzo nada")
	vdtSinEfectos(t, "CA-502", "un editor que falla", antes, vdtFoto(t, root))

	ed, capt := vdtEditor(t, "editor", "Implementa algo", 0)
	for _, terminal := range []bool{true, false} {
		s = vdtCorrer(t, "CA-502", vdtOpts{dir: root, terminal: terminal, env: map[string]string{"EDITOR": ed}},
			"pedir", "writer", "precios", "-m", "")
		vdtExit(t, "CA-502 (-m vacio)", s, 1)
		vdtDiceErr(t, "CA-502", s, "pedido vacio: no se lanzo nada")
		if vdtEditorLlamado(capt) {
			t.Fatalf("CA-502: con -m \"\" no se abre el editor")
		}
	}
	s = vdtCorrer(t, "CA-502", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "   \n  ")
	vdtExit(t, "CA-502 (-m en blanco)", s, 1)
	vdtDiceErr(t, "CA-502", s, "pedido vacio: no se lanzo nada")
	vdtSinEfectos(t, "CA-502", "-m vacio", antes, vdtFoto(t, root))
	f.ninguna(t, "CA-502", "un pedido vacio")
}

// Caso limite: un pedido de relleno es exit 1 con el texto de hoy.
func TestCA502_PedidoDeRelleno(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-502", root, "precios", "")
	antes := vdtFoto(t, root)
	s := vdtCorrer(t, "CA-502", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "<pedido>")
	vdtExit(t, "CA-502", s, 1)
	vdtDiceErr(t, "CA-502", s, `el pedido "<pedido>" no dice nada: escribi lo que el rol tiene que hacer`)
	vdtSinEfectos(t, "CA-502", "un pedido de relleno", antes, vdtFoto(t, root))
	f.ninguna(t, "CA-502", "un pedido de relleno")
}

// CA-502: hoom pedir reviewer -m es exit 2; sin -m no se abre el editor.
func TestCA502_ReviewerNoLlevaPedido(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtReview(t, "CA-502", root, "precios")
	antes := vdtFoto(t, root)
	s := vdtCorrer(t, "CA-502", vdtOpts{dir: root}, "pedir", "reviewer", "precios", "-m", "revisa bien")
	vdtExit(t, "CA-502", s, 2)
	if !strings.Contains(s.err, "la revision arma su propio pedido: no lleva -m") {
		t.Fatalf("CA-502: el texto del contrato:\n%s", s)
	}
	vdtSinEfectos(t, "CA-502", "pedir reviewer -m", antes, vdtFoto(t, root))
	f.ninguna(t, "CA-502", "pedir reviewer -m")

	ed, capt := vdtEditor(t, "editor", "revisa", 0)
	s = vdtCorrer(t, "CA-502", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed}}, "pedir", "reviewer", "precios")
	if vdtEditorLlamado(capt) {
		t.Fatalf("CA-502: pedir reviewer no abre el editor:\n%s", s)
	}
	if len(f.llamadas(t, "claude")) == 0 {
		t.Fatalf("CA-502: pedir reviewer lanza la revision:\n%s", s)
	}
}

// ===========================================================================
// CA-503: las lineas valores: y →
// ===========================================================================

// CA-503: antes de lanzar, en este orden: tarjeta, valores (con el origen
// de cada valor) y → (el comando largo); despues la narracion. Los valores
// llegan al provider y al sidecar.
func TestCA503_ValoresYFlechaConHoomYaml(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", vdtNarra)
	root := vdtProyecto(t, "agents:\n  writer:\n    model: m-yaml\n    effort: high\n    timeout: 45m\n")
	vdtWriter(t, "CA-503", root, "precios", "presupuesto_usd: 5\n")
	s := vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa el spec")

	wantV := "valores: provider claude (tarjeta) · modelo m-yaml (hoom.yaml) · esfuerzo high (hoom.yaml) · presupuesto 5 (tarjeta) · tiempo limite 45m (hoom.yaml)"
	wantF := `→ hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude --model m-yaml --effort high --budget-usd 5 --timeout 45m "Implementa el spec"`
	if got := vdtValoresDe(t, "CA-503", s.out); got != wantV {
		t.Fatalf("CA-503: la linea de valores:\n quiero %q\n fue    %q", wantV, got)
	}
	if got := vdtFlechaDe(t, "CA-503", s.out); got != wantF {
		t.Fatalf("CA-503: la linea →:\n quiero %q\n fue    %q", wantF, got)
	}
	_, iT := vdtLineaQueEmpieza(s.out, "tarjeta: precios (argumento)")
	_, iV := vdtLineaQueEmpieza(s.out, "valores: ")
	_, iF := vdtLineaQueEmpieza(s.out, "→ ")
	iN := -1
	for i, l := range vdtLineas(s.out) {
		if strings.Contains(l, "NARRACION-UNICA-DEL-CLI") {
			iN = i
			break
		}
	}
	if !(iT == 0 && iV > iT && iF > iV && iN > iF) {
		t.Fatalf("CA-503 y CA-504: el orden es tarjeta, valores, →, narracion (%d %d %d %d):\n%s", iT, iV, iF, iN, s)
	}
	args := vdtUnaLlamada(t, "CA-503", "pedir writer", f, "claude")
	if v, _ := vdtSigue(args, "--model"); v != "m-yaml" {
		t.Fatalf("CA-503: el modelo de hoom.yaml llega al provider: %q", args)
	}
	if v, _ := vdtSigue(args, "--effort"); v != "high" {
		t.Fatalf("CA-503: el esfuerzo de hoom.yaml llega al provider: %q", args)
	}
	if v, _ := vdtSigue(args, "--max-budget-usd"); v != "5" {
		t.Fatalf("CA-503: el presupuesto llega al provider: %q", args)
	}
	metas := runcmd.Metas(root)
	if len(metas) != 1 || metas[0].TimeoutS != 2700 {
		t.Fatalf("CA-503 y CA-494: el run lleva el tiempo limite de hoom.yaml (timeout_s 2700): %+v", metas)
	}
}

// CA-503: lo pedido en el comando gana, con origen comando.
func TestCA503_ValoresDelComando(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    model: m-yaml\n    effort: high\n    timeout: 45m\n")
	vdtWriter(t, "CA-503", root, "precios", "presupuesto_usd: 5\n")
	s := vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "writer", "precios", "--provider", "claude", "--model", "m-cmd",
		"--effort", "low", "--budget-usd", "2.5", "--timeout", "1h30m", "-m", "Implementa")
	wantV := "valores: provider claude (comando) · modelo m-cmd (comando) · esfuerzo low (comando) · presupuesto 2.5 (comando) · tiempo limite 1h30m (comando)"
	wantF := `→ hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude --model m-cmd --effort low --budget-usd 2.5 --timeout 1h30m "Implementa"`
	if got := vdtValoresDe(t, "CA-503", s.out); got != wantV {
		t.Fatalf("CA-503: la linea de valores:\n quiero %q\n fue    %q", wantV, got)
	}
	if got := vdtFlechaDe(t, "CA-503", s.out); got != wantF {
		t.Fatalf("CA-503: la linea →:\n quiero %q\n fue    %q", wantF, got)
	}
	args := vdtUnaLlamada(t, "CA-503", "pedir writer", f, "claude")
	if v, _ := vdtSigue(args, "--effort"); v != "low" {
		t.Fatalf("CA-496: el esfuerzo pedido llega al writer: %q", args)
	}
}

// CA-503: sin nada declarado, "del provider (provider)" y los 30 minutos de
// hoom; la → no lleva --model, --effort ni --timeout.
func TestCA503_SinNadaDeclarado(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-503", root, "precios", "")
	s := vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa")
	v := vdtValoresDe(t, "CA-503", s.out)
	if !strings.HasPrefix(v, "valores: provider claude (tarjeta) · modelo del provider (provider) · esfuerzo del provider (provider) · presupuesto ") ||
		!strings.HasSuffix(v, " · tiempo limite 30m (provider)") {
		t.Fatalf("CA-503: sin nada declarado, los del provider y 30m (provider): %q", v)
	}
	wantF := `→ hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude "Implementa"`
	if got := vdtFlechaDe(t, "CA-503", s.out); got != wantF {
		t.Fatalf("CA-503: la → sin valores que no existen:\n quiero %q\n fue    %q", wantF, got)
	}
}

// CA-503: budget_usd 0 sin presupuesto en la tarjeta es "sin tope
// (hoom.yaml)".
func TestCA503_BudgetCeroSinTope(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    budget_usd: 0\n")
	vdtWriter(t, "CA-503", root, "precios", "")
	s := vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa")
	if v := vdtValoresDe(t, "CA-503", s.out); !strings.Contains(v, " · presupuesto sin tope (hoom.yaml) · ") {
		t.Fatalf("CA-503: budget_usd 0 sin presupuesto en la tarjeta: %q", v)
	}
	if fl := vdtFlechaDe(t, "CA-503", s.out); strings.Contains(fl, "--budget-usd") {
		t.Fatalf("CA-503: sin tope no hay --budget-usd: %q", fl)
	}
	if args := vdtUnaLlamada(t, "CA-503", "pedir writer", f, "claude"); strings.Contains(strings.Join(args, " "), "--max-budget-usd") {
		t.Fatalf("CA-503: sin tope el provider no recibe tope: %q", args)
	}
}

// Caso limite: budget_usd de hoom.yaml con codex (no acepta tope) no se
// pasa y la linea lo dice; con --budget-usd explicito y codex, el error de
// hoy, sin lanzar.
func TestCA503_CodexNoAceptaTope(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	f.cli(t, "codex", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    provider: codex\n    budget_usd: 3\n")
	vdtWriter(t, "CA-503", root, "precios", "")
	s := vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa")
	v := vdtValoresDe(t, "CA-503", s.out)
	if !strings.HasPrefix(v, "valores: provider codex (hoom.yaml) · ") || !strings.Contains(v, " · presupuesto sin tope (codex no acepta tope) · ") {
		t.Fatalf("CA-503: codex de hoom.yaml y presupuesto sin tope (codex no acepta tope): %q", v)
	}
	if fl := vdtFlechaDe(t, "CA-503", s.out); !strings.Contains(fl, " --provider codex") || strings.Contains(fl, "--budget-usd") {
		t.Fatalf("CA-503: la → con codex y sin --budget-usd: %q", fl)
	}
	vdtUnaLlamada(t, "CA-503", "pedir writer con codex", f, "codex")
	if len(f.llamadas(t, "claude")) != 0 {
		t.Fatalf("CA-492: el provider de hoom.yaml (codex) gana a claude")
	}

	f2 := vdtPATH(t)
	f2.cli(t, "claude", "")
	f2.cli(t, "codex", "")
	s = vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "writer", "precios", "--provider", "codex", "--budget-usd", "2", "-m", "Implementa")
	if s.code == 0 {
		t.Fatalf("CA-503: --budget-usd explicito con codex es el error de hoy:\n%s", s)
	}
	f2.ninguna(t, "CA-503", "--budget-usd con codex")
}

// CA-503: el pedido de la → va en una linea y recortado a 60 caracteres (no
// bytes) con …; el pedido que recibe el provider va entero.
func TestCA503_PedidoRecortadoA60(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-503", root, "precios", "")
	base := `→ hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude `
	casos := []struct {
		pedido, enLinea string
	}{
		{strings.Repeat("a", 60), `"` + strings.Repeat("a", 60) + `"`},
		{strings.Repeat("a", 61), `"` + strings.Repeat("a", 60) + `…"`},
		{strings.Repeat("ñ", 70), `"` + strings.Repeat("ñ", 60) + `…"`},
		{"linea uno\nlinea dos", `"linea uno linea dos"`},
	}
	for _, c := range casos {
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		s := vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", c.pedido)
		if got := vdtFlechaDe(t, "CA-503", s.out); got != base+c.enLinea {
			t.Fatalf("CA-503: la → del pedido %q:\n quiero %q\n fue    %q", c.pedido, base+c.enLinea, got)
		}
		if p := f.pedidos(t, "claude"); len(p) != 1 || !strings.Contains(p[0], c.pedido) {
			t.Fatalf("CA-503: el provider recibe el pedido entero %q: %q", c.pedido, p)
		}
	}

	// el propuesto, sin terminal, tambien se recorta en la linea
	propuesto := vdtAccion(t, "CA-503", vdtCarta(t, root, "precios"), boardcmd.ActPedirWriter).Pedido
	if len([]rune(propuesto)) <= 60 {
		t.Fatalf("CA-503: fixture: el pedido propuesto del writer pasa de 60 caracteres: %q", propuesto)
	}
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	s := vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "writer", "precios")
	if got, want := vdtFlechaDe(t, "CA-503", s.out), base+`"`+string([]rune(propuesto)[:60])+`…"`; got != want {
		t.Fatalf("CA-503: la → del pedido propuesto:\n quiero %q\n fue    %q", want, got)
	}
}

// CA-503 y CA-504: el arquitecto no lleva --spec; en Backlog crea la tarea.
func TestCA503_ArquitectoEnBacklog(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtBacklog(t, "CA-503", root, "precios")
	s := vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "arquitecto", "precios", "-m", "Escribi el spec de precios")
	want := `→ hoom agent --role arquitecto --task precios --provider claude "Escribi el spec de precios"`
	if got := vdtFlechaDe(t, "CA-503", s.out); got != want {
		t.Fatalf("CA-503: la → del arquitecto no lleva --spec:\n quiero %q\n fue    %q", want, got)
	}
	if fi, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", "precios")); err != nil || !fi.IsDir() {
		t.Fatalf("CA-504: pedir al arquitecto en Backlog crea la tarea: %v\n%s", err, s)
	}
	vdtUnaLlamada(t, "CA-503", "pedir arquitecto", f, "claude")
}

// CA-503: la → del reviewer es hoom review, sin pedido ni --timeout.
func TestCA503_FlechaDelReviewer(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "review:\n  timeout: 60m\n")
	vdtReview(t, "CA-503", root, "precios")
	s := vdtCorrer(t, "CA-503", vdtOpts{dir: root}, "pedir", "reviewer", "precios")
	want := `→ hoom review --task precios --spec .hoom/specs/precios.md --provider claude`
	if got := vdtFlechaDe(t, "CA-503", s.out); got != want {
		t.Fatalf("CA-503: la → del reviewer:\n quiero %q\n fue    %q", want, got)
	}
	if v := vdtValoresDe(t, "CA-503", s.out); !strings.HasSuffix(v, " · tiempo limite 1h (hoom.yaml)") {
		t.Fatalf("CA-503: el tiempo limite de review: en la linea de valores: %q", v)
	}
}

// ===========================================================================
// CA-504 / CA-505: el lanzamiento compartido, desde la terminal
// ===========================================================================

// CA-504: la narracion va al log del sobre y, ademas, a la salida estandar.
func TestCA504_NarracionAlLogYALaSalida(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", vdtNarra)
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-504", root, "precios", "")
	s := vdtCorrer(t, "CA-504", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa")
	vdtDiceOut(t, "CA-504", s, "NARRACION-UNICA-DEL-CLI")
	sobres := vdtSobres(t, root)
	if len(sobres) != 1 {
		t.Fatalf("CA-504: un sobre: %+v", sobres)
	}
	log, err := os.ReadFile(filepath.Join(root, ".hoom", "envelopes", sobres[0].ID+".log"))
	if err != nil || !strings.Contains(string(log), "NARRACION-UNICA-DEL-CLI") {
		t.Fatalf("CA-504: el log del sobre tiene la narracion: %v\n%s", err, log)
	}
}

// CA-504: la cinta corre solo despues de un lanzamiento del Studio: un
// writer entregable lanzado desde la terminal sobre una tarjeta con auto no
// encadena al reviewer.
func TestCA504_LaCintaNoCorreDesdeLaTerminal(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	f.cli(t, "codex", `i=0
{
  printf 'package app\n\n'
  while [ $i -lt 450 ]; do printf 'var cinta%d = %d\n' $i $i; i=$((i+1)); done
} > cinta.go
git add -A && git commit -qm cinta || exit 1`)
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-504", root, "precios", "presupuesto_usd: 5\nauto: hasta-humano\n")
	vdtCorrer(t, "CA-504", vdtOpts{dir: root}, "pedir", "writer", "precios", "--provider", "codex", "-m", "Implementa el spec hasta que verify de verde.")
	time.Sleep(3 * time.Second) // una cinta mal encadenada lanzaria en este plazo
	if n := len(vdtSobres(t, root)); n != 1 {
		t.Fatalf("CA-504: desde la terminal no corre la cinta: un solo sobre, hay %d: %+v", n, vdtSobres(t, root))
	}
	if ll := f.llamadas(t, "claude"); len(ll) != 0 {
		t.Fatalf("CA-504: la cinta no pidio al reviewer: %q", ll)
	}
}

// CA-505: con el candado de archivo de la tarjeta tomado, hoom pedir sale
// con 1 y MsgLanzando, sin lanzar ni crear la tarea.
func TestCA505_CandadoTomadoEnPedir(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-505", root, "precios", "")
	vdtBacklog(t, "CA-505", root, "nueva")
	for _, c := range []struct{ slug, rol string }{{"precios", "writer"}, {"nueva", "arquitecto"}} {
		soltar, err := hoomfs.Lock(lanzar.Candado(root, c.slug), 0)
		if err != nil {
			t.Fatal(err)
		}
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "CA-505", vdtOpts{dir: root}, "pedir", c.rol, c.slug, "-m", "Haz lo del spec")
		soltar()
		vdtExit(t, "CA-505 ("+c.slug+")", s, 1)
		vdtDiceErr(t, "CA-505", s, "ya se esta lanzando un trabajo en esta tarjeta")
		vdtSinEfectos(t, "CA-505", "pedir con el candado tomado", antes, vdtFoto(t, root))
	}
	f.ninguna(t, "CA-505", "el candado tomado")

	// soltado, se lanza
	vdtCorrer(t, "CA-505", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Haz lo del spec")
	vdtUnaLlamada(t, "CA-505", "pedir con el candado libre", f, "claude")
}

// ===========================================================================
// CA-506: exit code y --json
// ===========================================================================

// CA-506: el exit code es el de hoom agent (el del registro del sobre) y
// --json deja en stdout solo el JSON de hoom agent; las lineas tarjeta,
// valores y → van a stderr.
func TestCA506_ExitCodeYJSONDelSobre(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", vdtNarra)
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-506", root, "precios", "")

	s := vdtCorrer(t, "CA-506", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa")
	sobres := vdtSobres(t, root)
	if len(sobres) != 1 || sobres[0].ExitCode != s.code {
		t.Fatalf("CA-506: el exit code (%d) es el del sobre: %+v\n%s", s.code, sobres, s)
	}

	s = vdtCorrer(t, "CA-506", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa otra vez", "--json")
	var res map[string]any
	if err := json.Unmarshal([]byte(s.out), &res); err != nil {
		t.Fatalf("CA-506: con --json stdout es solo JSON: %v\n%s", err, s)
	}
	for _, k := range []string{"role", "provider", "dir", "envelope_id", "stage", "status", "exit_code"} {
		if _, ok := res[k]; !ok {
			t.Fatalf("CA-506: el JSON es el de hoom agent (falta %q): %v", k, res)
		}
	}
	if ec, _ := res["exit_code"].(float64); int(ec) != s.code || res["role"] != "writer" {
		t.Fatalf("CA-506: exit_code del JSON (%v) = exit code (%d), rol writer: %v", res["exit_code"], s.code, res)
	}
	id, _ := res["envelope_id"].(string)
	if _, err := os.Stat(filepath.Join(root, ".hoom", "envelopes", id+".json")); id == "" || err != nil {
		t.Fatalf("CA-506: el envelope_id es el sobre en disco: %q %v", id, err)
	}
	for _, l := range []string{"tarjeta: precios (argumento)", "valores: ", "→ "} {
		if !strings.Contains(s.err, l) || strings.Contains(s.out, l) {
			t.Fatalf("CA-506: con --json la linea %q va a stderr:\n%s", l, s)
		}
	}
	if strings.Contains(s.out, "NARRACION-UNICA-DEL-CLI") {
		t.Fatalf("CA-506: con --json la narracion no ensucia stdout:\n%s", s)
	}
}

// CA-506: para el reviewer, el exit code y el JSON son los de hoom review.
func TestCA506_ExitCodeYJSONDeLaReview(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtReview(t, "CA-506", root, "precios")
	s := vdtCorrer(t, "CA-506", vdtOpts{dir: root}, "pedir", "reviewer", "precios", "--json")
	var res map[string]any
	if err := json.Unmarshal([]byte(s.out), &res); err != nil {
		t.Fatalf("CA-506: con --json stdout es solo el JSON de hoom review: %v\n%s", err, s)
	}
	for _, k := range []string{"lenses", "passes", "status", "exit_code"} {
		if _, ok := res[k]; !ok {
			t.Fatalf("CA-506: el JSON es el de hoom review (falta %q): %v", k, res)
		}
	}
	if ec, _ := res["exit_code"].(float64); int(ec) != s.code {
		t.Fatalf("CA-506: exit_code del JSON (%v) = exit code (%d)", res["exit_code"], s.code)
	}
}
