//go:build linux

// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre el
// encierro de Linux, directo con Envolver, Arrancar y Liberar: piensan como
// un writer que se quiere escapar (setsid, doble fork, nohup, un symlink o
// una ruta relativa fuera del arbol, $HOME, el material de git, la senal a
// hoom, /dev/tty, un socket abstracto). Un test del encierro que no
// encuentra bwrap FALLA, no se salta (aclaracion 1). Cubre CA-678 a CA-682,
// CA-694 y la vista (VistaDe).
package encierro_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
)

// cdwInfo es el nivel de este Linux para una tarea sin provider, y exige que
// pueda encerrar con bwrap.
func cdwInfo(t *testing.T, ca string) encierro.Info {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Fatalf("%s: bwrap no esta en el PATH: un test del encierro falla, no se salta (aclaracion 1): %v", ca, err)
	}
	s := encierro.Sondear()
	i := encierro.NivelDe(encierro.Caso{Sistema: "linux", Sonda: s, Tarea: true})
	if i.Mecanismo != encierro.MecanismoBwrap {
		t.Fatalf("%s: con bwrap en el PATH este Linux tiene que poder encerrar (sonda %+v): %+v", ca, s, i)
	}
	return i
}

func cdwDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func cdwEscribir(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cdwLeer(p string) (string, bool) {
	raw, err := os.ReadFile(p)
	return string(raw), err == nil
}

// cdwVistaSimple: un arbol de run escribible y la cache del writer de la
// tarea demo en un checkout principal aparte, que todavia no existe.
func cdwVistaSimple(t *testing.T) encierro.Vista {
	t.Helper()
	arbol := cdwDir(t)
	return encierro.Vista{Dir: arbol, Escribibles: []string{arbol}, Cache: encierro.Cache(cdwDir(t), "demo", "writer")}
}

type cdwCorrida struct {
	salida string
	err    error
	dur    time.Duration
}

// cdwCorrer corre `sh -c script` encerrado con v, con su gracia, un plazo
// (el contexto del cmd) y env extra.
func cdwCorrer(t *testing.T, ca string, v encierro.Vista, gracia, plazo time.Duration, script string, env ...string) cdwCorrida {
	t.Helper()
	return cdwCorrerCon(t, ca, cdwInfo(t, ca), v, gracia, plazo, script, env...)
}

func cdwCorrerCon(t *testing.T, ca string, info encierro.Info, v encierro.Vista, gracia, plazo time.Duration, script string, env ...string) cdwCorrida {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), plazo)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
	cmd.Dir = v.Dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.WaitDelay = 20 * time.Second
	if err := encierro.Envolver(cmd, encierro.Plan{Info: info, Vista: v}); err != nil {
		t.Fatalf("%s: Envolver: %v", ca, err)
	}
	desde := time.Now()
	if err := encierro.Arrancar(cmd, gracia); err != nil {
		t.Fatalf("%s: Arrancar: %v", ca, err)
	}
	err := cmd.Wait()
	encierro.Liberar(cmd)
	return cdwCorrida{salida: out.String(), err: err, dur: time.Since(desde)}
}

func cdwToken() string {
	return fmt.Sprintf("cdwtok%d_%d", os.Getpid(), rand.New(rand.NewSource(time.Now().UnixNano())).Int63())
}

// cdwConToken son los pids (de afuera) cuyo argv contiene token
// (aclaracion 4: los pids de adentro no sirven desde afuera).
func cdwConToken(token string) []int {
	var out []int
	es, _ := os.ReadDir("/proc")
	for _, e := range es {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err == nil && bytes.Contains(raw, []byte(token)) {
			out = append(out, pid)
		}
	}
	return out
}

