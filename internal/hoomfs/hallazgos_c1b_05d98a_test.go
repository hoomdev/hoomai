//go:build !windows

// Tests de regresion del hallazgo 20261021T145451_05d98a (medium, risk) de la
// review de los fixes de C1, sobre hoomfs.PublicarNuevo, con que se publican
// los items (.hoom/specs/items-y-columna-derivada.md, CA-263 y CA-265) y los
// registros de review (CA-293).
//
// El defecto: si el hard link fallaba, PublicarNuevo creaba el nombre final
// con O_EXCL y escribia ahi directo, asi el archivo quedaba visible a medio
// escribir (y un crash lo dejaba parcial para siempre).
//
// El contrato nuevo, escrito desde el pedido y no desde la implementacion
// (enmienda 1 de la spec): PublicarNuevo(path, data, mode, candado) escribe
// data a un temporal, lo cierra y, bajo hoomfs.Lock(candado) (por ejemplo
// LockPath(root, "items")), lo enlaza (hard link) al nombre final: una sola
// operacion que nunca reemplaza. Solo en un volumen sin hard links (el enlace
// falla con ENOTSUP, EOPNOTSUPP o EPERM) crea el nombre final con O_EXCL y
// escribe: tampoco pisa, pero mientras escribe se puede ver a medias; ese
// camino lo cubre hallazgos_c1b_cbbc8c_434aec_test.go. Si path ya existe
// (archivo, directorio, symlink, aunque apunte a la nada) falla con un error
// que es os.ErrExist y no lo toca. Estos tests corren en el filesystem
// normal, con hard links, es decir por el camino del enlace. Asi:
//
//   - publica entero y nunca pisa;
//   - de N publicaciones simultaneas del mismo nombre (goroutines o procesos)
//     gana una sola, las demas reciben os.ErrExist, y el archivo es entero el
//     del ganador;
//   - un lector que mira el nombre final nunca lo ve parcial;
//   - un fallo de escritura (RLIMIT_FSIZE en un subproceso) no deja ningun
//     archivo con el nombre final, ni temporales;
//   - mientras otro tiene el candado, el nombre final no aparece.
//
// En b72de8f la firma es otra (el cuarto parametro era la funcion de enlace)
// y este archivo no compila: es lo esperado hasta el arreglo.
package hoomfs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/quick"
	"time"
)

const (
	pnEnvModo    = "HOOMFS_C1B_05D98A_MODO" // "publicar" | "carrera"
	pnEnvPath    = "HOOMFS_C1B_05D98A_PATH" // el nombre final, o el directorio en "carrera"
	pnEnvCandado = "HOOMFS_C1B_05D98A_CANDADO"
	pnEnvTam     = "HOOMFS_C1B_05D98A_TAM"     // bytes que publica
	pnEnvSemilla = "HOOMFS_C1B_05D98A_SEMILLA" // semilla de los datos
	pnEnvLimite  = "HOOMFS_C1B_05D98A_LIMITE"  // RLIMIT_FSIZE en "publicar"; -1 = sin limite
	pnEnvNombres = "HOOMFS_C1B_05D98A_NOMBRES" // cuantos nombres publica en "carrera"
)

// pnDatos son n bytes deterministas que dependen de la semilla: binarios, con
// NUL y saltos de linea adentro, distintos para cada semilla.
func pnDatos(n int, semilla int) []byte {
	b := make([]byte, n)
	x := uint32(2166136261) ^ uint32(semilla)*16777619
	for i := range b {
		x ^= uint32(i)
		x *= 16777619
		b[i] = byte(x>>24) ^ byte(semilla)
	}
	return b
}

// pnRaiz arma un proyecto vacio: el directorio donde se publica
// (.hoom/items) y el candado del contrato (LockPath(root, "items")), que vive
// fuera de ese directorio.
func pnRaiz(t *testing.T) (dir, candado string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(root, ".hoom", "items")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, LockPath(root, "items")
}

