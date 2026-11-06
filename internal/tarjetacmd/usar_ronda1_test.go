//go:build darwin || linux

// Test adversarial de la ronda 1 de la review de verbos-de-la-tarjeta:
// hallazgo 5672b8 (CA-500). hoom usar <slug> que no puede escribir
// .hoom/cache/tarjeta (un error de E/S a mitad de la escritura) no pierde la
// tarjeta que ya estaba fijada: la fijada sigue siendo la de antes, entera.
//
// El fallo se provoca con RLIMIT_FSIZE, como historia_hallazgos3_unix_test.go
// de itemcmd: Go ignora SIGXFSZ, asi que la escritura que pasa el tope falla
// con EFBIG ("file too large"), un error de E/S de verdad como el del disco
// lleno. El limite es de todo el proceso, asi que nunca se baja en el de 'go
// test': el test se re-ejecuta a si mismo en un proceso hijo, que baja el
// limite, corre hoom usar y reporta por stdout; el padre mira el disco.
// Todo identificador de este archivo lleva el prefijo u1.
package tarjetacmd_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	// u1Tope es el limite de tamano de archivo del hijo: "precios\n" entra,
	// la linea del slug largo no.
	u1Tope = 24
	// u1Largo es la tarjeta que el hijo intenta fijar (su linea pasa u1Tope).
	u1Largo  = "otra-tarjeta-con-un-nombre-bastante-largo"
	u1EnvDir = "HOOM_TEST_U1_5672B8_DIR"
	u1Marca  = "U1-5672B8-RESULTADO: "
	u1Test   = "TestHallazgo_5672b8_CA500_UsarQueNoPuedeEscribirConservaLaFijada"
)

type u1Reporte struct {
	Code int    `json:"code"`
	Out  string `json:"out"`
	Err  string `json:"err"`
}

// u1Hijo corre hoom usar u1Largo desde dir con el tope puesto, lo restaura y
// reporta.
func u1Hijo(t *testing.T, dir string) {
	var antes syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
		t.Fatal(err)
	}
	if antes.Cur < u1Tope {
		t.Fatalf("el limite de tamano de archivo del proceso (%d) ya es menor que %d", antes.Cur, u1Tope)
	}
	nuevo := antes
	nuevo.Cur = u1Tope
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &nuevo); err != nil {
		t.Fatal(err)
	}
	s := vdtCorrer(t, "hallazgo 5672b8", vdtOpts{dir: dir}, "usar", u1Largo)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
		t.Fatalf("no pude restaurar RLIMIT_FSIZE: %v", err)
	}
	raw, err := json.Marshal(u1Reporte{Code: s.code, Out: s.out, Err: s.err})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s%s\n", u1Marca, raw)
}

func u1CorrerHijo(t *testing.T, dir string) u1Reporte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+u1Test+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), u1EnvDir+"="+dir)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var rep *u1Reporte
	sc := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	for sc.Scan() {
		if l, ok := strings.CutPrefix(sc.Text(), u1Marca); ok {
			var r u1Reporte
			if jerr := json.Unmarshal([]byte(l), &r); jerr != nil {
				t.Fatalf("el reporte del hijo es JSON: %v\n%s", jerr, l)
			}
			rep = &r
		}
	}
	if rep == nil || err != nil {
		t.Fatalf("el proceso hijo corre y reporta: %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errb.String())
	}
	return *rep
}

// hallazgo 5672b8, CA-500: hoom usar <slug> que falla al escribir
// .hoom/cache/tarjeta no deja la fijada vacia ni a medias: sigue la de antes,
// y hoom usar la muestra; el verbo no dice que fijo la nueva.
func TestHallazgo_5672b8_CA500_UsarQueNoPuedeEscribirConservaLaFijada(t *testing.T) {
	if dir := os.Getenv(u1EnvDir); dir != "" {
		u1Hijo(t, dir)
		return
	}
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	vdtItem(t, root, u1Largo, "")
	vdtExit(t, "hallazgo 5672b8", vdtCorrer(t, "hallazgo 5672b8", vdtOpts{dir: root}, "usar", "precios"), 0)
	if got, _ := vdtFijada(t, root); got != "precios\n" {
		t.Fatalf("hallazgo 5672b8: fixture: precios fijada: %q", got)
	}
	if len(u1Largo)+1 <= u1Tope {
		t.Fatalf("hallazgo 5672b8: fixture: la linea de %s pasa el tope de %d bytes", u1Largo, u1Tope)
	}

	rep := u1CorrerHijo(t, root)
	if rep.Code == 0 || strings.Contains(rep.Out, "tarjeta fijada: "+u1Largo) {
		t.Fatalf("hallazgo 5672b8, CA-500: hoom usar que no pudo escribir no dice que fijo %s (exit %d):\nstdout:\n%s\nstderr:\n%s", u1Largo, rep.Code, rep.Out, rep.Err)
	}
	if got, ok := vdtFijada(t, root); !ok || got != "precios\n" {
		t.Fatalf("hallazgo 5672b8, CA-500: una escritura que fallo conserva la tarjeta fijada de antes (%q), quedo %q (existe: %v)", "precios\n", got, ok)
	}
	s := vdtCorrer(t, "hallazgo 5672b8", vdtOpts{dir: root}, "usar")
	vdtExit(t, "hallazgo 5672b8", s, 0)
	if !strings.Contains(s.out, "tarjeta: precios") {
		t.Fatalf("hallazgo 5672b8, CA-500: hoom usar sigue mostrando precios:\n%s", s)
	}
	ents, _ := os.ReadDir(filepath.Join(root, ".hoom", "cache"))
	for _, e := range ents {
		if e.Name() != "tarjeta" && strings.Contains(e.Name(), "tarjeta") {
			t.Logf("hallazgo 5672b8: aviso: quedo un temporal en .hoom/cache: %s", e.Name())
		}
	}
}
