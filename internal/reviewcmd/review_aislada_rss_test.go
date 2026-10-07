// Hallazgo 78162b (tarea estabilizar-ci) y los de su review, 00cbcf y 24067b:
// con que memoria se juzga al hoom hijo de CA-414 y CA-416.
//
// raHoomConReloj (review_aislada_orden_test.go) corre el binario de hoom y
// raCLIVolvio le exige no pasar de raTopeRSS residentes: una review que lee
// en proporcion a algo que no tenia que leer (un hoom.yaml symlink a
// /dev/zero, un FIFO, una base rota) no pasa. La cifra sale de ru_maxrss
// (wait4), y en linux esa cifra no es solo del hijo: Go lanza a sus hijos con
// clone(CLONE_VFORK|CLONE_VM) y, al hacer exec, el kernel le anota al hijo el
// pico del espacio de memoria que deja, que es el de quien lo lanzo. Queda
// ru_maxrss = max(pico de quien lo lanzo al exec, pico propio del hijo). Con
// el binario de tests gordo (-race) los 12 casos del run 37355349622 del CI
// fallaban con la misma cifra, 324 MiB, que era la del binario de tests.
//
// El primer arreglo le muestreaba el VmHWM al hijo mientras vivia. Un hoom
// que rechaza enseguida vive unos 20 ms y podia morir antes de la primera
// muestra; sin muestra volvia la cifra heredada, y con ella el rojo falso
// (hallazgos 00cbcf y 24067b). Ahora no se muestrea nada y no hay carrera que
// perder: a hoom lo lanza un proceso FLACO, el medidor (raMedidor), que es
// este mismo binario de tests vuelto a ejecutar con ese papel (TestMain). Lo
// que hoom hereda es el pico del medidor, muy por debajo del tope, asi que
// para lo que pregunta raCLIVolvio su ru_maxrss es el suyo, viva lo que viva.
// Y si el medidor engordara, eso es un error del fixture, que raCifraDeHoom
// dice con todas las letras: no un fallo de hoom. El camino es el mismo en
// linux y en macOS (donde ru_maxrss ya era del hijo). En los sistemas que no
// son unix los tests de este paquete no compilan, ni antes ni ahora (usan
// syscall.Rusage, Mmap, Mkfifo).
//
// Aca estan el medidor, lo que informa, las piezas puras que leen el informe
// (raLeerInforme, raCifraDeHoom, raVmHWM) y sus tests. La cota no cambia: 128
// MiB, sobre la memoria PROPIA del hijo.
package reviewcmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/quick"
	"time"
)

// ---------------------------------------------------------------- la medida

const (
	// raTopeRSS es lo mas que puede llegar a tener residente el hoom hijo
	// para raCLIVolvio (CA-414, CA-416): una fraccion de lo que cuesta leer
	// /dev/zero. Es de la memoria propia del hijo.
	raTopeRSS = 128 << 20

	// raMargenDelMedidor es cuanto tiene que quedar el pico del medidor por
	// debajo de raTopeRSS para que raCifraDeHoom le crea al ru_maxrss de hoom.
	// En linux hoom hereda el pico del medidor: con un medidor de hasta
	// raTopeRSS - raMargenDelMedidor (112 MiB), un ru_maxrss por encima del
	// tope no puede ser heredado. El margen cubre que las dos cifras (la que
	// el kernel le anoto a hoom al exec y el VmHWM que el medidor se lee
	// despues) salen de contadores aproximados y no coinciden al kB. En macOS
	// el medidor pesa unos 7 MiB, y unos 25 con -race.
	raMargenDelMedidor = 16 << 20

	// raTechoRSS es cuanto puede llegar a tener residente hoom mientras
	// corre: si lo pasa, el vigilante de raHoomConReloj lo mata sin esperar a
	// que termine.
	raTechoRSS = 512 << 20

	// raTic es cada cuanto el vigilante de raHoomConReloj mira a hoom.
	raTic = 25 * time.Millisecond

	// raPapelMedidor: con esta variable en el entorno, este binario de tests
	// no corre tests: hace de medidor de hoom (TestMain, raMedidor).
	raPapelMedidor = "HOOM_TW_MEDIDOR"

	// raFDInforme es el descriptor por el que el medidor le escribe su
	// informe a quien lo lanzo: el primero de los que no son stdin, stdout ni
	// stderr, que son de hoom.
	raFDInforme = 3

	// raExitMedidorRoto es con lo que sale el medidor cuando no pudo lanzar a
	// hoom o esperarlo. Lo que paso lo dice el informe; este numero no decide
	// nada (hoom tambien puede salir con el).
	raExitMedidorRoto = 125

	// raPlazoDelInforme es cuanto espera raHoomConReloj lo que falte del
	// informe cuando el medidor ya termino. Lo que escribio ya esta en el
	// pipe; el plazo es para no quedarse esperando si alguien mas se quedo
	// con la otra punta.
	raPlazoDelInforme = 2 * time.Second
)

// TestMain reparte los papeles de este binario de tests. Con raPapelMedidor
// en el entorno no corre ningun test: hace de medidor de hoom y sale con lo
// que salio hoom. El papel se reparte aca, antes de m.Run, y no dentro de un
// test (como los papeles del experimento de mas abajo), porque el medidor no
// puede imprimir nada: su stdout y su stderr son los de hoom, y un test deja
// ahi su PASS y su ok.
//
// El medidor sale con syscall.Exit y no con os.Exit: con -race, os.Exit(0)
// pasa por el cierre del detector de carreras, que duerme un segundo antes de
// salir, y eso seria un segundo por cada hoom que termina bien. El medidor no
// tiene nada que cerrar: no deja nada a medio escribir, y su unica goroutine
// es la que mira si sigue vivo quien lo lanzo.
func TestMain(m *testing.M) {
	if os.Getenv(raPapelMedidor) != "" {
		syscall.Exit(raMedidor(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// raMedidor es el papel de medidor: lanza a hoom (argv: su ruta y sus
// argumentos), lo espera y sale con lo que salio hoom. No se mete con el: le
// pasa tal cual su stdin, su stdout, su stderr, su directorio y su entorno
// (menos raPapelMedidor, para que un hoom que es este mismo binario no haga
// de medidor el tambien), y no le deja el descriptor del informe. Devuelve el
// codigo de salida del medidor.
//
// El informe va por el descriptor raFDInforme, en lineas (raLeerInforme las
// lee):
//
//	hoom <pid>
//	fin <codigo> <bytes de hoom> <bytes del medidor>
//
// La primera sale apenas hoom arranca, para que el vigilante de memoria sepa
// a quien mirar. La ultima sale cuando hoom termino: con que salio (-1 si lo
// mato una senal), su ru_maxrss (el de wait4) y el pico de memoria residente
// del propio medidor, las dos en bytes. Si el medidor no puede lanzarlo o
// esperarlo, en lugar de la linea que toca escribe
//
//	error <por que>
//
// Es el unico proceso que lanza el medidor, y lo lanza con el medidor recien
// nacido: lo que hoom hereda en linux es el pico de un proceso que no hizo
// nada mas que arrancar.
//
// El medidor sigue a quien lo lanzo (grupoSeguirAQuienLoLanzo): si ese
// proceso muere sin cortarlo, el medidor mata a su grupo, que es el de hoom.
// Asi el reloj de raEnOtroProceso, que corta a un proceso que lanzo medidores,
// no los deja vivos (hallazgo ef9963).
func raMedidor(argv []string) int {
	var st syscall.Stat_t
	if err := syscall.Fstat(raFDInforme, &st); err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFIFO || len(argv) == 0 {
		fmt.Fprintf(os.Stderr, "fixture: al medidor de hoom lo lanza raHoomConReloj, con la ruta de hoom, sus argumentos y un pipe para el informe en el descriptor %d (argumentos: %d; descriptor: %v, modo %o)\n",
			raFDInforme, len(argv), err, st.Mode)
		return raExitMedidorRoto
	}
	syscall.CloseOnExec(raFDInforme)
	grupoSeguirAQuienLoLanzo()
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, raPapelMedidor+"=") {
			env = append(env, kv)
		}
	}
	pid, err := syscall.ForkExec(argv[0], argv, &syscall.ProcAttr{Env: env, Files: []uintptr{0, 1, 2}})
	if err != nil {
		raInformar(raLineaDeError("no pude lanzar " + argv[0] + ": " + err.Error()))
		return raExitMedidorRoto
	}
	raInformar(raLineaDeHoom(pid))
	var (
		ws syscall.WaitStatus
		ru syscall.Rusage
	)
	for {
		if _, err = syscall.Wait4(pid, &ws, 0, &ru); err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		raInformar(raLineaDeError("no pude esperar a hoom con wait4: " + err.Error()))
		return raExitMedidorRoto
	}
	code := ws.ExitStatus() // -1: lo mato una senal
	raInformar(raLineaDeFin(code, raBytesDeRuMaxrss(int64(ru.Maxrss)), raPicoDeEsteProceso()))
	if code < 0 {
		return 128 + int(ws.Signal())
	}
	return code
}

// raInformar escribe una linea del informe del medidor. Si no puede (quien
// lo lanzo ya no esta) no hay a quien avisarle.
func raInformar(linea string) {
	for b := []byte(linea); len(b) > 0; {
		n, err := syscall.Write(raFDInforme, b)
		if err != nil && err != syscall.EINTR {
			return
		}
		if n > 0 {
			b = b[n:]
		}
	}
}

// raLineaDeHoom, raLineaDeFin y raLineaDeError arman las lineas del informe
// del medidor.
func raLineaDeHoom(pid int) string {
	return "hoom " + strconv.Itoa(pid) + "\n"
}

func raLineaDeFin(code int, hoom, medidor uint64) string {
	return fmt.Sprintf("fin %d %d %d\n", code, hoom, medidor)
}

func raLineaDeError(motivo string) string {
	motivo = strings.Join(strings.Fields(motivo), " ") // en una sola linea
	if motivo == "" {
		motivo = "sin motivo"
	}
	return "error " + motivo + "\n"
}

// raBytesDeRuMaxrss pasa a bytes el ru_maxrss de un rusage: macOS lo da en
// bytes; linux (y los BSD), en kB.
func raBytesDeRuMaxrss(maxrss int64) uint64 {
	switch {
	case maxrss <= 0:
		return 0
	case runtime.GOOS == "darwin":
		return uint64(maxrss)
	case uint64(maxrss) > math.MaxUint64>>10:
		return math.MaxUint64
	}
	return uint64(maxrss) << 10
}

// raPicoDeEsteProceso es el pico de memoria residente de este proceso, en
// bytes (0 si no se sabe). Donde hay /proc/<pid>/status (linux) es su VmHWM
// y no su ru_maxrss: el VmHWM es del espacio de memoria y arranca de cero en
// el exec, que es justo lo que le hereda un hijo, mientras que el ru_maxrss
// propio trae ademas el pico de quien lanzo a este proceso. Donde no hay
// VmHWM es el ru_maxrss propio: en macOS es la cifra exacta, y en un linux
// que no informe VmHWM es una cota superior (puede hacer parecer gordo a un
// medidor flaco, nunca al reves).
func raPicoDeEsteProceso() uint64 {
	if hwm, ok := raVmHWMDe(os.Getpid()); ok {
		return hwm
	}
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return raBytesDeRuMaxrss(int64(ru.Maxrss))
}

// raVmHWM saca de un /proc/<pid>/status el VmHWM, el pico de memoria
// residente del proceso, en bytes. El VmHWM es del espacio de memoria: arranca
// de cero en el exec, asi que el de un proceso es solo suyo. ok es false si el
// texto no lo trae (un hilo del kernel o un zombie no tienen lineas Vm*) o si
// la linea no es "VmHWM: <kB en decimal> kB", que es lo unico que escribe el
// kernel: ante la duda no hay cifra.
func raVmHWM(status string) (bytes uint64, ok bool) {
	for _, linea := range strings.Split(status, "\n") {
		resto, es := strings.CutPrefix(linea, "VmHWM:")
		if !es {
			continue
		}
		f := strings.Fields(resto)
		if len(f) != 2 || f[1] != "kB" {
			return 0, false
		}
		kb, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil || kb > math.MaxUint64>>10 {
			return 0, false
		}
		return kb << 10, true
	}
	return 0, false
}

// raVmHWMDe lee el VmHWM del proceso pid, en bytes. ok es false donde no hay
// /proc (fuera de linux) o si el proceso ya no esta.
func raVmHWMDe(pid int) (bytes uint64, ok bool) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, false
	}
	return raVmHWM(string(raw))
}

