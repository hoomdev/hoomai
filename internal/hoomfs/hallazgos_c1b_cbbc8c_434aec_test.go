//go:build !windows

// Tests de hoomfs.PublicarNuevo en un volumen SIN hard links, con que se
// publican los items (.hoom/specs/items-y-columna-derivada.md, CA-263 y
// CA-265) y los registros de review, que son append-only (CA-293). Escritos
// desde la spec, nunca desde la implementacion. Cubren los hallazgos
// 20261021T174339_cbbc8c (high, resilience) y 20261021T173339_434aec (medium,
// risk) de la delta 1 de fixes-cabina-c1b, y 20261021T191854_589ad5 (high,
// risk), 20261021T192837_25172f (high, resilience), 20261021T192448_8c594b
// (medium, reliability) y 20261021T192844_4f802a (medium, resilience) de la
// delta 2.
//
// EL CONTRATO CAMBIO: enmienda 2 de la spec, que reemplaza a la 1 por
// decision de Henry. La version anterior de este archivo exigia la enmienda
// 1: sin hard links, PublicarNuevo creaba el nombre final con O_EXCL y lo
// escribia (visible a medias), y si la escritura fallaba lo borraba. La delta
// 2 mostro que ese respaldo abria otros bordes: el borrado por nombre tras un
// fallo podia borrar el archivo de otro escritor (589ad5, 25172f), el
// temporal y el final necesitaban el doble de espacio (8c594b) y un item
// cortado justo despues de su ultimo campo obligatorio parseaba valido
// (4f802a). Henry eligio quitar el respaldo. Por eso se sacaron de aca las
// exigencias de publicar por O_EXCL (publica entero sin hard links, el
// fallo de escritura borra lo que creo, en el limite de RLIMIT_FSIZE
// publica, de N publicaciones gana una). NO es debilitar los tests: ahora se
// exige algo mas fuerte, que sin hard links no se escriba NADA, y con eso
// los cuatro hallazgos de la delta 2 quedan sin objeto.
//
// El contrato (enmienda 2): PublicarNuevo publica con un enlace (hard link)
// bajo el candado: atomico, entero y sin pisar. Si el enlace falla con "no
// soportado" (ENOTSUP, EOPNOTSUPP o EPERM: exFAT, FAT):
//
//   - devuelve un error que envuelve hoomfs.ErrSinEnlaces (y que lo dice:
//     .hoom necesita un sistema de archivos con hard links);
//   - no escribe nada: no deja el nombre final (ni por un instante), ni
//     temporales, y no toca un nombre que ya estaba (archivo, symlink, aunque
//     apunte a la nada, directorio, FIFO);
//   - un escritor ajeno que crea el nombre con O_EXCL al mismo tiempo gana
//     siempre: PublicarNuevo nunca gana ni toca su archivo;
//   - suelta el candado y deja el nombre libre: con hard links, despues, se
//     publica entero.
//
// Un error de enlace que no es "no soportado" (EACCES, EIO, ENOSPC, EROFS)
// se devuelve tal cual: no es ErrSinEnlaces ni os.ErrExist.
//
// Un volumen sin hard links se simula reemplazando la variable de paquete
// enlazar (os.Link) por un enlace que falla como os.Link (un *os.LinkError)
// con ENOTSUP, EOPNOTSUPP o EPERM. ErrSinEnlaces se usa a traves de
// slErrSinEnlaces (hallazgos_c1b_cbbc8c_434aec_sinenlaces_test.go): en
// 903c6d8 no existe y ese archivo no compila, es lo esperado hasta el
// arreglo.
//
// HOOMFS_C1B_CBBC8C_RONDAS cambia las rondas de la carrera con el escritor
// ajeno (por defecto 6000, 400 con -short) y HOOMFS_C1B_CBBC8C_SEGUNDOS su
// tope de tiempo (por defecto 90).
package hoomfs

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/quick"
	"time"
)

