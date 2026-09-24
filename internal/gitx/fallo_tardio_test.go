// Tests del hallazgo 20260924T145643_57c18a sobre
// .hoom/specs/tablero-de-solo-lectura.md (CA-315) y el contrato de
// gitx.BranchDiff: cuando git falla, Available es false, la nota trae las
// palabras de git ("git diff <base>...HEAD fallo: <stderr de git>") y el
// error es nil. Eso vale TAMBIEN cuando git ya emitio mas de maxBytes de
// parche antes de fallar (un fallo tardio): un parche cortado nunca tapa la
// salida no-cero de git. Lo que no cambia: un parche grande que git entrega
// entero y sin error sale truncado, disponible, sin nota, de a lo sumo
// maxBytes y cortado en fin de linea.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ftMax es el tope del parche del tablero (CA-315: 256 KiB).
const ftMax = 256 << 10

// ftCaso arma un repo con una rama "tarea" que agrega, en este orden de
// ruta: .gitattributes, a_big.txt (~625 KiB de lineas, si grande) y el
// archivo z_* con un driver de diff configurado en el repo. Segun el valor
// de la config, git diff sano o git diff que muere con 128 al llegar al
// archivo z_*, despues de haber emitido todo lo anterior.
type ftCaso struct {
	grande  bool
	archivo string // el archivo con el atributo diff=<driver>
	driver  string
	clave   string // config del driver: textconv o xfuncname
	valor   string
}

var (
	// B: textconv que sale no-cero -> "fatal: unable to read files to diff"
	ftTextconvRoto = ftCaso{grande: true, archivo: "z_conv.txt", driver: "roto", clave: "textconv", valor: "false"}
	// C: xfuncname invalido -> "fatal: Invalid regexp to look for hunk header: (["
	ftXfuncnameRoto = ftCaso{grande: true, archivo: "z_re.txt", driver: "malo", clave: "xfuncname", valor: "(["}
	// D: el mismo textconv roto, sin el archivo grande delante (falla antes del corte)
	ftTextconvRotoChico = ftCaso{grande: false, archivo: "z_conv.txt", driver: "roto", clave: "textconv", valor: "false"}
	// sano: el gemelo de B con un textconv que funciona
	ftTextconvSano = ftCaso{grande: true, archivo: "z_conv.txt", driver: "roto", clave: "textconv", valor: "cat"}
)

const ftLineasGrandes = 10000 // 64 bytes cada una: 640000 bytes