// raInforme es lo que el medidor dejo dicho de una corrida de hoom.
type raInforme struct {
	pid     int    // el de hoom (0: el medidor no llego a lanzarlo)
	fin     bool   // trae la ultima linea: hoom termino y el medidor lo espero
	code    int    // con que salio hoom (-1: lo mato una senal)
	hoom    uint64 // el ru_maxrss de hoom, en bytes
	medidor uint64 // el pico de memoria residente del medidor, en bytes
	falla   string // el medidor no pudo lanzar a hoom o esperarlo: por que
}

// raLeerInforme lee el informe del medidor (raMedidor dice como es). Puede
// venir cortado entre dos lineas, si al medidor lo mataron: vacio, o solo con
// la primera. Todo lo demas es un error: una linea sin terminar, una que no
// es la que toca, una de mas, un numero que no es un decimal a secas o que no
// entra. Ante la duda no hay informe: quien lo lee no adivina una cifra.
func raLeerInforme(texto string) (raInforme, error) {
	var inf raInforme
	for n := 1; texto != ""; n++ {
		linea, resto, entera := strings.Cut(texto, "\n")
		if !entera {
			return raInforme{}, fmt.Errorf("la linea %d quedo sin terminar: %q", n, linea)
		}
		texto = resto
		clave, valor, _ := strings.Cut(linea, " ")
		switch {
		case inf.fin || inf.falla != "":
			return raInforme{}, fmt.Errorf("la linea %d esta de mas: %q", n, linea)
		case clave == "error" && valor != "":
			inf.falla = valor
		case clave == "hoom" && n == 1:
			pid, ok := raDecimal(valor)
			if !ok || pid == 0 || pid > math.MaxInt32 {
				return raInforme{}, fmt.Errorf("la linea %d no trae el pid de hoom: %q", n, linea)
			}
			inf.pid = int(pid)
		case clave == "fin" && n == 2:
			f := strings.Split(valor, " ")
			if len(f) != 3 {
				return raInforme{}, fmt.Errorf("la linea %d no trae el codigo y las dos cifras: %q", n, linea)
			}
			code, okCode := raCodigo(f[0])
			hoom, okHoom := raDecimal(f[1])
			medidor, okMedidor := raDecimal(f[2])
			if !okCode || !okHoom || !okMedidor {
				return raInforme{}, fmt.Errorf("la linea %d no trae el codigo (-1 o de 0 a 255) y las dos cifras: %q", n, linea)
			}
			inf.fin, inf.code, inf.hoom, inf.medidor = true, code, hoom, medidor
		default:
			return raInforme{}, fmt.Errorf("la linea %d no es la que toca: %q", n, linea)
		}
	}
	return inf, nil
}

// raCodigo lee con que salio hoom: -1 (lo mato una senal) o de 0 a 255.
func raCodigo(s string) (int, bool) {
	if s == "-1" {
		return -1, true
	}
	n, ok := raDecimal(s)
	if !ok || n > 255 {
		return 0, false
	}
	return int(n), true
}

// raDecimal lee un entero sin signo escrito solo con digitos decimales.
func raDecimal(s string) (uint64, bool) {
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}

// raCifraDeHoom dice, con el informe de una corrida que termino, con cuanta
// memoria residente se juzga a hoom, en bytes: su ru_maxrss. En linux esa
// cifra es max(pico del medidor al lanzarlo, pico propio de hoom, pico de los
// procesos que hoom lanzo y espero), asi que es la de hoom para raCLIVolvio
// siempre que el medidor quede bien por debajo del tope.
//
// El error es SIEMPRE del fixture, nunca un fallo de hoom: el informe no trae
// alguna de las dos cifras, o el medidor paso de raTopeRSS -
// raMargenDelMedidor y entonces un ru_maxrss alto ya no se sabe de quien es.
// Sin cifra de la que fiarse no hay cifra: raHoomConReloj falla con este
// error en vez de dejar pasar a hoom o de culparlo.
//
// Lo que la cifra no separa, y es lo mismo que antes del hallazgo: la memoria
// de los procesos que hoom lanza y espera (git, el CLI de IA) cuenta como de
// hoom; y por debajo del pico del medidor no distingue nada: un hoom mas
// flaco que el medidor figura con el pico del medidor (en linux).
func raCifraDeHoom(inf raInforme) (uint64, error) {
	const MiB = 1 << 20
	switch {
	case !inf.fin:
		return 0, errors.New("el informe del medidor no llega al final: no dice con que salio hoom ni cuanta memoria tuvo")
	case inf.medidor == 0:
		return 0, errors.New("el medidor no pudo decir cuanta memoria residente llego a tener el mismo: sin eso no se sabe si el ru_maxrss de hoom es de hoom")
	case inf.medidor > raTopeRSS-raMargenDelMedidor:
		return 0, fmt.Errorf("el medidor llego a %d MiB residentes y puede tener hasta %d (el tope de %d MiB menos un margen de %d): en linux hoom hereda ese pico, asi que su ru_maxrss (%d MiB) ya no se sabe de quien es. No es un fallo de hoom: hay que adelgazar el medidor (lo que este binario de tests hace antes de TestMain). Si este linux no informa VmHWM en /proc/<pid>/status, esa cifra del medidor es su ru_maxrss, que trae el pico de quien lo lanzo",
			inf.medidor/MiB, (raTopeRSS-raMargenDelMedidor)/MiB, raTopeRSS/MiB, raMargenDelMedidor/MiB, inf.hoom/MiB)
	case inf.hoom == 0:
		return 0, errors.New("wait4 no le dio al medidor el ru_maxrss de hoom")
	}
	return inf.hoom, nil
}

// ---------------------------------------------------------------- los tests

// CA-414 / CA-416 (hallazgos 78162b, 00cbcf y 24067b): la cifra con la que
// raCLIVolvio juzga al hoom hijo. El tope sigue siendo 128 MiB sobre la
// memoria propia del hijo; la cifra es su ru_maxrss siempre que el medidor
// que lo lanzo haya quedado bien por debajo del tope. Si no, o si falta
// alguna de las dos cifras, no hay cifra: hay un error del fixture.
func TestHallazgo_00cbcf_24067b_LaCifraDeHoomEsSuRuMaxrssConUnMedidorFlaco(t *testing.T) {
	const (
		kB     = uint64(1) << 10
		MiB    = uint64(1) << 20
		tope   = uint64(raTopeRSS)
		limite = uint64(raTopeRSS - raMargenDelMedidor) // lo mas que puede pesar el medidor
	)
	if tope != 128*MiB {
		t.Fatalf("CA-414 / CA-416: el tope del hoom hijo es 128 MiB residentes, no %d bytes", tope)
	}
	if margen := uint64(raMargenDelMedidor); margen == 0 || margen > tope/2 {
		t.Fatalf("el margen del medidor (%d bytes) es una parte del tope (%d bytes): ni nada ni mas de la mitad", margen, tope)
	}
	if techo := uint64(raTechoRSS); techo <= tope {
		t.Fatalf("el techo con el que el vigilante mata a hoom (%d bytes) queda por encima del tope con el que se lo juzga (%d bytes)", techo, tope)
	}
	casos := []struct {
		nombre        string
		hoom, medidor uint64
		sinFin        bool
		quiero        uint64
		hay           bool // hay cifra
		pasa          bool // raCLIVolvio lo deja pasar
	}{
		// lo que se ve
		{"macOS con -race: hoom con lo suyo, el medidor con 25 MiB", 14 * MiB, 25 * MiB, false, 14 * MiB, true, true},
		{"linux con -race: un hoom mas flaco que el medidor figura con el pico del medidor", 25 * MiB, 25 * MiB, false, 25 * MiB, true, true},
		{"linux sin -race: el medidor pesa 7 MiB", 14 * MiB, 7 * MiB, false, 14 * MiB, true, true},
		{"el hijo que sale enseguida: su cifra, chica", 2 * MiB, 7 * MiB, false, 2 * MiB, true, true},

		// glotones
		{"un gloton", 600 * MiB, 25 * MiB, false, 600 * MiB, true, false},
		{"un gloton de 192 MiB: exacto, ya no hay padre de 320 MiB que lo tape", 192 * MiB, 25 * MiB, false, 192 * MiB, true, false},
		{"la cifra mas grande que entra", math.MaxUint64, 25 * MiB, false, math.MaxUint64, true, false},

		// el tope, como siempre: pasarlo es tener MAS de 128 MiB
		{"justo en el tope", tope, 25 * MiB, false, tope, true, true},
		{"un byte sobre el tope", tope + 1, 25 * MiB, false, tope + 1, true, false},
		{"un kB sobre el tope", tope + kB, 25 * MiB, false, tope + kB, true, false},
		{"un kB bajo el tope", tope - kB, 25 * MiB, false, tope - kB, true, true},

		// el medidor en su limite: con el todavia se cree la cifra
		{"el medidor justo en su limite, hoom flaco", limite, limite, false, limite, true, true},
		{"el medidor justo en su limite, hoom gloton", 600 * MiB, limite, false, 600 * MiB, true, false},
		{"el medidor con un byte", 14 * MiB, 1, false, 14 * MiB, true, true},

		// un medidor gordo: un error del fixture, pese lo que pese hoom
		{"el medidor un byte sobre su limite, hoom flaco", limite + 1, limite + 1, false, 0, false, false},
		{"el medidor un kB sobre su limite, hoom gloton", 600 * MiB, limite + kB, false, 0, false, false},
		{"el medidor en el tope", tope, tope, false, 0, false, false},
		{"el medidor como el binario de tests del run 37355349622", 324 * MiB, 324 * MiB, false, 0, false, false},
		{"el medidor con la cifra mas grande que entra", 14 * MiB, math.MaxUint64, false, 0, false, false},

		// sin alguna de las cifras
		{"el medidor no dijo su pico", 14 * MiB, 0, false, 0, false, false},
		{"el medidor no dijo su pico y hoom es un gloton", 600 * MiB, 0, false, 0, false, false},
		{"wait4 no dio el ru_maxrss de hoom", 0, 25 * MiB, false, 0, false, false},
		{"sin ninguna de las dos", 0, 0, false, 0, false, false},
		{"el informe no llega al final", 14 * MiB, 25 * MiB, true, 0, false, false},
		{"el informe no llega al final y no trae nada", 0, 0, true, 0, false, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			inf := raInforme{pid: 4242, fin: !c.sinFin, hoom: c.hoom, medidor: c.medidor}
			got, err := raCifraDeHoom(inf)
			if got != c.quiero || (err == nil) != c.hay {
				t.Fatalf("raCifraDeHoom(%+v) = %d bytes, %v; quiero %d bytes y que haya cifra = %v", inf, got, err, c.quiero, c.hay)
			}
			if !c.hay {
				// sin cifra, quien llama no tiene con que dejar pasar a hoom
				if c.pasa {
					t.Fatalf("fixture: un caso sin cifra no pasa")
				}
				return
			}
			if pasa := got <= raTopeRSS; pasa != c.pasa {
				t.Fatalf("con %d bytes residentes raCLIVolvio deja pasar = %v, no %v", got, pasa, c.pasa)
			}
		})
	}

	// un medidor gordo no es un fallo de hoom, y el error lo dice
	_, err := raCifraDeHoom(raInforme{pid: 4242, fin: true, hoom: 324 * MiB, medidor: 324 * MiB})
	if err == nil || !strings.Contains(err.Error(), "No es un fallo de hoom") || !strings.Contains(err.Error(), "324 MiB") {
		t.Fatalf("con un medidor de 324 MiB el error dice que el gordo es el medidor y que no es un fallo de hoom: %v", err)
	}

	// en el modelo del kernel: hoom hereda el pico que el medidor tenia al
	// lanzarlo, el medidor informa su pico de despues (que no baja) y el
	// ru_maxrss de hoom es el mayor entre lo heredado y lo suyo. Si hay cifra,
	// nunca es menos que lo propio de hoom (no hay verde falso) y pasa del tope
	// solo si lo propio de hoom paso (no hay rojo falso); y no hay cifra solo
	// cuando el medidor paso de su limite
	t.Run("modelo-del-kernel", func(t *testing.T) {
		modelo := func(alLanzarlo, crecioDespues, propio uint64) bool {
			alLanzarlo, propio = alLanzarlo+1, propio+1
			medidor := alLanzarlo + crecioDespues
			got, err := raCifraDeHoom(raInforme{pid: 4242, fin: true, hoom: max(alLanzarlo, propio), medidor: medidor})
			switch {
			case err != nil && (medidor <= limite || got != 0):
				t.Logf("sin cifra con un medidor flaco: al lanzarlo %d, medidor %d, propio %d: %d, %v", alLanzarlo, medidor, propio, got, err)
			case err == nil && medidor > limite:
				t.Logf("una cifra con un medidor gordo: al lanzarlo %d, medidor %d, propio %d: %d", alLanzarlo, medidor, propio, got)
			case err == nil && got < propio:
				t.Logf("verde falso: al lanzarlo %d, medidor %d, propio %d: %d", alLanzarlo, medidor, propio, got)
			case err == nil && (got > tope) != (propio > tope):
				t.Logf("rojo falso: al lanzarlo %d, medidor %d, propio %d: %d", alLanzarlo, medidor, propio, got)
			default:
				return true
			}
			return false
		}
		// un medidor de cualquier tamano hasta 1 GiB, con un hoom de hasta 4 TiB
		cualquiera := func(alLanzarloKB uint16, crecioDespuesKB uint16, propioKB uint32) bool {
			return modelo(uint64(alLanzarloKB)*8*kB, uint64(crecioDespuesKB)*8*kB, uint64(propioKB)*kB)
		}
		if err := quick.Check(cualquiera, &quick.Config{MaxCount: 20000}); err != nil {
			t.Fatal(err)
		}
		// un medidor dentro de su limite, con un hoom a menos de 32 KiB del tope,
		// que al azar no sale
		cerca := func(alLanzarloKB uint16, crecioDespuesKB uint16, delTope int16) bool {
			alLanzarlo := uint64(alLanzarloKB) * kB           // hasta 64 MiB
			crecioDespues := uint64(crecioDespuesKB) * kB / 2 // hasta 32 MiB mas
			return modelo(alLanzarlo, crecioDespues, uint64(int64(tope)+int64(delTope))-1)
		}
		if err := quick.Check(cerca, &quick.Config{MaxCount: 20000}); err != nil {
			t.Fatal(err)
		}
	})
}

