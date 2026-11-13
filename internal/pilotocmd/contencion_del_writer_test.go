// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre el
// piloto y su guardado, como consumidor externo (proyectos git reales, CLIs
// de IA falsos en un PATH minimo, nunca uno real), en cualquier sistema:
//
//   - CA-692: el writer que cambio git ajeno o la config de los agentes y
//     murio por su timeout no se releva, y el piloto se detiene antes del
//     proximo guardado con el Why del caso 8;
//   - CA-687: en cerrar-hallazgos, un gate que cambia el material deja el
//     veredicto rojo y el piloto se detiene con su texto, que nombra
//     material_de_git, antes de guardar;
//   - CA-676: con exigir y un nivel menor, el guardado se niega con su texto
//     y el verify de cerrar-hallazgos no corre;
//   - CA-688 y CA-685: itemcmd.SaveTarea y el material, git-ajeno y el
//     ultimo veredicto.
//
// Reusa los helpers de autonomia-entre-firmas (apProyecto, apRun, apr1Ronda...).
package pilotocmd_test

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func cdwSha(data string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(data), data)))
	return hex.EncodeToString(sum[:])
}

func cdwComun(t *testing.T, wt string) string {
	t.Helper()
	p := apGit(t, wt, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(p) {
		p = filepath.Join(wt, p)
	}
	return p
}

func cdwHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

// cdwReplace es el cuerpo de shell que crea una ref de reemplazo (git-ajeno)
// entre dos blobs nuevos, y la ruta que la nombra.
func cdwReplace(t *testing.T, wt, a, b string) (cuerpo, ruta string) {
	t.Helper()
	cuerpo = "printf '" + a + "' | git hash-object -w --stdin >/dev/null\n" +
		"printf '" + b + "' | git hash-object -w --stdin >/dev/null\n" +
		"git replace -f " + cdwSha(a) + " " + cdwSha(b) + "\n"
	return cuerpo, filepath.Join(cdwComun(t, wt), "refs", "replace", cdwSha(a))
}

// ---------------------------------------------------------------- CA-692

// cdwEnWriter arma la tarjeta en la columna Writer: spec firmado, un
// test-writer real (CLI falso) y su trabajo guardado.
func cdwEnWriter(t *testing.T, ca string, f *apFalsos) (root, wt, slug string) {
	t.Helper()
	root = apProyecto(t, "")
	apr1Gate(t, root, "grep -q implementado app.go")
	slug = "vencido"
	apItem(t, root, slug, apConAuto)
	wt = apTarea(t, root, slug)
	apEscribir(t, wt, apSpecRel(slug), apSpecTexto("- CA-1: el total redondea bien."))
	apFirmar(t, wt, slug)
	apCommit(t, wt, "spec aprobado")
	apColumna(t, ca, root, slug, boardcmd.ColTestWriter)
	apr1Fake(t, f, "claude", "printf 'package app\\n\\n// CA-1: el total redondea bien\\n' > precios_test.go\nexit 0\n")
	apr1Correr(t, root, agentcmd.Options{Role: "test-writer", Provider: "claude", Task: slug, Prompt: "escribi los tests de CA-1"})
	apGit(t, wt, "add", "-A")
	apGit(t, wt, "commit", "-q", "--allow-empty", "-m", apGuardaMs+slug)
	apColumna(t, ca, root, slug, boardcmd.ColWriter)
	return root, wt, slug
}

// CA-692: el writer vencido que cambio git ajeno o la config de los agentes
// no se releva ni se reintenta: el piloto se detiene con el Why del caso 8,
// exit 1, sin lanzar nada, sin sobre nuevo y sin guardar (app.go sigue sin
// guardar y HEAD no se mueve).
func TestCA692_ElWriterVencidoQueCambioGitAjenoOLaConfigNoSeReleva(t *testing.T) {
	for _, caso := range []string{"git-ajeno", "config-de-agentes"} {
		ca := "CA-692 (" + caso + ")"
		f := apPATH(t)
		home := cdwHome(t)
		root, wt, slug := cdwEnWriter(t, ca, f)
		var cuerpo string
		var de func(finding.Finding, string) bool
		switch caso {
		case "git-ajeno":
			cuerpo, _ = cdwReplace(t, wt, "cdw vencido a", "cdw vencido b")
			de = agentcmd.DeGitAjeno
		default:
			settings := filepath.Join(home, ".claude", "settings.json")
			apEscribir(t, filepath.Dir(settings), "settings.json", `{"hooks":{}}`)
			cuerpo = "printf '{\"hooks\":{\"Stop\":[{\"command\":\"sh\"}]}}' > '" + settings + "'\n"
			de = agentcmd.DeConfigDeAgentes
		}
		apr1Fake(t, f, "claude", "printf 'package app // a medias\\n' > app.go\n"+cuerpo+"exec sleep 60\n")
		desde := time.Now()
		w := apr1Correr(t, root, agentcmd.Options{Role: "writer", Provider: "claude", Task: slug, Prompt: "implementa el spec",
			Timeout: 3 * time.Second, Pilot: true, Columna: boardcmd.ColWriter, Regla: boardcmd.ReglaCadena})
		if time.Since(desde) > 45*time.Second || w.Status == envelope.StatusDeliverable {
			t.Fatalf("%s: fixture: el writer muere por su timeout sin entregar: %+v", ca, w)
		}
		if rec := apRegistro(t, ca, root, w.EnvelopeID); rec.Corte != envelope.CorteTiempo {
			t.Fatalf("%s: fixture: el registro del writer dice corte tiempo: %+v", ca, rec)
		}
		hay := false
		for _, d := range []string{wt, root} {
			items, _, _ := finding.List(d, apBase, true)
			for _, it := range items {
				if it.Status == finding.StatusOpen && de(it.Finding, "writer") {
					hay = true
				}
			}
		}
		if !hay {
			t.Fatalf("%s: el sobre del writer vencido registra el hallazgo high de %s", ca, caso)
		}
		f.cli(t, "claude") // si el piloto lo releva, este claude termina enseguida
		head, claude, sobres := apHead(t, wt), f.llamadas(t, "claude"), len(envelope.List(root))
		s := apRun(t, ca, root, slug)
		ls := apSoloLineasDelPiloto(t, ca, s)
		for _, l := range ls {
			if strings.Contains(l, " pide al ") || strings.Contains(l, boardcmd.ReglaRelevoPorTiempo) || strings.Contains(l, boardcmd.ReglaReintento) ||
				apGuarda.MatchString(l) {
				t.Fatalf("%s: el piloto no releva, no reintenta y no guarda: %q\n%s", ca, l, s)
			}
		}
		want := apDetiene + "el trabajo del writer no cerro entregable (" + envelope.StatusNotDeliverable + "): el piloto se detiene ante un rojo"
		if s.code != 1 || apUltima(ls) != want {
			t.Fatalf("%s: se detiene con el Why del caso 8, %q, y exit 1:\n%s", ca, want, s)
		}
		if n := f.llamadas(t, "claude"); n != claude {
			t.Fatalf("%s: ningun rol corre: claude %d -> %d", ca, claude, n)
		}
		if n := len(envelope.List(root)); n != sobres {
			t.Fatalf("%s: ningun sobre nuevo: %d -> %d", ca, sobres, n)
		}
		if h := apHead(t, wt); h != head || !strings.Contains(apGit(t, wt, "status", "--porcelain"), "app.go") {
			t.Fatalf("%s: se detiene antes del proximo guardado: HEAD %s -> %s, status %q", ca, head, h, apGit(t, wt, "status", "--porcelain"))
		}
	}
}

// ---------------------------------------------------------------- CA-687

// CA-687: en cerrar-hallazgos, el gate del verify (con la vista del writer)
// que cambia el material deja el veredicto nuevo rojo con material_de_git en
// fail, y el piloto se detiene con su texto de veredicto rojo, que nombra
// material_de_git, antes de guardar.
func TestCA687_CerrarHallazgosConUnGateQueCambiaElMaterial(t *testing.T) {
	const ca = "CA-687"
	f := apPATH(t)
	fx := apr1Ronda(t, "c=$(git rev-parse --git-common-dir); mkdir -p $c/info; printf x > $c/info/attributes; true")
	head := apHead(t, fx.wt)
	s := apRun(t, ca, fx.root, fx.slug)
	ls := apSoloLineasDelPiloto(t, ca, s)
	nuevos := apNuevos(t, fx.wt, fx.veredictos)
	if len(nuevos) != 1 {
		t.Fatalf("%s: cerrar-hallazgos deja un veredicto nuevo: %d\n%s", ca, len(nuevos), s)
	}
	var id string
	var v verdict.Verdict
	for k, nv := range nuevos {
		id, v = k, nv
	}
	var g *verdict.GateResult
	for i := range v.Gates {
		if v.Gates[i].Name == "material_de_git" {
			g = &v.Gates[i]
		}
	}
	if g == nil || g.Status != verdict.StatusFail || v.Verdict == "green" {
		t.Fatalf("%s: el veredicto nuevo %s trae material_de_git en fail y es rojo: %+v", ca, id, v.Gates)
	}
	pre := apDetiene + "despues de cerrar los hallazgos de la ronda 1, el veredicto " + id + " sigue rojo en "
	u := apUltima(ls)
	if s.code != 1 || !strings.HasPrefix(u, pre) || !strings.Contains(strings.TrimPrefix(u, pre), "material_de_git") || !strings.HasSuffix(u, ": te toca") {
		t.Fatalf("%s: se detiene con %q...material_de_git...: te toca, y exit 1:\n%s", ca, pre, s)
	}
	for _, l := range ls {
		if apGuarda.MatchString(l) {
			t.Fatalf("%s: se detiene antes de guardar: %q\n%s", ca, l, s)
		}
	}
	if h := apHead(t, fx.wt); h != head {
		if msg := apGit(t, fx.wt, "log", "-1", "--format=%s"); msg == apGuardaMs+fx.slug {
			t.Fatalf("%s: no guarda despues de un veredicto con material_de_git en fail: HEAD %s -> %s (%q)", ca, head, h, msg)
		}
	}
	f.ninguna(t, ca, "cerrar-hallazgos")
}

// ---------------------------------------------------------------- CA-676

// CA-676: con la seccion encierro exigiendo completo y un nivel menor (sin
// bwrap), el piloto se detiene con un Why que contiene el texto de
// ErrExigirDelGuardado y el verify de cerrar-hallazgos no corre.
func TestCA676_CerrarHallazgosConExigirNoCorreElVerify(t *testing.T) {
	const ca = "CA-676"
	restaurar := encierro.ReemplazarSonda(encierro.Sonda{})
	defer restaurar()
	f := apPATH(t)
	fx := apr1Ronda(t, "true")
	raw, err := os.ReadFile(filepath.Join(fx.root, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	apEscribir(t, fx.root, "hoom.yaml", string(raw)+"encierro:\n  exigir: completo\n")
	apCommit(t, fx.root, "exige encierro completo")
	nivel := encierro.Nivel(true, nil).Nivel
	if encierro.Alcanza(nivel, "completo") {
		t.Fatalf("%s: fixture: sin bwrap el nivel no alcanza completo: %s", ca, nivel)
	}
	texto := itemcmd.ErrExigirDelGuardado(nivel, "completo")
	if texto != "el encierro de este sistema es "+nivel+" y hoom.yaml exige completo: no guardo" {
		t.Fatalf("%s: ErrExigirDelGuardado: %q", ca, texto)
	}
	s := apRun(t, ca, fx.root, fx.slug)
	ls := apSoloLineasDelPiloto(t, ca, s)
	if s.code != 1 || !strings.HasPrefix(apUltima(ls), apDetiene) || !strings.Contains(apUltima(ls), texto) {
		t.Fatalf("%s: se detiene con un Why que contiene %q, exit 1:\n%s", ca, texto, s)
	}
	if n := apNuevos(t, fx.wt, fx.veredictos); len(n) != 0 {
		t.Fatalf("%s: el verify de cerrar-hallazgos no corre: %d veredictos nuevos", ca, len(n))
	}
	f.ninguna(t, ca, "cerrar-hallazgos con exigir")
}

// ---------------------------------------------------------------- SaveTarea

// cdwTarjeta es una tarjeta con su tarea guardada y app.go sin guardar.
func cdwTarjeta(t *testing.T, extra, slug string) (root, wt string) {
	t.Helper()
	root = apProyecto(t, extra)
	wt = apTuAprobacion(t, root, slug, apConAuto, true)
	apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
	return root, wt
}

// cdwVeredicto escribe en wt un veredicto completo con el gate material_de_git
// en estado (o sin el, con "").
func cdwVeredicto(t *testing.T, wt string, cuando time.Time, material string) string {
	t.Helper()
	gates := []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusPass}}
	if material != "" {
		gates = append(gates, verdict.GateResult{Name: "material_de_git", Required: true, Status: material, Scope: "full",
			OutputTail: filepath.Join(cdwComun(t, wt), "info", "attributes")})
	}
	v := &verdict.Verdict{Project: "demo", CreatedAt: cuando, Git: gitx.Snapshot(wt, apBase), Gates: gates}
	v.Finalize()
	if _, err := verdict.Write(wt, v); err != nil {
		t.Fatal(err)
	}
	return v.ID
}

// CA-688: el guardado se niega antes de empezar, sin commitear, si el ultimo
// veredicto completo del arbol tiene material_de_git en fail; con uno mas
// nuevo sin el, guarda.
func TestCA688_ElGuardadoSeNiegaConElUltimoVeredictoConMaterial(t *testing.T) {
	const ca = "CA-688"
	apPATH(t)
	root, wt := cdwTarjeta(t, "", "material")
	cdwVeredicto(t, wt, time.Now().UTC().Add(-time.Minute), verdict.StatusFail)
	head := apHead(t, wt)
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "material")
	if err == nil || err.Error() != itemcmd.ErrVeredictoConMaterial || len(res.Commits) != 0 || apHead(t, wt) != head {
		t.Fatalf("%s: el guardado se niega con %q sin commitear: %+v %v", ca, itemcmd.ErrVeredictoConMaterial, res, err)
	}
	cdwVeredicto(t, wt, time.Now().UTC(), "")
	if res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "material"); err != nil || len(res.Commits) != 1 {
		t.Fatalf("%s: con un veredicto mas nuevo sin material_de_git, guarda: %+v %v", ca, res, err)
	}
}

