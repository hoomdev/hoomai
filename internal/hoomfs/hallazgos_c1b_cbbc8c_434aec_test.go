//go:build !windows

// Tests de los hallazgos 20261021T174339_cbbc8c (high, resilience) y
// 20261021T173339_434aec (medium, risk) de la delta 1 de fixes-cabina-c1b,
// sobre hoomfs.PublicarNuevo, con que se publican los items
// (.hoom/specs/items-y-columna-derivada.md, CA-263 y CA-265) y los registros
// de review, que son append-only (CA-293). Escritos desde la enmienda 1 de la
// spec y la decision de Henry, nunca desde la implementacion.
//
// El defecto: en un volumen sin hard links, PublicarNuevo comprobaba que el
// nombre final no existiera y despues renombraba el temporal encima. Entre
// las dos llamadas un escritor que no toma el candado (otra version de hoom,
// git, cualquier otro programa) puede crear el nombre, y el renombre lo pisa:
// PublicarNuevo dice que publico y el archivo del otro se perdio.
//
// El contrato (enmienda 1): PublicarNuevo publica con un enlace (hard link)
// bajo el candado: entero y sin pisar. Si el enlace falla con "no soportado"
// (ENOTSUP, EOPNOTSUPP o EPERM: exFAT, FAT, algunos mounts de red), crea el
// nombre final con O_EXCL y escribe:
//
//   - publica entero (cuando termina, el archivo tiene todos los bytes) y no
//     deja temporales;
//   - NUNCA pisa a otro escritor: si el nombre existe o aparece (archivo,
//     symlink, aunque apunte a la nada, directorio, FIFO) devuelve un error
//     que es os.ErrExist y no toca ese archivo;
//   - mientras escribe se puede ver a medias: es un limite declarado y aca no
//     se exige lo contrario;
//   - si la escritura falla, borra el archivo que el mismo creo: no queda ni
//     el final ni un temporal, y el nombre queda libre;
//   - un error de enlace que no es "no soportado" (EACCES, EIO, ...) se
//     devuelve tal cual: no se cae al camino de O_EXCL.
//
// Un volumen sin hard links se simula reemplazando la variable de paquete
// enlazar (os.Link) por un enlace que falla como os.Link (un *os.LinkError)
// con ENOTSUP, EOPNOTSUPP o EPERM. En 37b6ecb enlazar no existe y este
// archivo no compila: es lo esperado hasta el arreglo.
//
// HOOMFS_C1B_CBBC8C_RONDAS cambia las rondas de la carrera con el escritor
// ajeno (por defecto 6000, 400 con -short) y HOOMFS_C1B_CBBC8C_SEGUNDOS su
// tope de tiempo (por defecto 90).
package hoomfs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
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

// slNoSoportado son los errores con que falla un enlace en un volumen sin hard
// links: los unicos que llevan al camino de O_EXCL (enmienda 1). En Linux
// ENOTSUP y EOPNOTSUPP son el mismo numero; en macOS no.
var slNoSoportado = []struct {
	nombre string
	errno  syscall.Errno
}{
	{"ENOTSUP", syscall.ENOTSUP},
	{"EOPNOTSUPP", syscall.EOPNOTSUPP},
	{"EPERM", syscall.EPERM},
}

// slEnlaceQueFalla es un enlace como os.Link en un volumen sin hard links: no
// crea nada y falla con un *os.LinkError. Rota por errnos, uno por llamada, y
// cuenta las llamadas.
func slEnlaceQueFalla(llamadas *atomic.Int64, errnos ...syscall.Errno) func(viejo, nuevo string) error {
	return func(viejo, nuevo string) error {
		n := llamadas.Add(1)
		return &os.LinkError{Op: "link", Old: viejo, New: nuevo, Err: errnos[int(n-1)%len(errnos)]}
	}
}

// slSinHardLinks reemplaza enlazar, mientras dure el test, por un enlace que
// falla con errnos (rotando), y devuelve el contador de llamadas. Ningun test
// de este archivo es paralelo: nadie mas publica mientras enlazar esta
// reemplazado.
func slSinHardLinks(t *testing.T, errnos ...syscall.Errno) *atomic.Int64 {
	t.Helper()
	var llamadas atomic.Int64
	previo := enlazar
	enlazar = slEnlaceQueFalla(&llamadas, errnos...)
	t.Cleanup(func() { enlazar = previo })
	return &llamadas
}

// slSeLlamo exige que PublicarNuevo haya intentado el enlace (si no, el
// camino sin hard links no se ejercito y el test no prueba nada).
func slSeLlamo(t *testing.T, llamadas *atomic.Int64) {
	t.Helper()
	if llamadas.Load() == 0 {
		t.Fatalf("fixture: PublicarNuevo nunca llamo a enlazar: el volumen sin hard links no se simulo")
	}
}