// CA-414 / CA-416 (hallazgos 00cbcf y 24067b): el informe del medidor se lee
// tal cual lo escribe raMedidor y de ninguna otra forma. Puede venir cortado
// entre dos lineas (al medidor lo mato el reloj o el vigilante); cualquier
// otra cosa es un error, nunca una cifra adivinada.
func TestHallazgo_00cbcf_24067b_ElInformeDelMedidorSeLeeTalCual(t *testing.T) {
	const max64 = "18446744073709551615"
	casos := []struct {
		nombre string
		texto  string
		quiero raInforme
		ok     bool
	}{
		// lo que escribe el medidor
		{"entero", "hoom 4242\nfin 0 14680064 26214400\n", raInforme{pid: 4242, fin: true, hoom: 14680064, medidor: 26214400}, true},
		{"hoom salio con error", "hoom 4242\nfin 1 14680064 26214400\n", raInforme{pid: 4242, fin: true, code: 1, hoom: 14680064, medidor: 26214400}, true},
		{"hoom salio con 255", "hoom 4242\nfin 255 14680064 26214400\n", raInforme{pid: 4242, fin: true, code: 255, hoom: 14680064, medidor: 26214400}, true},
		{"a hoom lo mato una senal", "hoom 4242\nfin -1 14680064 26214400\n", raInforme{pid: 4242, fin: true, code: -1, hoom: 14680064, medidor: 26214400}, true},
		{"sin cifras: las lee, y es raCifraDeHoom quien no las cree", "hoom 1\nfin 0 0 0\n", raInforme{pid: 1, fin: true}, true},
		{"las cifras mas grandes que entran", "hoom 2147483647\nfin 0 " + max64 + " " + max64 + "\n", raInforme{pid: math.MaxInt32, fin: true, hoom: math.MaxUint64, medidor: math.MaxUint64}, true},
		{"no pudo lanzar a hoom", "error no pude lanzar /no/existe/hoom: no such file or directory\n", raInforme{falla: "no pude lanzar /no/existe/hoom: no such file or directory"}, true},
		{"no pudo esperarlo", "hoom 4242\nerror no pude esperar a hoom con wait4: no child processes\n", raInforme{pid: 4242, falla: "no pude esperar a hoom con wait4: no child processes"}, true},

		// cortado entre dos lineas: al medidor lo mataron
		{"vacio: lo mataron antes de lanzar a hoom", "", raInforme{}, true},
		{"solo la primera linea: lo mataron esperando a hoom", "hoom 4242\n", raInforme{pid: 4242}, true},

		// cortado en medio de una linea
		{"la primera sin terminar", "hoom 4242", raInforme{}, false},
		{"la ultima sin terminar", "hoom 4242\nfin 0 14680064 26214400", raInforme{}, false},
		{"la ultima cortada en una cifra", "hoom 4242\nfin 0 1468", raInforme{}, false},
		{"el error sin terminar", "error no pude lanzar", raInforme{}, false},

		// lineas que no tocan
		{"el fin sin hoom", "fin 0 14680064 26214400\n", raInforme{}, false},
		{"dos hoom", "hoom 4242\nhoom 4243\n", raInforme{}, false},
		{"dos fin", "hoom 4242\nfin 0 1 2\nfin 0 1 2\n", raInforme{}, false},
		{"algo despues del fin", "hoom 4242\nfin 0 1 2\nhoom 4242\n", raInforme{}, false},
		{"un error despues del fin", "hoom 4242\nfin 0 1 2\nerror tarde\n", raInforme{}, false},
		{"algo despues del error", "error no pude\nhoom 4242\n", raInforme{}, false},
		{"dos errores", "error uno\nerror dos\n", raInforme{}, false},
		{"una linea vacia", "\n", raInforme{}, false},
		{"una linea vacia al final", "hoom 4242\nfin 0 1 2\n\n", raInforme{}, false},
		{"una linea vacia en el medio", "hoom 4242\n\nfin 0 1 2\n", raInforme{}, false},
		{"otra clave", "pid 4242\n", raInforme{}, false},
		{"la clave en mayusculas", "HOOM 4242\n", raInforme{}, false},
		{"con sangria", " hoom 4242\n", raInforme{}, false},
		{"con retorno de carro", "hoom 4242\r\nfin 0 1 2\r\n", raInforme{}, false},
		{"lo que imprime un test, no un medidor", "PASS\nok  \tgithub.com/hoomdev/hoomai/internal/reviewcmd\t0.012s\n", raInforme{}, false},
		{"binario", "\x00\x01\xff\xfehoom\x00 1\n", raInforme{}, false},

		// el pid
		{"sin pid", "hoom\n", raInforme{}, false},
		{"el pid vacio", "hoom \n", raInforme{}, false},
		{"el pid 0", "hoom 0\n", raInforme{}, false},
		{"el pid negativo", "hoom -4242\n", raInforme{}, false},
		{"el pid con signo", "hoom +4242\n", raInforme{}, false},
		{"el pid con decimales", "hoom 4242.0\n", raInforme{}, false},
		{"el pid en hexadecimal", "hoom 0x1092\n", raInforme{}, false},
		{"el pid con guion bajo", "hoom 4_242\n", raInforme{}, false},
		{"el pid en letras", "hoom alguno\n", raInforme{}, false},
		{"el pid no entra en 31 bits", "hoom 2147483648\n", raInforme{}, false},
		{"el pid no entra en 64 bits", "hoom 18446744073709551616\n", raInforme{}, false},
		{"dos espacios antes del pid", "hoom  4242\n", raInforme{}, false},
		{"algo despues del pid", "hoom 4242 4243\n", raInforme{}, false},
		{"un espacio despues del pid", "hoom 4242 \n", raInforme{}, false},
		{"un tab en vez del espacio", "hoom\t4242\n", raInforme{}, false},

		// el fin
		{"el fin sin nada", "hoom 4242\nfin\n", raInforme{}, false},
		{"el fin sin cifras", "hoom 4242\nfin 0\n", raInforme{}, false},
		{"el fin con una sola cifra", "hoom 4242\nfin 0 14680064\n", raInforme{}, false},
		{"el fin con una cifra de mas", "hoom 4242\nfin 0 1 2 3\n", raInforme{}, false},
		{"el fin con dos espacios", "hoom 4242\nfin 0 1  2\n", raInforme{}, false},
		{"el fin con un espacio al final", "hoom 4242\nfin 0 1 2 \n", raInforme{}, false},
		{"el codigo 256", "hoom 4242\nfin 256 1 2\n", raInforme{}, false},
		{"el codigo -2", "hoom 4242\nfin -2 1 2\n", raInforme{}, false},
		{"el codigo -0", "hoom 4242\nfin -0 1 2\n", raInforme{}, false},
		{"el codigo con signo", "hoom 4242\nfin +1 1 2\n", raInforme{}, false},
		{"el codigo en letras", "hoom 4242\nfin bien 1 2\n", raInforme{}, false},
		{"el codigo no entra en 64 bits", "hoom 4242\nfin 18446744073709551616 1 2\n", raInforme{}, false},
		{"la cifra de hoom negativa", "hoom 4242\nfin 0 -1 2\n", raInforme{}, false},
		{"la cifra de hoom en kB", "hoom 4242\nfin 0 14336kB 2\n", raInforme{}, false},
		{"la cifra de hoom con decimales", "hoom 4242\nfin 0 1.5 2\n", raInforme{}, false},
		{"la cifra de hoom no entra en 64 bits", "hoom 4242\nfin 0 18446744073709551616 2\n", raInforme{}, false},
		{"la cifra del medidor negativa", "hoom 4242\nfin 0 1 -2\n", raInforme{}, false},
		{"la cifra del medidor en hexadecimal", "hoom 4242\nfin 0 1 0x2\n", raInforme{}, false},
		{"la cifra del medidor no entra en 64 bits", "hoom 4242\nfin 0 1 18446744073709551616\n", raInforme{}, false},
		{"digitos sin fin", "hoom 4242\nfin 0 " + strings.Repeat("9", 400) + " 2\n", raInforme{}, false},

		// el error
		{"el error sin motivo", "error\n", raInforme{}, false},
		{"el error con el motivo vacio", "error \n", raInforme{}, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := raLeerInforme(c.texto)
			if got != c.quiero || (err == nil) != c.ok {
				t.Fatalf("raLeerInforme(%q) = %+v, %v; quiero %+v y sin error = %v", c.texto, got, err, c.quiero, c.ok)
			}
		})
	}

	// propiedad: lo que el medidor escribe se lee de vuelta, sean cuales sean
	// las cifras; cortado entre dos lineas se lee lo que llego; cortado en
	// cualquier otro lado es un error
	t.Run("lo-que-escribe-el-medidor-se-lee-de-vuelta", func(t *testing.T) {
		ida := func(pid uint32, code uint8, senal bool, hoom, medidor uint64, corte uint16) bool {
			quiero := raInforme{pid: int(pid>>1) + 1, fin: true, code: int(code), hoom: hoom, medidor: medidor}
			if senal {
				quiero.code = -1
			}
			primera := raLineaDeHoom(quiero.pid)
			texto := primera + raLineaDeFin(quiero.code, hoom, medidor)
			if got, err := raLeerInforme(texto); err != nil || got != quiero {
				t.Logf("raLeerInforme(%q) = %+v, %v; quiero %+v", texto, got, err, quiero)
				return false
			}
			cortado := texto[:int(corte)%len(texto)]
			got, err := raLeerInforme(cortado)
			switch cortado {
			case "":
				quiero = raInforme{}
			case primera:
				quiero = raInforme{pid: quiero.pid}
			default:
				if err == nil || got != (raInforme{}) {
					t.Logf("raLeerInforme(%q), cortado en medio de una linea, = %+v, %v", cortado, got, err)
					return false
				}
				return true
			}
			if err != nil || got != quiero {
				t.Logf("raLeerInforme(%q), cortado entre dos lineas, = %+v, %v; quiero %+v", cortado, got, err, quiero)
				return false
			}
			return true
		}
		if err := quick.Check(ida, &quick.Config{MaxCount: 20000}); err != nil {
			t.Fatal(err)
		}
	})

	// propiedad: el motivo de un error llega en una sola linea, diga lo que
	// diga, y no se confunde con las otras
	t.Run("el-motivo-de-un-error-va-en-una-linea", func(t *testing.T) {
		ida := func(motivo string, despuesDeHoom bool) bool {
			antes, quiero := "", raInforme{}
			if despuesDeHoom {
				antes, quiero.pid = raLineaDeHoom(4242), 4242
			}
			linea := raLineaDeError(motivo)
			quiero.falla = strings.TrimSuffix(strings.TrimPrefix(linea, "error "), "\n")
			got, err := raLeerInforme(antes + linea)
			if err != nil || got != quiero || quiero.falla == "" || strings.Count(linea, "\n") != 1 {
				t.Logf("raLeerInforme(%q) = %+v, %v; quiero %+v", antes+linea, got, err, quiero)
				return false
			}
			return true
		}
		if err := quick.Check(ida, &quick.Config{MaxCount: 5000}); err != nil {
			t.Fatal(err)
		}
		for _, motivo := range []string{"", " ", "\n", "dos\nlineas", "con\r\nretorno", "fin 0 1 2", "hoom 4242\nfin 0 1 2\n"} {
			texto := raLineaDeError(motivo)
			if got, err := raLeerInforme(texto); err != nil || got.falla == "" || got.fin || got.pid != 0 || strings.Count(texto, "\n") != 1 {
				t.Fatalf("raLeerInforme(raLineaDeError(%q) = %q) = %+v, %v: es un error, en una sola linea", motivo, texto, got, err)
			}
		}
	})
}

