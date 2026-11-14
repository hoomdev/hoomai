//go:build linux

// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre el
// sobre (agentcmd) en Linux con encierro: escrituras (CA-678, CA-681),
// procesos (CA-679), lo que el CLI ve (CA-680), caches (CA-682), los gates
// del sobre (CA-683, CA-687), secretos (CA-694) y donde escribe cada rol
// (CA-695). Sin bwrap fallan, no se saltan (aclaracion 1). Los resultados que
// el CLI falso necesita contar los deja en una ruta de `escribibles`.
package agentcmd

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
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// cdwlEncierro exige bwrap y un nivel distinto de ninguno en esta maquina:
// sin eso el test falla (aclaracion 1).
func cdwlEncierro(t *testing.T, ca string) encierro.Info {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Fatalf("%s: bwrap no esta en el PATH: los tests del encierro fallan sin el (%v)", ca, err)
	}
	info := encierro.Nivel(true, cdwProv(t, "claude"))
	if info.Nivel == encierro.NivelNinguno {
		t.Fatalf("%s: este sistema no tiene encierro (%s): los tests del encierro fallan sin el", ca, info.Motivo)
	}
	return info
}

// cdwlDir es un directorio temporal sin symlinks.
func cdwlDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// cdwlProyecto es un proyecto con la seccion encierro (exigir no), con
// salida (un directorio de afuera) en escribibles, y extra debajo.
func cdwlProyecto(t *testing.T, ca, extra string) (root, wt, salida string) {
	t.Helper()
	cdwlEncierro(t, ca)
	salida = cdwlDir(t)
	root, wt = cdwProyecto(t, "encierro:\n  exigir: no\n  escribibles:\n    - "+salida+"\n"+extra)
	return root, wt, salida
}

func cdwlLeer(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("el CLI falso dejo %s: %v", p, err)
	}
	return string(raw)
}

func cdwlToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	tok := "cdwtok" + hex.EncodeToString(b)
	t.Cleanup(func() { cdwlMatar(tok) })
	return tok
}

// cdwlVivos son los pids cuyo argv contiene tok.
func cdwlVivos(tok string) []int {
	var out []int
	es, _ := os.ReadDir("/proc")
	for _, e := range es {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if strings.Contains(string(raw), tok) {
			out = append(out, pid)
		}
	}
	return out
}