// CA-688: un hook del guardado que crea una pieza del material hace fallar
// el guardado, que deshace su commit y devuelve el texto del contrato; uno
// que no lo cambia guarda como hoy.
func TestCA688_UnHookQueCambiaElMaterialHaceFallarElGuardado(t *testing.T) {
	const ca = "CA-688"
	apPATH(t)
	root, wt := cdwTarjeta(t, "", "hook")
	ruta := filepath.Join(cdwComun(t, wt), "info", "attributes")
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\nmkdir -p '"+filepath.Dir(ruta)+"'\nprintf '*.go filter=x\\n' > '"+ruta+"'\nexit 0\n")
	head := apHead(t, wt)
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "hook")
	want := itemcmd.ErrHookCambioMaterial([]string{ruta})
	if want != "un hook del guardado cambio el material de git: "+ruta+": no guardo" {
		t.Fatalf("%s: ErrHookCambioMaterial: %q", ca, want)
	}
	if err == nil || err.Error() != want {
		t.Fatalf("%s: el guardado falla con %q: %+v %v", ca, want, res, err)
	}
	if h := apHead(t, wt); h != head {
		t.Fatalf("%s: el guardado deshace su commit: HEAD %s -> %s", ca, head, h)
	}
	if !strings.Contains(apGit(t, wt, "status", "--porcelain"), "app.go") {
		t.Fatalf("%s: lo que habia sin guardar sigue sin guardar", ca)
	}

	// control: un hook que no cambia el material guarda como hoy
	root, wt = cdwTarjeta(t, "", "inocuo")
	otro := filepath.Join(t.TempDir(), "corrio")
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\n: > '"+otro+"'\nexit 0\n")
	res, err = itemcmd.SaveTarea(root, apBase, apBlockOn, "inocuo")
	if err != nil || len(res.Commits) != 1 || !apr1Existe(otro) {
		t.Fatalf("%s: un hook que no cambia el material guarda: %+v %v (corrio %v)", ca, res, err, apr1Existe(otro))
	}
}

