// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-420): la evidencia de un rango. EvidenceDesde es Evidence con el
// comienzo explicito: el git diff --find-renames de desde a HEAD con las
// mismas rutas (fuera de .hoom/ mas .hoom/agents/), el mismo tope, los
// mismos marcadores, el spec de HEAD, la misma guarda de arbol sucio y el
// mismo fallo cerrado; Evidence(dir, base, ...) es EvidenceDesde con el
// merge-base: los mismos bytes que hoy. Los fixtures estan en
// review_por_diferencia_helpers_test.go.
package reviewcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rdIgual exige que dos evidencias sean la misma: los mismos bytes de diff y
// de spec, el mismo sha256, los mismos Bytes y el mismo Over.
func rdIgual(t *testing.T, ca, caso string, got, want Evidencia) {
	t.Helper()
	if !bytes.Equal(got.Diff, want.Diff) {
		t.Fatalf("%s: %s: el diff es byte a byte el de referencia (%d vs %d bytes):\n--- EvidenceDesde\n%s\n--- referencia\n%s",
			ca, caso, len(got.Diff), len(want.Diff), got.Diff, want.Diff)
	}
	if !bytes.Equal(got.Spec, want.Spec) || (got.Spec == nil) != (want.Spec == nil) {
		t.Fatalf("%s: %s: el spec es el de referencia (nil %v vs %v): %q vs %q", ca, caso, got.Spec == nil, want.Spec == nil, got.Spec, want.Spec)
	}
	if got.SHA256 != want.SHA256 || got.Bytes != want.Bytes || got.Over != want.Over {
		t.Fatalf("%s: %s: sha256, Bytes y Over son los de referencia: %s/%d/%v vs %s/%d/%v",
			ca, caso, got.SHA256, got.Bytes, got.Over, want.SHA256, want.Bytes, want.Over)
	}
}

