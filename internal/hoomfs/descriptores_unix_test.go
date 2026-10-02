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
// Hallazgo 25f656 de la sexta ronda: una carpeta por la que reabrir un
// descriptor ABRE un directorio (os.OpenRoot anda) que despues no se deja
// mirar (Stat(".") falla) tiene su propia causa, "<esa ruta>: <el error de
// Stat>"; "no reabre el mismo directorio" es solo de una que abre OTRO
// directorio (el senuelo). Cada causa nombra su ruta.
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
	"regexp"
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
// errno) que tiene que dar reabrir un descriptor por ella. tramo, si no es
// nil, dice ademas que tiene que decir su tramo del error (ver fdTramo).
type fdInutil struct {
	caso  string
	slug  string
	causa syscall.Errno
	armar func(t *testing.T, p string)
	tramo fdTramo
}

// fdTramo revisa el tramo seg del error que corresponde a la carpeta de
// descriptores p: "" si dice lo que tiene que decir, si no que le falta.
type fdTramo func(p, seg string) string

// fdOtroDirectorio es lo que dice la causa de una carpeta que reabre un
// descriptor como OTRO directorio (hallazgo 25f656: solo esa).
const fdOtroDirectorio = "no reabre el mismo directorio"

// fdNoSeMira dice como falla, en este sistema, mirar (Stat(".")) un
// directorio de modo 0400 (se lee, no se busca) abierto como os.Root: el
// error y su errno; o nil y por que esa forma no se puede armar aca (root
// entra igual, el sistema de archivos lo deja mirar, o ni se abre).
func fdNoSeMira(t *testing.T) (error, syscall.Errno, string) {
	t.Helper()
	d := filepath.Join(t.TempDir(), "sin-busqueda")
	if err := os.Mkdir(d, 0o700); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := os.Chmod(d, 0o400); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	t.Cleanup(func() { os.Chmod(d, 0o700) })
	r, err := os.OpenRoot(d)
	if err != nil {
		return nil, 0, fmt.Sprintf("un directorio de modo 0400 no se abre como os.Root: %v", err)
	}
	defer r.Close()
	_, err = r.Stat(".")
	if err == nil {
		return nil, 0, "un directorio de modo 0400 abierto como os.Root se mira igual (root, o el sistema de archivos no pide permiso de busqueda)"
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return nil, 0, fmt.Sprintf("mirar un directorio de modo 0400 falla sin errno: %v", err)
	}
	return err, errno, ""
}

// fdArmarNoSeMira arma en p una carpeta cuyo cada numero, de 0 a bastante
// mas que el descriptor mas alto abierto ahora, es un directorio de modo
// 0400: reabrir un descriptor por ella abre un directorio (os.OpenRoot anda,
// alcanza con permiso de lectura) que despues no se deja mirar (Stat(".")
// necesita permiso de busqueda).
func fdArmarNoSeMira(t *testing.T, p string) {
	t.Helper()
	f, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	alto := int(f.Fd())
	f.Close()
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var hechos []string
	t.Cleanup(func() {
		for _, d := range hechos {
			os.Chmod(d, 0o700)
		}
	})
	for n := 0; n <= alto+256; n++ {
		d := filepath.Join(p, strconv.Itoa(n))
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatalf("fixture: %v", err)
		}
		hechos = append(hechos, d)
		if err := os.Chmod(d, 0o400); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
}

// fdTramoNoSeMira: el tramo de una carpeta que abre y no se mira es
// "<carpeta>/<descriptor>: <el error de Stat>" (statErr), y no dice
// fdOtroDirectorio.
func fdTramoNoSeMira(statErr error) fdTramo {
	return func(p, seg string) string {
		re := regexp.MustCompile(regexp.QuoteMeta(p) + `/[0-9]+: ` + regexp.QuoteMeta(statErr.Error()))
		if !re.MatchString(seg) {
			return fmt.Sprintf("una carpeta que abre un directorio que no se deja mirar da como causa la ruta que abrio y el error de Stat, \"%s/<descriptor>: %s\"", p, statErr)
		}
		if strings.Contains(seg, fdOtroDirectorio) {
			return fmt.Sprintf("una carpeta que abre un directorio que no se deja mirar no dice %q: eso es de una que abre OTRO directorio", fdOtroDirectorio)
		}
		return ""
	}
}

