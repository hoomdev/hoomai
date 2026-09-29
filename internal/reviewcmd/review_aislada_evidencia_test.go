// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (enmiendas 1, 3 y 4: CA-401, CA-402, CA-412..CA-414, CA-416): hoom congela
// la evidencia (el cambio COMMITEADO entero contra el merge-base, con
// borrados y renombres, de todo lo que esta fuera de .hoom/ mas
// .hoom/agents/, + el spec de HEAD) una vez, leyendo con tope; se niega a
// correr si pasa el tope; la entrada del spec en HEAD tiene que ser un
// archivo regular; un error de git falla cerrado; un aviso de git no es un
// error; y con el arbol sucio fuera de .hoom/ se niega sin armar nada. Los
// fixtures estan en review_aislada_helpers_test.go.
//
// ENMIENDA 4 (re-expresion): la review revisa solo lo commiteado. Los
// fixtures que dejaban el cambio sin commitear lo commitean en la rama
// feature, con las mismas afirmaciones sobre el contenido de la evidencia.
// Los tests que fijaban comportamiento que la enmienda borra (la regla del
// .gitignore de la enmienda 3 con archivos sin rastrear, las guardas de los
// no rastreados FIFO y symlink, el spec symlink que se resolvia dentro del
// arbol) se re-expresan contra el contrato nuevo: arbol sucio = negativa que
// nombra la ruta (CA-416); spec = su entrada en HEAD (CA-412).
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
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// ---------------------------------------------------------------- CA-401

