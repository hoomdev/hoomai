// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md sobre la
// funcion de lanzamiento compartida (CA-491, CA-503, CA-505): los valores de
// un lanzamiento son lo pedido o, si no, lo que la accion propone (que ya
// aplico hoom.yaml), cada uno con su origen; el candado de lanzamiento de una
// tarjeta es de archivo en .hoom/cache/locks/ y, tomado, el lanzamiento se
// rechaza con MsgLanzando sin efectos.
package lanzar_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/valores"
)

func TestMain(m *testing.M) {
	borrarConfig, err := gittest.AislarGit()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture:", err)
		os.Exit(2)
	}
	code := m.Run()
	borrarConfig()
	os.Exit(code)
}

func vdtF(v float64) *float64 { return &v }

// vdtAccion es una accion de rol tal como la propone la tarjeta despues de
// aplicar hoom.yaml.
func vdtAccion() boardcmd.Action {
	return boardcmd.Action{ID: boardcmd.ActPedirWriter, Role: "writer", Enabled: true, Pedido: "Implementa el spec",
		Model: "m-accion", Effort: "e-accion", TimeoutS: 2700, BudgetUSD: vdtF(2),
		Providers: []boardcmd.ProviderOption{
			{Name: "claude", OK: true, Budget: true, MinBudgetUSD: 0.5},
			{Name: "opencode", OK: false, Why: "opencode no puede recibir el contrato del rol"},
			{Name: "codex", OK: true, Default: true},
		},
		Origen: valores.Origenes{Provider: valores.OrigenHoomYAML, Model: valores.OrigenHoomYAML,
			Effort: valores.OrigenHoomYAML, BudgetUSD: valores.OrigenTarjeta, Timeout: valores.OrigenHoomYAML}}
}

// CA-491 y CA-503: sin nada pedido, los valores son los que propone la
// accion (el provider default, su model, effort, budget y timeout_s), con
// los origenes de la accion.
func TestCA491_ValoresSinPedidoSonLosDeLaAccion(t *testing.T) {
	a := vdtAccion()
	c := boardcmd.Card{Slug: "precios", Actions: []boardcmd.Action{a}}
	e := lanzar.Valores(c, a, lanzar.Request{Action: a.ID, Pedido: "x"})
	if e.Provider != "codex" || e.Model != "m-accion" || e.Effort != "e-accion" ||
		e.BudgetUSD == nil || *e.BudgetUSD != 2 || e.Timeout != 45*time.Minute {
		t.Fatalf("CA-491: sin pedir nada, los valores de la accion (codex, m-accion, e-accion, 2, 45m): %+v", e)
	}
	if e.Origen != a.Origen {
		t.Fatalf("CA-503: cada valor conserva el origen que trae la accion: %+v vs %+v", e.Origen, a.Origen)
	}
}

// CA-491 y CA-503: lo pedido gana en cada valor, con origen comando; lo no
// pedido sigue siendo de la accion.
func TestCA491_ValoresPedidosGananConOrigenComando(t *testing.T) {
	a := vdtAccion()
	c := boardcmd.Card{Slug: "precios", Actions: []boardcmd.Action{a}}
	e := lanzar.Valores(c, a, lanzar.Request{Action: a.ID, Provider: "claude", Model: "m-cmd", Effort: "e-cmd",
		BudgetUSD: vdtF(1), Timeout: 10 * time.Minute, Pedido: "x"})
	if e.Provider != "claude" || e.Model != "m-cmd" || e.Effort != "e-cmd" ||
		e.BudgetUSD == nil || *e.BudgetUSD != 1 || e.Timeout != 10*time.Minute {
		t.Fatalf("CA-491: lo pedido gana: %+v", e)
	}
	want := valores.Origenes{Provider: valores.OrigenComando, Model: valores.OrigenComando, Effort: valores.OrigenComando,
		BudgetUSD: valores.OrigenComando, Timeout: valores.OrigenComando}
	if e.Origen != want {
		t.Fatalf("CA-503: lo pedido tiene origen comando: %+v", e.Origen)
	}

	// solo el esfuerzo pedido: el resto, de la accion
	e = lanzar.Valores(c, a, lanzar.Request{Action: a.ID, Effort: "low", Pedido: "x"})
	if e.Effort != "low" || e.Origen.Effort != valores.OrigenComando || e.Model != "m-accion" ||
		e.Origen.Model != valores.OrigenHoomYAML || e.Provider != "codex" || e.Timeout != 45*time.Minute {
		t.Fatalf("CA-491: cada valor por separado: %+v", e)
	}
}

