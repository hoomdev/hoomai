//go:build !windows

// Tests de regresion del hallazgo 20261020T202026_c6ddec (medium,
// reliability) de la review de los fixes de C1
// (.hoom/specs/items-y-columna-derivada.md, CA-293: la review limpia deja
// rastro, append-only).
//
// El registro de review se publica ENTERO o no se publica. Si escribirlo
// falla en cualquier punto (a la mitad o en el ultimo byte), no queda en
// .hoom/reviews/ ningun archivo con el nombre final ni un parcial, Records no
// lo ve (ni registro ni aviso), y Run informa no-entregable sin record_id.
// Hoy WriteRecord abre el nombre final y no lo retira si la escritura falla:
// con el fallo en el ultimo byte queda un JSON completo que Records acepta
// (el tablero ve la review cumplida mientras Run dijo no-entregable), y con
// el fallo a la mitad queda un parcial que Records avisa para siempre.
//
// El fallo se provoca de forma deterministica como lo hizo el refutador: un
// subproceso (este binario de test re-ejecutado) baja su RLIMIT_FSIZE y
// ignora SIGXFSZ, asi la escritura que pasa del limite devuelve EFBIG.
// Providers falsos en el PATH: ningun CLI de IA real.
package reviewcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	c1bEnvModo   = "HOOM_C1B_C6DDEC_MODO"   // "registro" | "run"
	c1bEnvDir    = "HOOM_C1B_C6DDEC_DIR"    // donde escribe WriteRecord, o el root de Run
	c1bEnvLimite = "HOOM_C1B_C6DDEC_LIMITE" // RLIMIT_FSIZE del subproceso, en bytes
)

// c1bSalida es lo que el subproceso informa por stdout (un pipe: el limite
// de tamano no lo alcanza).
type c1bSalida struct {
	Err      string `json:"err"`
	RecordID string `json:"record_id"`
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
	Passes   int    `json:"passes"`
	Cross    string `json:"cross"`
	Out      string `json:"out"`
}

// c1bRegistro es el registro que escriben los tests de WriteRecord: id y
// created_at fijos, asi su tamano en disco es el mismo en cada escritura.
func c1bRegistro() Record {
	var hallazgos, notas []string
	for i := 0; i < 40; i++ {
		hallazgos = append(hallazgos, fmt.Sprintf("20261020T2020%02d_c6d%03d", i%60, i))
		notas = append(notas, fmt.Sprintf("el hallazgo %d no trae la tarea de la review", i))
	}
	return Record{
		ID:        "20261020T202026_c6d001",
		CreatedAt: time.Date(2026, 10, 20, 20, 20, 26, 0, time.UTC),
		Task:      "precios", Spec: ".hoom/specs/precios.md", Fingerprint: strings.Repeat("ab", 32),
		VerdictID: "20261020T201000_v00001", Verdict: "green", Lenses: Lentes,
		Provider: "codex", Writer: "claude", Cross: CrossYes, WritersDeclared: []string{"claude"},
		Findings: hallazgos, Notes: notas, Model: "gpt-x", Effort: "high",
		Desde: strings.Repeat("1", 40), Hasta: strings.Repeat("2", 40), Cobertura: CoberturaCompleta, Base: "main",
	}
}

// TestHallazgoC6ddecSubproceso no es un test: es el subproceso con el limite
// de tamano de archivo (hallazgo 20261020T202026_c6ddec, CA-293). Sin la
// variable de entorno no hace nada.
func TestHallazgoC6ddecSubproceso(t *testing.T) {
	modo := os.Getenv(c1bEnvModo)
	if modo == "" {
		return
	}
	dir := os.Getenv(c1bEnvDir)
	limite, err := strconv.ParseUint(os.Getenv(c1bEnvLimite), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "subproceso: limite: %v\n", err)
		os.Exit(2)
	}
	signal.Ignore(syscall.SIGXFSZ)
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		fmt.Fprintf(os.Stderr, "subproceso: getrlimit: %v\n", err)
		os.Exit(2)
	}
	lim.Cur = limite
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		fmt.Fprintf(os.Stderr, "subproceso: setrlimit: %v\n", err)
		os.Exit(2)
	}
	var s c1bSalida
	switch modo {
	case "registro":
		r, err := WriteRecord(dir, c1bRegistro())
		if err != nil {
			s.Err = err.Error()
		}
		s.RecordID = r.ID
	case "run":
		var out bytes.Buffer
		res, err := Run(dir, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "claude"}, &out)
		if err != nil {
			s.Err = err.Error()
		}
		s.RecordID, s.Status, s.ExitCode, s.Passes, s.Cross, s.Out = res.RecordID, res.Status, res.ExitCode, len(res.Passes), res.Cross, out.String()
	default:
		fmt.Fprintf(os.Stderr, "subproceso: modo desconocido %q\n", modo)
		os.Exit(2)
	}
	raw, _ := json.Marshal(s)
	os.Stdout.Write(append([]byte("C1B-SALIDA "), append(raw, '\n')...))
	os.Exit(0)
}

