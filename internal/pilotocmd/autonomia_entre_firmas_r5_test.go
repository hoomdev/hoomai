// Tests de regresion de la ronda 5 de autonomia-entre-firmas, como
// consumidor externo (proyectos git reales, CLIs de IA falsos en un PATH
// minimo, nunca uno real), sobre la aclaracion 24 (un guardado que falla no
// deja commit, y un hook no cambia lo que se guarda):
//
//   - hallazgo 20261113T030054_048c45 (CA-638, CA-644): un hook del
//     guardado que cuela un test protegido, o una ruta que el guardado no
//     pidio, hace fallar el guardado;
//   - hallazgos 20261113T030408_1a9510 y 20261113T030719_a808f9 (CA-644): un
//     post-commit que no termina no deja el commit.
package pilotocmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/pilotocmd"
)

const apr5Protegido = "package app\n\n// CA-1: el total redondea bien\n"

// apr5Protegida arma una tarjeta en Writer con un test protegido: spec
// firmado, precios_test.go guardado en la tarea y el sobre del test-writer
// que lo trasplanto (Trasplantados), como en las rondas anteriores. No deja
// nada sin guardar.
func apr5Protegida(t *testing.T, ca string) (root, wt, slug string) {
	t.Helper()
	root = apProyecto(t, "")
	slug = "colada"
	apItem(t, root, slug, apConAuto)
	wt = apTarea(t, root, slug)
	apEscribir(t, wt, apSpecRel(slug), apSpecTexto("- CA-1: el total redondea bien."))
	apFirmar(t, wt, slug)
	apEscribir(t, wt, "precios_test.go", apr5Protegido)
	apCommit(t, wt, "spec firmado y test del test-writer")
	ini := time.Now().UTC().Add(-time.Hour)
	if err := envelope.Write(root, envelope.Record{ID: ini.Format("20060102T150405") + "_ae5001", Role: "test-writer",
		Provider: "claude", Task: slug, Dir: wt, Spec: apSpecRel(slug), Columna: boardcmd.ColTestWriter, Stage: "verify",
		Step: 5, Steps: 6, Status: envelope.StatusNotDeliverable, ExitCode: 1, Trasplantados: []string{"precios_test.go"},
		Pedido: "los tests del spec", StartedAt: ini, UpdatedAt: ini.Add(5 * time.Minute), EndedAt: ini.Add(5 * time.Minute)}); err != nil {
		t.Fatalf("%s: fixture: el sobre del test-writer: %v", ca, err)
	}
	if prot, err := agentcmd.TestsProtegidos(root, slug); err != nil || !reflect.DeepEqual(prot, []string{"precios_test.go"}) {
		t.Fatalf("%s: fixture: la tarea protege precios_test.go: %q %v", ca, prot, err)
	}
	return root, wt, slug
}

// apr5Antes es la tarea antes de un guardado que tiene que fallar: su raiz,
// su worktree, su slug, HEAD y la punta de hoom/<slug>.
//
// Hallazgo low 20261113T040622_b163a0 (ronda 6): apr5NoGuardo recibia nueve
// argumentos y usaba protegido == "" como bandera. Lo re-expresan apr5Antes
// (lo de antes del guardado, tomado con apr5Mirar), apr5Fallo (la detencion
// y el candado), apr5NoGuardo (apr5Fallo mas HEAD y la rama donde estaban) y
// apr5ProtegidoIntacto (el test protegido en HEAD como estaba), con las
// mismas exigencias que antes.
type apr5Antes struct{ root, wt, slug, head, rama string }

// apr5Mirar toma HEAD y la punta de hoom/<slug> de la tarea.
func apr5Mirar(t *testing.T, root, wt, slug string) apr5Antes {
	t.Helper()
	return apr5Antes{root: root, wt: wt, slug: slug, head: apHead(t, wt), rama: apGit(t, wt, "rev-parse", "hoom/"+slug)}
}

// apr5Fallo exige la detencion de un guardado que fallo (aclaracion 24):
// exit 1, `no pudo guardar el trabajo de la tarjeta: <error>` con un error
// no vacio, sin linea de guardado y el candado de la tarjeta libre.
func apr5Fallo(t *testing.T, ca string, s apSalida, root, slug string) {
	t.Helper()
	ls := apSoloLineasDelPiloto(t, ca, s)
	pre := apDetiene + "no pudo guardar el trabajo de la tarjeta: "
	if s.code != 1 || !strings.HasPrefix(apUltima(ls), pre) || strings.TrimSpace(strings.TrimPrefix(apUltima(ls), pre)) == "" {
		t.Fatalf("%s: se detiene con `%s<error>` y exit 1:\n%s", ca, pre, s)
	}
	for _, l := range ls {
		if apGuarda.MatchString(l) {
			t.Fatalf("%s: no dice que guardo: %q", ca, l)
		}
	}
	soltar, err := hoomfs.Lock(pilotocmd.Candado(root, slug), 0)
	if err != nil {
		t.Fatalf("%s: al detenerse, el piloto suelta su candado: %v", ca, err)
	}
	soltar()
}

