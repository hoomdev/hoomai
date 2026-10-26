//go:build darwin || linux

// Test de los hallazgos 83b100 y 68b1e2 (delta 2 completa de la review de
// historia-en-el-tablero, ronda 6 del test-writer): con muchos archivos de
// hallazgo ilegibles de nombres distintos en un arbol, juntar sus fallas no
// puede costar O(N^2). Build y el doctor del proyecto responden en un costo
// que crece lineal con N; el doctor falla cerrado nombrando cada falla y
// Build las trae como advertencias, una por archivo, sin repetidas. Escrito
// desde el spec .hoom/specs/historia-en-el-tablero.md, el contrato del
// doctor que falla cerrado (endurecimiento-1, d0d331) y el texto de los
// hallazgos, sin leer la implementacion. Todo identificador de este archivo
// lleva el prefijo h6.
//
// El costo se mide en tiempo de CPU del proceso (getrusage: usuario mas
// sistema; por eso solo darwin y linux), no en tiempo de reloj: con 'go test
// ./...' otros paquetes corren a la vez y le roban CPU a este, lo que infla
// el reloj pero casi no el CPU propio. Ningun test de este paquete es
// paralelo, asi que el CPU del proceso es el de lo que se mide. Las
// mediciones de N y de h6Factor*N se intercalan (dos arboles) y de cada una se toma
// el minimo de hasta h6Vueltas vueltas.
package boardcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"
	"time"
)

const (
	// h6N es el tamano chico; el grande es h6Factor veces mas. Con estos
	// tamanos la parte cuadratica pesa en el grande lo bastante para verse
	// por encima de la lineal (abrir y leer cada archivo), que la diluye.
	h6N      = 8000
	h6Factor = 8
	// h6Tope es cuanto puede crecer el costo de Build o del doctor al pasar
	// de h6N a h6Factor*h6N archivos. Lineal crece unas 8 veces; cuadratico
	// puro, 64, y diluido por la parte lineal, unas 25 o mas. Calibrado
	// (CPU, minimo de vueltas, darwin arm64): la correccion (ac73d5f) da
	// x8,0 a x8,3, normal y con -race; el costo cuadratico (f3a79d3) da
	// x26,6 a x26,9, y x34,6 con -race. El tope deja margen de los dos
	// lados (1,7 veces por arriba de lo lineal, 1,9 por debajo de lo
	// cuadratico).
	h6Tope = 14.0
	// h6Perdido: un costo que ya crecio mas de h6Perdido veces el tope no va
	// a bajar del tope en otra vuelta (el CPU propio varia unos puntos), asi
	// que se deja de medir: el test falla sin pagar el cuadratico de nuevo.
	h6Perdido = 1.5
	// h6Vueltas es cuantas veces, a lo sumo, se mide cada costo.
	h6Vueltas = 3
)

// h6Nombre es el archivo de hallazgo ilegible numero i.
func h6Nombre(i int) string { return fmt.Sprintf("20261024T000000_%06d.json", i) }

// h6NombreRe encuentra esos nombres en un texto.
var h6NombreRe = regexp.MustCompile(`20261024T000000_[0-9]{6}\.json`)

// h6Escribir arma un proyecto con un item y n archivos de hallazgo ilegibles
// de nombres distintos.
func h6Escribir(t *testing.T, n int) string {
	t.Helper()
	root := bdRepo(t, htBlockOn)
	bdEscribir(t, root, ".hoom/items/x.yaml", htItemYAML("x", "feature", ""))
	dir := filepath.Join(root, ".hoom", "findings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(dir, h6Nombre(i)), []byte("{esto no es un hallazgo"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// h6CPU es el tiempo de CPU del proceso hasta ahora (usuario mas sistema).
func h6CPU(t *testing.T) time.Duration {
	t.Helper()
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		t.Fatal(err)
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// h6Medir corre f y devuelve su costo en CPU y en reloj.
func h6Medir(t *testing.T, f func()) (cpu, reloj time.Duration) {
	t.Helper()
	c0, r0 := h6CPU(t), time.Now()
	f()
	return h6CPU(t) - c0, time.Since(r0)
}

// h6Nombres exige que los textos nombren, entre todos, exactamente los
// archivos [0, n), cada uno una sola vez.
func h6Nombres(t *testing.T, ca, que string, n int, textos ...string) {
	t.Helper()
	vistos := map[string]int{}
	for _, s := range textos {
		for _, m := range h6NombreRe.FindAllString(s, -1) {
			vistos[m]++
		}
	}
	for i := 0; i < n; i++ {
		switch vistos[h6Nombre(i)] {
		case 1:
		case 0:
			t.Fatalf("%s: %s nombra cada archivo ilegible: falta %s", ca, que, h6Nombre(i))
		default:
			t.Fatalf("%s: %s nombra cada archivo ilegible UNA vez: %s aparece %d veces", ca, que, h6Nombre(i), vistos[h6Nombre(i)])
		}
	}
	if len(vistos) != n {
		t.Fatalf("%s: %s nombra %d archivos ilegibles distintos, no %d", ca, que, len(vistos), n)
	}
}

// h6Build es el contrato de Build con n archivos ilegibles: el tablero, con
// una advertencia por archivo, sin repetidas.
func h6Build(t *testing.T, ca string, b Board, err error, n int) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: Build con %d hallazgos ilegibles da el tablero (con advertencias): %v", ca, n, err)
	}
	if len(b.Warnings) != n {
		t.Fatalf("%s: Build trae una advertencia por archivo ilegible (%d), trajo %d", ca, n, len(b.Warnings))
	}
	repetidas := map[string]bool{}
	for _, w := range b.Warnings {
		if repetidas[w] {
			t.Fatalf("%s: Build no repite advertencias: %q", ca, w)
		}
		repetidas[w] = true
	}
	h6Nombres(t, ca, "Build (sus advertencias)", n, b.Warnings...)
}

// h6Doctor es el contrato del doctor del proyecto con n archivos ilegibles:
// falla cerrado (un error, ningun reporte) y su error los nombra a todos.
func h6Doctor(t *testing.T, ca string, r DoctorReport, err error, n int) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: con %d hallazgos ilegibles el doctor del proyecto falla cerrado (un error), no da un reporte de %d problemas",
			ca, n, len(r.Problems))
	}
	h6Nombres(t, ca, "el error del doctor", n, err.Error())
}