// CA-414 / CA-416 (hallazgos 00cbcf y 24067b): el ru_maxrss de wait4 se pasa
// a bytes segun el sistema (macOS lo da en bytes; linux, en kB), sin dar la
// vuelta y sin inventar nada con una cifra que no es positiva.
func TestHallazgo_00cbcf_24067b_ElRuMaxrssEnBytes(t *testing.T) {
	porKB := uint64(1) << 10
	if runtime.GOOS == "darwin" {
		porKB = 1
	}
	casos := []struct {
		maxrss int64
		quiero uint64
	}{
		{0, 0},
		{-1, 0},
		{math.MinInt64, 0},
		{1, porKB},
		{14336, 14336 * porKB},
		{131072, 131072 * porKB},
	}
	for _, c := range casos {
		if got := raBytesDeRuMaxrss(c.maxrss); got != c.quiero {
			t.Fatalf("raBytesDeRuMaxrss(%d) = %d bytes en %s, no %d", c.maxrss, got, runtime.GOOS, c.quiero)
		}
	}
	// lo mas grande: no da la vuelta
	if got := raBytesDeRuMaxrss(math.MaxInt64); got < math.MaxInt64 {
		t.Fatalf("raBytesDeRuMaxrss(el mayor) = %d bytes: dio la vuelta", got)
	}
	// y los de este proceso, que el sistema da como quiere. Su pico propio (el
	// que el medidor informa de si mismo) no puede ser menos que la mitad de lo
	// residente de recien ni mas que 512 veces eso: con la unidad mal leida (un
	// factor 1024, para un lado o para el otro) no cae ahi. Y su ru_maxrss no
	// puede ser menos que ese pico. Mas no se le pide: en linux trae ademas el
	// pico de quien lanzo a este binario, que puede ser cualquiera
	t.Run("el-de-este-proceso", func(t *testing.T) {
		recien := raRSS(os.Getpid())
		if recien == 0 {
			t.Skip("no se cuanta memoria residente tiene este proceso (ni /proc/<pid>/statm ni ps)")
		}
		pico := raPicoDeEsteProceso()
		if pico < recien/2 || pico/512 > recien {
			t.Fatalf("el pico de memoria residente de este proceso (%d bytes) no esta entre la mitad de lo residente de recien (%d bytes) y 512 veces eso", pico, recien)
		}
		var ru syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
			t.Fatalf("getrusage: %v", err)
		}
		if propio := raBytesDeRuMaxrss(int64(ru.Maxrss)); propio < pico/2 {
			t.Fatalf("el ru_maxrss de este proceso (%d, que serian %d bytes) no llega a la mitad de su pico de memoria residente (%d bytes)", ru.Maxrss, propio, pico)
		}
	})
}

// raStatusDeHoom es el /proc/<pid>/status de un proceso de Go como hoom, tal
// cual lo escribe linux: 14336 kB de VmHWM son 14 MiB.
const raStatusDeHoom = "Name:\thoom\n" +
	"Umask:\t0022\n" +
	"State:\tS (sleeping)\n" +
	"Tgid:\t4242\n" +
	"Ngid:\t0\n" +
	"Pid:\t4242\n" +
	"PPid:\t4200\n" +
	"TracerPid:\t0\n" +
	"Uid:\t1001\t1001\t1001\t1001\n" +
	"Gid:\t118\t118\t118\t118\n" +
	"FDSize:\t64\n" +
	"Groups:\t4 24 27 30 46 118 999 \n" +
	"NStgid:\t4242\n" +
	"NSpid:\t4242\n" +
	"NSpgid:\t4200\n" +
	"NSsid:\t1890\n" +
	"Kthread:\t0\n" +
	"VmPeak:\t 1240712 kB\n" +
	"VmSize:\t 1240712 kB\n" +
	"VmLck:\t       0 kB\n" +
	"VmPin:\t       0 kB\n" +
	"VmHWM:\t   14336 kB\n" +
	"VmRSS:\t   12160 kB\n" +
	"RssAnon:\t    4480 kB\n" +
	"RssFile:\t    7680 kB\n" +
	"RssShmem:\t       0 kB\n" +
	"VmData:\t   50692 kB\n" +
	"VmStk:\t     132 kB\n" +
	"VmExe:\t    2364 kB\n" +
	"VmLib:\t       8 kB\n" +
	"VmPTE:\t     112 kB\n" +
	"VmSwap:\t       0 kB\n" +
	"HugetlbPages:\t       0 kB\n" +
	"CoreDumping:\t0\n" +
	"THP_enabled:\t1\n" +
	"untag_mask:\t0xffffffffffffffff\n" +
	"Threads:\t7\n" +
	"SigQ:\t0/63429\n" +
	"SigPnd:\t0000000000000000\n" +
	"ShdPnd:\t0000000000000000\n" +
	"SigBlk:\tfffffffc3bba3a00\n" +
	"SigIgn:\t0000000000000000\n" +
	"SigCgt:\tfffffffd7fc1feff\n" +
	"CapInh:\t0000000000000000\n" +
	"CapPrm:\t0000000000000000\n" +
	"CapEff:\t0000000000000000\n" +
	"CapBnd:\t000001ffffffffff\n" +
	"CapAmb:\t0000000000000000\n" +
	"NoNewPrivs:\t0\n" +
	"Seccomp:\t0\n" +
	"Seccomp_filters:\t0\n" +
	"Speculation_Store_Bypass:\tthread vulnerable\n" +
	"SpeculationIndirectBranch:\tconditional enabled\n" +
	"Cpus_allowed:\tf\n" +
	"Cpus_allowed_list:\t0-3\n" +
	"Mems_allowed:\t00000000,00000001\n" +
	"Mems_allowed_list:\t0\n" +
	"voluntary_ctxt_switches:\t58\n" +
	"nonvoluntary_ctxt_switches:\t3\n"

// raStatusSinMemoria es el status de un proceso sin espacio de memoria (un
// zombie; un hilo del kernel se ve igual): no trae ninguna linea Vm*.
const raStatusSinMemoria = "Name:\thoom\n" +
	"State:\tZ (zombie)\n" +
	"Tgid:\t4242\n" +
	"Ngid:\t0\n" +
	"Pid:\t4242\n" +
	"PPid:\t4200\n" +
	"TracerPid:\t0\n" +
	"Uid:\t1001\t1001\t1001\t1001\n" +
	"Gid:\t118\t118\t118\t118\n" +
	"FDSize:\t0\n" +
	"Groups:\t4 24 27 30 46 118 999 \n" +
	"NStgid:\t4242\n" +
	"NSpid:\t4242\n" +
	"NSpgid:\t4200\n" +
	"NSsid:\t1890\n" +
	"Kthread:\t0\n" +
	"Threads:\t1\n" +
	"SigQ:\t0/63429\n" +
	"SigPnd:\t0000000000000000\n" +
	"voluntary_ctxt_switches:\t61\n" +
	"nonvoluntary_ctxt_switches:\t3\n"

