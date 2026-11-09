//go:build !windows

// Run y la comprobacion de los enlaces que falla cerrada (CA-524, CA-522,
// CA-533; hallazgos 20261109T013223_4491ad, 20261109T013252_86d3e3,
// 20261109T012552_08f821 y 20261109T012437_ca4fb9 de la review delta 2 de
// tablero-en-la-terminal). Del contrato de Run (go doc del paquete): "The
// check fails closed: when it cannot be made (the root does not open, a
// component cannot be read) no editor opens either, and the bar says in red
// "no se pudo abrir el editor: <error>"; a file that is not there yet, under
// components inside the root, opens. Nothing outside os.Root is looked at to
// decide." Escrito desde ese contrato, sin leer la implementacion. Solo Unix:
// los enlaces se crean con os.Symlink y los permisos se niegan con chmod
// 0o000 (como root no niegan nada: esos casos se saltan). Con los mundos y
// las ayudas de tt_run_enlaces_unix_test.go y tt_run_test.go (terminal,
// datos, lanzador y editor falsos; Intervalo chico; cada espera con su tiempo
// maximo).
package tablero_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ttMsgNoSePudo es el comienzo del aviso de un editor que no se abre (el
// resto es el error, que da el sistema).
const ttMsgNoSePudo = "no se pudo abrir el editor: "

// ttSinRoot salta el test cuando corre como root: root lee lo que los
// permisos 0o000 niegan, y el caso no se puede armar.
func ttSinRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("como root los permisos 0o000 no niegan nada")
	}
}

// ttCerrar deja el directorio p sin permisos (0o000) y devuelve como
// abrirlo de nuevo (0o755); t.Cleanup lo abre igual, para que TempDir pueda
// borrarlo. Cuando el sistema no niega la lectura (un sistema de archivos que
// no mira los permisos), salta el test: el caso no se puede armar.
func ttCerrar(t *testing.T, p string) (abrir func()) {
	t.Helper()
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatalf("no pude cerrar %s: %v", p, err)
	}
	var una sync.Once
	abrir = func() {
		una.Do(func() {
			if err := os.Chmod(p, 0o755); err != nil {
				t.Errorf("no pude abrir de nuevo %s: %v", p, err)
			}
		})
	}
	t.Cleanup(abrir)
	if _, err := os.ReadDir(p); err == nil {
		abrir()
		t.Skipf("el sistema no niega leer %s con 0o000", p)
	}
	return abrir
}

// ttMundoQueCierra es ttMundoConEnlaces con color (sin NO_COLOR, para ver
// el rojo) y 200 columnas (el error del sistema entra en la barra).
func ttMundoQueCierra(t *testing.T, archivo string) *ttMundo {
	t.Helper()
	w := ttMundoConEnlaces(t, archivo)
	w.term.cambiar(200, 24)
	env := map[string]string{"HOOM_ASCII": "1", "LANG": "C"}
	w.e.Getenv = func(k string) string { return env[k] }
	return w
}

// ttEscritoDesde es todo lo que Run escribio en la terminal desde marca, sin
// las SGR del color.
func ttEscritoDesde(w *ttMundo, marca int) string {
	var b strings.Builder
	for _, e := range w.r.copia()[marca:] {
		if e.que == "escribe" {
			b.WriteString(e.dato)
		}
	}
	return ttSGR.ReplaceAllString(b.String(), "")
}

// ttBarraEnRojo espera una linea del ultimo cuadro con texto y exige que
// texto vaya en rojo; devuelve la linea, con sus SGR.
func ttBarraEnRojo(t *testing.T, ca, que string, w *ttMundo, texto string) string {
	t.Helper()
	var barra string
	ttEsperar(t, ca, que+": "+texto+" en la barra", w, func() bool {
		for _, l := range w.ultimoMarco() {
			if strings.Contains(l, texto) {
				barra = l
				return true
			}
		}
		return false
	})
	if !ttConColor(ttRojo, barra, texto) {
		t.Fatalf("%s: %s: %q va en rojo: %q", ca, que, texto, barra)
	}
	return barra
}

// ttNoSePudo exige el aviso de la comprobacion que no se pudo hacer: la barra
// dice en rojo "no se pudo abrir el editor: <error>", con un error que no
// esta vacio, y desde marca nada dice que el archivo esta fuera del proyecto.
func ttNoSePudo(t *testing.T, ca, que string, w *ttMundo, marca int) {
	t.Helper()
	barra := ttBarraEnRojo(t, ca, que, w, ttMsgNoSePudo)
	resto := barra[strings.Index(barra, ttMsgNoSePudo)+len(ttMsgNoSePudo):]
	if strings.TrimSpace(ttSGR.ReplaceAllString(resto, "")) == "" {
		t.Fatalf("%s: %s: el aviso lleva el error despues de %q: %q", ca, que, ttMsgNoSePudo, barra)
	}
	if s := ttEscritoDesde(w, marca); strings.Contains(s, "fuera del proyecto") {
		t.Fatalf("%s: %s: una comprobacion que no se pudo hacer no dice que el archivo esta fuera del proyecto: %q",
			ca, que, ttCola(s, 2000))
	}
}