// pnEntradas lista TODO lo que hay en dir (ocultos incluidos), ordenado.
func pnEntradas(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// pnSolo exige que en dir haya exactamente esos nombres: ningun temporal ni
// parcial.
func pnSolo(t *testing.T, ca, dir string, nombres ...string) {
	t.Helper()
	want := append([]string(nil), nombres...)
	sort.Strings(want)
	if got := pnEntradas(t, dir); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%s: en %s quedan solo %q, sin temporales ni parciales: %q", ca, dir, want, got)
	}
}

// pnIgual exige que path tenga exactamente want.
func pnIgual(t *testing.T, ca, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %s se lee: %v", ca, path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: %s tiene los %d bytes publicados, entero: tiene %d bytes distintos", ca, path, len(want), len(got))
	}
}

// Hallazgo 20261021T145451_05d98a. CA-263 y CA-265: PublicarNuevo crea el
// archivo entero (vacio, de un byte, binario, grande), con a lo sumo los
// permisos pedidos, sin dejar temporales. Una segunda publicacion del mismo
// nombre (con otros datos, o con los mismos) falla con os.ErrExist y no toca
// el archivo: mismo contenido, mismo archivo, mismos permisos.
func TestCA265_Hallazgo05d98a_PublicaEnteroYSinPisar(t *testing.T) {
	casos := []struct {
		nombre string
		datos  []byte
		modo   os.FileMode
	}{
		{"vacio", []byte{}, 0o644},
		{"un byte NUL", []byte{0}, 0o644},
		{"texto con unicode", []byte("titulo: Precios por región ñandú 🦙\npedido: |\n  x\n"), 0o644},
		{"binario de 64 KiB", pnDatos(64<<10, 1), 0o600},
		{"grande de 8 MiB", pnDatos(8<<20, 2), 0o644},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			dir, candado := pnRaiz(t)
			final := filepath.Join(dir, "precios.yaml")
			if err := PublicarNuevo(final, c.datos, c.modo, candado); err != nil {
				t.Fatalf("CA-263: hallazgo 05d98a: PublicarNuevo sobre un nombre libre publica: %v", err)
			}
			pnIgual(t, "CA-263: hallazgo 05d98a", final, c.datos)
			info, err := os.Lstat(final)
			if err != nil {
				t.Fatal(err)
			}
			perm := info.Mode().Perm()
			if !info.Mode().IsRegular() || perm&^c.modo != 0 || perm&0o600 != 0o600 {
				t.Fatalf("CA-263: hallazgo 05d98a: el publicado es un archivo regular con a lo sumo los permisos %v (y el dueno lee y escribe): %v", c.modo, info.Mode())
			}
			pnSolo(t, "CA-263: hallazgo 05d98a", dir, "precios.yaml")

			for _, otro := range []struct {
				que   string
				datos []byte
			}{
				{"con otros datos", pnDatos(len(c.datos)+17, 9)},
				{"con los mismos datos", c.datos},
				{"vacio", nil},
			} {
				err := PublicarNuevo(final, otro.datos, 0o600, candado)
				if !errors.Is(err, os.ErrExist) {
					t.Fatalf("CA-265: hallazgo 05d98a: publicar %s sobre un nombre que ya existe falla con os.ErrExist, fue %v", otro.que, err)
				}
				pnIgual(t, "CA-265: hallazgo 05d98a: el que ya estaba queda byte a byte igual", final, c.datos)
				despues, err := os.Lstat(final)
				if err != nil || !os.SameFile(info, despues) || despues.Mode() != info.Mode() {
					t.Fatalf("CA-265: hallazgo 05d98a: publicar %s no reemplaza el archivo que ya estaba (mismo archivo, mismos permisos): %v %v", otro.que, despues, err)
				}
				pnSolo(t, "CA-265: hallazgo 05d98a: el intento fallido no deja temporales", dir, "precios.yaml")
			}
		})
	}
}

