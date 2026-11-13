// Ronda 2 del test-writer de contencion-del-writer: regresiones de la review
// delta de codex sobre las fotos de git-ajeno (agentcmd), en cualquier
// sistema. Aclaracion 28: una foto nunca junta en un mismo valor dos estados
// distintos; lo que no puede leer, o lo que pasa su tope, falla cerrado.
//
//   - 61babd: una ref de reemplazo suelta que es un symlink;
//   - a48bb4: una ref de reemplazo suelta de mas de 4096 bytes;
//   - d91180: un packed-refs enorme y disperso, con tope chico;
//   - 59fa6c (e2e): con el job de Windows creado, el sobre de un provider
//     sin estado sigue en ninguno.
package agentcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
)

// r2Ref es la ruta de la ref suelta de reemplazo del objeto sha.
func r2Ref(comun, sha string) string { return filepath.Join(comun, "refs", "replace", sha) }

// ---------------------------------------------------------------- 61babd

// CA-685 (id 61babd; aclaracion 28): una ref suelta refs/replace/<objeto>
// que es un symlink a un archivo de afuera del directorio comun: cambiar lo
// que dice el destino (otro SHA valido, mismo tamano) o cambiar el destino
// del symlink cuenta como git-ajeno, con la ruta de la ref. Control: si nada
// cambia, no hay cambios.
func TestR2_61babd_CA685_UnaRefDeReemplazoQueEsUnSymlink(t *testing.T) {
	const ca = "CA-685"
	for _, caso := range []string{"cambia lo que dice el destino", "cambia el destino del symlink", "control: nada cambia"} {
		_, wt := cdwProyecto(t, "")
		comun := cdwComun(t, wt)
		a, _ := cdwObjeto(t, comun, "r2 61babd a: "+caso)
		b, _ := cdwObjeto(t, comun, "r2 61babd b: "+caso)
		c, _ := cdwObjeto(t, comun, "r2 61babd c: "+caso)
		afuera := t.TempDir()
		destino := filepath.Join(afuera, "destino")
		cdwEscribir(t, destino, b+"\n")
		ref := r2Ref(comun, a)
		if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(destino, ref); err != nil {
			t.Fatalf("fixture: la ref de reemplazo como symlink: %v", err)
		}
		antes := FotoGitAjeno(wt)
		time.Sleep(20 * time.Millisecond)
		switch caso {
		case "cambia lo que dice el destino":
			cdwEscribir(t, destino, c+"\n")
		case "cambia el destino del symlink":
			otro := filepath.Join(afuera, "otro")
			cdwEscribir(t, otro, c+"\n")
			if err := os.Remove(ref); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(otro, ref); err != nil {
				t.Fatal(err)
			}
		}
		got := CambiosGitAjeno(antes)
		if strings.HasPrefix(caso, "control") {
			if got != nil {
				t.Fatalf("%s: %s: sin cambios, nil: %q", ca, caso, got)
			}
			continue
		}
		if !r1Tiene(got, ref) {
			t.Fatalf("%s: %s: cuenta como git-ajeno con la ruta de la ref %s: %q", ca, caso, ref, got)
		}
	}
}

// ---------------------------------------------------------------- a48bb4

// CA-685 (id a48bb4; aclaracion 28): una ref suelta de reemplazo de mas de
// 4096 bytes (un SHA valido y despues mas de 4 KiB de espacios o de saltos
// de linea) reescrita con otro SHA valido, igual de grande, cuenta como
// git-ajeno con su ruta. Control: si nada cambia, no hay cambios.
func TestR2_a48bb4_CA685_UnaRefDeReemplazoDeMasDe4KiB(t *testing.T) {
	const ca = "CA-685"
	for _, relleno := range []string{" ", "\n"} {
		for _, cambia := range []bool{true, false} {
			caso := "relleno " + strings.ReplaceAll(relleno, "\n", `\n`)
			if !cambia {
				caso += " (control: nada cambia)"
			}
			_, wt := cdwProyecto(t, "")
			comun := cdwComun(t, wt)
			a, _ := cdwObjeto(t, comun, "r2 a48bb4 a: "+caso)
			b, _ := cdwObjeto(t, comun, "r2 a48bb4 b: "+caso)
			c, _ := cdwObjeto(t, comun, "r2 a48bb4 c: "+caso)
			cola := strings.Repeat(relleno, 5000)
			ref := r2Ref(comun, a)
			cdwEscribir(t, ref, b+cola)
			if st, err := os.Stat(ref); err != nil || st.Size() <= 4096 {
				t.Fatalf("fixture: la ref pasa los 4096 bytes: %v %v", st, err)
			}
			antes := FotoGitAjeno(wt)
			time.Sleep(20 * time.Millisecond)
			if cambia {
				cdwEscribir(t, ref, c+cola)
			}
			got := CambiosGitAjeno(antes)
			if !cambia {
				if got != nil {
					t.Fatalf("%s: %s: sin cambios, nil: %q", ca, caso, got)
				}
				continue
			}
			if !r1Tiene(got, ref) {
				t.Fatalf("%s: %s: reescrita con otro SHA e igual de grande, cuenta como git-ajeno con su ruta %s: %q", ca, caso, ref, got)
			}
		}
	}
}

