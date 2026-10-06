// Hallazgo 78162b (tarea estabilizar-ci): con que memoria se juzga al hoom
// hijo de CA-414 y CA-416.
//
// raHoomConReloj (review_aislada_orden_test.go) corre el binario de hoom y
// raCLIVolvio le exige no pasar de raTopeRSS residentes: una review que lee
// en proporcion a algo que no tenia que leer (un hoom.yaml symlink a
// /dev/zero, un FIFO, una base rota) no pasa. La cifra salia de ru_maxrss
// (wait4), y en linux esa cifra no es solo del hijo: Go lanza a sus hijos con
// clone(CLONE_VFORK|CLONE_VM) y, al hacer exec, el kernel le anota al hijo el
// pico del espacio de memoria que deja, que es el del padre. Queda
// ru_maxrss = max(pico del padre al exec, pico propio del hijo). Con el
// binario de tests gordo (-race) los 12 casos del run 37355349622 del CI
// fallaban con la misma cifra, 324 MiB, que era la del padre.
//
// Aca estan las piezas puras de la medida (raVmHWM, raPicoPropio), sus
// constantes y sus tests. La cota no cambia: 128 MiB, sobre la memoria PROPIA
// del hijo. En macOS ru_maxrss ya es la del hijo y la medida queda como
// estaba.
package reviewcmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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

	// raMargenDelPadre es por cuanto tiene que pasar ru_maxrss al pico del
	// padre para que raPicoPropio lo tome como del hijo. Cubre que las dos
	// cifras (la que el kernel anoto al exec y el VmHWM que se lee despues)
	// salen de contadores aproximados y no coinciden al kB.
	raMargenDelPadre = 8 << 20

	// raTicLinux es cada cuanto raHoomConReloj mira al hijo en linux, donde
	// mirar es leer dos archivos de /proc; raTicOtros, en el resto, donde es
	// lanzar un ps.
	raTicLinux = 2 * time.Millisecond
	raTicOtros = 25 * time.Millisecond
)

// raTic es cada cuanto raHoomConReloj mira al hijo en este sistema.
func raTic() time.Duration {
	if runtime.GOOS == "linux" {
		return raTicLinux
	}
	return raTicOtros
}

// raVmHWM saca de un /proc/<pid>/status el VmHWM, el pico de memoria
// residente del proceso, en bytes. El VmHWM es del espacio de memoria: arranca
// de cero en el exec, asi que el de un hijo es solo suyo. ok es false si el
// texto no lo trae (un hilo del kernel o un zombie no tienen lineas Vm*) o si
// la linea no es "VmHWM: <kB en decimal> kB", que es lo unico que escribe el
// kernel: ante la duda no hay cifra, y sin cifra la medida es la estricta.
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

// raPicoPropio decide, en linux, cuanta memoria residente llego a tener el
// hijo, en bytes, con tres cifras en bytes:
//
//   - ruMaxrss: el ru_maxrss que dio wait4 por el hijo, que es
//     max(pico del padre al exec, pico propio del hijo);
//   - padre: el VmHWM del proceso que lo lanzo, leido DESPUES del Wait (0 = no
//     se pudo leer). Despues y no antes: el pico no baja, asi que el de
//     despues nunca es menor que el que heredo el hijo; uno leido antes del
//     exec podria quedar por debajo de lo heredado y hacer pasar por memoria
//     del hijo lo que era del padre (un rojo falso);
//   - muestreado: el mayor VmHWM que se le leyo al hijo mientras vivia (0 = no
//     se le leyo nada).
//
// Si ruMaxrss pasa al padre por mas de raMargenDelPadre no puede ser
// heredado: es el pico del hijo, exacto. Si no, ruMaxrss no dice nada del
// hijo y vale lo muestreado. Sin el pico del padre o sin ninguna muestra no
// hay con que bajar la cifra y vale ruMaxrss, estricto, como antes del
// hallazgo: un muestreo roto se ve como un rojo, nunca como un verde.
//
// Lo que se le puede escapar, siempre dentro de la banda en la que ruMaxrss no
// decide (el pico del hijo entre raTopeRSS y padre + raMargenDelPadre):
//
//   - lo que el hijo crezca despues de la ultima muestra (un tic, o lo que
//     tarde el planificador en volver al test): ahi la cifra queda por debajo
//     del pico real. Se acepta porque CA-414 y CA-416 buscan una review que
//     lee en proporcion a algo enorme o que no termina (/dev/zero, un FIFO):
//     pasar de raTopeRSS le lleva mucho mas que un tic y no vuelve sola al
//     llegar; sigue creciendo hasta el vigilante de 512 MiB o el reloj;
//   - la memoria de los procesos que el hijo lanza y espera (git): ruMaxrss
//     los incluye, el muestreo mira solo al hijo. Se acepta porque CA-414 y
//     CA-416 miden lo que lee hoom, y el vigilante tampoco los mira;
//   - cuanto mas crezca el padre entre el exec y el Wait, mas ancha la banda:
//     padre queda por encima de lo que heredo el hijo. Eso nunca da un rojo
//     falso; solo deja mas casos en manos del muestreo.
//
// Fuera de la banda no se escapa nada, y con un padre de hasta raTopeRSS -
// raMargenDelPadre la banda esta vacia: la medida es la exacta. Y el margen
// puede quedar corto (si las dos cifras del kernel difieren en mas de
// raMargenDelPadre): entonces lo heredado se toma por memoria del hijo, que
// es el rojo falso de antes del hallazgo, nunca un verde falso.
func raPicoPropio(ruMaxrss, padre, muestreado uint64) uint64 {
	if padre == 0 || muestreado == 0 {
		return ruMaxrss
	}
	if ruMaxrss > padre && ruMaxrss-padre > raMargenDelPadre {
		return ruMaxrss
	}
	return muestreado
}

