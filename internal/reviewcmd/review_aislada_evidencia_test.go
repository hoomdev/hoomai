// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (enmienda 1: CA-401, CA-402, CA-412..CA-414): hoom congela la evidencia (el
// cambio entero contra el merge-base, con borrados y renombres, + el spec)
// una vez, leyendo con tope; se niega a correr si pasa el tope; el spec tiene
// que ser un archivo regular del arbol; un error de git falla cerrado. Los
// fixtures estan en review_aislada_helpers_test.go.
package reviewcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// ---------------------------------------------------------------- CA-401

// CA-401: Evidence arma el cambio entero contra el merge-base y el arbol de
// trabajo: rastreado commiteado o no, no rastreado como archivo nuevo
// (tambien con nombre no ASCII), binario sin contenido, nada de .hoom/; mas
// el spec; Bytes y SHA256 cierran.
func TestCA401_EvidenciaDelCandidato(t *testing.T) {
	root := raRepo(t, "")
	write(t, root, "committed.go", "package app\n\nvar Base = 1\n")
	write(t, root, "imagen.bin", "\x00\x01MARCA-BINARIA-BASE\x00")
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\nversion base\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "base")
	git(t, root, "checkout", "-q", "-b", "feature")
	write(t, root, "committed.go", "package app\n\nvar Base = 2 // commiteado en la rama\n")
	git(t, root, "commit", "-q", "-am", "rama")
	// sin commitear, rastreado
	write(t, root, "app.go", "package app\n\nfunc SinCommitear() {}\n")
	// no rastreados
	write(t, root, "nuevo.go", "package app\n\n// archivo nuevo sin rastrear ñandú\nfunc Nuevo() {}\n")
	write(t, root, "dir con espacio/ñu.go", "package dir\n\n// contenido-unico-ñu\n")
	write(t, root, "nuevo.bin", "\x00MARCA-BINARIA-NUEVO-ARCHIVO\x00\xff")
	// binario rastreado y modificado
	write(t, root, "imagen.bin", "\x00\x02MARCA-BINARIA-NUEVA\x00\xff")
	// lo de .hoom/ no va nunca, ni el spec cambiado ni un archivo suelto
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\nversion rama: SPEC-NO-VA-EN-EL-DIFF\n")
	write(t, root, ".hoom/notas.txt", "NOTA-HOOM-FUERA-DEL-DIFF\n")

	g := gitx.Snapshot(root, "main")
	for _, f := range []string{"committed.go", "app.go", "nuevo.go", "imagen.bin", ".hoom/specs/x.md"} {
		if !raTiene(g.ChangedFiles, f) {
			t.Fatalf("CA-401: fixture: %s es parte del cambio: %v", f, g.ChangedFiles)
		}
	}
	ev, err := Evidence(root, "main", ".hoom/specs/x.md", raTopeGrande)
	if err != nil {
		t.Fatalf("CA-401: Evidence: %v", err)
	}
	if ev.Over {
		t.Fatalf("CA-401: una evidencia chica no pasa el tope: %+v", ev.Bytes)
	}
	diff := string(ev.Diff)
	if !raTieneNombre(diff, "dir con espacio/ñu.go") {
		t.Fatalf("CA-401: el no rastreado con nombre no ASCII va con su nombre (crudo o citado por git):\n%s", diff)
	}
	for _, quiero := range []string{
		"diff --git a/committed.go b/committed.go", "-var Base = 1", "+var Base = 2 // commiteado en la rama",
		"diff --git a/app.go b/app.go", "+func SinCommitear() {}",
		"+++ b/nuevo.go", "+// archivo nuevo sin rastrear ñandú",
		"+// contenido-unico-ñu",
		"imagen.bin", "nuevo.bin", "Binary files",
	} {
		if !strings.Contains(diff, quiero) {
			t.Fatalf("CA-401: el diff del candidato trae %q:\n%s", quiero, diff)
		}
	}
	if !strings.Contains(diff, "new file mode") {
		t.Fatalf("CA-401: un no rastreado va como parche de archivo nuevo:\n%s", diff)
	}
	for _, nunca := range []string{"MARCA-BINARIA", "GIT binary patch", "SPEC-NO-VA-EN-EL-DIFF", "NOTA-HOOM-FUERA-DEL-DIFF", "a/.hoom/", "b/.hoom/"} {
		if strings.Contains(diff, nunca) {
			t.Fatalf("CA-401: el diff no trae %q (binarios sin contenido, nada de .hoom/):\n%s", nunca, diff)
		}
	}
	spec, _ := os.ReadFile(filepath.Join(root, ".hoom", "specs", "x.md"))
	if !bytes.Equal(ev.Spec, spec) {
		t.Fatalf("CA-401: Spec es el texto del spec tal cual esta en el arbol: %q", ev.Spec)
	}
	if ev.Bytes != len(ev.Diff)+len(ev.Spec) {
		t.Fatalf("CA-401: Bytes = len(Diff)+len(Spec): %d vs %d+%d", ev.Bytes, len(ev.Diff), len(ev.Spec))
	}
	if ev.SHA256 != raSHA(ev.Diff, ev.Spec) || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(ev.SHA256) {
		t.Fatalf("CA-401: SHA256 es el hex de Diff seguido de Spec: %q", ev.SHA256)
	}

	// determinista: armarla dos veces da los mismos bytes
	otra, err := Evidence(root, "main", ".hoom/specs/x.md", raTopeGrande)
	if err != nil || !bytes.Equal(otra.Diff, ev.Diff) || otra.SHA256 != ev.SHA256 {
		t.Fatalf("CA-401: la evidencia del mismo arbol es la misma: %v", err)
	}
}