// CA-414 / CA-416 (hallazgo 78162b): el VmHWM sale de la linea VmHWM de
// /proc/<pid>/status, en bytes, y de ninguna otra. Un texto que no la trae, o
// que la trae distinta de como la escribe el kernel, no da cifra: nunca una
// cifra inventada. Es la cifra que el medidor informa de si mismo en linux
// (raPicoDeEsteProceso).
func TestHallazgo_78162b_VmHWMDeUnStatus(t *testing.T) {
	const kB = uint64(1) << 10
	casos := []struct {
		nombre string
		status string
		quiero uint64
		ok     bool
	}{
		{"el status de un hoom", raStatusDeHoom, 14336 * kB, true},
		{"un zombie o un hilo del kernel: sin lineas Vm", raStatusSinMemoria, 0, false},
		{"vacio", "", 0, false},
		{"solo saltos de linea", "\n\n\n", 0, false},
		{"la linea sola, sin salto final", "VmHWM:\t   14336 kB", 14336 * kB, true},
		{"cero", "VmHWM:\t       0 kB\n", 0, true},
		{"un kB", "VmHWM:\t       1 kB\n", kB, true},
		{"mas de 4 GiB", "VmHWM:\t 8388608 kB\n", 8388608 * kB, true},

		// espacios raros
		{"un espacio en vez del tab", "VmHWM: 14336 kB\n", 14336 * kB, true},
		{"sin nada tras los dos puntos", "VmHWM:14336 kB\n", 14336 * kB, true},
		{"tabs y espacios de mas", "VmHWM:\t \t  14336 \t  kB  \t\n", 14336 * kB, true},
		{"sin espacio antes de la unidad", "VmHWM:\t14336kB\n", 0, false},
		{"con sangria: no es la linea del kernel", "Name:\thoom\n  VmHWM:\t14336 kB\n", 0, false},

		// la unidad
		{"sin unidad", "VmHWM:\t14336\n", 0, false},
		{"en MB", "VmHWM:\t14 MB\n", 0, false},
		{"en KB", "VmHWM:\t14336 KB\n", 0, false},
		{"en kb", "VmHWM:\t14336 kb\n", 0, false},
		{"en bytes", "VmHWM:\t14680064 B\n", 0, false},
		{"en paginas", "VmHWM:\t3584 pages\n", 0, false},
		{"algo despues de la unidad", "VmHWM:\t14336 kB (peak)\n", 0, false},

		// el numero
		{"sin numero", "VmHWM:\t kB\n", 0, false},
		{"sin nada", "VmHWM:\n", 0, false},
		{"negativo", "VmHWM:\t-14336 kB\n", 0, false},
		{"con signo", "VmHWM:\t+14336 kB\n", 0, false},
		{"con decimales", "VmHWM:\t14336.5 kB\n", 0, false},
		{"en hexadecimal", "VmHWM:\t0x3800 kB\n", 0, false},
		{"con separador de miles", "VmHWM:\t14,336 kB\n", 0, false},
		{"con guion bajo", "VmHWM:\t14_336 kB\n", 0, false},
		{"en letras", "VmHWM:\tmucho kB\n", 0, false},
		{"el mayor que entra en bytes", "VmHWM:\t18014398509481983 kB\n", 18014398509481983 * kB, true},
		{"uno mas: no entra en bytes", "VmHWM:\t18014398509481984 kB\n", 0, false},
		{"no entra en 64 bits ni en kB", "VmHWM:\t18446744073709551616 kB\n", 0, false},
		{"digitos sin fin", "VmHWM:\t" + strings.Repeat("9", 400) + " kB\n", 0, false},

		// otras lineas que se le parecen
		{"solo el pico virtual y el RSS de ahora", "VmPeak:\t 1240712 kB\nVmRSS:\t   12160 kB\n", 0, false},
		{"otra clave que empieza igual", "VmHWMx:\t14336 kB\n", 0, false},
		{"en minusculas", "vmhwm:\t14336 kB\n", 0, false},
		{"sin los dos puntos", "VmHWM\t14336 kB\n", 0, false},
		{"un proceso que se llama como la linea", "Name:\tVmHWM: 99 kB\nVmHWM:\t   14336 kB\n", 14336 * kB, true},
		{"un proceso que se llama como la linea y no tiene memoria", "Name:\tVmHWM: 99 kB\nState:\tZ (zombie)\n", 0, false},
		{"decide la primera linea VmHWM", "VmHWM:\t     100 kB\nVmHWM:\t     200 kB\n", 100 * kB, true},
		{"decide la primera linea VmHWM aunque sea basura", "VmHWM:\tbasura\nVmHWM:\t     200 kB\n", 0, false},

		// basura
		{"binario", "\x00\x01\xff\xfeVmHWM\x00:\x00 1 kB", 0, false},
		{"la salida de ps de macOS", "  RSS\n14336\n", 0, false},
		{"un statm", "310178 3584 1920 591 0 12673 0\n", 0, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, ok := raVmHWM(c.status)
			if got != c.quiero || ok != c.ok {
				t.Fatalf("raVmHWM(%q) = %d bytes, %v; quiero %d, %v", c.status, got, ok, c.quiero, c.ok)
			}
		})
	}

	// propiedad: lo que el kernel escribe se lee de vuelta, sea cual sea la
	// cifra y este donde este la linea, sin confundirla con las vecinas
	t.Run("lo-que-escribe-el-kernel-se-lee-de-vuelta", func(t *testing.T) {
		ida := func(pico, hwm, rss uint64, sinMemoria bool) bool {
			hwm >>= 10 // en kB entra siempre en bytes
			status := fmt.Sprintf("Name:\thoom\nVmPeak:\t%8d kB\nVmSize:\t%8d kB\nVmHWM:\t%8d kB\nVmRSS:\t%8d kB\nThreads:\t7\n", pico, pico, hwm, rss)
			quiero, ok := hwm<<10, true
			if sinMemoria {
				status = fmt.Sprintf("Name:\thoom\nState:\tZ (zombie)\nThreads:\t1\nvoluntary_ctxt_switches:\t%d\n", hwm)
				quiero, ok = 0, false
			}
			got, gotOK := raVmHWM(status)
			if got != quiero || gotOK != ok {
				t.Logf("raVmHWM(%q) = %d, %v; quiero %d, %v", status, got, gotOK, quiero, ok)
				return false
			}
			return true
		}
		if err := quick.Check(ida, &quick.Config{MaxCount: 5000}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("un-proceso-que-no-esta", func(t *testing.T) {
		// el pid 0 no es de ningun proceso: ni en linux tiene /proc/0
		if got, ok := raVmHWMDe(0); ok || got != 0 {
			t.Fatalf("raVmHWMDe(0) = %d bytes, %v: de un proceso que no esta no hay cifra", got, ok)
		}
	})

	// el de verdad: el de este proceso, que el kernel escribe como quiere. El
	// pico no puede ser mucho menos que lo residente de recien (statm) ni
	// mucho mas que el ru_maxrss propio, que en linux es el VmHWM o mas (lo
	// heredado de quien lanzo el test). Las tres cifras salen de contadores
	// aproximados, asi que se compara con holgura (el doble): con la unidad
	// mal leida (un factor 1024) o con el pico virtual no cae ahi
	t.Run("el-status-de-este-proceso", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("solo linux tiene /proc/<pid>/status")
		}
		raw, err := os.ReadFile("/proc/self/status")
		if err != nil {
			t.Skipf("sin /proc/self/status: %v", err)
		}
		if !strings.Contains(string(raw), "\nVmHWM:") {
			t.Skipf("este kernel no informa VmHWM:\n%s", raw)
		}
		recien := raRSS(os.Getpid())
		hwm, ok := raVmHWMDe(os.Getpid())
		if !ok {
			t.Fatalf("raVmHWMDe no saca el VmHWM de este proceso:\n%s", raw)
		}
		var ru syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
			t.Fatalf("getrusage: %v", err)
		}
		if hwm < recien/2 || hwm/2 > uint64(ru.Maxrss)<<10 {
			t.Fatalf("el VmHWM de este proceso (%d kB) no esta entre la mitad de lo residente de recien (%d kB) y el doble de su ru_maxrss (%d kB):\n%s",
				hwm>>10, recien>>10, ru.Maxrss, raw)
		}
	})
}

// ---------------------------------------------------------------- el medidor, a mano

// raMedidorSuelto es lo que se ve de lanzar al medidor a mano.
type raMedidorSuelto struct {
	exit           int
	stdout, stderr string
	informe        string // lo que escribio por raFDInforme
	vencio         bool   // no termino a tiempo: se lo corto, a el y a su grupo
}

// raLanzarMedidor lanza al medidor sin raHoomConReloj, para mirarle el
// protocolo: en dir, con stdin por su stdin, con extra sumado al entorno y,
// si conInforme, con el descriptor raFDInforme. argv es la ruta de hoom y sus
// argumentos. Tiene un reloj de 60 s: un medidor que no termina es un error
// del fixture.
func raLanzarMedidor(t *testing.T, dir, stdin string, extra []string, conInforme bool, argv ...string) raMedidorSuelto {
	t.Helper()
	m := raLanzarMedidorHasta(t, time.After(60*time.Second), dir, stdin, extra, conInforme, argv...)
	if m.vencio {
		t.Fatalf("fixture: el medidor con %q no termino en 60 s: se lo corto\nstdout:\n%s\nstderr:\n%s", argv, m.stdout, m.stderr)
	}
	return m
}

// raLanzarMedidorHasta es raLanzarMedidor con el reloj por afuera (vence) y
// sin cortar el test cuando vence: lo dice en vencio. Cuando vence corta al
// grupo entero del medidor, que es el de hoom y el de lo que hoom lanzo
// (grupoCorrer): no queda vivo ninguno, ni nadie esperando sus pipes
// (hallazgo ef9963).
func raLanzarMedidorHasta(t *testing.T, vence <-chan time.Time, dir, stdin string, extra []string, conInforme bool, argv ...string) raMedidorSuelto {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("fixture: no se cual es este binario: %v", err)
	}
	lee, escribe, err := os.Pipe()
	if err != nil {
		t.Fatalf("fixture: sin pipe para el informe del medidor: %v", err)
	}
	defer lee.Close()
	cmd := exec.Command(exe, argv...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), extra...), raPapelMedidor+"=1")
	cmd.Stdin = strings.NewReader(stdin)
	var o, e strings.Builder
	cmd.Stdout, cmd.Stderr = &o, &e
	if conInforme {
		cmd.ExtraFiles = []*os.File{escribe}
	}
	vencio, err := grupoCorrer(t, cmd, vence)
	_ = escribe.Close()
	if cmd.ProcessState == nil {
		t.Fatalf("fixture: no pude lanzar el medidor con %q: %v", argv, err)
	}
	// el medidor ya salio: si hoom o un hijo suyo se hubieran quedado con el
	// descriptor del informe, esto no terminaria; con el plazo, falla
	_ = lee.SetReadDeadline(time.Now().Add(10 * time.Second))
	informe, err := io.ReadAll(lee)
	if err != nil {
		t.Fatalf("hallazgos 00cbcf y 24067b: cuando el medidor sale, nadie mas tiene abierto el descriptor de su informe (%v); llego %q", err, informe)
	}
	return raMedidorSuelto{exit: cmd.ProcessState.ExitCode(), stdout: o.String(), stderr: e.String(), informe: string(informe), vencio: vencio}
}

