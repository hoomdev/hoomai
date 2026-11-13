//go:build linux

// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre el
// guardado del piloto en Linux con encierro (CA-684, CA-694, CA-682): cada
// git de SaveTarea corre encerrado, con la cache guardado, TopeGit y su
// gracia y commit.gpgsign=false; `hoom item save` (Save) no. Sin bwrap fallan
// (aclaracion 1). Lo que el hook cuenta lo deja en su $TMPDIR (la cache
// guardado, escribible adentro y legible desde el test).
package pilotocmd_test

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
	"github.com/hoomdev/hoomai/internal/itemcmd"
)

const cdwlSeccion = "encierro:\n  exigir: no\n"

func cdwlEncierro(t *testing.T, ca string) {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Fatalf("%s: bwrap no esta en el PATH: los tests del encierro fallan sin el (%v)", ca, err)
	}
	if info := encierro.Nivel(true, nil); info.Nivel == encierro.NivelNinguno {
		t.Fatalf("%s: este sistema no tiene encierro (%s)", ca, info.Motivo)
	}
}

func cdwlToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	tok := "cdwptok" + hex.EncodeToString(b)
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

func cdwlNadieVivo(t *testing.T, ca, tok string) {
	t.Helper()
	limite := time.Now().Add(3 * time.Second)
	for len(cdwlVivos(tok)) > 0 {
		if time.Now().After(limite) {
			t.Fatalf("%s: no queda vivo ningun proceso que lanzo el hook: %v", ca, cdwlVivos(tok))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// cdwlBuscar es el contenido del archivo nombre bajo dir ("" si no esta).
func cdwlBuscar(dir, nombre string) string {
	var out string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() == nombre {
			raw, _ := os.ReadFile(p)
			out = string(raw)
		}
		return nil
	})
	return out
}

// CA-684, CA-694 y CA-682: con la seccion, el hook del guardado corre
// encerrado: lo que escribe en $HOME no deja nada, el proceso que deja con
// setsid no queda vivo, ve vacios los secretos y lee el resto del $HOME, y
// su $TMPDIR esta en la cache guardado; el guardado guarda.
func TestCA684_ElHookDelGuardadoCorreEncerrado(t *testing.T) {
	const ca = "CA-684"
	cdwlEncierro(t, ca)
	apPATH(t)
	home := cdwHome(t)
	for rel, body := range map[string]string{".ssh/id_ed25519": "clave", ".gnupg/pubring.kbx": "anillo",
		".config/gh/hosts.yml": "token", ".aws/credentials": "secreto", ".docker/config.json": `{"auths":{}}`,
		"no-es-secreto/leeme.txt": "se lee"} {
		apEscribir(t, home, rel, body)
	}
	tok := cdwlToken(t)
	root, wt := cdwTarjeta(t, cdwlSeccion, "encerrado")
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\n"+
		"printf x > \"$HOME/hook-escribio\" 2>/dev/null\n"+
		"setsid sh -c 'while :; do sleep 1; done' "+tok+" </dev/null >/dev/null 2>&1 &\n"+
		"{\n"+
		"for d in .ssh .gnupg .config/gh .aws; do echo \"$d=$(ls -A \"$HOME/$d\" 2>/dev/null | wc -l)\"; done\n"+
		"echo \"docker=$(cat \"$HOME/.docker/config.json\" 2>/dev/null | wc -c)\"\n"+
		"echo \"leeme=$(cat \"$HOME/no-es-secreto/leeme.txt\" 2>/dev/null)\"\n"+
		"echo \"tmpdir=$TMPDIR\"\n"+
		"} > \"$TMPDIR/cdw-hook.txt\"\nsleep 0.3\nexit 0\n")
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "encerrado")
	if err != nil || len(res.Commits) != 1 {
		t.Fatalf("%s: el guardado encerrado guarda: %+v %v", ca, res, err)
	}
	if _, err := os.Stat(filepath.Join(home, "hook-escribio")); err == nil {
		t.Errorf("%s: lo que el hook encerrado escribe en $HOME no deja nada", ca)
	}
	cdwlNadieVivo(t, ca, tok)
	cache := encierro.Cache(root, "encerrado", encierro.QuienGuardado)
	got := cdwlBuscar(cache, "cdw-hook.txt")
	if got == "" {
		t.Fatalf("CA-682: el hook escribe en su $TMPDIR, dentro de la cache guardado %s", cache)
	}
	for _, l := range []string{".ssh=0", ".gnupg=0", ".config/gh=0", ".aws=0", "docker=0", "leeme=se lee"} {
		if !strings.Contains(got, l+"\n") {
			t.Errorf("CA-694: el hook encerrado da %q:\n%s", l, got)
		}
	}
	if !strings.Contains(got, "tmpdir="+cache) {
		t.Errorf("CA-682: el $TMPDIR del hook esta en la cache guardado %s:\n%s", cache, got)
	}
}

