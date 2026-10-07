//go:build unix

// Tests de contrato de hoomfs.AbrirRegularEn y hoomfs.AbrirDirEn para el
// hallazgo 15a8a5 (tarea estabilizar-ci; CA-442 de
// .hoom/specs/evidencia-en-disco.md: lo que se abre es lo que se miro).
//
// El spec pide que cada componente sea "el mismo objeto antes y despues de
// abrirlo". De ese objeto, el Lstat de antes deja comparar el dispositivo, el
// numero de inodo y el tipo, y el numero solo no es una identidad: un sistema
// de archivos le puede dar un numero liberado al siguiente objeto que crea
// (linux lo hace enseguida; macOS/APFS no). os.SameFile no ve la diferencia:
// compara el dispositivo y el numero y nada mas. Con el numero reutilizado
// hay dos casos, y solo el primero esta decidido:
//
//   - OTRO TIPO (decidido: 15a8a5). Un nombre mirado (Lstat) como symlink,
//     FIFO o directorio, y ocupado despues por un archivo regular que se
//     quedo con ese numero, no es el archivo regular que antes describe, y
//     AbrirRegularEn lo rechaza; uno mirado como archivo regular, symlink o
//     FIFO, y ocupado despues por un directorio con ese numero, no es el
//     directorio que antes describe, y AbrirDirEn lo rechaza.
//   - EL MISMO TIPO (indistinguible, no decidido: hallazgo 1caad0). Un
//     archivo regular borrado y otro creado con su numero, o un directorio
//     borrado y otro creado con el suyo, tampoco son el mismo objeto. Pero
//     stat no expone una generacion del inodo: con el mismo dispositivo,
//     numero y tipo, lo nuevo es para el helper igual a lo mirado reescrito
//     en el lugar, que tampoco se detecto nunca. Que contrato gana (el spec
//     pasa a decir que la identidad es numero y tipo y que la foto registra
//     lo que hay en el nombre, o el inodo mirado se fija con un descriptor)
//     lo decide el seguimiento 1caad0, no estos tests: aceptan que el helper
//     lo rechace o que abra lo que hay AHORA en el nombre, y ninguna otra
//     cosa, y dejan en el log cual de las dos salidas vieron.
//
// La carrera TestCA442_AbrirRegularEnDecideSobreElDescriptorAunqueLaEntradaCambie
// encontro el primer caso una vez en el CI de linux ("lo mirado era
// L--------- y AbrirRegularEn devolvio un archivo") y por suerte: hace falta
// que el numero se reutilice justo entre la mirada y la apertura. Aca hay dos
// tests que no esperan a la suerte, y los dos exigen lo mismo (moComprobar):
//
//   - uno PROVOCA el reuso: crea en el nombre una entrada de un tipo, la
//     mira, la borra y crea otra, hasta que la nueva sale con el numero de la
//     borrada. Si el sistema de archivos no reutiliza numeros (APFS) no hay
//     nada que medir, y se saltea diciendolo.
//   - otro lo CALCA: cuando el sistema no le dio a lo nuevo el numero
//     liberado, se lo copia al Lstat de antes. No depende del sistema de
//     archivos, asi que tambien mide en macOS.
//
// Escritos desde el contrato publico (go doc) y el hallazgo, sin leer la
// implementacion.
package hoomfs

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Los tipos de entrada que se ponen en el nombre.
const (
	moRegular = iota
	moSymlink
	moFifo
	moDir
)

// moTipos dice, de cada tipo, como se llama y que dice de el Mode().Type().
var moTipos = [...]struct {
	nombre string
	modo   fs.FileMode
}{
	moRegular: {"un archivo regular", 0},
	moSymlink: {"un symlink", fs.ModeSymlink},
	moFifo:    {"un FIFO", fs.ModeNamedPipe},
	moDir:     {"un directorio", fs.ModeDir},
}