// slNoSoportado son los errores con que falla un enlace en un volumen sin hard
// links: los unicos que llevan a ErrSinEnlaces (enmienda 2). En Linux ENOTSUP
// y EOPNOTSUPP son el mismo numero; en macOS no.
var slNoSoportado = []struct {
	nombre string
	errno  syscall.Errno
}{
	{"ENOTSUP", syscall.ENOTSUP},
	{"EOPNOTSUPP", syscall.EOPNOTSUPP},
	{"EPERM", syscall.EPERM},
}

// slTodosNoSoportado son los errnos de slNoSoportado, para rotarlos.
func slTodosNoSoportado() []syscall.Errno {
	errnos := make([]syscall.Errno, 0, len(slNoSoportado))
	for _, e := range slNoSoportado {
		errnos = append(errnos, e.errno)
	}
	return errnos
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

// slSinHardLinks reemplaza enlazar por un enlace que falla con errnos
// (rotando) hasta que se llama a restaurar o termina el test, y devuelve el
// contador de llamadas. Ningun test de este archivo es paralelo: nadie mas
// publica mientras enlazar esta reemplazado.
func slSinHardLinks(t *testing.T, errnos ...syscall.Errno) (llamadas *atomic.Int64, restaurar func()) {
	t.Helper()
	llamadas = new(atomic.Int64)
	previo := enlazar
	enlazar = slEnlaceQueFalla(llamadas, errnos...)
	var una sync.Once
	restaurar = func() { una.Do(func() { enlazar = previo }) }
	t.Cleanup(restaurar)
	return llamadas, restaurar
}

// slSeLlamo exige que PublicarNuevo haya intentado el enlace (si no, el
// camino sin hard links no se ejercito y el test no prueba nada).
func slSeLlamo(t *testing.T, llamadas *atomic.Int64) {
	t.Helper()
	if llamadas.Load() == 0 {
		t.Fatalf("fixture: PublicarNuevo nunca llamo a enlazar: el volumen sin hard links no se simulo")
	}
}

// slEsSinEnlaces explica por que err no es el error de un volumen sin hard
// links sobre un nombre libre, o "" si lo es: no nil, envuelve ErrSinEnlaces,
// lo dice en su texto, y no es os.ErrExist (el nombre estaba libre).
func slEsSinEnlaces(err error) string {
	switch {
	case err == nil:
		return "devolvio nil (publico)"
	case !errors.Is(err, slErrSinEnlaces):
		return fmt.Sprintf("no envuelve hoomfs.ErrSinEnlaces: %v", err)
	case errors.Is(err, os.ErrExist):
		return fmt.Sprintf("es os.ErrExist con el nombre libre: %v", err)
	case !strings.Contains(err.Error(), slErrSinEnlaces.Error()):
		return fmt.Sprintf("no dice que .hoom necesita hard links (%q): %v", slErrSinEnlaces.Error(), err)
	}
	return ""
}

// slNoEsta exige que el nombre final no exista (Lstat: ni archivo, ni
// symlink, ni nada).
func slNoEsta(t *testing.T, ca, final string) {
	t.Helper()
	if info, err := os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
		sz := int64(-1)
		if err == nil {
			sz = info.Size()
		}
		t.Fatalf("%s: el nombre final %s no existe; Lstat = %v (%d bytes)", ca, final, err, sz)
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
	t.Fatalf("CA-265: sin hard links, PublicarNuevo %s no termina en 30 s", que)
	return nil
}

// Hallazgos 20261021T191854_589ad5, 20261021T192837_25172f,
// 20261021T192448_8c594b y 20261021T192844_4f802a (enmienda 2). CA-263 y
// CA-293: sin hard links (el enlace falla con ENOTSUP, EOPNOTSUPP o EPERM),
// PublicarNuevo sobre un nombre libre devuelve un error que envuelve
// ErrSinEnlaces y no escribe nada: ni el nombre final (entero, parcial o
// vacio), ni temporales, para datos vacios, de un byte, unicode, binarios y
// grandes. Intentarlo otra vez da lo mismo y no acumula nada. Guarda: el
// candado quedo suelto y el nombre libre, asi que con hard links, despues, se
// publica entero.
func TestCA263_Hallazgos589ad5_25172f_8c594b_4f802a_SinHardLinksNoEscribeNada(t *testing.T) {
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
				llamadas, restaurar := slSinHardLinks(t, e.errno)
				dir, candado := pnRaiz(t)
				final := filepath.Join(dir, "precios.yaml")
				for intento := 1; intento <= 3; intento++ {
					err := PublicarNuevo(final, c.datos, c.modo, candado)
					slSeLlamo(t, llamadas)
					if por := slEsSinEnlaces(err); por != "" {
						t.Fatalf("CA-263: enmienda 2, hallazgos 589ad5/25172f: sin hard links (%s), intento %d, PublicarNuevo sobre un nombre libre falla con ErrSinEnlaces y no publica por otro camino: %s",
							e.nombre, intento, por)
					}
					slNoEsta(t, fmt.Sprintf("CA-263: enmienda 2, hallazgos 589ad5/4f802a: sin hard links (%s), intento %d, no se escribe nada", e.nombre, intento), final)
					if got := pnEntradas(t, dir); len(got) != 0 {
						t.Fatalf("CA-263: enmienda 2, hallazgo 8c594b: sin hard links (%s), intento %d, no quedan temporales ni parciales en %s: %q",
							e.nombre, intento, dir, got)
					}
				}

				// guarda: el candado quedo suelto y el nombre libre
				restaurar()
				if err := PublicarNuevo(final, c.datos, c.modo, candado); err != nil {
					t.Fatalf("CA-263: guarda: tras fallar sin hard links el candado se suelta y el nombre queda libre: con hard links se publica: %v", err)
				}
				pnIgual(t, "CA-263: guarda: con hard links", final, c.datos)
				pnSolo(t, "CA-263: guarda: con hard links", dir, "precios.yaml")
			})
		}
	}
}

