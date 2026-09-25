// Tests de regresion de la review cruzada de la cabina (C1, 2026-09-24),
// hallazgo 20260924T200412_151300: `envelope.Write` tragaba el error de
// escritura, asi que nadie podia saber si el primer registro del sobre
// estaba de verdad en disco (CA-335: Started se llama con el primer registro
// ya en disco). Write sigue siendo best-effort para quien lo ignora (CA-202:
// un directorio no escribible no rompe nada, sin panic), pero ahora DICE si
// escribio: nil cuando el registro quedo, el error cuando no.
package envelope

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"testing/quick"
	"time"
)

// h3Bloquear planta un DIRECTORIO (no vacio) donde va
// .hoom/envelopes/<id>.json: ningun archivo puede quedar en esa ruta.
func h3Bloquear(t *testing.T, root, id string) string {
	t.Helper()
	p := filepath.Join(root, ".hoom", DirName, id+".json")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "ocupado"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func h3Rec(id string) Record {
	return Record{ID: id, Role: "writer", Provider: "claude", Stage: "spec", Step: 1, Steps: 5,
		Status: StatusRunning, ExitCode: -1, StartedAt: time.Now().UTC(), PID: os.Getpid()}
}

// h3Llamar corre Write y convierte un panic en un fallo con nombre: CA-202
// exige que escribir donde no se puede no entre en panico.
func h3Llamar(t *testing.T, caso, root string, rec Record) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CA-202: %s: Write entro en panico: %v", caso, r)
		}
	}()
	return Write(root, rec)
}

// Hallazgo 20260924T200412_151300 (CA-335, CA-202): con un directorio donde
// va .hoom/envelopes/<id>.json el registro no se puede escribir, y Write lo
// dice devolviendo el error (sin panic y sin tocar lo que ya estaba). Un
// registro que si se escribe devuelve nil, antes y despues del que fallo.
func TestHallazgo_151300_WriteDiceSiNoPudoEscribir(t *testing.T) {
	root := t.TempDir()

	const bueno = "20260924T200412_h3a001"
	if err := h3Llamar(t, "registro escribible", root, h3Rec(bueno)); err != nil {
		t.Fatalf("CA-335: un registro que quedo en disco devuelve nil: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", DirName, bueno+".json")); err != nil {
		t.Fatalf("CA-335: fixture: el registro escribible esta en disco: %v", err)
	}

	const tapado = "20260924T200412_h3a002"
	p := h3Bloquear(t, root, tapado)
	err := h3Llamar(t, "directorio en lugar del registro", root, h3Rec(tapado))
	if err == nil {
		t.Fatalf("CA-335: hallazgo 151300: con un directorio en %s el registro no se escribio y Write debe devolver el error, no nil", p)
	}
	if st, e := os.Stat(p); e != nil || !st.IsDir() {
		t.Fatalf("CA-202: Write no destruye lo que habia en la ruta del registro: %v", e)
	}
	for _, r := range List(root) {
		if r.ID == tapado {
			t.Fatalf("CA-202: un registro que no se escribio no aparece en List: %+v", r)
		}
	}

	// best-effort para quien lo ignora: el que falla no rompe a los demas
	actualizado := h3Rec(bueno)
	actualizado.Stage, actualizado.Step = "run", 3
	if err := h3Llamar(t, "despues de un fallo", root, actualizado); err != nil {
		t.Fatalf("CA-202: un fallo anterior no rompe el siguiente Write: %v", err)
	}
	recs := List(root)
	if len(recs) != 1 || recs[0].ID != bueno || recs[0].Stage != "run" {
		t.Fatalf("CA-202: el registro escribible sigue su vida: %+v", recs)
	}
}

// Hallazgo 20260924T200412_151300 (CA-202): los otros "no se pudo" tambien
// son un error y ninguno entra en panico: .hoom es un archivo (no se puede
// crear el directorio), .hoom/envelopes es de solo lectura, y una raiz
// imposible.
func TestHallazgo_151300_WriteDevuelveCadaImposibilidad(t *testing.T) {
	// .hoom es un archivo: .hoom/envelopes/ no puede existir
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".hoom"), []byte("no soy un directorio\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h3Llamar(t, ".hoom es un archivo", root, h3Rec("20260924T200412_h3b001")); err == nil {
		t.Fatal("CA-202: hallazgo 151300: sin donde crear .hoom/envelopes/, Write devuelve el error")
	}

	// una raiz que no puede existir
	imposible := filepath.Join(t.TempDir(), "no", "existe", "\x00")
	if err := h3Llamar(t, "raiz imposible", imposible, h3Rec("20260924T200412_h3b002")); err == nil {
		t.Fatal("CA-202: hallazgo 151300: una raiz imposible es un error, no nil")
	}

	// .hoom/envelopes de solo lectura
	if os.Geteuid() == 0 {
		t.Log("CA-202: corriendo como root los permisos no frenan la escritura: se saltea el caso de solo lectura")
		return
	}
	root = t.TempDir()
	dir := filepath.Join(root, ".hoom", DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hoom", ".gitignore"), []byte(DirName+"/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if err := h3Llamar(t, ".hoom/envelopes de solo lectura", root, h3Rec("20260924T200412_h3b003")); err == nil {
		t.Fatal("CA-202: hallazgo 151300: con .hoom/envelopes/ de solo lectura el registro no se escribe y Write lo dice")
	}
	if n := len(List(root)); n != 0 {
		t.Fatalf("CA-202: nada quedo escrito: %d registros", n)
	}
}

// Hallazgo 20260924T200412_151300 (CA-335, CA-202), propiedad: para
// cualquier registro con id, Write devuelve nil SI Y SOLO SI despues el
// registro esta en disco (List lo encuentra con su estado). Mitad de los
// casos tienen la ruta tapada por un directorio.
func TestHallazgo_151300_PropiedadNilSiYSoloSiQuedoEnDisco(t *testing.T) {
	root := t.TempDir()
	n := 0
	prop := func(tapar bool, paso uint8, nota string) bool {
		n++
		id := fmt.Sprintf("20260924T2004%02d_h3%04x", n%60, n)
		if tapar {
			h3Bloquear(t, root, id)
		}
		rec := h3Rec(id)
		rec.Step, rec.Note = int(paso%6)+1, nota
		err := h3Llamar(t, "propiedad", root, rec)
		enDisco := false
		for _, r := range List(root) {
			if r.ID == id && r.Step == rec.Step && r.Note == nota {
				enDisco = true
			}
		}
		if (err == nil) != enDisco {
			t.Logf("CA-335: id %s tapado=%v: Write devolvio %v y el registro en disco=%v", id, tapar, err, enDisco)
			return false
		}
		return true
	}
	cfg := &quick.Config{MaxCount: 40, Rand: rand.New(rand.NewSource(151300))}
	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("CA-335: hallazgo 151300: Write devuelve nil exactamente cuando el registro quedo en disco: %v", err)
	}
}