// CA-401 (enmienda 1, caso limite): una rama que COMMITEA el borrado de
// auth.go y el renombre de b.go a c.go. La evidencia trae el borrado con su
// 'deleted file' y las lineas quitadas, y el renombre con sus dos lados (las
// cabeceras 'rename from/to' de git o el par borrado + alta): no sale de
// git.ChangedFiles, que deja afuera los borrados commiteados y el origen del
// renombre. Sigue sin nada de .hoom/ (tampoco un borrado commiteado ahi), con
// el no rastreado de nombre no ASCII y el binario sin contenido; un archivo
// que solo EMPIEZA con .hoom esta fuera de .hoom/ y va.
func TestCA401_BorradoYRenombreCommiteados(t *testing.T) {
	root := raRepo(t, "")
	write(t, root, "auth.go", "package app\n\n// Autoriza es la comprobacion de permisos que la rama borra.\n"+
		"func Autoriza(rol string) bool {\n\treturn rol == \"admin\" // LINEA-DE-AUTORIZACION\n}\n")
	var b strings.Builder
	b.WriteString("package app\n\n// b.go se renombra entero a c.go en la rama.\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "var Renombrada%02d = %d\n", i, i)
	}
	write(t, root, "b.go", b.String())
	write(t, root, ".hoom/specs/viejo.md", "# viejo\n\nSPEC-VIEJO-BORRADO-EN-LA-RAMA\n")
	write(t, root, ".hoom/specs/sigue.md", "# sigue\n\nversion base\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "base con auth y b")
	git(t, root, "checkout", "-q", "-b", "feature")
	git(t, root, "rm", "-q", "auth.go")
	git(t, root, "mv", "b.go", "c.go")
	git(t, root, "rm", "-q", ".hoom/specs/viejo.md")
	write(t, root, ".hoom/specs/sigue.md", "# sigue\n\nSPEC-CAMBIADO-Y-COMMITEADO\n")
	write(t, root, ".hoom-fuera.go", "package app\n\n// EMPIEZA-CON-HOOM-PERO-ESTA-FUERA\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "borra auth, renombra b a c")
	// sin rastrear: nombre no ASCII, binario, y algo bajo .hoom/
	write(t, root, "piñata/ñandú.go", "package pinata\n\n// NO-ASCII-SIN-RASTREAR\n")
	write(t, root, "datos.bin", "\x00\x01BINARIO-SIN-CONTENIDO\x00\xff")
	write(t, root, ".hoom/notas.txt", "NOTA-HOOM-FUERA-DEL-DIFF\n")

	ev, err := Evidence(root, "main", "", raTopeGrande)
	if err != nil {
		t.Fatalf("CA-401: Evidence: %v", err)
	}
	if ev.Over {
		t.Fatalf("CA-401: una evidencia chica no pasa el tope: %d", ev.Bytes)
	}
	diff := string(ev.Diff)

	borrado := raSeccion(diff, "diff --git a/auth.go b/auth.go")
	if borrado == "" || !strings.Contains(borrado, "deleted file mode") {
		t.Fatalf("CA-401: el borrado commiteado de auth.go va con su 'deleted file mode':\n%s", diff)
	}
	for _, quitada := range []string{
		"-func Autoriza(rol string) bool {",
		"-\treturn rol == \"admin\" // LINEA-DE-AUTORIZACION",
	} {
		if !strings.Contains(borrado, quitada) {
			t.Fatalf("CA-401: el borrado trae la linea quitada %q:\n%s", quitada, borrado)
		}
	}

	renombre := strings.Contains(diff, "\nrename from b.go\n") && strings.Contains(diff, "\nrename to c.go\n")
	par := strings.Contains(raSeccion(diff, "diff --git a/b.go b/b.go"), "deleted file mode") &&
		strings.Contains(raSeccion(diff, "diff --git a/c.go b/c.go"), "new file mode")
	if !renombre && !par {
		t.Fatalf("CA-401: el renombre b.go -> c.go trae sus dos lados (rename from/to, o el borrado de b.go y el alta de c.go):\n%s", diff)
	}

	for _, quiero := range []string{"+// EMPIEZA-CON-HOOM-PERO-ESTA-FUERA", "+// NO-ASCII-SIN-RASTREAR", "datos.bin", "Binary files"} {
		if !strings.Contains(diff, quiero) {
			t.Fatalf("CA-401: la evidencia trae %q:\n%s", quiero, diff)
		}
	}
	if !raTieneNombre(diff, "piñata/ñandú.go") {
		t.Fatalf("CA-401: el no rastreado de nombre no ASCII va con su nombre (crudo o citado por git):\n%s", diff)
	}
	for _, nunca := range []string{"BINARIO-SIN-CONTENIDO", "GIT binary patch", "SPEC-VIEJO-BORRADO-EN-LA-RAMA",
		"SPEC-CAMBIADO-Y-COMMITEADO", "NOTA-HOOM-FUERA-DEL-DIFF", "a/.hoom/", "b/.hoom/"} {
		if strings.Contains(diff, nunca) {
			t.Fatalf("CA-401: la evidencia no trae %q (binario sin contenido, nada de .hoom/):\n%s", nunca, diff)
		}
	}
	if ev.Spec != nil || ev.Bytes != len(ev.Diff) || ev.SHA256 != raSHA(ev.Diff, nil) {
		t.Fatalf("CA-401: sin --spec, Spec nil, Bytes = len(Diff) y SHA256 del diff: spec %q, %d vs %d, %q",
			ev.Spec, ev.Bytes, len(ev.Diff), ev.SHA256)
	}
}

// CA-401 (caso limite): sin --spec la evidencia no tiene bloque de spec:
// Spec nil, Bytes = len(Diff), SHA256 del diff solo.
func TestCA401_SinSpecNoHayBloqueDeSpec(t *testing.T) {
	root := raRepo(t, "")
	raCambio(t, root)
	ev, err := Evidence(root, "main", "", raTopeGrande)
	if err != nil {
		t.Fatalf("CA-401: %v", err)
	}
	if ev.Spec != nil || ev.Bytes != len(ev.Diff) || len(ev.Diff) == 0 || ev.Over {
		t.Fatalf("CA-401: sin spec, Spec nil y Bytes = len(Diff) > 0: %+v", ev)
	}
	if ev.SHA256 != raSHA(ev.Diff, nil) {
		t.Fatalf("CA-401: sin spec el SHA256 es el del diff: %q", ev.SHA256)
	}
	if !strings.Contains(string(ev.Diff), "+func Nuevo() {}") {
		t.Fatalf("CA-401: el diff trae el cambio sin commitear:\n%s", ev.Diff)
	}
}

// CA-401: el diff sale del MERGE-BASE, no de la punta de la base: lo que la
// base hizo despues de abrir la rama no aparece (ni revertido).
func TestCA401_DiffDesdeElMergeBase(t *testing.T) {
	root := raRepo(t, "")
	write(t, root, "app.go", "package app\n\n// linea base\n")
	git(t, root, "commit", "-q", "-am", "base")
	git(t, root, "checkout", "-q", "-b", "feature")
	write(t, root, "rama.go", "package app\n\nvar EnLaRama = true\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "rama")
	git(t, root, "checkout", "-q", "main")
	write(t, root, "app.go", "package app\n\n// linea base\n// CAMBIO-EN-MAIN-DESPUES-DE-LA-RAMA\n")
	write(t, root, "solo_main.go", "package app\n\n// SOLO-EN-MAIN\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "main avanza")
	git(t, root, "checkout", "-q", "feature")
	write(t, root, "app.go", "package app\n\n// linea base\n// cambio de la rama sin commitear\n")

	ev := raEvidencia(t, "CA-401", root, "")
	diff := string(ev.Diff)
	if !strings.Contains(diff, "+var EnLaRama = true") || !strings.Contains(diff, "+// cambio de la rama sin commitear") {
		t.Fatalf("CA-401: el diff trae lo commiteado en la rama y lo sin commitear:\n%s", diff)
	}
	if strings.Contains(diff, "CAMBIO-EN-MAIN-DESPUES-DE-LA-RAMA") || strings.Contains(diff, "SOLO-EN-MAIN") || strings.Contains(diff, "solo_main.go") {
		t.Fatalf("CA-401: lo que la base hizo despues de la rama no es del candidato:\n%s", diff)
	}
}

// CA-401: las 4 pasadas reciben exactamente los mismos bytes de evidencia, y
// son los de Evidence; el Result y el registro dicen Bytes y SHA256.
func TestCA401_LasCuatroPasadasRecibenLosMismosBytes(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\nCriterio con unicode: ñandú €\n")
	cx := raInstalar(t, bin, "codex", "")
	ev := raEvidenciaCruda(t, "CA-401", root, ".hoom/specs/x.md")

	res, out := raRevisar(t, "CA-401", root, Options{Provider: "codex", Spec: ".hoom/specs/x.md"})
	if res.Status != "revisado" || cx.veces() != 4 {
		t.Fatalf("CA-401: fixture: las 4 lentes corren: %+v\n%s", res, out)
	}
	bloque := raBloque(t, ev, ".hoom/specs/x.md", true)
	var prefijo string
	for n := 1; n <= 4; n++ {
		p := cx.pedido(t, n)
		i := strings.Index(p, bloque)
		if i < 0 {
			t.Fatalf("CA-401: la pasada %d recibe el spec y el diff enteros, byte a byte, entre sus marcadores:\n%s", n, p)
		}
		if n == 1 {
			prefijo = p[:i+len(bloque)]
		} else if p[:i+len(bloque)] != prefijo {
			t.Fatalf("CA-401: la pasada %d recibe otra evidencia que la 1", n)
		}
	}
	if res.EvidenceBytes != ev.Bytes || res.EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("CA-401: el Result dice la evidencia que recibieron las lentes: %d %q vs %d %q",
			res.EvidenceBytes, res.EvidenceSHA256, ev.Bytes, ev.SHA256)
	}
	recs := raRegistros(t, root)
	if len(recs) != 1 || recs[0].EvidenceBytes != ev.Bytes || recs[0].EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("CA-401: el registro dice la evidencia: %+v", recs)
	}
}

// CA-401 (caso limite): --lens x recibe la misma evidencia, en una pasada.
func TestCA401_UnaLenteMismaEvidencia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	cx := raInstalar(t, bin, "codex", "")
	ev := raEvidenciaCruda(t, "CA-401", root, "")

	res, out := raRevisar(t, "CA-401", root, Options{Provider: "codex", Lens: "resilience"})
	if cx.veces() != 1 || len(res.Passes) != 1 {
		t.Fatalf("CA-401: --lens es una pasada: %+v\n%s", res, out)
	}
	if !strings.Contains(cx.pedido(t, 1), raBloque(t, ev, "", false)) || res.EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("CA-401: la pasada unica recibe la misma evidencia entera")
	}
}