// apr5NoGuardo es apr5Fallo mas ningun commit nuevo: HEAD y la rama de la
// tarea donde estaban en a.
func apr5NoGuardo(t *testing.T, ca string, s apSalida, a apr5Antes) {
	t.Helper()
	apr5Fallo(t, ca, s, a.root, a.slug)
	if h, r := apHead(t, a.wt), apGit(t, a.wt, "rev-parse", "hoom/"+a.slug); h != a.head || r != a.rama {
		t.Fatalf("%s: ningun commit nuevo en la rama: HEAD %s -> %s, hoom/%s %s -> %s", ca, a.head, h, a.slug, a.rama, r)
	}
}

// apr5ProtegidoIntacto exige que el test protegido en HEAD siga como estaba.
func apr5ProtegidoIntacto(t *testing.T, ca, wt string) {
	t.Helper()
	if enHead := apGit(t, wt, "show", "HEAD:precios_test.go"); enHead+"\n" != apr5Protegido {
		t.Fatalf("%s: el test protegido en HEAD queda como estaba: %q", ca, enHead)
	}
}

// Hallazgo 20261113T030054_048c45 (CA-638, CA-644, aclaracion 24): con un
// test protegido de la tarea, un cambio de codigo sin guardar y un hook
// pre-commit en el repo de la tarea, `hoom piloto` corre el hook y el
// guardado falla: exit 1, `no pudo guardar el trabajo de la tarjeta:
// <error>`, ningun commit nuevo en la rama y el test protegido en HEAD como
// estaba. Variantes: a) el hook edita el test protegido y lo agrega; b) el
// hook agrega al indice un archivo nuevo que no es test (una ruta que el
// guardado no pidio); c) el hook edita el test protegido en disco sin
// agregarlo.
func TestHallazgo048c45_CA638_CA644_UnHookNoCuelaNadaEnElGuardado(t *testing.T) {
	for _, c := range []struct {
		nombre, cuerpo string
	}{
		{"a) edita el test protegido y lo agrega",
			"printf '// CA-1: cambiado por el hook\\n' >> precios_test.go\ngit add precios_test.go\nexit 0\n"},
		{"b) agrega al indice una ruta que el guardado no pidio",
			"printf 'package app\\n\\n// colado por el hook\\n' > colado.go\ngit add colado.go\nexit 0\n"},
		{"c) edita el test protegido en disco sin agregarlo",
			"printf '// CA-1: cambiado por el hook\\n' >> precios_test.go\nexit 0\n"},
	} {
		ca := "048c45/CA-638/CA-644 (" + c.nombre + ")"
		f := apPATH(t)
		root, wt, slug := apr5Protegida(t, ca)
		apEscribir(t, wt, "app.go", "package app\n\n// el codigo del writer, sin guardar\n")
		antes := apr5Mirar(t, root, wt, slug)
		corrio := filepath.Join(t.TempDir(), "corrio")
		apr4Hook(t, wt, "pre-commit", "#!/bin/sh\n: > '"+corrio+"'\n"+c.cuerpo)

		s := apRun(t, ca, root, slug)
		if !apr1Existe(corrio) {
			t.Fatalf("%s: fixture: el guardado del piloto corre el hook pre-commit (aclaracion 23):\n%s", ca, s)
		}
		apr5NoGuardo(t, ca, s, antes)
		apr5ProtegidoIntacto(t, ca, wt)
		f.ninguna(t, ca, "un piloto que no pudo guardar")
	}
}

