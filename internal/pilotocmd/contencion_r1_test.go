// Ronda 1 del test-writer de contencion-del-writer: regresiones de la review
// de codex sobre el piloto y su guardado, en cualquier sistema:
//
//   - 318ca0 (CA-685, CA-687): en cerrar-hallazgos, un gate que crea una ref
//     de reemplazo deja git_ajeno en fail y el piloto se detiene con su texto
//     de veredicto rojo, que nombra git_ajeno, antes de guardar;
//   - 6a7c72 (CA-688, CA-689; aclaracion 21): un hook del guardado que crea
//     config de los agentes sin rastrear hace fallar el guardado con
//     itemcmd.ErrHookCambioConfig;
//   - f83160 (CA-684; aclaracion 24): con la seccion encierro, el git del
//     guardado lleva commit.gpgsign=false aunque no haya bwrap;
//   - 7adc7b (CA-688): cuando la rama no puede volver, el error del deshacer
//     no dice que volvio.
package pilotocmd_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------- punto 4

// CA-685 y CA-687 (id 318ca0; aclaracion 20): en cerrar-hallazgos, el gate
// del verify que crea una ref bajo refs/replace/ deja el veredicto nuevo rojo
// con git_ajeno en fail (con la ruta), y el piloto se detiene con su texto de
// veredicto rojo, que nombra git_ajeno, antes de guardar. Ningun CLI corre.
func TestR1_318ca0_CA685_CA687_CerrarHallazgosConUnGateQueRompeGitAjeno(t *testing.T) {
	const ca = "CA-685"
	f := apPATH(t)
	const a, b = "r1-piloto-a", "r1-piloto-b"
	fx := apr1Ronda(t, "x=$(printf "+a+" | git hash-object -w --stdin); y=$(printf "+b+" | git hash-object -w --stdin); git replace -f $x $y; true")
	ruta := filepath.Join(cdwComun(t, fx.wt), "refs", "replace", cdwSha(a))
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
		if v.Gates[i].Name == "git_ajeno" {
			g = &v.Gates[i]
		}
	}
	if g == nil || g.Status != verdict.StatusFail || !strings.Contains(g.OutputTail, ruta) || v.Verdict == "green" {
		t.Fatalf("%s: el veredicto nuevo %s trae git_ajeno en fail con %s y es rojo: %+v", ca, id, ruta, v.Gates)
	}
	pre := apDetiene + "despues de cerrar los hallazgos de la ronda 1, el veredicto " + id + " sigue rojo en "
	u := apUltima(ls)
	if s.code != 1 || !strings.HasPrefix(u, pre) || !strings.Contains(strings.TrimPrefix(u, pre), "git_ajeno") || !strings.HasSuffix(u, ": te toca") {
		t.Fatalf("%s: se detiene con %q...git_ajeno...: te toca, y exit 1:\n%s", ca, pre, s)
	}
	for _, l := range ls {
		if apGuarda.MatchString(l) {
			t.Fatalf("%s: se detiene antes de guardar: %q\n%s", ca, l, s)
		}
	}
	if h := apHead(t, fx.wt); h != head {
		if msg := apGit(t, fx.wt, "log", "-1", "--format=%s"); msg == apGuardaMs+fx.slug {
			t.Fatalf("%s: no guarda despues de un veredicto con git_ajeno en fail: HEAD %s -> %s (%q)", ca, head, h, msg)
		}
	}
	f.ninguna(t, ca, "cerrar-hallazgos")
}

// ---------------------------------------------------------------- punto 5