// ---------------------------------------------------------------- CA-402

func raHallazgos(t *testing.T, dir string) int {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(dir, ".hoom", "findings"))
	return len(entries)
}

// CA-402: con la evidencia sobre el tope de hoom.yaml, ninguna pasada, ni
// registro ni hallazgos, exit 1 y el texto del contrato; el mismo cambio con
// el tope por defecto corre (el tope de hoom.yaml lo cambia).
func TestCA402_EvidenciaSobreElTopeNoLanzaPasadas(t *testing.T) {
	bin := raPATH(t)
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "var Relleno%02d = \"cuarenta bytes de relleno por linea\"\n", i)
	}
	root := raRepo(t, "review:\n  max_evidence_kib: 1\n")
	write(t, root, "app.go", b.String())
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if cx.veces() != 0 {
		t.Fatalf("CA-402: sobre el tope no se lanza ninguna pasada (codex corrio %d veces)\n%s", cx.veces(), out)
	}
	if res.Status != "no-entregable" || res.ExitCode != 1 || len(res.Passes) != 0 {
		t.Fatalf("CA-402: sobre el tope es no-entregable con exit 1 y sin pasadas: %+v\n%s", res, out)
	}
	if recs, _ := Records(root); len(recs) != 0 {
		t.Fatalf("CA-402: sin registro de review: %+v", recs)
	}
	if n := raHallazgos(t, root); n != 0 {
		t.Fatalf("CA-402: sin hallazgos: %d", n)
	}
	if !strings.Contains(out, raNoEntregable(1)+"\n") {
		t.Fatalf("CA-402: la salida dice %q:\n%s", raNoEntregable(1), out)
	}
	ev, err := Evidence(root, "main", "", 1024)
	if err != nil {
		t.Fatalf("CA-402: Evidence: %v", err)
	}
	raOver(t, "CA-402", ev, 1024)

	// el mismo cambio, sin tope en hoom.yaml (320 KiB), corre
	root2 := raRepo(t, "")
	write(t, root2, "app.go", b.String())
	res, out = raRevisar(t, "CA-402", root2, Options{Provider: "codex"})
	if res.Status != "revisado" || cx.veces() != 1 {
		t.Fatalf("CA-402: bajo el tope por defecto la misma evidencia corre: %+v\n%s", res, out)
	}
}

// CA-402: exactamente en el tope Evidence no pone Over y la review corre; un
// byte mas, Over y la negativa con el tope en KiB.
func TestCA402_IgualAlTopeCorreYUnByteMasNo(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  max_evidence_kib: 2\n")
	cx := raInstalar(t, bin, "codex", "")
	relleno := func(k int) { write(t, root, "relleno.go", "// "+strings.Repeat("x", k)+"\n") }

	relleno(1000)
	ev := raEvidencia(t, "CA-402", root, "")
	if ev.Bytes > 2048 {
		t.Fatalf("CA-402: fixture: con 1000 bytes de relleno la evidencia no pasa 2 KiB: %d", ev.Bytes)
	}
	k := 1000 + 2048 - ev.Bytes
	relleno(k)
	if ev = raEvidencia(t, "CA-402", root, ""); ev.Bytes != 2048 {
		t.Fatalf("CA-402: fixture: la evidencia queda exactamente en 2048 bytes: %d", ev.Bytes)
	}
	justa, err := Evidence(root, "main", "", 2048)
	if err != nil || justa.Over || justa.Bytes != 2048 || justa.SHA256 != ev.SHA256 || !bytes.Equal(justa.Diff, ev.Diff) {
		t.Fatalf("CA-402: exactamente en el tope no hay Over y la evidencia es la entera: %v, Over %v, %d bytes", err, justa.Over, justa.Bytes)
	}
	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if res.Status != "revisado" || cx.veces() != 1 {
		t.Fatalf("CA-402: igual al tope (2048 bytes, 2 KiB) corre: %+v\n%s", res, out)
	}

	relleno(k + 1) // 2049 bytes de evidencia
	if ev = raEvidencia(t, "CA-402", root, ""); ev.Bytes != 2049 {
		t.Fatalf("CA-402: fixture: un byte mas da 2049: %d", ev.Bytes)
	}
	pasada, err := Evidence(root, "main", "", 2048)
	if err != nil {
		t.Fatalf("CA-402: Evidence: %v", err)
	}
	raOver(t, "CA-402", pasada, 2048)
	antes := len(raRegistros(t, root))
	res, out = raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if res.Status != "no-entregable" || res.ExitCode != 1 || cx.veces() != 1 {
		t.Fatalf("CA-402: un byte sobre el tope no corre: %+v\n%s", res, out)
	}
	if len(raRegistros(t, root)) != antes {
		t.Fatal("CA-402: la negativa no escribe registro")
	}
	if !strings.Contains(out, raNoEntregable(2)) {
		t.Fatalf("CA-402: la negativa nombra el tope de hoom.yaml (2 KiB):\n%s", out)
	}
}

// CA-402 (caso limite): Over se decide sobre el diff y el spec JUNTOS, al
// byte: con el tope en el total no hay Over (y la evidencia es la misma que
// con cualquier tope mayor); un byte menos, o un tope donde el diff solo
// entra pero el spec lo empuja afuera, es Over.
func TestCA402_OverAlByteConDiffYSpecJuntos(t *testing.T) {
	root := raRepo(t, "")
	raCambio(t, root)
	const spec = ".hoom/specs/x.md"
	write(t, root, spec, "# Spec x\n\n"+strings.Repeat("criterio del spec que empuja la evidencia\n", 80))
	full := raEvidencia(t, "CA-402", root, spec)
	total := full.Bytes
	if len(full.Spec) < 2048 || len(full.Diff)+1 >= total {
		t.Fatalf("CA-402: fixture: el spec pesa en la evidencia: diff %d, spec %d", len(full.Diff), len(full.Spec))
	}
	for _, tope := range []int{total + 4096, total + 1, total, total - 1, len(full.Diff) + 1, len(full.Diff)} {
		ev, err := Evidence(root, "main", spec, tope)
		if err != nil {
			t.Fatalf("CA-402: Evidence con tope %d: %v", tope, err)
		}
		if tope < total {
			raOver(t, fmt.Sprintf("CA-402 (tope %d, evidencia %d)", tope, total), ev, tope)
			continue
		}
		if ev.Over || ev.Bytes != total || ev.SHA256 != full.SHA256 || !bytes.Equal(ev.Diff, full.Diff) || !bytes.Equal(ev.Spec, full.Spec) {
			t.Fatalf("CA-402: con tope %d >= %d la evidencia es la entera, sin Over: Over %v, %d bytes", tope, total, ev.Over, ev.Bytes)
		}
	}
}