func cdwMatar(token string) {
	for _, pid := range cdwConToken(token) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// cdwNadieVivo espera hasta plazo a que no quede ningun proceso con token;
// si queda, lo mata y falla.
func cdwNadieVivo(t *testing.T, ca, que, token string, plazo time.Duration) {
	t.Helper()
	hasta := time.Now().Add(plazo)
	for {
		vivos := cdwConToken(token)
		if len(vivos) == 0 {
			return
		}
		if time.Now().After(hasta) {
			var cmds []string
			for _, p := range vivos {
				raw, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(p), "cmdline"))
				cmds = append(cmds, strings.ReplaceAll(string(raw), "\x00", " "))
			}
			cdwMatar(token)
			t.Fatalf("%s: %s: quedaron vivos %v: %q", ca, que, vivos, cmds)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// cdwAparece espera hasta plazo a que haya n procesos con token.
func cdwAparece(token string, n int, plazo time.Duration) bool {
	hasta := time.Now().Add(plazo)
	for time.Now().Before(hasta) {
		if len(cdwConToken(token)) >= n {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// cdwEscapes deja un proceso con setsid, otro con doble fork y otro con
// nohup, cada uno con el token en su argv, y termina.
const cdwEscapes = `setsid sh -c 'sleep 30; :' "$CDW_TOK-setsid" </dev/null >/dev/null 2>&1 &
( sh -c 'sleep 30; :' "$CDW_TOK-doble" </dev/null >/dev/null 2>&1 & )
nohup sh -c 'sleep 30; :' "$CDW_TOK-nohup" </dev/null >/dev/null 2>&1 &
sleep 0.5
exit 0
`

// ---------------------------------------------------------------- CA-679

// CA-679: setsid, doble fork y nohup no sobreviven al encierro cuando su
// proceso principal termina. Control: sin encierro los tres quedan vivos.
func TestCA679_NadaSobreviveAlProcesoPrincipal(t *testing.T) {
	const ca = "CA-679"
	tok := cdwToken()
	t.Cleanup(func() { cdwMatar(tok) })

	// control: el fixture deja de verdad tres procesos vivos sin encierro
	ctl := exec.Command("/bin/sh", "-c", cdwEscapes)
	ctl.Env = append(os.Environ(), "CDW_TOK="+tok+"ctl")
	ctl.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if out, err := ctl.CombinedOutput(); err != nil {
		t.Fatalf("%s: fixture: el control corre: %v\n%s", ca, err, out)
	}
	if !cdwAparece(tok+"ctl", 3, 3*time.Second) {
		t.Fatalf("%s: fixture: sin encierro, setsid, doble fork y nohup quedan vivos (%v)", ca, cdwConToken(tok+"ctl"))
	}
	cdwMatar(tok + "ctl")

	v := cdwVistaSimple(t)
	c := cdwCorrer(t, ca, v, 0, 30*time.Second, cdwEscapes, "CDW_TOK="+tok)
	if c.err != nil {
		t.Fatalf("%s: el principal sale 0: %v\n%s", ca, c.err, c.salida)
	}
	cdwNadieVivo(t, ca, "al terminar el principal", tok, 3*time.Second)
}

// CA-679: al vencer el plazo, el grupo principal recibe SIGTERM y tiene su
// gracia (su trap corre entero) antes de que termine el encierro.
func TestCA679_AlVencerElPlazoRecibeSIGTERMConSuGracia(t *testing.T) {
	const ca = "CA-679"
	v := cdwVistaSimple(t)
	script := `trap 'printf term > "$PWD/term"; sleep 0.4; printf fin > "$PWD/fin"; exit 0' TERM
printf listo > "$PWD/listo"
while :; do sleep 0.05; done
`
	c := cdwCorrer(t, ca, v, 4*time.Second, 1500*time.Millisecond, script)
	if _, ok := cdwLeer(filepath.Join(v.Dir, "listo")); !ok {
		t.Fatalf("%s: fixture: el principal arranco: %v\n%s", ca, c.err, c.salida)
	}
	if _, ok := cdwLeer(filepath.Join(v.Dir, "term")); !ok {
		t.Fatalf("%s: al vencer el plazo el grupo principal recibe SIGTERM (su trap no corrio): %v\n%s", ca, c.err, c.salida)
	}
	if _, ok := cdwLeer(filepath.Join(v.Dir, "fin")); !ok {
		t.Fatalf("%s: la gracia deja terminar el trap antes de cerrar el encierro: %v\n%s", ca, c.err, c.salida)
	}
	if c.dur < time.Second || c.dur > 12*time.Second {
		t.Fatalf("%s: el plazo no cambia: termino a los %s (plazo 1.5 s, gracia 4 s)", ca, c.dur)
	}
}

// CA-679: uno que ignora SIGTERM y deja un setsid: pasada la gracia, el
// encierro termina con todo adentro, en un tiempo acotado.
func TestCA679_ElQueIgnoraSIGTERMMuereTrasLaGracia(t *testing.T) {
	const ca = "CA-679"
	tok := cdwToken()
	t.Cleanup(func() { cdwMatar(tok) })
	v := cdwVistaSimple(t)
	script := `trap '' TERM HUP INT
setsid sh -c 'trap "" TERM HUP INT; sleep 30; :' "$CDW_TOK" </dev/null >/dev/null 2>&1 &
while :; do sleep 0.05; done
`
	c := cdwCorrer(t, ca, v, 500*time.Millisecond, time.Second, script, "CDW_TOK="+tok)
	if c.dur > 15*time.Second {
		t.Fatalf("%s: tras el plazo y la gracia el encierro termina: tardo %s", ca, c.dur)
	}
	cdwNadieVivo(t, ca, "el que ignora SIGTERM y su setsid", tok, 3*time.Second)
}

// TestCDWAyudante es el proceso de hoom que lanza un encierro y despues
// muere (CA-679, --die-with-parent). Solo hace algo bajo CDW_AYUDANTE.
func TestCDWAyudante(t *testing.T) {
	if os.Getenv("CDW_AYUDANTE") != "muere-con-hoom" {
		return
	}
	arbol, tok := os.Getenv("CDW_ARBOL"), os.Getenv("CDW_TOK")
	v := encierro.Vista{Dir: arbol, Escribibles: []string{arbol}, Cache: filepath.Join(arbol, "cache")}
	// Arrancar (como procesos.Arrancar) le pone Cancel: el cmd nace con contexto
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "sleep 60; :", tok)
	cmd.Dir = arbol
	i := encierro.NivelDe(encierro.Caso{Sistema: "linux", Sonda: encierro.Sondear(), Tarea: true})
	if err := encierro.Envolver(cmd, encierro.Plan{Info: i, Vista: v}); err != nil {
		t.Fatal(err)
	}
	if err := encierro.Arrancar(cmd, 0); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(arbol, "listo"), []byte("listo"), 0o644)
	time.Sleep(2 * time.Minute)
}

// CA-679: el encierro muere con el hoom que lo lanzo, aunque sea por SIGKILL.
func TestCA679_MuereConHoomAunqueSeaPorSIGKILL(t *testing.T) {
	const ca = "CA-679"
	cdwInfo(t, ca)
	tok := cdwToken()
	t.Cleanup(func() { cdwMatar(tok) })
	arbol := cdwDir(t)
	h := exec.Command(os.Args[0], "-test.run=^TestCDWAyudante$", "-test.count=1")
	h.Env = append(os.Environ(), "CDW_AYUDANTE=muere-con-hoom", "CDW_ARBOL="+arbol, "CDW_TOK="+tok)
	var salida bytes.Buffer
	h.Stdout, h.Stderr = &salida, &salida
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	murio := make(chan error, 1)
	go func() { murio <- h.Wait() }()
	terminado := false
	esperar := func() error {
		if terminado {
			return nil
		}
		terminado = true
		return <-murio
	}
	defer func() { _ = h.Process.Kill(); _ = esperar() }()
	// el ayudante que falla (o sale) antes de lanzar el encierro corta el test
	// con su salida, sin esperar el plazo
	hasta := time.Now().Add(20 * time.Second)
	for len(cdwConToken(tok)) == 0 {
		select {
		case err := <-murio:
			terminado = true
			t.Fatalf("%s: fixture: el ayudante termino sin lanzar el encierro: %v\n%s", ca, err, salida.String())
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(hasta) {
			_ = h.Process.Kill()
			err := esperar()
			t.Fatalf("%s: fixture: el ayudante no lanzo el encierro en 20 s (%v): %v\n%s", ca, cdwConToken(tok), err, salida.String())
		}
	}
	if err := h.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = esperar()
	cdwNadieVivo(t, ca, "tras el SIGKILL al hoom que lo lanzo", tok, 5*time.Second)
}

// ---------------------------------------------------------------- CA-678

// CA-678 (directo): lo encerrado escribe en su arbol y en su cache, y nada
// fuera: $HOME, otro directorio de /tmp, un symlink del arbol que apunta
// afuera, una ruta relativa que sale del arbol.
func TestCA678_SoloEscribeEnSuArbolYSuCache(t *testing.T) {
	const ca = "CA-678"
	home := cdwDir(t)
	t.Setenv("HOME", home)
	fuera := cdwDir(t)
	v := cdwVistaSimple(t)
	vecino := filepath.Join(filepath.Dir(v.Dir), filepath.Base(v.Dir)+"-vecino")
	if err := os.MkdirAll(vecino, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(vecino) })
	if err := os.Symlink(fuera, filepath.Join(v.Dir, "escape")); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(v.Dir, filepath.Join(vecino, "rel.txt"))
	if err != nil {
		t.Fatal(err)
	}
	script := `printf x > "$PWD/adentro.txt"
mkdir -p "$PWD/sub/hondo" && printf x > "$PWD/sub/hondo/a.txt"
printf x > "$HOME/escapa.txt" 2>/dev/null
mkdir "$HOME/.config" 2>/dev/null
printf x > "` + fuera + `/directo.txt" 2>/dev/null
printf x > escape/por-symlink.txt 2>/dev/null
printf x > "` + rel + `" 2>/dev/null
printf x > /tmp/cdw-` + filepath.Base(v.Dir) + `.txt 2>/dev/null
printf x > "$TMPDIR/en-la-cache.txt"
exit 0
`
	c := cdwCorrer(t, ca, v, 0, 30*time.Second, script)
	if c.err != nil {
		t.Fatalf("%s: el script sale 0: %v\n%s", ca, c.err, c.salida)
	}
	for _, p := range []string{filepath.Join(v.Dir, "adentro.txt"), filepath.Join(v.Dir, "sub", "hondo", "a.txt")} {
		if _, ok := cdwLeer(p); !ok {
			t.Errorf("%s: lo que escribe en su arbol queda: falta %s", ca, p)
		}
	}
	for _, p := range []string{filepath.Join(home, "escapa.txt"), filepath.Join(home, ".config"), filepath.Join(fuera, "directo.txt"),
		filepath.Join(fuera, "por-symlink.txt"), filepath.Join(vecino, "rel.txt"), "/tmp/cdw-" + filepath.Base(v.Dir) + ".txt"} {
		if _, err := os.Lstat(p); err == nil {
			os.RemoveAll(p)
			t.Errorf("%s: lo que intenta escribir fuera de su arbol no deja nada: quedo %s", ca, p)
		}
	}
	if !cdwHayArchivo(v.Cache, "en-la-cache.txt") {
		t.Errorf("%s: lo que escribe en su cache (TMPDIR) queda dentro de %s", ca, v.Cache)
	}
}

// cdwHayArchivo dice si hay un archivo llamado nombre bajo dir.
func cdwHayArchivo(dir, nombre string) bool {
	hay := false
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() == nombre {
			hay = true
		}
		return nil
	})
	return hay
}

// CA-682: Envolver crea la cache que no existe y los directorios a los que
// apunta el entorno; adentro TMPDIR, XDG_CACHE_HOME y las del perfil estan
// en la cache.
func TestCA682_LaCacheSeCreaYElEntornoApuntaAdentro(t *testing.T) {
	const ca = "CA-682"
	v := cdwVistaSimple(t)
	v.VarsCache = []string{"GOCACHE", "GOMODCACHE", "GOTMPDIR"}
	if _, err := os.Stat(v.Cache); !os.IsNotExist(err) {
		t.Fatalf("%s: fixture: la cache no existe antes: %v", ca, err)
	}
	script := `for k in TMPDIR XDG_CACHE_HOME GOCACHE GOMODCACHE GOTMPDIR; do
  eval "d=\$$k"
  if [ -d "$d" ] && printf x > "$d/prueba-$k"; then echo "$k=$d"; else echo "$k=NO:$d"; fi
done
`
	c := cdwCorrer(t, ca, v, 0, 30*time.Second, script)
	if c.err != nil {
		t.Fatalf("%s: %v\n%s", ca, c.err, c.salida)
	}
	if st, err := os.Stat(v.Cache); err != nil || !st.IsDir() {
		t.Fatalf("%s: hoom crea la cache que no existia: %v", ca, err)
	}
	for _, k := range []string{"TMPDIR", "XDG_CACHE_HOME", "GOCACHE", "GOMODCACHE", "GOTMPDIR"} {
		want := k + "=" + filepath.Join(v.Cache, k)
		if !strings.Contains(c.salida, want+"\n") {
			t.Errorf("%s: adentro %s es un directorio de la cache, que existe y se escribe: quiero %q en\n%s", ca, k, want, c.salida)
		}
		if _, ok := cdwLeer(filepath.Join(v.Cache, k, "prueba-"+k)); !ok {
			t.Errorf("%s: lo escrito en %s queda en la cache", ca, k)
		}
	}
}

// ---------------------------------------------------------------- CA-680

// CA-680: no ve el pid del test (que hace de hoom) ni puede senalarlo.
func TestCA680_NoVeNiSenalaAHoom(t *testing.T) {
	const ca = "CA-680"
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, syscall.SIGUSR1)
	defer signal.Stop(ch)
	pid := strconv.Itoa(os.Getpid())
	v := cdwVistaSimple(t)
	script := `if kill -USR1 ` + pid + ` 2>/dev/null; then echo senal=si; else echo senal=no; fi
if kill -0 ` + pid + ` 2>/dev/null; then echo existe=si; else echo existe=no; fi
n=0; for d in /proc/[0-9]*; do n=$((n+1)); done; echo procesos=$n
`
	c := cdwCorrer(t, ca, v, 0, 30*time.Second, script)
	select {
	case s := <-ch:
		t.Fatalf("%s: lo encerrado senalo al proceso de hoom: %v\n%s", ca, s, c.salida)
	case <-time.After(400 * time.Millisecond):
	}
	if !strings.Contains(c.salida, "senal=no") || !strings.Contains(c.salida, "existe=no") {
		t.Fatalf("%s: adentro el pid de hoom no se ve ni se senala:\n%s", ca, c.salida)
	}
	var n int
	for _, l := range strings.Split(c.salida, "\n") {
		if strings.HasPrefix(l, "procesos=") {
			n, _ = strconv.Atoi(strings.TrimPrefix(l, "procesos="))
		}
	}
	if n == 0 || n > 8 {
		t.Fatalf("%s: adentro se ven solo los procesos del encierro (su propio /proc): %d\n%s", ca, n, c.salida)
	}
}