// Cuanto se insiste, por caso, en provocar un reuso: hasta moVueltas
// vueltas o moPlazo, lo que pase primero, y moBasta reusos alcanzan. Los
// casos corren a la vez, asi que en un sistema que no reutiliza numeros
// (ninguna cantidad de vueltas lo cambia) el test entero cuesta un moPlazo.
const (
	moVueltas = 4000
	moPlazo   = 200 * time.Millisecond
	moBasta   = 100
)

// moCaso es un cambio del nombre entre la mirada y la apertura: se miro una
// entrada de tipo antes, y lo que hay despues es un archivo regular (se abre
// con AbrirRegularEn) o, si dir, un directorio (se abre con AbrirDirEn).
type moCaso struct {
	caso   string
	nombre string
	dir    bool
	antes  int
}

func (c moCaso) ahora() int {
	if c.dir {
		return moDir
	}
	return moRegular
}

// otroTipo: lo que hay ahora no es del tipo que se miro.
func (c moCaso) otroTipo() bool { return c.antes != c.ahora() }

// nota marca en el log los casos de mismo tipo: ahi lo que el helper hizo es
// lo que se vio, no lo que el contrato pide (valen las dos salidas).
func (c moCaso) nota() string {
	if c.otroTipo() {
		return ""
	}
	return " [mismo tipo: indistinguible, no decidido (1caad0): valen rechazarlo y abrir lo de ahora; esto es lo que se vio]"
}

// moCasos son todos: cada tipo mirado, con cada helper.
func moCasos() []moCaso {
	var out []moCaso
	for _, dir := range []bool{false, true} {
		c := moCaso{nombre: "20261006T154221_15a8a5.json", dir: dir}
		helper := "AbrirRegularEn"
		if dir {
			c.nombre, helper = "findings", "AbrirDirEn"
		}
		for _, antes := range []int{moSymlink, moFifo, moDir, moRegular} {
			c.antes = antes
			c.caso = fmt.Sprintf("%s: mirado %s, despues %s", helper, moTipos[antes].nombre, moTipos[c.ahora()].nombre)
			if !c.otroTipo() {
				c.caso = fmt.Sprintf("%s: mirado %s, despues otro", helper, moTipos[antes].nombre)
			}
			out = append(out, c)
		}
	}
	return out
}

// moPoner crea en p una entrada de ese tipo. Un archivo regular lleva marca
// de contenido; un directorio, un archivo que se llama marca (con marca ""
// queda vacio); un symlink apunta a marca, que no existe. El FIFO es un
// mkfifo a secas y no el de internal/carreratest: aca nadie llega a abrirlo
// (se borra antes de llamar al helper), y son miles.
func moPoner(p string, tipo int, marca string) error {
	switch tipo {
	case moRegular:
		return os.WriteFile(p, []byte(marca), 0o644)
	case moSymlink:
		return os.Symlink(marca, p)
	case moFifo:
		return syscall.Mkfifo(p, 0o600)
	}
	if err := os.Mkdir(p, 0o755); err != nil || marca == "" {
		return err
	}
	return os.WriteFile(filepath.Join(p, marca), []byte("{}\n"), 0o644)
}

// moVuelta es una vuelta de un caso: el Lstat de lo que se miro, el de lo
// que hay ahora en el nombre, y la marca de lo de ahora (el contenido del
// archivo regular, o el nombre de lo unico que hay en el directorio), que es
// de esta vuelta y de ninguna otra.
type moVuelta struct {
	antes, ahora fs.FileInfo
	marca        string
}

