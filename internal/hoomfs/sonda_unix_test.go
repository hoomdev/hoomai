//go:build !windows

// Los casos de la sonda de la enmienda 2 de .hoom/specs/evidencia-en-disco.md
// (CA-445: sin /dev/fd ni /proc "la evidencia queda ilegible y el detalle
// dice que hoom necesita /dev/fd o /proc"), hallazgo 2d68e4 de la sexta
// ronda de review: sondear trataba un fallo al abrir "/" como
// disponibilidad, DescriptoresDisponibles quedaba en nil para siempre, y la
// foto que despues no podia reabrir la evidencia no decia por que.
//
// El contrato: sondear prueba con el PRIMER directorio de dirsSonda que se
// abre (por defecto "/", "." y os.TempDir(), en ese orden). Con ninguno que
// se abra no miro, y lo dice: ErrSinSonda ("hoom necesita /dev/fd o /proc y
// no pudo comprobar si los hay") con el fallo de apertura de cada uno, en
// orden, separados por "; " y sin saltos de linea; nunca nil sin haber
// reabierto un descriptor, y nunca ErrSinDescriptores (no comprobo que
// falten). Con uno que se abre, lo de siempre: nil si alguna carpeta de
// fdDirs lo reabre, si no ErrSinDescriptores.
//
// La costura es la del paquete (dirsSonda, fdDirs, sondear). Estos tests la
// cambian, asi que NO corren en paralelo y la restauran al final; antes de
// cambiarla llenan el cache de DescriptoresDisponibles con la sonda de
// verdad (sondaUsar, fdUsar). DescriptoresDisponibles en si (mira una vez
// por proceso) se pregunta en un proceso aparte, con la costura puesta
// antes de mirar.
//
// Escritos desde el contrato (go doc y la costura que expone el writer), sin
// leer la implementacion.
package hoomfs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// sondaPorDefecto son los directorios con que sondear prueba por defecto,
// en orden.
func sondaPorDefecto() []string { return []string{"/", ".", os.TempDir()} }

// sondaUsar pone dirsSonda = dirs hasta el final del test (o subtest) y lo
// restaura. Antes llena el cache de DescriptoresDisponibles con la sonda de
// verdad y devuelve lo que dijo.
func sondaUsar(t *testing.T, dirs ...string) error {
	t.Helper()
	cache := DescriptoresDisponibles()
	viejo := dirsSonda
	dirsSonda = append([]string(nil), dirs...)
	t.Cleanup(func() { dirsSonda = viejo })
	return cache
}

// sondaForma es una entrada de dirsSonda: un directorio que se abre, o algo
// que no se abre y la causa (el errno) que tiene que dar abrirlo.
type sondaForma struct {
	caso  string
	slug  string
	abre  bool
	causa syscall.Errno
	armar func(t *testing.T, p string)
}

// sondaFormas son las formas de una entrada de dirsSonda: no existe, es un
// directorio sin permiso (modo 000; se omite como root, que entra igual), o
// es un directorio que se abre.
func sondaFormas(t *testing.T) []sondaForma {
	t.Helper()
	todas := []sondaForma{
		{"no existe", "no-existe", false, syscall.ENOENT, func(t *testing.T, p string) {}},
		{"es un directorio sin permiso (modo 000)", "sin-permiso", false, syscall.EACCES, func(t *testing.T, p string) {
			if err := os.Mkdir(p, 0o700); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			if err := os.Chmod(p, 0); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			t.Cleanup(func() { os.Chmod(p, 0o700) })
		}},
		{"es un directorio que se abre", "directorio", true, 0, func(t *testing.T, p string) {
			aeEscribir(t, p, "LEEME", []byte("un directorio cualquiera para sondear\n"))
		}},
	}
	if os.Geteuid() != 0 {
		return todas
	}
	var out []sondaForma
	for _, f := range todas {
		if f.causa == syscall.EACCES {
			t.Logf("CA-445: como root se omite %q (root entra igual)", f.caso)
			continue
		}
		out = append(out, f)
	}
	return out
}

// sondaPartir separa las formas en la que se abre y las que no.
func sondaPartir(formas []sondaForma) (sondaForma, []sondaForma) {
	var abre sondaForma
	var cierran []sondaForma
	for _, f := range formas {
		if f.abre {
			abre = f
		} else {
			cierran = append(cierran, f)
		}
	}
	return abre, cierran
}