// fdTramoOtroDirectorio: el tramo de una carpeta que reabre el descriptor
// como otro directorio (el senuelo) nombra la ruta que abrio,
// "<carpeta>/<descriptor>", y dice fdOtroDirectorio.
func fdTramoOtroDirectorio(p, seg string) string {
	if !regexp.MustCompile(regexp.QuoteMeta(p) + `/[0-9]+`).MatchString(seg) {
		return fmt.Sprintf("la causa nombra la ruta que se abrio, \"%s/<descriptor>\"", p)
	}
	if !strings.Contains(seg, fdOtroDirectorio) {
		return fmt.Sprintf("una carpeta que reabre el descriptor como OTRO directorio dice %q", fdOtroDirectorio)
	}
	return ""
}

// fdInutiles son las formas de una carpeta de descriptores que no sirve. Las
// de EACCES (modo 000; numeros que abren y no se miran) se omiten como root
// (root entra igual); la de numeros que abren y no se miran, tambien en un
// sistema de archivos que deja mirar un directorio sin permiso de busqueda.
// Esa se arma UNA vez por test (cientos de directorios) y cada entrada es un
// symlink a ella, como /dev/fd en linux lo es a /proc/self/fd: la ruta que
// se reabre es la de la entrada. Con directorios de verdad, en
// TestCA445_UnaCarpetaQueAbreUnDirectorioQueNoSeMiraDiceSuCausa.
func fdInutiles(t *testing.T) []fdInutil {
	t.Helper()
	noSeMira := fdInutil{"es una carpeta cuyos numeros abren un directorio que no se deja mirar (modo 0400)", "no-se-mira", 0, nil, nil}
	if statErr, errno, porQueNo := fdNoSeMira(t); statErr != nil {
		compartida := filepath.Join(t.TempDir(), "no-se-mira")
		fdArmarNoSeMira(t, compartida)
		noSeMira.causa = errno
		noSeMira.armar = func(t *testing.T, p string) {
			t.Helper()
			if err := os.Symlink(compartida, p); err != nil {
				t.Fatalf("fixture: %v", err)
			}
		}
		noSeMira.tramo = fdTramoNoSeMira(statErr)
	} else {
		t.Logf("CA-445 (25f656): se omite %q: %s", noSeMira.caso, porQueNo)
	}
	todas := []fdInutil{
		{"no existe", "no-existe", syscall.ENOENT, func(t *testing.T, p string) {}, nil},
		{"es un archivo regular", "archivo", syscall.ENOTDIR, func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte("no soy una carpeta de descriptores\n"), 0o644); err != nil {
				t.Fatalf("fixture: %v", err)
			}
		}, nil},
		{"es un directorio sin permiso (modo 000)", "sin-permiso", syscall.EACCES, func(t *testing.T, p string) {
			if err := os.Mkdir(p, 0o700); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			if err := os.Chmod(p, 0); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			t.Cleanup(func() { os.Chmod(p, 0o700) })
		}, nil},
		{"es un directorio que no es de descriptores", "no-es-de-descriptores", syscall.ENOENT, func(t *testing.T, p string) {
			aeEscribir(t, p, "LEEME", []byte("aca no hay descriptores\n"))
			aeEscribir(t, p, "fd/LEEME", []byte("tampoco\n"))
		}, nil},
	}
	if noSeMira.armar != nil {
		// Antes de la ultima: la primera y la ultima siguen siendo las de
		// "dos que no sirven y despues la que anda".
		ultima := todas[len(todas)-1]
		todas = append(todas[:len(todas)-1], noSeMira, ultima)
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

// causasEnOrden exige que msg, el texto de un error, diga que hoom necesita
// "/dev/fd o /proc", vaya en una linea, y lleve la causa de cada ruta
// probada: cada una de rutas en su orden, con su errno en su tramo
// (causas[i] 0: no se pide errno), separadas por "; "; y, si tramos[i] no es
// nil, que el tramo de rutas[i] diga lo que pide tramos[i] (hallazgo
// 25f656). Que error es (ErrSinDescriptores, ErrSinSonda) lo mira quien
// llama. Sin rutas, mira solo lo primero: "/dev/fd o /proc" y una linea.
func causasEnOrden(t *testing.T, quien, caso, msg string, rutas []string, causas []syscall.Errno, tramos []fdTramo) {
	t.Helper()
	if !strings.Contains(msg, "/dev/fd o /proc") {
		t.Fatalf("CA-445: %s: %s: el error dice que hoom necesita \"/dev/fd o /proc\": %q", caso, quien, msg)
	}
	if strings.ContainsAny(msg, "\n\r") {
		t.Fatalf("CA-445: %s: %s: las causas van en una linea, separadas por \"; \", sin saltos: %q", caso, quien, msg)
	}
	segs := strings.Split(msg, "; ")
	desde := -1
	for i, p := range rutas {
		donde := strings.Index(msg, p)
		if donde < 0 {
			t.Fatalf("CA-445: %s: %s: el error nombra cada causa: falta la de %s (%s): %q", caso, quien, p, causas[i], msg)
		}
		if donde <= desde {
			t.Fatalf("CA-445: %s: %s: las causas van en el orden de las rutas probadas %q: %s aparece antes que la anterior: %q", caso, quien, rutas, p, msg)
		}
		if i > 0 && !strings.Contains(msg[desde:donde], "; ") {
			t.Fatalf("CA-445: %s: %s: la causa de %s y la de %s van separadas por \"; \": %q", caso, quien, rutas[i-1], p, msg)
		}
		desde = donde
		var tramo string
		for _, s := range segs {
			if strings.Contains(s, p) {
				tramo = s
				break
			}
		}
		if causas[i] != 0 && !strings.Contains(tramo, causas[i].Error()) {
			t.Fatalf("CA-445: %s: %s: la causa de %s dice %q (su errno), no %q: %q", caso, quien, p, causas[i].Error(), tramo, msg)
		}
		if i < len(tramos) && tramos[i] != nil {
			if falta := tramos[i](p, tramo); falta != "" {
				t.Fatalf("CA-445 (25f656): %s: %s: la causa de %s: %s; dice %q: %q", caso, quien, p, falta, tramo, msg)
			}
		}
	}
}

// fdNoAbre exige que, con las carpetas de descriptores rutas (ninguna anda)
// en fdDirs, AbrirDirEn no devuelva root y su error sea ErrSinDescriptores
// con cada causa, que sondear diga lo mismo, y que DescriptoresDisponibles
// siga diciendo lo que dijo con las de verdad (mira una vez por proceso).
// tramos es lo que pide causasEnOrden de cada tramo (nil: solo el errno).
func fdNoAbre(t *testing.T, caso string, cache error, rutas []string, causas []syscall.Errno, tramos []fdTramo) {
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
	causasEnOrden(t, "AbrirDirEn", caso, err.Error(), rutas, causas, tramos)

	s := sondear()
	if !errors.Is(s, ErrSinDescriptores) {
		t.Fatalf("CA-445: %s: sin ninguna carpeta de descriptores que ande en %q, sondear es ErrSinDescriptores, no %v (un fallo de la sonda no es disponibilidad)", caso, rutas, s)
	}
	if errors.Is(s, ErrSinSonda) {
		t.Fatalf("CA-445 (2d68e4): %s: con un directorio de dirsSonda que se abre (%q), sondear miro: es ErrSinDescriptores, no ErrSinSonda: %v", caso, dirsSonda, s)
	}
	causasEnOrden(t, "sondear", caso, s.Error(), rutas, causas, tramos)

	if otra := DescriptoresDisponibles(); otra != cache && !errors.Is(otra, cache) {
		t.Fatalf("CA-445: %s: DescriptoresDisponibles mira una vez por proceso: con las de verdad dijo %v y ahora %v", caso, cache, otra)
	}
}

// CA-445 (hallazgos 669a42, c32559; 25f656): con fdDirs = {una carpeta de
// descriptores que no sirve, la que anda}, sea como sea que no sirve (no
// existe; es un archivo regular, ENOTDIR; es un directorio sin permiso,
// EACCES; sus numeros abren un directorio que no se deja mirar; es un
// directorio que no es de descriptores), AbrirDirEn prueba la siguiente y
// abre el directorio mirado (el mismo, os.SameFile), y sondear es nil.
// Tambien con la que anda primero y con dos que no sirven delante.
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

// CA-445 (hallazgos 669a42, c32559; 25f656): con fdDirs = {dos carpetas de
// descriptores que no sirven}, de cualquier par de formas (en los dos
// ordenes, y la misma forma dos veces), AbrirDirEn no abre y su error es
// ErrSinDescriptores con las DOS causas (cada ruta con su errno, en orden,
// separadas por "; ", sin saltos de linea; la de una que abre y no se mira,
// "<ruta>/<descriptor>: <error de Stat>"); sondear da el mismo tipo de
// error con las mismas causas (y no ErrSinSonda: "/" se abrio, miro); y
// DescriptoresDisponibles no cambia (mira una vez por proceso). No necesita
// "la que anda": corre en cualquier sistema.
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
				fdNoAbre(t, caso, cache, []string{a, b}, []syscall.Errno{u1.causa, u2.causa}, []fdTramo{u1.tramo, u2.tramo})
			})
		}
	}
	t.Run("una sola que no sirve", func(t *testing.T) {
		u := inutiles[0]
		a := fdArmar(t, t.TempDir(), 0, u)
		cache := fdUsar(t, a)
		fdNoAbre(t, "{"+u.caso+"}", cache, []string{a}, []syscall.Errno{u.causa}, []fdTramo{u.tramo})
	})
}