// Hallazgo 20261021T145451_05d98a. CA-265: si el nombre final ya esta tomado
// por algo que no es un archivo regular (un directorio, un symlink a un
// archivo, un symlink que no apunta a nada, un FIFO), PublicarNuevo falla con
// os.ErrExist sin tocarlo: no lo reemplaza, no escribe a traves del symlink
// (ni crea su destino), no se bloquea en el FIFO y no deja temporales.
func TestCA265_Hallazgo05d98a_NombreOcupadoPorOtraCosaNoSePisa(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func(t *testing.T, final, afuera string)
		sigue  func(t *testing.T, final, afuera string)
	}{
		{"directorio vacio", func(t *testing.T, final, _ string) {
			if err := os.Mkdir(final, 0o755); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			if es, err := os.ReadDir(final); err != nil || len(es) != 0 {
				t.Fatalf("el directorio sigue ahi y vacio: %v %v", es, err)
			}
		}},
		{"directorio con un archivo", func(t *testing.T, final, _ string) {
			if err := os.MkdirAll(final, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(final, "adentro"), []byte("intacto"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			pnIgual(t, "CA-265: hallazgo 05d98a: lo de adentro del directorio", filepath.Join(final, "adentro"), []byte("intacto"))
		}},
		{"symlink a un archivo", func(t *testing.T, final, afuera string) {
			if err := os.WriteFile(afuera, []byte("el destino"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(afuera, final); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, afuera string) {
			if dest, err := os.Readlink(final); err != nil || dest != afuera {
				t.Fatalf("el symlink sigue apuntando a %s: %q %v", afuera, dest, err)
			}
			pnIgual(t, "CA-265: hallazgo 05d98a: el destino del symlink", afuera, []byte("el destino"))
		}},
		{"symlink que no apunta a nada", func(t *testing.T, final, afuera string) {
			if err := os.Symlink(afuera, final); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, afuera string) {
			if dest, err := os.Readlink(final); err != nil || dest != afuera {
				t.Fatalf("el symlink sigue apuntando a %s: %q %v", afuera, dest, err)
			}
			if _, err := os.Lstat(afuera); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("PublicarNuevo no escribe a traves del symlink: su destino %s no existe, Lstat = %v", afuera, err)
			}
		}},
		{"FIFO", func(t *testing.T, final, _ string) {
			if err := syscall.Mkfifo(final, 0o644); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			if info, err := os.Lstat(final); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				t.Fatalf("el FIFO sigue ahi: %v %v", info, err)
			}
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			dir, candado := pnRaiz(t)
			final := filepath.Join(dir, "precios.yaml")
			afuera := filepath.Join(filepath.Dir(filepath.Dir(dir)), "afuera.txt")
			c.armar(t, final, afuera)
			antes, err := os.Lstat(final)
			if err != nil {
				t.Fatal(err)
			}
			hecho := make(chan error, 1)
			go func() { hecho <- PublicarNuevo(final, pnDatos(4096, 5), 0o644, candado) }()
			select {
			case err = <-hecho:
			case <-time.After(30 * time.Second):
				t.Fatalf("CA-265: hallazgo 05d98a: PublicarNuevo sobre un nombre tomado por un %s no termina en 30 s", c.nombre)
			}
			if !errors.Is(err, os.ErrExist) {
				t.Fatalf("CA-265: hallazgo 05d98a: el nombre final esta tomado por un %s: PublicarNuevo falla con os.ErrExist, fue %v", c.nombre, err)
			}
			despues, err := os.Lstat(final)
			if err != nil || !os.SameFile(antes, despues) || despues.Mode() != antes.Mode() {
				t.Fatalf("CA-265: hallazgo 05d98a: el %s que ocupaba el nombre no se reemplaza: %v %v", c.nombre, despues, err)
			}
			c.sigue(t, final, afuera)
			pnSolo(t, "CA-265: hallazgo 05d98a", dir, "precios.yaml")
		})
	}
}

// Hallazgo 20261021T145451_05d98a. CA-265: N publicaciones simultaneas del
// mismo nombre desde goroutines: gana exactamente una, las demas reciben
// os.ErrExist, y el archivo es ENTERO el del ganador (nunca uno pisado por
// otro ni una mezcla). Al final no queda ningun temporal.
func TestCA265_Hallazgo05d98a_CarreraEntreGoroutinesGanaUna(t *testing.T) {
	dir, candado := pnRaiz(t)
	const rondas, competidores = 20, 8
	datosDe := func(ronda, i int) []byte { return pnDatos(256<<10+4096*i, 100*ronda+i) }
	var nombres []string
	for ronda := 0; ronda < rondas; ronda++ {
		nombre := fmt.Sprintf("mismo-%02d.yaml", ronda)
		nombres = append(nombres, nombre)
		final := filepath.Join(dir, nombre)
		var wg sync.WaitGroup
		largada := make(chan struct{})
		errs := make([]error, competidores)
		for i := 0; i < competidores; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-largada
				errs[i] = PublicarNuevo(final, datosDe(ronda, i), 0o644, candado)
			}(i)
		}
		close(largada)
		wg.Wait()
		ganador := -1
		for i, err := range errs {
			switch {
			case err == nil && ganador == -1:
				ganador = i
			case err == nil:
				t.Fatalf("CA-265: hallazgo 05d98a: dos publicaciones de %s ganaron a la vez (%d y %d)", nombre, ganador, i)
			case !errors.Is(err, os.ErrExist):
				t.Fatalf("CA-265: hallazgo 05d98a: el perdedor %d de %s recibe os.ErrExist, fue %v", i, nombre, err)
			}
		}
		if ganador == -1 {
			t.Fatalf("CA-265: hallazgo 05d98a: una de las %d publicaciones de %s gana: %v", competidores, nombre, errs)
		}
		pnIgual(t, fmt.Sprintf("CA-265: hallazgo 05d98a: %s es el del ganador %d", nombre, ganador), final, datosDe(ronda, ganador))
	}
	pnSolo(t, "CA-265: hallazgo 05d98a", dir, nombres...)
}