// CA-680: sin terminal de control (tty_nr 0 en /proc/self/stat), lider de su
// sesion, sin poder abrir /dev/tty, y con no_new_privs.
func TestCA680_SinTerminalLiderDeSuSesionYSinPrivilegios(t *testing.T) {
	const ca = "CA-680"
	v := cdwVistaSimple(t)
	script := `read -r l < /proc/self/stat; echo "stat=$l"
if ( : < /dev/tty ) 2>/dev/null; then echo tty=abre; else echo tty=no; fi
while read -r k val; do [ "$k" = "NoNewPrivs:" ] && echo "nnp=$val"; done < /proc/self/status
`
	c := cdwCorrer(t, ca, v, 0, 30*time.Second, script)
	var stat string
	for _, l := range strings.Split(c.salida, "\n") {
		if strings.HasPrefix(l, "stat=") {
			stat = strings.TrimPrefix(l, "stat=")
		}
	}
	pid, sesion, tty := cdwStat(stat)
	if tty != "0" {
		t.Fatalf("%s: tty_nr es 0: %q\n%s", ca, tty, c.salida)
	}
	if sesion == "" || sesion == "0" || sesion != pid {
		t.Fatalf("%s: es lider de su sesion (sesion %q, pid %q)\n%s", ca, sesion, pid, c.salida)
	}
	if !strings.Contains(c.salida, "tty=no") {
		t.Fatalf("%s: no puede abrir /dev/tty:\n%s", ca, c.salida)
	}
	if !strings.Contains(c.salida, "nnp=1") {
		t.Fatalf("%s: corre con no_new_privs:\n%s", ca, c.salida)
	}
}