// ttSinErr exige que Run no haya escrito nada en Err.
func ttSinErr(t *testing.T, ca, que string, w *ttMundo) {
	t.Helper()
	if got := w.r.texto("err"); got != "" {
		t.Fatalf("%s: %s: el aviso va a la barra, no a Err: %q", ca, que, got)
	}
}

// CA-524, CA-522, CA-533 (hallazgos 20261109T013223_4491ad,
// 20261109T013252_86d3e3, 20261109T012552_08f821, 20261109T012437_ca4fb9):
// un componente que no se puede leer ("cerrado/", un directorio de adentro
// con permisos 0o000) hace que la comprobacion no se pueda hacer: enter sobre
// un hallazgo, y o con el spec, en "cerrado/x.go" (que existe) o
// "cerrado/nuevo.go" (que no se sabe si existe: no se puede mirar) no llaman
// al Abrir del Editor falso, la barra dice en rojo "no se pudo abrir el
// editor: <error>" y nunca "esta fuera del proyecto"; la TUI sigue (q sale
// con 0, nada en Err). Los permisos se niegan con Run ya arrancado.
func TestCA524_RunComponenteQueNoSeLeeNoAbre(t *testing.T) {
	ttSinRoot(t)
	for _, desde := range []string{"hallazgo", "spec"} {
		for _, archivo := range []string{"cerrado/x.go", "cerrado/nuevo.go"} {
			que := desde + " en " + archivo
			t.Run(que, func(t *testing.T) {
				w := ttMundoQueCierra(t, archivo)
				ttArchivo(t, w.e.Dir, "cerrado/x.go", "package cerrado\n")
				cerrado := filepath.Join(w.e.Dir, "cerrado")
				fin, marca, _ := ttPedirEditorTras(t, "CA-524", w, desde, archivo, func() { ttCerrar(t, cerrado) })
				ttNoSePudo(t, "CA-524", que, w, marca)
				ttNoAbrio(t, "CA-524", que, w, marca)
				ttSalirConQ(t, "CA-533", que, w, fin)
				ttNoAbrio(t, "CA-524", que, w, marca)
				ttSinErr(t, "CA-524", que, w)
			})
		}
	}
}

// CA-524, CA-522, CA-533 (hallazgos 20261109T013223_4491ad,
// 20261109T013252_86d3e3, 20261109T012552_08f821, 20261109T012437_ca4fb9):
// la raiz del proyecto que no se abre. Con Run ya arrancado (el hoom.yaml se
// lee al empezar) y la tarjeta abierta, la raiz queda con permisos 0o000: ni
// se abre ni se puede buscar nada en ella, la abra Run cuando la abra. enter
// sobre un hallazgo, y o con el spec, en "real.go" (que existe) o "nuevo.go"
// (que no se sabe si existe) no llaman al Abrir, la barra dice en rojo "no se
// pudo abrir el editor: <error>" y nunca "esta fuera del proyecto". La TUI
// sigue y la comprobacion se hace de nuevo con cada pedido: con la raiz
// abierta otra vez, la misma tecla abre el editor con raiz/<archivo>; q sale
// con 0.
func TestCA524_RunRaizQueNoAbreNoAbre(t *testing.T) {
	ttSinRoot(t)
	for _, desde := range []string{"hallazgo", "spec"} {
		for _, archivo := range []string{"real.go", "nuevo.go"} {
			que := desde + " en " + archivo + " con la raiz cerrada"
			t.Run(que, func(t *testing.T) {
				w := ttMundoQueCierra(t, archivo)
				var abrirRaiz func()
				fin, marca, linea := ttPedirEditorTras(t, "CA-524", w, desde, archivo, func() { abrirRaiz = ttCerrar(t, w.e.Dir) })
				ttNoSePudo(t, "CA-524", que, w, marca)
				ttNoAbrio(t, "CA-524", que, w, marca)
				abrirRaiz()
				otra := len(w.r.copia())
				w.term.mandar(ttTeclaEditor(desde))
				ttAbrioCon(t, "CA-524", que+", abierta otra vez", w, otra, fmt.Sprintf("%s:%d", filepath.Join(w.e.Dir, archivo), linea))
				ttSalirConQ(t, "CA-533", que, w, fin)
				ttSinErr(t, "CA-524", que, w)
			})
		}
	}
}