// CA-684: con commit.gpgsign=true en el repo, el guardado encerrado guarda
// sin firmar: el gpg.program del repo no corre.
func TestCA684_ElGuardadoEncerradoNoFirma(t *testing.T) {
	const ca = "CA-684"
	cdwlEncierro(t, ca)
	apPATH(t)
	cdwHome(t)
	root, wt := cdwTarjeta(t, cdwlSeccion, "firma")
	marca := filepath.Join(t.TempDir(), "gpg-corrio")
	gpg := filepath.Join(t.TempDir(), "gpg-falso")
	if err := os.WriteFile(gpg, []byte("#!/bin/sh\n: > '"+marca+"'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	apGit(t, wt, "config", "commit.gpgsign", "true")
	apGit(t, wt, "config", "gpg.program", gpg)
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "firma")
	if err != nil || len(res.Commits) != 1 {
		t.Fatalf("%s: el guardado encerrado corre con commit.gpgsign=false y guarda: %+v %v", ca, res, err)
	}
	if apr1Existe(marca) {
		t.Fatalf("%s: el gpg.program del repo no corre en el guardado", ca)
	}
}

// CA-684 y CA-679: un hook que pasa TopeGit recibe SIGTERM, tiene su gracia
// (deja su marca), termina con todo lo que lanzo, no queda index.lock y el
// guardado falla sin commitear.
func TestCA684_ElHookQuePasaElTopeRecibeSIGTERMYSuGracia(t *testing.T) {
	const ca = "CA-684"
	cdwlEncierro(t, ca)
	tope := itemcmd.TopeGit
	itemcmd.TopeGit = time.Second
	t.Cleanup(func() { itemcmd.TopeGit = tope })
	apPATH(t)
	cdwHome(t)
	tok := cdwlToken(t)
	root, wt := cdwTarjeta(t, cdwlSeccion, "tope")
	cache := encierro.Cache(root, "tope", encierro.QuienGuardado)
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\n"+
		"trap 'echo term > \"$TMPDIR/cdw-sigterm\"; exit 1' TERM\n"+
		"setsid sh -c 'while :; do sleep 1; done' "+tok+" </dev/null >/dev/null 2>&1 &\n"+
		"echo arranco > \"$TMPDIR/cdw-arranco\"\n"+ // con contenido: cdwlBuscar da "" si no esta
		"while :; do sleep 0.1; done\n")
	head := apHead(t, wt)
	lock := apGit(t, wt, "rev-parse", "--git-path", "index.lock")
	if !filepath.IsAbs(lock) {
		lock = filepath.Join(wt, lock)
	}
	listo := make(chan error, 1)
	go func() {
		_, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "tope")
		listo <- err
	}()
	var err error
	select {
	case err = <-listo:
	case <-time.After(30 * time.Second):
		t.Fatalf("%s: con TopeGit en 1 s el guardado termina en un tiempo acotado", ca)
	}
	if err == nil {
		t.Fatalf("%s: el guardado vencido falla", ca)
	}
	if cdwlBuscar(cache, "cdw-arranco") == "" {
		t.Fatalf("%s: fixture: el hook encerrado arranco (su $TMPDIR en %s): %v", ca, cache, err)
	}
	if cdwlBuscar(cache, "cdw-sigterm") == "" {
		t.Fatalf("%s: al vencer el tope el hook recibe SIGTERM y tiene su gracia: %v", ca, err)
	}
	cdwlNadieVivo(t, ca, tok)
	if apr1Existe(lock) {
		t.Fatalf("%s: no queda index.lock", ca)
	}
	if h := apHead(t, wt); h != head {
		t.Fatalf("%s: el guardado que fallo no commitea: %s -> %s", ca, head, h)
	}
}

// CA-684: `hoom item save` (Save) no corre encerrado: su hook escribe en
// $HOME como hoy, aunque el proyecto tenga la seccion.
func TestCA684_HoomItemSaveNoCorreEncerrado(t *testing.T) {
	const ca = "CA-684"
	cdwlEncierro(t, ca)
	apPATH(t)
	home := cdwHome(t)
	root := apProyecto(t, cdwlSeccion)
	const slug = "a-mano"
	apItem(t, root, slug, apSinAuto)
	apr4Hook(t, root, "pre-commit", "#!/bin/sh\nprintf x > \"$HOME/save-escribio\"\nexit 0\n")
	apEscribir(t, root, ".hoom/items/"+slug+".yaml", strings.Replace(apItemTexto(slug, apSinAuto), "prioridad: media", "prioridad: alta", 1))
	res, err := itemcmd.Save(root, apBase, apBlockOn, slug, nil)
	if err != nil || len(res.Commits) == 0 {
		t.Fatalf("%s: fixture: hoom item save guarda la tarjeta: %+v %v", ca, res, err)
	}
	if !apr1Existe(filepath.Join(home, "save-escribio")) {
		t.Fatalf("%s: el hook de hoom item save corre como hoy y escribe en $HOME", ca)
	}
}