// Hallazgos 20261021T174339_cbbc8c, 20261021T191854_589ad5 y
// 20261021T192837_25172f (enmienda 2). CA-265 y CA-293: sin hard links, si el
// nombre final ya esta tomado (un archivo de otro, un archivo vacio, un
// symlink a un archivo, un symlink que no apunta a nada, un directorio vacio
// o con algo adentro, un FIFO), PublicarNuevo falla (con ErrSinEnlaces u
// os.ErrExist) y no lo toca: no lo trunca, no lo reemplaza ni lo borra, no
// escribe a traves del symlink (ni crea su destino), no se bloquea en el FIFO
// y no deja temporales.
func TestCA265_Hallazgos589ad5_25172f_SinHardLinksNombreOcupadoQuedaIntacto(t *testing.T) {
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
			pnIgual(t, "CA-265: hallazgo 589ad5: el archivo del otro escritor", final, []byte("titulo: del otro escritor\n"))
		}},
		{"archivo vacio de otro escritor", func(t *testing.T, final, _ string) {
			if err := os.WriteFile(final, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			pnIgual(t, "CA-265: hallazgo 589ad5: el archivo vacio del otro escritor sigue vacio", final, []byte{})
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
				t.Fatalf("CA-265: hallazgo 589ad5: el symlink sigue apuntando a %s: %q %v", afuera, dest, err)
			}
			pnIgual(t, "CA-265: hallazgo 589ad5: no escribe a traves del symlink: su destino", afuera, []byte("el destino"))
		}},
		{"symlink que no apunta a nada", func(t *testing.T, final, afuera string) {
			if err := os.Symlink(afuera, final); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, afuera string) {
			if dest, err := os.Readlink(final); err != nil || dest != afuera {
				t.Fatalf("CA-265: hallazgo 589ad5: el symlink sigue apuntando a %s: %q %v", afuera, dest, err)
			}
			if _, err := os.Lstat(afuera); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("CA-265: hallazgo 589ad5: PublicarNuevo no crea el destino del symlink %s, Lstat = %v", afuera, err)
			}
		}},
		{"directorio vacio", func(t *testing.T, final, _ string) {
			if err := os.Mkdir(final, 0o755); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			if es, err := os.ReadDir(final); err != nil || len(es) != 0 {
				t.Fatalf("CA-265: hallazgo 589ad5: el directorio sigue ahi y vacio: %v %v", es, err)
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
			if es, err := os.ReadDir(final); err != nil || len(es) != 1 {
				t.Fatalf("CA-265: hallazgo 589ad5: el directorio sigue con lo suyo y nada mas: %v %v", es, err)
			}
			pnIgual(t, "CA-265: hallazgo 589ad5: lo de adentro del directorio", filepath.Join(final, "adentro"), []byte("intacto"))
		}},
		{"FIFO", func(t *testing.T, final, _ string) {
			if err := syscall.Mkfifo(final, 0o644); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final, _ string) {
			if info, err := os.Lstat(final); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
				t.Fatalf("CA-265: hallazgo 589ad5: el FIFO sigue ahi: %v %v", info, err)
			}
		}},
	}
	for _, e := range slNoSoportado {
		for _, c := range casos {
			t.Run(e.nombre+"/"+c.nombre, func(t *testing.T) {
				llamadas, _ := slSinHardLinks(t, e.errno)
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
				if err == nil {
					t.Fatalf("CA-265: enmienda 2: sin hard links (%s), con el nombre tomado por un %s PublicarNuevo falla, devolvio nil", e.nombre, c.nombre)
				}
				if !errors.Is(err, slErrSinEnlaces) && !errors.Is(err, os.ErrExist) {
					t.Fatalf("CA-265: enmienda 2: sin hard links (%s), con el nombre tomado por un %s el error es ErrSinEnlaces u os.ErrExist, fue %v", e.nombre, c.nombre, err)
				}
				t.Logf("enlazar llamado %d veces; error: %v", llamadas.Load(), err)
				despues, err := os.Lstat(final)
				if err != nil || !os.SameFile(antes, despues) || despues.Mode() != antes.Mode() || despues.Size() != antes.Size() {
					t.Fatalf("CA-265: hallazgos 589ad5/25172f: sin hard links, el %s que ocupaba el nombre no se reemplaza, ni se borra, ni cambia: %v %v", c.nombre, despues, err)
				}
				c.sigue(t, final, afuera)
				pnSolo(t, "CA-265: hallazgo 8c594b: sin hard links, el intento fallido no deja temporales", dir, "precios.yaml")
			})
		}
	}
}