// sondaArmar deja la forma f en base/sonda<i>-<slug> y devuelve la ruta.
func sondaArmar(t *testing.T, base string, i int, f sondaForma) string {
	t.Helper()
	p := filepath.Join(base, "sonda"+strconv.Itoa(i)+"-"+f.slug)
	f.armar(t, p)
	return p
}

// sondaVisto es lo que dijo una sonda (sondear o DescriptoresDisponibles):
// nil, ErrSinSonda, ErrSinDescriptores, y su texto.
type sondaVisto struct {
	Nil             bool
	SinSonda        bool
	SinDescriptores bool
	Msg             string
}

func sondaVer(err error) sondaVisto {
	v := sondaVisto{Nil: err == nil, SinSonda: errors.Is(err, ErrSinSonda), SinDescriptores: errors.Is(err, ErrSinDescriptores)}
	if err != nil {
		v.Msg = err.Error()
	}
	return v
}

// sondaNoMiro exige que v sea lo de una sonda que no pudo mirar (ningun
// directorio de dirsSonda se abrio): no nil (nunca nil sin haber reabierto
// un descriptor), ErrSinSonda, no ErrSinDescriptores (no comprobo que
// falten), que diga "/dev/fd o /proc", en una linea, y con el fallo de
// apertura de cada entrada de rutas, en orden, separados por "; " (causas[i]
// es su errno; 0, solo la ruta).
func sondaNoMiro(t *testing.T, quien, caso string, v sondaVisto, rutas []string, causas []syscall.Errno) {
	t.Helper()
	if v.Nil {
		t.Fatalf("CA-445 (2d68e4): %s: sin ningun directorio de dirsSonda que se abra, %s no miro: nunca es nil sin haber reabierto un descriptor", caso, quien)
	}
	if !v.SinSonda {
		t.Fatalf("CA-445 (2d68e4): %s: sin ningun directorio de dirsSonda que se abra, %s es ErrSinSonda (no pudo comprobar), no %q", caso, quien, v.Msg)
	}
	if v.SinDescriptores {
		t.Fatalf("CA-445 (2d68e4): %s: sin ningun directorio de dirsSonda que se abra, %s no es ErrSinDescriptores: no comprobo que falten /dev/fd y /proc: %q", caso, quien, v.Msg)
	}
	if !strings.Contains(v.Msg, "/dev/fd o /proc") {
		t.Fatalf("CA-445 (2d68e4): %s: %s dice que hoom necesita \"/dev/fd o /proc\": %q", caso, quien, v.Msg)
	}
	if strings.ContainsAny(v.Msg, "\n\r") {
		t.Fatalf("CA-445 (2d68e4): %s: %s: los fallos van en una linea, separados por \"; \", sin saltos: %q", caso, quien, v.Msg)
	}
	if len(rutas) > 0 {
		fdCausas(t, quien, caso, v.Msg, rutas, causas, nil)
	}
}

// sondaMiro exige que v sea lo de una sonda que miro con un directorio de
// dirsSonda que se abrio: nunca ErrSinSonda; con las carpetas de
// descriptores de siempre (fdRutas nil), nil si este sistema reabre por
// alguna (anda) y si no ErrSinDescriptores; con fdRutas (ninguna sirve),
// ErrSinDescriptores con la causa de cada una (fdErrnos).
func sondaMiro(t *testing.T, quien, caso string, v sondaVisto, anda string, fdRutas []string, fdErrnos []syscall.Errno) {
	t.Helper()
	if v.SinSonda {
		t.Fatalf("CA-445 (2d68e4): %s: con un directorio de dirsSonda que se abre, %s miro: no es ErrSinSonda: %q", caso, quien, v.Msg)
	}
	switch {
	case fdRutas == nil && anda != "":
		if !v.Nil {
			t.Fatalf("CA-445 (2d68e4): %s: con un directorio de dirsSonda que se abre y %s que reabre, %s es nil, no %q", caso, anda, quien, v.Msg)
		}
	case fdRutas == nil:
		if !v.SinDescriptores {
			t.Fatalf("CA-445 (2d68e4): %s: con un directorio de dirsSonda que se abre en un sistema que no reabre por /dev/fd ni /proc/self/fd, %s es ErrSinDescriptores, no %q", caso, quien, v.Msg)
		}
	default:
		if v.Nil || !v.SinDescriptores {
			t.Fatalf("CA-445 (2d68e4): %s: con un directorio de dirsSonda que se abre y ninguna carpeta de descriptores que sirva (%q), %s es ErrSinDescriptores, no %q", caso, fdRutas, quien, v.Msg)
		}
		fdCausas(t, quien, caso, v.Msg, fdRutas, fdErrnos, nil)
	}
}