// CA-401 (re-expresado por la enmienda 4): Evidence arma el cambio
// COMMITEADO entero contra el merge-base: lo de cada commit de la rama,
// archivos nuevos (tambien con nombre no ASCII), binarios sin contenido, y
// nada de .hoom/ fuera de .hoom/agents/, ni commiteado ni sin commitear; mas
// el spec tal como esta en HEAD; Bytes y SHA256 cierran.
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
	// el segundo commit de la rama: un rastreado, archivos nuevos (no ASCII,
	// binario), un binario rastreado que cambia, y .hoom/ que no va
	write(t, root, "app.go", "package app\n\nfunc EnElSegundoCommit() {}\n")
	write(t, root, "nuevo.go", "package app\n\n// archivo nuevo commiteado ñandú\nfunc Nuevo() {}\n")
	write(t, root, "dir con espacio/ñu.go", "package dir\n\n// contenido-unico-ñu\n")
	write(t, root, "nuevo.bin", "\x00MARCA-BINARIA-NUEVO-ARCHIVO\x00\xff")
	write(t, root, "imagen.bin", "\x00\x02MARCA-BINARIA-NUEVA\x00\xff")
	const specRama = "# Spec x\n\nversion rama: SPEC-NO-VA-EN-EL-DIFF\n"
	write(t, root, ".hoom/specs/x.md", specRama)
	write(t, root, ".hoom/notas.txt", "NOTA-HOOM-FUERA-DEL-DIFF\n")
	raCommit(t, root, "segundo commit de la rama")
	// sin commitear y solo bajo .hoom/: no ensucia el arbol ni va
	write(t, root, ".hoom/notas-locales.txt", "NOTA-HOOM-SIN-COMMITEAR\n")
	raLimpio(t, "CA-401", root)

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
		t.Fatalf("CA-401: el archivo nuevo con nombre no ASCII va con su nombre (crudo o citado por git):\n%s", diff)
	}
	for _, quiero := range []string{
		"diff --git a/committed.go b/committed.go", "-var Base = 1", "+var Base = 2 // commiteado en la rama",
		"diff --git a/app.go b/app.go", "+func EnElSegundoCommit() {}",
		"+++ b/nuevo.go", "+// archivo nuevo commiteado ñandú",
		"+// contenido-unico-ñu",
		"imagen.bin", "nuevo.bin", "Binary files",
	} {
		if !strings.Contains(diff, quiero) {
			t.Fatalf("CA-401: el diff del candidato trae %q:\n%s", quiero, diff)
		}
	}
	if !strings.Contains(raSeccion(diff, "diff --git a/nuevo.go b/nuevo.go"), "new file mode") {
		t.Fatalf("CA-401: un archivo nuevo va como parche de archivo nuevo:\n%s", diff)
	}
	for _, nunca := range []string{"MARCA-BINARIA", "GIT binary patch", "SPEC-NO-VA-EN-EL-DIFF", "NOTA-HOOM-FUERA-DEL-DIFF",
		"NOTA-HOOM-SIN-COMMITEAR", "a/.hoom/", "b/.hoom/"} {
		if strings.Contains(diff, nunca) {
			t.Fatalf("CA-401: el diff no trae %q (binarios sin contenido, nada de .hoom/ fuera de .hoom/agents/):\n%s", nunca, diff)
		}
	}
	if !bytes.Equal(ev.Spec, []byte(specRama)) {
		t.Fatalf("CA-401/CA-412: Spec es el texto del spec tal cual esta en HEAD: %q", ev.Spec)
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

// CA-401 (enmienda 1, caso limite; enmienda 4: todo commiteado): una rama que
// COMMITEA el borrado de auth.go y el renombre de b.go a c.go. La evidencia
// trae el borrado con su 'deleted file' y las lineas quitadas, y el renombre
// con sus dos lados (las cabeceras 'rename from/to' de git o el par borrado +
// alta): no sale de git.ChangedFiles, que deja afuera los borrados
// commiteados y el origen del renombre. Sigue sin nada de .hoom/ (tampoco un
// borrado commiteado ahi), con el archivo nuevo de nombre no ASCII y el
// binario sin contenido; un archivo que solo EMPIEZA con .hoom esta fuera de
// .hoom/ y va.
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
	// otro commit de la rama: nombre no ASCII y binario
	write(t, root, "piñata/ñandú.go", "package pinata\n\n// NO-ASCII-COMMITEADO\n")
	write(t, root, "datos.bin", "\x00\x01BINARIO-SIN-CONTENIDO\x00\xff")
	raCommit(t, root, "no ascii y binario")
	// sin commitear bajo .hoom/: no cuenta ni va
	write(t, root, ".hoom/notas.txt", "NOTA-HOOM-FUERA-DEL-DIFF\n")
	raLimpio(t, "CA-401", root)

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

	for _, quiero := range []string{"+// EMPIEZA-CON-HOOM-PERO-ESTA-FUERA", "+// NO-ASCII-COMMITEADO", "datos.bin", "Binary files"} {
		if !strings.Contains(diff, quiero) {
			t.Fatalf("CA-401: la evidencia trae %q:\n%s", quiero, diff)
		}
	}
	if !raTieneNombre(diff, "piñata/ñandú.go") {
		t.Fatalf("CA-401: el archivo nuevo de nombre no ASCII va con su nombre (crudo o citado por git):\n%s", diff)
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
	ev := raEvidencia(t, "CA-401", root, "")
	if ev.Spec != nil || ev.Bytes != len(ev.Diff) || len(ev.Diff) == 0 || ev.Over {
		t.Fatalf("CA-401: sin spec, Spec nil y Bytes = len(Diff) > 0: %+v", ev)
	}
	if ev.SHA256 != raSHA(ev.Diff, nil) {
		t.Fatalf("CA-401: sin spec el SHA256 es el del diff: %q", ev.SHA256)
	}
	if !strings.Contains(string(ev.Diff), "+func Nuevo() {}") {
		t.Fatalf("CA-401: el diff trae el cambio commiteado en la rama:\n%s", ev.Diff)
	}
}

// CA-401: el diff sale del MERGE-BASE, no de la punta de la base: lo que la
// base hizo despues de abrir la rama no aparece (ni revertido). Enmienda 4:
// los dos cambios de la rama estan commiteados.
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
	write(t, root, "app.go", "package app\n\n// linea base\n// cambio de la rama en su segundo commit\n")
	raCommit(t, root, "segundo commit de la rama")

	ev := raEvidencia(t, "CA-401", root, "")
	diff := string(ev.Diff)
	if !strings.Contains(diff, "+var EnLaRama = true") || !strings.Contains(diff, "+// cambio de la rama en su segundo commit") {
		t.Fatalf("CA-401: el diff trae los dos commits de la rama:\n%s", diff)
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
	raCommit(t, root, "spec")
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

// CA-401 (enmienda 4): "todo lo que esta fuera de .hoom/ mas .hoom/agents/ (el
// contrato de los roles es parte del cambio)". Una rama que commitea un
// cambio en .hoom/agents/04-writer.md, un contrato nuevo y el borrado de
// otro: los tres van en la evidencia, como los escribe git. No van: un
// directorio que solo EMPIEZA con 'agents' (.hoom/agents-viejo/), el spec,
// los hallazgos ni el resto de .hoom/, ni una edicion sin commitear de
// .hoom/agents/ (que tampoco ensucia el arbol).
func TestCA401_CambioCommiteadoEnHoomAgentsVaEnLaEvidencia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	write(t, root, ".hoom/agents/04-writer.md", "# Writer\n\nlinea base del writer\n")
	write(t, root, ".hoom/agents/07-characterizer.md", "# Characterizer\n\nCONTRATO-QUE-LA-RAMA-BORRA\n")
	write(t, root, ".hoom/agents-viejo/nota.md", "nota base\n")
	write(t, root, ".hoom/specs/s.md", "# s\n\nbase\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "base con contratos")
	raCambio(t, root)
	write(t, root, ".hoom/agents/04-writer.md", "# Writer\n\nlinea de la rama: CONTRATO-CAMBIADO-EN-LA-RAMA\n")
	write(t, root, ".hoom/agents/99-nuevo.md", "# Nuevo\n\nAGENTE-NUEVO-EN-LA-RAMA\n")
	git(t, root, "rm", "-q", ".hoom/agents/07-characterizer.md")
	write(t, root, ".hoom/agents-viejo/nota.md", "FUERA-DE-AGENTS-POR-PREFIJO\n")
	write(t, root, ".hoom/specs/s.md", "# s\n\nSPEC-DE-LA-RAMA-NO-VA\n")
	write(t, root, ".hoom/findings/20260926T000000_aaaaaa.json", `{"id":"20260926T000000_aaaaaa","created_at":"2026-09-26T00:00:00Z",`+
		`"severity":"low","lens":"risk","file":"app.go","description":"HALLAZGO-NO-VA","author":"reviewer@codex"}`+"\n")
	write(t, root, ".hoom/intake/nota.md", "INTAKE-NO-VA\n")
	raCommit(t, root, "la rama toca .hoom/agents/ y el resto de .hoom/")
	write(t, root, ".hoom/agents/04-writer.md", "# Writer\n\nEDICION-SIN-COMMITEAR-DE-AGENTS\n")
	raLimpio(t, "CA-401", root)

	ev := raEvidencia(t, "CA-401", root, "")
	diff := string(ev.Diff)
	w := raSeccion(diff, "diff --git a/.hoom/agents/04-writer.md b/.hoom/agents/04-writer.md")
	if !strings.Contains(w, "-linea base del writer") || !strings.Contains(w, "+linea de la rama: CONTRATO-CAMBIADO-EN-LA-RAMA") {
		t.Fatalf("CA-401: el cambio commiteado en .hoom/agents/04-writer.md va en la evidencia con sus dos lados:\n%s", diff)
	}
	if n := raSeccion(diff, "diff --git a/.hoom/agents/99-nuevo.md b/.hoom/agents/99-nuevo.md"); !strings.Contains(n, "new file mode") ||
		!strings.Contains(n, "+AGENTE-NUEVO-EN-LA-RAMA") {
		t.Fatalf("CA-401: un contrato nuevo commiteado en .hoom/agents/ va como archivo nuevo:\n%s", diff)
	}
	if d := raSeccion(diff, "diff --git a/.hoom/agents/07-characterizer.md b/.hoom/agents/07-characterizer.md"); !strings.Contains(d, "deleted file mode") ||
		!strings.Contains(d, "-CONTRATO-QUE-LA-RAMA-BORRA") {
		t.Fatalf("CA-401: un contrato borrado en la rama va con su 'deleted file mode':\n%s", diff)
	}
	if !strings.Contains(diff, "+func Nuevo() {}") {
		t.Fatalf("CA-401: la evidencia trae el cambio de codigo:\n%s", diff)
	}
	for _, nunca := range []string{"agents-viejo", "FUERA-DE-AGENTS-POR-PREFIJO", "SPEC-DE-LA-RAMA-NO-VA", "HALLAZGO-NO-VA",
		"INTAKE-NO-VA", "EDICION-SIN-COMMITEAR-DE-AGENTS", "a/.hoom/specs", "b/.hoom/findings", "b/.hoom/intake"} {
		if strings.Contains(diff, nunca) {
			t.Fatalf("CA-401: de .hoom/ solo va lo commiteado en .hoom/agents/: la evidencia no trae %q:\n%s", nunca, diff)
		}
	}

	// y la pasada recibe esa misma evidencia
	cx := raInstalar(t, bin, "codex", "")
	res, out := raRevisar(t, "CA-401", root, Options{Provider: "codex", Lens: "risk"})
	if res.Status != "revisado" || cx.veces() != 1 || res.EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("CA-401: la review corre con esa evidencia: %+v\n%s", res, out)
	}
	if p := cx.pedido(t, 1); !strings.Contains(p, raBloque(t, ev, "", false)) || strings.Contains(p, "EDICION-SIN-COMMITEAR-DE-AGENTS") {
		t.Fatalf("CA-401: el pedido trae la evidencia con .hoom/agents/ y nada sin commitear:\n%s", p)
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
	raCommit(t, root, "relleno")
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
	raCommit(t, root2, "relleno")
	res, out = raRevisar(t, "CA-402", root2, Options{Provider: "codex"})
	if res.Status != "revisado" || cx.veces() != 1 {
		t.Fatalf("CA-402: bajo el tope por defecto la misma evidencia corre: %+v\n%s", res, out)
	}
}

// CA-402: exactamente en el tope Evidence no pone Over y la review corre; un
// byte mas, Over y la negativa con el tope en KiB. Cada relleno se commitea
// en la rama (enmienda 4).
func TestCA402_IgualAlTopeCorreYUnByteMasNo(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  max_evidence_kib: 2\n")
	cx := raInstalar(t, bin, "codex", "")
	relleno := func(k int) {
		write(t, root, "relleno.go", "// "+strings.Repeat("x", k)+"\n")
		raCommit(t, root, fmt.Sprintf("relleno de %d", k))
	}

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
	raCommit(t, root, "spec")
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
	raCommit(t, root, "grande")
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
// Enmienda 4: la documentacion esta commiteada (el arbol limpio no decide
// nada aca).
func TestCA402_CeroLentesNoArmaEvidencia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  max_evidence_kib: 1\n")
	write(t, root, "README.md", strings.Repeat("documentacion larga\n", 400))
	raCommit(t, root, "solo documentacion")
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
	raCommit(t, root, "relleno")
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
//
// Enmienda 4: el spec se lee de HEAD. Su entrada en HEAD tiene que ser un
// archivo regular (no un symlink, a donde sea, ni un submodulo): si no,
// `el spec <ruta> no es un archivo del arbol`, sin leer nada. Lo que no esta
// en HEAD (un spec sin commitear, tambien un symlink sin commitear) es un
// spec que no existe: la review sigue sin el.

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

// raSpecsQueNoSonDelArbol arma, cada uno en su repo, un spec cuya entrada en
// HEAD existe pero no es un archivo regular: se commitea en main, y la rama
// (raCambio, despues) la hereda. Devuelve la ruta que se pasa como --spec.
// Re-expresa (enmienda 4) los casos sin commitear de antes, y el spec symlink
// a un archivo del arbol, que antes se resolvia y leia.
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
	{"symlink-commiteado-absoluto-a-un-archivo-de-afuera", func(t *testing.T, root string) string {
		const spec = ".hoom/specs/x.md"
		raSymlink(t, root, spec, raFuera(t, "fuera.md", "# SECRETO-FUERA-DEL-ARBOL\n"))
		git(t, root, "add", spec)
		git(t, root, "commit", "-q", "-m", "spec symlink absoluto a un archivo de afuera")
		return spec
	}},
	{"symlink-commiteado-a-un-directorio-del-arbol", func(t *testing.T, root string) string {
		const spec = ".hoom/specs/x.md"
		write(t, root, "sub/a.go", "package sub\n")
		raSymlink(t, root, spec, "../../sub")
		git(t, root, "add", "-A")
		git(t, root, "commit", "-q", "-m", "spec symlink a un directorio")
		return spec
	}},
	{"symlink-commiteado-a-un-archivo-del-arbol", func(t *testing.T, root string) string {
		const spec = ".hoom/specs/alias.md"
		write(t, root, ".hoom/specs/x.md", "# Spec x\n\ntexto del spec real\n")
		raSymlink(t, root, spec, "x.md")
		git(t, root, "add", "-A")
		git(t, root, "commit", "-q", "-m", "spec symlink a otro spec del arbol")
		return spec
	}},
	{"submodulo-commiteado", func(t *testing.T, root string) string {
		const spec = ".hoom/specs/x.md"
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = root
		sha, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		git(t, root, "update-index", "--add", "--cacheinfo", "160000,"+strings.TrimSpace(string(sha))+","+spec)
		git(t, root, "commit", "-q", "-m", "spec que es un submodulo")
		// como un submodulo sin inicializar: un directorio vacio
		if err := os.MkdirAll(filepath.Join(root, spec), 0o755); err != nil {
			t.Fatal(err)
		}
		return spec
	}},
}

// CA-412: un spec cuya entrada en HEAD no es un archivo regular (symlink
// commiteado a un archivo de afuera, relativo o absoluto, a un directorio o
// a otro spec del arbol; un submodulo) hace que Evidence devuelva 'el spec
// <ruta> no es un archivo del arbol', sin leer su destino.
func TestCA412_SpecQueNoEsArchivoDelArbolEsError(t *testing.T) {
	for _, c := range raSpecsQueNoSonDelArbol {
		t.Run(c.nombre, func(t *testing.T) {
			root := raRepo(t, "")
			spec := c.armar(t, root)
			raCambio(t, root)
			raLimpio(t, "CA-412", root)
			ev, err, _ := raEvidenceConReloj(t, "CA-412", 60*time.Second, root, "main", spec, raTopeGrande)
			if err == nil || !strings.Contains(err.Error(), raNoEsDelArbol(spec)) {
				t.Fatalf("CA-412: Evidence devuelve %q: %v (spec leido: %q)", raNoEsDelArbol(spec), err, ev.Spec)
			}
			if bytes.Contains(ev.Spec, []byte("SECRETO-FUERA-DEL-ARBOL")) || bytes.Contains(ev.Spec, []byte("texto del spec real")) {
				t.Fatalf("CA-412: el destino del spec no se lee: %q", ev.Spec)
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
			raLimpio(t, "CA-412", root)
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

// raSpecsSinCommitear arma, cada uno en su repo, un spec que esta en el
// arbol de trabajo pero NO en HEAD (bajo .hoom/, asi que tampoco ensucia el
// arbol). Devuelve la ruta y lo que no puede aparecer en ningun lado.
var raSpecsSinCommitear = []struct {
	nombre string
	armar  func(t *testing.T, root string) (spec, oculto string)
}{
	{"archivo-sin-commitear", func(t *testing.T, root string) (string, string) {
		write(t, root, ".hoom/specs/x.md", "# Spec x\n\nSPEC-SIN-COMMITEAR-CA412\n")
		return ".hoom/specs/x.md", "SPEC-SIN-COMMITEAR-CA412"
	}},
	{"symlink-sin-commitear-a-un-archivo-de-afuera", func(t *testing.T, root string) (string, string) {
		raSymlink(t, root, ".hoom/specs/x.md", raFuera(t, "fuera.md", "# SECRETO-FUERA-DEL-ARBOL\n"))
		return ".hoom/specs/x.md", "SECRETO-FUERA-DEL-ARBOL"
	}},
	{"symlink-sin-commitear-a-un-directorio-del-arbol", func(t *testing.T, root string) (string, string) {
		write(t, root, "sub/a.go", "package sub\n\n// CONTENIDO-DE-SUB\n")
		git(t, root, "add", "sub/a.go")
		git(t, root, "commit", "-q", "-m", "sub")
		raSymlink(t, root, ".hoom/specs/x.md", "../../sub")
		return ".hoom/specs/x.md", ""
	}},
}

// CA-412 (enmienda 4, "una edicion sin commitear del spec no entra"): un spec
// que solo esta en el arbol de trabajo (un archivo, o un symlink a afuera o a
// un directorio, sin commitear) no esta en HEAD: Evidence no falla, Spec es
// nil y nada de su contenido se lee; la review sigue con 'Spec: <ruta> (no
// existe en este arbol)' y sin bloque de spec (CA-334).
func TestCA412_SpecSinCommitearNoEstaEnHEAD(t *testing.T) {
	for _, c := range raSpecsSinCommitear {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, "")
			raCambio(t, root)
			spec, oculto := c.armar(t, root)
			raLimpio(t, "CA-412", root)

			ev, err, _ := raEvidenceConReloj(t, "CA-412", 60*time.Second, root, "main", spec, raTopeGrande)
			if err != nil || ev.Over {
				t.Fatalf("CA-412: un spec que no esta en HEAD no es error: %v (Over %v)", err, ev.Over)
			}
			if ev.Spec != nil || ev.Bytes != len(ev.Diff) || ev.SHA256 != raSHA(ev.Diff, nil) || len(ev.Diff) == 0 {
				t.Fatalf("CA-412: sin el spec en HEAD, Spec nil y la evidencia es la del diff: spec %q, %d bytes vs diff %d", ev.Spec, ev.Bytes, len(ev.Diff))
			}

			cx := raInstalar(t, bin, "codex", "")
			g := gitx.Snapshot(root, "main")
			res, out := raRevisar(t, "CA-412", root, Options{Provider: "codex", Lens: "risk", Spec: spec})
			if res.Status != "revisado" || cx.veces() != 1 {
				t.Fatalf("CA-412: con un spec que no esta en HEAD la review sigue: %+v\n%s", res, out)
			}
			p := cx.pedido(t, 1)
			raRevisarPedido(t, p, "risk", "codex", g, ev, spec, false, "No hay veredicto vigente: la review no reemplaza a 'hoom verify'.")
			if oculto != "" && (strings.Contains(p, oculto) || strings.Contains(out, oculto)) {
				t.Fatalf("CA-412: lo que no esta en HEAD no aparece en el pedido ni en la salida: %q", oculto)
			}
		})
	}
}

// CA-412 (enmienda 4): una edicion sin commitear de un spec que SI esta en
// HEAD no entra: la evidencia y el pedido llevan el texto de HEAD, tambien
// si el spec se borro del arbol de trabajo o se cambio por un symlink a
// afuera. Nada de eso ensucia el arbol (esta bajo .hoom/).
func TestCA412_EdicionSinCommitearDelSpecNoEntra(t *testing.T) {
	const spec = ".hoom/specs/x.md"
	const commiteado = "# Spec x\n\n- CA-1: la version commiteada del spec.\n"
	casos := []struct {
		nombre string
		tocar  func(t *testing.T, root string) string // devuelve lo que no puede aparecer
	}{
		{"editado-sin-commitear", func(t *testing.T, root string) string {
			write(t, root, spec, "# Spec x\n\n- CA-1: EDICION-SIN-COMMITEAR-DEL-SPEC.\n")
			return "EDICION-SIN-COMMITEAR-DEL-SPEC"
		}},
		{"borrado-sin-commitear", func(t *testing.T, root string) string {
			if err := os.Remove(filepath.Join(root, spec)); err != nil {
				t.Fatal(err)
			}
			return ""
		}},
		{"cambiado-por-un-symlink-de-afuera-sin-commitear", func(t *testing.T, root string) string {
			if err := os.Remove(filepath.Join(root, spec)); err != nil {
				t.Fatal(err)
			}
			raSymlink(t, root, spec, raFuera(t, "fuera.md", "# SECRETO-FUERA-DEL-ARBOL\n"))
			return "SECRETO-FUERA-DEL-ARBOL"
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, "")
			raCambio(t, root)
			write(t, root, spec, commiteado)
			raCommit(t, root, "spec")
			oculto := c.tocar(t, root)
			raLimpio(t, "CA-412", root)

			ev, err, _ := raEvidenceConReloj(t, "CA-412", 60*time.Second, root, "main", spec, raTopeGrande)
			if err != nil || string(ev.Spec) != commiteado {
				t.Fatalf("CA-412: el spec se lee de HEAD, no del arbol de trabajo: %v %q", err, ev.Spec)
			}
			cx := raInstalar(t, bin, "codex", "")
			g := gitx.Snapshot(root, "main")
			res, out := raRevisar(t, "CA-412", root, Options{Provider: "codex", Lens: "risk", Spec: spec})
			if res.Status != "revisado" || cx.veces() != 1 {
				t.Fatalf("CA-412: la review corre con el spec de HEAD: %+v\n%s", res, out)
			}
			p := cx.pedido(t, 1)
			raRevisarPedido(t, p, "risk", "codex", g, ev, spec, true, "No hay veredicto vigente: la review no reemplaza a 'hoom verify'.")
			if oculto != "" && strings.Contains(p, oculto) {
				t.Fatalf("CA-412: la edicion sin commitear del spec no entra en el pedido: %q\n%s", oculto, p)
			}
		})
	}
}

// CA-412 (caso limite): una ruta de spec que sale del arbol con '..' no es
// una entrada de HEAD: Evidence no lee el archivo de afuera. O se niega con
// 'el spec <ruta> no es un archivo del arbol', o sigue sin spec (Spec nil);
// nunca otro error ni el contenido de afuera.
func TestCA412_RutaQueSaleDelArbolNoSeLee(t *testing.T) {
	root := raRepo(t, "")
	raCambio(t, root)
	rel, err := filepath.Rel(root, raFuera(t, "fuera.md", "# SECRETO-FUERA-DEL-ARBOL\n"))
	if err != nil || !strings.HasPrefix(rel, "..") {
		t.Fatalf("CA-412: fixture: la ruta sale del arbol: %q %v", rel, err)
	}
	spec := filepath.ToSlash(rel)
	ev, err, _ := raEvidenceConReloj(t, "CA-412", 60*time.Second, root, "main", spec, raTopeGrande)
	if bytes.Contains(ev.Spec, []byte("SECRETO-FUERA-DEL-ARBOL")) || bytes.Contains(ev.Diff, []byte("SECRETO-FUERA-DEL-ARBOL")) {
		t.Fatalf("CA-412: el archivo de afuera no se lee: %q", ev.Spec)
	}
	if err != nil && !strings.Contains(err.Error(), raNoEsDelArbol(spec)) {
		t.Fatalf("CA-412: una ruta que sale del arbol se niega con %q (o sigue sin spec): %v", raNoEsDelArbol(spec), err)
	}
	if err == nil && ev.Spec != nil {
		t.Fatalf("CA-412: sin error, una ruta que sale del arbol no tiene spec: %q", ev.Spec)
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
	r := map[string]any{"over": ev.Over, "bytes": ev.Bytes, "spec_nil": ev.Spec == nil}
	if err != nil {
		r["err"] = err.Error()
	}
	raw, _ := json.Marshal(r)
	fmt.Println("RA-CA412 " + string(raw))
}

// CA-412: un spec symlink a /dev/zero nunca se lee: vuelve enseguida y sin
// asignar memoria. Commiteado (su entrada en HEAD es un symlink), Evidence
// devuelve 'el spec <ruta> no es un archivo del arbol'; sin commitear
// (enmienda 4: no esta en HEAD), Evidence sigue sin spec. Corre en un
// proceso hijo con reloj de 60 s y un vigilante de 256 MiB.
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
			raLimpio(t, "CA-412", root)

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
				Err     string `json:"err"`
				Over    bool   `json:"over"`
				SpecNil bool   `json:"spec_nil"`
			}
			if err := json.Unmarshal([]byte(m[1]), &r); err != nil {
				t.Fatalf("CA-412: resultado ilegible del hijo: %v %s", err, m[1])
			}
			if commitear && !strings.Contains(r.Err, raNoEsDelArbol(spec)) {
				t.Fatalf("CA-412: Evidence devuelve %q con un spec symlink a /dev/zero commiteado, devolvio %s", raNoEsDelArbol(spec), m[1])
			}
			if !commitear && (r.Err != "" || r.Over || !r.SpecNil) {
				t.Fatalf("CA-412: un spec symlink a /dev/zero sin commitear no esta en HEAD: sin error, sin Over y sin spec; fue %s", m[1])
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
	raRevisarPedido(t, cx.pedido(t, 1), "risk", "codex", g, ev, spec, false,
		"No hay veredicto vigente: la review no reemplaza a 'hoom verify'.")
}

// CA-412 (caso limite): un spec de 0 bytes (commiteado: enmienda 4) existe:
// la linea Spec: sin la nota, y su bloque va vacio (el marcador del spec
// seguido del del diff).
func TestCA412_SpecVacioLlevaSuBloqueVacio(t *testing.T) {
	const spec = ".hoom/specs/vacio.md"
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	write(t, root, spec, "")
	raCommit(t, root, "spec vacio")
	cx := raInstalar(t, bin, "codex", "")

	ev, err := Evidence(root, "main", spec, raTopeGrande)
	if err != nil || ev.Over || ev.Spec == nil || len(ev.Spec) != 0 || ev.Bytes != len(ev.Diff) || ev.SHA256 != raSHA(ev.Diff, nil) {
		t.Fatalf("CA-412: un spec vacio existe (Spec vacio, no nil) y no suma bytes: %v, Over %v, spec %q (nil %v), %d bytes", err, ev.Over, ev.Spec, ev.Spec == nil, ev.Bytes)
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
	raRevisarPedido(t, p, "risk", "codex", g, ev, spec, true, "No hay veredicto vigente: la review no reemplaza a 'hoom verify'.")
}

// ---------------------------------------------------------------- CA-413

// CA-413 (re-expresado por la enmienda 4: antes el archivo estaba sin
// rastrear): con un tope de 1 KiB y un archivo de 64 MiB COMMITEADO en la
// rama, Evidence vuelve enseguida con Over, Diff y Spec vacios, SHA256 vacio
// y Bytes entre el tope y el tope + 64 KiB. Y deja de leer de verdad:
// mientras arma la evidencia el proceso no asigna ni la cuarta parte del
// archivo.
func TestCA413_ArchivoCommiteadoDe64MiBConTopeDe1KiB(t *testing.T) {
	root := raRepo(t, "")
	linea := strings.Repeat("x", 63) + "\n"
	raRama(t, root)
	if err := os.WriteFile(filepath.Join(root, "enorme.txt"), bytes.Repeat([]byte(linea), (64<<20)/len(linea)), 0o644); err != nil {
		t.Fatal(err)
	}
	raCommit(t, root, "archivo de 64 MiB")
	raLimpio(t, "CA-413", root)
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
// 32 MiB (commiteado: se lee de HEAD) con tope de 1 KiB es Over, con lo
// leido acotado, sin leerlo entero.
func TestCA413_SpecEnormeConTopeDe1KiB(t *testing.T) {
	root := raRepo(t, "")
	raCambio(t, root)
	const spec = ".hoom/specs/grande.md"
	linea := "criterio de relleno de un spec enorme, sesenta y cuatro bytes\n"
	write(t, root, spec, strings.Repeat(linea, (32<<20)/len(linea)))
	raCommit(t, root, "spec enorme")
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

// CA-414 (enmienda 4: merge-base, `git status` o `git diff`): si falla git
// merge-base (una base que no existe, un clon shallow sin el merge-base),
// git status o git diff, Evidence devuelve el error y `hoom review` no lanza
// ninguna pasada ni escribe registro: no revisa en silencio otro parche. Los
// cambios estan commiteados: el arbol limpio no es lo que frena.
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
			raLimpio(t, "CA-414", root)
			return root, "rama-que-no-existe"
		}},
		{"clon-shallow-sin-merge-base", func(t *testing.T, bin string) (string, string) {
			clon := raClonShallow(t)
			write(t, clon, "rama.go", "package app\n\nvar Rama = 3 // commiteado en el clon\n")
			raCommit(t, clon, "rama 3")
			raLimpio(t, "CA-414", clon)
			return clon, "main"
		}},
		{"status-falla", func(t *testing.T, bin string) (string, string) {
			root := raRepo(t, "")
			raCambio(t, root)
			raLimpio(t, "CA-414", root)
			raGitRoto(t, bin, real, "status")
			return root, "main"
		}},
		{"diff-falla", func(t *testing.T, bin string) (string, string) {
			root := raRepo(t, "")
			raCambio(t, root)
			raLimpio(t, "CA-414", root)
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
// Enmienda 4: con el arbol de trabajo sucio fuera de .hoom/ (un rastreado
// modificado, algo en el indice, un borrado, un sin rastrear no ignorado),
// Evidence devuelve `la review revisa solo lo commiteado y hay cambios sin
// commitear (<ruta>): commitealos antes de revisar` con la primera ruta, sin
// armar la evidencia; `hoom review` no lanza ninguna pasada ni escribe
// registro; y nada sin commitear aparece en la salida. Lo ignorado y lo que
// esta bajo .hoom/ no cuentan. Re-expresa la regla del .gitignore de la
// enmienda 3 y las guardas de los no rastreados raros (FIFO, symlink): todos
// esos arboles son ahora arboles sucios.

// raSecreto es lo que guarda un .env local: no puede aparecer en ningun lado.
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

// raRamaGitignore abre la rama feature sobre la base con .gitignore y le
// commitea un cambio de codigo (una lente).
func raRamaGitignore(t *testing.T) string {
	t.Helper()
	root := raBaseConGitignore(t)
	git(t, root, "checkout", "-q", "-b", "feature")
	raCambio(t, root)
	return root
}

// raConFIFO crea un FIFO en p y, al terminar el test, lo destraba.
func raConFIFO(t *testing.T, p string) {
	t.Helper()
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("sin mkfifo")
	}
	if out, err := exec.Command(mkfifo, p).CombinedOutput(); err != nil {
		t.Fatalf("fixture: mkfifo: %v %s", err, out)
	}
	t.Cleanup(func() { raDestrabarFIFO(t, p) })
}

// raCasoSucio es un arbol con un cambio commiteado (una lente) y algo sin
// commitear fuera de .hoom/. rutas son las que la negativa puede nombrar (la
// primera que liste git; un directorio nuevo sin rastrear puede nombrarse
// entero); oculto es lo que no puede aparecer en ningun lado ("" = nada).
type raCasoSucio struct {
	nombre string
	armar  func(t *testing.T) (root string, rutas []string, oculto string)
}

var raCasosSucios = []raCasoSucio{
	{"rastreado-modificado", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "app.go", "package app\n\n// SIN-COMMITEAR-416-MODIFICADO\n")
		return root, []string{"app.go"}, "SIN-COMMITEAR-416-MODIFICADO"
	}},
	{"rastreado-modificado-y-en-el-indice", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "app.go", "package app\n\n// SIN-COMMITEAR-416-EN-EL-INDICE\n")
		git(t, root, "add", "app.go")
		return root, []string{"app.go"}, "SIN-COMMITEAR-416-EN-EL-INDICE"
	}},
	{"nuevo-en-el-indice", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "nuevo.go", "package app\n\n// SIN-COMMITEAR-416-NUEVO-EN-EL-INDICE\n")
		git(t, root, "add", "nuevo.go")
		return root, []string{"nuevo.go"}, "SIN-COMMITEAR-416-NUEVO-EN-EL-INDICE"
	}},
	{"rastreado-borrado", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "otro.go", "package app\n\n// OTRO-COMMITEADO\n")
		raCommit(t, root, "otro.go")
		if err := os.Remove(filepath.Join(root, "otro.go")); err != nil {
			t.Fatal(err)
		}
		return root, []string{"otro.go"}, ""
	}},
	{"borrado-en-el-indice", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "otro.go", "package app\n\n// OTRO-COMMITEADO\n")
		raCommit(t, root, "otro.go")
		git(t, root, "rm", "-q", "otro.go")
		return root, []string{"otro.go"}, ""
	}},
	{"sin-rastrear", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "nuevo.go", "package app\n\n// SIN-COMMITEAR-416-SIN-RASTREAR\n")
		return root, []string{"nuevo.go"}, "SIN-COMMITEAR-416-SIN-RASTREAR"
	}},
	{"binario-sin-rastrear", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "datos.bin", "\x00\x01MARCA-BINARIA-SIN-COMMITEAR-416\x00\xff")
		return root, []string{"datos.bin"}, "MARCA-BINARIA-SIN-COMMITEAR-416"
	}},
	{"nombre-no-ascii-sin-rastrear", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "piñata/ñandú.go", "package pinata\n\n// SIN-COMMITEAR-416-NO-ASCII\n")
		return root, []string{"piñata/ñandú.go", "piñata"}, "SIN-COMMITEAR-416-NO-ASCII"
	}},
	{"directorio-nuevo-sin-rastrear", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "dir nuevo/a.go", "package dir\n\n// SIN-COMMITEAR-416-DIRECTORIO\n")
		return root, []string{"dir nuevo/a.go", "dir nuevo"}, "SIN-COMMITEAR-416-DIRECTORIO"
	}},
	{"rastreado-cambiado-por-un-symlink-de-afuera", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		if err := os.Remove(filepath.Join(root, "app.go")); err != nil {
			t.Fatal(err)
		}
		raSymlink(t, root, "app.go", raFuera(t, "fuera.go", "// SECRETO-DE-AFUERA-POR-SYMLINK-416\n"))
		return root, []string{"app.go"}, "SECRETO-DE-AFUERA-POR-SYMLINK-416"
	}},
	// antes TestCA401_NoRastreadoSymlinkAUnArchivoDeAfueraNoSeLee
	{"symlink-sin-rastrear-a-un-archivo-de-afuera", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		raSymlink(t, root, "enlace-afuera", raFuera(t, "fuera.txt", "SECRETO-FUERA-DEL-ARBOL-POR-SYMLINK\n"))
		return root, []string{"enlace-afuera"}, "SECRETO-FUERA-DEL-ARBOL-POR-SYMLINK"
	}},
	// antes TestCA401_NoRastreadoSymlinkAUnFIFONoCuelga: un symlink sin
	// rastrear a un FIFO (fuera del arbol o dentro, que git no lista) es un
	// arbol sucio; la negativa vuelve enseguida, sin abrir el FIFO
	{"symlink-sin-rastrear-a-un-fifo-de-afuera", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		fifo := filepath.Join(dir, "tuberia")
		raConFIFO(t, fifo)
		raSymlink(t, root, "enlace-a-fifo", fifo)
		return root, []string{"enlace-a-fifo"}, ""
	}},
	{"symlink-sin-rastrear-a-un-fifo-del-arbol", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		raConFIFO(t, filepath.Join(root, "tuberia"))
		raSymlink(t, root, "enlace-a-fifo", "tuberia")
		return root, []string{"enlace-a-fifo"}, ""
	}},
	{"varios-sucios", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		raCambio(t, root)
		write(t, root, "otro.go", "package app\n")
		raCommit(t, root, "otro.go")
		write(t, root, "app.go", "package app\n\n// SIN-COMMITEAR-416-VARIOS\n")
		write(t, root, "zz-nuevo.go", "package app\n\n// SIN-COMMITEAR-416-VARIOS-NUEVO\n")
		if err := os.Remove(filepath.Join(root, "otro.go")); err != nil {
			t.Fatal(err)
		}
		return root, []string{"app.go", "otro.go", "zz-nuevo.go"}, "SIN-COMMITEAR-416-VARIOS"
	}},
	// los casos de la regla del .gitignore de la enmienda 3: el .env que el
	// cambio destapa esta sin rastrear y no ignorado, asi que el arbol esta
	// sucio y el secreto no sale nunca
	{"env-que-la-rama-destapo-en-su-gitignore-commiteado", func(t *testing.T) (string, []string, string) {
		root := raRamaGitignore(t)
		write(t, root, ".gitignore", "*.log\n")
		git(t, root, "commit", "-q", "-am", "la rama saca .env del .gitignore")
		write(t, root, ".env", raSecreto+"\n")
		return root, []string{".env"}, raSecreto
	}},
	{"gitignore-sin-commitear-que-destapa-el-env", func(t *testing.T) (string, []string, string) {
		root := raRamaGitignore(t)
		write(t, root, ".gitignore", "*.log\n")
		write(t, root, ".env", raSecreto+"\n")
		return root, []string{".gitignore", ".env"}, raSecreto
	}},
	{"gitignore-borrado-en-un-commit-de-la-rama", func(t *testing.T) (string, []string, string) {
		root := raRamaGitignore(t)
		git(t, root, "rm", "-q", ".gitignore")
		git(t, root, "commit", "-q", "-m", "la rama borra el .gitignore")
		write(t, root, ".env", raSecreto+"\n")
		return root, []string{".env"}, raSecreto
	}},
	{"gitignore-borrado-sin-commitear", func(t *testing.T) (string, []string, string) {
		root := raRamaGitignore(t)
		if err := os.Remove(filepath.Join(root, ".gitignore")); err != nil {
			t.Fatal(err)
		}
		write(t, root, ".env", raSecreto+"\n")
		return root, []string{".gitignore", ".env"}, raSecreto
	}},
	{"gitignore-de-un-subdirectorio-modificado-sin-commitear", func(t *testing.T) (string, []string, string) {
		root := raRamaGitignore(t)
		write(t, root, "sub/.gitignore", "*.tmp\n!clave.log\n")
		write(t, root, "sub/clave.log", raSecreto+"\n")
		return root, []string{"sub/.gitignore", "sub/clave.log"}, raSecreto
	}},
	{"gitignore-nuevo-sin-rastrear-en-un-subdirectorio", func(t *testing.T) (string, []string, string) {
		root := raRamaGitignore(t)
		write(t, root, "otro/.gitignore", "!clave.log\n")
		write(t, root, "otro/clave.log", raSecreto+"\n")
		return root, []string{"otro/.gitignore", "otro/clave.log", "otro"}, raSecreto
	}},
	{"gitignore-nuevo-commiteado-en-un-subdirectorio", func(t *testing.T) (string, []string, string) {
		root := raRamaGitignore(t)
		write(t, root, "otro/.gitignore", "!clave.log\n")
		git(t, root, "add", "otro/.gitignore")
		git(t, root, "commit", "-q", "-m", "la rama destapa otro/clave.log")
		write(t, root, "otro/clave.log", raSecreto+"\n")
		return root, []string{"otro/clave.log"}, raSecreto
	}},
	{"gitignore-modificado-sin-commitear-sin-nada-sin-rastrear", func(t *testing.T) (string, []string, string) {
		root := raRamaGitignore(t)
		write(t, root, ".gitignore", ".env\n*.log\n*.bak\n# LINEA-NUEVA-SIN-COMMITEAR-416\n")
		write(t, root, ".env", raSecreto+"\n") // sigue ignorado
		return root, []string{".gitignore"}, raSecreto
	}},
	{"gitignore-que-ignora-mas-con-un-no-rastreado-cualquiera", func(t *testing.T) (string, []string, string) {
		root := raRamaGitignore(t)
		write(t, root, ".gitignore", ".env\n*.log\n*.bak\n")
		git(t, root, "commit", "-q", "-am", "la rama ignora *.bak")
		write(t, root, ".env", raSecreto+"\n") // ignorado: no es el que ensucia
		write(t, root, "nuevo.go", "package app\n\n// SIN-COMMITEAR-416-CUALQUIERA\n")
		return root, []string{"nuevo.go"}, raSecreto
	}},
	// antes TestCA416_GitignoreNuevoQueSeIgnoraASiMismo: git no lista el
	// .gitignore nuevo (se ignora a si mismo) pero si el .env que destapa
	{"gitignore-nuevo-que-se-ignora-a-si-mismo-en-un-subdirectorio", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		write(t, root, ".gitignore", "*.env\n")
		git(t, root, "add", "-A")
		git(t, root, "commit", "-q", "-m", "la base ignora *.env")
		raCambio(t, root)
		write(t, root, "sub/.gitignore", ".gitignore\n!.env\n")
		write(t, root, "sub/.env", raSecreto+"\n")
		return root, []string{"sub/.env", "sub"}, raSecreto
	}},
	{"gitignore-nuevo-que-se-ignora-a-si-mismo-en-la-raiz-sobre-info-exclude", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		write(t, root, ".git/info/exclude", "*.env\n")
		raCambio(t, root)
		write(t, root, ".gitignore", ".gitignore\n!.env\n")
		write(t, root, ".env", raSecreto+"\n")
		return root, []string{".env"}, raSecreto
	}},
	// antes la variante con-un-nuevo-go-sin-rastrear de
	// TestCA416_GitignoreEnUnDirectorioIgnoradoNoCuenta
	{"no-rastreado-junto-a-directorios-ignorados", func(t *testing.T) (string, []string, string) {
		root := raRepo(t, "")
		write(t, root, ".gitignore", "node_modules/\n.venv/\n")
		git(t, root, "add", "-A")
		git(t, root, "commit", "-q", "-m", "la base ignora node_modules/ y .venv/")
		raCambio(t, root)
		write(t, root, "node_modules/x/.gitignore", "!*\n")
		write(t, root, "node_modules/x/index.js", "// DENTRO-DE-NODE-MODULES\n")
		write(t, root, ".venv/.gitignore", "*\n")
		write(t, root, ".venv/bin/activate", "# DENTRO-DE-VENV\n")
		write(t, root, "nuevo.go", "package app\n\n// SIN-COMMITEAR-416-JUNTO-A-IGNORADOS\n")
		return root, []string{"nuevo.go"}, "SIN-COMMITEAR-416-JUNTO-A-IGNORADOS"
	}},
}