// ---------------------------------------------------------------- los tests

// CA-414 / CA-416 (hallazgo 78162b): la cifra con la que raCLIVolvio juzga
// al hoom hijo. El tope sigue siendo 128 MiB; lo que cambia es de quien es la
// memoria que se compara con el.
func TestHallazgo_78162b_PicoPropioDecideDeQuienEsElRuMaxrss(t *testing.T) {
	const (
		kB  = uint64(1) << 10
		MiB = uint64(1) << 20
	)
	if tope := uint64(raTopeRSS); tope != 128*MiB {
		t.Fatalf("CA-414 / CA-416: el tope del hoom hijo es 128 MiB residentes, no %d bytes", tope)
	}
	if tope, margen := uint64(raTopeRSS), uint64(raMargenDelPadre); margen == 0 || margen > tope/8 {
		t.Fatalf("el margen (%d bytes) es una fraccion chica del tope (%d bytes)", margen, tope)
	}
	casos := []struct {
		nombre                      string
		ruMaxrss, padre, muestreado uint64
		quiero                      uint64
		pasa                        bool // raCLIVolvio lo deja pasar
	}{
		// lo que se vio
		{"el dogfood con -race del run 37355349622: los 324 MiB eran del padre",
			324 * MiB, 324 * MiB, 14 * MiB, 14 * MiB, true},
		{"el paso sin -race de hoy: los 108 MiB tambien son del padre",
			108 * MiB, 108 * MiB, 14 * MiB, 14 * MiB, true},
		{"el padre siguio creciendo despues del exec",
			300 * MiB, 324 * MiB, 14 * MiB, 14 * MiB, true},
		{"un hijo mas grande que su padre flaco, bajo el tope: exacto",
			60 * MiB, 20 * MiB, 55 * MiB, 60 * MiB, true},

		// glotones
		{"un gloton por encima del padre: exacto aunque el muestreo no lo haya visto",
			600 * MiB, 324 * MiB, 14 * MiB, 600 * MiB, false},
		{"un gloton por encima del padre, visto a medias por el muestreo",
			600 * MiB, 324 * MiB, 590 * MiB, 600 * MiB, false},
		{"un gloton dentro de la banda: lo dice el muestreo",
			324 * MiB, 324 * MiB, 200 * MiB, 200 * MiB, false},
		{"un gloton por encima de un padre flaco",
			200 * MiB, 108 * MiB, 14 * MiB, 200 * MiB, false},

		// el tope, como siempre: pasarlo es tener MAS de 128 MiB
		{"justo en el tope dentro de la banda",
			324 * MiB, 324 * MiB, 128 * MiB, 128 * MiB, true},
		{"un kB sobre el tope dentro de la banda",
			324 * MiB, 324 * MiB, 128*MiB + kB, 128*MiB + kB, false},
		{"justo en el tope por encima del padre",
			128 * MiB, 100 * MiB, 14 * MiB, 128 * MiB, true},
		{"un kB sobre el tope por encima del padre",
			128*MiB + kB, 100 * MiB, 14 * MiB, 128*MiB + kB, false},

		// los bordes del margen
		{"un kB por debajo del margen: no alcanza para ser del hijo",
			324*MiB + raMargenDelPadre - kB, 324 * MiB, 14 * MiB, 14 * MiB, true},
		{"justo en el margen: no alcanza para ser del hijo",
			324*MiB + raMargenDelPadre, 324 * MiB, 14 * MiB, 14 * MiB, true},
		{"un byte por encima del margen: es del hijo",
			324*MiB + raMargenDelPadre + 1, 324 * MiB, 14 * MiB, 324*MiB + raMargenDelPadre + 1, false},
		{"un kB por encima del margen: es del hijo",
			324*MiB + raMargenDelPadre + kB, 324 * MiB, 14 * MiB, 324*MiB + raMargenDelPadre + kB, false},
		{"igual al padre",
			100 * MiB, 100 * MiB, 14 * MiB, 14 * MiB, true},
		{"padre + margen no da la vuelta al sumar",
			100 * MiB, math.MaxUint64, 14 * MiB, 14 * MiB, true},
		{"ru_maxrss - padre no da la vuelta al restar",
			math.MaxUint64, math.MaxUint64 - kB, 14 * MiB, 14 * MiB, true},

		// sin con que bajar la cifra: la estricta, la de antes
		{"sin el VmHWM del padre: ru_maxrss, que aca era del padre",
			324 * MiB, 0, 14 * MiB, 324 * MiB, false},
		{"sin el VmHWM del padre, con un hijo flaco de un padre flaco",
			14 * MiB, 0, 14 * MiB, 14 * MiB, true},
		{"sin el VmHWM del padre no vale ni un muestreo mayor",
			100 * MiB, 0, 120 * MiB, 100 * MiB, true},
		{"sin el VmHWM del padre tampoco con un ru_maxrss menor que el margen",
			4 * MiB, 0, 6 * MiB, 4 * MiB, true},
		{"sin ninguna muestra del hijo: ru_maxrss",
			324 * MiB, 324 * MiB, 0, 324 * MiB, false},
		{"sin ninguna muestra del hijo, con el padre bajo el tope",
			108 * MiB, 108 * MiB, 0, 108 * MiB, true},
		{"sin nada",
			0, 0, 0, 0, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := raPicoPropio(c.ruMaxrss, c.padre, c.muestreado)
			if got != c.quiero {
				t.Fatalf("raPicoPropio(ru_maxrss %d, padre %d, muestreado %d) = %d bytes, no %d",
					c.ruMaxrss, c.padre, c.muestreado, got, c.quiero)
			}
			if pasa := got <= raTopeRSS; pasa != c.pasa {
				t.Fatalf("con %d bytes residentes raCLIVolvio deja pasar = %v, no %v", got, pasa, c.pasa)
			}
		})
	}
}