// slEsperar espera lo que devuelve PublicarNuevo, con un tope. Si el nombre
// es un FIFO y PublicarNuevo quedo bloqueado abriendolo, lo suelta abriendo
// el FIFO para leer sin bloquear, asi no queda nada colgado.
func slEsperar(t *testing.T, hecho <-chan error, final, que string) error {
	t.Helper()
	select {
	case err := <-hecho:
		return err
	case <-time.After(30 * time.Second):
	}
	if f, err := os.OpenFile(final, os.O_RDONLY|syscall.O_NONBLOCK, 0); err == nil {
		select {
		case <-hecho:
		case <-time.After(5 * time.Second):
		}
		f.Close()
	}
	t.Fatalf("CA-265: hallazgo cbbc8c: sin hard links, PublicarNuevo %s no termina en 30 s", que)
	return nil
}

// Hallazgos 20261021T174339_cbbc8c y 20261021T173339_434aec. CA-263 y
// CA-265: sin hard links (el enlace falla con ENOTSUP, EOPNOTSUPP o EPERM),
// PublicarNuevo publica igual: el archivo entero (vacio, de un byte, unicode,
// binario, grande), regular, con a lo sumo los permisos pedidos, sin
// temporales. Una segunda publicacion del mismo nombre falla con os.ErrExist
// y no lo toca: mismo archivo, mismo contenido, mismos permisos.
func TestCA263_HallazgoCbbc8c_SinHardLinksPublicaEntero(t *testing.T) {
	casos := []struct {
		nombre string
		datos  []byte
		modo   os.FileMode
	}{
		{"vacio", []byte{}, 0o644},
		{"un byte NUL", []byte{0}, 0o644},
		{"texto con unicode", []byte("titulo: Precios por región ñandú 🦙\npedido: |\n  x\n"), 0o644},
		{"binario de 64 KiB", pnDatos(64<<10, 11), 0o600},
		{"grande de 8 MiB", pnDatos(8<<20, 12), 0o644},
	}
	for _, e := range slNoSoportado {
		for _, c := range casos {
			t.Run(e.nombre+"/"+c.nombre, func(t *testing.T) {
				llamadas := slSinHardLinks(t, e.errno)
				dir, candado := pnRaiz(t)
				final := filepath.Join(dir, "precios.yaml")
				if err := PublicarNuevo(final, c.datos, c.modo, candado); err != nil {
					t.Fatalf("CA-263: hallazgo cbbc8c: sin hard links (%s), PublicarNuevo sobre un nombre libre publica con O_EXCL: %v", e.nombre, err)
				}
				slSeLlamo(t, llamadas)
				pnIgual(t, "CA-263: hallazgo cbbc8c: sin hard links", final, c.datos)
				info, err := os.Lstat(final)
				if err != nil {
					t.Fatal(err)
				}
				perm := info.Mode().Perm()
				if !info.Mode().IsRegular() || perm&^c.modo != 0 || perm&0o600 != 0o600 {
					t.Fatalf("CA-263: hallazgo cbbc8c: sin hard links, el publicado es un archivo regular con a lo sumo los permisos %v (y el dueno lee y escribe): %v", c.modo, info.Mode())
				}
				pnSolo(t, "CA-263: hallazgo cbbc8c: sin hard links no quedan temporales", dir, "precios.yaml")

				for _, otro := range []struct {
					que   string
					datos []byte
				}{
					{"con otros datos", pnDatos(len(c.datos)+17, 19)},
					{"con los mismos datos", c.datos},
					{"vacio", nil},
				} {
					err := PublicarNuevo(final, otro.datos, 0o600, candado)
					if !errors.Is(err, os.ErrExist) {
						t.Fatalf("CA-265: hallazgo cbbc8c: sin hard links, publicar %s sobre un nombre que ya existe falla con os.ErrExist, fue %v", otro.que, err)
					}
					pnIgual(t, "CA-265: hallazgo cbbc8c: sin hard links, el que ya estaba queda byte a byte igual", final, c.datos)
					despues, err := os.Lstat(final)
					if err != nil || !os.SameFile(info, despues) || despues.Mode() != info.Mode() {
						t.Fatalf("CA-265: hallazgo cbbc8c: sin hard links, publicar %s no reemplaza el archivo que ya estaba (mismo archivo, mismos permisos): %v %v", otro.que, despues, err)
					}
					pnSolo(t, "CA-265: hallazgo cbbc8c: sin hard links, el intento fallido no deja temporales", dir, "precios.yaml")
				}
			})
		}
	}
}

