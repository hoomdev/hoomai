//go:build linux

// Ronda 1 del test-writer de contencion-del-writer: regresiones de la review
// de codex sobre el encierro de Linux, directo con VistaDe, Envolver,
// Arrancar y Comprobar: la administracion del principal (fab30f), Windows sin
// job simulado con ReemplazarAdopcion (aclaracion 22) y la carrera entre el
// arranque y la gracia (69b8a2); en la ronda 1b, la gracia de los
// descendientes cuando el principal muere (a5c166). Sin bwrap fallan, no se
// saltan (aclaracion 1).
package encierro_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// r1q es s entre comillas simples para sh.
func r1q(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// r1Locks son los *.lock de la raiz del directorio comun.
func r1Locks(comun string) []string {
	var out []string
	es, _ := os.ReadDir(comun)
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".lock") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- punto 1

// CA-678 (id fab30f): encerrado con la vista del writer de una tarea que es
// un worktree enlazado, lo de adentro no cambia, no reemplaza ni borra
// ninguna entrada de encierro.AdministracionPrincipal que exista en la raiz
// del directorio comun (HEAD e index del principal incluidos), no mueve el
// HEAD del principal con git, no escribe su index con git, no crea una
// administracion nueva bajo <comun>/worktrees/ ni escribe la de otra tarea, y
// no deja locks en la raiz del comun. Un git commit en hoom/demo funciona.
func TestR1_fab30f_CA678_ElEncerradoNoMueveAlPrincipal(t *testing.T) {
	const ca = "CA-678"
	cdwInfo(t, ca)
	home := cdwDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	root, wt := cdwRepo(t)
	comun := filepath.Join(root, ".git")
	head, _ := cdwLeer(filepath.Join(comun, "HEAD"))
	sha, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	// mas administracion del principal que exista
	cdwEscribir(t, filepath.Join(comun, "ORIG_HEAD"), string(sha))
	cdwEscribir(t, filepath.Join(comun, "FETCH_HEAD"), strings.TrimSpace(string(sha))+"\t\tbranch 'main' of x\n")
	cdwEscribir(t, filepath.Join(comun, "SQUASH_MSG"), "squash\n")
	cdwEscribir(t, filepath.Join(comun, "info", "sparse-checkout"), "/*\n")
	antes := map[string]string{}
	var b strings.Builder
	for i, e := range encierro.AdministracionPrincipal {
		p := filepath.Join(comun, filepath.FromSlash(e))
		st, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			antes[e] = "dir"
			b.WriteString("( printf x > " + r1q(filepath.Join(p, "r1-colado")) + " ) 2>/dev/null && echo " + r1q("ESCRIBIO "+e+" (adentro)") + "\n")
			continue
		}
		antes[e], _ = cdwLeer(p)
		tmp := "\"$TMPDIR/r1-reemplazo-" + strconv.Itoa(i) + "\""
		b.WriteString("( printf 'colado\\n' >> " + r1q(p) + " ) 2>/dev/null && echo " + r1q("ESCRIBIO "+e+" (agrega)") + "\n")
		b.WriteString("printf 'otro\\n' > " + tmp + " && mv -f " + tmp + " " + r1q(p) + " 2>/dev/null && echo " + r1q("ESCRIBIO "+e+" (reemplaza)") + "\n")
		b.WriteString("rm -f " + r1q(p) + " 2>/dev/null; [ -e " + r1q(p) + " ] || echo " + r1q("ESCRIBIO "+e+" (borra)") + "\n")
	}
	for _, e := range []string{"HEAD", "index", "ORIG_HEAD", "FETCH_HEAD", "SQUASH_MSG", "logs/HEAD"} {
		if _, ok := antes[e]; !ok {
			t.Fatalf("%s: fixture: %s existe y esta en AdministracionPrincipal (%q)", ca, e, encierro.AdministracionPrincipal)
		}
	}
	otraHead, _ := cdwLeer(filepath.Join(comun, "worktrees", "otra", "HEAD"))
	locks := r1Locks(comun)
	b.WriteString("git -C " + r1q(root) + " symbolic-ref HEAD refs/heads/r1-otra 2>/dev/null && echo 'ESCRIBIO HEAD (symbolic-ref)'\n")
	b.WriteString("git -C " + r1q(root) + " update-index --chmod=+x app.go 2>/dev/null && echo 'ESCRIBIO index (update-index)'\n")
	b.WriteString("mkdir " + r1q(filepath.Join(comun, "worktrees", "r1-nueva")) + " 2>/dev/null && echo 'ESCRIBIO worktrees/r1-nueva'\n")
	b.WriteString("git -C " + r1q(root) + " worktree add -q --detach \"$TMPDIR/r1-wt\" 2>/dev/null && echo 'ESCRIBIO worktree add'\n")
	b.WriteString("( printf x >> " + r1q(filepath.Join(comun, "worktrees", "otra", "HEAD")) + " ) 2>/dev/null && echo 'ESCRIBIO worktrees/otra/HEAD'\n")
	b.WriteString("printf 'package app\\n// r1\\n' > app.go && git add app.go && git commit -q -m 'r1 del writer' && echo commit=ok\n")
	v, err := encierro.VistaDe(encierro.Lugar{Principal: root, Tarea: "demo", Arbol: wt, Quien: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	c := cdwCorrer(t, ca, v, 0, 60*time.Second, b.String())
	if strings.Contains(c.salida, "ESCRIBIO") {
		t.Errorf("%s: nada encerrado cambia la administracion del principal:\n%s", ca, c.salida)
	}
	for e, a := range antes {
		p := filepath.Join(comun, filepath.FromSlash(e))
		if a == "dir" {
			if _, err := os.Lstat(filepath.Join(p, "r1-colado")); err == nil {
				t.Errorf("%s: %s/ (que existe) no se escribe", ca, e)
			}
			continue
		}
		if got, ok := cdwLeer(p); !ok || got != a {
			t.Errorf("%s: %s del principal sigue como estaba (%v): %q", ca, e, ok, got)
		}
	}
	if got, _ := cdwLeer(filepath.Join(comun, "HEAD")); got != head {
		t.Errorf("%s: el HEAD del principal no se mueve: %q -> %q", ca, head, got)
	}
	if _, err := os.Lstat(filepath.Join(comun, "worktrees", "r1-nueva")); err == nil {
		t.Errorf("%s: no se crea una administracion nueva bajo worktrees/", ca)
	}
	if es, _ := os.ReadDir(filepath.Join(comun, "worktrees")); len(es) != 2 {
		var ns []string
		for _, e := range es {
			ns = append(ns, e.Name())
		}
		t.Errorf("%s: worktrees/ sigue con las dos tareas: %q", ca, ns)
	}
	if got, _ := cdwLeer(filepath.Join(comun, "worktrees", "otra", "HEAD")); got != otraHead {
		t.Errorf("%s: la administracion de la otra tarea no se escribe: %q", ca, got)
	}
	if got := r1Locks(comun); strings.Join(got, ",") != strings.Join(locks, ",") {
		t.Errorf("%s: no quedan locks en la raiz del comun que bloqueen al principal: %q -> %q", ca, locks, got)
	}
	if !strings.Contains(c.salida, "commit=ok") {
		t.Fatalf("%s: el commit del writer en su rama funciona:\n%s", ca, c.salida)
	}
	out, err := exec.Command("git", "-C", wt, "log", "-1", "--format=%s", "hoom/demo").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "r1 del writer" {
		t.Fatalf("%s: el commit queda en hoom/demo: %q %v", ca, out, err)
	}
}

// ---------------------------------------------------------------- punto 2

// CA-676 y CA-677 (ids c5790e, 5380e1, 15f87d, c1983c, 185514; aclaracion
// 22): con ReemplazarAdopcion simulando Windows sin job, Comprobar de un
// lanzamiento con exigir parcial da el nivel real (ninguno, MotivoSinJob),
// devuelve un *SinJob con ese nivel y su exigir, y mata el arbol en el acto;
// con exigir no da el mismo nivel real, sin error, y el proceso sigue vivo.
// Sin el reemplazo, en Linux Comprobar da el nivel del plan.
func TestR1_c5790e_CA676_CA677_ComprobarSinJob(t *testing.T) {
	const ca = "CA-676"
	info := cdwInfo(t, ca)
	lanzar := func(exigir string) (*exec.Cmd, context.CancelFunc, encierro.Plan, string) {
		t.Helper()
		tok := cdwToken()
		t.Cleanup(func() { cdwMatar(tok) })
		v := cdwVistaSimple(t)
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "exec sh -c 'while :; do sleep 1; done' "+tok)
		cmd.Dir = v.Dir
		cmd.WaitDelay = 20 * time.Second
		p := encierro.Plan{Info: info, Vista: v, Exigir: exigir}
		if err := encierro.Envolver(cmd, p); err != nil {
			t.Fatalf("%s: Envolver: %v", ca, err)
		}
		if err := encierro.Arrancar(cmd, 2*time.Second); err != nil {
			t.Fatalf("%s: Arrancar: %v", ca, err)
		}
		return cmd, cancel, p, tok
	}
	cerrar := func(cmd *exec.Cmd, cancel context.CancelFunc) time.Duration {
		desde := time.Now()
		cancel()
		_ = cmd.Wait()
		encierro.Liberar(cmd)
		return time.Since(desde)
	}

	// sin el reemplazo: Linux tiene lo que el plan dice
	cmd, cancel, p, tok := lanzar(manifest.ExigirParcial)
	got, err := encierro.Comprobar(cmd, p)
	if err != nil || !r1NivelIgual(got, info) {
		t.Errorf("%s: en Linux Comprobar da el nivel del plan, sin error: %+v %v", ca, got, err)
	}
	cerrar(cmd, cancel)
	cdwNadieVivo(t, ca, "control", tok, 5*time.Second)

	restaurar := encierro.ReemplazarAdopcion("windows", func(int) bool { return false })
	defer restaurar()

	// exigir parcial: mata en el acto y da *SinJob
	cmd, cancel, p, tok = lanzar(manifest.ExigirParcial)
	if !cdwAparece(tok, 1, 10*time.Second) {
		t.Fatalf("%s: fixture: el comando encerrado arranco (%v)", ca, cdwConToken(tok))
	}
	got, err = encierro.Comprobar(cmd, p)
	var sj *encierro.SinJob
	if !errors.As(err, &sj) || !r1NivelIgual(sj.Info, r1SinJob) || sj.Exigir != manifest.ExigirParcial {
		t.Errorf("%s: sin job y con exigir parcial, Comprobar devuelve un *SinJob con el nivel real y su exigir: %#v", ca, err)
	}
	if !r1NivelIgual(got, r1SinJob) {
		t.Errorf("%s: Comprobar da el nivel real (ninguno, %q): %+v", ca, encierro.MotivoSinJob, got)
	}
	listo := make(chan struct{})
	go func() { _ = cmd.Wait(); close(listo) }()
	select {
	case <-listo:
	case <-time.After(10 * time.Second):
		cancel()
		<-listo
		t.Errorf("%s: sin job el arbol muere en el acto: el comando seguia a los 10 s", ca)
	}
	encierro.Liberar(cmd)
	cancel()
	cdwNadieVivo(t, ca, "sin job con exigir parcial", tok, 5*time.Second)

	// exigir no: el nivel real, sin error, y sigue corriendo
	cmd, cancel, p, tok = lanzar(manifest.ExigirNo)
	if !cdwAparece(tok, 1, 10*time.Second) {
		t.Fatalf("%s: fixture: el comando encerrado arranco (%v)", ca, cdwConToken(tok))
	}
	got, err = encierro.Comprobar(cmd, p)
	if err != nil || !r1NivelIgual(got, r1SinJob) {
		t.Errorf("%s: sin job y con exigir no, Comprobar da el nivel real sin error: %+v %v", ca, got, err)
	}
	time.Sleep(300 * time.Millisecond)
	if len(cdwConToken(tok)) == 0 {
		t.Errorf("%s: con exigir no el comando sigue corriendo", ca)
	}
	cerrar(cmd, cancel)
	cdwNadieVivo(t, ca, "sin job con exigir no, al cancelar", tok, 5*time.Second)
}

// ---------------------------------------------------------------- punto 10

// CA-679 (id 69b8a2): un comando encerrado con trap de TERM, cancelado lo
// antes posible despues de Arrancar, igual recibe SIGTERM y su gracia: la
// marca del trap aparece, y la de su fin despues de dormir dentro de la
// gracia. Varias veces en el mismo test.
func TestR1_69b8a2_CA679_CanceladoEnSeguidaRecibeSIGTERMYSuGracia(t *testing.T) {
	const ca = "CA-679"
	info := cdwInfo(t, ca)
	script := "trap 'printf term > \"$PWD/term\"; sleep 0.3; printf fin > \"$PWD/fin\"; exit 0' TERM\n" +
		"while :; do sleep 0.05; done\n"
	for vuelta := 0; vuelta < 8; vuelta++ {
		v := cdwVistaSimple(t)
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
		cmd.Dir = v.Dir
		cmd.WaitDelay = 20 * time.Second
		if err := encierro.Envolver(cmd, encierro.Plan{Info: info, Vista: v}); err != nil {
			cancel()
			t.Fatalf("%s: Envolver: %v", ca, err)
		}
		desde := time.Now()
		if err := encierro.Arrancar(cmd, 4*time.Second); err != nil {
			cancel()
			t.Fatalf("%s: Arrancar: %v", ca, err)
		}
		cancel()
		_ = cmd.Wait()
		encierro.Liberar(cmd)
		dur := time.Since(desde)
		if _, ok := cdwLeer(filepath.Join(v.Dir, "term")); !ok {
			t.Fatalf("%s: vuelta %d: cancelado en seguida, el comando igual recibe SIGTERM (su trap no corrio; termino a los %s)", ca, vuelta, dur)
		}
		if _, ok := cdwLeer(filepath.Join(v.Dir, "fin")); !ok {
			t.Fatalf("%s: vuelta %d: la gracia deja terminar el trap (no aparecio su fin; termino a los %s)", ca, vuelta, dur)
		}
		if dur > 15*time.Second {
			t.Fatalf("%s: vuelta %d: termina en un tiempo acotado: %s", ca, vuelta, dur)
		}
	}
}

// ---------------------------------------------------------------- a5c166

// r1HijoConTrap es el hijo con trap de TERM: duerme 0.3 s dentro de su
// gracia y deja la marca term en su arbol. DIR es su arbol, ya citado.
const r1HijoConTrap = "d=DIR\n" +
	"trap 'sleep 0.3; echo term > \"$d/term\"; exit 0' TERM\n" +
	": > \"$d/listo\"\n" +
	"while :; do sleep 0.05; done\n"

// r1HijoQueIgnora es el hijo que ignora TERM: solo el SIGKILL lo termina.
const r1HijoQueIgnora = "d=DIR\n" +
	"trap '' TERM\n" +
	": > \"$d/listo\"\n" +
	"while :; do sleep 0.05; done\n"

// r1HijoConGracia corre encerrado, con Envolver y Arrancar(cmd, gracia), un
// principal SIN trap (muere apenas recibe SIGTERM) que lanza el hijo y lo
// espera. El hijo lleva en su argv <tok>-hijo, que solo el tiene (se arma
// adentro). Cuando el hijo dejo su marca listo (su trap ya esta puesto)
// cancela el contexto: asi no hay carrera con el arranque. Devuelve el
// arbol, el token, el momento de cancelar y un canal que se cierra cuando
// Wait y Liberar volvieron.
func r1HijoConGracia(t *testing.T, ca string, info encierro.Info, hijo string, gracia time.Duration) (dir, tok string, cancelado time.Time, hecho <-chan struct{}) {
	t.Helper()
	v := cdwVistaSimple(t)
	tok = cdwToken()
	t.Cleanup(func() { cdwMatar(tok) })
	script := filepath.Join(v.Dir, "hijo.sh")
	cdwEscribir(t, script, strings.ReplaceAll(hijo, "DIR", r1q(v.Dir)))
	salida, err := os.Create(filepath.Join(cdwDir(t), "salida"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { salida.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sh "+r1q(script)+" \"$0-hijo\" &\nwait\n", tok)
	cmd.Dir = v.Dir
	cmd.Stdout, cmd.Stderr = salida, salida
	cmd.WaitDelay = 20 * time.Second
	if err := encierro.Envolver(cmd, encierro.Plan{Info: info, Vista: v}); err != nil {
		t.Fatalf("%s: Envolver: %v", ca, err)
	}
	if err := encierro.Arrancar(cmd, gracia); err != nil {
		t.Fatalf("%s: Arrancar: %v", ca, err)
	}
	fin := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		encierro.Liberar(cmd)
		close(fin)
	}()
	hasta := time.Now().Add(15 * time.Second)
	for {
		if _, ok := cdwLeer(filepath.Join(v.Dir, "listo")); ok {
			break
		}
		if time.Now().After(hasta) {
			cancel()
			raw, _ := os.ReadFile(salida.Name())
			t.Fatalf("%s: fixture: el hijo arranca y pone su trap (no dejo listo en 15 s): %s", ca, raw)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(cdwConToken(tok+"-hijo")) == 0 {
		cancel()
		t.Fatalf("%s: fixture: el hijo se ve desde afuera con %s-hijo en su argv", ca, tok)
	}
	cancelado = time.Now()
	cancel()
	return v.Dir, tok, cancelado, fin
}

// r1Esperar dice si hecho se cerro a mas tardar en hasta.
func r1Esperar(hecho <-chan struct{}, hasta time.Time) bool {
	select {
	case <-hecho:
		return true
	case <-time.After(time.Until(hasta)):
	}
	select {
	case <-hecho:
		return true
	default:
		return false
	}
}

// CA-679 y CA-684 (hallazgo a5c166): cuando el proceso principal del
// encierro muere por su SIGTERM, el encierro no se desarma con sus
// descendientes adentro: el hijo con trap de TERM recibe su SIGTERM y tiene
// su gracia (su marca aparece), todo termina dentro de la gracia mas un
// margen y no queda nadie vivo. Determinista: se cancela cuando el trap del
// hijo ya esta puesto. Cinco veces en el mismo test.
func TestR1_a5c166_CA679_CA684_ElHijoTieneSuGraciaAunqueElPrincipalMuera(t *testing.T) {
	const ca = "CA-679"
	const gracia, margen = 2 * time.Second, 3 * time.Second
	info := cdwInfo(t, ca)
	for vuelta := 0; vuelta < 5; vuelta++ {
		que := "vuelta " + strconv.Itoa(vuelta)
		dir, tok, cancelado, hecho := r1HijoConGracia(t, ca, info, r1HijoConTrap, gracia)
		hasta := cancelado.Add(gracia + margen)
		if !r1Esperar(hecho, hasta) {
			t.Fatalf("%s: %s: el encierro termina dentro de la gracia mas un margen (%s desde cancelar)", ca, que, gracia+margen)
		}
		for {
			if _, ok := cdwLeer(filepath.Join(dir, "term")); ok {
				break
			}
			if time.Now().After(hasta) {
				t.Fatalf("%s: %s: el hijo recibe su SIGTERM y tiene su gracia aunque el principal muera: su trap no dejo la marca (a los %s de cancelar)",
					ca, que, time.Since(cancelado))
			}
			time.Sleep(20 * time.Millisecond)
		}
		cdwNadieVivo(t, ca, que+": al terminar", tok, time.Until(hasta))
	}
}

// CA-679 y CA-684 (hallazgo a5c166), control: el hijo que ignora TERM sigue
// vivo a mitad de la gracia aunque el principal ya murio, el SIGKILL le
// llega al vencerla, y dentro de la gracia mas un margen no queda nadie vivo.
func TestR1_a5c166_CA679_CA684_ElHijoQueIgnoraTERMRecibeSIGKILLAlVencerLaGracia(t *testing.T) {
	const ca = "CA-679"
	const gracia, margen = 2 * time.Second, 3 * time.Second
	info := cdwInfo(t, ca)
	_, tok, cancelado, hecho := r1HijoConGracia(t, ca, info, r1HijoQueIgnora, gracia)
	time.Sleep(time.Until(cancelado.Add(gracia / 2)))
	if len(cdwConToken(tok+"-hijo")) == 0 {
		t.Fatalf("%s: el hijo que ignora TERM sigue vivo a mitad de la gracia: el SIGKILL llega al vencerla, no antes (%s desde cancelar)", ca, time.Since(cancelado))
	}
	hasta := cancelado.Add(gracia + margen)
	if !r1Esperar(hecho, hasta) {
		t.Fatalf("%s: el encierro termina dentro de la gracia mas un margen (%s desde cancelar)", ca, gracia+margen)
	}
	cdwNadieVivo(t, ca, "el hijo que ignora TERM, al vencer la gracia", tok, time.Until(hasta))
}