func cdwlMatar(tok string) {
	for _, pid := range cdwlVivos(tok) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// cdwlNadieVivo exige que, a lo sumo en 3 s, no quede vivo ningun proceso con
// tok en su argv.
func cdwlNadieVivo(t *testing.T, ca, caso, tok string) {
	t.Helper()
	limite := time.Now().Add(3 * time.Second)
	for {
		vivos := cdwlVivos(tok)
		if len(vivos) == 0 {
			return
		}
		if time.Now().After(limite) {
			cdwlMatar(tok)
			t.Fatalf("%s: %s: no queda vivo ningun proceso que lanzo el CLI; quedaron %v", ca, caso, vivos)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// cdwlEscapes deja tres procesos que se escapan del grupo del CLI (setsid,
// doble fork y nohup), cada uno con tok en su argv (un sh que no hace exec).
func cdwlEscapes(tok string) string {
	bucle := "'while :; do sleep 1; done'"
	return "setsid sh -c " + bucle + " " + tok + "-setsid </dev/null >/dev/null 2>&1 &\n" +
		"sh -c \"sh -c " + bucle + " " + tok + "-doble </dev/null >/dev/null 2>&1 &\" </dev/null >/dev/null 2>&1\n" +
		"nohup sh -c " + bucle + " " + tok + "-nohup </dev/null >/dev/null 2>&1 &\n" +
		"sleep 0.3\n"
}

// cdwlTrabajo es lo que cada rol escribe para que el sobre tenga algo
// propio que hacer (sin exit).
func cdwlTrabajo(rol string) string {
	switch rol {
	case "writer":
		return ar1Codigo
	case "test-writer":
		return strings.TrimSuffix(aeaEscribeTW, "exit 0\n")
	}
	return ""
}

// ---------------------------------------------------------------- CA-678

// CA-678 y CA-690: el writer encerrado con el provider falso (ConEstado)
// escribe en su arbol, en su cache, en una ruta de Escribe y en una de
// escribibles; lo que intenta escribir en $HOME, en hoom.yaml y en
// .hoom/items/<slug>.yaml del principal, en el worktree de otra tarea o en
// /tmp fuera de su arbol no deja nada.
func TestCA678_CA690_ElWriterEncerradoSoloEscribeDondePuede(t *testing.T) {
	const ca = "CA-678"
	root, wt, salida := cdwlProyecto(t, ca, "")
	home := os.Getenv("HOME")
	if err := taskcmd.Start(root, "otra", "main"); err != nil {
		t.Fatalf("fixture: la otra tarea: %v", err)
	}
	otra := filepath.Join(root, ".hoom", "worktrees", "otra")
	sesiones := filepath.Join(home, ".falso-cdw", "sesiones")
	if err := os.MkdirAll(sesiones, 0o755); err != nil {
		t.Fatal(err)
	}
	fuera := cdwlDir(t)
	items := filepath.Join(root, ".hoom", "items", aeaTarea+".yaml")
	itemAntes, itemErr := os.ReadFile(items)
	yamlAntes := cdwlLeer(t, filepath.Join(root, "hoom.yaml"))
	nuevoTmp := "/tmp/" + cdwlToken(t)
	script := ar1Codigo +
		"printf x > \"$HOME/intruso\" 2>/dev/null\n" +
		"printf x >> " + qcComillas(filepath.Join(root, "hoom.yaml")) + " 2>/dev/null\n" +
		"mkdir -p " + qcComillas(filepath.Dir(items)) + " 2>/dev/null\n" +
		"printf x >> " + qcComillas(items) + " 2>/dev/null\n" +
		"printf x > " + qcComillas(filepath.Join(otra, "intruso.go")) + " 2>/dev/null\n" +
		"printf x > " + qcComillas(filepath.Join(fuera, "intruso")) + " 2>/dev/null\n" +
		"printf x > " + qcComillas(nuevoTmp) + " 2>/dev/null\n" +
		"printf x > \"$TMPDIR/en-la-cache\"\n" +
		"printf x > " + qcComillas(filepath.Join(sesiones, "s.json")) + "\n" +
		"printf x > " + qcComillas(filepath.Join(salida, "en-escribibles")) + "\n" +
		"exit 0\n"
	res, out := cdwCorrer(t, root, "writer", aeaTarea, script, Options{Provider: cdwConEstadoNombre})
	got, _ := cdwClave(t, root, res.EnvelopeID)
	if got == nil || got["nivel"] == encierro.NivelNinguno {
		t.Fatalf("%s: el writer con un provider con ConEstado corre encerrado: %v\n%s", ca, got, out)
	}
	for _, p := range []string{filepath.Join(home, "intruso"), filepath.Join(otra, "intruso.go"), filepath.Join(fuera, "intruso"), nuevoTmp} {
		if cdwExiste(p) {
			t.Errorf("%s: lo que el CLI encerrado intenta escribir en %s no deja nada", ca, p)
		}
	}
	if cdwLeerOVacio(filepath.Join(root, "hoom.yaml")) != yamlAntes {
		t.Errorf("%s: el hoom.yaml del principal no cambia", ca)
	}
	if itemAhora, err := os.ReadFile(items); (itemErr == nil) != (err == nil) || string(itemAhora) != string(itemAntes) {
		t.Errorf("%s: .hoom/items/%s.yaml del principal no cambia", ca, aeaTarea)
	}
	cache := encierro.Cache(root, aeaTarea, "writer")
	if !cdwHayAdentro(cache, "en-la-cache") {
		t.Errorf("%s: lo que escribe en $TMPDIR (en %s) queda", ca, cache)
	}
	for _, p := range []string{filepath.Join(wt, "app.go"), filepath.Join(sesiones, "s.json"), filepath.Join(salida, "en-escribibles")} {
		if !cdwExiste(p) {
			t.Errorf("%s: lo que escribe en %s queda", ca, p)
		}
	}
	if !strings.Contains(cdwLeerOVacio(filepath.Join(wt, "app.go")), "implementado") {
		t.Errorf("%s: lo que escribe en el arbol de la tarea queda", ca)
	}
}

func cdwLeerOVacio(p string) string {
	raw, _ := os.ReadFile(p)
	return string(raw)
}

// cdwHayAdentro dice si hay un archivo nombre en algun lugar bajo dir.
func cdwHayAdentro(dir, nombre string) bool {
	hay := false
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() == nombre {
			hay = true
		}
		return nil
	})
	return hay
}

// ---------------------------------------------------------------- CA-681

// CA-681: dentro del encierro, el material de git que existe es de solo
// lectura para el writer; su git commit en hoom/<slug> funciona.
func TestCA681_ElMaterialQueExisteNoSeEscribeYElCommitFunciona(t *testing.T) {
	const ca = "CA-681"
	root, wt, salida := cdwlProyecto(t, ca, "")
	comun := cdwComun(t, wt)
	hooks := ar6Ruta(t, wt, "--git-path", "hooks")
	ar6Escribir(t, filepath.Join(hooks, "pre-push"), "#!/bin/sh\nexit 0\n", 0o755)
	cdwEscribir(t, filepath.Join(comun, "info", "attributes"), "*.txt diff\n")
	cdwEscribir(t, filepath.Join(comun, "info", "exclude"), "# nada\n")
	cdwEscribir(t, filepath.Join(comun, "objects", "info", "alternates"), "")
	piezas := map[string]string{
		"hooks/pre-push": filepath.Join(hooks, "pre-push"), "config": filepath.Join(comun, "config"),
		"info/attributes": filepath.Join(comun, "info", "attributes"), "info/exclude": filepath.Join(comun, "info", "exclude"),
		"objects/info/alternates": filepath.Join(comun, "objects", "info", "alternates"), ".git": filepath.Join(wt, ".git"),
	}
	antes := map[string]string{}
	script := ar1Codigo
	for n, p := range piezas {
		antes[n] = cdwLeerOVacio(p)
		script += "if printf '\\n# intruso\\n' >> " + qcComillas(p) + " 2>/dev/null; then echo " + qcComillas(n+" escribio") + "; else echo " + qcComillas(n+" fallo") + "; fi >> " + qcComillas(filepath.Join(salida, "piezas")) + "\n"
	}
	script += "touch nuevo.go\ngit add app.go nuevo.go && git commit -q -m 'commit del writer' && echo commit-ok >> " + qcComillas(filepath.Join(salida, "piezas")) + "\nexit 0\n"
	head := cdwGitOut(t, wt, "rev-parse", "HEAD")
	res, out := cdwCorrer(t, root, "writer", aeaTarea, script, Options{})
	piezasOut := cdwlLeer(t, filepath.Join(salida, "piezas"))
	for n, p := range piezas {
		if !strings.Contains(piezasOut, n+" fallo") || cdwLeerOVacio(p) != antes[n] {
			t.Errorf("%s: escribir %s (%s) falla dentro del encierro:\n%s", ca, n, p, piezasOut)
		}
	}
	if !strings.Contains(piezasOut, "commit-ok") || cdwGitOut(t, wt, "rev-parse", "HEAD") == head {
		t.Fatalf("%s: el git commit del writer en hoom/%s funciona:\n%s\n%+v\n%s", ca, aeaTarea, piezasOut, res, out)
	}
	if res.Scope.Cuts() {
		if _, ok := aeaViolacion(res.Scope, RuleMaterialDeGit, ""); ok {
			t.Fatalf("%s: intentar y no poder no cambia el material: %+v", ca, res.Scope.Violations)
		}
	}
}

// CA-681: crear una pieza del material que no existia (info/attributes) es
// material-de-git, tambien con encierro.
func TestCA681_CrearUnaPiezaQueNoExistiaEsMaterialDeGit(t *testing.T) {
	const ca = "CA-681"
	root, wt, _ := cdwlProyecto(t, ca, "")
	comun := cdwComun(t, wt)
	ruta := filepath.Join(comun, "info", "attributes")
	if cdwExiste(ruta) {
		t.Fatalf("%s: fixture: info/attributes no existe", ca)
	}
	antes := aeaVeredictos(root, wt)
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"mkdir -p "+qcComillas(filepath.Dir(ruta))+" 2>/dev/null\nprintf '*.go diff\\n' > "+qcComillas(ruta)+" 2>/dev/null\nexit 0\n", Options{})
	if !cdwExiste(ruta) {
		t.Logf("%s: el encierro no dejo crear %s: nada que cortar (lo que el CA pide si se crea)", ca, ruta)
		return
	}
	ar6Cortado(t, ca, "crea info/attributes encerrado", root, wt, res, out, antes, ruta)
}

// ---------------------------------------------------------------- CA-679

// CA-679: el CLI de cada rol que ejecuta deja un proceso con setsid, otro con
// doble fork y otro con nohup, y termina: cuando cierra el sobre no queda
// ninguno vivo.
func TestCA679_NingunRolDejaProcesosVivos(t *testing.T) {
	const ca = "CA-679"
	for _, rol := range cdwRolesQueEjecutan {
		root, _, _ := cdwlProyecto(t, ca, "")
		tok := cdwlToken(t)
		res, out := cdwCorrer(t, root, rol, aeaTarea, cdwlEscapes(tok)+cdwlTrabajo(rol)+"exit 0\n", Options{})
		if res.RunID == "" {
			t.Fatalf("%s: %s: fixture: el run corrio: %+v\n%s", ca, rol, res, out)
		}
		cdwlNadieVivo(t, ca, rol, tok)
	}
}

// CA-679: cuando el run muere por su timeout tampoco queda ninguno vivo, y
// el registro dice corte tiempo.
func TestCA679_AlMorirPorTimeoutNoQuedaNadie(t *testing.T) {
	const ca = "CA-679"
	root, _, _ := cdwlProyecto(t, ca, "")
	tok := cdwlToken(t)
	inicio := time.Now()
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+cdwlEscapes(tok)+"sleep 120\nexit 0\n", Options{Timeout: 3 * time.Second})
	if time.Since(inicio) > 60*time.Second {
		t.Fatalf("%s: el timeout de 3 s corta el run: %v", ca, time.Since(inicio))
	}
	if r := aeaRegistro(t, root, res.EnvelopeID); r.Corte != envelope.CorteTiempo {
		t.Fatalf("%s: fixture: el run murio por su timeout: %+v\n%s", ca, r, out)
	}
	cdwlNadieVivo(t, ca, "timeout", tok)
}

