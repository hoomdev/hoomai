//go:build darwin || linux

// Tests de los hallazgos d03734 y a5e1d2 de la review de
// historia-en-el-tablero: un `hoom item backfill` que falla a mitad de la
// escritura no deja nada a medias. El fallo se provoca con RLIMIT_FSIZE (por
// eso solo darwin y linux): Go ignora SIGXFSZ, asi que una escritura que pasa
// el tope falla con EFBIG ("file too large"), un error de E/S de verdad como
// el del disco lleno, y deterministico. Escritos desde el spec, sin leer la
// implementacion. Todo identificador de este archivo lleva el prefijo h1.
package itemcmd

import (
	"bytes"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// h1ConTopeDeArchivo corre f con el limite blando de RLIMIT_FSIZE en tope
// bytes (el duro no se toca) y lo restaura despues, aunque f entre en panico.
// Ningun test de este paquete es paralelo: el limite es del proceso.
func h1ConTopeDeArchivo(t *testing.T, tope uint64, f func()) {
	t.Helper()
	var antes syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
		t.Fatal(err)
	}
	if antes.Cur < tope {
		t.Skipf("el limite de tamano de archivo del proceso (%d) ya es menor que %d", antes.Cur, tope)
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

// CA-447 (caso limite "nada a medias"), hallazgos d03734 y a5e1d2: tres
// tareas califican, en orden a-uno, b-dos y c-tres. El item de b-dos (su
// titulo, el encabezado de su spec, ocupa 16 KiB) no se puede escribir con un
// tope de 8 KiB por archivo; el de a-uno, chico, si. Backfill termina con
// error y no queda NINGUNO de los items de esa corrida (ni a-uno, escrito
// antes del fallo, ni c-tres); el item que ya existia sigue intacto. Por
// Backfill y por el comando.
func TestCA447_Hallazgod03734_FallaElSegundoNadaAMedias(t *testing.T) {
	const tope = 8 * 1024
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
			t.Fatalf("precondicion: el titulo de b-dos es su encabezado de 16 KiB (su item no entra en %d bytes): %d bytes", tope, len(c.Titulo))
		}
	}

	antes := htFoto(t, root)
	h1ConTopeDeArchivo(t, tope, func() {
		_, err = Backfill(root, "main", "high", false, time.Now().UTC())
	})
	if err == nil {
		t.Fatal("CA-447 (d03734): si no puede escribir el item de b-dos, backfill termina con error")
	}
	htSinCambios(t, "CA-447 (d03734)", "un backfill que falla en el segundo item", antes, htFoto(t, root))

	req, err := Parse([]string{"backfill"})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	h1ConTopeDeArchivo(t, tope, func() {
		err = Execute(root, "main", "high", req, &out, &errb)
	})
	if err == nil {
		t.Fatalf("CA-447 (a5e1d2): 'hoom item backfill' termina con error si no puede escribir un item:\n%s", out.String())
	}
	htSinCambios(t, "CA-447 (a5e1d2)", "'hoom item backfill' que falla en el segundo item", antes, htFoto(t, root))
}
