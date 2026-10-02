//go:build !windows

// Los casos de las carpetas de descriptores de la enmienda 2 de
// .hoom/specs/evidencia-en-disco.md (CA-445: "sin /dev/fd ni /proc ... la
// evidencia queda ilegible y el detalle dice que hoom necesita /dev/fd o
// /proc"), hallazgos 669a42 y c32559 de la quinta ronda de review: el
// contrato admite cualquiera de los dos mecanismos, asi que una carpeta de
// descriptores que no sirve (no existe, es un archivo, no deja entrar, no es
// de descriptores) no tapa la siguiente que si; y un fallo de todas no se
// vuelve "disponible": es ErrSinDescriptores con cada causa.
//
// La costura es la del paquete (fdDirs: donde rootDe reabre un descriptor,
// en orden; sondear: la sonda sin cache de DescriptoresDisponibles). Estos
// tests la cambian, asi que NO corren en paralelo y la restauran al final;
// antes de cambiarla llenan el cache de DescriptoresDisponibles con la de
// verdad, para que ningun otro test herede una sonda de estas.
//
// "La que anda" es la primera de las carpetas por defecto (/dev/fd,
// /proc/self/fd) por la que este sistema reabre DE VERDAD un directorio
// abierto por su descriptor (el mismo directorio, os.SameFile): que exista
// no alcanza (el refutador lo mostro con sandbox-exec negando /dev/fd/N).
// Si ninguna anda, los casos que la necesitan se omiten y lo dice el log;
// los que no la necesitan corren igual.
//
// Escritos desde el contrato (go doc y la costura que expone el writer), sin
// leer la implementacion.
package hoomfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// fdPorDefecto son las carpetas de descriptores por defecto del contrato.
var fdPorDefecto = []string{"/dev/fd", "/proc/self/fd"}

// fdUsar pone fdDirs = dirs hasta el final del test (o subtest) y lo
// restaura. Antes llena el cache de DescriptoresDisponibles con las de
// verdad y devuelve lo que dijo.
func fdUsar(t *testing.T, dirs ...string) error {
	t.Helper()
	cache := DescriptoresDisponibles()
	viejo := fdDirs
	fdDirs = append([]string(nil), dirs...)
	t.Cleanup(func() { fdDirs = viejo })
	return cache
}

// fdReabrir reabre, por la carpeta de descriptores base, un directorio
// abierto por su descriptor, y dice si lo que abrio es ESE directorio (nil)
// o por que no.
func fdReabrir(t *testing.T, base string) error {
	t.Helper()
	dir := t.TempDir()
	f, err := os.Open(dir)
	if err != nil {
		t.Fatalf("fixture: abrir %s: %v", dir, err)
	}
	defer f.Close()
	want, err := f.Stat()
	if err != nil {
		t.Fatalf("fixture: Stat de %s: %v", dir, err)
	}
	p := base + "/" + strconv.Itoa(int(f.Fd()))
	r, err := os.OpenRoot(p)
	runtime.KeepAlive(f)
	if err != nil {
		return err
	}
	defer r.Close()
	fi, err := r.Stat(".")
	if err != nil {
		return fmt.Errorf("%s se abre pero no se mira: %w", p, err)
	}
	if !os.SameFile(fi, want) {
		return fmt.Errorf("%s abre otro directorio, no %s", p, dir)
	}
	return nil
}

// fdQueAnda devuelve la primera carpeta por defecto por la que este sistema
// reabre de verdad un directorio por su descriptor ("" si ninguna), y lo que
// paso con cada una.
func fdQueAnda(t *testing.T) (string, map[string]error) {
	t.Helper()
	probado := map[string]error{}
	anda := ""
	for _, base := range fdPorDefecto {
		probado[base] = fdReabrir(t, base)
		if probado[base] == nil && anda == "" {
			anda = base
		}
	}
	return anda, probado
}

// fdLaQueAnda es fdQueAnda, u omite el test si ninguna anda.
func fdLaQueAnda(t *testing.T) string {
	t.Helper()
	anda, probado := fdQueAnda(t)
	if anda == "" {
		t.Skipf("CA-445: ninguna carpeta de descriptores por defecto reabre un directorio en este sistema (%v): no hay \"la que anda\" para este caso", probado)
	}
	return anda
}