// CA-416: con el arbol sucio fuera de .hoom/ (cada forma: rastreado
// modificado, en el indice, borrado, sin rastrear no ignorado, un .env que la
// rama destapo, un symlink a un FIFO...), Evidence devuelve el error del
// contrato con la primera ruta, enseguida y sin armar la evidencia; `hoom
// review` no lanza ninguna pasada, no escribe registro, no sale con 0, dice
// por que, y nada sin commitear (ni el secreto del .env, ni lo que hay del
// otro lado de un symlink) aparece en la salida, en el error ni en .hoom/.
func TestCA416_ArbolSucioEsErrorYLaReviewNoLanzaPasadas(t *testing.T) {
	for _, c := range raCasosSucios {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root, rutas, oculto := c.armar(t)
			if len(raSuciedad(t, root)) == 0 {
				t.Fatalf("CA-416: fixture: el arbol esta sucio fuera de .hoom/")
			}

			ev, err, _ := raEvidenceConReloj(t, "CA-416", 20*time.Second, root, "main", "", raTopeGrande)
			if err == nil {
				t.Fatalf("CA-416: Evidence devuelve %q con una de %q; armo una evidencia de %d bytes:\n%s", raErrSucio("<ruta>"), rutas, ev.Bytes, ev.Diff)
			}
			if ruta, ok := raRutaSucia(err.Error()); !ok || !raRutaAceptada(ruta, rutas) {
				t.Fatalf("CA-416: Evidence devuelve %q con la primera ruta sucia (una de %q): %v", raErrSucio("<ruta>"), rutas, err)
			}
			if len(ev.Diff) != 0 || len(ev.Spec) != 0 || ev.SHA256 != "" || ev.Over {
				t.Fatalf("CA-416: sin armar la evidencia: diff %d bytes, spec %d bytes, sha256 %q, Over %v", len(ev.Diff), len(ev.Spec), ev.SHA256, ev.Over)
			}
			if oculto != "" && strings.Contains(err.Error(), oculto) {
				t.Fatalf("CA-416: el error no trae nada sin commitear (%q): %v", oculto, err)
			}

			cx := raInstalar(t, bin, "codex", "")
			res, rerr, out := raRunConReloj(t, "CA-416", 20*time.Second, root, Options{Provider: "codex", Lens: "risk"})
			msg := out
			if rerr != nil {
				msg += "\n" + rerr.Error()
			}
			if cx.veces() != 0 || len(res.Passes) != 0 || res.Status == "revisado" {
				t.Fatalf("CA-416: con el arbol sucio no se lanza ninguna pasada (codex %d): %+v\n%s", cx.veces(), res, msg)
			}
			if rerr == nil && res.ExitCode == 0 {
				t.Fatalf("CA-416: la negativa por arbol sucio no sale con 0: %+v\n%s", res, msg)
			}
			if recs, _ := Records(root); len(recs) != 0 {
				t.Fatalf("CA-416: sin pasadas no hay registro: %+v", recs)
			}
			if ruta, ok := raRutaSucia(msg); !ok || !raRutaAceptada(ruta, rutas) {
				t.Fatalf("CA-416: la review dice %q con la primera ruta sucia (una de %q):\n%s", raErrSucio("<ruta>"), rutas, msg)
			}
			if oculto != "" && strings.Contains(msg, oculto) {
				t.Fatalf("CA-416: nada sin commitear aparece en la salida ni en el error (%q):\n%s", oculto, msg)
			}
			raSinTextoEnHoom(t, "CA-416", root, oculto)
		})
	}
}