// Hallazgos 20261021T174339_cbbc8c, 20261021T173339_434aec,
// 20261021T191854_589ad5 y 20261021T192837_25172f (enmienda 2). CA-265 y
// CA-293: sin hard links, en cada ronda un escritor ajeno, sin candado, crea
// el nombre final con O_CREATE|O_EXCL y lo escribe al mismo tiempo que
// PublicarNuevo intenta publicarlo. El ajeno gana SIEMPRE: su O_EXCL nunca
// encuentra el nombre tomado (PublicarNuevo no lo crea ni por un instante),
// PublicarNuevo nunca devuelve nil (falla con ErrSinEnlaces u os.ErrExist) y
// el archivo queda entero el del ajeno (no lo pisa, no lo trunca, no lo
// borra). Al final solo quedan los nombres del ajeno, sin temporales. El
// momento en que crea el ajeno se reparte al azar a lo largo de lo que tarda
// un intento de PublicarNuevo, y se exige que las dos cosas se hayan
// solapado en el tiempo (si no, la carrera no se dio).
func TestCA265_Hallazgos589ad5_25172f_434aec_SinHardLinksElAjenoSiempreGana(t *testing.T) {
	if prev := runtime.GOMAXPROCS(0); prev < 2 {
		runtime.GOMAXPROCS(2)
		defer runtime.GOMAXPROCS(prev)
	}
	llamadas, _ := slSinHardLinks(t, slTodosNoSoportado()...)
	dir, candado := pnRaiz(t)

	rondas := 6000
	if testing.Short() {
		rondas = 400
	}
	rondas = ajEntero(t, "HOOMFS_C1B_CBBC8C_RONDAS", rondas)
	tope := time.Duration(ajEntero(t, "HOOMFS_C1B_CBBC8C_SEGUNDOS", 90)) * time.Second

	// Calibracion: cuanto tarda un intento sin hard links y sin competencia.
	// Cada uno falla con ErrSinEnlaces y no deja nada.
	var muestras []time.Duration
	for i := 0; i < 40; i++ {
		p := filepath.Join(dir, fmt.Sprintf("calibracion-%02d.yaml", i))
		t0 := time.Now()
		err := PublicarNuevo(p, ajDatos("calibracion", i), 0o644, candado)
		muestras = append(muestras, time.Since(t0))
		if por := slEsSinEnlaces(err); por != "" {
			t.Fatalf("CA-263: enmienda 2: sin hard links, PublicarNuevo sobre un nombre libre falla con ErrSinEnlaces: %s", por)
		}
		if got := pnEntradas(t, dir); len(got) != 0 {
			t.Fatalf("CA-263: enmienda 2: sin hard links no queda nada en %s: %q", dir, got)
		}
	}
	slSeLlamo(t, llamadas)
	sort.Slice(muestras, func(i, j int) bool { return muestras[i] < muestras[j] })
	mediana := muestras[len(muestras)/2]

	rnd := rand.New(rand.NewSource(0x589ad5))
	var (
		hechas, solapadas, publico, ocupado, pisado, otros int
		nombres                                            []string
		primera                                            string
	)
	falla := func(format string, args ...any) {
		otros++
		if primera == "" {
			primera = fmt.Sprintf(format, args...)
		}
		t.Errorf(format, args...)
	}
	inicio := time.Now()
	for ronda := 0; ronda < rondas && time.Since(inicio) < tope; ronda++ {
		nombre := fmt.Sprintf("r%05d.yaml", ronda)
		final := filepath.Join(dir, nombre)
		nombres = append(nombres, nombre)
		datosPub, datosAj := ajDatos("publicar", ronda), ajDatos("ajeno", ronda)
		var d time.Duration
		if rnd.Intn(10) != 0 {
			d = time.Duration(rnd.Float64() * 1.5 * float64(mediana))
		}

		largada := make(chan struct{})
		var t0 time.Time
		type resPub struct {
			err    error
			p0, p1 time.Time
		}
		type resAj struct {
			crear, escribir error
			a0, a1          time.Time
		}
		chPub := make(chan resPub, 1)
		chAj := make(chan resAj, 1)
		listos := make(chan struct{}, 2)
		go func() {
			listos <- struct{}{}
			<-largada
			p0 := time.Now()
			err := PublicarNuevo(final, datosPub, 0o644, candado)
			chPub <- resPub{err, p0, time.Now()}
		}()
		go func() {
			listos <- struct{}{}
			<-largada
			ajEspera(t0, d)
			a0 := time.Now()
			c, w := ajAjeno(final, datosAj)
			chAj <- resAj{c, w, a0, time.Now()}
		}()
		<-listos
		<-listos
		t0 = time.Now()
		close(largada)
		rp, ra := <-chPub, <-chAj
		hechas++
		if ra.a0.Before(rp.p1) && ra.a1.After(rp.p0) {
			solapadas++
		}

		if ra.escribir != nil {
			t.Fatalf("ronda %d: el ajeno creo %s pero no pudo escribirlo: %v", ronda, nombre, ra.escribir)
		}
		if rp.err == nil {
			publico++
			falla("CA-265: enmienda 2, hallazgos 589ad5/25172f: sin hard links, ronda %d: PublicarNuevo devolvio nil (publico) con el ajeno creando %s a los %v de la largada; nunca gana",
				ronda, nombre, d)
		} else if !errors.Is(rp.err, slErrSinEnlaces) && !errors.Is(rp.err, os.ErrExist) {
			falla("CA-265: enmienda 2: sin hard links, ronda %d: PublicarNuevo falla con ErrSinEnlaces u os.ErrExist, fallo con %v", ronda, rp.err)
		}
		if ra.crear != nil {
			ocupado++
			falla("CA-265: enmienda 2, hallazgos 589ad5/25172f: sin hard links, ronda %d: el ajeno no pudo crear %s con O_EXCL (a los %v de la largada): PublicarNuevo ocupo el nombre: %v",
				ronda, nombre, d, ra.crear)
		} else if got, err := os.ReadFile(final); err != nil || !bytes.Equal(got, datosAj) {
			pisado++
			falla("CA-293: hallazgos 434aec/589ad5/25172f: sin hard links, ronda %d: %s es entero el del ajeno, PublicarNuevo no lo pisa, no lo trunca ni lo borra (err %v, %d bytes, publicado=%v)",
				ronda, nombre, err, len(got), bytes.Equal(got, datosPub))
		}
		if otros > 20 {
			t.Fatalf("demasiados errores; corto en la ronda %d; el primero: %s", ronda, primera)
		}
	}

	t.Logf("enmienda 2: sin hard links, %d rondas en %v (mediana de un intento solo %v): solapadas %d; PublicarNuevo publico %d, ocupo el nombre %d, el del ajeno no quedo entero %d",
		hechas, time.Since(inicio).Round(time.Millisecond), mediana, solapadas, publico, ocupado, pisado)
	if hechas >= 100 && solapadas == 0 {
		t.Fatalf("fixture: la carrera no se dio: en %d rondas el ajeno nunca creo el nombre mientras PublicarNuevo corria", hechas)
	}
	pnSolo(t, "CA-265: hallazgo 8c594b: sin hard links, solo los nombres del ajeno, sin temporales", dir, nombres...)
}