// CA-688 y CA-689 (id 6a7c72; aclaracion 21): un hook del guardado
// (pre-commit) que crea sin rastrear .claude/settings.json, .mcp.json o algo
// bajo .codex/ en el arbol hace fallar el guardado: deshace su commit (HEAD
// queda donde estaba, lo sin guardar sigue sin guardar) y el error empieza
// con ErrHookCambioConfig de esa ruta. Un hook que no toca esa config guarda.
func TestR1_6a7c72_CA688_CA689_UnHookQueCreaConfigDeAgentesHaceFallarElGuardado(t *testing.T) {
	const ca = "CA-688"
	if got := itemcmd.ErrHookCambioConfig([]string{".mcp.json"}); got != "un hook del guardado cambio la config de los agentes: .mcp.json: no guardo" {
		t.Fatalf("%s: ErrHookCambioConfig: %q", ca, got)
	}
	for i, c := range []struct{ rel, cuerpo string }{
		{".claude/settings.json", `{"hooks":{"Stop":[{"command":"r1-del-hook"}]}}`},
		{".mcp.json", `{"mcpServers":{"r1":{"command":"r1-del-hook"}}}`},
		{".codex/config.toml", "model = \"r1-del-hook\"\n"},
	} {
		apPATH(t)
		cdwHome(t)
		slug := "config" + string(rune('a'+i))
		root, wt := cdwTarjeta(t, "", slug)
		abs := filepath.Join(wt, filepath.FromSlash(c.rel))
		apr4Hook(t, wt, "pre-commit", "#!/bin/sh\nmkdir -p '"+filepath.Dir(abs)+"'\nprintf '%s' '"+c.cuerpo+"' > '"+abs+"'\nexit 0\n")
		head := apHead(t, wt)
		res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, slug)
		// el texto del contrato es el comienzo del error (despues puede venir
		// el sufijo del deshacer); la ruta, como la nombra la foto
		wantRel := itemcmd.ErrHookCambioConfig([]string{c.rel})
		wantAbs := itemcmd.ErrHookCambioConfig([]string{abs})
		if err == nil || !(strings.HasPrefix(err.Error(), wantRel) || strings.HasPrefix(err.Error(), wantAbs)) {
			t.Fatalf("%s: %s: el guardado falla con un error que empieza con %q: %+v %v", ca, c.rel, wantRel, res, err)
		}
		if h := apHead(t, wt); h != head {
			t.Fatalf("%s: %s: el guardado deshace su commit: HEAD %s -> %s", ca, c.rel, head, h)
		}
		if !strings.Contains(apGit(t, wt, "status", "--porcelain"), "app.go") {
			t.Fatalf("%s: %s: lo que habia sin guardar sigue sin guardar", ca, c.rel)
		}
	}

	// control: un hook que no toca la config de los agentes guarda como hoy
	apPATH(t)
	cdwHome(t)
	root, wt := cdwTarjeta(t, "", "inocuo")
	otro := filepath.Join(t.TempDir(), "corrio")
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\n: > '"+otro+"'\nexit 0\n")
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "inocuo")
	if err != nil || len(res.Commits) != 1 || !apr1Existe(otro) {
		t.Fatalf("%s: un hook que no toca la config de los agentes guarda: %+v %v (corrio %v)", ca, res, err, apr1Existe(otro))
	}
}

// ---------------------------------------------------------------- punto 8