// CA-524, CA-522, CA-533 (hallazgos 20261109T013223_4491ad,
// 20261109T013252_86d3e3, 20261109T012552_08f821, 20261109T012437_ca4fb9):
// nada de afuera de os.Root se mira para decidir. Enlaces del proyecto a un
// directorio de afuera con permisos 0o000 ("lcerrado", con x.go adentro), a
// un archivo dentro de ese directorio ("fcerrado") y a un directorio de
// afuera que no existe ("dnoexiste"), y el "dangling" de la ronda 2: enter
// sobre un hallazgo, y o con el spec, en "lcerrado/x.go", "lcerrado/nuevo.go",
// "lcerrado", "fcerrado", "dnoexiste/x.go" o "dangling" dicen en rojo `el
// archivo "<archivo>" esta fuera del proyecto` y no llaman al Abrir: los
// permisos y la existencia de lo de afuera no cambian la respuesta (mirarlo
// daria "no se pudo abrir el editor", que tampoco se dice). q sale con 0.
func TestCA524_RunNadaDeAfueraDecide(t *testing.T) {
	casos := []string{"lcerrado/x.go", "lcerrado/nuevo.go", "lcerrado", "fcerrado", "dnoexiste/x.go", "dangling"}
	for _, desde := range []string{"hallazgo", "spec"} {
		for _, archivo := range casos {
			que := desde + " en " + archivo
			t.Run(que, func(t *testing.T) {
				w := ttMundoQueCierra(t, archivo)
				afuera, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				ttArchivo(t, afuera, "cerrado/x.go", "package afuera\n")
				for enlace, destino := range map[string]string{
					"lcerrado":  filepath.Join(afuera, "cerrado"),
					"fcerrado":  filepath.Join(afuera, "cerrado", "x.go"),
					"dnoexiste": filepath.Join(afuera, "no-existe"),
				} {
					if err := os.Symlink(destino, filepath.Join(w.e.Dir, enlace)); err != nil {
						t.Fatalf("no pude crear el enlace %s -> %s: %v", enlace, destino, err)
					}
				}
				if _, err := os.Lstat(filepath.Join(afuera, "no-existe")); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("el destino de dnoexiste no existe: %v", err)
				}
				if os.Geteuid() != 0 {
					ttCerrar(t, filepath.Join(afuera, "cerrado"))
				}
				fin, marca, _ := ttPedirEditorTras(t, "CA-524", w, desde, archivo, func() {})
				ttBarraEnRojo(t, "CA-524", que, w, ttFuera(archivo))
				ttNoAbrio(t, "CA-524", que, w, marca)
				if s := ttEscritoDesde(w, marca); strings.Contains(s, ttMsgNoSePudo) {
					t.Fatalf("CA-524: %s: lo de afuera no se mira: no dice %q: %q", que, ttMsgNoSePudo, ttCola(s, 2000))
				}
				ttSalirConQ(t, "CA-533", que, w, fin)
				ttNoAbrio(t, "CA-524", que, w, marca)
				ttSinErr(t, "CA-524", que, w)
			})
		}
	}
}

// CA-524, CA-522, CA-533 (hallazgos 20261109T013223_4491ad,
// 20261109T013252_86d3e3, 20261109T012552_08f821, 20261109T012437_ca4fb9):
// un archivo que todavia no esta, bajo componentes de adentro, abre: ademas
// del "nuevo.go" de la ronda 2, uno bajo un directorio que falta
// ("sub/nueva/x.go"), uno bajo dos que faltan ("nueva/otra/x.go"), uno por un
// enlace de adentro a un directorio ("dsub/nuevo.go") y uno por ese enlace y
// un directorio que falta ("dsub/nueva/x.go"): enter sobre un hallazgo, y o
// con el spec, llaman al Abrir con raiz/<archivo> y su linea, sin aviso de
// fuera del proyecto ni de editor que no se pudo abrir; q sale con 0.
func TestCA524_RunArchivoNuevoBajoComponentesDeAdentroAbre(t *testing.T) {
	casos := []string{"sub/nueva/x.go", "nueva/otra/x.go", "dsub/nuevo.go", "dsub/nueva/x.go"}
	for _, desde := range []string{"hallazgo", "spec"} {
		for _, archivo := range casos {
			que := desde + " en " + archivo
			t.Run(que, func(t *testing.T) {
				w := ttMundoConEnlaces(t, archivo)
				if _, err := os.Lstat(filepath.Join(w.e.Dir, archivo)); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("%s todavia no existe: %v", archivo, err)
				}
				fin, marca, linea := ttPedirEditor(t, "CA-524", w, desde, archivo)
				ttAbrioCon(t, "CA-524", que, w, marca, fmt.Sprintf("%s:%d", filepath.Join(w.e.Dir, archivo), linea))
				if s := ttEscritoDesde(w, marca); strings.Contains(s, ttMsgNoSePudo) {
					t.Fatalf("CA-524: %s: un archivo nuevo de adentro no dice %q: %q", que, ttMsgNoSePudo, ttCola(s, 2000))
				}
				ttSalirConQ(t, "CA-533", que, w, fin)
				ttSinErr(t, "CA-524", que, w)
			})
		}
	}
}