// Hallazgos 20261021T191854_589ad5 y 20261021T192844_4f802a (enmienda 2).
// CA-263: sin hard links, el nombre final nunca aparece, ni por un instante:
// un observador que mira el nombre sin parar mientras PublicarNuevo intenta
// publicar 1 MiB no lo ve nunca (ni vacio, ni a medias, ni entero). Sin
// enlace no hay publicacion a medias que un lector pueda tomar por un item.
func TestCA263_Hallazgos589ad5_4f802a_SinHardLinksElNombreNuncaAparece(t *testing.T) {
	if prev := runtime.GOMAXPROCS(0); prev < 2 {
		runtime.GOMAXPROCS(2)
		defer runtime.GOMAXPROCS(prev)
	}
	llamadas, _ := slSinHardLinks(t, slTodosNoSoportado()...)
	dir, candado := pnRaiz(t)
	rondas := 120
	if testing.Short() {
		rondas = 30
	}
	var vistas, miradas int64
	var primera string
	for ronda := 0; ronda < rondas; ronda++ {
		final := filepath.Join(dir, fmt.Sprintf("%02d-precios.yaml", ronda))
		datos := pnDatos(1<<20, 600+ronda)
		var parar atomic.Bool
		var wg sync.WaitGroup
		var visto atomic.Int64
		var miro atomic.Int64
		var tam atomic.Int64
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !parar.Load() {
				miro.Add(1)
				if info, err := os.Lstat(final); err == nil {
					if visto.Add(1) == 1 {
						tam.Store(info.Size())
					}
				}
			}
		}()
		err := PublicarNuevo(final, datos, 0o644, candado)
		parar.Store(true)
		wg.Wait()
		vistas += visto.Load()
		miradas += miro.Load()
		if visto.Load() > 0 && primera == "" {
			primera = fmt.Sprintf("ronda %d: %s se vio %d veces (la primera con %d de %d bytes; PublicarNuevo devolvio %v)",
				ronda, filepath.Base(final), visto.Load(), tam.Load(), len(datos), err)
		}
		if vistas > 0 {
			t.Fatalf("CA-263: enmienda 2, hallazgos 589ad5/4f802a: sin hard links el nombre final no aparece nunca, ni por un instante; aparecio: %s", primera)
		}
		if por := slEsSinEnlaces(err); por != "" {
			t.Fatalf("CA-263: enmienda 2: sin hard links, ronda %d, PublicarNuevo falla con ErrSinEnlaces: %s", ronda, por)
		}
		slNoEsta(t, fmt.Sprintf("CA-263: enmienda 2: sin hard links, ronda %d", ronda), final)
	}
	slSeLlamo(t, llamadas)
	t.Logf("enmienda 2: sin hard links, %d rondas, %d miradas al nombre final, ninguna lo vio", rondas, miradas)
	if miradas < int64(rondas) {
		t.Fatalf("fixture: el observador casi no miro (%d miradas en %d rondas): el test no prueba nada", miradas, rondas)
	}
	if got := pnEntradas(t, dir); len(got) != 0 {
		t.Fatalf("CA-263: enmienda 2, hallazgo 8c594b: sin hard links no queda nada en %s: %q", dir, got)
	}
}