// cdwStat saca pid, sesion y tty_nr de una linea de /proc/<pid>/stat.
func cdwStat(l string) (pid, sesion, tty string) {
	i := strings.LastIndexByte(l, ')')
	if i < 0 {
		return "", "", ""
	}
	cab := strings.Fields(l[:i])
	f := strings.Fields(l[i+1:])
	if len(cab) == 0 || len(f) < 5 {
		return "", "", ""
	}
	return cab[0], f[3], f[4]
}

// CA-680: el entorno sin las variables quitadas, con la config de git del
// entorno, y RunUser tapado con un directorio vacio.
func TestCA680_ElEntornoYRunUser(t *testing.T) {
	const ca = "CA-680"
	for _, k := range encierro.VariablesQuitadas {
		t.Setenv(k, "/tmp/de-la-persona-"+k)
	}
	run := cdwDir(t)
	cdwEscribir(t, filepath.Join(run, "bus"), "socket de mentira")
	cdwEscribir(t, filepath.Join(run, "gnupg", "S.gpg-agent"), "otro")
	restaurar := encierro.ReemplazarRunUser(run)
	defer restaurar()
	if encierro.RunUser() != run {
		t.Fatalf("%s: ReemplazarRunUser cambia RunUser: %q", ca, encierro.RunUser())
	}
	v := cdwVistaSimple(t)
	script := `env
echo "gc.auto=$(git config --get gc.auto)"
echo "maintenance.auto=$(git config --get maintenance.auto)"
echo "core.fsmonitor=$(git config --get core.fsmonitor)"
echo "runuser=[$(ls -A '` + run + `' | tr '\n' ' ')]"
`
	c := cdwCorrer(t, ca, v, 0, 30*time.Second, script)
	for _, k := range encierro.VariablesQuitadas {
		if strings.Contains(c.salida, "\n"+k+"=") || strings.HasPrefix(c.salida, k+"=") {
			t.Errorf("%s: %s no llega a lo encerrado:\n%s", ca, k, c.salida)
		}
	}
	for _, w := range []string{"gc.auto=0\n", "maintenance.auto=false\n", "core.fsmonitor=false\n", "runuser=[]\n"} {
		if !strings.Contains(c.salida, w) {
			t.Errorf("%s: falta %q en lo que ve adentro:\n%s", ca, w, c.salida)
		}
	}
	if _, ok := cdwLeer(filepath.Join(run, "bus")); !ok {
		t.Fatalf("%s: tapar RunUser no lo borra afuera", ca)
	}
	restaurar()
	if encierro.RunUser() == run {
		t.Fatalf("%s: restaurar devuelve el RunUser de antes", ca)
	}
}