// moCambiar da la vuelta i del caso c en dir: pone en el nombre una entrada
// del tipo mirado, la mira (el Lstat del que llama, por el root), la borra y
// pone la de ahora. Lo mirado no deja nada atras: un directorio va vacio y
// lo demas es un solo inodo, asi lo unico que se libera es su numero.
func moCambiar(t *testing.T, r *os.Root, dir string, c moCaso, i int) moVuelta {
	t.Helper()
	p := filepath.Join(dir, c.nombre)
	marcaAntes := ""
	if c.antes != moDir {
		marcaAntes = fmt.Sprintf("antes-%06d.json", i)
	}
	if err := moPoner(p, c.antes, marcaAntes); err != nil {
		t.Fatalf("fixture: %s: poner %s: %v", c.caso, moTipos[c.antes].nombre, err)
	}
	antes, err := r.Lstat(c.nombre)
	if err != nil || antes.Mode().Type() != moTipos[c.antes].modo {
		t.Fatalf("fixture: %s: lo mirado es %s: %v %v", c.caso, moTipos[c.antes].nombre, antes, err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatalf("fixture: %s: borrar lo mirado: %v", c.caso, err)
	}
	v := moVuelta{antes: antes, marca: fmt.Sprintf("ahora-%06d.json", i)}
	if err := moPoner(p, c.ahora(), v.marca); err != nil {
		t.Fatalf("fixture: %s: poner %s: %v", c.caso, moTipos[c.ahora()].nombre, err)
	}
	if v.ahora, err = r.Lstat(c.nombre); err != nil || v.ahora.Mode().Type() != moTipos[c.ahora()].modo {
		t.Fatalf("fixture: %s: lo que hay ahora es %s: %v %v", c.caso, moTipos[c.ahora()].nombre, v.ahora, err)
	}
	return v
}

// moQuitar deja el nombre libre para la vuelta que sigue.
func moQuitar(t *testing.T, dir string, c moCaso) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(dir, c.nombre)); err != nil {
		t.Fatalf("fixture: %s: quitar %s: %v", c.caso, c.nombre, err)
	}
}

// moInodo dice el numero de inodo de fi, para los mensajes.
func moInodo(fi fs.FileInfo) string {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprint(st.Ino)
	}
	return "?"
}

// moComprobar es lo que el contrato pide de una vuelta en la que lo mirado y
// lo que hay ahora comparten dispositivo y numero de inodo (os.SameFile; como
// dice de donde salio el numero compartido):
//
//   - si son de OTRO TIPO, lo de ahora no es lo que antes describe: el
//     helper lo rechaza con error y sin devolver archivo ni root (y si el
//     error es *NoRegular, no dice que la entrada es regular).
//   - si son del MISMO tipo, es indistinguible y NO ESTA DECIDIDO (hallazgo
//     1caad0). Lo de ahora tampoco es el objeto mirado (ese se borro, y el
//     spec pide "el mismo objeto antes y despues de abrirlo"), pero stat no
//     expone una generacion del inodo: con el mismo dispositivo, numero y
//     tipo, lo de ahora es para el helper igual a lo mirado reescrito en el
//     lugar. Que contrato gana (el spec pasa a decir que la identidad es
//     numero y tipo, o el inodo mirado se fija con un descriptor) lo decide
//     el seguimiento 1caad0 y no este test, que acepta dos salidas y ninguna
//     otra. RECHAZARLO: error, y ni archivo ni root (con la misma regla para
//     un *NoRegular). O ABRIRLO: entonces lo abierto es lo que hay AHORA en
//     el nombre (os.SameFile con el Lstat de ahora, y lee o lista la marca
//     de esta vuelta), nunca el contenido de la entrada borrada ni otra cosa.
//
// Devuelve el error del rechazo (nil si abrio lo de ahora). Cada helper tiene
// su recorrido, con su apertura, su cierre y sus aserciones: moComprobarDir y
// moComprobarRegular.
func moComprobar(t *testing.T, r *os.Root, c moCaso, v moVuelta, como string) error {
	t.Helper()
	if !os.SameFile(v.antes, v.ahora) {
		t.Fatalf("fixture: %s: lo mirado y lo de ahora comparten dispositivo y numero de inodo (%s y %s)", c.caso, moInodo(v.antes), moInodo(v.ahora))
	}
	que := fmt.Sprintf("%s, con el numero de inodo %s %s", c.caso, moInodo(v.ahora), como)
	if c.dir {
		return moComprobarDir(t, r, c, v, que)
	}
	return moComprobarRegular(t, r, c, v, que)
}