// c1bSubproceso corre el subproceso con el limite y devuelve lo que informo.
func c1bSubproceso(t *testing.T, modo, dir string, limite int64) c1bSalida {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHallazgoC6ddecSubproceso$", "-test.count=1", "-test.timeout=240s")
	cmd.Env = append(os.Environ(), c1bEnvModo+"="+modo, c1bEnvDir+"="+dir, c1bEnvLimite+"="+strconv.FormatInt(limite, 10))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("el subproceso fallo: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	for _, l := range strings.Split(stdout.String(), "\n") {
		if rest, ok := strings.CutPrefix(l, "C1B-SALIDA "); ok {
			var s c1bSalida
			if err := json.Unmarshal([]byte(rest), &s); err != nil {
				t.Fatalf("la salida del subproceso no es JSON: %v\n%s", err, rest)
			}
			return s
		}
	}
	t.Fatalf("el subproceso no informo nada:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	return c1bSalida{}
}

// c1bEnReviews lista TODO lo que hay en dir/.hoom/reviews/ (ocultos
// incluidos), con su tamano.
func c1bEnReviews(t *testing.T, dir string) []string {
	t.Helper()
	entradas, err := os.ReadDir(filepath.Join(dir, ".hoom", RecordsDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []string
	for _, e := range entradas {
		tam := int64(-1)
		if info, err := e.Info(); err == nil {
			tam = info.Size()
		}
		out = append(out, fmt.Sprintf("%s (%d bytes)", e.Name(), tam))
	}
	return out
}

// c1bTamano escribe el registro sin limite en un directorio aparte y devuelve
// el tamano con que WriteRecord lo publica.
func c1bTamano(t *testing.T) int64 {
	t.Helper()
	dir := t.TempDir()
	r, err := WriteRecord(dir, c1bRegistro())
	if err != nil {
		t.Fatalf("CA-293: fixture: sin limite WriteRecord publica el registro: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, ".hoom", RecordsDir, r.ID+".json"))
	if err != nil {
		t.Fatalf("CA-293: fixture: el registro esta en .hoom/reviews/<id>.json: %v", err)
	}
	if recs, warns := Records(dir); len(recs) != 1 || len(warns) != 0 {
		t.Fatalf("CA-293: fixture: Records lee el registro escrito sin limite: %d registros, avisos %q", len(recs), warns)
	}
	if info.Size() < 1024 {
		t.Fatalf("fixture: el registro de prueba tiene que ser grande para fallar a la mitad: %d bytes", info.Size())
	}
	return info.Size()
}

// Hallazgo 20261020T202026_c6ddec (CA-293): si la escritura del registro
// falla a la mitad o en el ultimo byte, WriteRecord devuelve el error y en
// .hoom/reviews/ no queda nada: ni el nombre final ni un parcial ni un
// temporal. Records no ve ningun registro ni avisa de ninguno, y el mismo
// registro se puede publicar despues entero (el id no quedo tomado).
func TestHallazgoC6ddec_WriteRecordQueFallaNoDejaNada(t *testing.T) {
	n := c1bTamano(t)
	for _, c := range []struct {
		nombre string
		limite int64
	}{
		{"falla a la mitad", n / 2},
		{"falla en el ultimo byte", n - 1},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			dir := t.TempDir()
			s := c1bSubproceso(t, "registro", dir, c.limite)
			if s.Err == "" {
				t.Fatalf("CA-293: fixture: con RLIMIT_FSIZE=%d (< %d bytes del registro) WriteRecord tiene que fallar", c.limite, n)
			}
			if got := c1bEnReviews(t, dir); len(got) != 0 {
				t.Errorf("CA-293: hallazgo c6ddec: %s (%v), y en .hoom/reviews/ quedo %v: el registro se publica entero o no se publica, sin el nombre final ni un parcial",
					c.nombre, s.Err, got)
			}
			recs, warns := Records(dir)
			if len(recs) != 0 || len(warns) != 0 {
				t.Errorf("CA-293: hallazgo c6ddec: %s, Records no ve un registro que WriteRecord dijo no escribir: %d registros, avisos %q",
					c.nombre, len(recs), warns)
			}
			if t.Failed() {
				return
			}
			r, err := WriteRecord(dir, c1bRegistro())
			if err != nil {
				t.Fatalf("CA-293: hallazgo c6ddec: tras el fallo, el mismo registro se publica entero sin limite: %v", err)
			}
			if recs, warns := Records(dir); len(recs) != 1 || recs[0].ID != r.ID || len(warns) != 0 {
				t.Fatalf("CA-293: hallazgo c6ddec: tras publicarlo, Records ve un registro y ningun aviso: %+v %q", recs, warns)
			}
		})
	}
}

// GUARDA (verde en la base). Hallazgo 20261020T202026_c6ddec (CA-293): con
// el limite exactamente igual al tamano del registro la escritura entra y
// el registro queda publicado entero, como siempre.
func TestHallazgoC6ddec_WriteRecordJustoEnElLimitePublica(t *testing.T) {
	n := c1bTamano(t)
	dir := t.TempDir()
	s := c1bSubproceso(t, "registro", dir, n)
	if s.Err != "" {
		t.Fatalf("CA-293: con RLIMIT_FSIZE igual al tamano (%d) el registro entra: %s", n, s.Err)
	}
	recs, warns := Records(dir)
	if len(recs) != 1 || recs[0].ID != s.RecordID || len(warns) != 0 {
		t.Fatalf("CA-293: el registro quedo publicado y Records lo lee entero: %+v %q (en disco %v)", recs, warns, c1bEnReviews(t, dir))
	}
	if len(recs[0].Findings) != 40 || len(recs[0].Notes) != 40 {
		t.Fatalf("CA-293: Records devuelve el registro entero: %d hallazgos, %d notas", len(recs[0].Findings), len(recs[0].Notes))
	}
}

// c1bProyecto arma la review que llega al final con un registro grande: la
// tarea precios con un cambio de codigo, el writer observado en codex y un
// item con muchas sesiones declaradas (ninguna de claude, que revisa). Las
// sesiones solo engordan writers_declared del registro: el resto de lo que
// escribe Run (sobres, sidecars, el log del run) queda chico.
func c1bProyecto(t *testing.T) (root, wt string) {
	t.Helper()
	root = cbRepo(t)
	wt = cbTarea(t, root, "precios")
	cbMeta(t, root, wt, "20261020T190000_c6dw01", "codex", "writer")
	var declarados []string
	for i := 0; i < 2000; i++ {
		declarados = append(declarados, fmt.Sprintf("declarado-%04d-%s", i, strings.Repeat("x", 90)))
	}
	cbItem(t, root, "precios", declarados...)
	return root, wt
}

// Hallazgo 20261020T202026_c6ddec (CA-293): una review que llega al final
// pero cuya escritura del registro falla a la mitad termina no-entregable sin
// record_id, y en .hoom/reviews/ del arbol revisado no queda ni el nombre
// final ni un parcial: Records no ve nada, asi el tablero no la cuenta.
func TestHallazgoC6ddec_RunConRegistroQueFallaNoDejaNada(t *testing.T) {
	const limite = 64 << 10
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")

	// control: sin limite la misma review termina revisado con un registro
	// mucho mas grande que el limite (lo engorda writers_declared)
	root, wt := c1bProyecto(t)
	var out bytes.Buffer
	res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "claude"}, &out)
	if err != nil || res.Status != "revisado" || res.RecordID == "" {
		t.Fatalf("CA-293: fixture: sin limite la review termina revisado con registro: %+v %v\n%s", res, err, out.String())
	}
	info, err := os.Stat(filepath.Join(wt, ".hoom", RecordsDir, res.RecordID+".json"))
	if err != nil || info.Size() < 3*limite {
		t.Fatalf("fixture: el registro de control pasa holgado el limite de %d bytes: %v %v", limite, info, err)
	}

	root, wt = c1bProyecto(t)
	s := c1bSubproceso(t, "run", root, limite)
	if s.Err != "" {
		t.Fatalf("CA-293: no poder escribir el registro es un resultado de la review, no un armado roto: %s\n%s", s.Err, s.Out)
	}
	if s.Passes != 1 || s.Cross != CrossYes {
		t.Fatalf("CA-293: fixture: la review llego al final con su pasada cruzada (las demas escrituras entran en el limite): %+v", s)
	}
	if s.Status != "no-entregable" || s.ExitCode != 1 || s.RecordID != "" {
		t.Errorf("CA-293: hallazgo c6ddec: si el registro no se pudo escribir la review es no-entregable, exit 1 y sin record_id: status %q exit %d record_id %q\n%s",
			s.Status, s.ExitCode, s.RecordID, s.Out)
	}
	if got := c1bEnReviews(t, wt); len(got) != 0 {
		t.Errorf("CA-293: hallazgo c6ddec: Run dijo %q y en .hoom/reviews/ del arbol revisado quedo %v: ni el nombre final ni un parcial", s.Status, got)
	}
	if recs, warns := Records(wt); len(recs) != 0 || len(warns) != 0 {
		t.Errorf("CA-293: hallazgo c6ddec: el tablero no ve una review que Run dio por no-entregable: %d registros, avisos %q", len(recs), warns)
	}
}