// ---------------------------------------------------------------- CA-680

// CA-680: el CLI de cada rol que ejecuta no ve el pid de hoom ni puede
// senalarlo, no tiene terminal de control y es lider de su sesion, no recibe
// las variables de los agentes y la sesion, ve vacio el directorio que hace
// de /run/user/<uid>, y recibe la config de git del contrato.
func TestCA680_LoQueVeElCLIDeCadaRol(t *testing.T) {
	const ca = "CA-680"
	runUser := cdwlDir(t)
	cdwEscribir(t, filepath.Join(runUser, "bus"), "x")
	restaurar := encierro.ReemplazarRunUser(runUser)
	defer restaurar()
	quitadas := map[string]string{"SSH_AUTH_SOCK": "/tmp/agente-ssh", "GPG_AGENT_INFO": "/tmp/gpg:1:1",
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=/tmp/bus", "DISPLAY": ":0", "WAYLAND_DISPLAY": "wayland-0", "XDG_RUNTIME_DIR": runUser}
	if !cdwMismasClaves(quitadas, encierro.VariablesQuitadas) {
		t.Fatalf("%s: fixture: las variables del contrato son VariablesQuitadas: %q", ca, encierro.VariablesQuitadas)
	}
	hoom := strconv.Itoa(os.Getpid())
	for _, rol := range cdwRolesQueEjecutan {
		root, _, salida := cdwlProyecto(t, ca, "")
		for k, v := range quitadas {
			t.Setenv(k, v)
		}
		o := qcComillas(filepath.Join(salida, rol))
		script := "{\n" +
			"test -e /proc/" + hoom + " && echo ve-pid || echo no-ve-pid\n" +
			"kill -0 " + hoom + " 2>/dev/null && echo senala || echo no-senala\n" +
			"echo \"pid=$$\"\n" +
			"echo \"stat=$(sed 's/.*) //' /proc/$$/stat)\"\n" +
			"echo \"runuser=$(ls -A " + qcComillas(runUser) + " 2>/dev/null | wc -l)\"\n" +
			"env | cut -d= -f1 | sed 's/^/var=/'\n" +
			"echo \"gc.auto=$(git config --get gc.auto)\"\n" +
			"echo \"maintenance.auto=$(git config --get maintenance.auto)\"\n" +
			"echo \"core.fsmonitor=$(git config --get core.fsmonitor)\"\n" +
			"} > " + o + " 2>&1\n" + cdwlTrabajo(rol) + "exit 0\n"
		res, out := cdwCorrer(t, root, rol, aeaTarea, script, Options{})
		got := cdwlLeer(t, filepath.Join(salida, rol))
		lineas := strings.Split(got, "\n")
		tiene := func(l string) bool {
			for _, x := range lineas {
				if x == l {
					return true
				}
			}
			return false
		}
		for _, l := range []string{"no-ve-pid", "no-senala", "runuser=0", "gc.auto=0", "maintenance.auto=false", "core.fsmonitor=false"} {
			if !tiene(l) {
				t.Errorf("%s: %s: el CLI encerrado da %q:\n%s\n%+v\n%s", ca, rol, l, got, res, out)
			}
		}
		for k := range quitadas {
			if tiene("var=" + k) {
				t.Errorf("%s: %s: el CLI encerrado no recibe %s:\n%s", ca, rol, k, got)
			}
		}
		var pid, stat string
		for _, x := range lineas {
			if strings.HasPrefix(x, "pid=") {
				pid = strings.TrimPrefix(x, "pid=")
			}
			if strings.HasPrefix(x, "stat=") {
				stat = strings.TrimPrefix(x, "stat=")
			}
		}
		// despues de "<pid> (<comm>) ": estado ppid pgrp session tty_nr ...
		campos := strings.Fields(stat)
		if len(campos) < 5 || campos[4] != "0" || campos[3] != pid {
			t.Errorf("%s: %s: el CLI no tiene terminal de control (tty_nr 0) y es lider de su sesion (session == pid %s): %q", ca, rol, pid, stat)
		}
	}
}

func cdwMismasClaves(m map[string]string, xs []string) bool {
	if len(m) != len(xs) {
		return false
	}
	for _, x := range xs {
		if _, ok := m[x]; !ok {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- CA-682

// CA-682: el run del writer recibe TMPDIR y XDG_CACHE_HOME dentro de su
// cache (que hoom crea), y lo que intenta escribir en la cache verify, en la
// de otro rol o en la guardado no deja nada.
func TestCA682_ElWriterSoloEscribeEnSuCache(t *testing.T) {
	const ca = "CA-682"
	root, _, salida := cdwlProyecto(t, ca, "")
	mia := encierro.Cache(root, aeaTarea, "writer")
	otras := []string{encierro.Cache(root, aeaTarea, encierro.QuienVerify), encierro.Cache(root, aeaTarea, "test-writer"),
		encierro.Cache(root, aeaTarea, encierro.QuienGuardado)}
	// una de las otras ya existe (la del verify de una vuelta anterior)
	if err := os.MkdirAll(otras[0], 0o755); err != nil {
		t.Fatal(err)
	}
	script := ar1Codigo + "{ echo \"tmpdir=$TMPDIR\"; echo \"xdg=$XDG_CACHE_HOME\"; } > " + qcComillas(filepath.Join(salida, "env")) + "\n"
	for i, d := range otras {
		script += "mkdir -p " + qcComillas(d) + " 2>/dev/null; printf x > " + qcComillas(filepath.Join(d, "intruso"+strconv.Itoa(i))) + " 2>/dev/null\n"
	}
	res, out := cdwCorrer(t, root, "writer", aeaTarea, script+"exit 0\n", Options{})
	env := cdwlLeer(t, filepath.Join(salida, "env"))
	for _, v := range []string{"tmpdir=", "xdg="} {
		var val string
		for _, l := range strings.Split(env, "\n") {
			if strings.HasPrefix(l, v) {
				val = strings.TrimPrefix(l, v)
			}
		}
		if val == "" || !(val == mia || strings.HasPrefix(val, mia+string(filepath.Separator))) {
			t.Errorf("%s: %s esta dentro de %s: %q\n%+v\n%s", ca, v, mia, val, res, out)
		}
	}
	if st, err := os.Stat(mia); err != nil || !st.IsDir() {
		t.Errorf("%s: hoom crea la cache del writer %s: %v", ca, mia, err)
	}
	for i, d := range otras {
		if cdwExiste(filepath.Join(d, "intruso"+strconv.Itoa(i))) {
			t.Errorf("%s: lo que el writer intenta escribir en %s no deja nada", ca, d)
		}
	}
}

// ---------------------------------------------------------------- CA-683

// CA-683: los gates del verify del sobre corren encerrados: uno que escribe
// en $HOME no deja nada afuera y uno que deja un proceso con setsid no lo
// deja vivo; el gate de gates_sin_encierro corre como hoy.
func TestCA683_LosGatesDelSobreCorrenEncerrados(t *testing.T) {
	const ca = "CA-683"
	cdwlEncierro(t, ca)
	for _, sinEncierro := range []bool{false, true} {
		tok := cdwlToken(t)
		cdwHome(t)
		home := os.Getenv("HOME")
		gate := "printf x > $HOME/gate-escribio 2>/dev/null; setsid sh -c 'while :; do sleep 1; done' " + tok +
			" </dev/null >/dev/null 2>&1 & sleep 0.3; true"
		extra := "encierro:\n  exigir: no\n"
		if sinEncierro {
			extra += "  gates_sin_encierro:\n    - test\n"
		}
		root, _ := aeaProyecto(t, gate, extra)
		res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{})
		if res.VerdictID == "" {
			t.Fatalf("%s: fixture: el gate del sobre corrio: %+v\n%s", ca, res, out)
		}
		if sinEncierro {
			if !cdwExiste(filepath.Join(home, "gate-escribio")) {
				t.Fatalf("%s: el gate de gates_sin_encierro corre como hoy y escribe en $HOME", ca)
			}
			cdwlMatar(tok)
			continue
		}
		if cdwExiste(filepath.Join(home, "gate-escribio")) {
			t.Fatalf("%s: el gate encerrado no deja nada en $HOME", ca)
		}
		cdwlNadieVivo(t, ca, "gate encerrado", tok)
	}
}

// CA-687 y CA-681 con la seccion: el gate de gates_sin_encierro que crea
// info/attributes deja material_de_git en fail; encerrado, no puede cambiar
// el que existe.
func TestCA687_ElGateSinEncierroQueCambiaElMaterial(t *testing.T) {
	const ca = "CA-687"
	cdwlEncierro(t, ca)
	gate := "c=$(git rev-parse --git-common-dir); mkdir -p $c/info 2>/dev/null; printf x > $c/info/attributes 2>/dev/null; true"
	for _, sinEncierro := range []bool{true, false} {
		cdwHome(t)
		extra := "encierro:\n  exigir: no\n"
		if sinEncierro {
			extra += "  gates_sin_encierro:\n    - test\n"
		}
		root, wt := aeaProyecto(t, gate, extra)
		ruta := filepath.Join(cdwComun(t, wt), "info", "attributes")
		if !sinEncierro {
			cdwEscribir(t, ruta, "*.go diff\n")
		}
		res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{})
		g := cdwGateDe(t, res.VerdictID, "material_de_git", wt, root)
		if sinEncierro {
			if g == nil || g.Status != verdict.StatusFail || !strings.Contains(g.OutputTail, ruta) || res.Stage != "verify" || res.ExitCode != 1 {
				t.Fatalf("%s: el gate de gates_sin_encierro que cambia el material deja material_de_git en fail con %s: %+v %+v\n%s", ca, ruta, g, res, out)
			}
			continue
		}
		if cdwLeerOVacio(ruta) != "*.go diff\n" || g != nil {
			t.Fatalf("%s: el gate encerrado no puede cambiar %s, que existe: %q %+v", ca, ruta, cdwLeerOVacio(ruta), g)
		}
	}
}

// cdwGateDe es el gate nombre del veredicto id, en cualquiera de los dirs.
func cdwGateDe(t *testing.T, id, nombre string, dirs ...string) *verdict.GateResult {
	t.Helper()
	for _, d := range dirs {
		all, err := verdict.LoadAll(d)
		if err != nil {
			continue
		}
		for i := range all {
			if all[i].ID != id {
				continue
			}
			for j := range all[i].Gates {
				if all[i].Gates[j].Name == nombre {
					g := all[i].Gates[j]
					return &g
				}
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- CA-694

// cdwlSecretos llena el $HOME del test con los secretos del contrato y un
// archivo que no es secreto.
func cdwlSecretos(t *testing.T, home string) map[string]string {
	t.Helper()
	archivos := map[string]string{
		".ssh/id_ed25519":         "clave privada",
		".gnupg/pubring.kbx":      "anillo",
		".config/gh/hosts.yml":    "token: gh",
		".aws/credentials":        "aws_secret_access_key = x",
		".docker/config.json":     `{"auths":{"x":{"auth":"y"}}}`,
		"no-es-secreto/leeme.txt": "se lee",
	}
	for rel, body := range archivos {
		cdwEscribir(t, filepath.Join(home, rel), body)
	}
	return archivos
}

const cdwlLeeSecretos = "for d in .ssh .gnupg .config/gh .aws; do echo \"$d=$(ls -A \"$HOME/$d\" 2>/dev/null | wc -l)\"; done\n" +
	"echo \"docker=$(cat \"$HOME/.docker/config.json\" 2>/dev/null | wc -c)\"\n" +
	"echo \"leeme=$(cat \"$HOME/no-es-secreto/leeme.txt\" 2>/dev/null)\"\n"

// CA-694: el CLI encerrado de cada rol que ejecuta ve vacios los directorios
// de secretos y vacio ~/.docker/config.json, no puede escribirlos, y lee el
// resto del $HOME; sin encierro se leen como hoy.
func TestCA694_ElCLIDeCadaRolNoVeLosSecretos(t *testing.T) {
	const ca = "CA-694"
	for _, rol := range cdwRolesQueEjecutan {
		root, _, salida := cdwlProyecto(t, ca, "")
		home := os.Getenv("HOME")
		archivos := cdwlSecretos(t, home)
		o := qcComillas(filepath.Join(salida, rol))
		script := "{\n" + cdwlLeeSecretos +
			"printf x > \"$HOME/.ssh/nuevo\" 2>/dev/null\n" +
			"printf x >> \"$HOME/.docker/config.json\" 2>/dev/null\n" +
			"printf x >> \"$HOME/.aws/credentials\" 2>/dev/null\n" +
			"} > " + o + " 2>&1\n" + cdwlTrabajo(rol) + "exit 0\n"
		res, out := cdwCorrer(t, root, rol, aeaTarea, script, Options{})
		got := cdwlLeer(t, filepath.Join(salida, rol))
		for _, l := range []string{".ssh=0", ".gnupg=0", ".config/gh=0", ".aws=0", "docker=0", "leeme=se lee"} {
			if !strings.Contains(got, l+"\n") {
				t.Errorf("%s: %s: el CLI encerrado da %q:\n%s\n%+v\n%s", ca, rol, l, got, res, out)
			}
		}
		if cdwExiste(filepath.Join(home, ".ssh", "nuevo")) {
			t.Errorf("%s: %s: no puede escribir en ~/.ssh", ca, rol)
		}
		for rel, body := range archivos {
			if cdwLeerOVacio(filepath.Join(home, rel)) != body {
				t.Errorf("%s: %s: %s queda como estaba", ca, rol, rel)
			}
		}
	}
	// sin encierro se leen como hoy
	root, _ := cdwProyecto(t, "")
	salida := cdwlDir(t)
	cdwlSecretos(t, os.Getenv("HOME"))
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"{\n"+cdwlLeeSecretos+"} > "+qcComillas(filepath.Join(salida, "sin"))+" 2>&1\nexit 0\n", Options{})
	got := cdwlLeer(t, filepath.Join(salida, "sin"))
	for _, l := range []string{".ssh=1", ".gnupg=1", ".config/gh=1", ".aws=1", "leeme=se lee"} {
		if !strings.Contains(got, l+"\n") {
			t.Errorf("%s: sin encierro, el CLI lee los secretos como hoy (%q):\n%s\n%+v\n%s", ca, l, got, res, out)
		}
	}
	if strings.Contains(got, "docker=0\n") {
		t.Errorf("%s: sin encierro ~/.docker/config.json se lee como hoy:\n%s", ca, got)
	}
}

// ---------------------------------------------------------------- CA-695

// CA-695: el CLI encerrado del test-writer escribe en su arbol ciego (lo que
// el trasplante lleva al arbol de la tarea), y lo que intenta escribir en el
// arbol de la tarea fuera de el no deja nada.
func TestCA695_ElTestWriterEscribeEnSuArbolCiego(t *testing.T) {
	const ca = "CA-695"
	root, wt, salida := cdwlProyecto(t, ca, "")
	intruso := filepath.Join(wt, "intruso_tw.go")
	script := "pwd > " + qcComillas(filepath.Join(salida, "pwd")) + "\n" +
		"printf 'package app\\n' > " + qcComillas(intruso) + " 2>/dev/null\n" + aeaEscribeTW
	res, out := cdwCorrer(t, root, "test-writer", aeaTarea, script, Options{})
	pwd := strings.TrimSpace(cdwlLeer(t, filepath.Join(salida, "pwd")))
	if pwd == wt || pwd == root {
		t.Fatalf("%s: el test-writer corre en su arbol ciego, no en %s: %+v\n%s", ca, pwd, res, out)
	}
	if cdwExiste(intruso) {
		t.Fatalf("%s: lo que el test-writer intenta escribir en el arbol de la tarea no deja nada", ca)
	}
	if !cdwExiste(filepath.Join(wt, aeaProtegido)) {
		t.Fatalf("%s: lo que escribe en su arbol ciego llega al arbol de la tarea por el trasplante: %+v\n%s", ca, res, out)
	}
}

// CA-695: el characterizer, el refutador, el orquestador con --task y el
// reviewer escriben en el arbol de la tarea y no en el checkout principal.
func TestCA695_LosOtrosRolesEscribenEnElArbolDeLaTarea(t *testing.T) {
	const ca = "CA-695"
	for _, rol := range []string{"characterizer", "refutador", "orquestador", "reviewer"} {
		root, wt, _ := cdwlProyecto(t, ca, "")
		nombre := "cdw_" + strings.ReplaceAll(rol, "-", "_") + "_test.go"
		res, out := cdwCorrer(t, root, rol, aeaTarea, "printf 'package app\\n' > "+nombre+"\nexit 0\n", Options{})
		if !cdwExiste(filepath.Join(wt, nombre)) || cdwExiste(filepath.Join(root, nombre)) {
			t.Fatalf("%s: %s: escribe en el arbol de la tarea (%v) y no en el principal (%v): %+v\n%s",
				ca, rol, cdwExiste(filepath.Join(wt, nombre)), cdwExiste(filepath.Join(root, nombre)), res, out)
		}
	}
}

// CA-695: un `hoom finding resolve` corrido desde el refutador encerrado deja
// su archivo en .hoom/findings/ del arbol de la tarea, como hoy.
func TestCA695_ElRefutadorResuelveEnElArbolDeLaTarea(t *testing.T) {
	const ca = "CA-695"
	bin := qcHoom(t)
	root, wt, salida := cdwlProyecto(t, ca, "")
	f, err := finding.Add(wt, "main", "high", "risk", "app.go", "el retry no respeta el backoff", "reviewer@codex")
	if err != nil {
		t.Fatal(err)
	}
	git(t, wt, "add", "-A")
	git(t, wt, "commit", "-q", "-m", "un hallazgo abierto")
	script := qcComillas(bin) + " finding resolve " + f.ID + " --as refutado --evidence 'es un duplicado del otro' > " +
		qcComillas(filepath.Join(salida, "resolve")) + " 2>&1\necho \"exit=$?\" >> " + qcComillas(filepath.Join(salida, "resolve")) + "\nexit 0\n"
	res, out := cdwCorrer(t, root, "refutador", aeaTarea, script, Options{})
	if !cdwExiste(filepath.Join(wt, qcResRel(f.ID))) || cdwExiste(filepath.Join(root, qcResRel(f.ID))) {
		t.Fatalf("%s: la resolucion queda en .hoom/findings/ del arbol de la tarea y no en el principal:\n%s\n%+v\n%s",
			ca, cdwLeerOVacio(filepath.Join(salida, "resolve")), res, out)
	}
}