// CA-494: una accion sin timeout declarado (timeout_s 1800) lanza con los
// 30 minutos de hoom, nunca con 0.
func TestCA494_ValoresTimeoutNuncaCero(t *testing.T) {
	a := vdtAccion()
	a.TimeoutS = 1800
	a.Origen.Timeout = valores.OrigenProvider
	e := lanzar.Valores(boardcmd.Card{Slug: "precios", Actions: []boardcmd.Action{a}}, a, lanzar.Request{Action: a.ID})
	if e.Timeout != 30*time.Minute {
		t.Fatalf("CA-494: timeout_s 1800 es 30m: %v", e.Timeout)
	}
}

// CA-505: el candado es un archivo de .hoom/cache/locks/ propio de la
// tarjeta.
func TestCA505_CandadoEsUnArchivoPorTarjeta(t *testing.T) {
	root := t.TempDir()
	got := lanzar.Candado(root, "precios")
	if got != filepath.Join(root, ".hoom", "cache", "locks", "lanzar-precios.lock") {
		t.Fatalf("CA-505: el candado de precios vive en .hoom/cache/locks/lanzar-precios.lock: %s", got)
	}
	if lanzar.Candado(root, "otra") == got {
		t.Fatalf("CA-505: cada tarjeta tiene su candado")
	}
}

func vdtGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	if args[0] == "init" {
		gittest.ApagarMantenimiento(t, dir)
	}
}

func vdtEscribir(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// vdtProyecto arma un proyecto con una tarjeta precios en Backlog (sin
// espacio de trabajo ni spec).
func vdtProyecto(t *testing.T) (string, *manifest.Manifest) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	vdtGit(t, root, "init", "-b", "main")
	vdtGit(t, root, "config", "user.email", "test@hoom.dev")
	vdtGit(t, root, "config", "user.name", "hoom test")
	vdtEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	vdtEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	vdtEscribir(t, root, "app.go", "package app\n")
	vdtEscribir(t, root, ".hoom/items/precios.yaml", "titulo: Precios por region\ntipo: feature\nprioridad: media\n"+
		"pedido: que el catalogo muestre precios por region\ncreado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n")
	vdtGit(t, root, "add", "-A")
	vdtGit(t, root, "commit", "-q", "-m", "inicial")
	m, err := manifest.Load(root, nil)
	if err != nil {
		t.Fatalf("fixture: hoom.yaml: %v", err)
	}
	return root, m
}

// CA-505: con el candado de archivo de la tarjeta tomado (otro proceso
// lanzando), Lanzar se rechaza con MsgLanzando (409 en el Studio) y no crea
// la tarea ni ningun sobre.
func TestCA505_CandadoTomadoRechazaSinEfectos(t *testing.T) {
	root, m := vdtProyecto(t)
	soltar, err := hoomfs.Lock(lanzar.Candado(root, "precios"), 0)
	if err != nil {
		t.Fatalf("fixture: tomar el candado: %v", err)
	}
	defer soltar()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, "precios",
		lanzar.Request{Action: boardcmd.ActPedirArquitecto, Provider: "claude", Pedido: "escribi el spec de precios"})
	var r *lanzar.Rechazo
	if !errors.As(err, &r) || tr != nil {
		t.Fatalf("CA-505: con el candado tomado, un *Rechazo y ningun trabajo: %v %+v", err, tr)
	}
	if r.Motivo != lanzar.MsgLanzando || r.Code != 409 {
		t.Fatalf("CA-505: el rechazo es 409 %q: %+v", lanzar.MsgLanzando, r)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", "precios")); !os.IsNotExist(err) {
		t.Fatalf("CA-505: un lanzamiento rechazado no crea la tarea: %v", err)
	}
	if ents, _ := os.ReadDir(filepath.Join(root, ".hoom", "envelopes")); len(ents) != 0 {
		t.Fatalf("CA-505: un lanzamiento rechazado no escribe sobres: %d", len(ents))
	}
	if lanzar.MsgLanzando != "ya se esta lanzando un trabajo en esta tarjeta" {
		t.Fatalf("CA-505: el texto del contrato: %q", lanzar.MsgLanzando)
	}
}
