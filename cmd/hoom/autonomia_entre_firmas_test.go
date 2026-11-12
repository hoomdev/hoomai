// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md sobre
// el binario: `hoom spec approve <ruta>` y `hoom aprobar <slug>` no lanzan
// ningun trabajo y avisan del piloto solo con `auto: hasta-humano`, tambien
// sobre una spec ya aprobada (CA-648, aclaracion 9); `hoom piloto <slug>
// [--json]` despacha, con sus exit codes y su candado (CA-647). CLIs de IA
// falsos al frente de un PATH minimo, nunca uno real.
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/pilotocmd"
)

const (
	aehConAuto = "presupuesto_usd: 5\nauto: hasta-humano\n"
	aehSinAuto = "presupuesto_usd: 5\n"
)

func aehAviso(slug string) string {
	return "la tarjeta tiene piloto automatico: sigue con: hoom piloto " + slug
}

func aehEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func aehHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Fatalf("fixture: HEAD de %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// aehTarjeta: proyecto con findings.block_on, el item de slug (con extra)
// guardado, su tarea abierta por el binario y un spec completo sin firmar en
// el espacio de trabajo (guardado).
func aehTarjeta(t *testing.T, bin, slug, extra string) (root, wt string) {
	t.Helper()
	root = vdtRepo(t, "findings:\n  block_on: high\n")
	aehEscribir(t, root, ".hoom/items/"+slug+".yaml", "titulo: Tarjeta "+slug+"\ntipo: feature\nprioridad: media\n"+
		"pedido: el pedido de "+slug+"\ncreado_por: \"Persona Git <persona@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n"+extra)
	qcGit(t, root, "add", "-A")
	qcGit(t, root, "commit", "-q", "-m", "item "+slug)
	if s := qcCorrer(t, bin, root, nil, "task", "start", slug); s.exit != 0 {
		t.Fatalf("fixture: hoom task start %s: %s", slug, s.todo())
	}
	wt = filepath.Join(root, ".hoom", "worktrees", slug)
	aehEscribir(t, wt, ".hoom/specs/"+slug+".md", "# Spec\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n"+
		"## Criterios de aceptacion\n\n- CA-1: todavia nadie lo cita.\n\n## Decisiones\nx\n\n## Riesgos\nx\n")
	qcGit(t, wt, "add", "-A")
	qcGit(t, wt, "commit", "-q", "-m", "spec sin aprobar")
	return root, wt
}

func aehTieneLinea(out, linea string) bool {
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) == linea {
			return true
		}
	}
	return false
}

func aehNadaLanzado(t *testing.T, ca, que, argv, root, wt, head string) {
	t.Helper()
	for _, name := range []string{"claude", "codex"} {
		if ll := vdtLlamadas(t, argv, name); len(ll) != 0 {
			t.Fatalf("%s: %s no lanza nada, y %s corrio: %q", ca, que, name, ll)
		}
	}
	if recs := envelope.List(root); len(recs) != 0 {
		t.Fatalf("%s: %s no lanza ningun trabajo: %+v", ca, que, recs)
	}
	if h := aehHead(t, wt); h != head {
		t.Fatalf("%s: %s no corre la vuelta del piloto (ni su guardado): HEAD %s -> %s", ca, que, head, h)
	}
}

// CA-648 y aclaracion 9: `hoom spec approve` desde el espacio de trabajo,
// con auto, firma (exit 0), agrega el aviso y no lanza nada; otra vez, sobre
// la spec ya aprobada, vuelve a avisar.
func TestCA648_SpecApproveConAutoAvisaYNoLanza(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude", "codex")
	const slug = "firma-por-terminal"
	root, wt := aehTarjeta(t, bin, slug, aehConAuto)
	head := aehHead(t, wt)
	for _, vez := range []string{"la primera firma", "la spec ya aprobada"} {
		s := qcCorrer(t, bin, wt, nil, "spec", "approve", ".hoom/specs/"+slug+".md")
		if s.exit != 0 || !aehTieneLinea(s.stdout, aehAviso(slug)) {
			t.Fatalf("CA-648: hoom spec approve (%s) con auto sale 0 y agrega %q:\n%s", vez, aehAviso(slug), s.todo())
		}
		if strings.Count(s.todo(), "piloto automatico") != 1 {
			t.Fatalf("CA-648: (%s) aprobar no decide ni imprime lineas del piloto, solo el aviso:\n%s", vez, s.todo())
		}
		aehNadaLanzado(t, "CA-648", "hoom spec approve", argv, root, wt, head)
	}
	if s := qcCorrer(t, bin, wt, nil, "spec", "approve", filepath.Join(wt, ".hoom", "specs", slug+".md")); s.exit != 0 ||
		!aehTieneLinea(s.stdout, aehAviso(slug)) {
		t.Fatalf("CA-648: con la ruta absoluta del spec tambien avisa:\n%s", s.todo())
	}
	aehNadaLanzado(t, "CA-648", "hoom spec approve", argv, root, wt, head)
}