// h6Op es Build o el doctor sobre un arbol. medir la corre una vez sobre
// root (con n archivos ilegibles), devuelve su CPU y su reloj y, si
// conContrato, le exige al resultado el contrato de esa operacion.
type h6Op struct {
	que     string
	medir   func(t *testing.T, ca, root string, n int, conContrato bool) (cpu, reloj time.Duration)
	chico   time.Duration // el minimo de CPU con h6N
	grande  time.Duration // el minimo de CPU con h6Factor*h6N
	relojes []string
}

// h6NuevaOp ata una operacion a su validador por el tipo de su resultado R:
// cada pareja queda explicita, y una equivocada (Build con el contrato del
// doctor) no compila.
func h6NuevaOp[R any](que string, correr func(root string) (R, error),
	exigir func(t *testing.T, ca string, res R, err error, n int)) *h6Op {
	return &h6Op{que: que, medir: func(t *testing.T, ca, root string, n int, conContrato bool) (time.Duration, time.Duration) {
		t.Helper()
		var res R
		var err error
		cpu, reloj := h6Medir(t, func() { res, err = correr(root) })
		if conContrato {
			exigir(t, ca, res, err, n)
		}
		return cpu, reloj
	}}
}

// CA-452 (y el doctor que falla cerrado), hallazgos 83b100 y 68b1e2: con
// h6N y con h6Factor*h6N archivos de hallazgo ilegibles de nombres distintos
// en .hoom/findings, Build y el doctor del proyecto cumplen el contrato
// (advertencias una por archivo sin repetidas; el doctor falla cerrado y los
// nombra a todos), y su costo crece lineal: al multiplicar N por h6Factor,
// su CPU no crece mas de h6Tope veces. Cada costo es el minimo de hasta
// h6Vueltas vueltas intercaladas; se deja de medir en cuanto el resultado es
// claro (todos por debajo del tope, o alguno a mas de h6Perdido veces el
// tope).
func TestCA452_Hallazgo83b100_FallasEnTiempoLineal(t *testing.T) {
	if testing.Short() {
		t.Skip("mide costos con decenas de miles de archivos")
	}
	ca := "CA-452 (83b100, 68b1e2)"
	grande := h6Factor * h6N
	rootN := h6Escribir(t, h6N)
	rootG := h6Escribir(t, grande)

	ops := []*h6Op{
		h6NuevaOp("Build", func(root string) (Board, error) { return Build(root, "main", "high", htAhora) }, h6Build),
		h6NuevaOp("el doctor del proyecto", func(root string) (DoctorReport, error) { return Doctor(root, "main", "high", htAhora) }, h6Doctor),
	}
	for vuelta := 1; vuelta <= h6Vueltas; vuelta++ {
		for _, op := range ops {
			for _, tam := range []struct {
				root string
				n    int
				min  *time.Duration
			}{{rootN, h6N, &op.chico}, {rootG, grande, &op.grande}} {
				cpu, reloj := op.medir(t, fmt.Sprintf("%s con %d", ca, tam.n), tam.root, tam.n, vuelta == 1)
				if vuelta == 1 || cpu < *tam.min {
					*tam.min = cpu
				}
				op.relojes = append(op.relojes, fmt.Sprintf("%d:%v", tam.n, reloj.Round(time.Millisecond)))
			}
		}
		listo, perdido := true, false
		for _, op := range ops {
			r := float64(op.grande) / float64(op.chico)
			listo = listo && r <= h6Tope
			perdido = perdido || r > h6Perdido*h6Tope
		}
		if listo || perdido {
			break
		}
	}
	for _, op := range ops {
		r := float64(op.grande) / float64(op.chico)
		t.Logf("%s: %s: CPU %v con %d y %v con %d: x%.1f (relojes %v)", ca, op.que, op.chico, h6N, op.grande, grande, r, op.relojes)
		if r > h6Tope {
			t.Errorf("%s: %s crece lineal con los hallazgos ilegibles: su CPU con %d fue %v y con %d, %v: x%.1f (tope x%.0f; lineal es x%d, cuadratico x%d)",
				ca, op.que, h6N, op.chico, grande, op.grande, r, h6Tope, h6Factor, h6Factor*h6Factor)
		}
	}
}