// CA-414 / CA-416 (hallazgos 00cbcf y 24067b): el medidor no se mete con
// hoom. Le pasa tal cual el stdin, el stdout, el stderr, los argumentos, el
// directorio y el entorno (menos su propio papel); sale con lo que sale hoom;
// no imprime nada suyo; no le deja el descriptor del informe; y deja dicho
// por ese descriptor el pid de hoom, con que salio y las dos cifras de
// memoria. Si no puede lanzar a hoom lo dice por el informe, no por el stderr
// de hoom; y sin descriptor para el informe no lanza nada.
func TestHallazgo_00cbcf_24067b_ElMedidorNoSeMeteConHoom(t *testing.T) {
	const sh = "/bin/sh"
	entero := func(t *testing.T, m raMedidorSuelto, code int) raInforme {
		t.Helper()
		inf, err := raLeerInforme(m.informe)
		if err != nil || !inf.fin || inf.pid <= 0 || inf.code != code || inf.falla != "" {
			t.Fatalf("el informe del medidor trae el pid de hoom y que salio con %d: %q (%+v, %v)\nstderr:\n%s", code, m.informe, inf, err, m.stderr)
		}
		if cifra, err := raCifraDeHoom(inf); err != nil || cifra == 0 || cifra > raTopeRSS {
			t.Fatalf("el informe del medidor trae la cifra de un sh, que no llega al tope, y la de un medidor flaco: %q (%d bytes, %v)", m.informe, cifra, err)
		}
		return inf
	}

	t.Run("stdin-stdout-stderr-argumentos-y-exit", func(t *testing.T) {
		const pedido = "linea 1 del pedido\nlinea 2, sin salto final"
		m := raLanzarMedidor(t, t.TempDir(), pedido, nil, true,
			sh, "-c", `cat; printf 'a stderr: %s|%s\n' "$1" "$2" >&2; exit 7`, "sh", "un argumento", "--otro=con espacios y 'comillas'")
		if m.exit != 7 {
			t.Fatalf("el medidor sale con lo que salio hoom (7), no con %d\nstderr:\n%s", m.exit, m.stderr)
		}
		if m.stdout != pedido {
			t.Fatalf("el stdin y el stdout de hoom pasan tal cual por el medidor, que no imprime nada suyo: stdout %q, no %q", m.stdout, pedido)
		}
		if quiero := "a stderr: un argumento|--otro=con espacios y 'comillas'\n"; m.stderr != quiero {
			t.Fatalf("el stderr y los argumentos de hoom pasan tal cual por el medidor, que no imprime nada suyo: stderr %q, no %q", m.stderr, quiero)
		}
		entero(t, m, 7)
	})

	t.Run("exit-0-y-sin-salida", func(t *testing.T) {
		m := raLanzarMedidor(t, t.TempDir(), "", nil, true, sh, "-c", "exit 0")
		if m.exit != 0 || m.stdout != "" || m.stderr != "" {
			t.Fatalf("con un hoom que sale con 0 sin decir nada, el medidor sale con 0 sin decir nada: exit %d, stdout %q, stderr %q", m.exit, m.stdout, m.stderr)
		}
		entero(t, m, 0)
	})

	t.Run("a-hoom-lo-mata-una-senal", func(t *testing.T) {
		m := raLanzarMedidor(t, t.TempDir(), "", nil, true, sh, "-c", "kill -9 $$")
		if quiero := 128 + int(syscall.SIGKILL); m.exit != quiero {
			t.Fatalf("si a hoom lo mata una senal el medidor sale con 128 + la senal (%d), no con %d\nstderr:\n%s", quiero, m.exit, m.stderr)
		}
		// -1, lo que Go dice del exit de un proceso al que mato una senal
		entero(t, m, -1)
	})

	t.Run("el-directorio-y-el-entorno-menos-su-papel", func(t *testing.T) {
		dir := t.TempDir()
		m := raLanzarMedidor(t, dir, "", []string{"HOOM_TW_MEDIDOR_MARCA=la marca, con espacios"}, true,
			sh, "-c", `: > aca; printf '%s|%s\n' "${`+raPapelMedidor+`-sin papel}" "$HOOM_TW_MEDIDOR_MARCA"`)
		if quiero := "sin papel|la marca, con espacios\n"; m.exit != 0 || m.stdout != quiero {
			t.Fatalf("hoom recibe el entorno del medidor sin %s (un hoom que fuera este binario haria de medidor el tambien): exit %d, stdout %q, no %q\nstderr:\n%s",
				raPapelMedidor, m.exit, m.stdout, quiero, m.stderr)
		}
		if _, err := os.Stat(filepath.Join(dir, "aca")); err != nil {
			t.Fatalf("hoom corre en el directorio del medidor: %v", err)
		}
		entero(t, m, 0)
	})

	t.Run("hoom-no-hereda-el-descriptor-del-informe", func(t *testing.T) {
		// si lo heredara, hoom podria escribir en el informe, y un hijo de hoom
		// que sobreviviera lo tendria abierto: quien lee no veria el final
		m := raLanzarMedidor(t, t.TempDir(), "", nil, true,
			sh, "-c", `if ( printf 'fin 0 1 1\n' >&`+strconv.Itoa(raFDInforme)+` ) 2>/dev/null; then echo abierto; else echo cerrado; fi`)
		if m.exit != 0 || m.stdout != "cerrado\n" {
			t.Fatalf("hoom no tiene el descriptor %d del medidor: exit %d, stdout %q, informe %q\nstderr:\n%s", raFDInforme, m.exit, m.stdout, m.informe, m.stderr)
		}
		entero(t, m, 0)
	})

	t.Run("hoom-no-existe", func(t *testing.T) {
		dir := t.TempDir()
		m := raLanzarMedidor(t, dir, "", nil, true, filepath.Join(dir, "no-existe", "hoom"), "review")
		inf, err := raLeerInforme(m.informe)
		if err != nil || inf.falla == "" || inf.fin || inf.pid != 0 || !strings.Contains(inf.falla, filepath.Join("no-existe", "hoom")) {
			t.Fatalf("si no puede lanzar a hoom el medidor lo dice en el informe, con la ruta: %q (%+v, %v)", m.informe, inf, err)
		}
		if m.exit != raExitMedidorRoto || m.stdout != "" || m.stderr != "" {
			t.Fatalf("si no puede lanzar a hoom el medidor sale con %d y no imprime nada en el stdout ni en el stderr, que son de hoom: exit %d, stdout %q, stderr %q",
				raExitMedidorRoto, m.exit, m.stdout, m.stderr)
		}
	})

	t.Run("sin-descriptor-para-el-informe-no-lanza-nada", func(t *testing.T) {
		dir := t.TempDir()
		m := raLanzarMedidor(t, dir, "", nil, false, sh, "-c", ": > corrio")
		if m.exit != raExitMedidorRoto || m.stdout != "" || !strings.Contains(m.stderr, "fixture:") || m.informe != "" {
			t.Fatalf("sin el descriptor %d el medidor dice que es un error del fixture y sale con %d: exit %d, stdout %q, stderr %q",
				raFDInforme, raExitMedidorRoto, m.exit, m.stdout, m.stderr)
		}
		if _, err := os.Stat(filepath.Join(dir, "corrio")); err == nil {
			t.Fatalf("sin el descriptor %d el medidor no lanza a hoom: hoom corrio", raFDInforme)
		}
	})

	t.Run("sin-hoom-no-lanza-nada", func(t *testing.T) {
		m := raLanzarMedidor(t, t.TempDir(), "", nil, true)
		if m.exit != raExitMedidorRoto || m.stdout != "" || !strings.Contains(m.stderr, "fixture:") || m.informe != "" {
			t.Fatalf("sin la ruta de hoom el medidor dice que es un error del fixture y sale con %d: exit %d, stdout %q, stderr %q, informe %q",
				raExitMedidorRoto, m.exit, m.stdout, m.stderr, m.informe)
		}
	})
}

// ---------------------------------------------------------------- de punta a punta

// raPapelRSS: con esta variable los tests de punta a punta de este archivo
// corren como uno de los procesos del experimento en vez de armarlo.
const raPapelRSS = "HOOM_TW_78162B_PAPEL"

const (
	// raLastreDelPadre es lo que ocupa el padre gordo antes de lanzar a los
	// hijos, y raGloton lo que ocupa el hijo gloton: mas que el tope y menos
	// que el padre, para que con el padre como medida su pico quedara
	// escondido debajo del heredado.
	raLastreDelPadre = 320 << 20
	raGloton         = 192 << 20

	// raVoraz es lo que ocupa el hijo voraz: mas que raTechoRSS, para que lo
	// mate el vigilante; y raSiestaDelVoraz, lo que se queda esperandolo.
	raVoraz          = raTechoRSS + 64<<20
	raSiestaDelVoraz = 30 * time.Second

	// raTandas tandas a la vez de raPorTanda hijos que salen enseguida, una
	// atras de otra: 200 hijos.
	raTandas   = 8
	raPorTanda = 25

	// raProcsDelPadreOcupado es con cuantos procesadores corre el padre de
	// esos hijos, todos con una goroutine que no los suelta.
	raProcsDelPadreOcupado = 2
)

// raOcupar deja n bytes residentes en este proceso y no los suelta. Van fuera
// del heap de Go: ni el GC los mueve ni el detector de carreras les lleva
// sombra, asi que ocupan n con y sin -race.
func raOcupar(t *testing.T, n int) {
	t.Helper()
	b, err := syscall.Mmap(-1, 0, n, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatalf("fixture: no pude mapear %d MiB: %v", n>>20, err)
	}
	for i := 0; i < len(b); i += os.Getpagesize() {
		b[i] = 1
	}
}

// raSoloEsteTest es el -test.run con el que este binario, vuelto a ejecutar,
// corre solo el test de primer nivel de t: el nombre sale de t, no de una
// cadena que haya que acordarse de cambiar cuando el test cambia de nombre
// (hallazgo 3909e4).
func raSoloEsteTest(t *testing.T) string {
	nombre, _, _ := strings.Cut(t.Name(), "/")
	return "-test.run=^" + regexp.QuoteMeta(nombre) + "$"
}

// raNoHayTests es lo que imprime un binario de tests cuando su -test.run no
// selecciona ninguno. Y sale con 0: sin mirar esto, pasa.
const raNoHayTests = "testing: warning: no tests to run"

// raEnOtroProceso corre otra vez el test de primer nivel de t, en un proceso
// aparte de este binario, con ese papel (raPapelRSS) y con esas opciones de
// mas. Devuelve lo que imprimio y como termino. Que el proceso no termine en
// 3 minutos o que no haya corrido ningun test son errores del fixture.
func raEnOtroProceso(t *testing.T, papel string, opciones ...string) (salida string, err error) {
	t.Helper()
	salida, falla, err := raEsteBinarioComoTest(t, raSoloEsteTest(t), papel, time.After(3*time.Minute), opciones...)
	if falla != "" {
		t.Fatalf("fixture: el proceso con el papel %q %s (%v):\n%s", papel, falla, err, salida)
	}
	return salida, err
}

// raEsteBinarioComoTest corre este binario con esa seleccion de tests (un
// -test.run) y ese papel hasta que termina o hasta que llega algo por vence,
// y entonces corta a su grupo de procesos entero (grupoCorrer; hallazgo
// ef9963). falla dice lo que el fixture no puede dejar pasar ("" si nada): que
// hubo que cortarlo, o que la seleccion no era de ningun test y el proceso
// salio bien sin correr nada (hallazgo 3909e4).
func raEsteBinarioComoTest(t *testing.T, seleccion, papel string, vence <-chan time.Time, opciones ...string) (salida, falla string, err error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("fixture: no se cual es este binario: %v", err)
	}
	cmd := exec.Command(exe, append([]string{seleccion, "-test.count=1"}, opciones...)...)
	cmd.Env = append(os.Environ(), raPapelRSS+"="+papel)
	var junta bytes.Buffer
	cmd.Stdout, cmd.Stderr = &junta, &junta
	vencio, err := grupoCorrer(t, cmd, vence)
	switch salida = junta.String(); {
	case vencio:
		falla = "no termino a tiempo y hubo que cortarlo"
	case strings.Contains(salida, raNoHayTests):
		falla = "no corrio ningun test: " + seleccion + " no selecciona ninguno"
	}
	return salida, falla, err
}

// raComoPadreGordo corre el test de primer nivel de t en otro proceso, con el
// papel de padre gordo, y falla si ese proceso falla. El padre gordo es un
// proceso aparte para no subirle el pico de memoria al binario de la suite.
func raComoPadreGordo(t *testing.T, opciones ...string) {
	t.Helper()
	salida, err := raEnOtroProceso(t, "padre-gordo", opciones...)
	if err != nil {
		t.Fatalf("el experimento con el padre gordo fallo (%v):\n%s", err, salida)
	}
	t.Logf("el experimento con el padre gordo:\n%s", salida)
}