// fdInutil es una carpeta de descriptores que no sirve, y la causa (el
// errno) que tiene que dar reabrir un descriptor por ella.
type fdInutil struct {
	caso  string
	slug  string
	causa syscall.Errno
	armar func(t *testing.T, p string)
}

// fdInutiles son las formas de una carpeta de descriptores que no sirve. La
// de modo 000 se omite como root (root entra igual).
func fdInutiles(t *testing.T) []fdInutil {
	t.Helper()
	todas := []fdInutil{
		{"no existe", "no-existe", syscall.ENOENT, func(t *testing.T, p string) {}},
		{"es un archivo regular", "archivo", syscall.ENOTDIR, func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte("no soy una carpeta de descriptores\n"), 0o644); err != nil {
				t.Fatalf("fixture: %v", err)
			}
		}},
		{"es un directorio sin permiso (modo 000)", "sin-permiso", syscall.EACCES, func(t *testing.T, p string) {
			if err := os.Mkdir(p, 0o700); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			if err := os.Chmod(p, 0); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			t.Cleanup(func() { os.Chmod(p, 0o700) })
		}},
		{"es un directorio que no es de descriptores", "no-es-de-descriptores", syscall.ENOENT, func(t *testing.T, p string) {
			aeEscribir(t, p, "LEEME", []byte("aca no hay descriptores\n"))
			aeEscribir(t, p, "fd/LEEME", []byte("tampoco\n"))
		}},
	}
	if os.Geteuid() != 0 {
		return todas
	}
	var out []fdInutil
	for _, u := range todas {
		if u.causa == syscall.EACCES {
			t.Logf("CA-445: como root se omite %q (root entra igual)", u.caso)
			continue
		}
		out = append(out, u)
	}
	return out
}

// fdArmar deja la forma u en base/entrada<i>-<slug> y devuelve la ruta.
func fdArmar(t *testing.T, base string, i int, u fdInutil) string {
	t.Helper()
	p := filepath.Join(base, "entrada"+strconv.Itoa(i)+"-"+u.slug)
	u.armar(t, p)
	return p
}

// fdEvidencia arma un directorio con un hallazgo adentro, abre su padre como
// root y devuelve el root, el Lstat del directorio y su nombre.
func fdEvidencia(t *testing.T) (*os.Root, os.FileInfo, string) {
	t.Helper()
	dir := t.TempDir()
	const nombre = "findings"
	aeEscribir(t, dir, nombre+"/20261002T161604_669a42.json", []byte("{\"severity\":\"medium\"}\n"))
	r := aeRoot(t, dir)
	return r, aeLstat(t, r, nombre), nombre
}

// fdAbre exige que, con las carpetas de descriptores que hay ahora en
// fdDirs, AbrirDirEn abra el directorio mirado (os.SameFile, y lista lo
// suyo) y que sondear sea nil.
func fdAbre(t *testing.T, caso string) {
	t.Helper()
	r, antes, nombre := fdEvidencia(t)
	d, err := aeAbrirDir(t, caso, r, nombre, antes)
	if err != nil || d == nil {
		t.Fatalf("CA-445: %s: con una carpeta de descriptores que anda en %q, AbrirDirEn abre el directorio mirado: root=%v err=%v", caso, fdDirs, d != nil, err)
	}
	if fi, err := d.Stat("."); err != nil || !os.SameFile(fi, antes) {
		t.Fatalf("CA-445: %s: AbrirDirEn abre el MISMO directorio que su Lstat (os.SameFile), por cualquier carpeta de %q: %v", caso, fdDirs, err)
	}
	if got, want := aeNombres(t, d), []string{"20261002T161604_669a42.json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CA-445: %s: el directorio que abre AbrirDirEn por %q es el mirado: lista %v, no %v", caso, fdDirs, got, want)
	}
	if err := sondear(); err != nil {
		t.Fatalf("CA-445: %s: con una carpeta de descriptores que anda en %q, sondear es nil, no %v", caso, fdDirs, err)
	}
}