// TestCDWAyudanteConecta (CA-680) intenta conectarse al socket abstracto de
// CDW_CONECTAR y escribe el resultado en CDW_SALIDA. Solo hace algo bajo
// esas variables.
func TestCDWAyudanteConecta(t *testing.T) {
	nombre, salida := os.Getenv("CDW_CONECTAR"), os.Getenv("CDW_SALIDA")
	if nombre == "" || salida == "" {
		return
	}
	res := "conecto"
	c, err := net.DialTimeout("unix", "@"+nombre, 2*time.Second)
	if err != nil {
		res = "no: " + err.Error()
	} else {
		_, _ = c.Write([]byte("hola"))
		c.Close()
	}
	_ = os.WriteFile(salida, []byte(res), 0o644)
}

// CA-680: con Landlock ABI 6 o mas, lo encerrado no se conecta a un socket
// abstracto que escucha afuera (control: sin encierro, si). Con un ABI menor
// el nivel es parcial con falta ipc, y el test dice lo que no pudo probar
// (aclaracion 1).
func TestCA680_NoSeConectaAUnSocketAbstractoDeAfuera(t *testing.T) {
	const ca = "CA-680"
	info := cdwInfo(t, ca)
	s := encierro.Sondear()
	if s.LandlockABI < 6 {
		if info.Nivel != encierro.NivelParcial || !reflect.DeepEqual(info.Falta, []string{encierro.FaltaIPC}) || info.Motivo != encierro.MotivoLandlock(s.LandlockABI) {
			t.Fatalf("%s: con Landlock ABI %d el nivel es parcial con falta ipc: %+v", ca, s.LandlockABI, info)
		}
		t.Logf("%s: Landlock ABI %d: no se puede probar que lo encerrado no se conecta a un socket abstracto; solo el nivel parcial con falta ipc", ca, s.LandlockABI)
		return
	}
	if info.Nivel != encierro.NivelCompleto {
		t.Fatalf("%s: con bwrap, espacios y ABI %d el nivel es completo: %+v", ca, s.LandlockABI, info)
	}
	nombre := "cdw-" + cdwToken()
	ln, err := net.Listen("unix", "@"+nombre)
	if err != nil {
		t.Fatalf("%s: fixture: escuchar en el socket abstracto: %v", ca, err)
	}
	defer ln.Close()
	var aceptadas atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			aceptadas.Add(1)
			c.Close()
		}
	}()
	// control sin encierro
	ctlSalida := filepath.Join(cdwDir(t), "control")
	ctl := exec.Command(os.Args[0], "-test.run=^TestCDWAyudanteConecta$", "-test.count=1")
	ctl.Env = append(os.Environ(), "CDW_CONECTAR="+nombre, "CDW_SALIDA="+ctlSalida)
	if out, err := ctl.CombinedOutput(); err != nil {
		t.Fatalf("%s: fixture: el ayudante corre: %v\n%s", ca, err, out)
	}
	if r, _ := cdwLeer(ctlSalida); r != "conecto" {
		t.Fatalf("%s: fixture: sin encierro el ayudante se conecta: %q", ca, r)
	}
	time.Sleep(100 * time.Millisecond)
	antes := aceptadas.Load()

	v := cdwVistaSimple(t)
	salida := filepath.Join(v.Dir, "resultado")
	script := `"$CDW_BIN" -test.run='^TestCDWAyudanteConecta$' -test.count=1 >/dev/null 2>&1; exit 0`
	c := cdwCorrer(t, ca, v, 0, 60*time.Second, script, "CDW_BIN="+os.Args[0], "CDW_CONECTAR="+nombre, "CDW_SALIDA="+salida)
	r, ok := cdwLeer(salida)
	if !ok {
		t.Fatalf("%s: fixture: el ayudante corrio adentro y dejo su resultado: %v\n%s", ca, c.err, c.salida)
	}
	time.Sleep(200 * time.Millisecond)
	if r == "conecto" || aceptadas.Load() != antes {
		t.Fatalf("%s: con Landlock ABI %d lo encerrado no se conecta al socket abstracto de afuera: %q (aceptadas %d -> %d)", ca, s.LandlockABI, r, antes, aceptadas.Load())
	}
}

// CA-680 y CA-683: adentro se pueden crear espacios de usuario anidados
// (el gate test de hoomai y el sandbox de cada CLI los necesitan).
func TestCA680_EspaciosDeUsuarioAnidados(t *testing.T) {
	const ca = "CA-680"
	v := cdwVistaSimple(t)
	c := cdwCorrer(t, ca, v, 0, 60*time.Second, `if bwrap --ro-bind / / true; then echo anidado=si; else echo anidado=no; fi`)
	if !strings.Contains(c.salida, "anidado=si") {
		t.Fatalf("%s: un bwrap anidado corre dentro del encierro:\n%s", ca, c.salida)
	}
}

// ---------------------------------------------------------------- CA-694

// cdwHomeConSecretos arma un $HOME con los secretos conocidos con contenido
// y otros archivos que se tienen que poder leer.
func cdwHomeConSecretos(t *testing.T) string {
	t.Helper()
	home := cdwDir(t)
	cdwEscribir(t, filepath.Join(home, ".ssh", "id_ed25519"), "CLAVE PRIVADA")
	cdwEscribir(t, filepath.Join(home, ".gnupg", "pubring.kbx"), "ANILLO")
	cdwEscribir(t, filepath.Join(home, ".config", "gh", "hosts.yml"), "token: GH")
	cdwEscribir(t, filepath.Join(home, ".aws", "credentials"), "aws_secret_access_key=AWS")
	cdwEscribir(t, filepath.Join(home, ".docker", "config.json"), `{"auths":{"x":"DOCKER"}}`)
	cdwEscribir(t, filepath.Join(home, ".bashrc"), "export LEIBLE=1")
	cdwEscribir(t, filepath.Join(home, ".config", "otro", "x.conf"), "LEIBLE TAMBIEN")
	cdwEscribir(t, filepath.Join(home, ".docker", "otro.json"), "VECINO")
	return home
}