// Hallazgos 20261021T174339_cbbc8c y 20261021T173339_434aec. CA-265: sin hard
// links, si el nombre final ya esta tomado (un archivo de otro, un symlink a
// un archivo, un symlink que no apunta a nada, un directorio, un FIFO),
// PublicarNuevo falla con os.ErrExist y no lo toca: no lo trunca ni lo
// reemplaza, no escribe a traves del symlink (ni crea su destino), no se
// bloquea en el FIFO y no deja temporales.
func TestCA265_HallazgoCbbc8c_SinHardLinksNombreOcupadoNoSePisa(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func(t *testing.T, final, afuera string)
		sigue  func(t *testing.T, final, afuera string)
	}{
		{"archivo de otro escritor", func(t *testing.T, final, _ string) {
			if err := os.WriteFile(final, []byte("titulo: del otro escritor\n"), 0o640); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			pnIgual(t, "CA-265: hallazgo cbbc8c: el archivo del otro escritor", final, []byte("titulo: del otro escritor\n"))
		}},
		{"archivo vacio de otro escritor", func(t *testing.T, final, _ string) {
			if err := os.WriteFile(final, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			pnIgual(t, "CA-265: hallazgo cbbc8c: el archivo vacio del otro escritor sigue vacio", final, []byte{})
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
				t.Fatalf("CA-265: hallazgo cbbc8c: el symlink sigue apuntando a %s: %q %v", afuera, dest, err)
			}
			pnIgual(t, "CA-265: hallazgo cbbc8c: no escribe a traves del symlink: su destino", afuera, []byte("el destino"))
		}},
		{"symlink que no apunta a nada", func(t *testing.T, final, afuera string) {
			if err := os.Symlink(afuera, final); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, afuera string) {
			if dest, err := os.Readlink(final); err != nil || dest != afuera {
				t.Fatalf("CA-265: hallazgo cbbc8c: el symlink sigue apuntando a %s: %q %v", afuera, dest, err)
			}
			if _, err := os.Lstat(afuera); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("CA-265: hallazgo cbbc8c: PublicarNuevo no crea el destino del symlink %s, Lstat = %v", afuera, err)
			}
		}},
		{"directorio vacio", func(t *testing.T, final, _ string) {
			if err := os.Mkdir(final, 0o755); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			if es, err := os.ReadDir(final); err != nil || len(es) != 0 {
				t.Fatalf("CA-265: hallazgo cbbc8c: el directorio sigue ahi y vacio: %v %v", es, err)
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
			pnIgual(t, "CA-265: hallazgo cbbc8c: lo de adentro del directorio", filepath.Join(final, "adentro"), []byte("intacto"))
		}},
		{"FIFO", func(t *testing.T, final, _ string) {
			if err := syscall.Mkfifo(final, 0o644); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			if info, err := os.Lstat(final); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				t.Fatalf("CA-265: hallazgo cbbc8c: el FIFO sigue ahi: %v %v", info, err)
			}
		}},
	}
	for _, e := range slNoSoportado {
		for _, c := range casos {
			t.Run(e.nombre+"/"+c.nombre, func(t *testing.T) {
				llamadas := slSinHardLinks(t, e.errno)
				dir, candado := pnRaiz(t)
				final := filepath.Join(dir, "precios.yaml")
				afuera := filepath.Join(filepath.Dir(filepath.Dir(dir)), "afuera.txt")
				c.armar(t, final, afuera)
				antes, err := os.Lstat(final)
				if err != nil {
					t.Fatal(err)
				}
				hecho := make(chan error, 1)
				go func() { hecho <- PublicarNuevo(final, pnDatos(4096, 15), 0o644, candado) }()
				err = slEsperar(t, hecho, final, "sobre un nombre tomado por un "+c.nombre)
				slSeLlamo(t, llamadas)
				if !errors.Is(err, os.ErrExist) {
					t.Fatalf("CA-265: hallazgo cbbc8c: sin hard links (%s), el nombre final esta tomado por un %s: PublicarNuevo falla con os.ErrExist, fue %v", e.nombre, c.nombre, err)
				}
				despues, err := os.Lstat(final)
				if err != nil || !os.SameFile(antes, despues) || despues.Mode() != antes.Mode() {
					t.Fatalf("CA-265: hallazgo cbbc8c: sin hard links, el %s que ocupaba el nombre no se reemplaza: %v %v", c.nombre, despues, err)
				}
				c.sigue(t, final, afuera)
				pnSolo(t, "CA-265: hallazgo cbbc8c: sin hard links, el intento fallido no deja temporales", dir, "precios.yaml")
			})
		}
	}
}

