//go:build linux

// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre
// runcmd en Linux con encierro (CA-679): un run encerrado que deja procesos
// con setsid y doble fork no deja ninguno vivo al terminar, al morir por su
// timeout ni cuando hoom lo cancela. Sin bwrap fallan (aclaracion 1).
package runcmd

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/providers"
)

func cdwlToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	tok := "cdwntok" + hex.EncodeToString(b)
	t.Cleanup(func() {
		for _, pid := range cdwlVivos(tok) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return tok
}

func cdwlVivos(tok string) []int {
	var out []int
	es, _ := os.ReadDir("/proc")
	for _, e := range es {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err == nil && strings.Contains(string(raw), tok) {
			out = append(out, pid)
		}
	}
	return out
}

func cdwlNadieVivo(t *testing.T, ca, caso, tok string) {
	t.Helper()
	limite := time.Now().Add(3 * time.Second)
	for len(cdwlVivos(tok)) > 0 {
		if time.Now().After(limite) {
			t.Fatalf("%s: %s: no queda vivo ningun proceso del run: %v", ca, caso, cdwlVivos(tok))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func cdwlEscapes(tok string) string {
	bucle := "'while :; do sleep 1; done'"
	return "setsid sh -c " + bucle + " " + tok + "-setsid </dev/null >/dev/null 2>&1 &\n" +
		"sh -c \"sh -c " + bucle + " " + tok + "-doble </dev/null >/dev/null 2>&1 &\" </dev/null >/dev/null 2>&1\n"
}

// cdwlPlan es el plan del writer de la tarea demo de un repo con su
// worktree, con el nivel de este sistema; falla sin bwrap.
func cdwlPlan(t *testing.T, ca string) (root, wt string, plan *encierro.Plan) {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Fatalf("%s: bwrap no esta en el PATH: los tests del encierro fallan sin el (%v)", ca, err)
	}
	root, err := filepath.EvalSymlinks(initRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	wt = filepath.Join(root, ".hoom", "worktrees", "demo")
	git(t, root, "worktree", "add", "-q", "-b", "hoom/demo", wt, "main")
	prov, err := providers.Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	info := encierro.Nivel(true, prov)
	if info.Nivel == encierro.NivelNinguno {
		t.Fatalf("%s: este sistema no tiene encierro (%s)", ca, info.Motivo)
	}
	var estado providers.Estado
	if ce, ok := prov.(providers.ConEstado); ok {
		estado = ce.Estado()
	}
	v, err := encierro.VistaDe(encierro.Lugar{Principal: root, Tarea: "demo", Arbol: wt, Quien: "writer", Estado: estado})
	if err != nil {
		t.Fatal(err)
	}
	return root, wt, &encierro.Plan{Info: info, Vista: v}
}

// CA-679 y CA-674: el run encerrado que termina no deja vivo nada de lo que
// lanzo; su sidecar lleva el nivel del plan.
func TestCA679_ElRunEncerradoQueTerminaNoDejaNada(t *testing.T) {
	const ca = "CA-679"
	root, wt, plan := cdwlPlan(t, ca)
	tok := cdwlToken(t)
	installFake(t, cdwlEscapes(tok)+"sleep 0.3\nexit 0\n")
	m := NewManager(root)
	r, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Dir: wt, Role: "writer", Encierro: plan})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, r.ID)
	cdwlNadieVivo(t, ca, "al terminar", tok)
	if meta := cpsMetaCerrado(t, root, r.ID); meta.Encierro == nil || meta.Encierro.Nivel != plan.Info.Nivel {
		t.Fatalf("CA-674: el sidecar del run encerrado lleva su nivel: %+v", meta.Encierro)
	}
}

// CA-679: al morir por su timeout tampoco queda nada vivo.
func TestCA679_ElRunEncerradoQueVenceNoDejaNada(t *testing.T) {
	const ca = "CA-679"
	root, wt, plan := cdwlPlan(t, ca)
	tok := cdwlToken(t)
	installFake(t, cdwlEscapes(tok)+"sleep 120\nexit 0\n")
	m := NewManager(root)
	r, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Dir: wt, Role: "writer", Encierro: plan, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(60 * time.Second)
	for {
		g, err := m.Get(r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if g.Status != StatusRunning {
			break
		}
		if time.Now().After(limite) {
			t.Fatalf("%s: el timeout de 2 s corta el run encerrado", ca)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if meta := cpsMetaCerrado(t, root, r.ID); meta.Corte != CorteTiempo {
		t.Fatalf("%s: fixture: el run murio por su timeout: %+v", ca, meta)
	}
	cdwlNadieVivo(t, ca, "al vencer", tok)
}

// CA-679: cuando hoom cancela el run encerrado tampoco queda nada vivo.
func TestCA679_ElRunEncerradoCanceladoNoDejaNada(t *testing.T) {
	const ca = "CA-679"
	root, wt, plan := cdwlPlan(t, ca)
	tok := cdwlToken(t)
	installFake(t, cdwlEscapes(tok)+"sleep 120\nexit 0\n")
	m := NewManager(root)
	r, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Dir: wt, Role: "writer", Encierro: plan})
	if err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(20 * time.Second)
	for len(cdwlVivos(tok)) < 2 {
		if time.Now().After(limite) {
			t.Fatalf("%s: fixture: el run encerrado lanzo sus procesos: %v", ca, cdwlVivos(tok))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := m.Cancel(r.ID); err != nil {
		t.Fatal(err)
	}
	m.Wait(r.ID)
	cdwlNadieVivo(t, ca, "al cancelar", tok)
}