// Hallazgo 20261113T030054_048c45 (CA-638, CA-644), de punta a punta: un
// run de writer (CLI falso, lanzado como por el piloto) instala un
// pre-commit en el directorio de hooks comun del repo de la tarea, un hook
// que editaria el test protegido y lo agregaria, junto con un cambio de
// codigo, sin tocar el test protegido.
//
// Re-expresado por la aclaracion 25 (ronda 6b), que es decision de la
// persona: antes este test exigia que el guardado del piloto corriera ese
// hook y fallara con `no pudo guardar` (aclaracion 24). Con la aclaracion 25
// cambiar el directorio de hooks es cambiar el material de git de la tarea,
// asi que ahora exige: el sobre del writer corta en `scope`, `no-entregable`,
// exit 1, sin veredicto, con un hallazgo high abierto del gate de scope
// (agentcmd.AutorGateDeScope) que nombra hooks/pre-commit; `hoom piloto` se
// detiene con exit 1 y el Why del caso 8, sin linea de guardado, y el hook no
// corre (su marca no aparece). Sigue exigiendo lo que vale de antes: el
// writer no toco el test protegido y su sobre no lleva la nota de
// tests-del-test-writer, no hay commit nuevo (HEAD y la rama donde estaban),
// el test protegido en HEAD queda como estaba y el piloto suelta su candado.
// TestHallazgos8e2d38_88ed5f_d908ff_CA638_CA644_ElPilotoNoGuardaDespuesDeUnHookDelWriter
// (ronda 6) cubre lo mismo desde el piloto; este conserva el hook que cuela
// el test protegido.
func TestHallazgo048c45_CA638_CA644_ElHookQueDejaUnWriterNoCuelaElTestProtegido(t *testing.T) {
	const ca = "048c45/CA-638/CA-644 (punta a punta, aclaracion 25)"
	f := apPATH(t)
	root, wt, slug := apr5Protegida(t, ca)
	corrio := filepath.Join(t.TempDir(), "corrio")
	hook := "#!/bin/sh\n: > '" + corrio + "'\nprintf '// CA-1: cambiado por el hook\\n' >> precios_test.go\ngit add precios_test.go\nexit 0\n"
	apr1Fake(t, f, "claude", "h=$(git rev-parse --git-path hooks/pre-commit)\nmkdir -p \"$(dirname \"$h\")\"\n"+
		"cat > \"$h\" <<'HOOM_HOOK'\n"+hook+"HOOM_HOOK\nchmod +x \"$h\"\n"+
		"printf 'package app // implementado\\n' > app.go\nexit 0\n")
	veredictos := func() int {
		n := 0
		for _, d := range []string{root, wt} {
			es, _ := os.ReadDir(filepath.Join(d, ".hoom", "verdicts"))
			n += len(es)
		}
		return n
	}
	vAntes := veredictos()
	var buf bytes.Buffer
	w, err := agentcmd.Run(root, apBase, agentcmd.Options{Role: "writer", Provider: "claude", Task: slug, Prompt: "implementa el spec",
		Pilot: true, Columna: boardcmd.ColWriter, Regla: boardcmd.ReglaCadena}, &buf)
	if err != nil || w.EnvelopeID == "" {
		t.Fatalf("%s: fixture: el sobre del writer corre: %v %+v\n%s", ca, err, w, buf.String())
	}
	if w.Note == agentcmd.NotaTestsDelTestWriter {
		t.Fatalf("%s: el writer no toco ningun test protegido, su sobre no lleva la nota de tests-del-test-writer: %+v", ca, w)
	}
	ruta := apGit(t, wt, "rev-parse", "--git-path", "hooks/pre-commit")
	if !filepath.IsAbs(ruta) {
		ruta = filepath.Join(wt, ruta)
	}
	if !apr1Existe(ruta) {
		t.Fatalf("%s: fixture: el writer dejo el hook en %s", ca, ruta)
	}
	if raw, _ := os.ReadFile(filepath.Join(wt, "precios_test.go")); string(raw) != apr5Protegido {
		t.Fatalf("%s: fixture: el writer no toco el test protegido: %q", ca, raw)
	}
	if w.Stage != "scope" || w.Status != envelope.StatusNotDeliverable || w.ExitCode != 1 {
		t.Fatalf("%s: el writer cambio el directorio de hooks: su sobre corta en scope, no-entregable, exit 1 (aclaracion 25): %+v\n%s", ca, w, buf.String())
	}
	if w.VerdictID != "" || veredictos() != vAntes {
		t.Fatalf("%s: el sobre cortado no deja veredicto: %q (veredictos %d -> %d)", ca, w.VerdictID, vAntes, veredictos())
	}
	if _, _, ok := apr6HallazgoDelGate(t, "hooks/pre-commit", wt, root); !ok {
		t.Fatalf("%s: un hallazgo high abierto de %q cuya descripcion nombra hooks/pre-commit (aclaracion 25)", ca, agentcmd.AutorGateDeScope)
	}
	if err := os.Remove(filepath.Join(f.bin, "claude")); err != nil { // el piloto no tiene a quien lanzar
		t.Fatal(err)
	}
	_ = os.Remove(corrio) // se mira solo lo que corre el piloto
	antes := apr5Mirar(t, root, wt, slug)

	s := apRun(t, ca, root, slug)
	ls := apSoloLineasDelPiloto(t, ca, s)
	for _, l := range ls {
		if apGuarda.MatchString(l) {
			t.Fatalf("%s: despues del sobre que dejo el hook, el piloto no guarda: %q\n%s", ca, l, s)
		}
	}
	if apr1Existe(corrio) {
		t.Fatalf("%s: el hook que dejo el writer no corre (aclaracion 25):\n%s", ca, s)
	}
	want := apDetiene + "el trabajo del writer no cerro entregable (" + envelope.StatusNotDeliverable + "): el piloto se detiene ante un rojo"
	if s.code != 1 || apUltima(ls) != want {
		t.Fatalf("%s: se detiene con el Why del caso 8, %q, y exit 1:\n%s", ca, want, s)
	}
	if h, r := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug); h != antes.head || r != antes.rama {
		t.Fatalf("%s: ningun commit nuevo en la rama: HEAD %s -> %s, hoom/%s %s -> %s", ca, antes.head, h, slug, antes.rama, r)
	}
	apr5ProtegidoIntacto(t, ca, wt)
	soltar, err := hoomfs.Lock(pilotocmd.Candado(root, slug), 0)
	if err != nil {
		t.Fatalf("%s: al detenerse, el piloto suelta su candado: %v", ca, err)
	}
	soltar()
}

