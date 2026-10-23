//go:build darwin || linux

// Tests de los hallazgos d03734 y a5e1d2 de la review de
// historia-en-el-tablero: un `hoom item backfill` que falla a mitad de la
// escritura no deja nada a medias. El fallo se provoca con RLIMIT_FSIZE (por
// eso solo darwin y linux): Go ignora SIGXFSZ, asi que una escritura que pasa
// el tope falla con EFBIG ("file too large"), un error de E/S de verdad como
// el del disco lleno, y deterministico. El limite es de TODO el proceso, asi
// que nunca se baja en el de 'go test' (chocaria su testlog y cualquier otra
// escritura): el test se re-ejecuta a si mismo en un proceso hijo, que baja
// el limite, corre backfill y reporta el error por stdout; el padre mira el
// disco. Escritos desde el spec, sin leer la implementacion. Todo
// identificador de este archivo lleva el prefijo h1.
package itemcmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	// h1Tope es el limite de tamano de archivo del hijo: el item de a-uno
	// entra, el de b-dos (su titulo ocupa 16 KiB) no.
	h1Tope = 8 * 1024
	// h1EnvRoot y h1EnvModo marcan el modo hijo: el proyecto y que correr
	// (backfill | execute).
	h1EnvRoot = "HOOM_TEST_H1_D03734_ROOT"
	h1EnvModo = "HOOM_TEST_H1_D03734_MODO"
	// h1Marca precede la linea JSON con la que el hijo reporta.
	h1Marca = "H1-D03734-RESULTADO: "
	// h1TestD03734 es el nombre exacto del test (el hijo se corre con el).
	h1TestD03734 = "TestCA447_Hallazgod03734_FallaElSegundoNadaAMedias"
)

// h1Reporte es lo que el hijo dice de su corrida.
type h1Reporte struct {
	Err    string `json:"err"`    // el error de Backfill/Execute; "" = salio bien
	Stdout string `json:"stdout"` // la salida de Execute
}

// h1ConTopeDeArchivo corre f con el limite blando de RLIMIT_FSIZE en tope
// bytes (el duro no se toca) y lo restaura despues, aunque f entre en panico.
// Solo se llama en el proceso hijo.
func h1ConTopeDeArchivo(t *testing.T, tope uint64, f func()) {
	t.Helper()
	var antes syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
		t.Fatal(err)
	}
	if antes.Cur < tope {
		t.Fatalf("el limite de tamano de archivo del proceso (%d) ya es menor que %d", antes.Cur, tope)
	}
	nuevo := antes
	nuevo.Cur = tope
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &nuevo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
			t.Fatalf("no pude restaurar RLIMIT_FSIZE: %v", err)
		}
	}()
	f()
}

// h1Hijo es el proceso hijo: con el tope puesto corre backfill sobre root
// (por Backfill o por el comando), restaura el tope y reporta por stdout.
func h1Hijo(t *testing.T, root, modo string) {
	var rep h1Reporte
	h1ConTopeDeArchivo(t, h1Tope, func() {
		var err error
		switch modo {
		case "backfill":
			_, err = Backfill(root, "main", "high", false, time.Now().UTC())
		case "execute":
			req, perr := Parse([]string{"backfill"})
			if perr != nil {
				err = fmt.Errorf("parse: %w", perr)
				break
			}
			var out, errb bytes.Buffer
			err = Execute(root, "main", "high", req, &out, &errb)
			rep.Stdout = out.String()
		default:
			err = fmt.Errorf("modo hijo desconocido %q", modo)
		}
		if err != nil {
			rep.Err = err.Error()
		}
	})
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s%s\n", h1Marca, raw)
}

