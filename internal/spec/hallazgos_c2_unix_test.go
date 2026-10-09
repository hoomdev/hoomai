//go:build unix

// Tests de regresion de la review de 4 lentes de la cabina C2 sobre
// .hoom/specs/tablero-de-solo-lectura.md:
//
//   - 20261009T191901_af65de (medium, risk, PRE-EXISTENTE; CA-312): el indice
//     de tokens lee "hasta 2 MiB" de cada archivo de test. Un *_test.go que
//     no es un archivo regular se saltea igual que un regular demasiado
//     grande: un symlink a un archivo de mas de 2 MiB (su tamano de enlace es
//     chico, el de su destino no), un symlink a /dev/zero (no termina nunca)
//     y un FIFO (abrirlo espera a un escritor). IndexTokens vuelve enseguida
//     y el resto del indice sale bien. El tablero lo corre en cada sondeo.
//
// Todo vive en t.TempDir(). Cada llamada tiene reloj: un cuelgue es un fallo,
// no un test colgado. El caso de /dev/zero corre en un proceso hijo con un
// vigilante de memoria asignada, asi que no depende de la memoria de la
// maquina.
package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// c2Reloj: lo que se espera a IndexTokens antes de darlo por colgado. Sobre
// un arbol de tres archivos tarda milisegundos.
const c2Reloj = 5 * time.Second

// c2Indice es lo que se mira de un TokenIndex.
type c2Indice struct {
	Scanned int                 `json:"scanned"`
	Files   map[string][]string `json:"files"`
	Err     string              `json:"err,omitempty"`
}

func c2Leer(root string, ids []string) c2Indice {
	x, err := IndexTokens(root)
	r := c2Indice{Files: map[string][]string{}}
	if err != nil {
		r.Err = err.Error()
		return r
	}
	r.Scanned = x.Scanned
	for _, id := range ids {
		r.Files[id] = x.Files(id)
	}
	return r
}

// c2Indexar corre IndexTokens con reloj. volvio es false si a los c2Reloj
// sigue sin volver.
func c2Indexar(root string, ids []string) (r c2Indice, volvio bool, hecho <-chan struct{}) {
	ch := make(chan c2Indice, 1)
	fin := make(chan struct{})
	go func() {
		ch <- c2Leer(root, ids)
		close(fin)
	}()
	select {
	case r = <-ch:
		return r, true, fin
	case <-time.After(c2Reloj):
		return r, false, fin
	}
}