// pnResultado es lo que informa un subproceso por stdout (un pipe: el limite
// de tamano no lo alcanza).
type pnResultado struct {
	Err       string   `json:"err"`
	Existe    bool     `json:"existe"`
	Resultado []string `json:"resultado"` // en "carrera": "ok" | "existe" | el error, por nombre
}

// TestHallazgo05d98aSubproceso no es un test: es el subproceso de los tests
// del hallazgo 20261021T145451_05d98a (CA-293 y CA-265). Sin la variable de
// entorno no hace nada. En "publicar" baja su RLIMIT_FSIZE, ignora SIGXFSZ
// (asi la escritura que pasa del limite devuelve EFBIG) y publica una vez. En
// "carrera" espera la largada y publica una serie de nombres que otros
// subprocesos publican a la vez.
func TestHallazgo05d98aSubproceso(t *testing.T) {
	modo := os.Getenv(pnEnvModo)
	if modo == "" {
		return
	}
	salir := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "subproceso: "+format+"\n", args...)
		os.Exit(2)
	}
	path, candado := os.Getenv(pnEnvPath), os.Getenv(pnEnvCandado)
	tam, err1 := strconv.Atoi(os.Getenv(pnEnvTam))
	semilla, err2 := strconv.Atoi(os.Getenv(pnEnvSemilla))
	if err1 != nil || err2 != nil {
		salir("tam/semilla: %v %v", err1, err2)
	}
	var r pnResultado
	switch modo {
	case "publicar":
		datos := pnDatos(tam, semilla)
		limite, err := strconv.ParseInt(os.Getenv(pnEnvLimite), 10, 64)
		if err != nil {
			salir("limite: %v", err)
		}
		if limite >= 0 {
			signal.Ignore(syscall.SIGXFSZ)
			var lim syscall.Rlimit
			if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
				salir("getrlimit: %v", err)
			}
			lim.Cur = uint64(limite)
			if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
				salir("setrlimit: %v", err)
			}
		}
		if err := PublicarNuevo(path, datos, 0o644, candado); err != nil {
			r.Err, r.Existe = err.Error(), errors.Is(err, os.ErrExist)
		}
	case "carrera":
		n, err := strconv.Atoi(os.Getenv(pnEnvNombres))
		if err != nil {
			salir("nombres: %v", err)
		}
		largada := filepath.Join(filepath.Dir(path), "largada")
		for limite := time.Now().Add(60 * time.Second); ; time.Sleep(time.Millisecond) {
			if _, err := os.Stat(largada); err == nil {
				break
			}
			if time.Now().After(limite) {
				salir("la largada no llego")
			}
		}
		for i := 0; i < n; i++ {
			final := filepath.Join(path, fmt.Sprintf("carrera-%02d.yaml", i))
			switch err := PublicarNuevo(final, pnDatos(tam+1024*semilla, 1000*semilla+i), 0o644, candado); {
			case err == nil:
				r.Resultado = append(r.Resultado, "ok")
			case errors.Is(err, os.ErrExist):
				r.Resultado = append(r.Resultado, "existe")
			default:
				r.Resultado = append(r.Resultado, err.Error())
			}
		}
	default:
		salir("modo desconocido %q", modo)
	}
	raw, _ := json.Marshal(r)
	os.Stdout.Write(append([]byte("PN-SALIDA "), append(raw, '\n')...))
	os.Exit(0)
}