// sondaMiraUnaVez exige que DescriptoresDisponibles siga diciendo lo que
// dijo con la sonda de verdad (cache): mira una vez por proceso.
func sondaMiraUnaVez(t *testing.T, caso string, cache error) {
	t.Helper()
	if otra := DescriptoresDisponibles(); otra != cache && !errors.Is(otra, cache) {
		t.Fatalf("CA-445 (2d68e4): %s: DescriptoresDisponibles mira una vez por proceso: con la sonda de verdad dijo %v y ahora, con dirsSonda %q, %v", caso, cache, dirsSonda, otra)
	}
}

// sondaFdInutiles arma dos carpetas de descriptores que no sirven (no
// existe; es un archivo regular) y devuelve sus rutas y sus errnos.
func sondaFdInutiles(t *testing.T) ([]string, []syscall.Errno) {
	t.Helper()
	base := t.TempDir()
	a := filepath.Join(base, "fd0-no-existe")
	b := filepath.Join(base, "fd1-archivo")
	if err := os.WriteFile(b, []byte("no soy una carpeta de descriptores\n"), 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return []string{a, b}, []syscall.Errno{syscall.ENOENT, syscall.ENOTDIR}
}

// CA-445 (hallazgo 2d68e4): sondear prueba por defecto con "/", "." y
// os.TempDir(), en ese orden, y con ellos mira (alguno se abre aca): no es
// ErrSinSonda. ErrSinSonda dice "hoom necesita /dev/fd o /proc y no pudo
// comprobar si los hay" (el diagnostico de CA-445, y que no pudo mirar), y
// no es ErrSinDescriptores ni al reves: no poder mirar no es haber
// comprobado que faltan.
func TestCA445_LaSondaPorDefectoYSuError(t *testing.T) {
	if !reflect.DeepEqual(dirsSonda, sondaPorDefecto()) {
		t.Fatalf("CA-445 (2d68e4): los directorios con que sondear prueba por defecto son %q, en ese orden, no %q", sondaPorDefecto(), dirsSonda)
	}
	if ErrSinSonda == nil {
		t.Fatal("CA-445 (2d68e4): ErrSinSonda existe")
	}
	if got, want := ErrSinSonda.Error(), "hoom necesita /dev/fd o /proc y no pudo comprobar si los hay"; got != want {
		t.Fatalf("CA-445 (2d68e4): ErrSinSonda dice %q, no %q", want, got)
	}
	if errors.Is(ErrSinSonda, ErrSinDescriptores) || errors.Is(ErrSinDescriptores, ErrSinSonda) {
		t.Fatal("CA-445 (2d68e4): ErrSinSonda (no pudo mirar) y ErrSinDescriptores (miro y no hay) son errores distintos")
	}
	var abre []string
	for _, d := range sondaPorDefecto() {
		if f, err := os.Open(d); err == nil {
			f.Close()
			abre = append(abre, d)
		}
	}
	if len(abre) == 0 {
		t.Logf("CA-445 (2d68e4): ninguno de %q se abre en este entorno: no se mira que sondear haya mirado", sondaPorDefecto())
		return
	}
	if s := sondear(); errors.Is(s, ErrSinSonda) {
		t.Fatalf("CA-445 (2d68e4): con los directorios por defecto (aca se abren %q), sondear miro: no es ErrSinSonda: %v", abre, s)
	}
	if d := DescriptoresDisponibles(); errors.Is(d, ErrSinSonda) {
		t.Fatalf("CA-445 (2d68e4): con los directorios por defecto (aca se abren %q), DescriptoresDisponibles miro: no es ErrSinSonda: %v", abre, d)
	}
}

// CA-445 (hallazgo 2d68e4): con dirsSonda sin ningun directorio que se abra
// (no existe; directorio sin permiso, modo 000), de a uno, de a dos en
// cualquier orden (tambien la misma forma dos veces) y de a tres, tambien
// con unicode en el nombre, sondear no miro y lo dice: ErrSinSonda con el
// fallo de apertura de cada uno en orden (su ruta y su errno), separados
// por "; ", en una linea, con "/dev/fd o /proc"; nunca nil y nunca
// ErrSinDescriptores, tampoco con fdDirs sin ninguna que sirva (no llego a
// probarlas). La lista vacia y la ruta vacia tampoco miran. Y
// DescriptoresDisponibles sigue diciendo lo que dijo: mira una vez.
func TestCA445_SinDirectorioQueSeAbraLaSondaNoMiroYLoDice(t *testing.T) {
	_, cierran := sondaPartir(sondaFormas(t))
	for _, f := range cierran {
		caso := "{" + f.slug + "}"
		t.Run(caso, func(t *testing.T) {
			a := sondaArmar(t, t.TempDir(), 0, f)
			cache := sondaUsar(t, a)
			sondaNoMiro(t, "sondear", caso, sondaVer(sondear()), []string{a}, []syscall.Errno{f.causa})
			sondaMiraUnaVez(t, caso, cache)
		})
	}
	for _, f1 := range cierran {
		for _, f2 := range cierran {
			caso := "{" + f1.slug + ", " + f2.slug + "}"
			t.Run(caso, func(t *testing.T) {
				base := t.TempDir()
				a, b := sondaArmar(t, base, 0, f1), sondaArmar(t, base, 1, f2)
				cache := sondaUsar(t, a, b)
				sondaNoMiro(t, "sondear", caso, sondaVer(sondear()), []string{a, b}, []syscall.Errno{f1.causa, f2.causa})
				sondaMiraUnaVez(t, caso, cache)
			})
		}
	}
	t.Run("con unicode en el nombre", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "sonda-ñandú-日本-no-existe")
		cache := sondaUsar(t, p)
		sondaNoMiro(t, "sondear", "{ñandú}", sondaVer(sondear()), []string{p}, []syscall.Errno{syscall.ENOENT})
		sondaMiraUnaVez(t, "{ñandú}", cache)
	})
	t.Run("tres, y fdDirs sin ninguna que sirva", func(t *testing.T) {
		base := t.TempDir()
		f0, fn := cierran[0], cierran[len(cierran)-1]
		rutas := []string{sondaArmar(t, base, 0, f0), sondaArmar(t, base, 1, fn), sondaArmar(t, base, 2, f0)}
		fd, _ := sondaFdInutiles(t)
		fdUsar(t, fd...)
		cache := sondaUsar(t, rutas...)
		sondaNoMiro(t, "sondear", "{tres que no se abren} con fdDirs que no sirven", sondaVer(sondear()), rutas, []syscall.Errno{f0.causa, fn.causa, f0.causa})
		sondaMiraUnaVez(t, "tres", cache)
	})
	t.Run("lista vacia", func(t *testing.T) {
		cache := sondaUsar(t)
		sondaNoMiro(t, "sondear", "{}", sondaVer(sondear()), nil, nil)
		sondaMiraUnaVez(t, "{}", cache)
	})
	t.Run("ruta vacia", func(t *testing.T) {
		cache := sondaUsar(t, "")
		sondaNoMiro(t, "sondear", `{""}`, sondaVer(sondear()), nil, nil)
		sondaMiraUnaVez(t, `{""}`, cache)
	})
}