func c2Raiz(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// c2Grande es un cuerpo de mas de 2 MiB que cita el token al principio y al
// final: saltearlo es no indexarlo en ninguna de las dos puntas.
func c2Grande(token string) string {
	return token + " al principio\n" + strings.Repeat("x", 3<<20) + "\n" + token + " al final\n"
}

func c2Symlink(t *testing.T, destino, enlace string) {
	t.Helper()
	if err := os.Symlink(destino, enlace); err != nil {
		t.Fatal(err)
	}
}

// c2ElRestoSaleBien: el archivo de test regular y chico del arbol (a_test.go,
// que cita CA-12) esta en el indice, y es lo unico que se leyo.
func c2ElRestoSaleBien(t *testing.T, r c2Indice, salteados string) {
	t.Helper()
	if r.Err != "" {
		t.Fatalf("af65de CA-312: IndexTokens no falla por tener en el arbol %s: %s", salteados, r.Err)
	}
	if got := strings.Join(r.Files["CA-12"], ","); got != "a_test.go" {
		t.Errorf("af65de CA-312: el resto del indice sale bien: Files(CA-12) es [a_test.go], fue [%s]", got)
	}
	if r.Scanned != 1 {
		t.Errorf("af65de CA-312: se lee 1 archivo de test (a_test.go); %s se saltean como un regular de mas de 2 MiB y no cuentan en Scanned: fue %d",
			salteados, r.Scanned)
	}
}

// Hallazgo 20261009T191901_af65de. CA-312: un *_test.go que es un symlink a
// un archivo de mas de 2 MiB (fuera del arbol, o adentro con un nombre que no
// es de test) se saltea igual que el archivo regular de mas de 2 MiB que esta
// al lado: ninguno de sus tokens entra al indice y no cuenta en Scanned.
// spec.Tokens (el indice mas "ids sin archivos") dice lo mismo.
func TestHallazgo_af65de_UnSymlinkAUnArchivoGrandeSeSalteaComoUnRegularGrande(t *testing.T) {
	root, fuera := c2Raiz(t), c2Raiz(t)
	tkEscribir(t, root, "a_test.go", "package a\n// CA-12\n")
	tkEscribir(t, root, "regular_grande_test.go", c2Grande("CA-888")) // el control: ya se saltea
	tkEscribir(t, fuera, "grande.txt", c2Grande("CA-777"))
	tkEscribir(t, root, "datos/grande.bin", c2Grande("CA-778"))
	c2Symlink(t, filepath.Join(fuera, "grande.txt"), filepath.Join(root, "enlace_afuera_test.go"))
	c2Symlink(t, filepath.Join(root, "datos", "grande.bin"), filepath.Join(root, "enlace_adentro_test.go"))

	ids := []string{"CA-12", "CA-777", "CA-778", "CA-888"}
	r, volvio, _ := c2Indexar(root, ids)
	if !volvio {
		t.Fatalf("af65de CA-312: IndexTokens no volvio en %s con dos symlinks a archivos de 3 MiB", c2Reloj)
	}
	for id, que := range map[string]string{
		"CA-888": "un archivo regular de mas de 2 MiB (el control)",
		"CA-777": "un symlink *_test.go a un archivo de mas de 2 MiB fuera del arbol",
		"CA-778": "un symlink *_test.go a un archivo de mas de 2 MiB del arbol",
	} {
		if got := r.Files[id]; len(got) != 0 {
			t.Errorf("af65de CA-312: %s se saltea (hasta 2 MiB): Files(%s) es vacio, fue %q", que, id, got)
		}
	}
	c2ElRestoSaleBien(t, r, "los dos symlinks a archivos grandes y el regular grande")

	missing, scanned, err := Tokens(root, ids)
	if err != nil {
		t.Fatalf("af65de CA-312: Tokens no falla sobre ese arbol: %v", err)
	}
	if strings.Join(missing, ",") != "CA-777,CA-778,CA-888" || scanned != 1 {
		t.Errorf("af65de CA-312: Tokens da lo mismo que el indice: sin token CA-777, CA-778 y CA-888, y 1 archivo leido; fue missing=%v scanned=%d",
			missing, scanned)
	}
}

// c2HijoDevZero es la variable que le dice al proceso hijo que arbol indexar.
const c2HijoDevZero = "HOOM_TEST_AF65DE_DEV_ZERO"

// c2TechoHijo: lo que el hijo puede asignar antes de que el vigilante lo
// corte. Indexar un arbol de dos archivos chicos asigna unos KiB; aun leyendo
// 2 MiB de un archivo para descartarlo quedan decenas de MiB de margen.
const c2TechoHijo = 64 << 20

// c2HijoIndexar corre en el proceso hijo: indexa root con un vigilante que
// corta el proceso si IndexTokens asigna mas de c2TechoHijo o no vuelve en
// c2Reloj, e informa el resultado por stdout.
func c2HijoIndexar(root string) {
	var antes runtime.MemStats
	runtime.ReadMemStats(&antes)
	t0 := time.Now()
	go func() {
		var ms runtime.MemStats
		for {
			runtime.ReadMemStats(&ms)
			if ms.TotalAlloc-antes.TotalAlloc > c2TechoHijo {
				fmt.Printf("C2-AF65DE-LEYO-DE-MAS %d MiB en %s\n", (ms.TotalAlloc-antes.TotalAlloc)>>20, time.Since(t0).Round(time.Millisecond))
				os.Exit(42)
			}
			if time.Since(t0) > c2Reloj {
				fmt.Println("C2-AF65DE-NO-VOLVIO")
				os.Exit(43)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	raw, _ := json.Marshal(c2Leer(root, []string{"CA-12"}))
	fmt.Println("C2-AF65DE " + string(raw))
	os.Exit(0)
}

// Hallazgo 20261009T191901_af65de. CA-312: un *_test.go que es un symlink a
// /dev/zero nunca se lee: IndexTokens vuelve enseguida, sin asignar memoria,
// y el resto del indice sale bien. Corre en un proceso hijo con un vigilante
// de 64 MiB asignados y reloj: leerlo es un fallo acotado, no un proceso que
// se come la memoria de la maquina.
func TestHallazgo_af65de_UnSymlinkADevZeroSeSalteaSinLeerlo(t *testing.T) {
	if root := os.Getenv(c2HijoDevZero); root != "" {
		c2HijoIndexar(root)
		return
	}
	if fi, err := os.Stat("/dev/zero"); err != nil || fi.Mode()&os.ModeDevice == 0 {
		t.Skip("sin /dev/zero")
	}
	root := c2Raiz(t)
	tkEscribir(t, root, "a_test.go", "package a\n// CA-12\n")
	c2Symlink(t, "/dev/zero", filepath.Join(root, "cero_test.go"))

	ctx, cancel := context.WithTimeout(context.Background(), 6*c2Reloj)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHallazgo_af65de_UnSymlinkADevZeroSeSalteaSinLeerlo$", "-test.count=1")
	cmd.Env = append(os.Environ(), c2HijoDevZero+"="+root)
	raw, _ := cmd.CombinedOutput()
	salida := string(raw)
	if len(salida) > 4000 {
		salida = salida[len(salida)-4000:]
	}
	if m := regexp.MustCompile(`(?m)^C2-AF65DE-LEYO-DE-MAS (.*)$`).FindStringSubmatch(salida); m != nil {
		t.Fatalf("af65de CA-312: con un cero_test.go symlink a /dev/zero IndexTokens lo leyo sin limite: asigno %s y seguia (el vigilante corto el proceso hijo a los %d MiB)",
			m[1], c2TechoHijo>>20)
	}
	if strings.Contains(salida, "C2-AF65DE-NO-VOLVIO") || ctx.Err() != nil {
		t.Fatalf("af65de CA-312: con un cero_test.go symlink a /dev/zero IndexTokens no volvio en %s\n%s", c2Reloj, salida)
	}
	m := regexp.MustCompile(`(?m)^C2-AF65DE (.*)$`).FindStringSubmatch(salida)
	if m == nil {
		t.Fatalf("af65de CA-312: el proceso hijo no informo el resultado:\n%s", salida)
	}
	var r c2Indice
	if err := json.Unmarshal([]byte(m[1]), &r); err != nil {
		t.Fatalf("af65de CA-312: resultado ilegible del hijo: %v %s", err, m[1])
	}
	c2ElRestoSaleBien(t, r, "el symlink a /dev/zero")
}

// c2SoltarLectores abre y cierra cada FIFO como escritor sin bloquear, hasta
// que la lectura colgada termine (o se rinda): un lector bloqueado en abrir o
// en leer el FIFO vuelve con fin de archivo. Solo hace falta cuando el test
// ya fallo; ENXIO es "nadie lo esta leyendo".
func c2SoltarLectores(hecho <-chan struct{}, fifos ...string) {
	for i := 0; i < 200; i++ {
		for _, f := range fifos {
			if fd, err := syscall.Open(f, syscall.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
				_ = syscall.Close(fd)
			}
		}
		select {
		case <-hecho:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Hallazgo 20261009T191901_af65de. CA-312: un FIFO con nombre de test (y un
// *_test.go symlink a un FIFO de afuera) no se abre: IndexTokens vuelve
// enseguida aunque nadie lo escriba, y el resto del indice sale bien.
func TestHallazgo_af65de_UnFIFOConNombreDeTestSeSalteaSinAbrirlo(t *testing.T) {
	root, fuera := c2Raiz(t), c2Raiz(t)
	tkEscribir(t, root, "a_test.go", "package a\n// CA-12\n")
	tubo, tuboAfuera := filepath.Join(root, "tubo_test.go"), filepath.Join(fuera, "tubo")
	for _, f := range []string{tubo, tuboAfuera} {
		if err := syscall.Mkfifo(f, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c2Symlink(t, tuboAfuera, filepath.Join(root, "enlace_al_tubo_test.go"))

	r, volvio, hecho := c2Indexar(root, []string{"CA-12"})
	if !volvio {
		c2SoltarLectores(hecho, tubo, tuboAfuera)
		t.Fatalf("af65de CA-312: IndexTokens no volvio en %s con un FIFO llamado tubo_test.go (y un symlink *_test.go a otro FIFO) en el arbol: se quedo esperando a un escritor",
			c2Reloj)
	}
	c2ElRestoSaleBien(t, r, "el FIFO y el symlink al FIFO")
}