// CA-414 / CA-416 (hallazgo 78162b), de punta a punta sobre raHoomConReloj y
// raCLIVolvio, con este binario de test haciendo de hoom. Un padre con 320
// MiB residentes lanza:
//
//   - un hijo flaco: raCLIVolvio lo deja pasar. Es el caso del hallazgo (en
//     linux, lanzado por el padre, su ru_maxrss seria el pico del padre, 320
//     MiB o mas) y no depende de -race ni de cuanto pese la suite. Ya no se
//     queda esperando a que lo miren: su cifra no depende de eso;
//   - un hijo gloton de 192 MiB: la medida pasa del tope, y es la suya,
//     exacta: 192 MiB o mas, y menos que los 320 del padre, que ya no lo tapa.
//
// El padre gordo es un proceso aparte para no subirle el pico al binario de
// la suite.
func TestHallazgo_78162b_RSSDelHijoNoEsElDelPadre(t *testing.T) {
	switch papel := os.Getenv(raPapelRSS); papel {
	case "hijo-flaco":

	case "hijo-gloton":
		raOcupar(t, raGloton)

	case "padre-gordo":
		raOcupar(t, raLastreDelPadre)
		exe, err := os.Executable()
		if err != nil {
			t.Fatalf("fixture: no se cual es este binario: %v", err)
		}
		args := []string{raSoloEsteTest(t), "-test.count=1", "-test.v"}
		dir := t.TempDir()

		t.Setenv(raPapelRSS, "hijo-flaco")
		c := raHoomConReloj(t, exe, dir, 60*time.Second, args...)
		raCLIVolvio(t, "CA-416", "un hijo flaco de un padre con 320 MiB", c)
		if c.code != 0 || c.maxRSS == 0 {
			t.Fatalf("el hijo flaco termina bien y con una cifra de memoria (exit %d, %d bytes):\n%s", c.code, c.maxRSS, c.stdoutStderr)
		}

		t.Setenv(raPapelRSS, "hijo-gloton")
		c = raHoomConReloj(t, exe, dir, 60*time.Second, args...)
		if c.colgado || c.glotona || c.code != 0 {
			t.Fatalf("fixture: el hijo gloton ocupa %d MiB y termina solo (colgado %v, lo mato el vigilante %v, exit %d):\n%s",
				raGloton>>20, c.colgado, c.glotona, c.code, c.stdoutStderr)
		}
		if c.maxRSS <= raTopeRSS {
			t.Fatalf("CA-414 / CA-416: un hijo que ocupo %d MiB pasa del tope de %d MiB aunque su padre tenga mas: la medida le vio %d MiB",
				raGloton>>20, raTopeRSS>>20, c.maxRSS>>20)
		}
		if c.maxRSS < raGloton || c.maxRSS >= raLastreDelPadre {
			t.Fatalf("CA-414 / CA-416: la cifra de un hijo que ocupo %d MiB es la suya, exacta: %d MiB o mas, y menos que los %d de su padre; la medida le vio %d MiB",
				raGloton>>20, raGloton>>20, raLastreDelPadre>>20, c.maxRSS>>20)
		}

	case "":
		// con -test.v: la cifra de cada hijo queda en el registro
		raComoPadreGordo(t, "-test.v")

	default:
		t.Fatalf("fixture: %s=%q no es un papel del experimento", raPapelRSS, papel)
	}
}

// CA-414 / CA-416 (hallazgos 00cbcf y 24067b): un hijo que sale ENSEGUIDA
// tiene su cifra igual, todas las veces. Es el caso que el muestreo perdia: un
// hoom que rechaza apenas arranca vive unos 20 ms, y el que lo lanzaba podia
// llegar a mirarlo cuando ya no estaba; sin muestra, en linux, la cifra era la
// heredada del padre y el test fallaba con la memoria del binario de tests.
//
// Aca el padre tiene 320 MiB residentes, 2 procesadores y los 2 ocupados por
// goroutines que no los sueltan (el planificador de Go tarda hasta 10 ms en
// sacarlas), y lanza 200 veces, de a 8 a la vez, un sh que sale apenas arranca
// con el codigo 3: vive un par de milisegundos, asi que lo comun es que ya
// este muerto cuando el padre vuelve a correr. Las 200 veces raCLIVolvio lo
// deja pasar, con su codigo y con una cifra: la que da wait4, que no depende
// de llegar a mirarlo. (En linux la cifra de un hijo mas flaco que el medidor
// es el pico del medidor: chica igual.)
func TestHallazgo_00cbcf_24067b_ElHijoQueSaleEnseguidaTieneSuCifra(t *testing.T) {
	switch papel := os.Getenv(raPapelRSS); papel {
	case "padre-gordo":
		raOcupar(t, raLastreDelPadre)
		// este proceso no hace otra cosa: se queda con 2 procesadores, ocupados
		runtime.GOMAXPROCS(raProcsDelPadreOcupado)
		var basta atomic.Bool
		defer basta.Store(true)
		for range raProcsDelPadreOcupado {
			go func() {
				for !basta.Load() {
				}
			}()
		}
		dir := t.TempDir()
		var menor, mayor atomic.Uint64
		menor.Store(math.MaxUint64)
		t.Run("tandas", func(t *testing.T) {
			for tanda := range raTandas {
				t.Run(strconv.Itoa(tanda), func(t *testing.T) {
					t.Parallel()
					for i := range raPorTanda {
						c := raHoomConReloj(t, "/bin/sh", dir, 60*time.Second, "-c", "exit 3")
						raCLIVolvio(t, "CA-416", "un hijo que sale enseguida, de un padre con 320 MiB y sin un procesador libre", c)
						if c.code != 3 || c.maxRSS == 0 {
							t.Fatalf("hallazgos 00cbcf y 24067b: el hijo %d de la tanda %d, que sale enseguida, tiene su codigo (3) y su cifra de memoria igual: exit %d, %d bytes\n%s",
								i, tanda, c.code, c.maxRSS, c.stdoutStderr)
						}
						for v := menor.Load(); c.maxRSS < v && !menor.CompareAndSwap(v, c.maxRSS); v = menor.Load() {
						}
						for v := mayor.Load(); c.maxRSS > v && !mayor.CompareAndSwap(v, c.maxRSS); v = mayor.Load() {
						}
					}
				})
			}
		})
		if !t.Failed() {
			// por stdout y no con t.Logf: este proceso corre sin -test.v, para no
			// dejar en el registro una linea por cada uno de los 200
			fmt.Printf("%d hijos que salen enseguida, todos con cifra: de %d a %d kB\n", raTandas*raPorTanda, menor.Load()>>10, mayor.Load()>>10)
		}

	case "":
		raComoPadreGordo(t, "-test.parallel="+strconv.Itoa(raTandas))

	default:
		t.Fatalf("fixture: %s=%q no es un papel del experimento", raPapelRSS, papel)
	}
}

// CA-414 / CA-416 (hallazgos 00cbcf y 24067b): con el medidor en el medio,
// el reloj y el vigilante de raHoomConReloj siguen cortando a hoom, y lo
// cortan entero: matan al grupo de procesos del medidor, que es el de hoom y
// el de lo que hoom lanzo.
//
//   - el reloj: un hoom que no vuelve (un sh que deja otro proceso detras y
//     espera 30 s) queda marcado como colgado al segundo, y no queda vivo
//     ninguno de sus procesos. Matando solo al medidor quedarian los dos;
//   - el vigilante: un hoom que pasa de los 512 MiB residentes y se queda ahi
//     queda marcado como gloton mucho antes de que lo alcance el reloj. Al
//     vigilante el pid de hoom se lo dice la primera linea del informe.
func TestHallazgo_00cbcf_24067b_ElRelojYElVigilanteMatanAlGrupoDeHoom(t *testing.T) {
	switch papel := os.Getenv(raPapelRSS); papel {
	case "hijo-voraz":
		raOcupar(t, raVoraz)
		time.Sleep(raSiestaDelVoraz)
		return
	case "":
	default:
		t.Fatalf("fixture: %s=%q no es un papel del experimento", raPapelRSS, papel)
	}

	t.Run("el-reloj", func(t *testing.T) {
		testigo := nuevoTestigoDeVida(t)
		c := raHoomConReloj(t, "/bin/sh", t.TempDir(), time.Second, "-c", testigo.sh()+"sleep 30 &\nsleep 30\n")
		if !c.colgado || c.glotona || c.code != -1 {
			t.Fatalf("un hoom que no vuelve en 1 s queda colgado, y sin codigo de salida (-1): colgado %v, gloton %v, exit %d\n%s", c.colgado, c.glotona, c.code, c.stdoutStderr)
		}
		arranco, nadie := testigo.espera(10 * time.Second)
		if !arranco {
			t.Fatalf("fixture: el sh que hace de hoom no llego a tomar el testigo en 1 s:\n%s", c.stdoutStderr)
		}
		if !nadie {
			t.Fatalf("hallazgos 00cbcf y 24067b: el reloj mata al grupo de procesos entero: 10 s despues sigue vivo hoom o algo que hoom lanzo")
		}
	})

	t.Run("el-vigilante", func(t *testing.T) {
		exe, err := os.Executable()
		if err != nil {
			t.Fatalf("fixture: no se cual es este binario: %v", err)
		}
		t.Setenv(raPapelRSS, "hijo-voraz")
		antes := time.Now()
		c := raHoomConReloj(t, exe, t.TempDir(), raSiestaDelVoraz-5*time.Second, raSoloEsteTest(t), "-test.count=1")
		if !c.glotona || c.colgado || c.code != -1 {
			t.Fatalf("un hoom que ocupa %d MiB y se queda ahi lo mata el vigilante, no el reloj, y queda sin codigo de salida (-1): gloton %v, colgado %v, exit %d, en %v\n%s",
				raVoraz>>20, c.glotona, c.colgado, c.code, time.Since(antes), c.stdoutStderr)
		}
		if c.maxRSS <= raTechoRSS {
			t.Fatalf("de un hoom al que mato el vigilante queda lo mas que el vigilante le vio, mas de %d MiB: %d MiB", raTechoRSS>>20, c.maxRSS>>20)
		}
	})
}

// raSiguioDeLargo es lo que imprime un papel de
// TestHallazgo_00cbcf_24067b_SinInformeEnteroNoHayCorrida si raHoomConReloj
// le devuelve una corrida en vez de cortar el test.
const raSiguioDeLargo = "raHoomConReloj siguio de largo"

// CA-414 / CA-416 (hallazgos 00cbcf y 24067b): sin el informe entero del
// medidor no hay corrida. Si el medidor no puede lanzar a hoom, o muere antes
// de decir con que salio hoom y cuanta memoria tuvo (sin que lo hayan matado
// el reloj ni el vigilante), raHoomConReloj corta el test con un error que lo
// dice: nunca devuelve una corrida sin cifra, que raCLIVolvio dejaria pasar.
// Cada caso corre en otro proceso, que tiene que fallar con ese error.
func TestHallazgo_00cbcf_24067b_SinInformeEnteroNoHayCorrida(t *testing.T) {
	casos := []struct {
		papel string
		hoom  func(dir string) []string // la ruta de hoom y sus argumentos
		dice  string
	}{
		{
			papel: "hoom-no-existe",
			hoom:  func(dir string) []string { return []string{filepath.Join(dir, "no-existe", "hoom"), "review"} },
			dice:  "no pude correr hoom [review]: no pude lanzar ",
		},
		{
			// un hoom que mata a quien lo lanzo: el medidor muere esperandolo
			papel: "el-medidor-muere-sin-terminar-su-informe",
			hoom:  func(string) []string { return []string{"/bin/sh", "-c", "kill -9 $PPID"} },
			dice:  "sin decir con que salio hoom ni cuanta memoria tuvo",
		},
	}
	papel := os.Getenv(raPapelRSS)
	for _, c := range casos {
		if papel == c.papel {
			dir := t.TempDir()
			argv := c.hoom(dir)
			corrida := raHoomConReloj(t, argv[0], dir, 60*time.Second, argv[1:]...)
			fmt.Printf("%s: %+v\n", raSiguioDeLargo, corrida)
			return
		}
	}
	if papel != "" {
		t.Fatalf("fixture: %s=%q no es un papel del experimento", raPapelRSS, papel)
	}
	for _, c := range casos {
		t.Run(c.papel, func(t *testing.T) {
			salida, err := raEnOtroProceso(t, c.papel)
			if strings.Contains(salida, raSiguioDeLargo) {
				t.Fatalf("hallazgos 00cbcf y 24067b: sin el informe entero del medidor raHoomConReloj no devuelve una corrida:\n%s", salida)
			}
			if err == nil || !strings.Contains(salida, c.dice) {
				t.Fatalf("hallazgos 00cbcf y 24067b: sin el informe entero del medidor raHoomConReloj corta el test diciendo %q (%v):\n%s", c.dice, err, salida)
			}
		})
	}
}

