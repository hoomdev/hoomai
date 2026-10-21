// Tests de los hallazgos 20261021T163152_9e2e0d (medium, risk) y
// 20261021T163600_c380a2 (medium, reliability) de la delta 1 de
// fixes-cabina-c1b, sobre hoomfs.PublicarNuevo, con que se publican los
// items (.hoom/specs/items-y-columna-derivada.md, CA-263 y CA-265) y los
// registros de review, que son append-only (CA-293). Escritos desde el
// contrato, nunca desde la implementacion:
//
// PublicarNuevo(path, data, mode, candado) nunca reemplaza un archivo que ya
// esta, aunque lo haya creado un escritor que NO toma el candado (otra
// version de hoom, git, cualquier otro programa). En un filesystem con hard
// links la publicacion es una sola operacion que no pisa. Asi, si un escritor
// ajeno crea el nombre final con O_CREATE|O_EXCL al mismo tiempo que
// PublicarNuevo publica, gana uno solo:
//
//   - si gana el ajeno, PublicarNuevo devuelve un error que es os.ErrExist y
//     el archivo es entero el del ajeno;
//   - si gana PublicarNuevo, el ajeno recibe EEXIST y el archivo es entero el
//     publicado;
//   - nunca hay dos ganadores (el ajeno cree que creo el archivo y
//     PublicarNuevo cree que lo publico) ni se pisa contenido, y no queda
//     ningun temporal.
//
// La carrera se repite muchas rondas. El momento en que el ajeno crea el
// nombre se ajusta solo, ronda a ronda, hacia el punto en que cada uno gana
// la mitad de las veces: ahi es donde PublicarNuevo decide si el nombre esta
// libre y lo ocupa. HOOMFS_C1B_9E2E0D_RONDAS cambia la cantidad de rondas
// (por defecto 6000, 400 con -short) y HOOMFS_C1B_9E2E0D_SEGUNDOS el tope de
// tiempo (por defecto 120).
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
	"strconv"
	"testing"
	"time"
)

// ajDatos son los bytes de un competidor de una ronda: distintos para cada
// ronda y cada competidor, de largo variable.
func ajDatos(quien string, ronda int) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s ronda %d\n", quien, ronda)
	for i := 0; i < 8+ronda%24; i++ {
		fmt.Fprintf(&b, "linea %d de %s en la ronda %d\n", i, quien, ronda)
	}
	return b.Bytes()
}

// ajEntero lee un entero positivo de la variable nombre, o def.
func ajEntero(t *testing.T, nombre string, def int) int {
	t.Helper()
	v := os.Getenv(nombre)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		t.Fatalf("%s=%q no es un entero positivo", nombre, v)
	}
	return n
}

// ajAjeno crea path con O_CREATE|O_EXCL (sin candado) y escribe data. Devuelve
// el error de la creacion; un fallo de escritura despues de crear es fatal
// para la ronda.
func ajAjeno(path string, data []byte) (crear error, escribir error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err, nil
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return nil, werr
}

// ajEspera gira (sin dormir: dormir tiene granularidad de milisegundos) hasta
// que pasen d desde t0.
func ajEspera(t0 time.Time, d time.Duration) {
	for time.Since(t0) < d {
	}
}