// CA-416 (control): con cambios SOLO bajo .hoom/ (un rastreado modificado, un
// sin rastrear, uno en el indice, un borrado, tambien en .hoom/agents/) o en
// archivos ignorados (por el .gitignore de la base, dentro de un directorio
// ignorado, por .git/info/exclude), el arbol no esta sucio: la evidencia se
// arma como siempre, con lo commiteado y nada de eso; la review corre y
// nada de eso llega al provider.
func TestCA416_CambiosSoloEnHoomOIgnoradosArmanLaEvidencia(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	write(t, root, ".gitignore", ".env\nbuild/\nnode_modules/\n")
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\nversion commiteada\n")
	write(t, root, ".hoom/agents/04-writer.md", "# Writer\n\ncontrato commiteado\n")
	write(t, root, ".hoom/borrar.md", "se borra sin commitear\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "base")
	raCambio(t, root)
	// bajo .hoom/
	write(t, root, ".hoom/specs/x.md", "# Spec x\n\nEDICION-SIN-COMMITEAR-DEL-SPEC\n")
	write(t, root, ".hoom/notas.txt", "NOTA-HOOM-SIN-COMMITEAR\n")
	write(t, root, ".hoom/items/y.yaml", "titulo: ITEM-EN-EL-INDICE\n")
	git(t, root, "add", ".hoom/items/y.yaml")
	if err := os.Remove(filepath.Join(root, ".hoom", "borrar.md")); err != nil {
		t.Fatal(err)
	}
	write(t, root, ".hoom/agents/04-writer.md", "# Writer\n\nCONTRATO-SIN-COMMITEAR\n")
	write(t, root, ".hoom/agents/99-nuevo.md", "# Nuevo\n\nAGENTE-SIN-RASTREAR\n")
	// ignorado
	write(t, root, ".env", raSecreto+"\n")
	write(t, root, "build/salida.bin", "\x00IGNORADO-EN-BUILD\x00")
	write(t, root, "node_modules/x/.gitignore", "!*\n")
	write(t, root, "node_modules/x/index.js", "// DENTRO-DE-NODE-MODULES\n")
	write(t, root, ".git/info/exclude", "*.local\n")
	write(t, root, "notas.local", "EXCLUIDO-POR-INFO-EXCLUDE\n")
	if s := raSuciedad(t, root); len(s) != 0 {
		t.Fatalf("CA-416: fixture: git no ve nada sin commitear fuera de .hoom/: %q", s)
	}

	ocultos := []string{"EDICION-SIN-COMMITEAR-DEL-SPEC", "NOTA-HOOM-SIN-COMMITEAR", "ITEM-EN-EL-INDICE", "CONTRATO-SIN-COMMITEAR",
		"AGENTE-SIN-RASTREAR", raSecreto, "IGNORADO-EN-BUILD", "DENTRO-DE-NODE-MODULES", "EXCLUIDO-POR-INFO-EXCLUDE"}
	ev, err, _ := raEvidenceConReloj(t, "CA-416", 60*time.Second, root, "main", ".hoom/specs/x.md", raTopeGrande)
	if err != nil || ev.Over {
		t.Fatalf("CA-416: con cambios solo en .hoom/ o ignorados la evidencia se arma como siempre: %v (Over %v)", err, ev.Over)
	}
	diff := string(ev.Diff)
	if !strings.Contains(diff, "+func Nuevo() {}") {
		t.Fatalf("CA-416: la evidencia trae el cambio commiteado:\n%s", diff)
	}
	if string(ev.Spec) != "# Spec x\n\nversion commiteada\n" {
		t.Fatalf("CA-412: el spec es el de HEAD, no la edicion sin commitear: %q", ev.Spec)
	}
	for _, o := range ocultos {
		if strings.Contains(diff, o) || strings.Contains(string(ev.Spec), o) {
			t.Fatalf("CA-416: nada sin commitear ni ignorado va en la evidencia: %q\n%s", o, diff)
		}
	}
	if ev.Bytes != len(ev.Diff)+len(ev.Spec) || ev.SHA256 != raSHA(ev.Diff, ev.Spec) {
		t.Fatalf("CA-401: Bytes y SHA256 cierran: %d, %q", ev.Bytes, ev.SHA256)
	}

	cx := raInstalar(t, bin, "codex", "")
	res, out := raRevisar(t, "CA-416", root, Options{Provider: "codex", Lens: "risk", Spec: ".hoom/specs/x.md"})
	if res.Status != "revisado" || cx.veces() != 1 || res.EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("CA-416: la review corre como siempre: codex %d, %+v\n%s", cx.veces(), res, out)
	}
	p := cx.pedido(t, 1)
	for _, o := range ocultos {
		if strings.Contains(p, o) || strings.Contains(out, o) {
			t.Fatalf("CA-416: %q no llega al provider ni a la salida", o)
		}
	}
}

