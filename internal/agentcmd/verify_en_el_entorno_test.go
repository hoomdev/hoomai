// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre el
// sobre de `hoom agent`:
//
//   - CA-768: el sobre de cualquier rol fotografia el .hoom/local.yaml del
//     principal (FotoLocal en Snapshot.Local) antes y despues del run; un run
//     que lo crea, lo cambia o lo borra es manipulacion (RuleTampering, ruta
//     .hoom/local.yaml, DetalleLocal, aclaracion 14): corta en scope,
//     no-entregable, exit 1, sin verify, con un hallazgo high del gate de
//     scope. Ningun manifiesto lo levanta;
//   - CA-765: la verify del sobre corre los gates con el .hoom/local.yaml del
//     principal, como toda verify.
//
// CLIs falsos (fakeProvider) y un `ddev` falso: nada real. Escritos desde el
// spec, las aclaraciones de la ronda 0 y el go doc, sin leer la
// implementacion.
package agentcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------- fixtures

func veeSha(body string) string {
	s := sha256.Sum256([]byte(body))
	return hex.EncodeToString(s[:])
}

func veeQ(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// veeSnap es una foto armada a mano con Local.
func veeSnap(local string) Snapshot {
	s := snap(map[string]string{})
	s.Local = local
	return s
}

// veeProyecto es aeaProyecto (tarea demo iniciada, gate test con cmd) con un
// $HOME del test y el .hoom/.gitignore del principal que ignora local.yaml,
// commiteado en main.
func veeProyecto(t *testing.T, gate string) (root, wt string) {
	t.Helper()
	cdwHome(t)
	root, wt = aeaProyecto(t, gate, "")
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, ".hoom/.gitignore", string(raw)+"local.yaml\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "local.yaml es de esta maquina")
	return root, wt
}

// veeFalsoDdev pone primero en el PATH un `ddev` que anota sus argumentos en
// una carpeta fuera del proyecto y corre `sh -c` con el ultimo.
func veeFalsoDdev(t *testing.T) string {
	t.Helper()
	bin, log := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\n" +
		"log=" + veeQ(log) + "\n" +
		"{ printf '%s\\000' \"$(pwd -P)\"; for a do printf '%s\\000' \"$a\"; done; } > \"$log/.tmp.$$\"\n" +
		"mv \"$log/.tmp.$$\" \"$log/llamada.$$\"\n" +
		"for ultimo do :; done\n" +
		"VEE_DENTRO=1\nexport VEE_DENTRO\n" +
		"exec sh -c \"$ultimo\"\n"
	if err := os.WriteFile(filepath.Join(bin, "ddev"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func veeLlamadas(t *testing.T, log string) [][]string {
	t.Helper()
	ents, err := os.ReadDir(log)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "llamada.") {
			b, err := os.ReadFile(filepath.Join(log, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"))
		}
	}
	return out
}

func veeSaltarSinSh(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("los CLIs falsos son scripts de sh")
	}
}

// veeNotaDeManipulacion es la nota con la que corta hoy un sobre que
// manipula la evidencia (un veredicto falso): la "nota de manipulacion" de
// CA-768, que no tiene constante exportada.
func veeNotaDeManipulacion(t *testing.T) string {
	t.Helper()
	root, _ := veeProyecto(t, "true")
	res, out := aeaCorrer(t, root, "writer", "mkdir -p .hoom/verdicts\n"+
		"printf '{\"id\":\"falso\"}\\n' > .hoom/verdicts/2026-11-21T00-00-00Z_falso.json\nexit 0\n", Options{})
	if res.Stage != "scope" || !res.Scope.Tampering || res.Note == "" {
		t.Fatalf("fixture: un veredicto falso corta en scope con la nota de manipulacion: %+v\n%s", res, out)
	}
	return res.Note
}

// ---------------------------------------------------------------- CA-768

// CA-768: CheckScope, con las dos fotos con Local y distintas, da la
// violacion de manipulacion en .hoom/local.yaml con DetalleLocal y corta;
// iguales, o alguna sin foto (""), no la da. Ningun manifiesto la levanta.
func TestCA768_CheckScopeConLocalDistintoEsManipulacion(t *testing.T) {
	const ca = "CA-768"
	sha1, sha2 := veeSha("en_el_host: [test]\n"), veeSha("ejecutar: \"ddev {cmd}\"\n")
	for _, c := range []struct {
		nombre, antes, despues string
	}{
		{"lo crea", ProtegidoAusente, sha1},
		{"lo cambia", sha1, sha2},
		{"lo borra", sha1, ProtegidoAusente},
		{"lo cambia por un directorio", sha1, HuellaNoRegular + "dir"},
		{"deja de poder leerse", sha1, HuellaIlegible},
	} {
		for _, pol := range []Policy{todo(), {Allow: []string{"**", ".hoom/**", entorno.Ruta}}, {}} {
			res := CheckScope(veeSnap(c.antes), veeSnap(c.despues), pol)
			v, ok := aeaViolacion(res, RuleTampering, entorno.Ruta)
			if !ok || v.Path != ".hoom/local.yaml" || v.Detail != DetalleLocal {
				t.Fatalf("%s: %s (allow %q): violacion %s en %s con %q: %+v", ca, c.nombre, pol.Allow, RuleTampering, entorno.Ruta, DetalleLocal, res.Violations)
			}
			if !res.Tampering || res.OK || !res.Cuts() {
				t.Fatalf("%s: %s (allow %q): es manipulacion y corta: %+v", ca, c.nombre, pol.Allow, res)
			}
		}
	}
	for _, c := range []struct{ nombre, antes, despues string }{
		{"igual", sha1, sha1},
		{"ausente las dos veces", ProtegidoAusente, ProtegidoAusente},
		{"sin foto antes", "", sha1},
		{"sin foto despues", sha1, ""},
		{"sin fotos", "", ""},
	} {
		res := CheckScope(veeSnap(c.antes), veeSnap(c.despues), todo())
		if _, ok := aeaViolacion(res, RuleTampering, entorno.Ruta); ok || res.Tampering || !res.OK {
			t.Fatalf("%s: %s: sin cambio comparable no hay violacion: %+v", ca, c.nombre, res)
		}
	}
	if DetalleLocal != "la plantilla de .hoom/local.yaml decide que corren los gates: un rol no la cambia" || RuleTampering != "manipulacion" {
		t.Fatalf("%s: textos: %q %q", ca, DetalleLocal, RuleTampering)
	}
	want := "manipulacion: el rol writer escribio .hoom/local.yaml - " + DetalleLocal
	if got := DescripcionViolacion(Violation{Path: entorno.Ruta, Rule: RuleTampering, Detail: DetalleLocal}, "writer"); got != want {
		t.Fatalf("%s: la descripcion del hallazgo es %q; fue %q", ca, want, got)
	}
}

// CA-768: FotoLocal lee el disco del principal: el sha256 de un archivo
// regular, ProtegidoAusente si no esta, HuellaNoRegular para un directorio,
// un symlink (sin seguirlo) o un FIFO (sin colgarse), HuellaIlegible si no
// se puede leer. Nunca "".
func TestCA768_FotoLocal(t *testing.T) {
	const ca = "CA-768"
	veeSaltarSinSh(t)
	dir := t.TempDir()
	p := filepath.Join(dir, ".hoom", "local.yaml")
	if got := FotoLocal(dir); got != ProtegidoAusente || got == "" {
		t.Fatalf("%s: sin .hoom: %q", ca, got)
	}
	write(t, dir, ".hoom/otro.yaml", "x")
	if got := FotoLocal(dir); got != ProtegidoAusente {
		t.Fatalf("%s: ausente: %q", ca, got)
	}
	body := "ejecutar: \"ddev exec sh -c {cmd}\"\n"
	write(t, dir, entorno.Ruta, body)
	if got := FotoLocal(dir); got != veeSha(body) {
		t.Fatalf("%s: regular: el sha256 de sus bytes %s: %q", ca, veeSha(body), got)
	}
	write(t, dir, entorno.Ruta, body+" ")
	if got := FotoLocal(dir); got != veeSha(body+" ") {
		t.Fatalf("%s: un byte mas cambia la foto: %q", ca, got)
	}

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := FotoLocal(dir); !strings.HasPrefix(got, HuellaNoRegular) {
		t.Fatalf("%s: un directorio: %q", ca, got)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}

	destino := filepath.Join(t.TempDir(), "real.yaml")
	if err := os.WriteFile(destino, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, p); err != nil {
		t.Fatal(err)
	}
	enlace := FotoLocal(dir)
	if !strings.HasPrefix(enlace, HuellaNoRegular) || enlace == veeSha(body) {
		t.Fatalf("%s: un symlink no se sigue: %q", ca, enlace)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "otro.yaml"), p); err != nil {
		t.Fatal(err)
	}
	if otro := FotoLocal(dir); !strings.HasPrefix(otro, HuellaNoRegular) || otro == enlace {
		t.Fatalf("%s: apuntar el symlink a otro lado cambia la foto: %q vs %q", ca, otro, enlace)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("mkfifo", p).Run(); err == nil {
		hecho := make(chan string, 1)
		go func() { hecho <- FotoLocal(dir) }()
		select {
		case got := <-hecho:
			if !strings.HasPrefix(got, HuellaNoRegular) {
				t.Fatalf("%s: un FIFO: %q", ca, got)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: FotoLocal no se cuelga con un FIFO", ca)
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}

	if os.Geteuid() != 0 {
		write(t, dir, entorno.Ruta, body)
		if err := os.Chmod(p, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(p, 0o644) })
		if got := FotoLocal(dir); got != HuellaIlegible {
			t.Fatalf("%s: ilegible: %q", ca, got)
		}
	}
}

// CA-768: el writer y el test-writer que crean, cambian o borran el
// .hoom/local.yaml del principal cortan en scope, no-entregable, exit 1, con
// la nota de manipulacion, sin verify ni veredicto, y un hallazgo high de
// `hoom gate de scope` que nombra .hoom/local.yaml.
func TestCA768_UnRolQueTocaElArchivoCortaEnScope(t *testing.T) {
	const ca = "CA-768"
	veeSaltarSinSh(t)
	notaManip := veeNotaDeManipulacion(t)
	for _, rol := range []string{"writer", "test-writer"} {
		trabajo := "printf 'package app // implementado\\n' > app.go\n"
		if rol == "test-writer" {
			trabajo = "printf 'package app\\n\\n// CA-1 nuevo\\n' > nuevo_test.go\n"
		}
		for _, c := range []struct {
			nombre, antes string
			script        func(local string) string
		}{
			{"lo crea", "", func(l string) string { return "printf 'ejecutar: \"sh -c {cmd}\"\\n' > " + veeQ(l) + "\n" }},
			{"lo cambia", "en_el_host: [test]\n", func(l string) string { return "printf 'ejecutar: \"true; {cmd}\"\\n' > " + veeQ(l) + "\n" }},
			{"le agrega una linea", "en_el_host: [test]\n", func(l string) string { return "echo '# nada' >> " + veeQ(l) + "\n" }},
			{"lo borra", "en_el_host: [test]\n", func(l string) string { return "rm " + veeQ(l) + "\n" }},
		} {
			caso := rol + ": " + c.nombre
			root, wt := veeProyecto(t, "true")
			local := filepath.Join(root, ".hoom", "local.yaml")
			if c.antes != "" {
				write(t, root, entorno.Ruta, c.antes)
			}
			antes := aeaVeredictos(root, wt)
			res, out := aeaCorrer(t, root, rol, trabajo+c.script(local)+"exit 0\n", Options{})
			if res.Stage != "scope" || res.Status != envelope.StatusNotDeliverable || res.ExitCode != 1 || !res.Scope.Cuts() || !res.Scope.Tampering {
				t.Fatalf("%s: %s: corta en scope, no-entregable, exit 1: %+v\n%s", ca, caso, res, out)
			}
			if res.VerdictID != "" || res.Check != nil || aeaVeredictos(root, wt) != antes {
				t.Fatalf("%s: %s: sin verify ni veredicto: %+v", ca, caso, res)
			}
			if res.Note != notaManip {
				t.Fatalf("%s: %s: la nota es la de manipulacion %q: %q", ca, caso, notaManip, res.Note)
			}
			if r := aeaRegistro(t, root, res.EnvelopeID); r.Stage != "scope" || r.Status != envelope.StatusNotDeliverable || r.Note != notaManip {
				t.Fatalf("%s: %s: el registro dice lo mismo: %+v", ca, caso, r)
			}
			v, ok := aeaViolacion(res.Scope, RuleTampering, entorno.Ruta)
			if !ok || v.Detail != DetalleLocal || v.FindingID == "" {
				t.Fatalf("%s: %s: violacion %s en %s con %q y su hallazgo: %+v", ca, caso, RuleTampering, entorno.Ruta, DetalleLocal, res.Scope.Violations)
			}
			f, ok := aeaHallazgo(t, v.FindingID, wt, root)
			if !ok {
				t.Fatalf("%s: %s: el hallazgo %s quedo en disco", ca, caso, v.FindingID)
			}
			want := "manipulacion: el rol " + rol + " escribio .hoom/local.yaml - " + DetalleLocal
			if f.Severity != "high" || f.Author != AutorGateDeScope || f.Description != want || f.Description != DescripcionViolacion(v, rol) {
				t.Fatalf("%s: %s: hallazgo high de %q con %q: %+v", ca, caso, AutorGateDeScope, want, f)
			}
		}
	}
}

// CA-768 (control): un sobre que solo lee el archivo, o lo reescribe con
// los mismos bytes, no lo manipula: no hay violacion en .hoom/local.yaml y
// el sobre no corta en scope.
func TestCA768_LeerloOReescribirloIgualNoCorta(t *testing.T) {
	const ca = "CA-768"
	veeSaltarSinSh(t)
	for _, c := range []struct {
		nombre string
		script func(local string) string
	}{
		{"lo lee", func(l string) string { return "cat " + veeQ(l) + " > /dev/null\n" }},
		{"lo reescribe igual", func(l string) string { return "printf 'en_el_host: [test]\\n' > " + veeQ(l) + "\n" }},
	} {
		root, _ := veeProyecto(t, "true")
		write(t, root, entorno.Ruta, "en_el_host: [test]\n")
		local := filepath.Join(root, ".hoom", "local.yaml")
		res, out := aeaCorrer(t, root, "writer", "printf 'package app // implementado\\n' > app.go\n"+c.script(local)+"exit 0\n", Options{})
		if _, ok := aeaViolacion(res.Scope, RuleTampering, entorno.Ruta); ok || res.Stage == "scope" {
			t.Fatalf("%s: %s: no es manipulacion: %+v\n%s", ca, c.nombre, res, out)
		}
	}
}

// ---------------------------------------------------------------- CA-765

// CA-765: la verify del sobre del writer corre los gates con el
// .hoom/local.yaml del principal: el veredicto lleva Entorno, el gate corre
// con el prefijo en el arbol de la tarea, con {rel} .hoom/worktrees/demo.
func TestCA765_LaVerifyDelSobreUsaElPrefijo(t *testing.T) {
	const ca = "CA-765"
	veeSaltarSinSh(t)
	log := veeFalsoDdev(t)
	root, wt := veeProyecto(t, "echo dentro=${VEE_DENTRO:-no}")
	tpl := "ddev exec -d /var/www/html/{rel} sh -c {cmd}"
	body := "ejecutar: \"" + tpl + "\"\n"
	write(t, root, entorno.Ruta, body)
	res, out := aeaCorrer(t, root, "writer", "printf 'package app // implementado\\n' > app.go\nexit 0\n", Options{})
	if res.VerdictID == "" {
		t.Fatalf("%s: el sobre llega a verify: %+v\n%s", ca, res, out)
	}
	var v *verdict.Verdict
	for _, d := range []string{wt, root} {
		all, _ := verdict.LoadAll(d)
		for _, x := range all {
			if x.ID == res.VerdictID {
				v = x
			}
		}
	}
	if v == nil {
		t.Fatalf("%s: el veredicto %s esta en disco", ca, res.VerdictID)
	}
	if v.Entorno == nil || v.Entorno.Ejecutar != tpl || v.Entorno.SHA256 != veeSha(body) {
		t.Fatalf("%s: el veredicto del sobre lleva Entorno: %+v", ca, v.Entorno)
	}
	var g *verdict.GateResult
	for i := range v.Gates {
		if v.Gates[i].Name == "test" {
			g = &v.Gates[i]
		}
	}
	if g == nil || g.Entorno != verdict.EntornoPrefijo || !strings.Contains(g.OutputTail, "dentro=1") || g.Command != "echo dentro=${VEE_DENTRO:-no}" {
		t.Fatalf("%s: el gate corre con el prefijo: %+v", ca, g)
	}
	ll := veeLlamadas(t, log)
	if len(ll) == 0 || ll[0][0] != wt || len(ll[0]) < 4 || ll[0][3] != "/var/www/html/.hoom/worktrees/demo" {
		t.Fatalf("%s: el prefijo corre en el arbol de la tarea con su {rel}: %q", ca, ll)
	}
}