// Hallazgos 20261021T174339_cbbc8c y 20261021T191854_589ad5 (enmienda 2).
// CA-265: sin hard links, N publicaciones simultaneas del mismo nombre desde
// goroutines: ninguna gana, todas fallan con ErrSinEnlaces (nadie creo el
// nombre, asi que ninguna es os.ErrExist), y cuando todas terminaron no hay
// nada: ni el nombre ni temporales.
func TestCA265_Hallazgos589ad5_8c594b_SinHardLinksCarreraEntreGoroutinesNingunaGana(t *testing.T) {
	llamadas, _ := slSinHardLinks(t, slTodosNoSoportado()...)
	dir, candado := pnRaiz(t)
	const rondas, competidores = 20, 8
	datosDe := func(ronda, i int) []byte { return pnDatos(256<<10+4096*i, 300*ronda+i) }
	for ronda := 0; ronda < rondas; ronda++ {
		final := filepath.Join(dir, fmt.Sprintf("mismo-%02d.yaml", ronda))
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
		for i, err := range errs {
			if por := slEsSinEnlaces(err); por != "" {
				t.Fatalf("CA-265: enmienda 2: sin hard links, la publicacion %d de %s falla con ErrSinEnlaces: %s", i, filepath.Base(final), por)
			}
		}
		slNoEsta(t, fmt.Sprintf("CA-265: enmienda 2: sin hard links, ronda %d", ronda), final)
		if got := pnEntradas(t, dir); len(got) != 0 {
			t.Fatalf("CA-265: enmienda 2, hallazgo 8c594b: sin hard links, ronda %d, no queda nada en %s: %q", ronda, dir, got)
		}
	}
	slSeLlamo(t, llamadas)
}