// Hallazgos 20261021T174339_cbbc8c y 20261021T173339_434aec. CA-265 y
// CA-293: sin hard links, en cada ronda un escritor ajeno, sin candado, crea
// el nombre final con O_CREATE|O_EXCL y lo escribe al mismo tiempo que
// PublicarNuevo lo publica. Gana uno solo: si gana el ajeno, PublicarNuevo
// devuelve os.ErrExist y el archivo es entero el del ajeno (no lo pisa ni lo
// borra); si gana PublicarNuevo, el ajeno recibe EEXIST y el archivo es entero
// el publicado. Nunca dos ganadores, y al final solo quedan los nombres
// finales. El momento en que crea el ajeno se ajusta solo, ronda a ronda,
// hacia el punto en que cada uno gana la mitad de las veces: ahi es donde
// PublicarNuevo decide si el nombre esta libre y lo ocupa.
func TestCA265_HallazgoCbbc8c_434aec_SinHardLinksEscritorAjenoNoEsPisado(t *testing.T) {
	if prev := runtime.GOMAXPROCS(0); prev < 2 {
		runtime.GOMAXPROCS(2)
		defer runtime.GOMAXPROCS(prev)
	}
	errnos := make([]syscall.Errno, 0, len(slNoSoportado))
	for _, e := range slNoSoportado {
		errnos = append(errnos, e.errno)
	}
	llamadas := slSinHardLinks(t, errnos...)
	dir, candado := pnRaiz(t)

	rondas := 6000
	if testing.Short() {
		rondas = 400
	}
	rondas = ajEntero(t, "HOOMFS_C1B_CBBC8C_RONDAS", rondas)
	tope := time.Duration(ajEntero(t, "HOOMFS_C1B_CBBC8C_SEGUNDOS", 90)) * time.Second

	// Calibracion: cuanto tarda una publicacion sin hard links y sin
	// competencia.
	var muestras []time.Duration
	for i := 0; i < 40; i++ {
		p := filepath.Join(dir, fmt.Sprintf("calibracion-%02d.yaml", i))
		t0 := time.Now()
		if err := PublicarNuevo(p, ajDatos("calibracion", i), 0o644, candado); err != nil {
			t.Fatalf("CA-263: hallazgo cbbc8c: sin hard links, PublicarNuevo sobre un nombre libre publica: %v", err)
		}
		muestras = append(muestras, time.Since(t0))
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	slSeLlamo(t, llamadas)
	sort.Slice(muestras, func(i, j int) bool { return muestras[i] < muestras[j] })
	mediana := muestras[len(muestras)/2]

	rnd := rand.New(rand.NewSource(0xcbbc8c))
	centro := float64(mediana)
	ancho := float64(mediana) / 2
	var (
		hechas, ganaAjeno, ganaPublicar, dobles, otros int
		nombres                                        []string
		primerDoble                                    string
	)
	inicio := time.Now()
	for ronda := 0; ronda < rondas && time.Since(inicio) < tope; ronda++ {
		nombre := fmt.Sprintf("r%05d.yaml", ronda)
		final := filepath.Join(dir, nombre)
		nombres = append(nombres, nombre)
		datosPub, datosAj := ajDatos("publicar", ronda), ajDatos("ajeno", ronda)
		d := time.Duration(centro + (rnd.Float64()-0.5)*ancho)
		if d < 0 {
			d = 0
		}

		largada := make(chan struct{})
		var t0 time.Time
		errPub := make(chan error, 1)
		type resAj struct{ crear, escribir error }
		errAj := make(chan resAj, 1)
		listos := make(chan struct{}, 2)
		go func() {
			listos <- struct{}{}
			<-largada
			errPub <- PublicarNuevo(final, datosPub, 0o644, candado)
		}()
		go func() {
			listos <- struct{}{}
			<-largada
			ajEspera(t0, d)
			c, w := ajAjeno(final, datosAj)
			errAj <- resAj{c, w}
		}()
		<-listos
		<-listos
		t0 = time.Now()
		close(largada)
		ep, ea := <-errPub, <-errAj
		hechas++

		if ea.escribir != nil {
			t.Fatalf("ronda %d: el ajeno creo %s pero no pudo escribirlo: %v", ronda, nombre, ea.escribir)
		}
		got, rerr := os.ReadFile(final)
		switch {
		case ep == nil && ea.crear == nil:
			dobles++
			if primerDoble == "" {
				que := "ni el publicado ni el del ajeno"
				switch {
				case rerr != nil:
					que = "ilegible: " + rerr.Error()
				case bytes.Equal(got, datosPub):
					que = "el publicado (el ajeno perdio lo que escribio)"
				case bytes.Equal(got, datosAj):
					que = "el del ajeno (lo publicado se perdio)"
				}
				primerDoble = fmt.Sprintf("ronda %d (%s, el ajeno creo a los %v de la largada): PublicarNuevo devolvio nil y el ajeno creo el archivo con O_EXCL: dos ganadores; el archivo es %s",
					ronda, nombre, d, que)
			}
		case ep == nil:
			ganaPublicar++
			if !errors.Is(ea.crear, os.ErrExist) {
				otros++
				t.Errorf("CA-265: hallazgo cbbc8c: sin hard links, ronda %d: gano PublicarNuevo: el ajeno recibe EEXIST, recibio %v", ronda, ea.crear)
			}
			if rerr != nil || !bytes.Equal(got, datosPub) {
				otros++
				t.Errorf("CA-265: hallazgo cbbc8c: sin hard links, ronda %d: gano PublicarNuevo: %s es entero lo publicado (err %v, %d bytes)", ronda, nombre, rerr, len(got))
			}
		case ea.crear == nil:
			ganaAjeno++
			if !errors.Is(ep, os.ErrExist) {
				otros++
				t.Errorf("CA-265: hallazgo cbbc8c: sin hard links, ronda %d: gano el ajeno: PublicarNuevo devuelve un error que es os.ErrExist, devolvio %v", ronda, ep)
			}
			if rerr != nil || !bytes.Equal(got, datosAj) {
				otros++
				t.Errorf("CA-293: hallazgo 434aec: sin hard links, ronda %d: gano el ajeno: %s sigue siendo entero el del ajeno, PublicarNuevo no lo pisa ni lo borra (err %v, %d bytes)", ronda, nombre, rerr, len(got))
			}
		default:
			otros++
			t.Errorf("CA-265: hallazgo cbbc8c: sin hard links, ronda %d: alguno de los dos gana: PublicarNuevo %v, ajeno %v", ronda, ep, ea.crear)
		}
		if otros > 20 {
			t.Fatalf("demasiados errores; corto en la ronda %d", ronda)
		}

		switch {
		case ea.crear == nil && ep != nil:
			centro += float64(mediana) / 20
		case ep == nil && ea.crear != nil:
			centro -= float64(mediana) / 20
		}
		if centro < 0 {
			centro = 0
		}
		if centro > 4*float64(mediana) {
			centro = 4 * float64(mediana)
		}
	}

	t.Logf("hallazgos cbbc8c/434aec: sin hard links, %d rondas en %v (mediana de una publicacion sola %v): gano PublicarNuevo %d, gano el ajeno %d, dos ganadores %d",
		hechas, time.Since(inicio).Round(time.Millisecond), mediana, ganaPublicar, ganaAjeno, dobles)
	if hechas >= 100 && (ganaPublicar == 0 || ganaAjeno == 0) && dobles == 0 {
		t.Fatalf("fixture: la carrera no se dio: en %d rondas gano siempre el mismo (PublicarNuevo %d, ajeno %d)", hechas, ganaPublicar, ganaAjeno)
	}
	if dobles > 0 {
		t.Errorf("CA-265 y CA-293: hallazgos cbbc8c/434aec: sin hard links, PublicarNuevo reemplazo el archivo que un escritor ajeno (sin candado) creo con O_EXCL en %d de %d rondas; la primera: %s",
			dobles, hechas, primerDoble)
	}
	pnSolo(t, "CA-265: hallazgos cbbc8c/434aec: sin hard links, solo los nombres finales, sin temporales", dir, nombres...)
}

// Hallazgo 20261021T174339_cbbc8c. CA-265: sin hard links, N publicaciones
// simultaneas del mismo nombre desde goroutines: gana exactamente una, las
// demas reciben os.ErrExist, y cuando todas terminaron el archivo es ENTERO
// el del ganador (nunca pisado por otro ni una mezcla). Sin temporales.
func TestCA265_HallazgoCbbc8c_SinHardLinksCarreraEntreGoroutinesGanaUna(t *testing.T) {
	errnos := make([]syscall.Errno, 0, len(slNoSoportado))
	for _, e := range slNoSoportado {
		errnos = append(errnos, e.errno)
	}
	llamadas := slSinHardLinks(t, errnos...)
	dir, candado := pnRaiz(t)
	const rondas, competidores = 20, 8
	datosDe := func(ronda, i int) []byte { return pnDatos(256<<10+4096*i, 300*ronda+i) }
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
				t.Fatalf("CA-265: hallazgo cbbc8c: sin hard links, dos publicaciones de %s ganaron a la vez (%d y %d)", nombre, ganador, i)
			case !errors.Is(err, os.ErrExist):
				t.Fatalf("CA-265: hallazgo cbbc8c: sin hard links, el perdedor %d de %s recibe os.ErrExist, fue %v", i, nombre, err)
			}
		}
		if ganador == -1 {
			t.Fatalf("CA-265: hallazgo cbbc8c: sin hard links, una de las %d publicaciones de %s gana: %v", competidores, nombre, errs)
		}
		pnIgual(t, fmt.Sprintf("CA-265: hallazgo cbbc8c: sin hard links, %s es entero el del ganador %d", nombre, ganador), final, datosDe(ronda, ganador))
	}
	slSeLlamo(t, llamadas)
	pnSolo(t, "CA-265: hallazgo cbbc8c: sin hard links", dir, nombres...)
}