// CA-445 (hallazgo 2d68e4): con un directorio de dirsSonda que se abre,
// detras de los que no se abren (uno o dos) o delante, sondear miro con el
// y dice lo de siempre: con las carpetas de descriptores de verdad, nil en
// un sistema que reabre (si no, ErrSinDescriptores); con fdDirs sin ninguna
// que sirva, ErrSinDescriptores con la causa de cada una. Nunca ErrSinSonda.
// Prueba con el PRIMERO que se abre: uno que se abre delante decide aunque
// despues venga un directorio que se abre y no se deja mirar (modo 0400).
func TestCA445_LaSondaPruebaConElPrimerDirectorioQueSeAbre(t *testing.T) {
	anda, probado := fdQueAnda(t)
	t.Logf("CA-445: la que anda en este sistema: %q (%v)", anda, probado)
	abre, cierran := sondaPartir(sondaFormas(t))
	type lista struct {
		nombre string
		formas []sondaForma
	}
	var listas []lista
	for _, f := range cierran {
		listas = append(listas,
			lista{"{" + f.slug + ", directorio}", []sondaForma{f, abre}},
			lista{"{directorio, " + f.slug + "}", []sondaForma{abre, f}},
			lista{"{" + f.slug + ", " + f.slug + ", directorio}", []sondaForma{f, f, abre}},
		)
	}
	listas = append(listas, lista{"{directorio}", []sondaForma{abre}})
	for _, l := range listas {
		for _, fdMalas := range []bool{false, true} {
			caso := l.nombre
			if fdMalas {
				caso += " con fdDirs que no sirven"
			}
			t.Run(caso, func(t *testing.T) {
				base := t.TempDir()
				var rutas []string
				for i, f := range l.formas {
					rutas = append(rutas, sondaArmar(t, base, i, f))
				}
				var fd []string
				var fdErr []syscall.Errno
				if fdMalas {
					fd, fdErr = sondaFdInutiles(t)
					fdUsar(t, fd...)
				}
				cache := sondaUsar(t, rutas...)
				sondaMiro(t, "sondear", caso, sondaVer(sondear()), anda, fd, fdErr)
				sondaMiraUnaVez(t, caso, cache)
			})
		}
	}
	t.Run("{directorio, uno que se abre y no se deja mirar}", func(t *testing.T) {
		if anda == "" {
			t.Skipf("CA-445: ninguna carpeta de descriptores por defecto reabre un directorio en este sistema (%v)", probado)
		}
		if statErr, _, porQueNo := fdNoSeMira(t); statErr == nil {
			t.Skipf("CA-445: no se puede armar un directorio que se abre y no se deja mirar: %s", porQueNo)
		}
		base := t.TempDir()
		d := sondaArmar(t, base, 0, abre)
		x := filepath.Join(base, "sonda1-no-se-mira")
		if err := os.Mkdir(x, 0o700); err != nil {
			t.Fatalf("fixture: %v", err)
		}
		if err := os.Chmod(x, 0o400); err != nil {
			t.Fatalf("fixture: %v", err)
		}
		t.Cleanup(func() { os.Chmod(x, 0o700) })
		cache := sondaUsar(t, d, x)
		v := sondaVer(sondear())
		if !v.Nil {
			t.Fatalf("CA-445 (2d68e4): con dirsSonda = {un directorio que se abre, uno que se abre y no se deja mirar}, sondear prueba con el PRIMERO que se abre: nil con %s, no %q", anda, v.Msg)
		}
		sondaMiraUnaVez(t, "{directorio, no se mira}", cache)
	})
}