// GUARDA. Hallazgo 20261021T174339_cbbc8c (enmienda 2). CA-265 y CA-293: solo
// "no soportado" (ENOTSUP, EOPNOTSUPP, EPERM) es ErrSinEnlaces. Si el enlace
// falla con cualquier otro error (EACCES, EIO, ENOSPC, EROFS), PublicarNuevo
// devuelve ese error tal cual (errors.Is), que no es ErrSinEnlaces ni
// os.ErrExist, no crea el nombre final por otro camino y no deja temporales.
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
			llamadas, _ := slSinHardLinks(t, e.errno)
			dir, candado := pnRaiz(t)
			final := filepath.Join(dir, "precios.yaml")
			err := PublicarNuevo(final, pnDatos(4096, 40), 0o644, candado)
			slSeLlamo(t, llamadas)
			if err == nil {
				t.Fatalf("CA-265: hallazgo cbbc8c: el enlace fallo con %s (no es \"no soportado\"): PublicarNuevo devuelve ese error y no publica por otro camino, devolvio nil", e.nombre)
			}
			if !errors.Is(err, e.errno) {
				t.Fatalf("CA-265: hallazgo cbbc8c: el enlace fallo con %s: PublicarNuevo devuelve ese error tal cual (errors.Is), devolvio %v", e.nombre, err)
			}
			if errors.Is(err, slErrSinEnlaces) {
				t.Fatalf("CA-265: enmienda 2: el enlace fallo con %s, que no es \"no soportado\": el error no es ErrSinEnlaces (no dice que faltan hard links): %v", e.nombre, err)
			}
			if errors.Is(err, os.ErrExist) {
				t.Fatalf("CA-265: hallazgo cbbc8c: el enlace fallo con %s y el nombre estaba libre: el error no es os.ErrExist: %v", e.nombre, err)
			}
			slNoEsta(t, fmt.Sprintf("CA-293: hallazgo cbbc8c: el enlace fallo con %s: PublicarNuevo no publica por otro camino", e.nombre), final)
			if got := pnEntradas(t, dir); len(got) != 0 {
				t.Fatalf("CA-293: hallazgo cbbc8c: el enlace fallo con %s y en %s quedaron temporales: %q", e.nombre, dir, got)
			}
		})
	}
}