const (
	slEnvModo    = "HOOMFS_C1B_CBBC8C_MODO" // "publicar"
	slEnvPath    = "HOOMFS_C1B_CBBC8C_PATH"
	slEnvCandado = "HOOMFS_C1B_CBBC8C_CANDADO"
	slEnvTam     = "HOOMFS_C1B_CBBC8C_TAM"
	slEnvSemilla = "HOOMFS_C1B_CBBC8C_SEMILLA"
	slEnvLimite  = "HOOMFS_C1B_CBBC8C_LIMITE" // RLIMIT_FSIZE que se pone al fallar el enlace; -1 = ninguno
	slEnvErrno   = "HOOMFS_C1B_CBBC8C_ERRNO"  // con que falla el enlace
)

// slResultado es lo que informa el subproceso por stdout (un pipe: el limite
// de tamano no lo alcanza).
type slResultado struct {
	Err       string `json:"err"`
	Existe    bool   `json:"existe"`
	Enlazados int64  `json:"enlazados"`
}

// TestHallazgoCbbc8cSubproceso no es un test: es el subproceso de los tests
// del hallazgo 20261021T174339_cbbc8c (CA-293). Sin la variable de entorno no
// hace nada. Reemplaza enlazar por un enlace que falla con el errno pedido y
// que, la primera vez que se lo llama (el temporal ya esta escrito y
// cerrado), baja RLIMIT_FSIZE con SIGXFSZ ignorada: asi la escritura del
// nombre final con O_EXCL pasa del limite y devuelve EFBIG. Publica una vez.
func TestHallazgoCbbc8cSubproceso(t *testing.T) {
	if os.Getenv(slEnvModo) != "publicar" {
		return
	}
	salir := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "subproceso: "+format+"\n", args...)
		os.Exit(2)
	}
	path, candado := os.Getenv(slEnvPath), os.Getenv(slEnvCandado)
	tam, err1 := strconv.Atoi(os.Getenv(slEnvTam))
	semilla, err2 := strconv.Atoi(os.Getenv(slEnvSemilla))
	limite, err3 := strconv.ParseInt(os.Getenv(slEnvLimite), 10, 64)
	errno, err4 := strconv.Atoi(os.Getenv(slEnvErrno))
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		salir("tam/semilla/limite/errno: %v %v %v %v", err1, err2, err3, err4)
	}
	var lim syscall.Rlimit
	if limite >= 0 {
		signal.Ignore(syscall.SIGXFSZ)
		if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
			salir("getrlimit: %v", err)
		}
		lim.Cur = uint64(limite)
	}
	var llamadas atomic.Int64
	falla := slEnlaceQueFalla(&llamadas, syscall.Errno(errno))
	enlazar = func(viejo, nuevo string) error {
		if llamadas.Load() == 0 && limite >= 0 {
			if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
				salir("setrlimit: %v", err)
			}
		}
		return falla(viejo, nuevo)
	}
	var r slResultado
	if err := PublicarNuevo(path, pnDatos(tam, semilla), 0o644, candado); err != nil {
		r.Err, r.Existe = err.Error(), errors.Is(err, os.ErrExist)
	}
	r.Enlazados = llamadas.Load()
	raw, _ := json.Marshal(r)
	os.Stdout.Write(append([]byte("SL-SALIDA "), append(raw, '\n')...))
	os.Exit(0)
}