// CA-402: sin tope en hoom.yaml el tope es 320 KiB: una evidencia mas grande
// se niega nombrandolo.
func TestCA402_TopePorDefecto320KiB(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; b.Len() < 330*1024; i++ {
		fmt.Fprintf(&b, "var Grande%05d = \"una linea de relleno para pasar el tope\"\n", i)
	}
	write(t, root, "grande.go", b.String())
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if cx.veces() != 0 || res.ExitCode != 1 || res.Status != "no-entregable" {
		t.Fatalf("CA-402: 330 KiB pasan el tope por defecto: codex %d, %+v\n%s", cx.veces(), res, out)
	}
	if !strings.Contains(out, raNoEntregable(320)) {
		t.Fatalf("CA-402: la negativa nombra el tope por defecto (320 KiB):\n%s", out)
	}
}

// CA-402 (caso limite): 0 lentes (solo documentacion) no arma evidencia y
// sigue SIN REVISAR, aunque la documentacion pase el tope. Guarda: el
// esqueleto ya es asi; el tope no puede adelantarse a la regla de 0 lentes.
func TestCA402_CeroLentesNoArmaEvidencia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  max_evidence_kib: 1\n")
	write(t, root, "README.md", strings.Repeat("documentacion larga\n", 400))
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if res.Status != "sin-revisar" || res.ExitCode != 0 || cx.veces() != 0 || len(res.Lenses) != 0 {
		t.Fatalf("CA-402: solo documentacion es SIN REVISAR como hoy: %+v\n%s", res, out)
	}
	if res.EvidenceBytes != 0 || res.EvidenceSHA256 != "" || strings.Contains(out, "NO ENTREGABLE") {
		t.Fatalf("CA-402: sin lentes no se arma evidencia: %+v\n%s", res, out)
	}
}

// CA-402 (caso limite): un modelo que no aguanta la evidencia aunque este
// bajo el tope: el run falla y la review es NO ENTREGABLE, como cualquier run
// fallido, sin registro. El CLI falso falla solo si el pedido trae mas de
// 2500 bytes: con la evidencia adentro (unos 3 KiB) no la aguanta.
func TestCA402_ModeloQueNoAguantaLaEvidenciaEsNoEntregable(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "var Relleno%02d = \"cuarenta bytes de relleno por linea\"\n", i)
	}
	write(t, root, "app.go", b.String())
	cx := raInstalar(t, bin, "codex",
		"for last; do :; done\n"+
			"if [ \"$last\" = \"-\" ]; then size=$(wc -c < \"$d/stdin.$n\"); else size=${#last}; fi\n"+
			"if [ \"$size\" -gt 2500 ]; then echo 'context window exceeded' >&2; exit 3; fi\n")

	res, out := raRevisar(t, "CA-402", root, Options{Provider: "codex"})
	if cx.veces() != 1 {
		t.Fatalf("CA-402: bajo el tope la pasada se lanza: codex %d\n%s", cx.veces(), out)
	}
	if res.Status != "no-entregable" || res.ExitCode != 1 || len(res.Passes) != 1 || res.Passes[0].RunStatus == "done" {
		t.Fatalf("CA-402: el run que no aguanta la evidencia falla y la review es NO ENTREGABLE: %+v\n%s", res, out)
	}
	if recs, _ := Records(root); len(recs) != 0 {
		t.Fatalf("CA-402: sin registro de review: %+v", recs)
	}
}

// ---------------------------------------------------------------- CA-412

// raSymlink planta en root/rel un symlink a destino (tal cual: relativo o
// absoluto).
func raSymlink(t *testing.T, root, rel, destino string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, p); err != nil {
		t.Fatal(err)
	}
}

// raSpecsQueNoSonDelArbol arma, cada uno en su repo con un cambio de codigo,
// un spec que existe pero no es un archivo regular dentro del arbol despues
// de resolver symlinks. Devuelve la ruta que se pasa como --spec.
var raSpecsQueNoSonDelArbol = []struct {
	nombre string
	armar  func(t *testing.T, root string) string
}{
	{"symlink-commiteado-relativo-a-un-archivo-de-afuera", func(t *testing.T, root string) string {
		const spec = ".hoom/specs/x.md"
		fuera := raFuera(t, "fuera.md", "# SECRETO-FUERA-DEL-ARBOL\n")
		rel, err := filepath.Rel(filepath.Join(root, ".hoom", "specs"), fuera)
		if err != nil {
			t.Fatal(err)
		}
		raSymlink(t, root, spec, rel)
		git(t, root, "add", spec)
		git(t, root, "commit", "-q", "-m", "spec symlink a un archivo de afuera")
		return spec
	}},
	{"symlink-sin-commitear-absoluto-a-un-archivo-de-afuera", func(t *testing.T, root string) string {
		const spec = ".hoom/specs/x.md"
		raSymlink(t, root, spec, raFuera(t, "fuera.md", "# SECRETO-FUERA-DEL-ARBOL\n"))
		return spec
	}},
	{"symlink-a-un-directorio-del-arbol", func(t *testing.T, root string) string {
		const spec = ".hoom/specs/x.md"
		write(t, root, "sub/a.go", "package sub\n")
		raSymlink(t, root, spec, "../../sub")
		return spec
	}},
	{"ruta-que-sale-del-arbol-con-puntos", func(t *testing.T, root string) string {
		rel, err := filepath.Rel(root, raFuera(t, "fuera.md", "# SECRETO-FUERA-DEL-ARBOL\n"))
		if err != nil || !strings.HasPrefix(rel, "..") {
			t.Fatalf("CA-412: fixture: la ruta sale del arbol: %q %v", rel, err)
		}
		return filepath.ToSlash(rel)
	}},
}

// CA-412: un spec que existe pero no es un archivo regular del arbol despues
// de resolver symlinks (symlink commiteado o no a un archivo de afuera,
// symlink a un directorio, ruta que sale con '..') hace que Evidence
// devuelva 'el spec <ruta> no es un archivo del arbol'.
func TestCA412_SpecQueNoEsArchivoDelArbolEsError(t *testing.T) {
	for _, c := range raSpecsQueNoSonDelArbol {
		t.Run(c.nombre, func(t *testing.T) {
			root := raRepo(t, "")
			spec := c.armar(t, root)
			raCambio(t, root)
			ev, err, _ := raEvidenceConReloj(t, "CA-412", 60*time.Second, root, "main", spec, raTopeGrande)
			if err == nil || !strings.Contains(err.Error(), raNoEsDelArbol(spec)) {
				t.Fatalf("CA-412: Evidence devuelve %q: %v (spec leido: %q)", raNoEsDelArbol(spec), err, ev.Spec)
			}
			if bytes.Contains(ev.Spec, []byte("SECRETO-FUERA-DEL-ARBOL")) {
				t.Fatalf("CA-412: el spec de afuera no se lee: %q", ev.Spec)
			}
		})
	}
}