// CA-414 / CA-416 (hallazgo 78162b), como propiedades. En el modelo del
// kernel (el hijo hereda el pico del padre al exec, el padre puede seguir
// creciendo, el muestreo puede perderse el final) la cifra nunca supera el
// pico propio del hijo (no hay rojo falso) ni baja de lo que se le vio (asi
// que es exacta si el muestreo no se perdio nada), y es exacta tambien si el
// hijo paso al padre por mas que el margen, vea lo que vea el muestreo. Y con
// cifras cualesquiera: la respuesta es una de las dos medidas, y es ru_maxrss
// siempre que no haya con que bajarla o que no pueda ser heredado.
func TestHallazgo_78162b_PicoPropioEnElModeloDelKernel(t *testing.T) {
	t.Run("modelo", func(t *testing.T) {
		// en kB, como las cifras del kernel: hasta 4 TiB. El muestreo se pierde
		// entre nada y casi todo lo que crecio el hijo, pero algo le vio
		modelo := func(heredadoKB, crecioElPadreKB, propioKB uint32, sePerdio uint16) bool {
			const kB = 1 << 10
			heredado := (uint64(heredadoKB) + 1) * kB
			padre := heredado + uint64(crecioElPadreKB)*kB
			propio := (uint64(propioKB) + 1) * kB
			muestreado := max(propio-propio/(1<<16)*uint64(sePerdio), kB)
			got := raPicoPropio(max(heredado, propio), padre, muestreado)
			switch {
			case got > propio:
				t.Logf("rojo falso: heredado %d, padre %d, propio %d, muestreado %d: %d", heredado, padre, propio, muestreado, got)
			case got < muestreado:
				t.Logf("menos de lo que se vio: heredado %d, padre %d, propio %d, muestreado %d: %d", heredado, padre, propio, muestreado, got)
			case propio > padre+raMargenDelPadre && got != propio:
				t.Logf("por encima del padre es exacto: heredado %d, padre %d, propio %d, muestreado %d: %d", heredado, padre, propio, muestreado, got)
			default:
				return true
			}
			return false
		}
		if err := quick.Check(modelo, &quick.Config{MaxCount: 20000}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cifras-cualesquiera", func(t *testing.T) {
		cualquiera := func(ruMaxrss, padre, muestreado uint64, sinPadre, sinMuestra bool) bool {
			if sinPadre {
				padre = 0
			}
			if sinMuestra {
				muestreado = 0
			}
			got := raPicoPropio(ruMaxrss, padre, muestreado)
			noEsHeredado := ruMaxrss > padre && ruMaxrss-padre > raMargenDelPadre
			switch {
			case got != ruMaxrss && got != muestreado:
				t.Logf("ni una medida ni la otra: ru_maxrss %d, padre %d, muestreado %d: %d", ruMaxrss, padre, muestreado, got)
			case (padre == 0 || muestreado == 0 || noEsHeredado) && got != ruMaxrss:
				t.Logf("tenia que ser ru_maxrss: ru_maxrss %d, padre %d, muestreado %d: %d", ruMaxrss, padre, muestreado, got)
			case padre != 0 && muestreado != 0 && !noEsHeredado && got != muestreado:
				t.Logf("tenia que ser lo muestreado: ru_maxrss %d, padre %d, muestreado %d: %d", ruMaxrss, padre, muestreado, got)
			default:
				return true
			}
			return false
		}
		if err := quick.Check(cualquiera, &quick.Config{MaxCount: 20000}); err != nil {
			t.Fatal(err)
		}
		// cerca del margen, que al azar no sale
		cerca := func(padreKB uint32, sobreElMargen int16, muestreadoKB uint32) bool {
			padre := (uint64(padreKB) + 1) << 10
			muestreado := (uint64(muestreadoKB) + 1) << 10
			ruMaxrss := uint64(int64(padre+raMargenDelPadre) + int64(sobreElMargen))
			got := raPicoPropio(ruMaxrss, padre, muestreado)
			quiero := muestreado
			if sobreElMargen > 0 {
				quiero = ruMaxrss
			}
			if got != quiero {
				t.Logf("a %d bytes del margen: ru_maxrss %d, padre %d, muestreado %d: %d, no %d", sobreElMargen, ruMaxrss, padre, muestreado, got, quiero)
				return false
			}
			return true
		}
		if err := quick.Check(cerca, &quick.Config{MaxCount: 20000}); err != nil {
			t.Fatal(err)
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
// que la trae distinta de como la escribe el kernel, no da cifra (raPicoPropio
// cae entonces en la medida estricta): nunca una cifra inventada.
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

// ---------------------------------------------------------------- de punta a punta

// raPapelRSS: con esta variable TestHallazgo_78162b_RSSDelHijoNoEsElDelPadre
// corre como uno de los procesos del experimento en vez de armarlo.
const raPapelRSS = "HOOM_TW_78162B_PAPEL"

const (
	// raLastreDelPadre es lo que ocupa el padre gordo antes de lanzar a los
	// hijos, y raGloton lo que ocupa el hijo gloton: mas que el tope y menos
	// que el padre, para que en linux su pico quede escondido debajo del
	// heredado.
	raLastreDelPadre = 320 << 20
	raGloton         = 192 << 20

	// raSiesta es lo que viven los hijos despues de ocupar su memoria.
	raSiesta = 300 * time.Millisecond
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

// CA-414 / CA-416 (hallazgo 78162b), de punta a punta sobre raHoomConReloj y
// raCLIVolvio, con este binario de test haciendo de hoom. Un padre con 320
// MiB residentes lanza:
//
//   - un hijo flaco: raCLIVolvio lo deja pasar. En linux es el caso del
//     hallazgo (el ru_maxrss del hijo es el pico del padre, 320 MiB o mas) y
//     no depende de -race ni de cuanto pese la suite;
//   - un hijo gloton de 192 MiB: la medida pasa del tope. En linux su pico
//     queda debajo del heredado, asi que solo lo ve el muestreo: si el
//     muestreo no mirara al hijo, o leyera mal la cifra, esto falla.
//
// En macOS ru_maxrss ya es del hijo y los dos casos lo confirman. El padre
// gordo es un proceso aparte para no subirle el pico al binario de la suite:
// en linux le ensancharia la banda (ver raPicoPropio) a todas las corridas
// que vienen despues.
func TestHallazgo_78162b_RSSDelHijoNoEsElDelPadre(t *testing.T) {
	const test = "TestHallazgo_78162b_RSSDelHijoNoEsElDelPadre"
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("fixture: no se cual es este binario: %v", err)
	}
	args := []string{"-test.run=^" + test + "$", "-test.count=1", "-test.v"}

	switch papel := os.Getenv(raPapelRSS); papel {
	case "hijo-flaco":
		time.Sleep(raSiesta)

	case "hijo-gloton":
		raOcupar(t, raGloton)
		time.Sleep(raSiesta)

	case "padre-gordo":
		raOcupar(t, raLastreDelPadre)
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

	case "":
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, args...)
		cmd.Env = append(os.Environ(), raPapelRSS+"=padre-gordo")
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("el experimento con el padre gordo fallo (%v):\n%s", err, raw)
		}
		t.Logf("el experimento con el padre gordo:\n%s", raw)

	default:
		t.Fatalf("fixture: %s=%q no es un papel del experimento", raPapelRSS, papel)
	}
}