// r1Gpg configura en el repo de wt commit.gpgsign=true y un gpg.program que
// deja su marca y falla.
func r1Gpg(t *testing.T, wt string) (marca string) {
	t.Helper()
	marca = filepath.Join(t.TempDir(), "gpg-corrio")
	gpg := filepath.Join(t.TempDir(), "gpg-falso")
	if err := os.WriteFile(gpg, []byte("#!/bin/sh\n: > '"+marca+"'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	apGit(t, wt, "config", "commit.gpgsign", "true")
	apGit(t, wt, "config", "gpg.program", gpg)
	return marca
}

// CA-684 (id f83160; aclaracion 24): con la seccion encierro y sin bwrap
// (ReemplazarSonda con la Sonda cero, exigir no), el guardado no firma: en un
// repo con commit.gpgsign=true y un gpg.program que falla, guarda y el
// gpg.program no corre. Sin la seccion, como hoy: el mismo guardado falla al
// firmar, sin commitear.
func TestR1_f83160_CA684_SinBwrapElGuardadoConLaSeccionNoFirma(t *testing.T) {
	const ca = "CA-684"
	restaurar := encierro.ReemplazarSonda(encierro.Sonda{})
	defer restaurar()
	if n := encierro.Nivel(true, nil); runtime.GOOS == "linux" && n.Nivel != encierro.NivelNinguno {
		t.Fatalf("%s: fixture: sin bwrap el nivel es ninguno: %+v", ca, n)
	}
	apPATH(t)
	cdwHome(t)
	root, wt := cdwTarjeta(t, "encierro:\n  exigir: no\n", "firma")
	marca := r1Gpg(t, wt)
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, "firma")
	if err != nil || len(res.Commits) != 1 {
		t.Fatalf("%s: con la seccion y sin bwrap, el guardado lleva commit.gpgsign=false y guarda: %+v %v", ca, res, err)
	}
	if apr1Existe(marca) {
		t.Fatalf("%s: con la seccion, el gpg.program del repo no corre en el guardado", ca)
	}

	// control: sin la seccion, el mismo guardado firma y falla
	apPATH(t)
	cdwHome(t)
	root, wt = cdwTarjeta(t, "", "sinseccion")
	marca = r1Gpg(t, wt)
	head := apHead(t, wt)
	res, err = itemcmd.SaveTarea(root, apBase, apBlockOn, "sinseccion")
	if err == nil || len(res.Commits) != 0 || apHead(t, wt) != head || !apr1Existe(marca) {
		t.Fatalf("%s: control: sin la seccion el guardado firma como hoy y falla sin commitear: %+v %v (gpg corrio %v)", ca, res, err, apr1Existe(marca))
	}
}

// ---------------------------------------------------------------- punto 9

// r1NoVolvio reconoce un texto que dice que la rama no volvio, incluida la
// frase que el deshacer ya usaba antes de esta tarea: `; y no pude devolver
// <rama> de <sha> a <sha>: <motivo>` (ronda 1b).
var r1NoVolvio = regexp.MustCompile(`(?i)\bno (volvi|vuelve|pudo volver|se pudo volver|regres|pude devolver \S+ de [0-9a-f]+ a [0-9a-f]+: )`)

// CA-688 (id 7adc7b): un hook del guardado que cambia el material de git y
// ademas impide que la rama vuelva: el error empieza con
// ErrHookCambioMaterial, dice que la rama NO volvio y no dice `vuelve a`.
// Dos maneras de impedir la vuelta: un post-commit que deja tomado el
// <rama>.lock (la vuelta tiene que fallar: si vuelve, es el fixture el que
// fallo), y un hook reference-transaction que, armado por el post-commit,
// rechaza toda transaccion (si hoom vuelve sin hooks, la rama vuelve y no
// hay nada que probar).
func TestR1_7adc7b_CA688_ElDeshacerQueNoPuedeVolverNoMiente(t *testing.T) {
	const ca = "CA-688"
	for _, modo := range []string{"lock", "reference-transaction"} {
		apPATH(t)
		cdwHome(t)
		slug := "novuelve" + strings.ReplaceAll(modo, "-", "")
		root, wt := cdwTarjeta(t, "", slug)
		comun := cdwComun(t, wt)
		ruta := filepath.Join(comun, "info", "attributes")
		material := "mkdir -p '" + filepath.Dir(ruta) + "'\nprintf '*.go filter=r1\\n' > '" + ruta + "'\n"
		switch modo {
		case "lock":
			lock := filepath.Join(comun, "refs", "heads", "hoom", slug+".lock")
			apr4Hook(t, wt, "post-commit", "#!/bin/sh\n"+material+": > '"+lock+"'\nexit 0\n")
		default:
			armado := filepath.Join(t.TempDir(), "armado")
			apr4Hook(t, wt, "post-commit", "#!/bin/sh\n"+material+": > '"+armado+"'\nexit 0\n")
			apr4Hook(t, wt, "reference-transaction", "#!/bin/sh\n[ -f '"+armado+"' ] && [ \"$1\" = prepared ] && exit 1\nexit 0\n")
		}
		head := apHead(t, wt)
		res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, slug)
		want := itemcmd.ErrHookCambioMaterial([]string{ruta})
		if err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Fatalf("%s: %s: el guardado falla con un error que empieza con %q: %+v %v", ca, modo, want, res, err)
		}
		volvio := apGit(t, wt, "rev-parse", "refs/heads/hoom/"+slug) == head
		if volvio {
			if modo == "lock" {
				t.Fatalf("%s: %s: fixture: con el lock tomado la rama no puede volver, y volvio: %v", ca, modo, err)
			}
			t.Logf("%s: %s: la rama volvio (el deshacer no corre los hooks): nada que probar", ca, modo)
			continue
		}
		if strings.Contains(err.Error(), "vuelve a") {
			t.Fatalf("%s: %s: la rama no volvio y el error no dice `vuelve a`: %v", ca, modo, err)
		}
		if !r1NoVolvio.MatchString(err.Error()) {
			t.Fatalf("%s: %s: la rama no volvio y el error lo dice: %v", ca, modo, err)
		}
	}
}