// CA-685: un hook del guardado que crea una ref de reemplazo hace fallar el
// guardado con ErrGitAjeno, que deshace su commit; mover otra rama o
// refs/stash no es git-ajeno.
func TestCA685_ElGuardadoConGitAjeno(t *testing.T) {
	const ca = "CA-685"
	apPATH(t)
	root, wt := cdwTarjeta(t, "", "ajeno")
	cuerpo, ruta := cdwReplace(t, wt, "cdw guardado a", "cdw guardado b")
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\n"+cuerpo+"exit 0\n")
	head := apHead(t, wt)
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "ajeno")
	want := itemcmd.ErrGitAjeno("ajeno", []string{ruta})
	if want != "el guardado cambio git fuera de hoom/ajeno: "+ruta+": no guardo" {
		t.Fatalf("%s: ErrGitAjeno: %q", ca, want)
	}
	if err == nil || err.Error() != want || apHead(t, wt) != head {
		t.Fatalf("%s: el guardado falla con %q y deshace su commit: %+v %v (HEAD %s -> %s)", ca, want, res, err, head, apHead(t, wt))
	}

	root, wt = cdwTarjeta(t, "", "stash")
	apGit(t, root, "branch", "otra-rama")
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\n"+
		"c=$(git commit-tree 'HEAD^{tree}' -p HEAD -m otra)\ngit update-ref refs/heads/otra-rama \"$c\"\ngit update-ref refs/stash HEAD\nexit 0\n")
	res, err = itemcmd.SaveTarea(root, apBase, apBlockOn, "stash")
	if err != nil || len(res.Commits) != 1 {
		t.Fatalf("%s: mover otra rama o refs/stash no es git-ajeno: %+v %v", ca, res, err)
	}
}

// CA-676: con la seccion exigiendo completo y un nivel menor, el guardado se
// niega con ErrExigirDelGuardado, sin commitear; con exigir no, guarda.
func TestCA676_ElGuardadoSeNiegaConExigir(t *testing.T) {
	const ca = "CA-676"
	restaurar := encierro.ReemplazarSonda(encierro.Sonda{})
	defer restaurar()
	apPATH(t)
	nivel := encierro.Nivel(true, nil).Nivel
	root, wt := cdwTarjeta(t, "encierro:\n  exigir: completo\n", "exige")
	head := apHead(t, wt)
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "exige")
	if err == nil || err.Error() != itemcmd.ErrExigirDelGuardado(nivel, "completo") || len(res.Commits) != 0 || apHead(t, wt) != head {
		t.Fatalf("%s: el guardado se niega con %q sin commitear: %+v %v", ca, itemcmd.ErrExigirDelGuardado(nivel, "completo"), res, err)
	}
	root, _ = cdwTarjeta(t, "encierro:\n  exigir: no\n", "noexige")
	if res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "noexige"); err != nil || len(res.Commits) != 1 {
		t.Fatalf("%s: con exigir no guarda con el nivel que haya: %+v %v", ca, res, err)
	}
}