// ---------------------------------------------------------------- d91180

// CA-685 (id d91180; aclaraciones 27 y 28): un packed-refs disperso de
// 64 GiB (Truncate: no ocupa disco). La foto y la comparacion con tope de
// 200 ms vuelven dentro del tope mas un margen (nunca lo leen entero); si
// despues de la foto crece (una ref de reemplazo al final) o cambia (una en
// el medio, mismo tamano), la comparacion cuenta como cambiado: la ruta del
// packed-refs, la de la ref o FotoSinTerminar.
func TestR2_d91180_CA685_UnPackedRefsEnormeYDispersoNoSeLeeEntero(t *testing.T) {
	const ca = "CA-685"
	const tope, plazo = 200 * time.Millisecond, 3 * time.Second
	const tam = int64(64) << 30
	for _, modo := range []string{"crece", "cambia"} {
		_, wt := cdwProyecto(t, "")
		comun := cdwComun(t, wt)
		a, _ := cdwObjeto(t, comun, "r2 d91180 a: "+modo)
		b, _ := cdwObjeto(t, comun, "r2 d91180 b: "+modo)
		packed := filepath.Join(comun, "packed-refs")
		r1Disperso(t, packed, tam)
		var antes GitAjeno
		r1Plazo(t, ca, modo+": FotoGitAjenoConTope con un packed-refs disperso de 64 GiB", plazo, func() {
			antes = FotoGitAjenoConTope(wt, tope, MaxLeidoGitAjeno)
		})
		time.Sleep(20 * time.Millisecond)
		linea := b + " refs/replace/" + a + "\n"
		switch modo {
		case "crece":
			r1Agregar(t, packed, linea)
		case "cambia":
			f, err := os.OpenFile(packed, os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteAt([]byte("\n"+linea), tam/2); err != nil {
				f.Close()
				t.Fatal(err)
			}
			f.Close()
		}
		var got []string
		r1Plazo(t, ca, modo+": CambiosGitAjenoConTope con un packed-refs disperso de 64 GiB", plazo, func() {
			got = CambiosGitAjenoConTope(antes, tope, MaxLeidoGitAjeno)
		})
		if !(r1Tiene(got, packed) || r1Tiene(got, r2Ref(comun, a)) || r1Tiene(got, FotoSinTerminar)) {
			t.Fatalf("%s: %s: el packed-refs que %s despues de la foto cuenta como cambiado (%s, %s o %s): %q",
				ca, modo, modo, packed, r2Ref(comun, a), FotoSinTerminar, got)
		}
	}
}

// r2Asignado son los bytes que el proceso reservo (TotalAlloc) mientras
// corria f.
func r2Asignado(f func()) uint64 {
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	f()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

// CA-685 (id d91180, ronda 2b; aclaracion 27): un packed-refs disperso de
// 512 MiB, por debajo de maxLeido (una cabecera valida al principio, un
// hueco, y dos refs al final), se lee de a poco: con maxLeido mayor que el
// archivo y un tope holgado (30 s), ni la foto ni la comparacion reservan el
// archivo entero (TotalAlloc sube menos de 64 MiB en cada una). Control: la
// comparacion detecta igual una ref de reemplazo agregada al final, con su
// ruta o la del packed-refs.
func TestR2_d91180_CA685_UnPackedRefsDeBajoDeMaxLeidoNoSeCargaEntero(t *testing.T) {
	const ca = "CA-685"
	const tope = 30 * time.Second
	const tam = int64(512) << 20
	const limite = uint64(64) << 20
	if tam > MaxLeidoGitAjeno {
		t.Fatalf("%s: fixture: el packed-refs (%d) queda por debajo de MaxLeidoGitAjeno (%d)", ca, tam, MaxLeidoGitAjeno)
	}
	_, wt := cdwProyecto(t, "")
	comun := cdwComun(t, wt)
	a, _ := cdwObjeto(t, comun, "r2 d91180 2b a")
	b, _ := cdwObjeto(t, comun, "r2 d91180 2b b")
	c, _ := cdwObjeto(t, comun, "r2 d91180 2b c")
	d, _ := cdwObjeto(t, comun, "r2 d91180 2b d")
	packed := filepath.Join(comun, "packed-refs")
	cola := "\n" + c + " refs/heads/r2-d91180\n" + d + " refs/replace/" + b + "\n"
	f, err := os.Create(packed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("# pack-refs with: peeled fully-peeled sorted \n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Truncate(tam); err != nil {
		f.Close()
		t.Fatalf("fixture: un packed-refs disperso de %d bytes: %v", tam, err)
	}
	if _, err := f.WriteAt([]byte(cola), tam-int64(len(cola))); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	var antes GitAjeno
	if n := r2Asignado(func() { antes = FotoGitAjenoConTope(wt, tope, MaxLeidoGitAjeno) }); n >= limite {
		t.Fatalf("%s: la foto lee el packed-refs de %d MiB de a poco: reservo %d MiB (quiero menos de %d)", ca, tam>>20, n>>20, limite>>20)
	}
	time.Sleep(20 * time.Millisecond)
	r1Agregar(t, packed, c+" refs/replace/"+a+"\n")
	var got []string
	if n := r2Asignado(func() { got = CambiosGitAjenoConTope(antes, tope, MaxLeidoGitAjeno) }); n >= limite {
		t.Fatalf("%s: la comparacion lee el packed-refs de %d MiB de a poco: reservo %d MiB (quiero menos de %d)", ca, tam>>20, n>>20, limite>>20)
	}
	if !r1Tiene(got, r2Ref(comun, a)) && !r1Tiene(got, packed) {
		t.Fatalf("%s: control: la ref de reemplazo agregada al final se ve, con su ruta (%s) o la del packed-refs: %q", ca, r2Ref(comun, a), got)
	}
}

// ---------------------------------------------------------------- 59fa6c

// CA-674 y CA-677 (id 59fa6c), e2e: con Windows simulado y el job creado
// (ReemplazarAdopcion con "windows" y un job que si se crea), el sobre de un
// provider falso sin ConEstado, con exigir no, corre y deja ninguno con su
// motivo (MotivoSinEstado) en el registro del sobre y en el sidecar del run:
// el job no le sube el nivel.
func TestR2_59fa6c_CA674_CA677_ConElJobElSobreSinEstadoSigueEnNinguno(t *testing.T) {
	const ca = "CA-677"
	restaurar := encierro.ReemplazarAdopcion("windows", func(int) bool { return true })
	defer restaurar()
	root, wt := cdwProyecto(t, cdwSeccion)
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{Provider: cdwSinEstadoNombre})
	if res.RunID == "" {
		t.Fatalf("%s: con exigir no, el provider sin ConEstado corre: %+v\n%s", ca, res, out)
	}
	want := encierro.Info{Nivel: encierro.NivelNinguno, Falta: []string{encierro.FaltaProcesos, encierro.FaltaEscrituras, encierro.FaltaIPC},
		Motivo: encierro.MotivoSinEstado(cdwSinEstadoNombre)}
	got, _ := cdwClave(t, root, res.EnvelopeID)
	cdwExigeInfo(t, "CA-674", "el registro del sobre con el job creado", got, want)
	meta, _ := cdwMetaCruda(t, res.RunID, root, wt)["encierro"].(map[string]any)
	cdwExigeInfo(t, ca, "el sidecar del run con el job creado", meta, want)
}