// CA-412: con ese spec `hoom review` no lanza ninguna pasada ni escribe
// registro, y el usuario ve por que.
func TestCA412_ReviewConSpecQueNoEsDelArbolNoLanzaPasadas(t *testing.T) {
	for _, c := range raSpecsQueNoSonDelArbol {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, "")
			spec := c.armar(t, root)
			raCambio(t, root)
			cx := raInstalar(t, bin, "codex", "")

			var out bytes.Buffer
			res, err := Run(root, "main", Options{Provider: "codex", Lens: "risk", Spec: spec}, &out)
			if cx.veces() != 0 || len(res.Passes) != 0 || res.Status == "revisado" {
				t.Fatalf("CA-412: sin un spec del arbol no se lanza ninguna pasada (codex %d): %+v %v\n%s", cx.veces(), res, err, out.String())
			}
			if recs, _ := Records(root); len(recs) != 0 {
				t.Fatalf("CA-412: sin pasadas no hay registro: %+v", recs)
			}
			msg := out.String()
			if err != nil {
				msg += err.Error()
			}
			if !strings.Contains(msg, raNoEsDelArbol(spec)) {
				t.Fatalf("CA-412: la review dice %q: %v\n%s", raNoEsDelArbol(spec), err, out.String())
			}
		})
	}
}

// CA-412 (caso limite): un spec symlink a un archivo regular DENTRO del arbol
// es un archivo del arbol despues de resolver symlinks: se lee.
func TestCA412_SpecSymlinkDentroDelArbolSeLee(t *testing.T) {
	root := raRepo(t, "")
	raCambio(t, root)
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\ntexto del spec real\n")
	raSymlink(t, root, ".hoom/specs/alias.md", "x.md")
	ev, err := Evidence(root, "main", ".hoom/specs/alias.md", raTopeGrande)
	if err != nil || string(ev.Spec) != "# Spec x\n\ntexto del spec real\n" {
		t.Fatalf("CA-412: un symlink a un archivo del arbol se lee: %v %q", err, ev.Spec)
	}
	if ev.Bytes != len(ev.Diff)+len(ev.Spec) || ev.SHA256 != raSHA(ev.Diff, ev.Spec) {
		t.Fatalf("CA-401: Bytes y SHA256 cierran con el spec leido: %d, %q", ev.Bytes, ev.SHA256)
	}
}

// raHijoDevZero: con esta variable el test corre como proceso hijo y arma la
// evidencia con un vigilante que lo mata si asigna mas de 256 MiB. Asi un
// Evidence que lee /dev/zero no se come la memoria de la maquina ni cuelga
// al padre, que ademas le pone reloj.
const raHijoDevZero = "HOOM_TW_CA412_DEVZERO_ROOT"

