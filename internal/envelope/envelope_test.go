// Tests del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md
// (CA-202): el registro del sobre es telemetria, y la telemetria rota nunca
// rompe un comando.
package envelope

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// CA-202: Write es best-effort, List devuelve mas nuevo primero y saltea todo
// lo que no pueda leer; un proyecto sin sobres devuelve vacio, no error.
func TestCA202_ElRegistroEsTelemetria(t *testing.T) {
	root := t.TempDir()

	// un proyecto que nunca corrio un sobre: vacio, sin error y sin crear nada
	if got := List(root); len(got) != 0 {
		t.Fatalf("CA-202: sin sobres la lista viene vacia: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", DirName)); err == nil {
		t.Fatal("CA-202: leer no puede crear el directorio")
	}

	base := time.Now().UTC()
	Write(root, Record{ID: "b", Role: "writer", Provider: "claude", Stage: "run", Step: 3, Steps: 5,
		Status: StatusRunning, StartedAt: base.Add(-time.Minute)})
	Write(root, Record{ID: "c", Role: "reviewer", Provider: "codex", Stage: "ok", Steps: 5,
		Status: StatusDeliverable, StartedAt: base})
	Write(root, Record{ID: "a", Role: "scout", Provider: "claude", Stage: "spec", Steps: 5,
		Status: StatusNotDeliverable, StartedAt: base.Add(-time.Hour)})

	// basura al lado: ilegible, de otra forma y sin id
	dir := filepath.Join(root, ".hoom", DirName)
	os.WriteFile(filepath.Join(dir, "roto.json"), []byte("{no es json"), 0o644)
	os.WriteFile(filepath.Join(dir, "otro.json"), []byte(`{"id":""}`), 0o644)
	os.WriteFile(filepath.Join(dir, "nota.txt"), []byte("hola"), 0o644)

	got := List(root)
	if len(got) != 3 {
		t.Fatalf("CA-202: la basura se saltea, los tres sobres quedan: %+v", got)
	}
	if got[0].ID != "c" || got[1].ID != "b" || got[2].ID != "a" {
		t.Fatalf("CA-202: mas nuevo primero: %+v", got)
	}
	if got[0].Status != StatusDeliverable || !got[0].Done() || got[1].Done() {
		t.Fatalf("CA-202: el estado viaja tal cual: %+v", got[:2])
	}
	if got[0].UpdatedAt.IsZero() {
		t.Fatal("CA-202: Write sella cuando se escribio")
	}

	// un registro sin id no se escribe, y escribir donde no se puede no
	// entra en panico ni rompe a quien lo llamo
	Write(root, Record{Role: "writer"})
	if len(List(root)) != 3 {
		t.Fatal("CA-202: un registro sin id no se escribe")
	}
	Write(filepath.Join(root, "no", "existe", "\x00"), Record{ID: "x"})
}

// CA-202: el directorio queda escondido de Git en cuanto se escribe el primer
// registro, aunque el proyecto nunca haya tenido .hoom/.gitignore.
func TestCA202_ElDirectorioNaceEscondido(t *testing.T) {
	root := t.TempDir()
	Write(root, Record{ID: "a", Role: "writer", Status: StatusRunning})
	gi, err := os.ReadFile(filepath.Join(root, ".hoom", ".gitignore"))
	if err != nil {
		t.Fatalf("CA-202: hoom debe dejar la regla escrita: %v", err)
	}
	if string(gi) != DirName+"/\n" {
		t.Fatalf("CA-202: la regla exacta y nada mas: %q", gi)
	}

	// y no la duplica ni pisa lo que ya habia
	os.WriteFile(filepath.Join(root, ".hoom", ".gitignore"), []byte("cache/\nruns/"), 0o644)
	Write(root, Record{ID: "b", Role: "writer", Status: StatusRunning})
	Write(root, Record{ID: "c", Role: "writer", Status: StatusRunning})
	gi, _ = os.ReadFile(filepath.Join(root, ".hoom", ".gitignore"))
	if string(gi) != "cache/\nruns/\n"+DirName+"/\n" {
		t.Fatalf("CA-202: agrega la regla sin pisar ni duplicar: %q", gi)
	}
}