// CA-445 (hallazgos 669a42, c32559; 25f656), como propiedad: para TODA
// lista de una a tres carpetas de descriptores tomadas de {las formas que no
// sirven (tambien la que abre y no se mira), la que anda} (con repeticion,
// en cualquier orden), AbrirDirEn abre el directorio mirado y sondear es nil
// si y solo si la lista tiene la que anda, este donde este; si no la tiene,
// los dos dan ErrSinDescriptores con la causa de cada una en orden.
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
			var tramos []fdTramo
			tiene := false
			for i, j := range l {
				if f := formas[j]; f < 0 {
					rutas = append(rutas, anda)
					tiene = true
				} else {
					rutas = append(rutas, fdArmar(t, base, i, inutiles[f]))
					causas = append(causas, inutiles[f].causa)
					tramos = append(tramos, inutiles[f].tramo)
				}
			}
			cache := fdUsar(t, rutas...)
			if tiene {
				fdAbre(t, caso)
				return
			}
			fdNoAbre(t, caso, cache, rutas, causas, tramos)
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

// CA-445 (hallazgo 25f656): una carpeta de descriptores por la que reabrir
// un descriptor ABRE un directorio (os.OpenRoot anda) que despues no se deja
// mirar (Stat(".") falla: cada numero es un directorio de modo 0400) no es
// una que ande, y su causa es la de verdad, "<carpeta>/<descriptor>: <el
// error de Stat>", nunca "no reabre el mismo directorio": eso es solo de
// una que abre OTRO directorio (el senuelo), que lo sigue diciendo. Cada
// causa nombra su ruta, en el orden de fdDirs, separadas por "; ", en
// AbrirDirEn y en sondear (que no es ErrSinSonda: miro). Con {esa, la que
// anda}, AbrirDirEn abre el directorio mirado y sondear es nil.
func TestCA445_UnaCarpetaQueAbreUnDirectorioQueNoSeMiraDiceSuCausa(t *testing.T) {
	statErr, errno, porQueNo := fdNoSeMira(t)
	if statErr == nil {
		t.Skipf("CA-445 (25f656): no se puede armar una carpeta cuyos numeros abren y no se miran: %s", porQueNo)
	}
	t.Logf("CA-445 (25f656): mirar un directorio de modo 0400 abierto como os.Root da %q", statErr)
	noSeMira := fdTramoNoSeMira(statErr)
	armarNoSeMira := func(t *testing.T, base string, i int) string {
		t.Helper()
		p := filepath.Join(base, "entrada"+strconv.Itoa(i)+"-no-se-mira")
		fdArmarNoSeMira(t, p)
		return p
	}
	noExiste := func(base string, i int) string {
		return filepath.Join(base, "entrada"+strconv.Itoa(i)+"-no-existe")
	}

	t.Run("{no se mira}", func(t *testing.T) {
		p := armarNoSeMira(t, t.TempDir(), 0)
		cache := fdUsar(t, p)
		fdNoAbre(t, "{no se mira}", cache, []string{p}, []syscall.Errno{errno}, []fdTramo{noSeMira})
	})
	t.Run("{no se mira, no existe}", func(t *testing.T) {
		base := t.TempDir()
		p, q := armarNoSeMira(t, base, 0), noExiste(base, 1)
		cache := fdUsar(t, p, q)
		fdNoAbre(t, "{no se mira, no existe}", cache, []string{p, q}, []syscall.Errno{errno, syscall.ENOENT}, []fdTramo{noSeMira, nil})
	})
	t.Run("{no existe, no se mira}", func(t *testing.T) {
		base := t.TempDir()
		q, p := noExiste(base, 0), armarNoSeMira(t, base, 1)
		cache := fdUsar(t, q, p)
		fdNoAbre(t, "{no existe, no se mira}", cache, []string{q, p}, []syscall.Errno{syscall.ENOENT, errno}, []fdTramo{nil, noSeMira})
	})
	t.Run("{senuelo}", func(t *testing.T) {
		s := fdSenuelo(t)
		cache := fdUsar(t, s)
		fdNoAbre(t, "{senuelo}", cache, []string{s}, []syscall.Errno{0}, []fdTramo{fdTramoOtroDirectorio})
	})
	t.Run("{senuelo, no se mira}", func(t *testing.T) {
		s, p := fdSenuelo(t), armarNoSeMira(t, t.TempDir(), 1)
		cache := fdUsar(t, s, p)
		fdNoAbre(t, "{senuelo, no se mira}", cache, []string{s, p}, []syscall.Errno{0, errno}, []fdTramo{fdTramoOtroDirectorio, noSeMira})
	})
	t.Run("{no se mira, senuelo}", func(t *testing.T) {
		p, s := armarNoSeMira(t, t.TempDir(), 0), fdSenuelo(t)
		cache := fdUsar(t, p, s)
		fdNoAbre(t, "{no se mira, senuelo}", cache, []string{p, s}, []syscall.Errno{errno, 0}, []fdTramo{noSeMira, fdTramoOtroDirectorio})
	})
	t.Run("{no se mira, la que anda}", func(t *testing.T) {
		anda := fdLaQueAnda(t)
		p := armarNoSeMira(t, t.TempDir(), 0)
		fdUsar(t, p, anda)
		fdAbre(t, "{no se mira, "+anda+"}")
	})
}