// CA-648: sin auto, `hoom spec approve` es lo de hoy: sin aviso, sin lanzar.
func TestCA648_SpecApproveSinAutoNoAvisa(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude", "codex")
	const slug = "firma-sin-piloto"
	root, wt := aehTarjeta(t, bin, slug, aehSinAuto)
	head := aehHead(t, wt)
	for i := 0; i < 2; i++ {
		s := qcCorrer(t, bin, wt, nil, "spec", "approve", ".hoom/specs/"+slug+".md")
		if s.exit != 0 || strings.Contains(s.todo(), "piloto automatico") || strings.Contains(s.todo(), "hoom piloto") {
			t.Fatalf("CA-648: sin auto firma sin aviso del piloto:\n%s", s.todo())
		}
	}
	aehNadaLanzado(t, "CA-648", "hoom spec approve sin auto", argv, root, wt, head)
}

// CA-648: `hoom aprobar <slug> --si` por el binario avisa con auto y no
// lanza nada.
func TestCA648_AprobarPorElBinarioAvisa(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude", "codex")
	const slug = "aprobar-con-piloto"
	root, wt := aehTarjeta(t, bin, slug, aehConAuto)
	head := aehHead(t, wt)
	s := qcCorrer(t, bin, root, nil, "aprobar", slug, "--si")
	if s.exit != 0 || !aehTieneLinea(s.stdout, aehAviso(slug)) {
		t.Fatalf("CA-648: hoom aprobar con auto sale 0 y agrega %q:\n%s", aehAviso(slug), s.todo())
	}
	aehNadaLanzado(t, "CA-648", "hoom aprobar", argv, root, wt, head)
}

// CA-647: `hoom piloto` por el binario: el uso es exit 2; en Tu aprobacion
// se detiene con 0 (una linea del piloto, o un JSON con --json); sin auto,
// 1; con el candado tomado por otro proceso, 1 y el texto del contrato.
func TestCA647_HoomPilotoPorElBinario(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude", "codex")
	const slug = "piloto-terminal"
	root, wt := aehTarjeta(t, bin, slug, aehConAuto)
	head := aehHead(t, wt)

	for _, args := range [][]string{{"piloto"}, {"piloto", slug, "--nope"}, {"piloto", "--json"}} {
		if s := qcCorrer(t, bin, root, nil, args...); s.exit != 2 || !strings.Contains(s.todo(), "Uso: hoom piloto <slug> [--json]") {
			t.Fatalf("CA-647: hoom %q es un error de uso, exit 2 con el uso:\n%s", args, s.todo())
		}
	}

	s := qcCorrer(t, bin, root, nil, "piloto", slug)
	ls := strings.Split(strings.TrimSpace(s.stdout), "\n")
	if s.exit != 0 || len(ls) != 1 || !strings.HasPrefix(ls[0], "piloto automatico: se detiene: ") {
		t.Fatalf("CA-647: en Tu aprobacion se detiene (una linea) con exit 0:\n%s", s.todo())
	}
	s = qcCorrer(t, bin, wt, nil, "piloto", slug, "--json")
	var r map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s.stdout), &r); err != nil || s.exit != 0 {
		t.Fatalf("CA-647: --json desde el worktree imprime el Resultado como JSON y sale 0: %v\n%s", err, s.todo())
	}
	for _, k := range []string{"slug", "lineas", "exit_code"} {
		if _, ok := r[k]; !ok {
			t.Fatalf("CA-647: el JSON trae %q: %s", k, s.stdout)
		}
	}
	if string(r["slug"]) != `"`+slug+`"` || strings.TrimSpace(string(r["exit_code"])) != "0" {
		t.Fatalf("CA-647: slug y exit_code del JSON: %s", s.stdout)
	}

	soltar, err := hoomfs.Lock(pilotocmd.Candado(root, slug), 0)
	if err != nil {
		t.Fatalf("fixture: tomar el candado del piloto: %v", err)
	}
	s = qcCorrer(t, bin, root, nil, "piloto", slug)
	soltar()
	if s.exit != 1 || !strings.Contains(s.stderr, "hoom: el piloto de "+slug+" ya corre en otro proceso") ||
		strings.Contains(s.stdout, "piloto automatico") {
		t.Fatalf("CA-647: con otro proceso en el candado sale con 1 y el texto del contrato, sin vuelta:\n%s", s.todo())
	}
	aehNadaLanzado(t, "CA-647", "hoom piloto en Tu aprobacion", argv, root, wt, head)

	const manual = "piloto-apagado"
	root2, _ := aehTarjeta(t, bin, manual, aehSinAuto)
	if s := qcCorrer(t, bin, root2, nil, "piloto", manual); s.exit != 1 || !strings.HasPrefix(strings.TrimSpace(s.stdout), "piloto automatico: se detiene: ") {
		t.Fatalf("CA-647: sin auto se detiene con exit 1:\n%s", s.todo())
	}
}