// Hallazgos 20261021T163152_9e2e0d y 20261021T163600_c380a2. CA-265 y
// CA-293: en cada ronda un escritor ajeno, sin candado, crea el nombre final
// con O_CREATE|O_EXCL al mismo tiempo que PublicarNuevo lo publica. Gana uno
// solo, el perdedor recibe os.ErrExist / EEXIST, el archivo es entero el del
// ganador, y al final en el directorio solo estan los nombres finales.
func TestCA265_Hallazgo9e2e0d_c380a2_EscritorAjenoSinCandadoNoEsPisado(t *testing.T) {
	// Las dos goroutines tienen que correr a la vez: con GOMAXPROCS=1 la
	// carrera no se da.
	if prev := runtime.GOMAXPROCS(0); prev < 2 {
		runtime.GOMAXPROCS(2)
		defer runtime.GOMAXPROCS(prev)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".hoom", "items")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	candado := LockPath(root, "items")

	rondas := 6000
	if testing.Short() {
		rondas = 400
	}
	rondas = ajEntero(t, "HOOMFS_C1B_9E2E0D_RONDAS", rondas)
	tope := time.Duration(ajEntero(t, "HOOMFS_C1B_9E2E0D_SEGUNDOS", 120)) * time.Second

	// Calibracion: cuanto tarda una publicacion sin competencia.
	var muestras []time.Duration
	for i := 0; i < 40; i++ {
		p := filepath.Join(dir, fmt.Sprintf("calibracion-%02d.yaml", i))
		t0 := time.Now()
		if err := PublicarNuevo(p, ajDatos("calibracion", i), 0o644, candado); err != nil {
			t.Fatalf("CA-263: PublicarNuevo sobre un nombre libre publica: %v", err)
		}
		muestras = append(muestras, time.Since(t0))
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(muestras, func(i, j int) bool { return muestras[i] < muestras[j] })
	mediana := muestras[len(muestras)/2]

	rnd := rand.New(rand.NewSource(0x9e2e0d))
	centro := float64(mediana) // cuando crea el ajeno, desde la largada
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
				t.Errorf("CA-265: hallazgos 9e2e0d/c380a2: ronda %d: gano PublicarNuevo: el ajeno recibe EEXIST, recibio %v", ronda, ea.crear)
			}
			if rerr != nil || !bytes.Equal(got, datosPub) {
				otros++
				t.Errorf("CA-265: hallazgos 9e2e0d/c380a2: ronda %d: gano PublicarNuevo: %s es entero lo publicado (err %v, %d bytes)", ronda, nombre, rerr, len(got))
			}
		case ea.crear == nil:
			ganaAjeno++
			if !errors.Is(ep, os.ErrExist) {
				otros++
				t.Errorf("CA-265: hallazgos 9e2e0d/c380a2: ronda %d: gano el ajeno: PublicarNuevo devuelve un error que es os.ErrExist, devolvio %v", ronda, ep)
			}
			if rerr != nil || !bytes.Equal(got, datosAj) {
				otros++
				t.Errorf("CA-293: hallazgos 9e2e0d/c380a2: ronda %d: gano el ajeno: %s sigue siendo entero el del ajeno, PublicarNuevo no lo pisa (err %v, %d bytes)", ronda, nombre, rerr, len(got))
			}
		default:
			otros++
			t.Errorf("CA-265: hallazgos 9e2e0d/c380a2: ronda %d: alguno de los dos gana: PublicarNuevo %v, ajeno %v", ronda, ep, ea.crear)
		}
		if otros > 20 {
			t.Fatalf("demasiados errores; corto en la ronda %d", ronda)
		}

		// El ajeno crea mas tarde si viene ganando y mas temprano si viene
		// perdiendo: la largada del ajeno se acerca al momento en que
		// PublicarNuevo ocupa el nombre.
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

	t.Logf("hallazgos 9e2e0d/c380a2: %d rondas en %v (mediana de una publicacion sola %v): gano PublicarNuevo %d, gano el ajeno %d, dos ganadores %d",
		hechas, time.Since(inicio).Round(time.Millisecond), mediana, ganaPublicar, ganaAjeno, dobles)
	if hechas >= 100 && (ganaPublicar == 0 || ganaAjeno == 0) && dobles == 0 {
		t.Fatalf("fixture: la carrera no se dio: en %d rondas gano siempre el mismo (PublicarNuevo %d, ajeno %d)", hechas, ganaPublicar, ganaAjeno)
	}
	if dobles > 0 {
		t.Errorf("CA-265 y CA-293: hallazgos 9e2e0d/c380a2: PublicarNuevo reemplazo el archivo que un escritor ajeno (sin candado) creo con O_EXCL en %d de %d rondas; la primera: %s",
			dobles, hechas, primerDoble)
	}

	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	quedan := map[string]bool{}
	for _, e := range es {
		quedan[e.Name()] = true
	}
	for _, n := range nombres {
		if !quedan[n] {
			t.Errorf("CA-265: hallazgos 9e2e0d/c380a2: %s existe al final de su ronda", n)
		}
		delete(quedan, n)
	}
	if len(quedan) > 0 {
		var sobra []string
		for n := range quedan {
			sobra = append(sobra, n)
		}
		sort.Strings(sobra)
		t.Errorf("CA-265: hallazgos 9e2e0d/c380a2: en %s quedan solo los nombres finales, sin temporales: sobran %q", dir, sobra)
	}
}