// moComprobarDir es moComprobar para AbrirDirEn: lo que hay ahora en el
// nombre es un directorio con el dispositivo y el numero de lo mirado. que
// dice el caso y el numero, para los mensajes.
func moComprobarDir(t *testing.T, r *os.Root, c moCaso, v moVuelta, que string) error {
	t.Helper()
	d, err := AbrirDirEn(r, c.nombre, v.antes)
	var fi fs.FileInfo
	var serr error
	var lista []string
	if d != nil {
		fi, serr = d.Stat(".")
		if es, lerr := fs.ReadDir(d.FS(), "."); lerr != nil {
			serr = lerr
		} else {
			for _, e := range es {
				lista = append(lista, e.Name())
			}
		}
		d.Close()
	}
	if c.otroTipo() {
		if err == nil || d != nil {
			t.Fatalf("CA-442 (15a8a5): %s: lo mirado era %s (%s) y el directorio de ahora no es lo que antes describe: AbrirDirEn no lo abre: root=%v err=%v (lista %v)", que, moTipos[c.antes].nombre, v.antes.Mode().Type(), d != nil, err, lista)
		}
		return err
	}
	// El mismo tipo: indistinguible, no decidido (1caad0). O lo rechaza, o
	// abre lo de ahora.
	if (err == nil) == (d == nil) {
		t.Fatalf("CA-442 (15a8a5, 1caad0): %s: otro directorio con el numero del mirado (indistinguible del mirado reescrito en el lugar): AbrirDirEn o lo rechaza (error y ningun root) o lo abre (un root y ningun error), no las dos cosas ni ninguna: root=%v err=%v (lista %v)", que, d != nil, err, lista)
	}
	if err != nil {
		return err
	}
	if serr != nil || !fi.IsDir() || !os.SameFile(fi, v.ahora) || !reflect.DeepEqual(lista, []string{v.marca}) {
		t.Fatalf("CA-442 (15a8a5, 1caad0): %s: si AbrirDirEn lo abre, el root es el directorio que hay AHORA en el nombre (os.SameFile con su Lstat) y lista [%s], nunca lo de la entrada borrada ni otra cosa: el de ahora=%v, lista %v (%v)", que, v.marca, fi != nil && os.SameFile(fi, v.ahora), lista, serr)
	}
	return nil
}

// moComprobarRegular es moComprobar para AbrirRegularEn: lo que hay ahora en
// el nombre es un archivo regular con el dispositivo y el numero de lo
// mirado. que dice el caso y el numero, para los mensajes.
func moComprobarRegular(t *testing.T, r *os.Root, c moCaso, v moVuelta, que string) error {
	t.Helper()
	f, err := AbrirRegularEn(r, c.nombre, v.antes)
	var fi fs.FileInfo
	var serr error
	var leyo []byte
	if f != nil {
		if fi, serr = f.Stat(); serr == nil {
			leyo, serr = io.ReadAll(f)
		}
		f.Close()
	}
	if c.otroTipo() {
		if err == nil || f != nil {
			t.Fatalf("CA-442 (15a8a5): %s: lo mirado era %s (%s) y el archivo regular de ahora no es lo que antes describe: AbrirRegularEn no lo abre: archivo=%v err=%v (leyo %q)", que, moTipos[c.antes].nombre, v.antes.Mode().Type(), f != nil, err, leyo)
		}
		aeRechazadoRegular(t, "15a8a5: "+que, nil, err, nil)
		return err
	}
	// El mismo tipo: indistinguible, no decidido (1caad0). O lo rechaza, o
	// abre lo de ahora.
	if (err == nil) == (f == nil) {
		t.Fatalf("CA-442 (15a8a5, 1caad0): %s: otro archivo regular con el numero del mirado (indistinguible del mirado reescrito en el lugar): AbrirRegularEn o lo rechaza (error y ningun archivo) o lo abre (un archivo y ningun error), no las dos cosas ni ninguna: archivo=%v err=%v (leyo %q)", que, f != nil, err, leyo)
	}
	if err != nil {
		aeRechazadoRegular(t, "15a8a5, 1caad0: "+que, nil, err, nil)
		return err
	}
	if serr != nil || !fi.Mode().IsRegular() || !os.SameFile(fi, v.ahora) || !bytes.Equal(leyo, []byte(v.marca)) {
		t.Fatalf("CA-442 (15a8a5, 1caad0): %s: si AbrirRegularEn lo abre, el archivo es el que hay AHORA en el nombre (os.SameFile con su Lstat) y se lee entero (%q), nunca el contenido de la entrada borrada ni otra cosa: el de ahora=%v, leyo %q (%v)", que, v.marca, fi != nil && os.SameFile(fi, v.ahora), leyo, serr)
	}
	return nil
}