// slPublicarEnSubproceso publica tam bytes en path desde un subproceso sin
// hard links (el enlace falla con errno) y, si limite >= 0, con RLIMIT_FSIZE =
// limite desde que falla el enlace.
func slPublicarEnSubproceso(t *testing.T, path, candado string, tam, semilla int, limite int64, errno syscall.Errno) slResultado {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHallazgoCbbc8cSubproceso$", "-test.count=1", "-test.timeout=240s")
	cmd.Env = append(os.Environ(), slEnvModo+"=publicar", slEnvPath+"="+path, slEnvCandado+"="+candado,
		slEnvTam+"="+strconv.Itoa(tam), slEnvSemilla+"="+strconv.Itoa(semilla),
		slEnvLimite+"="+strconv.FormatInt(limite, 10), slEnvErrno+"="+strconv.Itoa(int(errno)))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("el subproceso fallo: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	for _, l := range strings.Split(stdout.String(), "\n") {
		if rest, ok := strings.CutPrefix(l, "SL-SALIDA "); ok {
			var r slResultado
			if err := json.Unmarshal([]byte(rest), &r); err != nil {
				t.Fatalf("la salida del subproceso no es JSON: %v\n%s", err, rest)
			}
			if r.Enlazados == 0 {
				t.Fatalf("fixture: en el subproceso PublicarNuevo nunca llamo a enlazar: el volumen sin hard links no se simulo (%+v)", r)
			}
			return r
		}
	}
	t.Fatalf("el subproceso no informo nada:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	return slResultado{}
}

// Hallazgo 20261021T174339_cbbc8c. CA-293 y CA-265: sin hard links,
// PublicarNuevo crea el nombre final con O_EXCL y escribe; si esa escritura
// falla (a la mitad, en el ultimo byte, en el primero), devuelve el error
// (que no es os.ErrExist: el nombre estaba libre) y borra el archivo que el
// mismo creo: no queda ni el final, ni parcial ni entero, ni un temporal. El
// nombre queda libre: despues se publica entero.
func TestCA293_HallazgoCbbc8c_SinHardLinksFalloDeEscrituraNoDejaNada(t *testing.T) {
	const tam = 1 << 20
	for _, e := range slNoSoportado {
		for k, c := range []struct {
			nombre string
			limite int64
		}{
			{"falla a la mitad", tam / 2},
			{"falla en el ultimo byte", tam - 1},
			{"falla en el primer byte", 0},
		} {
			t.Run(e.nombre+"/"+c.nombre, func(t *testing.T) {
				semilla := 20 + k
				dir, candado := pnRaiz(t)
				final := filepath.Join(dir, "20261021T174339_c1b003.json")
				r := slPublicarEnSubproceso(t, final, candado, tam, semilla, c.limite, e.errno)
				if r.Err == "" {
					t.Fatalf("CA-293: enmienda 1: sin hard links (%s) PublicarNuevo crea el final con O_EXCL y lo escribe; con RLIMIT_FSIZE=%d (< %d bytes) desde que fallo el enlace, esa escritura falla y PublicarNuevo tiene que devolver el error, devolvio nil",
						e.nombre, c.limite, tam)
				}
				if r.Existe {
					t.Errorf("CA-293: hallazgo cbbc8c: un fallo de escritura no es os.ErrExist (el nombre estaba libre): %s", r.Err)
				}
				if info, err := os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
					sz := int64(-1)
					if err == nil {
						sz = info.Size()
					}
					t.Fatalf("CA-293: hallazgo cbbc8c: sin hard links, %s (%s) y quedo un archivo con el nombre final (%d bytes, Lstat = %v): PublicarNuevo borra el archivo que el mismo creo",
						c.nombre, r.Err, sz, err)
				}
				if got := pnEntradas(t, dir); len(got) != 0 {
					t.Fatalf("CA-293: hallazgo cbbc8c: sin hard links, %s y en %s quedaron temporales o parciales: %q", c.nombre, dir, got)
				}
				llamadas := slSinHardLinks(t, e.errno)
				if err := PublicarNuevo(final, pnDatos(tam, semilla), 0o644, candado); err != nil {
					t.Fatalf("CA-293: hallazgo cbbc8c: tras el fallo el nombre no quedo tomado: sin hard links se publica entero sin limite: %v", err)
				}
				slSeLlamo(t, llamadas)
				pnIgual(t, "CA-293: hallazgo cbbc8c: sin hard links", final, pnDatos(tam, semilla))
				pnSolo(t, "CA-293: hallazgo cbbc8c: sin hard links", dir, filepath.Base(final))
			})
		}
	}
}