// Hallazgos 20261113T030408_1a9510 y 20261113T030719_a808f9 (CA-644,
// aclaraciones 19 y 24): un hook post-commit que no termina corre despues de
// que git ya movio HEAD. Con TopeGit en 2 s y el post-commit durmiendo mas,
// `hoom piloto` sale con exit 1 y `no pudo guardar el trabajo de la tarjeta:
// <error>`, HEAD es el de antes del guardado (el cambio no quedo commiteado:
// app.go en HEAD es el de antes), el candado queda libre, el post-commit
// corrio y no queda vivo ningun proceso de su grupo (la marca que escribiria
// en el worktree a los 4 s no aparece).
func TestHallazgos1a9510_a808f9_CA644_UnPostCommitColgadoNoDejaElCommit(t *testing.T) {
	const ca = "1a9510/a808f9/CA-644"
	tope := itemcmd.TopeGit
	itemcmd.TopeGit = 2 * time.Second
	t.Cleanup(func() { itemcmd.TopeGit = tope })

	f := apPATH(t)
	root := apProyecto(t, "")
	const slug = "post-commit-colgado"
	wt := apTuAprobacion(t, root, slug, apConAuto, true)
	appAntes := apGit(t, wt, "show", "HEAD:app.go")
	apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
	antes := apr5Mirar(t, root, wt, slug)

	aparte := t.TempDir()
	pids, corrio := filepath.Join(aparte, "pids"), filepath.Join(aparte, "corrio")
	marca := filepath.Join(wt, "marca-del-hook")
	apr4Hook(t, wt, "post-commit", "#!/bin/sh\necho $$ >> '"+pids+"'\n: > '"+corrio+"'\nsleep 4\n: > '"+marca+"'\nexec sleep 60\n")
	t.Cleanup(func() { apr3Matar(pids) })

	desde := time.Now()
	p := apr1Arrancar(root, slug)
	if !p.esperar(30 * time.Second) {
		apr3Matar(pids)
		p.esperar(2 * time.Minute)
		t.Fatalf("%s: con un post-commit que no termina, el piloto termina en un tiempo acotado (TopeGit 2 s); a los 30 s seguia:\n%s", ca, p.salida())
	}
	s := p.salida()
	if !apr1Existe(corrio) {
		t.Fatalf("%s: el post-commit corrio (el guardado llego a crear el commit):\n%s", ca, s)
	}
	apr5NoGuardo(t, ca, s, antes)
	if enHead := apGit(t, wt, "show", "HEAD:app.go"); enHead != appAntes {
		t.Fatalf("%s: el cambio sigue sin commitear respecto de HEAD: app.go en HEAD es %q", ca, enHead)
	}
	for time.Since(desde) < 8*time.Second {
		if apr1Existe(marca) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if apr1Existe(marca) {
		t.Fatalf("%s: no queda vivo ningun proceso del grupo del post-commit: algo escribio %s en el worktree", ca, marca)
	}
	f.ninguna(t, ca, "un piloto que no pudo guardar")
}