// CA-445 (hallazgo 2d68e4), como propiedad: para TODA lista de una a tres
// entradas de dirsSonda tomadas de {no existe, modo 000, un directorio que
// se abre} (con repeticion, en cualquier orden), con las carpetas de
// descriptores de verdad o con unas que no sirven: si la lista tiene un
// directorio que se abre, este donde este, sondear miro (lo de siempre:
// nil o ErrSinDescriptores, nunca ErrSinSonda); si no, no miro: ErrSinSonda
// con el fallo de cada entrada en orden, nunca nil ni ErrSinDescriptores.
// DescriptoresDisponibles no cambia.
func TestCA445_CualquierListaDeSondasMiraSiYSoloSiTieneUnDirectorioQueSeAbre(t *testing.T) {
	anda, _ := fdQueAnda(t)
	formas := sondaFormas(t)
	var listas [][]int
	for n := 1; n <= 3; n++ {
		idx := make([]int, n)
		for {
			listas = append(listas, append([]int(nil), idx...))
			k := n - 1
			for k >= 0 && idx[k] == len(formas)-1 {
				idx[k] = 0
				k--
			}
			if k < 0 {
				break
			}
			idx[k]++
		}
	}
	for _, l := range listas {
		var nombres []string
		for _, j := range l {
			nombres = append(nombres, formas[j].slug)
		}
		for _, fdMalas := range []bool{false, true} {
			caso := "{" + strings.Join(nombres, ", ") + "}"
			if fdMalas {
				caso += " con fdDirs que no sirven"
			}
			t.Run(caso, func(t *testing.T) {
				base := t.TempDir()
				var rutas []string
				var causas []syscall.Errno
				tiene := false
				for i, j := range l {
					f := formas[j]
					rutas = append(rutas, sondaArmar(t, base, i, f))
					causas = append(causas, f.causa)
					tiene = tiene || f.abre
				}
				var fd []string
				var fdErr []syscall.Errno
				if fdMalas {
					fd, fdErr = sondaFdInutiles(t)
					fdUsar(t, fd...)
				}
				cache := sondaUsar(t, rutas...)
				v := sondaVer(sondear())
				if tiene {
					sondaMiro(t, "sondear", caso, v, anda, fd, fdErr)
				} else {
					sondaNoMiro(t, "sondear", caso, v, rutas, causas)
				}
				sondaMiraUnaVez(t, caso, cache)
			})
		}
	}
	t.Logf("CA-445: %d listas de dirsSonda, cada una con dos fdDirs", len(listas))
}