// CA-416 (control, re-expresa TestCA416_GitignoreTocadoSinNoRastreados y
// TestCA416_NoRastreadosSinGitignoreTocado de la enmienda 3): un .gitignore
// que la rama commitea (tambien uno que solo cambio la base despues de la
// rama, o archivos que se llaman parecido) es parte del cambio commiteado,
// y lo que ese .gitignore ignora no ensucia el arbol ni viaja: la evidencia
// trae el .gitignore y el codigo commiteados, sin el .env ni lo ignorado.
func TestCA416_GitignoreCommiteadoYLoIgnoradoNoFrenanLaEvidencia(t *testing.T) {
	casos := []struct {
		nombre    string
		armar     func(t *testing.T, root string)
		gitignore bool // la evidencia trae el cambio del .gitignore de la raiz
	}{
		{"la-rama-commitea-su-gitignore", func(t *testing.T, root string) {
			write(t, root, ".gitignore", ".env\n*.log\n*.bak\n# LINEA-NUEVA-DEL-GITIGNORE\n")
			git(t, root, "commit", "-q", "-am", "la rama ignora *.bak")
		}, true},
		{"la-rama-no-toca-el-gitignore", func(t *testing.T, root string) {
			write(t, root, "rama.go", "package app\n\nvar EnLaRama = true\n")
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "rama")
		}, false},
		{"solo-la-base-toco-el-gitignore-despues-de-la-rama", func(t *testing.T, root string) {
			git(t, root, "checkout", "-q", "main")
			write(t, root, ".gitignore", ".env\n*.log\n*.bak\n")
			git(t, root, "commit", "-q", "-am", "main ignora *.bak")
			git(t, root, "checkout", "-q", "feature")
		}, false},
		{"archivos-que-no-son-un-gitignore", func(t *testing.T, root string) {
			write(t, root, ".gitignore.bak", "*.log\n")
			write(t, root, "docs/plantilla.gitignore", "*.log\n")
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "archivos con gitignore en el nombre")
		}, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRamaGitignore(t)
			c.armar(t, root)
			write(t, root, "nuevo.go", "package app\n\n// NUEVO-COMMITEADO-QUE-VA\n")
			write(t, root, ".hoom/specs/x.md", "# Spec x\n")
			raCommit(t, root, "nuevo.go y el spec")
			write(t, root, ".env", raSecreto+"\n")               // ignorado por la base y por la rama
			write(t, root, "sub/cache.tmp", "IGNORADO-EN-SUB\n") // ignorado por sub/.gitignore
			if c.gitignore {
				write(t, root, "copia.bak", "IGNORADO-POR-LA-RAMA\n") // ignorado por la linea nueva
			}
			raLimpio(t, "CA-416", root)

			ev, err := Evidence(root, "main", ".hoom/specs/x.md", raTopeGrande)
			if err != nil || ev.Over {
				t.Fatalf("CA-416: con lo commiteado y lo ignorado la evidencia se arma como siempre: %v (Over %v)", err, ev.Over)
			}
			diff := string(ev.Diff)
			if !strings.Contains(diff, "+func Nuevo() {}") || !strings.Contains(diff, "+// NUEVO-COMMITEADO-QUE-VA") {
				t.Fatalf("CA-416: la evidencia trae el codigo commiteado:\n%s", diff)
			}
			tieneGI := strings.Contains(diff, "diff --git a/.gitignore b/.gitignore")
			if c.gitignore && (!tieneGI || !strings.Contains(diff, "+# LINEA-NUEVA-DEL-GITIGNORE")) {
				t.Fatalf("CA-416: la evidencia trae el cambio commiteado del .gitignore:\n%s", diff)
			}
			if !c.gitignore && tieneGI {
				t.Fatalf("CA-416: el .gitignore no es parte de este cambio:\n%s", diff)
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

// CA-416 (control, re-expresa la variante sin-otro-no-rastreado de
// TestCA416_GitignoreEnUnDirectorioIgnoradoNoCuenta): un .gitignore dentro
// de un directorio que la base ignora (node_modules/x/.gitignore, el
// .venv/.gitignore que deja virtualenv) no destapa nada: git no entra en un
// directorio ignorado. Sin nada mas sin commitear, el arbol esta limpio y la
// evidencia se arma sin nada de esos directorios. (Con un nuevo.go sin
// rastrear al lado, el arbol esta sucio: raCasosSucios.)
func TestCA416_GitignoreEnUnDirectorioIgnoradoNoCuenta(t *testing.T) {
	root := raRepo(t, "")
	write(t, root, ".gitignore", "node_modules/\n.venv/\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "la base ignora node_modules/ y .venv/")
	raCambio(t, root)
	write(t, root, "node_modules/x/.gitignore", "!*\n")
	write(t, root, "node_modules/x/index.js", "// DENTRO-DE-NODE-MODULES\n")
	write(t, root, ".venv/.gitignore", "*\n")
	write(t, root, ".venv/bin/activate", "# DENTRO-DE-VENV\n")
	if s := raSuciedad(t, root); len(s) != 0 {
		t.Fatalf("CA-416: fixture: lo de los directorios ignorados no esta sin rastrear: %q", s)
	}

	ev, err, _ := raEvidenceConReloj(t, "CA-416", 60*time.Second, root, "main", "", raTopeGrande)
	if err != nil || ev.Over {
		t.Fatalf("CA-416: un .gitignore dentro de un directorio ignorado no ensucia el arbol: la evidencia se arma: %v (Over %v)", err, ev.Over)
	}
	diff := string(ev.Diff)
	if !strings.Contains(diff, "+func Nuevo() {}") {
		t.Fatalf("CA-416: la evidencia trae el cambio de codigo:\n%s", diff)
	}
	for _, nunca := range []string{"node_modules", ".venv", "DENTRO-DE-"} {
		if strings.Contains(diff, nunca) {
			t.Fatalf("CA-416: nada de los directorios ignorados va en la evidencia: %q\n%s", nunca, diff)
		}
	}
	if ev.Bytes != len(ev.Diff) || ev.SHA256 != raSHA(ev.Diff, nil) {
		t.Fatalf("CA-401: Bytes y SHA256 cierran: %d vs %d, %q", ev.Bytes, len(ev.Diff), ev.SHA256)
	}
}

// ---------------------------------------------------------------- FIFO y avisos de git

// raDestrabarFIFO abre el FIFO para escribir y lo cierra: quien este colgado
// leyendolo (Evidence o un git suyo) recibe EOF y sigue. Si el test fallo,
// insiste un rato, por si el lector lo vuelve a abrir.
func raDestrabarFIFO(t *testing.T, fifo string) {
	veces := 1
	if t.Failed() {
		veces = 40
	}
	for i := 0; i < veces; i++ {
		if f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			f.Close()
		}
		if veces > 1 {
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// raNoRastreados es la lista de no rastreados de root tal como la da git, sin
// lo ignorado: solo para comprobar que un fixture es el que dice ser.
func raNoRastreados(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fixture: git ls-files --others: %v", err)
	}
	var lista []string
	for _, s := range strings.Split(string(out), "\x00") {
		if s != "" {
			lista = append(lista, s)
		}
	}
	return lista
}

// raMarcaFIFO es lo que un escritor deja en el FIFO suelto si alguien lo abre
// para leer: si aparece en algun lado, la review leyo el FIFO.
const raMarcaFIFO = "LEIDO-DEL-FIFO-SUELTO-ZX84"

// CA-416 / CA-401 (re-expresado por la enmienda 5; la enmienda 4 aceptaba la
// negativa o la evidencia): "con un FIFO suelto, la evidencia se arma como
// siempre y el FIFO no entra" (caso limite: "Un FIFO suelto no es un archivo
// para git (no se lista ni se puede commitear): no cuenta y nunca entra en la
// evidencia"). Con un FIFO suelto sin rastrear (mkfifo, no un symlink) y todo
// lo demas commiteado, Evidence arma la evidencia de lo commiteado enseguida
// (reloj de 10 s), sin error, sin Over, sin el nombre del FIFO ni nada leido
// de el; y `hoom review` corre su pasada con esa misma evidencia, sin el FIFO
// en el pedido ni en la salida. Dos formas: nadie escribe en el FIFO (abrirlo
// para leer colgaria) y un escritor espera para dejar raMarcaFIFO (leerlo la
// traeria). Contraste: un symlink sin rastrear a ese mismo FIFO SI es un
// arbol sucio y la negativa nombra el symlink, sin pasadas (raCasosSucios
// cubre tambien el symlink a un FIFO de afuera).
func TestCA401_FIFOSinRastrearNoEsParteDeLaEvidencia(t *testing.T) {
	for _, escritor := range []bool{false, true} {
		nombre := "fifo-sin-escritor"
		if escritor {
			nombre = "fifo-con-un-escritor-esperando"
		}
		t.Run(nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, "")
			raCambio(t, root)
			fifo := filepath.Join(root, "tuberia")
			raConFIFO(t, fifo)
			if lista := raNoRastreados(t, root); len(lista) != 0 {
				t.Fatalf("CA-416: fixture: git no lista un FIFO: %v", lista)
			}
			raLimpio(t, "CA-416", root)
			if escritor {
				listo := make(chan struct{})
				go func() {
					defer close(listo)
					f, err := os.OpenFile(fifo, os.O_WRONLY, 0) // espera a que alguien lo abra para leer
					if err != nil {
						return
					}
					_, _ = f.WriteString(raMarcaFIFO + "\n")
					f.Close()
				}()
				t.Cleanup(func() {
					// si nadie lo abrio para leer, el escritor sigue esperando: se lo destraba
					if r, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0); err == nil {
						select {
						case <-listo:
						case <-time.After(5 * time.Second):
						}
						r.Close()
					}
				})
			}

			ev, err, _ := raEvidenceConReloj(t, "CA-416", 10*time.Second, root, "main", "", raTopeGrande)
			if err != nil {
				t.Fatalf("CA-416: un FIFO suelto no cuenta: Evidence arma la evidencia sin error: %v", err)
			}
			diff := string(ev.Diff)
			if ev.Over || !strings.Contains(diff, "+func Nuevo() {}") {
				t.Fatalf("CA-416: con un FIFO suelto la evidencia se arma como siempre, con el cambio commiteado (Over %v):\n%s", ev.Over, diff)
			}
			if strings.Contains(diff, "tuberia") || strings.Contains(diff, raMarcaFIFO) {
				t.Fatalf("CA-416: el FIFO suelto nunca entra en la evidencia (ni su nombre ni lo que traiga):\n%s", diff)
			}
			if ev.Bytes != len(ev.Diff) || ev.SHA256 != raSHA(ev.Diff, nil) {
				t.Fatalf("CA-401: Bytes y SHA256 cierran: %d vs %d, %q", ev.Bytes, len(ev.Diff), ev.SHA256)
			}

			cx := raInstalar(t, bin, "codex", "")
			res, rerr, out := raRunConReloj(t, "CA-416", 20*time.Second, root, Options{Provider: "codex", Lens: "risk"})
			if rerr != nil || res.Status != "revisado" || cx.veces() != 1 || len(res.Passes) != 1 || res.EvidenceSHA256 != ev.SHA256 {
				t.Fatalf("CA-416: con un FIFO suelto hoom review corre su pasada con la evidencia de siempre (codex %d): %+v %v\n%s", cx.veces(), res, rerr, out)
			}
			if p := cx.pedido(t, 1); strings.Contains(p, "tuberia") || strings.Contains(p, raMarcaFIFO) || strings.Contains(out, raMarcaFIFO) {
				t.Fatalf("CA-416: el FIFO suelto no llega al provider ni a la salida:\n%s", p)
			}

			// contraste: un symlink sin rastrear a ese FIFO es un arbol sucio
			raSymlink(t, root, "enlace-a-tuberia", "tuberia")
			ev, err, _ = raEvidenceConReloj(t, "CA-416", 10*time.Second, root, "main", "", raTopeGrande)
			if err == nil {
				t.Fatalf("CA-416: un symlink sin rastrear (aunque apunte a un FIFO) ensucia el arbol: Evidence armo %d bytes", ev.Bytes)
			}
			if ruta, ok := raRutaSucia(err.Error()); !ok || ruta != "enlace-a-tuberia" {
				t.Fatalf("CA-416: la negativa nombra el symlink: %v", err)
			}
			res, rerr, out = raRunConReloj(t, "CA-416", 10*time.Second, root, Options{Provider: "codex", Lens: "risk"})
			msg := out
			if rerr != nil {
				msg += "\n" + rerr.Error()
			}
			if ruta, ok := raRutaSucia(msg); !ok || ruta != "enlace-a-tuberia" || cx.veces() != 1 || len(res.Passes) != 0 || strings.Contains(msg, raMarcaFIFO) {
				t.Fatalf("CA-416: con el symlink al FIFO la review se niega nombrandolo, sin pasadas (codex %d): %+v\n%s", cx.veces(), res, msg)
			}
		})
	}
}

// CA-401 (caso limite; enmienda 4: notas.txt commiteado): con core.autocrlf
// true, o con un .gitattributes de la base que pone '*.txt eol=crlf', git
// avisa por stderr ("LF will be replaced by CRLF") cada vez que lee un
// archivo de texto con fines de linea LF. Es un aviso, no un error de git ni
// un arbol sucio: Evidence no falla, el archivo nuevo notas.txt va con sus
// lineas (y el cambio de app.go con las suyas), y el aviso no entra en la
// evidencia.
func TestCA401_AvisoDeCRLFNoRompeLaEvidencia(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func(t *testing.T, root string)
	}{
		{"core-autocrlf-true", func(t *testing.T, root string) {
			git(t, root, "config", "core.autocrlf", "true")
		}},
		{"gitattributes-eol-crlf-en-la-base", func(t *testing.T, root string) {
			write(t, root, ".gitattributes", "*.txt eol=crlf\n")
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "la base pide CRLF en *.txt")
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			root := raRepo(t, "")
			c.armar(t, root)
			raCambio(t, root)
			write(t, root, "notas.txt", "primera linea de las notas\nSEGUNDA-LINEA-CON-LF\n")
			// fixture: git avisa al leer notas.txt (diff --no-index sale con 1
			// porque hay diferencias: no es el aviso)
			cmd := exec.Command("git", "diff", "--no-index", "--no-textconv", "/dev/null", "notas.txt")
			cmd.Dir = root
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			_ = cmd.Run()
			if !strings.Contains(stderr.String(), "LF will be replaced by CRLF") {
				t.Fatalf("CA-401: fixture: git avisa por stderr al leer notas.txt: %q", stderr.String())
			}
			raCommit(t, root, "notas con LF")
			raLimpio(t, "CA-401", root)

			ev, err, _ := raEvidenceConReloj(t, "CA-401", 60*time.Second, root, "main", "", raTopeGrande)
			if err != nil || ev.Over {
				t.Fatalf("CA-401: un aviso de CRLF de git no es un error: Evidence: %v (Over %v)", err, ev.Over)
			}
			diff := string(ev.Diff)
			nuevo := raSeccion(diff, "diff --git a/notas.txt b/notas.txt")
			if !strings.Contains(nuevo, "new file mode") || !strings.Contains(nuevo, "+primera linea de las notas") ||
				!strings.Contains(nuevo, "+SEGUNDA-LINEA-CON-LF") {
				t.Fatalf("CA-401: el archivo nuevo notas.txt va como parche de archivo nuevo con sus lineas:\n%s", diff)
			}
			if !strings.Contains(diff, "+func Nuevo() {}") {
				t.Fatalf("CA-401: la evidencia trae el cambio de app.go:\n%s", diff)
			}
			if strings.Contains(diff, "LF will be replaced") || strings.Contains(diff, "warning:") {
				t.Fatalf("CA-401: el aviso de git no entra en la evidencia:\n%s", diff)
			}
			if ev.Bytes != len(ev.Diff) || ev.SHA256 != raSHA(ev.Diff, nil) {
				t.Fatalf("CA-401: Bytes y SHA256 cierran: %d vs %d, %q", ev.Bytes, len(ev.Diff), ev.SHA256)
			}
		})
	}
}