func ftRepo(t *testing.T, c ftCaso) string {
	t.Helper()
	idAislarGit(t) // sin la config global ni del sistema del que corre el test
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dfGit(t, dir, "init", "-b", "main")
	dfGit(t, dir, "config", "user.email", "test@hoom.dev")
	dfGit(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "base.txt", "base\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "inicial")
	dfGit(t, dir, "checkout", "-q", "-b", "tarea")
	if c.grande {
		var b strings.Builder
		b.Grow(ftLineasGrandes * 64)
		for i := 0; i < ftLineasGrandes; i++ {
			fmt.Fprintf(&b, "linea %08d %s\n", i, strings.Repeat("g", 48))
		}
		write(t, dir, "a_big.txt", b.String())
	}
	dfGit(t, dir, "config", "diff."+c.driver+"."+c.clave, c.valor)
	write(t, dir, ".gitattributes", c.archivo+" diff="+c.driver+"\n")
	write(t, dir, c.archivo, "uno\ndos\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "cambio de la tarea")
	return dir
}

// ftGitDiff corre `git diff --no-color --no-ext-diff <base>...HEAD --` en
// dir y devuelve stdout, stderr y el codigo de salida, sin juzgarlos.
func ftGitDiff(t *testing.T, dir, base string, extra ...string) (string, string, int) {
	t.Helper()
	args := append([]string{"diff", "--no-color", "--no-ext-diff"}, extra...)
	args = append(args, base+"...HEAD", "--")
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("fixture: no se pudo correr git diff: %v", err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

// ftFixtureFalloTardio confirma que el git de esta maquina reproduce el
// fallo tardio: numstat sale 0, y el parche sale no-cero despues de haber
// escrito mas de ftMax bytes. Devuelve el stderr de git (sus palabras).
func ftFixtureFalloTardio(t *testing.T, dir string) string {
	t.Helper()
	if _, errNum, code := ftGitDiff(t, dir, "main", "--numstat"); code != 0 {
		t.Fatalf("fixture: git diff --numstat main...HEAD sale 0 en el fallo tardio, salio %d: %s", code, errNum)
	}
	out, stderr, code := ftGitDiff(t, dir, "main")
	if code == 0 || !strings.Contains(stderr, "fatal:") {
		t.Fatalf("fixture: git diff main...HEAD falla con fatal: exit=%d stderr=%q", code, stderr)
	}
	if len(out) <= ftMax {
		t.Fatalf("fixture: git emite mas de %d bytes de parche antes de fallar, emitio %d", ftMax, len(out))
	}
	return strings.TrimSpace(stderr)
}

// ftAssertFallo: el contrato de "git fallo" de BranchDiff.
func ftAssertFallo(t *testing.T, d Diff, err error, fatal string) {
	t.Helper()
	if err != nil {
		t.Fatalf("57c18a CA-315: si git falla el error es nil (lo dice la nota): %v", err)
	}
	if d.Available || d.Truncated {
		t.Fatalf("57c18a CA-315: git salio no-cero: available=false y truncated=false aunque ya hubiera emitido mas de maxBytes; fue available=%v truncated=%v len(patch)=%d note=%q",
			d.Available, d.Truncated, len(d.Patch), d.Note)
	}
	if d.Patch != "" {
		t.Fatalf("57c18a CA-315: si git falla no hay parche (ni cortado): len=%d", len(d.Patch))
	}
	if !strings.HasPrefix(d.Note, "git diff main...HEAD fallo:") {
		t.Fatalf("57c18a CA-315: la nota empieza con %q, fue %q", "git diff main...HEAD fallo:", d.Note)
	}
	if !strings.Contains(d.Note, fatal) {
		t.Fatalf("57c18a CA-315: la nota trae las palabras de git %q, fue %q", fatal, d.Note)
	}
}

// Hallazgo 20260924T145643_57c18a, caso B. CA-315: a_big.txt (~625 KiB) y
// despues z_conv.txt con un textconv que sale no-cero: git ya emitio mas de
// 256 KiB de parche cuando muere con "fatal: unable to read files to diff".
// BranchDiff con maxBytes 256 KiB no puede devolver ese parche cortado como
// si fuera un diff disponible: available false, sin parche, nota con git.
func TestHallazgo_57c18a_FalloTardioTextconvNoSeTapaConElCorte(t *testing.T) {
	dir := ftRepo(t, ftTextconvRoto)
	fatal := ftFixtureFalloTardio(t, dir)
	if !strings.Contains(fatal, "unable to read files to diff") {
		t.Logf("git de esta maquina dice otra cosa para el textconv roto: %q", fatal)
	}
	d, err := BranchDiff(dir, "main", ftMax)
	ftAssertFallo(t, d, err, fatal)
}

// Hallazgo 20260924T145643_57c18a, caso C. CA-315: a_big.txt (~625 KiB) y
// despues z_re.txt con un xfuncname invalido: git muere con "fatal: Invalid
// regexp to look for hunk header: ([" despues de haber emitido mas de 256
// KiB. Mismo contrato que el caso B.
func TestHallazgo_57c18a_FalloTardioXfuncnameNoSeTapaConElCorte(t *testing.T) {
	dir := ftRepo(t, ftXfuncnameRoto)
	fatal := ftFixtureFalloTardio(t, dir)
	if !strings.Contains(fatal, "Invalid regexp to look for hunk header") {
		t.Logf("git de esta maquina dice otra cosa para el xfuncname invalido: %q", fatal)
	}
	d, err := BranchDiff(dir, "main", ftMax)
	ftAssertFallo(t, d, err, fatal)
}

// Hallazgo 20260924T145643_57c18a, casos B y C con otros topes. CA-315: el
// fallo tardio no depende de donde cae el corte: con cualquier maxBytes por
// debajo de lo que git emitio antes de morir, el resultado es el de "git
// fallo". Sin tope (0) git entrega todo y el fallo se ve (guarda).
func TestHallazgo_57c18a_FalloTardioConCualquierTope(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		c      ftCaso
	}{{"textconv", ftTextconvRoto}, {"xfuncname", ftXfuncnameRoto}} {
		dir := ftRepo(t, caso.c)
		fatal := ftFixtureFalloTardio(t, dir)
		for _, max := range []int{4096, 64 << 10, 512 << 10, 0} {
			t.Run(fmt.Sprintf("%s/max=%d", caso.nombre, max), func(t *testing.T) {
				d, err := BranchDiff(dir, "main", max)
				ftAssertFallo(t, d, err, fatal)
			})
		}
	}
}

// Guarda del hallazgo 20260924T145643_57c18a, caso D. CA-315: el mismo
// textconv roto, sin el archivo grande delante: git muere antes de llegar a
// maxBytes y BranchDiff ya da available false con la nota de git.
func TestHallazgo_57c18a_GuardaFalloTempranoYaEsNoDisponible(t *testing.T) {
	dir := ftRepo(t, ftTextconvRotoChico)
	out, stderr, code := ftGitDiff(t, dir, "main")
	if code == 0 || !strings.Contains(stderr, "fatal:") || len(out) >= ftMax {
		t.Fatalf("fixture: git diff falla antes del corte: exit=%d len=%d stderr=%q", code, len(out), stderr)
	}
	d, err := BranchDiff(dir, "main", ftMax)
	ftAssertFallo(t, d, err, strings.TrimSpace(stderr))
}

// Guarda del hallazgo 20260924T145643_57c18a. CA-315: el gemelo sano de B
// (mismo archivo grande y mismo driver, con un textconv que funciona): git
// entrega el parche entero con exit 0, y BranchDiff lo corta como siempre:
// available, sin nota, truncated, <= maxBytes, en fin de linea y prefijo
// exacto de git diff main...HEAD.
func TestHallazgo_57c18a_GuardaDiffGrandeSanoSigueTruncadoYDisponible(t *testing.T) {
	dir := ftRepo(t, ftTextconvSano)
	crudo, stderr, code := ftGitDiff(t, dir, "main")
	if code != 0 || len(crudo) <= ftMax {
		t.Fatalf("fixture: el gemelo sano sale 0 con mas de %d bytes: exit=%d len=%d stderr=%q", ftMax, code, len(crudo), stderr)
	}
	d, err := BranchDiff(dir, "main", ftMax)
	if err != nil {
		t.Fatalf("57c18a CA-315: un diff sano no da error: %v", err)
	}
	if !d.Available || d.Note != "" || !d.Truncated {
		t.Fatalf("57c18a CA-315: un diff grande sano sale disponible, sin nota y truncado: available=%v note=%q truncated=%v",
			d.Available, d.Note, d.Truncated)
	}
	if len(d.Patch) > ftMax || !strings.HasSuffix(d.Patch, "\n") {
		t.Fatalf("57c18a CA-315: el parche cortado no pasa maxBytes y termina en fin de linea: len=%d", len(d.Patch))
	}
	if want := crudo[:strings.LastIndexByte(crudo[:ftMax], '\n')+1]; d.Patch != want {
		t.Fatalf("57c18a CA-315: el corte es el ultimo fin de linea antes de maxBytes de git diff main...HEAD: len %d, quiere %d", len(d.Patch), len(want))
	}
	porRuta := map[string]DiffFile{}
	for _, f := range d.Files {
		porRuta[f.Path] = f
	}
	if f, ok := porRuta["a_big.txt"]; !ok || f.Insertions != ftLineasGrandes {
		t.Fatalf("57c18a CA-315: el numstat va entero aunque el parche se corte: %+v", d.Files)
	}
}
