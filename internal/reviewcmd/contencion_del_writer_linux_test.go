//go:build linux

// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre la
// review encerrada en Linux: ningun proceso de una pasada sobrevive
// (CA-679) y la pasada escribe en el arbol de la tarea, donde `hoom finding
// add` deja sus hallazgos (CA-695). Sin bwrap fallan (aclaracion 1). Este
// archivo no escribe shebangs.
package reviewcmd

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
)

func cdwlEncierro(t *testing.T, ca string) {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Fatalf("%s: bwrap no esta en el PATH: los tests del encierro fallan sin el (%v)", ca, err)
	}
	if info := encierro.Nivel(true, cdwProv(t, "claude")); info.Nivel == encierro.NivelNinguno {
		t.Fatalf("%s: este sistema no tiene encierro (%s)", ca, info.Motivo)
	}
}

func cdwlToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	tok := "cdwrtok" + hex.EncodeToString(b)
	t.Cleanup(func() { cdwlMatar(tok) })
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

func cdwlMatar(tok string) {
	for _, pid := range cdwlVivos(tok) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// CA-679: una pasada encerrada que deja un proceso con setsid y otro con
// doble fork no deja ninguno vivo cuando cierra la review.
func TestCA679_NingunaPasadaDejaProcesosVivos(t *testing.T) {
	const ca = "CA-679"
	cdwlEncierro(t, ca)
	root, _, opt := cdwTarea(t, "encierro:\n  exigir: no\n")
	tok := cdwlToken(t)
	bucle := "'while :; do sleep 1; done'"
	fakeProvider(t, "claude", "setsid sh -c "+bucle+" "+tok+"-setsid </dev/null >/dev/null 2>&1 &\n"+
		"sh -c \"sh -c "+bucle+" "+tok+"-doble </dev/null >/dev/null 2>&1 &\" </dev/null >/dev/null 2>&1\n"+
		"sleep 0.3\nexit 0\n")
	res, out, err := cdwRevisar(t, root, opt)
	if err != nil || len(res.Passes) == 0 {
		t.Fatalf("%s: fixture: la review corrio: %v %+v\n%s", ca, err, res, out)
	}
	limite := time.Now().Add(3 * time.Second)
	for len(cdwlVivos(tok)) > 0 {
		if time.Now().After(limite) {
			vivos := cdwlVivos(tok)
			cdwlMatar(tok)
			t.Fatalf("%s: al cerrar la review no queda vivo ningun proceso de la pasada; quedaron %v", ca, vivos)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// CA-695: la pasada encerrada corre en el arbol de la tarea, y un `hoom
// finding add` corrido desde ella (con el HoomBin de la review) deja su
// archivo en .hoom/findings/ del arbol de la tarea y no en el principal.
func TestCA695_LaPasadaRegistraSuHallazgoEnElArbolDeLaTarea(t *testing.T) {
	const ca = "CA-695"
	cdwlEncierro(t, ca)
	bin := qcHoom(t)
	root, wt, opt := cdwTarea(t, "encierro:\n  exigir: no\n")
	opt.HoomBin = bin
	fakeProvider(t, "claude", "printf x > cdw-escribio-la-pasada.txt\n"+
		"'"+bin+"' finding add --sev low --lens risk --file app.go --author reviewer@claude 'EnLaTarea no tiene test (app.go:3)' >/dev/null 2>&1\nexit 0\n")
	res, out, err := cdwRevisar(t, root, opt)
	// que la review acepte o no un arbol que la pasada ensucio no es de este
	// CA: lo que cuenta es donde cayo lo escrito
	t.Logf("%s: la review termino: %v exit %d", ca, err, res.ExitCode)
	if !cdwExiste(filepath.Join(wt, "cdw-escribio-la-pasada.txt")) || cdwExiste(filepath.Join(root, "cdw-escribio-la-pasada.txt")) {
		t.Fatalf("%s: la pasada escribe en el arbol de la tarea y no en el principal:\n%s", ca, out)
	}
	enArbol, _ := filepath.Glob(filepath.Join(wt, ".hoom", "findings", "*.json"))
	enRaiz, _ := filepath.Glob(filepath.Join(root, ".hoom", "findings", "*.json"))
	if len(enArbol) == 0 || len(enRaiz) != 0 {
		t.Fatalf("%s: el hallazgo de la pasada queda en .hoom/findings/ del arbol de la tarea (%q) y no en el principal (%q):\n%s", ca, enArbol, enRaiz, out)
	}
}