// fdCausas exige que msg sea el de ErrSinDescriptores con la causa de cada
// carpeta de descriptores probada: la ruta de cada una en su orden, cada
// una con su errno en su tramo, separadas por "; ", y sin saltos de linea.
func fdCausas(t *testing.T, quien, caso, msg string, rutas []string, causas []syscall.Errno) {
	t.Helper()
	if !strings.Contains(msg, "/dev/fd o /proc") {
		t.Fatalf("CA-445: %s: %s: el error dice que hoom necesita \"/dev/fd o /proc\": %q", caso, quien, msg)
	}
	if strings.ContainsAny(msg, "\n\r") {
		t.Fatalf("CA-445: %s: %s: las causas van en una linea, separadas por \"; \", sin saltos: %q", caso, quien, msg)
	}
	tramos := strings.Split(msg, "; ")
	desde := -1
	for i, p := range rutas {
		donde := strings.Index(msg, p)
		if donde < 0 {
			t.Fatalf("CA-445: %s: %s: el error nombra cada causa: falta la de %s (%s): %q", caso, quien, p, causas[i], msg)
		}
		if donde <= desde {
			t.Fatalf("CA-445: %s: %s: las causas van en el orden de las carpetas %q: %s aparece antes que la anterior: %q", caso, quien, rutas, p, msg)
		}
		if i > 0 && !strings.Contains(msg[desde:donde], "; ") {
			t.Fatalf("CA-445: %s: %s: la causa de %s y la de %s van separadas por \"; \": %q", caso, quien, rutas[i-1], p, msg)
		}
		desde = donde
		var tramo string
		for _, s := range tramos {
			if strings.Contains(s, p) {
				tramo = s
				break
			}
		}
		if !strings.Contains(tramo, causas[i].Error()) {
			t.Fatalf("CA-445: %s: %s: la causa de %s dice %q (su errno), no %q: %q", caso, quien, p, causas[i].Error(), tramo, msg)
		}
	}
}

// fdNoAbre exige que, con las carpetas de descriptores rutas (ninguna anda)
// en fdDirs, AbrirDirEn no devuelva root y su error sea ErrSinDescriptores
// con cada causa, que sondear diga lo mismo, y que DescriptoresDisponibles
// siga diciendo lo que dijo con las de verdad (mira una vez por proceso).
func fdNoAbre(t *testing.T, caso string, cache error, rutas []string, causas []syscall.Errno) {
	t.Helper()
	r, antes, nombre := fdEvidencia(t)
	d, err := aeAbrirDir(t, caso, r, nombre, antes)
	if d != nil || err == nil {
		var vio []string
		if d != nil {
			vio = aeNombres(t, d)
		}
		t.Fatalf("CA-445: %s: sin ninguna carpeta de descriptores que ande en %q, AbrirDirEn no abre: root=%v err=%v (lista %v)", caso, rutas, d != nil, err, vio)
	}
	if !errors.Is(err, ErrSinDescriptores) {
		t.Fatalf("CA-445: %s: sin ninguna carpeta de descriptores que ande en %q, el error de AbrirDirEn es ErrSinDescriptores, no %v", caso, rutas, err)
	}
	fdCausas(t, "AbrirDirEn", caso, err.Error(), rutas, causas)

	s := sondear()
	if !errors.Is(s, ErrSinDescriptores) {
		t.Fatalf("CA-445: %s: sin ninguna carpeta de descriptores que ande en %q, sondear es ErrSinDescriptores, no %v (un fallo de la sonda no es disponibilidad)", caso, rutas, s)
	}
	fdCausas(t, "sondear", caso, s.Error(), rutas, causas)

	if otra := DescriptoresDisponibles(); otra != cache && !errors.Is(otra, cache) {
		t.Fatalf("CA-445: %s: DescriptoresDisponibles mira una vez por proceso: con las de verdad dijo %v y ahora %v", caso, cache, otra)
	}
}

