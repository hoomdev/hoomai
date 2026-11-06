// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md sobre el
// binario: hoom agent gana --effort y --timeout (CA-493), hoom run gana
// --timeout (CA-495), un --timeout invalido es exit 2 con el texto del
// contrato, los verbos de la tarjeta se despachan (CA-498, CA-512) y los
// verbos viejos no deducen la tarjeta (CA-513).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
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

// vdtFalso pone al frente del PATH un CLI de IA falso que anota sus
// argumentos (separados por NUL) en un archivo por invocacion.
func vdtFalso(t *testing.T, names ...string) string {
	t.Helper()
	bin, argv := t.TempDir(), t.TempDir()
	for _, name := range names {
		s := "#!/bin/sh\nprintf '%s\\000' \"$@\" > '" + argv + "/" + name + ".'$$\nexit 0\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	return argv
}

func vdtLlamadas(t *testing.T, argv, name string) [][]string {
	t.Helper()
	entries, _ := os.ReadDir(argv)
	var out [][]string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), name+".") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(argv, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00"))
	}
	return out
}

func vdtSigue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// vdtRepo es qcRepo con el .gitignore canonico de .hoom, y extra en
// hoom.yaml.
func vdtRepo(t *testing.T, extra string) string {
	t.Helper()
	dir := qcRepo(t)
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	raw, err := os.ReadFile(filepath.Join(dir, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hoom.yaml"), []byte(string(raw)+extra), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".hoom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hoom", ".gitignore"), []byte(hoomfs.GitignoreBody()), 0o644); err != nil {
		t.Fatal(err)
	}
	qcGit(t, dir, "add", "-A")
	qcGit(t, dir, "commit", "-q", "-m", "gitignore de .hoom")
	return dir
}

func vdtTimeoutTexto(v string) string {
	return `--timeout: "` + v + `" no es una duracion entre 1m y 12h (ej. 45m, 1h30m)`
}

// CA-493 y caso limite: un --timeout invalido en hoom agent es exit 2 con el
// texto del contrato, sin correr el CLI.
func TestCA493_AgentTimeoutInvalidoEsExit2(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude")
	dir := vdtRepo(t, "")
	for _, v := range []string{"0", "30", "13h", "59s"} {
		s := qcCorrer(t, bin, dir, nil, "agent", "--role", "writer", "--timeout", v, "implementa")
		if s.exit != 2 || !strings.Contains(s.stderr, vdtTimeoutTexto(v)) {
			t.Fatalf("CA-493: hoom agent --timeout %s es exit 2 con %q: %d\n%s", v, vdtTimeoutTexto(v), s.exit, s.todo())
		}
	}
	if ll := vdtLlamadas(t, argv, "claude"); len(ll) != 0 {
		t.Fatalf("CA-493: un --timeout invalido no corre el CLI: %q", ll)
	}
}

// CA-495: hoom run gana --timeout, con las mismas reglas.
func TestCA495_RunTimeoutInvalidoEsExit2(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude")
	dir := vdtRepo(t, "")
	for _, v := range []string{"0", "30", "13h"} {
		s := qcCorrer(t, bin, dir, nil, "run", "--provider", "claude", "--timeout", v, "hola")
		if s.exit != 2 || !strings.Contains(s.stderr, vdtTimeoutTexto(v)) {
			t.Fatalf("CA-495: hoom run --timeout %s es exit 2 con %q: %d\n%s", v, vdtTimeoutTexto(v), s.exit, s.todo())
		}
	}
	if ll := vdtLlamadas(t, argv, "claude"); len(ll) != 0 {
		t.Fatalf("CA-495: un --timeout invalido no corre el CLI: %q", ll)
	}
}

// CA-495 y CA-494: hoom run --timeout 1m corre y su sidecar trae
// timeout_s 60.
func TestCA495_RunAceptaTimeout(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude")
	dir := vdtRepo(t, "")
	s := qcCorrer(t, bin, dir, nil, "run", "--provider", "claude", "--timeout", "1m", "hola")
	if s.exit != 0 || len(vdtLlamadas(t, argv, "claude")) != 1 {
		t.Fatalf("CA-495: hoom run acepta --timeout y corre el CLI: %d\n%s", s.exit, s.todo())
	}
	metas := runcmd.Metas(dir)
	if len(metas) != 1 || metas[0].TimeoutS != 60 {
		t.Fatalf("CA-494: el sidecar de hoom run --timeout 1m trae timeout_s 60: %+v", metas)
	}
}

// CA-493: hoom agent acepta --effort y --timeout; el provider recibe el
// esfuerzo y el sidecar el tiempo limite.
func TestCA493_AgentAceptaEffortYTimeout(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude")
	dir := vdtRepo(t, "")
	s := qcCorrer(t, bin, dir, nil, "agent", "--role", "writer", "--effort", "high", "--timeout", "45m", "implementa")
	ll := vdtLlamadas(t, argv, "claude")
	if len(ll) != 1 {
		t.Fatalf("CA-493: hoom agent con --effort y --timeout corre el CLI una vez: %d\n%s", len(ll), s.todo())
	}
	if v := vdtSigue(ll[0], "--effort"); v != "high" {
		t.Fatalf("CA-493: el provider recibe el esfuerzo (--effort high): %q", ll[0])
	}
	metas := runcmd.Metas(dir)
	if len(metas) != 1 || metas[0].TimeoutS != 2700 {
		t.Fatalf("CA-493: el sidecar trae timeout_s 2700: %+v", metas)
	}
}

// CA-493: sin flags, hoom agent toma los valores de agents.<rol> de
// hoom.yaml.
func TestCA493_AgentUsaHoomYaml(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude")
	dir := vdtRepo(t, "agents:\n  writer:\n    model: m-yaml\n    effort: e-yaml\n    timeout: 20m\n")
	s := qcCorrer(t, bin, dir, nil, "agent", "--role", "writer", "implementa")
	ll := vdtLlamadas(t, argv, "claude")
	if len(ll) != 1 || vdtSigue(ll[0], "--model") != "m-yaml" || vdtSigue(ll[0], "--effort") != "e-yaml" {
		t.Fatalf("CA-493: sin flags, el modelo y el esfuerzo de agents.writer: %q\n%s", ll, s.todo())
	}
	if metas := runcmd.Metas(dir); len(metas) != 1 || metas[0].TimeoutS != 1200 {
		t.Fatalf("CA-493: sin --timeout, el de agents.writer.timeout (1200 s): %+v", metas)
	}
}

// vdtConTarjetaFijada arma un proyecto con la tarjeta precios (item, tarea)
// fijada con .hoom/cache/tarjeta.
func vdtConTarjetaFijada(t *testing.T, bin string) (dir, wt string) {
	t.Helper()
	dir = vdtRepo(t, "")
	if s := qcCorrer(t, bin, dir, nil, "item", "add", "Precios", "--pedido", "que el catalogo muestre precios"); s.exit != 0 {
		t.Fatalf("fixture: hoom item add: %s", s.todo())
	}
	qcGit(t, dir, "add", "-A")
	qcGit(t, dir, "commit", "-q", "-m", "item")
	if s := qcCorrer(t, bin, dir, nil, "task", "start", "precios"); s.exit != 0 {
		t.Fatalf("fixture: hoom task start: %s", s.todo())
	}
	p := filepath.Join(dir, filepath.FromSlash(tarjetacmd.ArchivoFijada))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("precios\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, filepath.Join(dir, ".hoom", "worktrees", "precios")
}

// CA-513: con una tarjeta fijada, hoom review sin --task sigue revisando el
// proyecto (no la tarjeta) y hoom agent sin --spec corre en el proyecto sin
// exigir spec: los verbos viejos no deducen la tarjeta.
func TestCA513_LosVerbosViejosNoDeducenLaTarjeta(t *testing.T) {
	bin := qcHoom(t)
	vdtFalso(t, "claude", "codex")
	dir, wt := vdtConTarjetaFijada(t, bin)

	s := qcCorrer(t, bin, dir, nil, "review")
	if strings.Contains(s.todo(), "tarjeta: precios") {
		t.Fatalf("CA-513: hoom review no deduce la tarjeta:\n%s", s.todo())
	}
	if _, err := os.Stat(filepath.Join(wt, ".hoom", "reviews")); err == nil {
		t.Fatalf("CA-513: hoom review sin --task no revisa el espacio de trabajo de la tarjeta fijada")
	}

	s = qcCorrer(t, bin, dir, nil, "agent", "--role", "writer", "implementa sin spec")
	if strings.Contains(s.todo(), "tarjeta: precios") {
		t.Fatalf("CA-513: hoom agent no deduce la tarjeta:\n%s", s.todo())
	}
	metas := runcmd.Metas(dir)
	if len(metas) != 1 || metas[0].Task != "" || metas[0].Dir != dir {
		t.Fatalf("CA-513: hoom agent sin --task ni --spec corre en el proyecto: %+v\n%s", metas, s.todo())
	}
}

// CA-498 y CA-512: el binario despacha los verbos de la tarjeta: hoom usar
// sin tarjeta, -h de cada verbo con exit 0.
func TestCA512_ElBinarioDespachaLosVerbos(t *testing.T) {
	bin := qcHoom(t)
	vdtFalso(t, "claude")
	dir := vdtRepo(t, "")
	s := qcCorrer(t, bin, dir, nil, "usar")
	if s.exit != 0 || !strings.Contains(s.stdout, "sin tarjeta fijada") {
		t.Fatalf("CA-500: hoom usar sin tarjeta fijada: %d\n%s", s.exit, s.todo())
	}
	for _, v := range tarjetacmd.Verbos {
		s := qcCorrer(t, bin, dir, nil, v, "-h")
		if s.exit != 0 || !strings.Contains(s.todo(), tarjetacmd.Uso(v)) {
			t.Fatalf("CA-512: hoom %s -h sale con 0 e imprime el uso: %d\n%s", v, s.exit, s.todo())
		}
	}
	s = qcCorrer(t, bin, dir, nil, "siguiente")
	if s.exit != 2 || !strings.Contains(s.stderr, "no se de que tarjeta hablas") {
		t.Fatalf("CA-498: hoom siguiente sin tarjeta es exit 2: %d\n%s", s.exit, s.todo())
	}
}