// Hallazgos 20261021T174339_cbbc8c, 20261021T191854_589ad5 y
// 20261021T192837_25172f (enmienda 2). CA-265 y CA-293, como propiedad: sin
// hard links (el enlace falla rotando ENOTSUP, EOPNOTSUPP y EPERM), para
// cualquier secuencia de operaciones sobre unos pocos nombres, donde cada
// una es o un escritor ajeno que crea el nombre (si esta libre) o
// PublicarNuevo: PublicarNuevo nunca publica (sobre un nombre libre falla
// con ErrSinEnlaces; sobre uno tomado, con ErrSinEnlaces u os.ErrExist), los
// archivos del ajeno quedan byte a byte como los dejo, y en el directorio no
// hay nada mas que lo que creo el ajeno.
func TestCA265_Hallazgos589ad5_25172f_SinHardLinksPropiedadNoEscribeNada(t *testing.T) {
	llamadas, _ := slSinHardLinks(t, slTodosNoSoportado()...)
	var fallas []string
	prop := func(ops []uint16) bool {
		dir, candado := pnRaiz(t)
		ajeno := map[string][]byte{}
		for k, op := range ops {
			nombre := fmt.Sprintf("n%d.yaml", op%4)
			p := filepath.Join(dir, nombre)
			datos := pnDatos(int(op>>3)%5000, 500+k)
			if op&4 != 0 {
				if _, ok := ajeno[nombre]; ok {
					continue
				}
				if c, w := ajAjeno(p, datos); c != nil || w != nil {
					fallas = append(fallas, fmt.Sprintf("op %d: el ajeno no pudo crear %s, que estaba libre: PublicarNuevo lo ocupo (%v %v)", k, nombre, c, w))
					return false
				}
				ajeno[nombre] = datos
				continue
			}
			err := PublicarNuevo(p, datos, 0o644, candado)
			if previo, ok := ajeno[nombre]; ok {
				if err == nil || (!errors.Is(err, slErrSinEnlaces) && !errors.Is(err, os.ErrExist)) {
					fallas = append(fallas, fmt.Sprintf("op %d: %s era del ajeno: ErrSinEnlaces u os.ErrExist, fue %v", k, nombre, err))
					return false
				}
				if got, _ := os.ReadFile(p); !bytes.Equal(got, previo) {
					fallas = append(fallas, fmt.Sprintf("op %d: %s del ajeno se toco: %d de %d bytes", k, nombre, len(got), len(previo)))
					return false
				}
				continue
			}
			if por := slEsSinEnlaces(err); por != "" {
				fallas = append(fallas, fmt.Sprintf("op %d: %s estaba libre: %s", k, nombre, por))
				return false
			}
			if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
				fallas = append(fallas, fmt.Sprintf("op %d: %s estaba libre y quedo algo con ese nombre (Lstat = %v)", k, nombre, err))
				return false
			}
		}
		var want []string
		for n, d := range ajeno {
			want = append(want, n)
			if got, _ := os.ReadFile(filepath.Join(dir, n)); !bytes.Equal(got, d) {
				fallas = append(fallas, fmt.Sprintf("%s del ajeno no quedo entero: %d de %d bytes", n, len(got), len(d)))
				return false
			}
		}
		sort.Strings(want)
		if got := pnEntradas(t, dir); strings.Join(got, "|") != strings.Join(want, "|") {
			fallas = append(fallas, fmt.Sprintf("en el directorio quedan %q, esperaba solo lo del ajeno %q", got, want))
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 40}); err != nil {
		t.Fatalf("CA-265: enmienda 2, hallazgos 589ad5/25172f: sin hard links PublicarNuevo no escribe nada y no toca lo ajeno: %q (%v)", fallas, err)
	}
	slSeLlamo(t, llamadas)
}