// CA-445 (hallazgos 669a42, c32559): con fdDirs = {una carpeta de
// descriptores que no sirve, la que anda}, sea como sea que no sirve (no
// existe; es un archivo regular, ENOTDIR; es un directorio sin permiso,
// EACCES; es un directorio que no es de descriptores), AbrirDirEn prueba la
// siguiente y abre el directorio mirado (el mismo, os.SameFile), y sondear
// es nil. Tambien con la que anda primero y con dos que no sirven delante.
func TestCA445_UnaCarpetaDeDescriptoresQueNoSirveNoTapaLaQueAnda(t *testing.T) {
	anda := fdLaQueAnda(t)
	t.Logf("CA-445: la que anda en este sistema: %s", anda)
	inutiles := fdInutiles(t)
	for _, u := range inutiles {
		t.Run(u.caso+" y despues la que anda", func(t *testing.T) {
			p := fdArmar(t, t.TempDir(), 0, u)
			fdUsar(t, p, anda)
			fdAbre(t, "{"+u.caso+", "+anda+"}")
		})
		t.Run("la que anda y despues una que "+u.caso, func(t *testing.T) {
			p := fdArmar(t, t.TempDir(), 1, u)
			fdUsar(t, anda, p)
			fdAbre(t, "{"+anda+", "+u.caso+"}")
		})
	}
	t.Run("dos que no sirven y despues la que anda", func(t *testing.T) {
		base := t.TempDir()
		a := fdArmar(t, base, 0, inutiles[0])
		b := fdArmar(t, base, 1, inutiles[len(inutiles)-1])
		fdUsar(t, a, b, anda)
		fdAbre(t, "{"+inutiles[0].caso+", "+inutiles[len(inutiles)-1].caso+", "+anda+"}")
	})
}

// CA-445 (hallazgos 669a42, c32559): con fdDirs = {dos carpetas de
// descriptores que no sirven}, de cualquier par de formas (en los dos
// ordenes, y la misma forma dos veces), AbrirDirEn no abre y su error es
// ErrSinDescriptores con las DOS causas (cada ruta con su errno, en orden,
// separadas por "; ", sin saltos de linea); sondear da el mismo tipo de
// error con las mismas causas; y DescriptoresDisponibles no cambia (mira
// una vez por proceso). No necesita "la que anda": corre en cualquier
// sistema.
func TestCA445_SinCarpetaDeDescriptoresQueAndeElErrorDiceCadaCausa(t *testing.T) {
	inutiles := fdInutiles(t)
	for _, u1 := range inutiles {
		for _, u2 := range inutiles {
			caso := "{" + u1.caso + ", " + u2.caso + "}"
			t.Run(caso, func(t *testing.T) {
				base := t.TempDir()
				a := fdArmar(t, base, 0, u1)
				b := fdArmar(t, base, 1, u2)
				cache := fdUsar(t, a, b)
				fdNoAbre(t, caso, cache, []string{a, b}, []syscall.Errno{u1.causa, u2.causa})
			})
		}
	}
	t.Run("una sola que no sirve", func(t *testing.T) {
		u := inutiles[0]
		a := fdArmar(t, t.TempDir(), 0, u)
		cache := fdUsar(t, a)
		fdNoAbre(t, "{"+u.caso+"}", cache, []string{a}, []syscall.Errno{u.causa})
	})
}

