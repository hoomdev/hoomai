// Tests de regresion de la ronda 7 de autonomia-entre-firmas, como
// consumidor externo (proyectos git reales, CLIs de IA falsos en un PATH
// minimo, nunca uno real), sobre la aclaracion 27 (el guardado reconoce su
// commit por lo que hizo git, no por el mensaje):
//
//   - hallazgos 20261113T050855_6dee36, 20261113T051350_ec7241 y
//     20261113T051350_4785a1 (CA-644): un commit-msg que reescribe la
//     primera linea no hace fallar el guardado; si el guardado falla despues,
//     la rama vuelve; y un commit de reemplazo con el mismo padre y el mismo
//     mensaje no se retira.
//
// Reusa los helpers de las rondas anteriores (apTuAprobacion, apr4Hook,
// apr5Mirar, apr5Fallo, apr5NoGuardo, apr6TopeChico, apr6Correr, apr3Matar).
package pilotocmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// apr7CommitMsg es un commit-msg que le antepone `[hook] ` a la primera
// linea del mensaje, deja la marca corrio y sale 0.
func apr7CommitMsg(corrio string) string {
	return "#!/bin/sh\n{ printf '[hook] '; cat \"$1\"; } > \"$1.hoom\" && mv \"$1.hoom\" \"$1\" || exit 1\n: > '" + corrio + "'\nexit 0\n"
}

// apr7Tarjeta arma una tarjeta en Tu aprobacion, guardada, con un cambio sin
// guardar en app.go.
func apr7Tarjeta(t *testing.T, slug string) (root, wt string) {
	t.Helper()
	root = apProyecto(t, "")
	wt = apTuAprobacion(t, root, slug, apConAuto, true)
	apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
	return root, wt
}

// Hallazgos 20261113T050855_6dee36, ec7241 y 4785a1 (CA-644, aclaracion
// 27), caso a: con un commit-msg que le antepone `[hook] ` a la primera
// linea y sale 0, `hoom piloto` guarda: exit 0, la primera linea es la de
// guardado con el commit nuevo, que es HEAD en hoom/<slug>, su padre es el
// HEAD de antes y su mensaje empieza con `[hook] `; app.go queda guardado y
// ninguna linea dice `no pudo guardar`.
func TestHallazgos6dee36_ec7241_4785a1_CA644_UnCommitMsgQueReescribeElMensajeNoHaceFallarElGuardado(t *testing.T) {
	const ca = "6dee36/ec7241/4785a1/CA-644 (a: commit-msg)"
	f := apPATH(t)
	f.cli(t, "claude")
	const slug = "mensaje-reescrito"
	root, wt := apr7Tarjeta(t, slug)
	antes := apr5Mirar(t, root, wt, slug)
	corrio := filepath.Join(t.TempDir(), "corrio")
	apr4Hook(t, wt, "commit-msg", apr7CommitMsg(corrio))

	s := apRun(t, ca, root, slug)
	if !apr1Existe(corrio) {
		t.Fatalf("%s: fixture: el guardado del piloto corre el commit-msg (aclaracion 23):\n%s", ca, s)
	}
	ls := apSoloLineasDelPiloto(t, ca, s)
	for _, l := range ls {
		if strings.Contains(l, "no pudo guardar") {
			t.Fatalf("%s: un commit-msg que reescribe el mensaje y sale 0 no hace fallar el guardado: %q\n%s", ca, l, s)
		}
	}
	if s.code != 0 || len(ls) != 2 || !strings.HasPrefix(ls[1], apDetiene) {
		t.Fatalf("%s: guarda y despues decide (se detiene en Tu aprobacion, exit 0):\n%s", ca, s)
	}
	m := apGuarda.FindStringSubmatch(ls[0])
	if m == nil {
		t.Fatalf("%s: la primera linea es `piloto automatico: guarda el trabajo de la tarjeta: <commit>`: %q", ca, ls[0])
	}
	nuevo := apHead(t, wt)
	if nuevo == antes.head || !strings.HasPrefix(nuevo, m[1]) {
		t.Fatalf("%s: el commit de la linea (%s) es el nuevo HEAD de la tarea (%s, antes %s)", ca, m[1], nuevo, antes.head)
	}
	if r := apGit(t, wt, "rev-parse", "hoom/"+slug); r != nuevo {
		t.Fatalf("%s: la rama de la tarea queda en el commit del guardado: hoom/%s %s, HEAD %s", ca, slug, r, nuevo)
	}
	if p := apGit(t, wt, "rev-parse", "HEAD^"); p != antes.head {
		t.Fatalf("%s: el padre del commit del guardado es el HEAD de antes: %s, antes %s", ca, p, antes.head)
	}
	if msg := apGit(t, wt, "log", "-1", "--format=%s"); !strings.HasPrefix(msg, "[hook] ") {
		t.Fatalf("%s: el commit queda con el mensaje que reescribio el commit-msg: %q", ca, msg)
	}
	if st := apGit(t, wt, "status", "--porcelain", "--", "app.go"); st != "" {
		t.Fatalf("%s: app.go queda guardado: %q", ca, st)
	}
	f.ninguna(t, ca, "un piloto que guarda en Tu aprobacion")
}