// cdwScriptSecretos: lo que lee y lo que intenta escribir un escapista.
const cdwScriptSecretos = `for d in .ssh .gnupg .config/gh .aws; do echo "dir $d=[$(ls -A "$HOME/$d" 2>&1 | tr '\n' ' ')]"; done
echo "docker=[$(cat "$HOME/.docker/config.json" 2>&1)]"
echo "bashrc=[$(cat "$HOME/.bashrc" 2>&1)]"
echo "otro=[$(cat "$HOME/.config/otro/x.conf" 2>&1)]"
( printf x > "$HOME/.ssh/nueva" ) 2>/dev/null && echo escribio-ssh
( printf x > "$HOME/.docker/config.json" ) 2>/dev/null && echo escribio-docker
( printf x > "$HOME/.aws/credentials" ) 2>/dev/null && echo escribio-aws
exit 0
`

// cdwSecretosTapados exige lo de CA-694 sobre lo que vio adentro.
func cdwSecretosTapados(t *testing.T, ca, quien, home, salida string) {
	t.Helper()
	for _, d := range []string{".ssh", ".gnupg", ".config/gh", ".aws"} {
		if !strings.Contains(salida, "dir "+d+"=[]") {
			t.Errorf("%s: %s: %s se ve vacio adentro:\n%s", ca, quien, d, salida)
		}
	}
	for _, w := range []string{"docker=[]", "bashrc=[export LEIBLE=1]", "otro=[LEIBLE TAMBIEN]"} {
		if !strings.Contains(salida, w) {
			t.Errorf("%s: %s: falta %q (el archivo tapado vacio, el resto del $HOME legible):\n%s", ca, quien, w, salida)
		}
	}
	if strings.Contains(salida, "escribio-") {
		t.Errorf("%s: %s: no puede escribir los secretos tapados:\n%s", ca, quien, salida)
	}
	for p, w := range map[string]string{".ssh/id_ed25519": "CLAVE PRIVADA", ".docker/config.json": `{"auths":{"x":"DOCKER"}}`, ".aws/credentials": "aws_secret_access_key=AWS"} {
		if got, _ := cdwLeer(filepath.Join(home, p)); got != w {
			t.Errorf("%s: %s: afuera %s sigue como estaba: %q", ca, quien, p, got)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "nueva")); err == nil {
		t.Errorf("%s: %s: nada nuevo en ~/.ssh", ca, quien)
	}
}

// CA-694 (directo): los secretos conocidos se ven vacios y no se escriben;
// el resto del $HOME se lee.
func TestCA694_LosSecretosSeVenVacios(t *testing.T) {
	const ca = "CA-694"
	home := cdwHomeConSecretos(t)
	t.Setenv("HOME", home)
	if !reflect.DeepEqual(encierro.SecretosDirs, []string{"~/.ssh", "~/.gnupg", "~/.config/gh", "~/.aws"}) ||
		!reflect.DeepEqual(encierro.SecretosArchivos, []string{"~/.docker/config.json"}) {
		t.Fatalf("%s: los secretos conocidos del contrato: %q %q", ca, encierro.SecretosDirs, encierro.SecretosArchivos)
	}
	v := cdwVistaSimple(t)
	c := cdwCorrer(t, ca, v, 0, 30*time.Second, cdwScriptSecretos)
	cdwSecretosTapados(t, ca, "encerrado", home, c.salida)
}

// CA-694: un secreto que no existe no se monta (no aparece adentro).
func TestCA694_UnSecretoQueNoExisteNoAparece(t *testing.T) {
	const ca = "CA-694"
	home := cdwDir(t)
	cdwEscribir(t, filepath.Join(home, ".bashrc"), "x")
	t.Setenv("HOME", home)
	v := cdwVistaSimple(t)
	c := cdwCorrer(t, ca, v, 0, 30*time.Second, `for d in .ssh .gnupg .aws .docker/config.json; do [ -e "$HOME/$d" ] && echo "aparece $d"; done; exit 0`)
	if strings.Contains(c.salida, "aparece") {
		t.Fatalf("%s: lo que no existe no se monta:\n%s", ca, c.salida)
	}
	for _, d := range []string{".ssh", ".gnupg", ".aws", ".docker"} {
		if _, err := os.Stat(filepath.Join(home, d)); err == nil {
			t.Fatalf("%s: tapar no crea %s afuera", ca, d)
		}
	}
}

// ---------------------------------------------------------------- Envolver

