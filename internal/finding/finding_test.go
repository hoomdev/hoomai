// Tests adversariales del spec .hoom/specs/hallazgos-y-refutador.md
// (CA-53..CA-56, CA-60): hallazgo inmutable, cierre solo con evidencia,
// transicion terminal unica, y la narracion jamas toca la huella.
package finding

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "rev@hoom.dev")
	git(t, root, "config", "user.name", "Reviewer")
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	return root
}

// CA-53: add crea el registro con todos los campos y la huella del arbol;
// resolver NO modifica el archivo del hallazgo (inmutable).
func TestCA53_HallazgoInmutable(t *testing.T) {
	root := initRepo(t)
	f, err := Add(root, "main", "high", "reliability", "app.go", "el retry no respeta el backoff", "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Severity != "high" || f.Lens != "reliability" || f.File != "app.go" || f.ID == "" {
		t.Fatalf("CA-53: campos incompletos: %+v", f)
	}
	if f.Fingerprint == "" || f.Fingerprint != gitx.Snapshot(root, "main").ChangeFingerprint {
		t.Fatalf("CA-53: la huella del arbol debe congelarse en el hallazgo: %+v", f)
	}
	if !strings.Contains(f.Author, "Reviewer") {
		t.Fatalf("CA-53: el autor sale del git config si no se declara: %q", f.Author)
	}

	path := filepath.Join(root, ".hoom", "findings", f.ID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, f.ID, "refutado", "el test TestRetry cubre el backoff y pasa", ""); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("CA-53: resolver modifico el archivo del hallazgo (debe ser INMUTABLE)")
	}
}

// CA-54: cerrar exige --as valido y evidencia no vacia; sin evidencia no
// se escribe NADA.
func TestCA54_EvidenciaObligatoria(t *testing.T) {
	root := initRepo(t)
	f, err := Add(root, "main", "low", "readability", "", "nombre confuso en helper", "")
	if err != nil {
		t.Fatal(err)
	}
	casos := []struct{ as, ev string }{
		{"refutado", ""},
		{"refutado", "   "},
		{"cerrado", "evidencia valida"}, // estado invalido
	}
	for _, c := range casos {
		if _, err := Resolve(root, f.ID, c.as, c.ev, ""); err == nil {
			t.Fatalf("CA-54: resolve con as=%q evidence=%q debia negarse", c.as, c.ev)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", "findings", f.ID+".res.json")); err == nil {
		t.Fatal("CA-54: un resolve rechazado no puede haber escrito la transicion")
	}
}

// CA-55: list deriva el estado del par hallazgo+transicion y marca la
// huella que ya no coincide.
func TestCA55_EstadosDerivadosYHuella(t *testing.T) {
	root := initRepo(t)
	abierto, _ := Add(root, "main", "medium", "risk", "app.go", "input sin sanitizar", "")
	resuelto, _ := Add(root, "main", "low", "readability", "", "typo en comentario", "")
	if _, err := Resolve(root, resuelto.ID, "corregido", "commit abc123 con gate verde", ""); err != nil {
		t.Fatal(err)
	}

	items, warnings, err := List(root, "main", false)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	if byID[abierto.ID].Status != StatusOpen || byID[resuelto.ID].Status != StatusCorrected {
		t.Fatalf("CA-55: estados mal derivados: %+v", byID)
	}
	if byID[abierto.ID].CodeChanged {
		t.Fatal("CA-55: sin cambios en el arbol no hay marca de codigo cambiado")
	}

	// cambia el arbol => los hallazgos quedan marcados "a re-verificar"
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app // editado\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	items, _, _ = List(root, "main", false)
	for _, it := range items {
		if !it.CodeChanged {
			t.Fatalf("CA-55: tras editar el arbol, %s debia marcar codigo cambiado", it.ID)
		}
	}

	// --open filtra los resueltos
	open, _, _ := List(root, "main", true)
	if len(open) != 1 || open[0].ID != abierto.ID {
		t.Fatalf("CA-55: --open debe dejar solo los abiertos: %+v", open)
	}
}

// CA-56: un hallazgo resuelto no admite segunda transicion; la original
// queda intacta.
func TestCA56_TransicionTerminalUnica(t *testing.T) {
	root := initRepo(t)
	f, _ := Add(root, "main", "high", "risk", "app.go", "posible inyeccion", "")
	if _, err := Resolve(root, f.ID, "refutado", "el input viene de constante, no de usuario", ""); err != nil {
		t.Fatal(err)
	}
	resPath := filepath.Join(root, ".hoom", "findings", f.ID+".res.json")
	before, _ := os.ReadFile(resPath)

	_, err := Resolve(root, f.ID, "corregido", "intento de pisar la historia", "")
	if err == nil || !strings.Contains(err.Error(), "ya esta resuelto") {
		t.Fatalf("CA-56: la segunda transicion debia fallar nombrando el estado: %v", err)
	}
	after, _ := os.ReadFile(resPath)
	if string(before) != string(after) {
		t.Fatal("CA-56: la transicion original fue modificada")
	}
}

// CA-60: registrar y resolver hallazgos NO cambia la huella del candidato:
// un check verde sigue verde.
func TestCA60_HallazgosFueraDeLaHuella(t *testing.T) {
	root := initRepo(t)
	v := &verdict.Verdict{
		Project: "demo",
		Git:     gitx.Snapshot(root, "main"),
		Gates:   []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusPass}},
	}
	v.Finalize()
	if _, err := verdict.Write(root, v); err != nil {
		t.Fatal(err)
	}
	if res, _ := checkcmd.Run(root, "main"); !res.OK {
		t.Fatalf("fixture: el check debia arrancar verde: %+v", res)
	}

	f, err := Add(root, "main", "medium", "reliability", "app.go", "hallazgo de prueba", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, f.ID, "refutado", "no reproduce con el test X", ""); err != nil {
		t.Fatal(err)
	}
	if res, _ := checkcmd.Run(root, "main"); !res.OK {
		t.Fatalf("CA-60: registrar/resolver hallazgos rompio un check verde: %+v", res)
	}
}

// CA-55 (borde): un JSON corrupto se omite con advertencia, jamas tumba
// el listado.
func TestCA55_CorruptoNoTumba(t *testing.T) {
	root := initRepo(t)
	Add(root, "main", "low", "readability", "", "hallazgo valido", "")
	if err := os.WriteFile(filepath.Join(root, ".hoom", "findings", "zzz.json"), []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	items, warnings, err := List(root, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(warnings) != 1 {
		t.Fatalf("CA-55: el valido se lista y el corrupto se advierte: items=%d warnings=%v", len(items), warnings)
	}
	raw, _ := json.Marshal(warnings)
	if !strings.Contains(string(raw), "zzz") {
		t.Fatalf("CA-55: la advertencia debe nombrar el archivo: %v", warnings)
	}
}