const sondaEnvPedido = "HOOMFS_SONDA_PEDIDO"

// sondaPedido es lo que el proceso aparte pone en la costura ANTES de
// preguntar por primera vez (Sonda en dirsSonda; Fd en fdDirs, si no es
// nil), lo que pone en dirsSonda entre la primera y la segunda pregunta
// (Despues), y donde deja lo que vio (Salida).
type sondaPedido struct {
	Sonda   []string
	Fd      []string
	Despues []string
	Salida  string
}

// sondaRespuesta es lo que vio el proceso aparte: DescriptoresDisponibles
// la primera vez, la segunda (con Despues en dirsSonda), y sondear sin cache
// con Despues (lo que habria dicho mirar de nuevo).
type sondaRespuesta struct {
	Primera, Segunda, Sondear sondaVisto
}

// sondaEnOtroProceso pregunta en un proceso que todavia no miro (el binario
// de este test, solo con TestHoomfsSondaSubproceso).
func sondaEnOtroProceso(t *testing.T, p sondaPedido) sondaRespuesta {
	t.Helper()
	p.Salida = filepath.Join(t.TempDir(), "visto.json")
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHoomfsSondaSubproceso$", "-test.count=1")
	cmd.Env = append(os.Environ(), sondaEnvPedido+"="+string(b))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CA-445: el proceso aparte fallo: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(p.Salida)
	if err != nil {
		t.Fatalf("CA-445: el proceso aparte no dejo lo que vio: %v\n%s", err, out)
	}
	var r sondaRespuesta
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("CA-445: lo que vio el proceso aparte: %v: %s", err, raw)
	}
	return r
}

// TestHoomfsSondaSubproceso no es un test: es el proceso aparte de
// TestCA445_DescriptoresDisponiblesSinSondaNoEsNilYMiraUnaVez, que pregunta
// en un proceso que todavia no miro. Sin la variable de entorno no hace
// nada.
func TestHoomfsSondaSubproceso(t *testing.T) {
	raw := os.Getenv(sondaEnvPedido)
	if raw == "" {
		return
	}
	var p sondaPedido
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("subproceso: pedido: %v", err)
	}
	dirsSonda = p.Sonda
	if p.Fd != nil {
		fdDirs = p.Fd
	}
	var r sondaRespuesta
	r.Primera = sondaVer(DescriptoresDisponibles())
	dirsSonda = p.Despues
	r.Segunda = sondaVer(DescriptoresDisponibles())
	r.Sondear = sondaVer(sondear())
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("subproceso: %v", err)
	}
	if err := os.WriteFile(p.Salida, b, 0o644); err != nil {
		t.Fatalf("subproceso: %v", err)
	}
}