// CA-677 y CA-679: con mecanismo "" (ninguno) o job, Envolver deja el cmd como
// esta; sin bwrap en el PATH es un error y tambien lo deja como esta.
func TestCA679_EnvolverSinBwrapDejaElCmd(t *testing.T) {
	const ca = "CA-679"
	v := cdwVistaSimple(t)
	nuevo := func() *exec.Cmd {
		c := exec.Command("/bin/sh", "-c", "exit 0")
		c.Dir = v.Dir
		c.Env = []string{"A=1"}
		return c
	}
	igual := func(a, b *exec.Cmd) bool {
		return a.Path == b.Path && reflect.DeepEqual(a.Args, b.Args) && a.Dir == b.Dir && reflect.DeepEqual(a.Env, b.Env) &&
			reflect.DeepEqual(a.SysProcAttr, b.SysProcAttr) && len(a.ExtraFiles) == len(b.ExtraFiles)
	}
	for _, i := range []encierro.Info{
		encierro.NivelDe(encierro.Caso{Sistema: "linux", Tarea: false}),
		{Nivel: encierro.NivelParcial, Mecanismo: encierro.MecanismoJob, Falta: []string{encierro.FaltaEscrituras, encierro.FaltaIPC}, Motivo: encierro.MotivoSinEscrituras},
	} {
		c := nuevo()
		if err := encierro.Envolver(c, encierro.Plan{Info: i, Vista: v}); err != nil || !igual(c, nuevo()) {
			t.Fatalf("%s: con mecanismo %q Envolver deja el cmd como esta: %v %+v", ca, i.Mecanismo, err, c)
		}
	}
	t.Setenv("PATH", cdwDir(t))
	c := nuevo()
	i := encierro.NivelDe(encierro.Caso{Sistema: "linux", Tarea: true, Sonda: encierro.Sonda{Bwrap: true, Espacio: true, LandlockABI: 6}})
	if err := encierro.Envolver(c, encierro.Plan{Info: i, Vista: v}); err == nil || !igual(c, nuevo()) {
		t.Fatalf("%s: sin bwrap en el PATH Envolver es un error y deja el cmd como esta: %v %+v", ca, err, c)
	}
	if _, err := os.Stat(v.Cache); err == nil {
		t.Logf("%s: la cache quedo creada aunque Envolver fallo", ca)
	}
}

// ---------------------------------------------------------------- vista

// cdwRepo: un repo con un commit, la tarea demo como worktree en
// .hoom/worktrees/demo en su rama hoom/demo, y otra tarea.
func cdwRepo(t *testing.T) (root, wt string) {
	t.Helper()
	root = cdwDir(t)
	g := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	g(root, "init", "-q", "-b", "main")
	g(root, "config", "user.email", "test@hoom.dev")
	g(root, "config", "user.name", "hoom test")
	g(root, "config", "gc.auto", "0")
	g(root, "config", "maintenance.auto", "false")
	cdwEscribir(t, filepath.Join(root, "app.go"), "package app\n")
	cdwEscribir(t, filepath.Join(root, ".gitignore"), ".hoom/\n")
	g(root, "add", "-A")
	g(root, "commit", "-q", "-m", "inicial")
	wt = filepath.Join(root, ".hoom", "worktrees", "demo")
	g(root, "worktree", "add", "-q", "-b", "hoom/demo", wt)
	g(root, "worktree", "add", "-q", "-b", "hoom/otra", filepath.Join(root, ".hoom", "worktrees", "otra"))
	return root, wt
}

func cdwContiene(xs []string, x string) bool {
	for _, y := range xs {
		if filepath.Clean(y) == filepath.Clean(x) {
			return true
		}
	}
	return false
}

// La vista (CA-678, CA-681, CA-682, CA-690): Dir, lo escribible, lo de solo
// lectura que existe (y no lo que no existe), la cache y las variables del
// perfil.
func TestCA678_CA681_LaVistaDeUnaTarea(t *testing.T) {
	home := cdwDir(t)
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".falso", "sesiones"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, wt := cdwRepo(t)
	comun := filepath.Join(root, ".git")
	cdwEscribir(t, filepath.Join(comun, "info", "attributes"), "*.go diff\n")
	cdwEscribir(t, filepath.Join(comun, "info", "exclude"), "# nada\n")
	cdwEscribir(t, filepath.Join(comun, "hooks", "pre-push"), "exit 0\n") // la carpeta de hooks existe y tiene un hook
	cdwEscribir(t, filepath.Join(wt, ".claude", "settings.json"), "{}")
	cdwEscribir(t, filepath.Join(wt, "falso.json"), "{}")
	escribible := cdwDir(t)
	v, err := encierro.VistaDe(encierro.Lugar{Principal: root, Tarea: "demo", Arbol: wt, Quien: "writer",
		Estado:      providers.Estado{Escribe: []string{"~/.falso/sesiones"}, EnArbol: []string{"falso.json", "falso-no-existe.json"}},
		Escribibles: []string{escribible, "~/.semgrep"}, Perfil: "go"})
	if err != nil {
		t.Fatalf("CA-678: VistaDe de una tarea: %v", err)
	}
	if filepath.Clean(v.Dir) != wt {
		t.Errorf("CA-678: Dir es el arbol de run: %q", v.Dir)
	}
	for _, p := range []string{wt, comun, filepath.Join(home, ".falso", "sesiones"), escribible, filepath.Join(home, ".semgrep")} {
		if !cdwContiene(v.Escribibles, p) {
			t.Errorf("CA-678/CA-690: %s es escribible: %q", p, v.Escribibles)
		}
	}
	for _, p := range []string{root, home, filepath.Join(root, ".hoom", "worktrees", "otra")} {
		if cdwContiene(v.Escribibles, p) {
			t.Errorf("CA-678: %s no es escribible: %q", p, v.Escribibles)
		}
	}
	if !cdwContiene(v.SoloLectura, filepath.Join(comun, "hooks")) && !cdwContiene(v.SoloLectura, filepath.Join(comun, "hooks", "pre-push")) {
		t.Errorf("CA-681: la carpeta de hooks efectiva (o su hook) queda de solo lectura: %q", v.SoloLectura)
	}
	for _, p := range []string{filepath.Join(comun, "config"), filepath.Join(comun, "info", "attributes"),
		filepath.Join(comun, "info", "exclude"), filepath.Join(wt, ".git"), filepath.Join(comun, "worktrees", "otra"),
		filepath.Join(wt, ".claude"), filepath.Join(wt, "falso.json")} {
		if !cdwContiene(v.SoloLectura, p) {
			t.Errorf("CA-681: %s, que existe, queda de solo lectura: %q", p, v.SoloLectura)
		}
	}
	for _, p := range []string{filepath.Join(comun, "shallow"), filepath.Join(comun, "objects", "info", "alternates"),
		filepath.Join(comun, "objects", "info", "grafts"), filepath.Join(wt, ".mcp.json"), filepath.Join(wt, ".codex"),
		filepath.Join(wt, "falso-no-existe.json"), filepath.Join(comun, "worktrees", "demo")} {
		if cdwContiene(v.SoloLectura, p) {
			t.Errorf("CA-681: %s no se monta de solo lectura (no existe, o es su propio directorio de administracion): %q", p, v.SoloLectura)
		}
	}
	if v.Cache != encierro.Cache(root, "demo", "writer") {
		t.Errorf("CA-682: la cache es la del writer de la tarea: %q", v.Cache)
	}
	vars, err := profiles.Cache("go")
	if err != nil || !reflect.DeepEqual(v.VarsCache, vars) {
		t.Errorf("CA-682: VarsCache es profiles.Cache del perfil: %q vs %q (%v)", v.VarsCache, vars, err)
	}
	// sin provider (un gate, el guardado): ningun Escribe; sin perfil, sin variables
	v2, err := encierro.VistaDe(encierro.Lugar{Principal: root, Tarea: "demo", Arbol: wt, Quien: encierro.QuienVerify})
	if err != nil {
		t.Fatal(err)
	}
	if cdwContiene(v2.Escribibles, filepath.Join(home, ".falso", "sesiones")) || len(v2.VarsCache) != 0 || v2.Cache != encierro.Cache(root, "demo", "verify") {
		t.Errorf("CA-682: la vista del verify no suma el Escribe de ningun provider y usa la cache verify: %+v", v2)
	}
}