// Hallazgos 5b4172 y 4c2a57: salga por donde salga la corrida de
// raHoomConReloj, no queda vivo nadie del grupo del medidor, que es el de
// hoom y el de lo que hoom lanzo: el grupo se corta ANTES de recolectar al
// medidor, tambien cuando su informe ya llego. Antes, las salidas sin informe
// final (el medidor muere con hoom vivo, el medidor informa un error, el
// informe no se entiende) cortaban el test sin matar a nadie, y la del informe
// entero dejaba vivo lo que hoom hubiera dejado detras. En cada caso lo que
// hace de hoom deja un sleep de 30 s detras (testigoDeVida.shConUnoDetras).
//
// Los dos ultimos no se le pueden pedir al medidor de verdad (un wait4 que
// falla, un informe roto): hace de medidor un sh que escribe ese informe por
// el descriptor raFDInforme y sale, sin llevarse a lo que lanzo. (Las salidas
// por el reloj y por el vigilante:
// TestHallazgo_00cbcf_24067b_ElRelojYElVigilanteMatanAlGrupoDeHoom.)
func TestHallazgo_5b4172_4c2a57_SalgaPorDondeSalgaLaCorridaNoQuedaNadieDelGrupo(t *testing.T) {
	const sh = "/bin/sh"
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("fixture: no se cual es este binario: %v", err)
	}
	// el medidor falso: lo que deja detras no se queda con el descriptor del
	// informe (como hoom con el medidor de verdad), y el informe es este
	falso := func(testigo *testigoDeVida, informe string) []string {
		fd := strconv.Itoa(raFDInforme)
		return []string{"-c", testigo.shConUnoDetras(fd+">&-") + "printf '" + informe + "' \"$!\" >&" + fd + "\n"}
	}
	casos := []struct {
		nombre  string
		medidor string                                // la ruta del medidor
		argv    func(testigo *testigoDeVida) []string // lo que va detras: la ruta de hoom y sus argumentos
		dice    string                                // el error del fixture ("": no hay, la corrida vuelve)
	}{
		{
			nombre:  "el-informe-trunco-el-medidor-muere-con-hoom-vivo",
			medidor: exe,
			argv: func(testigo *testigoDeVida) []string {
				return []string{sh, "-c", testigo.shConUnoDetras("") + "kill -9 $PPID\nwait\n"}
			},
			dice: "sin decir con que salio hoom ni cuanta memoria tuvo",
		},
		{
			nombre:  "el-informe-entero-hoom-deja-un-proceso-detras",
			medidor: exe,
			argv: func(testigo *testigoDeVida) []string {
				return []string{sh, "-c", testigo.shConUnoDetras("") + "exit 0\n"}
			},
		},
		{
			nombre:  "el-informe-dice-error-el-medidor-no-pudo-esperar-a-hoom",
			medidor: sh,
			argv: func(testigo *testigoDeVida) []string {
				return falso(testigo, `hoom %s\nerror no pude esperar a hoom con wait4: interrupted system call\n`)
			},
			dice: "no pude esperar a hoom con wait4",
		},
		{
			nombre:  "el-informe-no-se-entiende",
			medidor: sh,
			argv: func(testigo *testigoDeVida) []string {
				return falso(testigo, `hoom %s\nfin de la corrida\n`)
			},
			dice: "no se entiende",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			testigo := nuevoTestigoDeVida(t)
			t.Parallel()
			argv := c.argv(testigo)
			antes := time.Now()
			corrida, err := raHoomMedido(t, c.medidor, argv[0], t.TempDir(), 60*time.Second, argv[1:]...)
			tardo := time.Since(antes)
			switch {
			case c.dice == "" && (err != nil || corrida.code != 0 || corrida.colgado || corrida.glotona):
				t.Fatalf("fixture: un hoom que sale con 0 dejando un proceso detras es una corrida que volvio: %+v, %v", corrida, err)
			case c.dice != "" && (err == nil || !strings.Contains(err.Error(), c.dice)):
				t.Fatalf("fixture: esta corrida sale por el error %q: %+v, %v", c.dice, corrida, err)
			}
			arranco, nadie := testigo.espera(10 * time.Second)
			if !arranco {
				t.Fatalf("fixture: lo que hace de hoom no llego a tomar el testigo: %+v, %v", corrida, err)
			}
			if !nadie {
				t.Fatalf("hallazgos 5b4172 y 4c2a57: por esta salida (%v) tambien se corta al grupo del medidor: 10 s despues sigue vivo hoom o algo que lanzo", err)
			}
			if tardo > 15*time.Second {
				t.Fatalf("hallazgos 5b4172 y 4c2a57: el grupo se corta apenas la corrida sale, sin esperar a lo que quedo vivo: tardo %v", tardo)
			}
		})
	}
}

// Hallazgo ef9963, en el lanzador del medidor: cuando vence su reloj no queda
// vivo nadie del grupo del medidor y el lanzador vuelve enseguida, aunque un
// proceso que hoom dejo detras tenga abierto el stdout del medidor. Con el
// exec.CommandContext de antes moria solo el medidor: hoom y lo suyo seguian,
// y Run esperaba el fin de archivo de ese pipe. El reloj vence recien cuando
// hoom ya tomo el testigo.
func TestHallazgo_ef9963_ElRelojDelLanzadorDelMedidorCortaAlGrupoEntero(t *testing.T) {
	testigo := nuevoTestigoDeVida(t)
	antes := time.Now()
	m := raLanzarMedidorHasta(t, testigo.alTomarlo(t), t.TempDir(), "", nil, true,
		"/bin/sh", "-c", "echo corriendo\n"+testigo.shConUnoDetras("")+"wait\necho no-llega\n")
	tardo := time.Since(antes)
	if !m.vencio || m.exit != -1 {
		t.Fatalf("hallazgo ef9963: un medidor cuyo hoom no vuelve queda cortado por el reloj, sin codigo de salida (-1): vencio %v, exit %d\nstdout:\n%s\nstderr:\n%s", m.vencio, m.exit, m.stdout, m.stderr)
	}
	if tardo > 15*time.Second {
		t.Fatalf("hallazgo ef9963: cuando vence el reloj el lanzador del medidor vuelve enseguida: tardo %v, lo que tarda en soltar el pipe el sleep de 30 s", tardo)
	}
	if m.stdout != "corriendo\n" {
		t.Fatalf("hallazgo ef9963: lo que hoom imprimio antes del corte queda en la salida, y nada mas: %q", m.stdout)
	}
	if inf, err := raLeerInforme(m.informe); err != nil || inf.pid <= 0 || inf.fin {
		t.Fatalf("hallazgo ef9963: de un medidor cortado queda el informe trunco, con el pid de hoom y nada mas: %q (%+v, %v)", m.informe, inf, err)
	}
	if arranco, nadie := testigo.espera(10 * time.Second); !arranco || !nadie {
		t.Fatalf("hallazgo ef9963: cuando vence el reloj del lanzador muere el grupo entero del medidor: hoom tomo el testigo %v, 10 s despues no queda vivo ni el ni el sleep que lanzo %v", arranco, nadie)
	}
}

// raTestigoDelPapel: por esta variable le llega la ruta del testigo de vida
// al proceso que corre con el papel hoom-que-no-vuelve.
const raTestigoDelPapel = "HOOM_TW_EF9963_TESTIGO"

// Hallazgo ef9963, en raEnOtroProceso: cuando vence el reloj del proceso
// aparte no queda vivo nadie de lo que ese proceso lanzo. Ahi el Wait no se
// bloqueaba, pero el medidor que el proceso habia lanzado, y su hoom, seguian
// vivos. El proceso aparte corre a un hoom que no vuelve (deja un sleep de 30
// s y se queda esperandolo), con el reloj de adentro en 60 s; el de afuera
// vence cuando ese hoom tomo el testigo.
//
// Al medidor de adentro no lo alcanza el corte de afuera: tiene su grupo
// propio, para que el reloj de adentro lo pueda cortar solo a el. Lo que lo
// mata es que el medidor sigue a quien lo lanzo (raMedidor).
func TestHallazgo_ef9963_ElRelojDeOtroProcesoNoDejaVivoLoQueEseProcesoLanzo(t *testing.T) {
	const papel = "hoom-que-no-vuelve"
	switch os.Getenv(raPapelRSS) {
	case papel:
		// a este proceso lo van a matar: no arma directorios, que no llegaria a
		// borrar. Corre a hoom en el del testigo, que es del proceso de afuera
		testigo := &testigoDeVida{ruta: os.Getenv(raTestigoDelPapel)}
		raHoomConReloj(t, "/bin/sh", filepath.Dir(testigo.ruta), 60*time.Second, "-c", testigo.shConUnoDetras("")+"wait\n")
		return
	case "":
	default:
		t.Fatalf("fixture: %s=%q no es un papel del experimento", raPapelRSS, os.Getenv(raPapelRSS))
	}
	testigo := nuevoTestigoDeVida(t)
	t.Setenv(raTestigoDelPapel, testigo.ruta)
	antes := time.Now()
	salida, falla, err := raEsteBinarioComoTest(t, raSoloEsteTest(t), papel, testigo.alTomarlo(t))
	if tardo := time.Since(antes); !strings.Contains(falla, "hubo que cortarlo") || tardo > 15*time.Second {
		t.Fatalf("hallazgo ef9963: un proceso aparte que no termina queda cortado por el reloj, enseguida y como error del fixture: %q, %v, en %v\n%s", falla, err, tardo, salida)
	}
	if arranco, nadie := testigo.espera(10 * time.Second); !arranco || !nadie {
		t.Fatalf("hallazgo ef9963: cuando vence el reloj del proceso aparte no queda vivo nadie de lo que lanzo: el hoom de su medidor tomo el testigo %v, 10 s despues no queda vivo ni el medidor, ni ese hoom, ni el sleep que dejo %v", arranco, nadie)
	}
}

// Hallazgo 3909e4: un proceso aparte que no selecciona ningun test es un
// error del fixture. El binario de tests avisa ("no tests to run") y sale con
// 0: un test que se vuelve a ejecutar con un nombre que ya no es el suyo
// pasaba sin correr su experimento. Y el -test.run de raSoloEsteTest es el
// del test de primer nivel, se lo pida desde donde se lo pida.
func TestHallazgo_3909e4_UnProcesoAparteSinNingunTestEsUnErrorDelFixture(t *testing.T) {
	const yo = "-test.run=^TestHallazgo_3909e4_UnProcesoAparteSinNingunTestEsUnErrorDelFixture$"
	t.Run("un-subtest/con.puntos", func(t *testing.T) {
		if got := raSoloEsteTest(t); got != yo {
			t.Fatalf("hallazgo 3909e4: desde un subtest, raSoloEsteTest selecciona el test de primer nivel, a secas: %q, no %q", got, yo)
		}
	})
	if os.Getenv(raPapelRSS) != "" {
		return // el control de abajo: este test, en otro proceso, corre
	}
	// con -race un binario de tests que sale con 0 duerme un segundo antes de
	// salir; aca no hay carrera que esperar
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	for _, c := range []struct {
		nombre, seleccion string
		falla             string // "": corre
	}{
		{"un-nombre-que-no-es-de-ningun-test", "-test.run=^TestHallazgo_3909e4_UnNombreQueYaNoEsDeNadie$", "no corrio ningun test"},
		{"control-el-nombre-de-este-test", raSoloEsteTest(t), ""},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			salida, falla, err := raEsteBinarioComoTest(t, c.seleccion, "control", time.After(time.Minute))
			if err != nil || (c.falla == "") != (falla == "") || !strings.Contains(falla, c.falla) {
				t.Fatalf("hallazgo 3909e4: el proceso sale con 0 las dos veces, y sin ningun test seleccionado es un error del fixture (%q): dijo %q, %v\n%s", c.falla, falla, err, salida)
			}
		})
	}
}