// pnSubproceso arma el subproceso (sin correrlo).
func pnSubproceso(modo, path, candado string, tam, semilla int, extra ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHallazgo05d98aSubproceso$", "-test.count=1", "-test.timeout=240s")
	cmd.Env = append(os.Environ(), pnEnvModo+"="+modo, pnEnvPath+"="+path, pnEnvCandado+"="+candado,
		pnEnvTam+"="+strconv.Itoa(tam), pnEnvSemilla+"="+strconv.Itoa(semilla))
	cmd.Env = append(cmd.Env, extra...)
	return cmd
}

// pnLeerSalida interpreta lo que informo el subproceso.
func pnLeerSalida(t *testing.T, err error, stdout, stderr *bytes.Buffer) pnResultado {
	t.Helper()
	if err != nil {
		t.Fatalf("el subproceso fallo: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	for _, l := range strings.Split(stdout.String(), "\n") {
		if rest, ok := strings.CutPrefix(l, "PN-SALIDA "); ok {
			var r pnResultado
			if err := json.Unmarshal([]byte(rest), &r); err != nil {
				t.Fatalf("la salida del subproceso no es JSON: %v\n%s", err, rest)
			}
			return r
		}
	}
	t.Fatalf("el subproceso no informo nada:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	return pnResultado{}
}

// pnPublicarConLimite publica tam bytes en path desde un subproceso con
// RLIMIT_FSIZE = limite (-1 = sin limite).
func pnPublicarConLimite(t *testing.T, path, candado string, tam, semilla int, limite int64) pnResultado {
	t.Helper()
	cmd := pnSubproceso("publicar", path, candado, tam, semilla, pnEnvLimite+"="+strconv.FormatInt(limite, 10))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	return pnLeerSalida(t, cmd.Run(), &stdout, &stderr)
}

// Hallazgo 20261021T145451_05d98a. CA-293 y CA-265: si la escritura falla (a
// la mitad, en el ultimo byte o en el primero), PublicarNuevo devuelve el
// error y no deja NINGUN archivo con el nombre final (ni parcial ni entero),
// y tampoco temporales. El nombre no quedo tomado: despues se publica entero.
func TestCA293_Hallazgo05d98a_FalloDeEscrituraNoDejaElFinal(t *testing.T) {
	const tam, semilla = 1 << 20, 3
	for _, c := range []struct {
		nombre string
		limite int64
	}{
		{"falla a la mitad", tam / 2},
		{"falla en el ultimo byte", tam - 1},
		{"falla en el primer byte", 0},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			dir, candado := pnRaiz(t)
			final := filepath.Join(dir, "20261021T145451_c1b001.json")
			r := pnPublicarConLimite(t, final, candado, tam, semilla, c.limite)
			if r.Err == "" {
				t.Fatalf("CA-293: fixture: con RLIMIT_FSIZE=%d (< %d bytes) PublicarNuevo tiene que fallar", c.limite, tam)
			}
			if r.Existe {
				t.Errorf("CA-293: hallazgo 05d98a: un fallo de escritura no es os.ErrExist (el nombre estaba libre): %s", r.Err)
			}
			if info, err := os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
				tam := int64(-1)
				if err == nil {
					tam = info.Size()
				}
				t.Fatalf("CA-293: hallazgo 05d98a: %s (%s) y quedo un archivo con el nombre final (%d bytes, Lstat = %v): se publica entero o no se publica",
					c.nombre, r.Err, tam, err)
			}
			if got := pnEntradas(t, dir); len(got) != 0 {
				t.Fatalf("CA-293: hallazgo 05d98a: %s y en %s quedaron temporales o parciales: %q", c.nombre, dir, got)
			}
			if err := PublicarNuevo(final, pnDatos(tam, semilla), 0o644, candado); err != nil {
				t.Fatalf("CA-293: hallazgo 05d98a: tras el fallo el nombre no quedo tomado: se publica entero sin limite: %v", err)
			}
			pnIgual(t, "CA-293: hallazgo 05d98a", final, pnDatos(tam, semilla))
			pnSolo(t, "CA-293: hallazgo 05d98a", dir, filepath.Base(final))
		})
	}
}