// CA-445 (hallazgo 2d68e4), en un proceso que todavia no miro:
// DescriptoresDisponibles con dirsSonda sin ningun directorio que se abra
// NO es nil (el hallazgo: quedaba en nil para siempre y la foto no decia
// que hoom necesita /dev/fd o /proc): es ErrSinSonda con cada fallo. Y mira
// una vez: aunque despues haya un directorio que se abre (con el que
// sondear ya diria otra cosa), sigue diciendo lo mismo. Con un directorio
// que se abre detras de uno que no, miro con el (nil en un sistema que
// reabre, si no ErrSinDescriptores) y sigue asi aunque despues no se abra
// ninguno. Con fdDirs sin ninguna que sirva, ErrSinDescriptores con cada
// causa (miro y no hay), nunca ErrSinSonda.
func TestCA445_DescriptoresDisponiblesSinSondaNoEsNilYMiraUnaVez(t *testing.T) {
	anda, probado := fdQueAnda(t)
	t.Logf("CA-445: la que anda en este sistema: %q (%v)", anda, probado)
	abre, cierran := sondaPartir(sondaFormas(t))

	t.Run("sin directorio que se abra, y despues uno que si", func(t *testing.T) {
		base := t.TempDir()
		f0, fn := cierran[0], cierran[len(cierran)-1]
		a, b := sondaArmar(t, base, 0, f0), sondaArmar(t, base, 1, fn)
		d := sondaArmar(t, base, 2, abre)
		r := sondaEnOtroProceso(t, sondaPedido{Sonda: []string{a, b}, Despues: []string{d}})
		sondaNoMiro(t, "DescriptoresDisponibles", "{"+f0.slug+", "+fn.slug+"}", r.Primera, []string{a, b}, []syscall.Errno{f0.causa, fn.causa})
		if r.Segunda != r.Primera {
			t.Fatalf("CA-445 (2d68e4): DescriptoresDisponibles mira una vez por proceso: dijo %+v y, con un directorio que se abre en dirsSonda, %+v", r.Primera, r.Segunda)
		}
		sondaMiro(t, "sondear (sin cache, despues)", "{directorio}", r.Sondear, anda, nil, nil)
	})
	t.Run("un directorio que se abre detras de uno que no, y despues ninguno", func(t *testing.T) {
		base := t.TempDir()
		f0 := cierran[0]
		a, d := sondaArmar(t, base, 0, f0), sondaArmar(t, base, 1, abre)
		x := filepath.Join(base, "sonda2-no-existe")
		r := sondaEnOtroProceso(t, sondaPedido{Sonda: []string{a, d}, Despues: []string{x}})
		sondaMiro(t, "DescriptoresDisponibles", "{"+f0.slug+", directorio}", r.Primera, anda, nil, nil)
		if r.Segunda != r.Primera {
			t.Fatalf("CA-445 (2d68e4): DescriptoresDisponibles mira una vez por proceso: dijo %+v y, sin ningun directorio que se abra en dirsSonda, %+v", r.Primera, r.Segunda)
		}
		sondaNoMiro(t, "sondear (sin cache, despues)", "{no-existe}", r.Sondear, []string{x}, []syscall.Errno{syscall.ENOENT})
	})
	t.Run("un directorio que se abre y fdDirs sin ninguna que sirva", func(t *testing.T) {
		base := t.TempDir()
		d := sondaArmar(t, base, 0, abre)
		fd, fdErr := sondaFdInutiles(t)
		r := sondaEnOtroProceso(t, sondaPedido{Sonda: []string{d}, Fd: fd, Despues: []string{d}})
		sondaMiro(t, "DescriptoresDisponibles", "{directorio} con fdDirs que no sirven", r.Primera, anda, fd, fdErr)
		if r.Segunda != r.Primera {
			t.Fatalf("CA-445 (2d68e4): DescriptoresDisponibles mira una vez por proceso: dijo %+v y despues %+v", r.Primera, r.Segunda)
		}
	})
}