// rdRamaVariada arma una rama con todo lo que la evidencia tiene que
// escribir como git: varios commits de codigo, un renombre, un borrado, un
// binario, un nombre no ASCII, un cambio en .hoom/agents/ (va), cambios en
// .hoom/ fuera de agents (no van), el spec cambiando entre commits, y main
// avanzando despues de abrir la rama (el merge-base no es la punta de main).
func rdRamaVariada(t *testing.T) (root, spec string) {
	t.Helper()
	spec = ".hoom/specs/x.md"
	root = raRepo(t, "")
	write(t, root, "viejo.go", "package app\n\n// VIEJO-SE-BORRA\nfunc Viejo() {}\n")
	write(t, root, "renombrar.go", "package app\n\n// SE-RENOMBRA\nfunc R() {}\n\nfunc R2() {}\n\nfunc R3() {}\n")
	write(t, root, ".hoom/agents/06-reviewer.md", "# Reviewer\n\ncontrato de la base\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "base con archivos que la rama toca")
	git(t, root, "checkout", "-q", "-b", "feature")
	rdCommit(t, root, "rama 1", map[string]string{
		"app.go": "package app\n\nfunc Uno() {}\n",
		spec:     "# Spec x\n\nversion 1\n",
	})
	git(t, root, "rm", "-q", "viejo.go")
	git(t, root, "mv", "renombrar.go", "renombrado.go")
	git(t, root, "commit", "-q", "-m", "rama 2: borrado y renombre")
	rdCommit(t, root, "rama 3", map[string]string{
		"dir con espacio/ñandú.go":    "package dir\n\n// contenido-unico-ñandú\n",
		"imagen.bin":                  "\x00\x01MARCA-BINARIA\x00\xff",
		".hoom/agents/06-reviewer.md": "# Reviewer\n\ncontrato de la rama: VA-EN-LA-EVIDENCIA\n",
		".hoom/notas.txt":             "NOTA-HOOM-NO-VA\n",
	})
	rdCommit(t, root, "rama 4", map[string]string{
		"app.go": "package app\n\nfunc Uno() {}\n\nfunc Cuatro() {}\n",
		spec:     "# Spec x\n\nversion 4: la de HEAD\n",
	})
	git(t, root, "checkout", "-q", "main")
	write(t, root, "main.go", "package app\n\n// MAIN-AVANZA-NO-VA\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "main avanza")
	git(t, root, "checkout", "-q", "feature")
	raLimpio(t, "CA-420", root)
	return root, spec
}

// CA-420: "EvidenceDesde(dir, mb, spec, max) con el merge-base devuelve los
// mismos bytes que Evidence(dir, base, spec, max)". Con una rama variada
// (renombre, borrado, binario, nombre no ASCII, .hoom/agents/, .hoom/ que no
// va, main avanzado), con y sin spec, y con un spec que no esta en HEAD: el
// diff, el spec, el sha256 y los Bytes son identicos.
func TestCA420_EvidenceDesdeConElMergeBaseEsEvidence(t *testing.T) {
	root, spec := rdRamaVariada(t)
	mb := rdMB(t, root)
	if mb == rdSha(t, root, "main") {
		t.Fatal("CA-420: fixture: main avanzo: el merge-base no es su punta")
	}
	for _, s := range []string{"", spec, ".hoom/specs/no-existe.md"} {
		want, err := Evidence(root, "main", s, raTopeGrande)
		if err != nil {
			t.Fatalf("CA-420: fixture: Evidence(%q): %v", s, err)
		}
		got, err := EvidenceDesde(root, mb, s, raTopeGrande)
		if err != nil {
			t.Fatalf("CA-420: EvidenceDesde con el merge-base (spec %q) arma la evidencia: %v", s, err)
		}
		rdIgual(t, "CA-420", "merge-base, spec "+s, got, want)
	}
}

// CA-420 (propiedad): para CADA commit c de la historia de HEAD (el
// merge-base, cada commit de la rama y HEAD mismo), EvidenceDesde(dir, c)
// es la evidencia del contrato para ese rango: la de Evidence contra una
// rama que apunta a c. Y lo commiteado antes de c no aparece, lo de despues
// si.
func TestCA420_EvidenceDesdeCadaCommitDeLaHistoria(t *testing.T) {
	root, spec := rdRamaVariada(t)
	commits := strings.Fields(rdGit(t, root, "rev-list", "--first-parent", "HEAD"))
	if len(commits) < 6 {
		t.Fatalf("CA-420: fixture: la historia tiene al menos 6 commits: %v", commits)
	}
	for _, c := range commits {
		want := rdOraculoCrudo(t, root, c, spec)
		got, err := EvidenceDesde(root, c, spec, raTopeGrande)
		if err != nil {
			t.Fatalf("CA-420: EvidenceDesde(%s) arma la evidencia de %s a HEAD: %v", c[:12], c[:12], err)
		}
		rdIgual(t, "CA-420", "desde "+c[:12], got, want)
		if !bytes.Equal(got.Spec, []byte("# Spec x\n\nversion 4: la de HEAD\n")) {
			t.Fatalf("CA-420: desde %s el spec es el de HEAD: %q", c[:12], got.Spec)
		}
		d := string(got.Diff)
		if strings.Contains(d, "NOTA-HOOM-NO-VA") || strings.Contains(d, "MAIN-AVANZA-NO-VA") || strings.Contains(d, "version 4") {
			t.Fatalf("CA-420: desde %s nada de .hoom/ fuera de agents, ni de main, ni el spec va en el diff:\n%s", c[:12], d)
		}
	}
	// lo de antes de c no aparece y lo de despues si: desde el commit del
	// borrado y el renombre (rama 2), esos ya no van; rama 3 y rama 4 si
	rama2 := rdSha(t, root, "HEAD~2")
	ev, err := EvidenceDesde(root, rama2, spec, raTopeGrande)
	if err != nil {
		t.Fatalf("CA-420: EvidenceDesde(rama 2): %v", err)
	}
	d := string(ev.Diff)
	if strings.Contains(d, "VIEJO-SE-BORRA") || strings.Contains(d, "renombrar.go") || strings.Contains(d, "+func Uno() {}") {
		t.Fatalf("CA-420: lo commiteado antes de desde (el borrado, el renombre, rama 1) no aparece:\n%s", d)
	}
	if !raTieneNombre(d, "ñandú.go") || !strings.Contains(d, "imagen.bin") || !strings.Contains(d, "VA-EN-LA-EVIDENCIA") || !strings.Contains(d, "+func Cuatro() {}") {
		t.Fatalf("CA-420: lo commiteado despues de desde (rama 3 y rama 4, con .hoom/agents/) aparece:\n%s", d)
	}
	if strings.Contains(d, "MARCA-BINARIA") {
		t.Fatalf("CA-420: un binario lleva la linea de git, sin contenido:\n%s", d)
	}
	// y desde el merge-base, el borrado y los dos lados del renombre si
	ev, err = EvidenceDesde(root, rdMB(t, root), spec, raTopeGrande)
	if err != nil {
		t.Fatal(err)
	}
	d = string(ev.Diff)
	if !strings.Contains(d, "deleted file mode") || !strings.Contains(d, "VIEJO-SE-BORRA") ||
		!strings.Contains(d, "rename from renombrar.go") || !strings.Contains(d, "rename to renombrado.go") {
		t.Fatalf("CA-420: desde el merge-base el diff trae el borrado y el renombre (--find-renames):\n%s", d)
	}
}

// rdOraculoCrudo es rdOraculo sin exigir que el diff tenga algo (desde HEAD
// el diff es vacio).
func rdOraculoCrudo(t *testing.T, root, desde, spec string) Evidencia {
	t.Helper()
	rama := "rd-oraculo-" + desde[:12]
	git(t, root, "branch", "-f", rama, desde)
	ev, err := Evidence(root, rama, spec, raTopeGrande)
	if err != nil {
		t.Fatalf("CA-420: fixture: Evidence contra la rama en %s: %v", desde[:12], err)
	}
	return ev
}

// CA-420: "el mismo tope". Con un tope de 1 KiB y un rango que lo pasa,
// EvidenceDesde devuelve Over sin guardar lo leido (Bytes entre el tope y el
// tope + 64 KiB); con un rango chico despues del commit grande, la misma
// rama entra.
func TestCA420_EvidenceDesdeConElMismoTope(t *testing.T) {
	root := raRepo(t, "")
	grande := rdCommit(t, root, "grande", map[string]string{"grande.go": rdCodigo(200, "RellenoGrande")})
	rdCommit(t, root, "chico", map[string]string{"chico.go": "package app\n\nfunc Chico() {}\n"})
	raLimpio(t, "CA-420", root)

	ev, err, _ := rdEvidenceDesdeConReloj(t, "CA-420", root, rdMB(t, root), "", 1024)
	if err != nil {
		t.Fatalf("CA-420: sobre el tope no es un error, es Over: %v", err)
	}
	raOver(t, "CA-420", ev, 1024)

	ev, err, _ = rdEvidenceDesdeConReloj(t, "CA-420", root, grande, "", 1024)
	if err != nil || ev.Over {
		t.Fatalf("CA-420: el rango chico despues del commit grande entra en 1 KiB: %+v %v", ev.Bytes, err)
	}
	if strings.Contains(string(ev.Diff), "RellenoGrande") || !strings.Contains(string(ev.Diff), "func Chico()") {
		t.Fatalf("CA-420: el rango trae solo lo de despues de desde:\n%s", ev.Diff)
	}
}

// rdEvidenceDesdeConReloj corre EvidenceDesde con un reloj de 60 s.
func rdEvidenceDesdeConReloj(t *testing.T, ca, root, desde, spec string, tope int) (Evidencia, error, bool) {
	t.Helper()
	type resultado struct {
		ev  Evidencia
		err error
	}
	ch := make(chan resultado, 1)
	go func() {
		ev, err := EvidenceDesde(root, desde, spec, tope)
		ch <- resultado{ev, err}
	}()
	select {
	case r := <-ch:
		return r.ev, r.err, true
	case <-time.After(60 * time.Second):
		t.Fatalf("%s: EvidenceDesde no volvio en 60 s", ca)
	}
	return Evidencia{}, nil, false
}

// CA-420: "la misma guarda de arbol sucio y el mismo fallo cerrado". Con un
// archivo sin rastrear fuera de .hoom/, EvidenceDesde devuelve la negativa
// del contrato nombrandolo, sin armar nada y sin su contenido; tambien con un
// desde que no existe (la guarda va primero). Con el arbol limpio, un desde
// que git no resuelve es un error, nunca una evidencia vacia. Un spec que en
// HEAD es un symlink es el error de CA-412.
func TestCA420_EvidenceDesdeArbolSucioYFalloCerrado(t *testing.T) {
	root, _, a, _ := rdRamaDos(t, "", "")
	const oculto = "SIN-COMMITEAR-CA420-ZX91"
	write(t, root, "nuevo.go", "package app\n\n// "+oculto+"\n")
	for _, desde := range []string{a, "no-existe-rd"} {
		ev, err, _ := rdEvidenceDesdeConReloj(t, "CA-420", root, desde, "", raTopeGrande)
		if err == nil {
			t.Fatalf("CA-420: con el arbol sucio EvidenceDesde(%s) se niega; armo %d bytes", desde, ev.Bytes)
		}
		if ruta, ok := raRutaSucia(err.Error()); !ok || ruta != "nuevo.go" {
			t.Fatalf("CA-420: EvidenceDesde(%s) devuelve %q antes que nada: %v", desde, raErrSucio("nuevo.go"), err)
		}
		if len(ev.Diff) != 0 || len(ev.Spec) != 0 || ev.SHA256 != "" || strings.Contains(err.Error(), oculto) {
			t.Fatalf("CA-420: sin armar la evidencia ni filtrar lo sin commitear: %+v %v", ev, err)
		}
	}
	if err := os.Remove(filepath.Join(root, "nuevo.go")); err != nil {
		t.Fatal(err)
	}
	raLimpio(t, "CA-420", root)

	for _, desde := range []string{"no-existe-rd", strings.Repeat("ab", 20)} {
		ev, err, _ := rdEvidenceDesdeConReloj(t, "CA-420", root, desde, "", raTopeGrande)
		if err == nil {
			t.Fatalf("CA-420: un desde que git no resuelve (%s) falla cerrado; devolvio %d bytes (Over %v, sha %q)", desde, ev.Bytes, ev.Over, ev.SHA256)
		}
	}

	raSymlink(t, root, ".hoom/specs/link.md", "/dev/zero")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "spec symlink")
	ev, err, _ := rdEvidenceDesdeConReloj(t, "CA-420", root, a, ".hoom/specs/link.md", raTopeGrande)
	if err == nil || !strings.Contains(err.Error(), raNoEsDelArbol(".hoom/specs/link.md")) {
		t.Fatalf("CA-420: el spec symlink en HEAD es %q tambien con rango: %v (%d bytes)", raNoEsDelArbol(".hoom/specs/link.md"), err, ev.Bytes)
	}
}

// CA-420: `hoom review --desde <c>` arma la evidencia de c a HEAD: el
// reviewer recibe, entre sus marcadores, exactamente la evidencia del
// contrato para ese rango (lo de antes de c no, lo de despues si), las
// cabeceras dicen su tamano y su sha256, y el Result y el registro la
// nombran.
func TestCA420_ReviewDesdeRevisaDeCAHEAD(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root, _, a, _ := rdRamaDos(t, "", spec)
	cx := raInstalar(t, bin, "codex", "")
	ev := rdOraculo(t, "CA-420", root, a, spec)
	if strings.Contains(string(ev.Diff), "MARCA-DE-A") || !strings.Contains(string(ev.Diff), "MARCA-DE-B") {
		t.Fatalf("CA-420: fixture: la evidencia de referencia es la de B:\n%s", ev.Diff)
	}

	res, out := rdRevisar(t, "CA-420", root, Options{Provider: "codex", Spec: spec, Desde: a})
	if res.Status != "revisado" || cx.veces() != 1 {
		t.Fatalf("CA-420: la review del rango corre (una lente): %+v\n%s", res, out)
	}
	ped := cx.pedido(t, 1)
	if !strings.Contains(ped, raBloque(t, ev, spec, true)) {
		t.Fatalf("CA-420: el reviewer recibe la evidencia de %s a HEAD entre sus marcadores:\n%s", a[:12], ped)
	}
	if strings.Contains(ped, "MARCA-DE-A") {
		t.Fatalf("CA-420: lo commiteado antes de --desde no aparece en el pedido:\n%s", ped)
	}
	if res.EvidenceSHA256 != ev.SHA256 || res.EvidenceBytes != ev.Bytes {
		t.Fatalf("CA-420: el Result nombra la evidencia del rango (%s, %d): %s, %d", ev.SHA256, ev.Bytes, res.EvidenceSHA256, res.EvidenceBytes)
	}
	cab := raCabecera(t, out)
	quiero := fmt.Sprintf("  evidencia   %d KiB (diff %d + spec %d), tope 320 KiB - sha256 %s",
		raKiB(ev.Bytes), raKiB(len(ev.Diff)), raKiB(len(ev.Spec)), ev.SHA256[:12])
	if cab[3] != quiero {
		t.Fatalf("CA-420: la linea evidencia es la de la evidencia del rango %q: %q", quiero, cab[3])
	}
	r := rdRegistro(t, "CA-420", root, res.RecordID)
	if r.EvidenceSHA256 != ev.SHA256 || r.EvidenceBytes != ev.Bytes {
		t.Fatalf("CA-420: el registro nombra la evidencia del rango: %+v", r)
	}
}

// CA-420: "el mismo tope" en `hoom review`: una rama cuya evidencia entera
// pasa max_evidence_kib no se revisa entera (NO ENTREGABLE, como hoy), y el
// rango de despues del commit grande, que entra, se revisa. Y un rango que
// pasa el tope es el mismo NO ENTREGABLE: sin pasadas, sin registro, exit 1.
func TestCA420_ReviewDesdeConElTopeDeHoomYaml(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "review:\n  max_evidence_kib: 2\n")
	grande := rdCommit(t, root, "grande", map[string]string{"grande.go": rdCodigo(200, "RellenoGrande")})
	rdCommit(t, root, "chico", map[string]string{"chico.go": "package app\n\nfunc Chico() {}\n", "chico2.go": "package app\n\nfunc Chico2() {}\n"})
	cx := raInstalar(t, bin, "codex", "")

	res, out := rdRevisar(t, "CA-420", root, Options{Provider: "codex"})
	if res.Status != "no-entregable" || cx.veces() != 0 || !strings.Contains(out, raNoEntregable(2)) {
		t.Fatalf("CA-420: fixture: la rama entera pasa el tope de 2 KiB: %+v\n%s", res, out)
	}

	res, out = rdRevisar(t, "CA-420", root, Options{Provider: "codex", Desde: grande})
	if res.Status != "revisado" || cx.veces() != 1 || res.RecordID == "" {
		t.Fatalf("CA-420: el rango chico entra en el mismo tope y se revisa: %+v\n%s", res, out)
	}
	if strings.Contains(cx.pedido(t, 1), "RellenoGrande") {
		t.Fatal("CA-420: el pedido del rango no trae el commit grande")
	}

	// un rango que pasa el tope: la negativa de hoy
	mb := rdMB(t, root)
	registros := rdRegistrosEn(root)
	res, err, out := rdCorrer(t, "CA-420", root, Options{Provider: "codex", Desde: mb})
	if err != nil || res.Status != "no-entregable" || res.ExitCode != 1 || cx.veces() != 1 || !strings.Contains(out, raNoEntregable(2)) {
		t.Fatalf("CA-420: un rango sobre el tope es NO ENTREGABLE, exit 1, sin pasadas: %+v %v\n%s", res, err, out)
	}
	if rdRegistrosEn(root) != registros {
		t.Fatal("CA-420: sobre el tope no hay registro")
	}
}