// CA-445 (hallazgos 669a42, c32559), como propiedad: para TODA lista de una
// a tres carpetas de descriptores tomadas de {las formas que no sirven, la
// que anda} (con repeticion, en cualquier orden), AbrirDirEn abre el
// directorio mirado y sondear es nil si y solo si la lista tiene la que
// anda, este donde este; si no la tiene, los dos dan ErrSinDescriptores con
// la causa de cada una en orden.
func TestCA445_CualquierListaDeCarpetasDeDescriptoresAbreSiYSoloSiTieneLaQueAnda(t *testing.T) {
	anda := fdLaQueAnda(t)
	inutiles := fdInutiles(t)
	// -1 es la que anda; i >= 0, inutiles[i].
	formas := []int{-1}
	for i := range inutiles {
		formas = append(formas, i)
	}
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
			if f := formas[j]; f < 0 {
				nombres = append(nombres, anda)
			} else {
				nombres = append(nombres, inutiles[f].slug)
			}
		}
		caso := "{" + strings.Join(nombres, ", ") + "}"
		t.Run(caso, func(t *testing.T) {
			base := t.TempDir()
			var rutas []string
			var causas []syscall.Errno
			tiene := false
			for i, j := range l {
				if f := formas[j]; f < 0 {
					rutas = append(rutas, anda)
					tiene = true
				} else {
					rutas = append(rutas, fdArmar(t, base, i, inutiles[f]))
					causas = append(causas, inutiles[f].causa)
				}
			}
			cache := fdUsar(t, rutas...)
			if tiene {
				fdAbre(t, caso)
				return
			}
			fdNoAbre(t, caso, cache, rutas, causas)
		})
	}
	t.Logf("CA-445: %d listas de carpetas de descriptores", len(listas))
}

// fdSenuelo arma una carpeta que parece de descriptores y no lo es: cada
// numero, de 0 a bastante mas que el descriptor mas alto abierto ahora, es
// OTRO directorio (con un archivo "senuelo" adentro). Reabrir un descriptor
// por ella abre un directorio, pero no el del descriptor.
func fdSenuelo(t *testing.T) string {
	t.Helper()
	f, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	alto := int(f.Fd())
	f.Close()
	p := filepath.Join(t.TempDir(), "senuelo-fd")
	for n := 0; n <= alto+256; n++ {
		aeEscribir(t, p, strconv.Itoa(n)+"/senuelo", []byte("no soy el directorio del descriptor\n"))
	}
	return p
}

// CA-445 (hallazgos 669a42, c32559), adversarial: una carpeta cuyo cada
// numero es OTRO directorio (reabrir por ella "anda", pero abre otra cosa)
// no es una carpeta de descriptores que ande. Con {senuelo, la que anda},
// AbrirDirEn abre el directorio mirado (os.SameFile, sin el senuelo) y
// sondear es nil; con {senuelo} y {senuelo, senuelo}, AbrirDirEn nunca
// devuelve un root de otro directorio, falla con ErrSinDescriptores, y
// sondear tambien (un directorio cualquiera no es "/" reabierto por su
// descriptor).
func TestCA445_UnaCarpetaQueAbreOtroDirectorioNoEsUnaQueAnde(t *testing.T) {
	sinNingunaQueAnde := func(t *testing.T, caso string) {
		t.Helper()
		r, antes, nombre := fdEvidencia(t)
		d, err := aeAbrirDir(t, caso, r, nombre, antes)
		if d != nil {
			fi, serr := d.Stat(".")
			t.Fatalf("CA-445: %s: AbrirDirEn nunca devuelve un root de otro directorio: devolvio uno (mismo=%v, %v) que lista %v (err %v)", caso, serr == nil && os.SameFile(fi, antes), serr, aeNombres(t, d), err)
		}
		if !errors.Is(err, ErrSinDescriptores) {
			t.Errorf("CA-445: %s: sin ninguna carpeta que reabra el descriptor, el error de AbrirDirEn es ErrSinDescriptores, no %v", caso, err)
		}
		if s := sondear(); !errors.Is(s, ErrSinDescriptores) {
			t.Errorf("CA-445: %s: una carpeta que abre otro directorio no hace disponible la sonda: sondear es ErrSinDescriptores, no %v", caso, s)
		}
	}
	t.Run("{senuelo}", func(t *testing.T) {
		fdUsar(t, fdSenuelo(t))
		sinNingunaQueAnde(t, "{senuelo}")
	})
	t.Run("{senuelo, senuelo}", func(t *testing.T) {
		fdUsar(t, fdSenuelo(t), fdSenuelo(t))
		sinNingunaQueAnde(t, "{senuelo, senuelo}")
	})
	t.Run("{senuelo, la que anda}", func(t *testing.T) {
		anda := fdLaQueAnda(t)
		fdUsar(t, fdSenuelo(t), anda)
		fdAbre(t, "{senuelo, "+anda+"}")
	})
}