// CA-442 (15a8a5): PROVOCA el reuso del numero de inodo en vez de esperarlo.
// Por cada caso, y hasta que alcanza: pone en el nombre una entrada del tipo
// mirado (symlink, FIFO, directorio, archivo regular), la mira, la borra y
// pone ahi un archivo regular (para AbrirRegularEn) o un directorio (para
// AbrirDirEn). Cada vez que lo nuevo sale con el dispositivo y el numero de
// lo borrado (os.SameFile), llama al helper con el Lstat de antes y exige lo
// de moComprobar: con otro tipo, error y nada abierto; con el mismo tipo
// (indistinguible, no decidido: 1caad0), o error y nada abierto, o lo que hay
// ahora en el nombre.
//
// Un caso que no vio ningun reuso se saltea diciendolo (no midio nada), y si
// no lo vio ninguno se saltea el test: en macOS/APFS es lo esperado, porque
// ahi un numero liberado no vuelve. Si corrio, dice cuantos reusos ejercito
// cada caso, y cuantos de esos el helper rechazo y cuantos abrio: en los de
// mismo tipo valen las dos salidas, y un cambio de comportamiento se ve ahi.
// Los casos van a la vez, pero el test no: mientras corre, los tests
// paralelos del paquete esperan y no le sacan los numeros liberados.
func TestCA442_UnNumeroDeInodoReutilizadoPorOtroTipoNoEsLoMirado(t *testing.T) {
	casos := moCasos()
	reusos, rechazos, vueltas := make([]int, len(casos)), make([]int, len(casos)), make([]int, len(casos))
	t.Run("casos", func(t *testing.T) {
		for i, c := range casos {
			t.Run(c.caso, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				r := aeRoot(t, dir)
				fin := time.Now().Add(moPlazo)
				for vueltas[i] < moVueltas && reusos[i] < moBasta && time.Now().Before(fin) {
					v := moCambiar(t, r, dir, c, vueltas[i])
					vueltas[i]++
					if os.SameFile(v.antes, v.ahora) {
						reusos[i]++
						if moComprobar(t, r, c, v, "reutilizado por el sistema de archivos") != nil {
							rechazos[i]++
						}
					}
					moQuitar(t, dir, c)
				}
				if reusos[i] == 0 {
					t.Skipf("CA-442 (15a8a5): %s: SIN EJERCITAR: en %d vueltas el sistema de archivos no le dio a lo nuevo el numero de inodo de lo borrado (APFS no lo hace)", c.caso, vueltas[i])
				}
				t.Logf("CA-442 (15a8a5): %s: %d reusos ejercitados en %d vueltas: %d RECHAZADOS, %d ABIERTOS (lo que hay ahora en el nombre)%s", c.caso, reusos[i], vueltas[i], rechazos[i], reusos[i]-rechazos[i], c.nota())
			})
		}
	})
	total, resumen := 0, make([]string, len(casos))
	for i, c := range casos {
		total += reusos[i]
		resumen[i] = fmt.Sprintf("%s: %d reusos en %d vueltas (%d rechazados, %d abiertos)", c.caso, reusos[i], vueltas[i], rechazos[i], reusos[i]-rechazos[i])
	}
	if total == 0 {
		t.Skipf("CA-442 (15a8a5): SIN EJERCITAR: este sistema de archivos no reutilizo ningun numero de inodo (APFS no lo hace; ext4 en linux si): %s", strings.Join(resumen, "; "))
	}
	t.Logf("CA-442 (15a8a5): %d reusos ejercitados: %s", total, strings.Join(resumen, "; "))
}