// CA-678: la vista da error fuera de un repo y con un perfil desconocido.
func TestCA678_VistaDeConErrores(t *testing.T) {
	fuera := cdwDir(t)
	if _, err := encierro.VistaDe(encierro.Lugar{Principal: fuera, Tarea: "demo", Arbol: fuera, Quien: "writer"}); err == nil {
		t.Fatal("CA-678: fuera de un repo git no hay vista: VistaDe es un error")
	}
	root, wt := cdwRepo(t)
	if _, err := encierro.VistaDe(encierro.Lugar{Principal: root, Tarea: "demo", Arbol: wt, Quien: "writer", Perfil: "perfil-que-no-existe"}); err == nil {
		t.Fatal("CA-682: un perfil desconocido es un error de VistaDe")
	}
}

// ---------------------------------------------------------------- CA-681

// CA-681 (directo, vista del writer de una tarea real): escribir en la
// carpeta de hooks, el config comun, info/attributes, info/exclude,
// objects/info/alternates y el .git del arbol falla, cuando existen; un git
// commit en hoom/demo funciona; crear una pieza que no existia no se
// bloquea.
func TestCA681_ElMaterialQueExisteEsDeSoloLectura(t *testing.T) {
	const ca = "CA-681"
	home := cdwDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	root, wt := cdwRepo(t)
	comun := filepath.Join(root, ".git")
	alt := cdwDir(t)
	if err := os.MkdirAll(filepath.Join(comun, "objects", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	cdwEscribir(t, filepath.Join(comun, "info", "attributes"), "*.go diff\n")
	cdwEscribir(t, filepath.Join(comun, "info", "exclude"), "# nada\n")
	cdwEscribir(t, filepath.Join(comun, "objects", "info", "alternates"), alt+"\n")
	cdwEscribir(t, filepath.Join(comun, "hooks", "pre-push"), "exit 0\n")
	piezas := []string{filepath.Join(comun, "hooks", "pre-push"), filepath.Join(comun, "config"), filepath.Join(comun, "info", "attributes"),
		filepath.Join(comun, "info", "exclude"), filepath.Join(comun, "objects", "info", "alternates"), filepath.Join(wt, ".git")}
	antes := map[string]string{}
	for _, p := range piezas {
		antes[p], _ = cdwLeer(p)
	}
	v, err := encierro.VistaDe(encierro.Lugar{Principal: root, Tarea: "demo", Arbol: wt, Quien: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, p := range piezas {
		b.WriteString("( printf 'colado\\n' >> '" + p + "' ) 2>/dev/null && echo 'ESCRIBIO " + p + "'\n")
	}
	b.WriteString("( printf 'exit 0\\n' > '" + filepath.Join(comun, "hooks", "pre-commit") + "' ) 2>/dev/null && echo 'CREO pre-commit'\n")
	b.WriteString("printf 'package app\\n// commit\\n' > app.go && git add app.go && git commit -q -m 'del writer' && echo commit=ok\n")
	b.WriteString("( printf '[core]\\n' > \"$(git rev-parse --git-dir)/config.worktree\" ) && echo creo=ok\n")
	c := cdwCorrer(t, ca, v, 0, 60*time.Second, b.String())
	if strings.Contains(c.salida, "ESCRIBIO") || strings.Contains(c.salida, "CREO pre-commit") {
		t.Errorf("%s: escribir el material que existe falla dentro del encierro:\n%s", ca, c.salida)
	}
	for _, p := range piezas {
		if got, _ := cdwLeer(p); got != antes[p] {
			t.Errorf("%s: %s sigue como estaba: %q", ca, p, got)
		}
	}
	if _, err := os.Stat(filepath.Join(comun, "hooks", "pre-commit")); err == nil {
		t.Errorf("%s: la carpeta de hooks que existe no se escribe: quedo hooks/pre-commit", ca)
	}
	if !strings.Contains(c.salida, "commit=ok") {
		t.Fatalf("%s: un git commit del writer en hoom/demo funciona:\n%s", ca, c.salida)
	}
	out, err := exec.Command("git", "-C", wt, "log", "-1", "--format=%s", "hoom/demo").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "del writer" {
		t.Fatalf("%s: el commit queda en hoom/demo: %q %v", ca, out, err)
	}
	if !strings.Contains(c.salida, "creo=ok") {
		t.Fatalf("%s: crear una pieza del material que no existia no se bloquea (la detecta el gate):\n%s", ca, c.salida)
	}
}