// Hallazgos 20261113T050855_6dee36, ec7241 y 4785a1 (CA-644, aclaracion
// 27), caso b: el mismo commit-msg mas un post-commit que se cuelga mas que
// TopeGit (2 s). `hoom piloto` termina en un tiempo acotado con exit 1 y
// `no pudo guardar el trabajo de la tarjeta: <error>`, sin linea de
// guardado, con el candado libre, y HEAD y la rama de la tarea vuelven a
// donde estaban aunque el mensaje del commit no sea el del guardado.
func TestHallazgos6dee36_ec7241_4785a1_CA644_ConElMensajeReescritoLaRamaVuelve(t *testing.T) {
	const ca = "6dee36/ec7241/4785a1/CA-644 (b: commit-msg y post-commit colgado)"
	apr6TopeChico(t)
	f := apPATH(t)
	f.cli(t, "claude")
	const slug = "mensaje-y-colgado"
	root, wt := apr7Tarjeta(t, slug)
	antes := apr5Mirar(t, root, wt, slug)
	aparte := t.TempDir()
	pids, corrioMsg, corrioPost := filepath.Join(aparte, "pids"), filepath.Join(aparte, "corrio-commit-msg"), filepath.Join(aparte, "corrio-post-commit")
	apr4Hook(t, wt, "commit-msg", apr7CommitMsg(corrioMsg))
	apr4Hook(t, wt, "post-commit", "#!/bin/sh\necho $$ >> '"+pids+"'\n: > '"+corrioPost+"'\nexec sleep 60\n")
	t.Cleanup(func() { apr3Matar(pids) })

	s := apr6Correr(t, ca, root, slug, pids)
	if !apr1Existe(corrioMsg) || !apr1Existe(corrioPost) {
		t.Fatalf("%s: fixture: el guardado corre el commit-msg (%v) y el post-commit (%v):\n%s", ca, apr1Existe(corrioMsg), apr1Existe(corrioPost), s)
	}
	apr5NoGuardo(t, ca, s, antes)
	f.ninguna(t, ca, "un piloto que no pudo guardar")
}

// Hallazgos 20261113T050855_6dee36, ec7241 y 4785a1 (CA-644, aclaracion
// 27), caso c: un post-commit reemplaza la punta con `git commit --amend
// --no-edit` (mismo padre, mismo mensaje; otra fecha de commit para que sea
// otro commit) y despues se cuelga mas que TopeGit (2 s). `hoom piloto`
// termina en un tiempo acotado con exit 1 y `no pudo guardar el trabajo de
// la tarjeta: <error>`, sin linea de guardado y con el candado libre; la
// rama de la tarea queda en el commit de reemplazo (no vuelve a la punta de
// antes), y el padre de ese reemplazo es la punta de antes.
func TestHallazgos6dee36_ec7241_4785a1_CA644_UnCommitDeReemplazoNoSeRetira(t *testing.T) {
	const ca = "6dee36/ec7241/4785a1/CA-644 (c: post-commit con --amend)"
	apr6TopeChico(t)
	f := apPATH(t)
	f.cli(t, "claude")
	const slug = "reemplazo"
	root, wt := apr7Tarjeta(t, slug)
	antes := apr5Mirar(t, root, wt, slug)
	aparte := t.TempDir()
	pids, original, reemplazo := filepath.Join(aparte, "pids"), filepath.Join(aparte, "original"), filepath.Join(aparte, "reemplazo")
	apr4Hook(t, wt, "post-commit", "#!/bin/sh\n[ -n \"$HOOM_AMEND\" ] && exit 0\necho $$ >> '"+pids+"'\n"+
		"git rev-parse HEAD > '"+original+"'\n"+
		"HOOM_AMEND=1 GIT_COMMITTER_DATE='2001-02-03T04:05:06Z' git commit -q --amend --no-edit && git rev-parse HEAD > '"+reemplazo+"'\n"+
		"exec sleep 60\n")
	t.Cleanup(func() { apr3Matar(pids) })

	s := apr6Correr(t, ca, root, slug, pids)
	leer := func(p string) string {
		raw, _ := os.ReadFile(p)
		return strings.TrimSpace(string(raw))
	}
	orig, reem := leer(original), leer(reemplazo)
	if orig == "" || reem == "" || orig == reem {
		t.Fatalf("%s: fixture: el post-commit reemplazo el commit del guardado (%q) por otro (%q):\n%s", ca, orig, reem, s)
	}
	if po, pr := apGit(t, wt, "rev-parse", orig+"^"), apGit(t, wt, "rev-parse", reem+"^"); po != antes.rama || pr != antes.rama {
		t.Fatalf("%s: fixture: el original y el reemplazo tienen como padre la punta de antes %s: %s, %s", ca, antes.rama, po, pr)
	}
	if mo, mr := apGit(t, wt, "log", "-1", "--format=%B", orig), apGit(t, wt, "log", "-1", "--format=%B", reem); mo != mr {
		t.Fatalf("%s: fixture: el reemplazo tiene el mismo mensaje: %q, %q", ca, mo, mr)
	}
	apr5Fallo(t, ca, s, root, slug)
	r := apGit(t, root, "rev-parse", "refs/heads/hoom/"+slug)
	if r != reem {
		t.Fatalf("%s: un commit que no creo el guardado no se retira: hoom/%s es %s, el reemplazo %s (antes %s)", ca, slug, r, reem, antes.rama)
	}
	if p := apGit(t, root, "rev-parse", r+"^"); p != antes.rama {
		t.Fatalf("%s: el padre del reemplazo en la rama es la punta de antes: %s, antes %s", ca, p, antes.rama)
	}
	f.ninguna(t, ca, "un piloto que no pudo guardar")
}