// GUARDA. Hallazgo 20261021T145451_05d98a. CA-293: con el limite exactamente
// igual al tamano, la escritura entra y el archivo queda publicado entero.
func TestCA293_Hallazgo05d98a_JustoEnElLimitePublica(t *testing.T) {
	const tam, semilla = 1 << 20, 4
	dir, candado := pnRaiz(t)
	final := filepath.Join(dir, "20261021T145451_c1b002.json")
	if r := pnPublicarConLimite(t, final, candado, tam, semilla, tam); r.Err != "" {
		t.Fatalf("CA-293: con RLIMIT_FSIZE igual al tamano (%d) el archivo entra: %s", tam, r.Err)
	}
	pnIgual(t, "CA-293: hallazgo 05d98a", final, pnDatos(tam, semilla))
	pnSolo(t, "CA-293: hallazgo 05d98a", dir, filepath.Base(final))
}

// Hallazgo 20261021T145451_05d98a. CA-265: la carrera tambien entre procesos
// (la CLI y el Studio): varios subprocesos publican los mismos nombres a la
// vez, cada uno con sus datos. Por nombre gana exactamente uno, los demas
// reciben os.ErrExist, y el archivo es entero el del ganador. Sin temporales.
func TestCA265_Hallazgo05d98a_CarreraEntreProcesosGanaUno(t *testing.T) {
	dir, candado := pnRaiz(t)
	const procesos, nombres, tam = 4, 15, 128 << 10
	type corrida struct {
		cmd            *exec.Cmd
		stdout, stderr bytes.Buffer
	}
	cs := make([]*corrida, procesos)
	for p := 0; p < procesos; p++ {
		c := &corrida{cmd: pnSubproceso("carrera", dir, candado, tam, p+1, pnEnvNombres+"="+strconv.Itoa(nombres))}
		c.cmd.Stdout, c.cmd.Stderr = &c.stdout, &c.stderr
		if err := c.cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cs[p] = c
	}
	largada := filepath.Join(filepath.Dir(dir), "largada")
	if err := os.WriteFile(largada, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	rs := make([]pnResultado, procesos)
	for p, c := range cs {
		rs[p] = pnLeerSalida(t, c.cmd.Wait(), &c.stdout, &c.stderr)
		if len(rs[p].Resultado) != nombres {
			t.Fatalf("fixture: el subproceso %d informa %d resultados, esperaba %d", p, len(rs[p].Resultado), nombres)
		}
	}
	var publicados []string
	for i := 0; i < nombres; i++ {
		nombre := fmt.Sprintf("carrera-%02d.yaml", i)
		publicados = append(publicados, nombre)
		ganador := -1
		for p := range rs {
			switch res := rs[p].Resultado[i]; {
			case res == "ok" && ganador == -1:
				ganador = p
			case res == "ok":
				t.Fatalf("CA-265: hallazgo 05d98a: dos procesos (%d y %d) publicaron %s a la vez", ganador, p, nombre)
			case res != "existe":
				t.Fatalf("CA-265: hallazgo 05d98a: el proceso perdedor %d de %s recibe os.ErrExist, fue %s", p, nombre, res)
			}
		}
		if ganador == -1 {
			t.Fatalf("CA-265: hallazgo 05d98a: uno de los %d procesos publica %s", procesos, nombre)
		}
		semilla := ganador + 1
		pnIgual(t, fmt.Sprintf("CA-265: hallazgo 05d98a: %s es entero el del proceso ganador %d", nombre, ganador),
			filepath.Join(dir, nombre), pnDatos(tam+1024*semilla, 1000*semilla+i))
	}
	pnSolo(t, "CA-265: hallazgo 05d98a", dir, publicados...)
}

// Hallazgo 20261021T145451_05d98a. CA-263 y CA-293: un lector que mira el
// nombre final MIENTRAS se publica un archivo grande nunca lo ve parcial:
// o no existe, o existe con todos sus bytes. Dos lectores giran sobre el
// nombre (Lstat y, la primera vez que aparece, lectura completa) desde antes
// de publicar hasta despues.
func TestCA263_Hallazgo05d98a_LectorNuncaVeParcial(t *testing.T) {
	dir, candado := pnRaiz(t)
	const publicaciones, tam = 6, 24 << 20
	for n := 0; n < publicaciones; n++ {
		final := filepath.Join(dir, fmt.Sprintf("grande-%02d.yaml", n))
		datos := pnDatos(tam, 50+n)
		var (
			parar    atomic.Bool
			ausentes atomic.Int64
			mu       sync.Mutex
			vistos   []string // lo que vieron los lectores que no es el archivo entero
			wg       sync.WaitGroup
		)
		anotar := func(s string) {
			mu.Lock()
			if len(vistos) < 5 {
				vistos = append(vistos, s)
			}
			mu.Unlock()
		}
		for l := 0; l < 2; l++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				leido := false
				for !parar.Load() {
					info, err := os.Lstat(final)
					if errors.Is(err, os.ErrNotExist) {
						ausentes.Add(1)
						continue
					}
					if err != nil {
						anotar(fmt.Sprintf("Lstat: %v", err))
						continue
					}
					if info.Size() != int64(tam) {
						anotar(fmt.Sprintf("con %d de %d bytes", info.Size(), tam))
						continue
					}
					if !leido {
						leido = true
						got, err := os.ReadFile(final)
						if err != nil || !bytes.Equal(got, datos) {
							anotar(fmt.Sprintf("leido con %d bytes distintos (%v)", len(got), err))
						}
					}
				}
			}()
		}
		for limite := time.Now().Add(10 * time.Second); ausentes.Load() == 0; time.Sleep(time.Millisecond) {
			if time.Now().After(limite) {
				parar.Store(true)
				wg.Wait()
				t.Fatalf("fixture: los lectores no arrancaron")
			}
		}
		err := PublicarNuevo(final, datos, 0o644, candado)
		time.Sleep(20 * time.Millisecond) // que los lectores lo vean ya publicado
		parar.Store(true)
		wg.Wait()
		if err != nil {
			t.Fatalf("CA-263: hallazgo 05d98a: PublicarNuevo de %d bytes publica: %v", tam, err)
		}
		if len(vistos) != 0 {
			t.Fatalf("CA-263: hallazgo 05d98a: un lector vio %s a medio publicar: %q", filepath.Base(final), vistos)
		}
		pnIgual(t, "CA-263: hallazgo 05d98a", final, datos)
	}
	var todos []string
	for n := 0; n < publicaciones; n++ {
		todos = append(todos, fmt.Sprintf("grande-%02d.yaml", n))
	}
	pnSolo(t, "CA-263: hallazgo 05d98a", dir, todos...)
}