// GUARDA. Hallazgo 20261021T174339_cbbc8c. CA-293: sin hard links y con
// RLIMIT_FSIZE exactamente igual al tamano desde que falla el enlace, la
// escritura entra y el archivo queda publicado entero, sin temporales.
func TestCA293_HallazgoCbbc8c_SinHardLinksJustoEnElLimitePublica(t *testing.T) {
	const tam = 1 << 20
	for k, e := range slNoSoportado {
		t.Run(e.nombre, func(t *testing.T) {
			semilla := 30 + k
			dir, candado := pnRaiz(t)
			final := filepath.Join(dir, "20261021T174339_c1b004.json")
			if r := slPublicarEnSubproceso(t, final, candado, tam, semilla, tam, e.errno); r.Err != "" {
				t.Fatalf("CA-293: hallazgo cbbc8c: sin hard links (%s), con RLIMIT_FSIZE igual al tamano (%d) el archivo entra: %s", e.nombre, tam, r.Err)
			}
			pnIgual(t, "CA-293: hallazgo cbbc8c: sin hard links", final, pnDatos(tam, semilla))
			pnSolo(t, "CA-293: hallazgo cbbc8c: sin hard links", dir, filepath.Base(final))
		})
	}
}

// GUARDA. Hallazgo 20261021T174339_cbbc8c. CA-265 y CA-293: solo "no
// soportado" (ENOTSUP, EOPNOTSUPP, EPERM) lleva al camino de O_EXCL. Si el
// enlace falla con cualquier otro error (EACCES, EIO, ENOSPC, EROFS),
// PublicarNuevo devuelve ese error (no os.ErrExist), no crea el nombre final
// por otro camino y no deja temporales.
func TestCA265_HallazgoCbbc8c_GuardaOtroErrorDeEnlaceSeDevuelve(t *testing.T) {
	for _, e := range []struct {
		nombre string
		errno  syscall.Errno
	}{
		{"EACCES", syscall.EACCES},
		{"EIO", syscall.EIO},
		{"ENOSPC", syscall.ENOSPC},
		{"EROFS", syscall.EROFS},
	} {
		t.Run(e.nombre, func(t *testing.T) {
			llamadas := slSinHardLinks(t, e.errno)
			dir, candado := pnRaiz(t)
			final := filepath.Join(dir, "precios.yaml")
			err := PublicarNuevo(final, pnDatos(4096, 40), 0o644, candado)
			slSeLlamo(t, llamadas)
			if err == nil {
				t.Fatalf("CA-265: hallazgo cbbc8c: el enlace fallo con %s (no es \"no soportado\"): PublicarNuevo devuelve ese error y no publica por O_EXCL, devolvio nil", e.nombre)
			}
			if !errors.Is(err, e.errno) {
				t.Fatalf("CA-265: hallazgo cbbc8c: el enlace fallo con %s: PublicarNuevo devuelve ese error (errors.Is), devolvio %v", e.nombre, err)
			}
			if errors.Is(err, os.ErrExist) {
				t.Fatalf("CA-265: hallazgo cbbc8c: el enlace fallo con %s y el nombre estaba libre: el error no es os.ErrExist: %v", e.nombre, err)
			}
			if _, lerr := os.Lstat(final); !errors.Is(lerr, os.ErrNotExist) {
				t.Fatalf("CA-293: hallazgo cbbc8c: el enlace fallo con %s: PublicarNuevo no cae al camino de O_EXCL, el nombre final no existe (Lstat = %v)", e.nombre, lerr)
			}
			if got := pnEntradas(t, dir); len(got) != 0 {
				t.Fatalf("CA-293: hallazgo cbbc8c: el enlace fallo con %s y en %s quedaron temporales: %q", e.nombre, dir, got)
			}
		})
	}
}