func raHijoEvidencia(root, spec string) {
	go func() {
		var ms runtime.MemStats
		for {
			runtime.ReadMemStats(&ms)
			if ms.TotalAlloc > 256<<20 {
				fmt.Println("RA-CA412-LEYO-DE-MAS")
				os.Exit(42)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	ev, err := Evidence(root, "main", spec, 1<<20)
	r := map[string]any{"over": ev.Over, "bytes": ev.Bytes}
	if err != nil {
		r["err"] = err.Error()
	}
	raw, _ := json.Marshal(r)
	fmt.Println("RA-CA412 " + string(raw))
}

// CA-412: un spec symlink a /dev/zero (commiteado en la base o sin commitear)
// hace que Evidence devuelva 'el spec <ruta> no es un archivo del arbol' SIN
// LEERLO: vuelve enseguida y sin asignar memoria. Corre en un proceso hijo con
// reloj de 60 s y un vigilante de 256 MiB.
func TestCA412_SpecSymlinkADevZeroNoSeLee(t *testing.T) {
	if root := os.Getenv(raHijoDevZero); root != "" {
		raHijoEvidencia(root, os.Getenv(raHijoDevZero+"_SPEC"))
		return
	}
	if fi, err := os.Stat("/dev/zero"); err != nil || fi.Mode()&os.ModeDevice == 0 {
		t.Skip("sin /dev/zero")
	}
	const spec = ".hoom/specs/x.md"
	for _, commitear := range []bool{true, false} {
		nombre := "sin-commitear"
		if commitear {
			nombre = "commiteado-en-la-base"
		}
		t.Run(nombre, func(t *testing.T) {
			root := raRepo(t, "")
			raSymlink(t, root, spec, "/dev/zero")
			if commitear {
				git(t, root, "add", spec)
				git(t, root, "commit", "-q", "-m", "spec symlink a /dev/zero")
			}
			raCambio(t, root)

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCA412_SpecSymlinkADevZeroNoSeLee$", "-test.count=1")
			cmd.Env = append(os.Environ(), raHijoDevZero+"="+root, raHijoDevZero+"_SPEC="+spec)
			raw, _ := cmd.CombinedOutput()
			salida := string(raw)
			if len(salida) > 4000 {
				salida = salida[len(salida)-4000:]
			}
			if ctx.Err() != nil {
				t.Fatalf("CA-412: con un spec symlink a /dev/zero Evidence no volvio en 60 s: lo esta leyendo\n%s", salida)
			}
			if strings.Contains(salida, "RA-CA412-LEYO-DE-MAS") {
				t.Fatalf("CA-412: con un spec symlink a /dev/zero Evidence asigno mas de 256 MiB: lo leyo\n%s", salida)
			}
			m := regexp.MustCompile(`(?m)^RA-CA412 (.*)$`).FindStringSubmatch(salida)
			if m == nil {
				t.Fatalf("CA-412: el proceso hijo no informo el resultado:\n%s", salida)
			}
			var r struct {
				Err string `json:"err"`
			}
			if err := json.Unmarshal([]byte(m[1]), &r); err != nil {
				t.Fatalf("CA-412: resultado ilegible del hijo: %v %s", err, m[1])
			}
			if !strings.Contains(r.Err, raNoEsDelArbol(spec)) {
				t.Fatalf("CA-412: Evidence devuelve %q con un spec symlink a /dev/zero, devolvio %s", raNoEsDelArbol(spec), m[1])
			}
		})
	}
}

// CA-412 (caso limite, re-expresa CA-334): un spec que no existe deja la
// review seguir: Evidence sin error, Spec nil, y el pedido dice
// 'Spec: <ruta> (no existe en este arbol)' sin bloque de spec.
func TestCA412_SpecQueNoExisteLaReviewSigue(t *testing.T) {
	const spec = ".hoom/specs/no-existe.md"
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")

	ev, err := Evidence(root, "main", spec, raTopeGrande)
	if err != nil || ev.Over {
		t.Fatalf("CA-412: un spec que no existe no es error: %v (Over %v)", err, ev.Over)
	}
	if ev.Spec != nil || ev.Bytes != len(ev.Diff) || ev.SHA256 != raSHA(ev.Diff, nil) || len(ev.Diff) == 0 {
		t.Fatalf("CA-412: sin el spec, Spec nil y la evidencia es la del diff: spec %q, %d bytes vs diff %d, %q", ev.Spec, ev.Bytes, len(ev.Diff), ev.SHA256)
	}
	g := gitx.Snapshot(root, "main")
	res, out := raRevisar(t, "CA-412", root, Options{Provider: "codex", Lens: "risk", Spec: spec})
	if res.Status != "revisado" || cx.veces() != 1 {
		t.Fatalf("CA-412: con un spec que no existe la review sigue: %+v\n%s", res, out)
	}
	raRevisarPedido(t, cx.pedido(t, 1), "risk", g, ev, spec, false,
		"No hay veredicto vigente: la review no reemplaza a 'hoom verify'.")
}

// CA-412 (caso limite): un spec de 0 bytes existe: la linea Spec: sin la
// nota, y su bloque va vacio (el marcador del spec seguido del del diff).
func TestCA412_SpecVacioLlevaSuBloqueVacio(t *testing.T) {
	const spec = ".hoom/specs/vacio.md"
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	write(t, root, spec, "")
	cx := raInstalar(t, bin, "codex", "")

	ev, err := Evidence(root, "main", spec, raTopeGrande)
	if err != nil || ev.Over || len(ev.Spec) != 0 || ev.Bytes != len(ev.Diff) || ev.SHA256 != raSHA(ev.Diff, nil) {
		t.Fatalf("CA-412: un spec vacio existe y no suma bytes: %v, Over %v, spec %q, %d bytes", err, ev.Over, ev.Spec, ev.Bytes)
	}
	g := gitx.Snapshot(root, "main")
	res, out := raRevisar(t, "CA-412", root, Options{Provider: "codex", Lens: "risk", Spec: spec})
	if res.Status != "revisado" || cx.veces() != 1 {
		t.Fatalf("CA-412: con un spec vacio la review corre: %+v\n%s", res, out)
	}
	p := cx.pedido(t, 1)
	h := raMarca(t, ev)
	if !strings.Contains(p, "\nSpec: "+spec+"\n=== spec "+spec+" "+h+" ===\n=== diff "+h+" ===\n") {
		t.Fatalf("CA-412: el bloque del spec vacio es su marcador seguido del marcador del diff:\n%s", p)
	}
	raRevisarPedido(t, p, "risk", g, ev, spec, true, "No hay veredicto vigente: la review no reemplaza a 'hoom verify'.")
}

// ---------------------------------------------------------------- CA-413

// CA-413: con un tope de 1 KiB y un archivo no rastreado de 64 MiB, Evidence
// vuelve enseguida con Over, Diff y Spec vacios, SHA256 vacio y Bytes entre
// el tope y el tope + 64 KiB. Y deja de leer de verdad: mientras arma la
// evidencia el proceso no asigna ni la cuarta parte del archivo.
func TestCA413_NoRastreadoDe64MiBConTopeDe1KiB(t *testing.T) {
	root := raRepo(t, "")
	linea := strings.Repeat("x", 63) + "\n"
	if err := os.WriteFile(filepath.Join(root, "enorme.txt"), bytes.Repeat([]byte(linea), (64<<20)/len(linea)), 0o644); err != nil {
		t.Fatal(err)
	}
	ev, err, asignado := raEvidenceConReloj(t, "CA-413", 60*time.Second, root, "main", "", 1024)
	if err != nil {
		t.Fatalf("CA-413: Evidence: %v", err)
	}
	raOver(t, "CA-413", ev, 1024)
	if asignado > 16<<20 {
		t.Fatalf("CA-413: con un tope de 1 KiB Evidence asigno %d MiB: no dejo de leer el archivo de 64 MiB", asignado>>20)
	}
}

// CA-413 (caso limite): el tope tambien corta la lectura del spec: un spec de
// 32 MiB con tope de 1 KiB es Over, con lo leido acotado, sin leerlo entero.
func TestCA413_SpecEnormeConTopeDe1KiB(t *testing.T) {
	root := raRepo(t, "")
	raCambio(t, root)
	const spec = ".hoom/specs/grande.md"
	linea := "criterio de relleno de un spec enorme, sesenta y cuatro bytes\n"
	write(t, root, spec, strings.Repeat(linea, (32<<20)/len(linea)))
	ev, err, asignado := raEvidenceConReloj(t, "CA-413", 60*time.Second, root, "main", spec, 1024)
	if err != nil {
		t.Fatalf("CA-413: Evidence: %v", err)
	}
	raOver(t, "CA-413", ev, 1024)
	if asignado > 16<<20 {
		t.Fatalf("CA-413: con un tope de 1 KiB Evidence asigno %d MiB: leyo el spec de 32 MiB entero", asignado>>20)
	}
}

// ---------------------------------------------------------------- CA-414

// raGitRoto pone en bin un git que falla (exit 128, con su mensaje en stderr)
// cuando le piden alguno de esos subcomandos y en todo lo demas es el git de
// verdad.
func raGitRoto(t *testing.T, bin, real string, subcomandos ...string) {
	t.Helper()
	s := "#!/bin/sh\nfor a in \"$@\"; do\n  case \"$a\" in\n    " + strings.Join(subcomandos, "|") + ")\n" +
		"      echo \"fatal: git $a roto por el test (CA-414)\" >&2\n      exit 128;;\n  esac\ndone\n" +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
}

// CA-414: si falla git merge-base (una base que no existe, un clon shallow
// sin el merge-base), el listado de no rastreados (git ls-files) o git diff,
// Evidence devuelve el error y `hoom review` no lanza ninguna pasada ni
// escribe registro: no revisa en silencio otro parche.
func TestCA414_GitQueFallaEsErrorYNoHayPasadas(t *testing.T) {
	real := raGitReal(t)
	casos := []struct {
		nombre string
		armar  func(t *testing.T, bin string) (root, base string)
	}{
		{"base-inexistente", func(t *testing.T, bin string) (string, string) {
			// la base que usa la review es la de hoom.yaml: base_branch apunta a
			// una rama que no existe
			root := raRepo(t, "")
			raw, err := os.ReadFile(filepath.Join(root, "hoom.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			write(t, root, "hoom.yaml", strings.Replace(string(raw), "base_branch: main\n", "base_branch: rama-que-no-existe\n", 1))
			git(t, root, "commit", "-q", "-am", "base_branch inexistente")
			raCambio(t, root)
			return root, "rama-que-no-existe"
		}},
		{"clon-shallow-sin-merge-base", func(t *testing.T, bin string) (string, string) {
			clon := raClonShallow(t)
			write(t, clon, "rama.go", "package app\n\nvar Rama = 3 // sin commitear\n")
			return clon, "main"
		}},
		{"ls-files-falla", func(t *testing.T, bin string) (string, string) {
			root := raRepo(t, "")
			raCambio(t, root)
			write(t, root, "nuevo.go", "package app\n\n// sin rastrear\n")
			raGitRoto(t, bin, real, "ls-files")
			return root, "main"
		}},
		{"diff-falla", func(t *testing.T, bin string) (string, string) {
			root := raRepo(t, "")
			raCambio(t, root)
			write(t, root, "nuevo.go", "package app\n\n// sin rastrear\n")
			raGitRoto(t, bin, real, "diff", "diff-index", "diff-files", "diff-tree")
			return root, "main"
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root, base := c.armar(t, bin)

			ev, err, _ := raEvidenceConReloj(t, "CA-414", 60*time.Second, root, base, "", raTopeGrande)
			if err == nil {
				t.Errorf("CA-414: %s: Evidence devuelve el error de git; devolvio una evidencia de %d bytes:\n%s", c.nombre, ev.Bytes, ev.Diff)
			}

			cx := raInstalar(t, bin, "codex", "")
			var out bytes.Buffer
			res, rerr := Run(root, base, Options{Provider: "codex", Lens: "risk"}, &out)
			if cx.veces() != 0 || len(res.Passes) != 0 || res.Status == "revisado" {
				t.Fatalf("CA-414: %s: la review no lanza ninguna pasada (codex %d): %+v %v\n%s", c.nombre, cx.veces(), res, rerr, out.String())
			}
			if recs, _ := Records(root); len(recs) != 0 {
				t.Fatalf("CA-414: %s: sin registro de review: %+v", c.nombre, recs)
			}
		})
	}
}

// ---------------------------------------------------------------- CA-416
//
// Enmienda 3: si el cambio toca algun .gitignore (rastreado y modificado,
// commiteado en la rama o no, borrado, o nuevo sin rastrear) y hay archivos
// sin rastrear fuera de .hoom/, Evidence se niega sin armar la evidencia:
// asi un cambio no destapa un secreto local (.env) para que viaje al
// provider.

// raErrGitignore es el texto del contrato (CA-416).
const raErrGitignore = "el cambio toca un .gitignore y hay archivos sin rastrear: commitealos o sacalos antes de revisar"

// raSecreto es lo que guarda el .env local: no puede aparecer en ningun lado.
const raSecreto = "API_KEY=SECRETO-DEL-ENV-QUE-NO-VIAJA-CA416"

// raBaseConGitignore es raRepo con un .gitignore en la raiz que ignora .env y
// *.log, y otro rastreado en sub/ que ignora *.tmp, commiteados en main.
func raBaseConGitignore(t *testing.T) string {
	t.Helper()
	root := raRepo(t, "")
	write(t, root, ".gitignore", ".env\n*.log\n")
	write(t, root, "sub/.gitignore", "*.tmp\n")
	write(t, root, "sub/uno.go", "package sub\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "la base ignora .env, *.log y sub/*.tmp")
	return root
}

// raRamaGitignore abre la rama feature sobre la base con .gitignore.
func raRamaGitignore(t *testing.T) string {
	t.Helper()
	root := raBaseConGitignore(t)
	git(t, root, "checkout", "-q", "-b", "feature")
	return root
}

// raSinSecretoEnHoom exige que ningun archivo bajo .hoom/ (registros, runs,
// hallazgos) traiga el secreto.
func raSinSecretoEnHoom(t *testing.T, ca, root string) {
	t.Helper()
	_ = filepath.Walk(filepath.Join(root, ".hoom"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !fi.Mode().IsRegular() {
			return nil
		}
		if raw, _ := os.ReadFile(p); bytes.Contains(raw, []byte(raSecreto)) {
			t.Fatalf("%s: el secreto del .env quedo en %s", ca, p)
		}
		return nil
	})
}

// raGitignoreTocadoConNoRastreados: cada caso toca un .gitignore y deja al
// menos un archivo sin rastrear fuera de .hoom/ (casi siempre el secreto que
// el cambio destapa), mas un cambio de codigo rastreado (una lente).
var raGitignoreTocadoConNoRastreados = []struct {
	nombre string
	armar  func(t *testing.T, root string)
}{
	// el caso que motivo la enmienda: la base ignora .env, la rama commitea un
	// .gitignore sin esa linea y hay un .env local con un secreto
	{"gitignore-commiteado-en-la-rama-destapa-el-env", func(t *testing.T, root string) {
		write(t, root, ".gitignore", "*.log\n")
		git(t, root, "commit", "-q", "-am", "la rama saca .env del .gitignore")
		write(t, root, ".env", raSecreto+"\n")
	}},
	{"gitignore-rastreado-modificado-sin-commitear", func(t *testing.T, root string) {
		write(t, root, ".gitignore", "*.log\n")
		write(t, root, ".env", raSecreto+"\n")
	}},
	{"gitignore-borrado-en-un-commit-de-la-rama", func(t *testing.T, root string) {
		git(t, root, "rm", "-q", ".gitignore")
		git(t, root, "commit", "-q", "-m", "la rama borra el .gitignore")
		write(t, root, ".env", raSecreto+"\n")
	}},
	{"gitignore-borrado-sin-commitear", func(t *testing.T, root string) {
		if err := os.Remove(filepath.Join(root, ".gitignore")); err != nil {
			t.Fatal(err)
		}
		write(t, root, ".env", raSecreto+"\n")
	}},
	{"gitignore-de-un-subdirectorio-rastreado-y-modificado", func(t *testing.T, root string) {
		write(t, root, "sub/.gitignore", "*.tmp\n!clave.log\n")
		write(t, root, "sub/clave.log", raSecreto+"\n")
	}},
	{"gitignore-nuevo-sin-rastrear-en-un-subdirectorio", func(t *testing.T, root string) {
		write(t, root, "otro/.gitignore", "!clave.log\n")
		write(t, root, "otro/clave.log", raSecreto+"\n")
	}},
	{"gitignore-nuevo-commiteado-en-un-subdirectorio", func(t *testing.T, root string) {
		write(t, root, "otro/.gitignore", "!clave.log\n")
		git(t, root, "add", "otro/.gitignore")
		git(t, root, "commit", "-q", "-m", "la rama destapa otro/clave.log")
		write(t, root, "otro/clave.log", raSecreto+"\n")
	}},
	// la regla es cerrada: tocar el .gitignore (aunque sea para ignorar MAS)
	// con cualquier no rastreado fuera de .hoom/ alcanza; el .env sigue
	// ignorado y tampoco viaja
	{"gitignore-que-ignora-mas-con-un-no-rastreado-cualquiera", func(t *testing.T, root string) {
		write(t, root, ".gitignore", ".env\n*.log\n*.bak\n")
		git(t, root, "commit", "-q", "-am", "la rama ignora *.bak")
		write(t, root, ".env", raSecreto+"\n")
		write(t, root, "nuevo.go", "package app\n\n// no rastreado cualquiera\n")
	}},
}

// CA-416: con un .gitignore tocado por el cambio y un archivo sin rastrear
// fuera de .hoom/, Evidence devuelve el error del contrato sin armar la
// evidencia: ni diff, ni spec, ni sha256, ni el secreto.
func TestCA416_GitignoreTocadoConNoRastreadosEsError(t *testing.T) {
	for _, c := range raGitignoreTocadoConNoRastreados {
		t.Run(c.nombre, func(t *testing.T) {
			root := raRamaGitignore(t)
			c.armar(t, root)
			raCambio(t, root)
			write(t, root, ".hoom/specs/x.md", "# Spec x\n")

			ev, err, _ := raEvidenceConReloj(t, "CA-416", 60*time.Second, root, "main", ".hoom/specs/x.md", raTopeGrande)
			if err == nil || !strings.Contains(err.Error(), raErrGitignore) {
				t.Fatalf("CA-416: Evidence devuelve %q: %v (evidencia de %d bytes)\n%s", raErrGitignore, err, ev.Bytes, ev.Diff)
			}
			if len(ev.Diff) != 0 || len(ev.Spec) != 0 || ev.SHA256 != "" {
				t.Fatalf("CA-416: sin armar la evidencia: diff %d bytes, spec %d bytes, sha256 %q", len(ev.Diff), len(ev.Spec), ev.SHA256)
			}
			if strings.Contains(err.Error(), raSecreto) {
				t.Fatalf("CA-416: el error no trae el secreto: %v", err)
			}
		})
	}
}

// CA-416: en esos casos `hoom review` no lanza ninguna pasada ni escribe
// registro, dice por que, y el secreto no aparece en la salida, en el error
// ni en nada de .hoom/: nunca llega al provider.
func TestCA416_ReviewConGitignoreTocadoYNoRastreadosNoLanzaPasadas(t *testing.T) {
	for _, c := range raGitignoreTocadoConNoRastreados {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRamaGitignore(t)
			c.armar(t, root)
			raCambio(t, root)
			cx := raInstalar(t, bin, "codex", "")

			var out bytes.Buffer
			res, err := Run(root, "main", Options{Provider: "codex", Lens: "risk"}, &out)
			if cx.veces() != 0 || len(res.Passes) != 0 || res.Status == "revisado" {
				t.Fatalf("CA-416: no se lanza ninguna pasada (codex %d): %+v %v\n%s", cx.veces(), res, err, out.String())
			}
			if recs, _ := Records(root); len(recs) != 0 {
				t.Fatalf("CA-416: sin pasadas no hay registro: %+v", recs)
			}
			msg := out.String()
			if err != nil {
				msg += "\n" + err.Error()
			}
			if !strings.Contains(msg, raErrGitignore) {
				t.Fatalf("CA-416: la review dice %q: %v\n%s", raErrGitignore, err, out.String())
			}
			if strings.Contains(msg, raSecreto) {
				t.Fatalf("CA-416: el secreto no aparece en la salida ni en el error:\n%s", msg)
			}
			raSinSecretoEnHoom(t, "CA-416", root)
		})
	}
}

// CA-416 (control): el cambio toca el .gitignore pero no hay nada sin
// rastrear fuera de .hoom/ (lo que el .gitignore ignora no esta sin rastrear,
// y un no rastreado bajo .hoom/ no cuenta): la evidencia se arma como
// siempre y trae el cambio del .gitignore. Commiteado en la rama o sin
// commitear.
func TestCA416_GitignoreTocadoSinNoRastreadosArmaLaEvidencia(t *testing.T) {
	for _, commitear := range []bool{true, false} {
		nombre := "sin-commitear"
		if commitear {
			nombre = "commiteado-en-la-rama"
		}
		t.Run(nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRamaGitignore(t)
			write(t, root, ".gitignore", ".env\n*.log\n*.bak\n# LINEA-NUEVA-DEL-GITIGNORE\n")
			if commitear {
				git(t, root, "commit", "-q", "-am", "la rama ignora *.bak")
			}
			raCambio(t, root)
			write(t, root, ".env", raSecreto+"\n")                // ignorado por la base y por la rama
			write(t, root, "copia.bak", "IGNORADO-POR-LA-RAMA\n") // ignorado por la linea nueva
			write(t, root, "sub/cache.tmp", "IGNORADO-EN-SUB\n")  // ignorado por sub/.gitignore
			write(t, root, ".hoom/specs/x.md", "# Spec x\n")      // sin rastrear, pero bajo .hoom/

			ev, err := Evidence(root, "main", ".hoom/specs/x.md", raTopeGrande)
			if err != nil || ev.Over {
				t.Fatalf("CA-416: sin no rastreados fuera de .hoom/ la evidencia se arma como siempre: %v (Over %v)", err, ev.Over)
			}
			diff := string(ev.Diff)
			if !strings.Contains(diff, "diff --git a/.gitignore b/.gitignore") || !strings.Contains(diff, "+# LINEA-NUEVA-DEL-GITIGNORE") ||
				!strings.Contains(diff, "+func Nuevo() {}") {
				t.Fatalf("CA-416: la evidencia trae el cambio del .gitignore y el del codigo:\n%s", diff)
			}
			for _, nunca := range []string{raSecreto, "IGNORADO-POR-LA-RAMA", "IGNORADO-EN-SUB"} {
				if strings.Contains(diff, nunca) {
					t.Fatalf("CA-416: lo ignorado no va en la evidencia: %q\n%s", nunca, diff)
				}
			}
			if string(ev.Spec) != "# Spec x\n" || ev.Bytes != len(ev.Diff)+len(ev.Spec) || ev.SHA256 != raSHA(ev.Diff, ev.Spec) {
				t.Fatalf("CA-416: la evidencia es la de siempre, con su spec, Bytes y SHA256: %q %d %q", ev.Spec, ev.Bytes, ev.SHA256)
			}

			cx := raInstalar(t, bin, "codex", "")
			res, out := raRevisar(t, "CA-416", root, Options{Provider: "codex", Lens: "risk", Spec: ".hoom/specs/x.md"})
			if res.Status != "revisado" || cx.veces() != 1 || res.EvidenceSHA256 != ev.SHA256 {
				t.Fatalf("CA-416: la review corre como siempre: codex %d, %+v\n%s", cx.veces(), res, out)
			}
			if strings.Contains(cx.pedido(t, 1), raSecreto) {
				t.Fatal("CA-416: el .env ignorado no viaja al provider")
			}
		})
	}
}

// CA-416 (control): hay archivos sin rastrear pero el cambio no toca ningun
// .gitignore: la evidencia se arma como siempre, con los no rastreados como
// archivos nuevos y sin lo ignorado. Tampoco cuenta un .gitignore que solo
// cambio la BASE despues de abrir la rama (el cambio es contra el
// merge-base), ni un archivo que se llama parecido sin ser un .gitignore.
func TestCA416_NoRastreadosSinGitignoreTocadoArmaLaEvidencia(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func(t *testing.T, root string)
	}{
		{"la-rama-no-toca-el-gitignore", func(t *testing.T, root string) {
			write(t, root, "rama.go", "package app\n\nvar EnLaRama = true\n")
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "rama")
		}},
		{"solo-la-base-toco-el-gitignore-despues-de-la-rama", func(t *testing.T, root string) {
			git(t, root, "checkout", "-q", "main")
			write(t, root, ".gitignore", ".env\n*.log\n*.bak\n")
			git(t, root, "commit", "-q", "-am", "main ignora *.bak")
			git(t, root, "checkout", "-q", "feature")
		}},
		{"archivos-que-no-son-un-gitignore", func(t *testing.T, root string) {
			write(t, root, ".gitignore.bak", "*.log\n")
			write(t, root, "docs/plantilla.gitignore", "*.log\n")
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "archivos con gitignore en el nombre")
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			root := raRamaGitignore(t)
			c.armar(t, root)
			raCambio(t, root)
			write(t, root, ".env", raSecreto+"\n")
			write(t, root, "nuevo.go", "package app\n\n// NO-RASTREADO-QUE-VA\n")

			ev, err := Evidence(root, "main", "", raTopeGrande)
			if err != nil || ev.Over {
				t.Fatalf("CA-416: sin un .gitignore tocado por el cambio, los no rastreados no frenan la evidencia: %v (Over %v)", err, ev.Over)
			}
			diff := string(ev.Diff)
			if !strings.Contains(diff, "+// NO-RASTREADO-QUE-VA") || !strings.Contains(diff, "+func Nuevo() {}") {
				t.Fatalf("CA-416: la evidencia trae el no rastreado y el cambio de codigo:\n%s", diff)
			}
			if strings.Contains(diff, raSecreto) || strings.Contains(diff, "diff --git a/.gitignore b/.gitignore") {
				t.Fatalf("CA-416: sin el .gitignore tocado, el .env sigue ignorado y el .gitignore no es parte del cambio:\n%s", diff)
			}
		})
	}
}