// Hallazgo 20261021T145451_05d98a. CA-265: el enlace al nombre final va
// bajo el candado que PublicarNuevo recibe. Mientras otro lo tiene, el nombre
// final no aparece; al soltarlo, la publicacion termina entera, o fallo sin
// dejar nada (si se canso de esperar). Nunca quedan temporales.
func TestCA265_Hallazgo05d98a_RespetaElCandado(t *testing.T) {
	dir, candado := pnRaiz(t)
	final := filepath.Join(dir, "precios.yaml")
	datos := pnDatos(1<<20, 7)
	soltar, err := Lock(candado, 2*time.Second)
	if err != nil {
		t.Fatalf("fixture: el test toma el candado: %v", err)
	}
	hecho := make(chan error, 1)
	go func() { hecho <- PublicarNuevo(final, datos, 0o644, candado) }()
	var res error
	termino := false
	for limite := time.Now().Add(800 * time.Millisecond); time.Now().Before(limite); time.Sleep(2 * time.Millisecond) {
		if _, err := os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
			soltar()
			t.Fatalf("CA-265: hallazgo 05d98a: con el candado %s tomado por otro, el nombre final aparecio (Lstat = %v)", candado, err)
		}
		if !termino {
			select {
			case res = <-hecho:
				termino = true
			default:
			}
		}
	}
	if termino && res == nil {
		soltar()
		t.Fatalf("CA-265: hallazgo 05d98a: con el candado tomado por otro, PublicarNuevo dijo publicar sin el candado")
	}
	soltar()
	if !termino {
		select {
		case res = <-hecho:
		case <-time.After(60 * time.Second):
			t.Fatalf("CA-265: hallazgo 05d98a: soltado el candado, PublicarNuevo no termina en 60 s")
		}
	}
	if res == nil {
		pnIgual(t, "CA-265: hallazgo 05d98a: soltado el candado, la publicacion termina entera", final, datos)
		pnSolo(t, "CA-265: hallazgo 05d98a", dir, "precios.yaml")
		return
	}
	if _, err := os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CA-265: hallazgo 05d98a: PublicarNuevo fallo esperando el candado (%v) y aun asi dejo el nombre final", res)
	}
	pnSolo(t, "CA-265: hallazgo 05d98a: el que se canso de esperar no deja temporales", dir)
}