// moCalcar deja en el Lstat antes el dispositivo y el numero de inodo de
// ahora. El FileInfo de os guarda el Stat_t que devuelve Sys(), y os.SameFile
// compara esos dos campos: despues de calcarlos, antes es, campo por campo,
// lo que el Lstat habria dicho en un sistema de archivos que le da a lo nuevo
// el numero liberado. Lo demas (tipo, nombre, tamano, fechas) queda como se
// miro. Si este sistema no deja armarlo asi, el test se saltea diciendolo.
func moCalcar(t *testing.T, c moCaso, antes, ahora fs.FileInfo) {
	t.Helper()
	a, okA := antes.Sys().(*syscall.Stat_t)
	b, okB := ahora.Sys().(*syscall.Stat_t)
	if !okA || !okB {
		t.Skipf("fixture: %s: SIN EJERCITAR: el Lstat de este sistema no trae un *syscall.Stat_t donde calcar el numero de inodo (%T, %T)", c.caso, antes.Sys(), ahora.Sys())
	}
	a.Dev, a.Ino = b.Dev, b.Ino
	if !os.SameFile(antes, ahora) || antes.Mode().Type() != moTipos[c.antes].modo {
		t.Skipf("fixture: %s: SIN EJERCITAR: calcar el dispositivo y el numero de inodo en el Lstat no alcanzo para os.SameFile (o le cambio el tipo: %s)", c.caso, antes.Mode().Type())
	}
}

// CA-442 (15a8a5): lo mismo que el test de arriba, sin depender de que el
// sistema de archivos reutilice el numero (mide tambien en macOS): una vuelta
// por caso, y si lo nuevo no salio con el numero de lo borrado, se lo calca
// al Lstat de antes (moCalcar). Con el mismo dispositivo y numero y OTRO
// tipo, AbrirRegularEn y AbrirDirEn rechazan lo que hay en el nombre; con el
// mismo tipo (indistinguible, no decidido: 1caad0), o lo rechazan o abren lo
// que hay ahora, y ninguna otra cosa (moComprobar). Cada caso deja en el log
// cual de las dos salidas vio.
func TestCA442_ElMismoNumeroDeInodoConOtroTipoNoEsLoMirado(t *testing.T) {
	for _, c := range moCasos() {
		t.Run(c.caso, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			r := aeRoot(t, dir)
			v := moCambiar(t, r, dir, c, 0)
			como := "reutilizado por el sistema de archivos"
			if !os.SameFile(v.antes, v.ahora) {
				moCalcar(t, c, v.antes, v.ahora)
				como = "calcado en el Lstat de antes"
			}
			if err := moComprobar(t, r, c, v, como); err != nil {
				t.Logf("CA-442 (15a8a5): %s: con el numero de inodo %s: RECHAZADO: %v%s", c.caso, como, err, c.nota())
			} else {
				t.Logf("CA-442 (15a8a5): %s: con el numero de inodo %s: ABIERTO lo que hay ahora en el nombre%s", c.caso, como, c.nota())
			}
		})
	}
}