// Hallazgo 20261021T174339_cbbc8c. CA-265, como propiedad: sin hard links
// (el enlace falla rotando ENOTSUP, EOPNOTSUPP y EPERM), para cualquier
// secuencia de publicaciones sobre unos pocos nombres, la primera de cada
// nombre publica sus datos enteros, todas las siguientes fallan con
// os.ErrExist sin tocarlo, y en el directorio no hay nada mas que los nombres
// publicados.
func TestCA265_HallazgoCbbc8c_SinHardLinksPropiedadLaPrimeraGana(t *testing.T) {
	errnos := make([]syscall.Errno, 0, len(slNoSoportado))
	for _, e := range slNoSoportado {
		errnos = append(errnos, e.errno)
	}
	llamadas := slSinHardLinks(t, errnos...)
	var fallas []string
	prop := func(ops []uint16) bool {
		dir, candado := pnRaiz(t)
		modelo := map[string][]byte{}
		for k, op := range ops {
			nombre := fmt.Sprintf("n%d.yaml", op%4)
			datos := pnDatos(int(op>>2)%5000, 500+k)
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
	if err := quick.Check(prop, &quick.Config{MaxCount: 40}); err != nil {
		t.Fatalf("CA-265: hallazgo cbbc8c: sin hard links, la primera publicacion de cada nombre gana y nada mas queda: %q (%v)", fallas, err)
	}
	slSeLlamo(t, llamadas)
}