// Hallazgo 20261021T145451_05d98a. CA-265, como propiedad: para cualquier
// secuencia de publicaciones sobre unos pocos nombres, la primera de cada
// nombre publica sus datos enteros, todas las siguientes fallan con
// os.ErrExist sin tocarlo, y en el directorio no hay nada mas que los nombres
// publicados.
func TestCA265_Hallazgo05d98a_PropiedadLaPrimeraGana(t *testing.T) {
	var fallas []string
	prop := func(ops []uint16) bool {
		dir, candado := pnRaiz(t)
		modelo := map[string][]byte{}
		for k, op := range ops {
			nombre := fmt.Sprintf("n%d.yaml", op%4)
			datos := pnDatos(int(op>>2)%5000, k)
			err := PublicarNuevo(filepath.Join(dir, nombre), datos, 0o644, candado)
			if previo, ok := modelo[nombre]; ok {
				if !errors.Is(err, os.ErrExist) {
					fallas = append(fallas, fmt.Sprintf("op %d: %s ya existia: os.ErrExist, fue %v", k, nombre, err))
					return false
				}
				if got, _ := os.ReadFile(filepath.Join(dir, nombre)); !bytes.Equal(got, previo) {
					fallas = append(fallas, fmt.Sprintf("op %d: %s se piso", k, nombre))
					return false
				}
				continue
			}
			if err != nil {
				fallas = append(fallas, fmt.Sprintf("op %d: %s estaba libre y no se publico: %v", k, nombre, err))
				return false
			}
			modelo[nombre] = datos
		}
		var want []string
		for n, d := range modelo {
			want = append(want, n)
			if got, _ := os.ReadFile(filepath.Join(dir, n)); !bytes.Equal(got, d) {
				fallas = append(fallas, fmt.Sprintf("%s no quedo entero: %d de %d bytes", n, len(got), len(d)))
				return false
			}
		}
		sort.Strings(want)
		if got := pnEntradas(t, dir); strings.Join(got, "|") != strings.Join(want, "|") {
			fallas = append(fallas, fmt.Sprintf("en el directorio quedan %q, esperaba %q", got, want))
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 60}); err != nil {
		t.Fatalf("CA-265: hallazgo 05d98a: la primera publicacion de cada nombre gana y nada mas queda: %q (%v)", fallas, err)
	}
}