// sondearConReloj es sondear con reloj (arRelojPorApertura). Si no vuelve
// (se colgo abriendo el FIFO fifo), suelta el FIFO hasta que vuelva, para
// que nada siga usando la costura despues del test, y dice que se colgo.
func sondearConReloj(t *testing.T, fifo string) (error, bool) {
	t.Helper()
	ch := make(chan error, 1)
	go func() { ch <- sondear() }()
	select {
	case err := <-ch:
		return err, false
	case <-time.After(arRelojPorApertura):
	}
	limite := time.Now().Add(10 * time.Second)
	for time.Now().Before(limite) {
		if w, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
		select {
		case err := <-ch:
			return err, true
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf("CA-445 (2d68e4): sondear sigue colgado en el FIFO %s aun soltandolo", fifo)
	return nil, true
}

// CA-445 (hallazgo 2d68e4): sondear prueba con un DIRECTORIO de dirsSonda
// que se abre: dirsSonda son "los directorios con que sondear prueba", y
// sondear reabre un directorio como la foto reabre cada directorio de la
// evidencia, que lo abre como archivo con O_DIRECTORY|O_NONBLOCK
// (decisiones de la enmienda 2). Una entrada que existe y no es un
// directorio no es un directorio que se abre:
//   - un archivo regular no decide la sonda: con {archivo, un directorio}
//     prueba con el directorio (nil en un sistema que reabre); con
//     {archivo} no miro: ErrSinSonda con su ruta, nunca ErrSinDescriptores,
//     que diria que el sistema no reabre un directorio por su descriptor
//     sin haberlo probado con uno;
//   - un FIFO que nadie escribe no la cuelga (vuelve con reloj) ni la
//     decide: lo mismo con {FIFO, un directorio} y con {FIFO}.
func TestCA445_LaSondaSoloPruebaConUnDirectorio(t *testing.T) {
	anda, probado := fdQueAnda(t)
	t.Logf("CA-445: la que anda en este sistema: %q (%v)", anda, probado)
	abre, _ := sondaPartir(sondaFormas(t))
	t.Run("{archivo, directorio}", func(t *testing.T) {
		base := t.TempDir()
		p := filepath.Join(base, "sonda0-archivo")
		aeEscribir(t, base, "sonda0-archivo", []byte("no soy un directorio\n"))
		d := sondaArmar(t, base, 1, abre)
		cache := sondaUsar(t, p, d)
		sondaMiro(t, "sondear", "{archivo regular, directorio}: un archivo regular no es un directorio que se abre; prueba con el directorio", sondaVer(sondear()), anda, nil, nil)
		sondaMiraUnaVez(t, "{archivo, directorio}", cache)
	})
	t.Run("{archivo}", func(t *testing.T) {
		base := t.TempDir()
		p := filepath.Join(base, "sonda0-archivo")
		aeEscribir(t, base, "sonda0-archivo", []byte("no soy un directorio\n"))
		cache := sondaUsar(t, p)
		sondaNoMiro(t, "sondear", "{archivo regular}: un archivo regular no es un directorio que se abre", sondaVer(sondear()), []string{p}, []syscall.Errno{0})
		sondaMiraUnaVez(t, "{archivo}", cache)
	})
	t.Run("{FIFO, directorio}", func(t *testing.T) {
		base := t.TempDir()
		fifo := filepath.Join(base, "sonda0-fifo")
		carreratest.Fifo(t, fifo)
		d := sondaArmar(t, base, 1, abre)
		cache := sondaUsar(t, fifo, d)
		err, colgo := sondearConReloj(t, fifo)
		if colgo {
			t.Errorf("CA-445 (2d68e4): {FIFO, directorio}: un FIFO que nadie escribe no cuelga la sonda: sondear no volvio en %s (volvio soltando el FIFO con %v)", arRelojPorApertura, err)
		}
		sondaMiro(t, "sondear", "{FIFO, directorio}: un FIFO no es un directorio que se abre; prueba con el directorio", sondaVer(err), anda, nil, nil)
		sondaMiraUnaVez(t, "{FIFO, directorio}", cache)
	})
	t.Run("{FIFO}", func(t *testing.T) {
		fifo := filepath.Join(t.TempDir(), "sonda0-fifo")
		carreratest.Fifo(t, fifo)
		cache := sondaUsar(t, fifo)
		err, colgo := sondearConReloj(t, fifo)
		if colgo {
			t.Errorf("CA-445 (2d68e4): {FIFO}: un FIFO que nadie escribe no cuelga la sonda: sondear no volvio en %s (volvio soltando el FIFO con %v)", arRelojPorApertura, err)
		}
		sondaNoMiro(t, "sondear", "{FIFO}: un FIFO no es un directorio que se abre", sondaVer(err), []string{fifo}, []syscall.Errno{0})
		sondaMiraUnaVez(t, "{FIFO}", cache)
	})
}