// h1CorrerHijo re-ejecuta este test en un proceso hijo sobre root y devuelve
// su reporte. El proceso de 'go test' nunca baja su limite.
func h1CorrerHijo(t *testing.T, root, modo string) h1Reporte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+h1TestD03734+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), h1EnvRoot+"="+root, h1EnvModo+"="+modo)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var rep *h1Reporte
	sc := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if l, ok := strings.CutPrefix(sc.Text(), h1Marca); ok {
			var r h1Reporte
			if jerr := json.Unmarshal([]byte(l), &r); jerr != nil {
				t.Fatalf("el reporte del hijo es JSON: %v\n%s", jerr, l)
			}
			rep = &r
		}
	}
	if rep == nil || err != nil {
		t.Fatalf("el proceso hijo (%s) corre y reporta: %v\nstdout:\n%s\nstderr:\n%s", modo, err, out.String(), errb.String())
	}
	return *rep
}

// CA-447 (caso limite "nada a medias"), hallazgos d03734 y a5e1d2: tres
// tareas califican, en orden a-uno, b-dos y c-tres. El item de b-dos (su
// titulo, el encabezado de su spec, ocupa 16 KiB) no se puede escribir con un
// tope de 8 KiB por archivo; el de a-uno, chico, si. Backfill termina con
// error y no queda NINGUNO de los items de esa corrida (ni a-uno, escrito
// antes del fallo, ni c-tres); el item que ya existia sigue intacto. Por
// Backfill y por el comando, cada uno en un proceso hijo con el tope.
func TestCA447_Hallazgod03734_FallaElSegundoNadaAMedias(t *testing.T) {
	if root := os.Getenv(h1EnvRoot); root != "" {
		h1Hijo(t, root, os.Getenv(h1EnvModo))
		return
	}
	root := htRepo(t, h1BlockOn)
	largo := strings.Repeat("g", 16*1024)
	htSpec(t, root, "b-dos", "# Spec: "+largo)
	htAprobar(t, root, "b-dos")
	for _, s := range []string{"a-uno", "c-tres", "ya-estaba"} {
		htHallazgo(t, root, s, "low")
	}
	icEscribir(t, root, ".hoom/items/ya-estaba.yaml", icItemYAML("Ya estaba", "2026-09-02T10:00:00Z", ""))
	htCommitear(t, root, "spec, hallazgos e item")
	htVeredicto(t, root, "b-dos", htT0, htGatesVerdes(), true)
	htCommitear(t, root, "verde de b-dos")

	pre, err := Backfill(root, "main", "high", true, time.Now().UTC())
	if err != nil {
		t.Fatalf("precondicion: backfill --dry-run: %v", err)
	}
	if got := htCandSlugs(pre.Creados); !reflect.DeepEqual(got, []string{"a-uno", "b-dos", "c-tres"}) ||
		!reflect.DeepEqual(pre.ConItem, []string{"ya-estaba"}) {
		t.Fatalf("precondicion: crearia a-uno, b-dos y c-tres, y ya-estaba tiene item: %+v", pre)
	}
	for _, c := range pre.Creados {
		if c.Slug == "b-dos" && c.Titulo != largo {
			t.Fatalf("precondicion: el titulo de b-dos es su encabezado de 16 KiB (su item no entra en %d bytes): %d bytes", h1Tope, len(c.Titulo))
		}
	}

	antes := htFoto(t, root)
	if rep := h1CorrerHijo(t, root, "backfill"); rep.Err == "" {
		t.Fatal("CA-447 (d03734): si no puede escribir el item de b-dos, backfill termina con error")
	}
	htSinCambios(t, "CA-447 (d03734)", "un backfill que falla en el segundo item", antes, htFoto(t, root))

	if rep := h1CorrerHijo(t, root, "execute"); rep.Err == "" {
		t.Fatalf("CA-447 (a5e1d2): 'hoom item backfill' termina con error si no puede escribir un item:\n%s", rep.Stdout)
	}
	htSinCambios(t, "CA-447 (a5e1d2)", "'hoom item backfill' que falla en el segundo item", antes, htFoto(t, root))
}
