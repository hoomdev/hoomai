// Ronda 3 del test-writer de contencion-del-writer: regresiones de la review
// delta de codex sobre las fotos de git-ajeno (agentcmd), en cualquier
// sistema. Aclaracion 30: toda foto de git-ajeno esta acotada en tiempo,
// entradas y bytes, tambien al recorrer directorios; pasar un tope da
// FotoSinTerminar, y una ref nunca queda fotografiada solo por sus metadatos
// porque se acabaron los bytes.
//
//   - d204f2 (y bea100, 6ce684): una ref de reemplazo que es un symlink a un
//     directorio enorme, o un subdirectorio enorme bajo refs/replace/;
//   - 4064a8: el presupuesto de bytes se agota antes de una ref.
package agentcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// r3Muchos crea en dir n archivos vacios con nombres largos.
func r3Muchos(t *testing.T, dir string, n int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	largo := strings.Repeat("r3", 80)
	for i := 0; i < n; i++ {
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%06d", largo, i)))
		if err != nil {
			t.Fatalf("fixture: el archivo %d de %d: %v", i, n, err)
		}
		f.Close()
	}
}

// r3BajoRef dice si alguna de xs es la ref p o algo debajo de ella.
func r3BajoRef(xs []string, p string) bool {
	for _, x := range xs {
		if x == p || strings.HasPrefix(x, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func r3Tam(t *testing.T, p string) int64 {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.Size()
}

// ---------------------------------------------------------------- d204f2

// CA-685 (id d204f2, tambien bea100 y 6ce684; aclaraciones 30 y 30b): una
// ref suelta refs/replace/<objeto> que es un symlink a un directorio externo
// con 100000 archivos vacios, y un subdirectorio real bajo refs/replace/
// igual de grande. El presupuesto de entradas de la foto es de 10000 como
// maximo: con tope de 100 ms y con tope de 10 s (ronda 3b), la foto y la
// comparacion vuelven dentro de 2 s y reservan menos de 16 MiB (TotalAlloc)
// cada una, y la comparacion cuenta como cambiado (FotoSinTerminar o la ruta
// de la ref) aunque nada haya cambiado.
func TestR3_d204f2_CA685_UnaRefDeReemplazoQueApuntaAUnDirectorioEnorme(t *testing.T) {
	const ca = "CA-685"
	const plazo = 2 * time.Second
	const limite = uint64(16) << 20
	const n = 100000
	for _, caso := range []string{"symlink a un directorio externo", "subdirectorio real bajo refs/replace"} {
		_, wt := cdwProyecto(t, "")
		comun := cdwComun(t, wt)
		var ref string
		if strings.HasPrefix(caso, "symlink") {
			a, _ := cdwObjeto(t, comun, "r3 d204f2 enorme a")
			afuera := filepath.Join(t.TempDir(), "enorme")
			r3Muchos(t, afuera, n)
			ref = r2Ref(comun, a)
			if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(afuera, ref); err != nil {
				t.Fatalf("fixture: la ref de reemplazo como symlink: %v", err)
			}
		} else {
			ref = r2Ref(comun, "r3-enorme")
			r3Muchos(t, ref, n)
		}
		for _, tope := range []time.Duration{100 * time.Millisecond, 10 * time.Second} {
			que := caso + ", tope " + tope.String()
			var antes GitAjeno
			if m := r2Asignado(func() {
				r1Plazo(t, ca, que+": FotoGitAjenoConTope", plazo, func() {
					antes = FotoGitAjenoConTope(wt, tope, MaxLeidoGitAjeno)
				})
			}); m >= limite {
				t.Fatalf("%s: %s: la foto no lee entero el directorio: reservo %d MiB (quiero menos de %d)", ca, que, m>>20, limite>>20)
			}
			var got []string
			if m := r2Asignado(func() {
				r1Plazo(t, ca, que+": CambiosGitAjenoConTope", plazo, func() {
					got = CambiosGitAjenoConTope(antes, tope, MaxLeidoGitAjeno)
				})
			}); m >= limite {
				t.Fatalf("%s: %s: la comparacion no lee entero el directorio: reservo %d MiB (quiero menos de %d)", ca, que, m>>20, limite>>20)
			}
			if !r1Tiene(got, FotoSinTerminar) && !r3BajoRef(got, ref) {
				t.Fatalf("%s: %s: %d entradas pasan el presupuesto de entradas: cuenta como cambiado (%s o %s) aunque nada cambie: %q", ca, que, n, FotoSinTerminar, ref, got)
			}
		}
	}
}

// CA-685 (id d204f2; aclaracion 30), control: un symlink de
// refs/replace/<objeto> a un directorio chico, sin cambios, no da cambios; si
// se agrega dentro un archivo con un SHA valido, cuenta como git-ajeno (la
// ruta de la ref o una debajo de ella).
func TestR3_d204f2_CA685_UnSymlinkAUnDirectorioChico(t *testing.T) {
	const ca = "CA-685"
	_, wt := cdwProyecto(t, "")
	comun := cdwComun(t, wt)
	a, _ := cdwObjeto(t, comun, "r3 d204f2 chico a")
	b, _ := cdwObjeto(t, comun, "r3 d204f2 chico b")
	c, _ := cdwObjeto(t, comun, "r3 d204f2 chico c")
	d, _ := cdwObjeto(t, comun, "r3 d204f2 chico d")
	chico := filepath.Join(t.TempDir(), "chico")
	cdwEscribir(t, filepath.Join(chico, b), c+"\n")
	ref := r2Ref(comun, a)
	if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(chico, ref); err != nil {
		t.Fatalf("fixture: la ref de reemplazo como symlink: %v", err)
	}
	antes := FotoGitAjeno(wt)
	if got := CambiosGitAjeno(antes); got != nil {
		t.Fatalf("%s: control: un symlink a un directorio chico, sin cambios, no da cambios: %q", ca, got)
	}
	time.Sleep(20 * time.Millisecond)
	cdwEscribir(t, filepath.Join(chico, d), b+"\n")
	if got := CambiosGitAjeno(antes); !r3BajoRef(got, ref) {
		t.Fatalf("%s: un archivo con un SHA valido agregado al directorio chico cuenta como git-ajeno (%s o debajo): %q", ca, ref, got)
	}
}

// ---------------------------------------------------------------- 4064a8

// CA-685 (id 4064a8; aclaracion 30): un packed-refs valido y grande (2000
// refs) y una ref de reemplazo suelta de mas de 4 KiB (un SHA valido y 8000
// espacios). Con un maxLeido mayor que el packed-refs solo pero menor que la
// suma de los dos, el presupuesto se agota antes de poder hashear una ref:
// aunque nada cambie, la comparacion da FotoSinTerminar, nunca vacio.
// Control: con un maxLeido que alcanza para los dos, sin cambios, nil.
func TestR3_4064a8_CA685_ElPresupuestoDeBytesSeAgotaAntesDeUnaRef(t *testing.T) {
	const ca = "CA-685"
	_, wt := cdwProyecto(t, "")
	comun := cdwComun(t, wt)
	head := cdwGitOut(t, wt, "rev-parse", "HEAD")
	a, _ := cdwObjeto(t, comun, "r3 4064a8 a")
	b, _ := cdwObjeto(t, comun, "r3 4064a8 b")
	var sb strings.Builder
	sb.WriteString("# pack-refs with: peeled fully-peeled sorted \n")
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&sb, "%s refs/heads/r3-%05d\n", head, i)
	}
	packed := filepath.Join(comun, "packed-refs")
	cdwEscribir(t, packed, sb.String())
	ref := r2Ref(comun, a)
	cdwEscribir(t, ref, b+strings.Repeat(" ", 8000))
	p, l := r3Tam(t, packed), r3Tam(t, ref)
	chico := p + l/2
	if l <= 4096 || chico <= p || chico >= p+l {
		t.Fatalf("fixture: la ref suelta pasa 4 KiB (%d) y maxLeido (%d) queda entre el packed-refs (%d) y la suma (%d)", l, chico, p, p+l)
	}

	// control: alcanza para los dos
	antes := FotoGitAjenoConTope(wt, TopeFotoGitAjeno, MaxLeidoGitAjeno)
	if got := CambiosGitAjenoConTope(antes, TopeFotoGitAjeno, MaxLeidoGitAjeno); got != nil {
		t.Fatalf("%s: control: con un maxLeido que alcanza para los dos, sin cambios, nil: %q", ca, got)
	}
	antes = FotoGitAjenoConTope(wt, TopeFotoGitAjeno, chico)
	if got := CambiosGitAjenoConTope(antes, TopeFotoGitAjeno, chico); !r1Tiene(got, FotoSinTerminar) {
		t.Fatalf("%s: con maxLeido %d (el packed-refs tiene %d y la ref suelta %d) la foto queda %s aunque nada cambie: %q", ca, chico, p, l, FotoSinTerminar, got)
	}
}
