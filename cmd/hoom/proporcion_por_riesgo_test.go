// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md contra el
// BINARIO:
//
//   - CA-792: un spec.topes invalido hace fallar `hoom verify` con su texto
//     exacto, sin correr gates ni escribir veredicto;
//   - CA-793 y CA-794: `hoom verify --spec` mide el spec sin aprobacion
//     vigente contra su tope y deja quieto al aprobado;
//   - CA-795: `hoom spec approve` y `hoom aprobar` salen con 1 sin registro, y
//     aprueban despues de `hoom item set --tope-palabras`;
//   - CA-796: `hoom item set --tope-*` escribe la tarjeta o sale con 1 sin
//     tocarla;
//   - CA-797: la linea de `hoom spec status` y sus exit codes (CA-12);
//   - CA-802: el aviso de citas en la salida de texto, sin cambiar el exit.
//
// Compila el binario: se omite con -short. Escritos desde el spec, las
// aclaraciones de la ronda 0 y las firmas publicas, sin leer la
// implementacion.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	hverdict "github.com/hoomdev/hoomai/internal/verdict"
)

const (
	pprSlug     = "precios"
	pprRuta     = ".hoom/specs/precios.md"
	pprItemRel  = ".hoom/items/precios.yaml"
	pprPorQue   = `--motivo "<por que>"`
	pprItemBase = "titulo: Precios\ntipo: feature\nprioridad: media\ncreado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-11-20T10:00:00Z\n"
)

// pprSpec es un spec completo (pasa Lint) con exactamente palabras palabras
// (strings.Fields sobre el archivo entero) y k criterios.
func pprSpec(t *testing.T, palabras, k int) string {
	t.Helper()
	var crit strings.Builder
	for i := 1; i <= k; i++ {
		fmt.Fprintf(&crit, "- CA-%d: criterio %d.\n", i, i)
	}
	body := "# Spec\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\n%C%\n\n## Casos limite\nx\n\n## Criterios de aceptacion\n\n" +
		crit.String() + "\n## Decisiones\nx\n\n## Riesgos\nx\n"
	base := len(strings.Fields(strings.Replace(body, "%C%", "x", 1)))
	if palabras < base {
		t.Fatalf("fixture: el esqueleto ya tiene %d palabras", base)
	}
	body = strings.Replace(body, "%C%", "x"+strings.Repeat(" palabra", palabras-base), 1)
	if n := len(strings.Fields(body)); n != palabras {
		t.Fatalf("fixture: el spec tiene %d palabras, no %d", n, palabras)
	}
	return body
}

// pprPruebas es un archivo de pruebas que cita una vez cada criterio CA-1..k
// y, en total, citasDelUno lineas a CA-1.
func pprPruebas(k, citasDelUno int) string {
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 1; i <= k; i++ {
		fmt.Fprintf(&b, "// CA-%d\n", i)
	}
	for i := 1; i < citasDelUno; i++ {
		fmt.Fprintf(&b, "// otra cita de CA-1 (%d)\n", i)
	}
	return b.String()
}

// pprProyecto es veeRepo (con yamlExtra antes de los gates y un gate test
// verde) con el spec precios de palabras/criterios, sus pruebas y, si
// itemExtra no es "-", su tarjeta (pprItemBase + itemExtra), todo commiteado.
func pprProyecto(t *testing.T, yamlExtra, itemExtra string, palabras, k, citasDelUno int) string {
	t.Helper()
	dir := veeRepo(t, yamlExtra, veeGate("test", "true"))
	veeEscribir(t, dir, pprRuta, pprSpec(t, palabras, k), 0o644)
	veeEscribir(t, dir, "precios_test.go", pprPruebas(k, citasDelUno), 0o644)
	if itemExtra != "-" {
		veeEscribir(t, dir, pprItemRel, pprItemBase+itemExtra, 0o644)
	}
	qcGit(t, dir, "add", "-A")
	qcGit(t, dir, "commit", "-q", "-m", "spec precios")
	return dir
}

// pprTarea es aehTarjeta con el spec precios de palabras palabras y un
// criterio: el item en el checkout principal, la tarea abierta por el binario
// y el spec guardado en su espacio de trabajo.
func pprTarea(t *testing.T, bin string, palabras int) (root, wt string) {
	t.Helper()
	root = vdtRepo(t, "findings:\n  block_on: high\n")
	aehEscribir(t, root, pprItemRel, pprItemBase+"pedido: el pedido de precios\n")
	qcGit(t, root, "add", "-A")
	qcGit(t, root, "commit", "-q", "-m", "item precios")
	if s := qcCorrer(t, bin, root, nil, "task", "start", pprSlug); s.exit != 0 {
		t.Fatalf("fixture: hoom task start %s: %s", pprSlug, s.todo())
	}
	wt = filepath.Join(root, ".hoom", "worktrees", pprSlug)
	aehEscribir(t, wt, pprRuta, pprSpec(t, palabras, 1))
	aehEscribir(t, wt, "precios_test.go", pprPruebas(1, 1))
	qcGit(t, wt, "add", "-A")
	qcGit(t, wt, "commit", "-q", "-m", "spec sin aprobar")
	return root, wt
}

func pprLineaPalabras(p int, r string, tope int) string {
	return fmt.Sprintf("el spec tiene %d palabras y el tope con riesgo %s es %d: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set %s --tope-palabras %d %s", p, r, tope, pprSlug, p, pprPorQue)
}

func pprLineaCriterios(c int, r string, tope int) string {
	return fmt.Sprintf("el spec tiene %d criterios y el tope con riesgo %s es %d: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set %s --tope-criterios %d %s", c, r, tope, pprSlug, c, pprPorQue)
}

// pprIndice es el indice de la linea no vacia de s que, sin los blancos de
// los bordes, es exactamente linea; -1 si ninguna.
func pprIndice(s, linea string) int {
	for i, l := range veeLineasNoVacias(s) {
		if strings.TrimSpace(l) == linea {
			return i
		}
	}
	return -1
}

func pprVeces(s, linea string) int {
	n := 0
	for _, l := range veeLineasNoVacias(s) {
		if strings.TrimSpace(l) == linea {
			n++
		}
	}
	return n
}

// pprVerify corre `hoom verify --spec <precios> --json` en dir.
func pprVerify(t *testing.T, ca, bin, dir string) (qcSalida, hverdict.Verdict) {
	t.Helper()
	r := qcCorrer(t, bin, dir, qcSinCorrida, "verify", "--spec", pprRuta, "--json")
	var v hverdict.Verdict
	if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
		t.Fatalf("%s: stdout es el veredicto: %v\n%s", ca, err, r.todo())
	}
	return r, v
}

func pprGateDe(t *testing.T, ca string, v hverdict.Verdict, nombre string) hverdict.GateResult {
	t.Helper()
	for _, g := range v.Gates {
		if g.Name == nombre {
			return g
		}
	}
	t.Fatalf("%s: el veredicto con --spec trae %s: %+v", ca, nombre, v.Gates)
	return hverdict.GateResult{}
}

func pprRegistros(dir string) int {
	es, _ := os.ReadDir(filepath.Join(dir, ".hoom", "approvals"))
	return len(es)
}

func pprLeer(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// pprValor es el valor de la clave de primer nivel clave en un YAML simple,
// sin comillas.
func pprValor(raw, clave string) (string, bool) {
	for _, l := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(l, clave+":") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(l, clave+":"))
		if u, err := strconv.Unquote(v); err == nil && strings.HasPrefix(v, `"`) {
			return u, true
		}
		if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
			return strings.ReplaceAll(v[1:len(v)-1], "''", "'"), true
		}
		return v, true
	}
	return "", false
}

// ---------------------------------------------------------------- CA-792

// CA-792: con cada error de spec.topes (los cinco, y los tres <donde> de
// "debe ser un mapa"), `hoom verify` (texto, --json y --spec) sale distinto
// de 0 con `hoom.yaml invalido: <texto>`, sin correr ningun gate ni escribir
// veredicto.
func TestCA792_VerifyConSpecTopesInvalido(t *testing.T) {
	const ca = "CA-792"
	veeSaltar(t)
	bin := qcHoom(t)
	for _, c := range []struct{ yaml, want string }{
		{"spec:\n  topez: 1\n", `spec: clave desconocida "topez" (validas: topes)`},
		{"spec:\n  topes:\n    medio: { palabras: 1 }\n", `spec.topes: clave desconocida "medio" (validas: bajo, normal, alto)`},
		{"spec:\n  topes:\n    normal: { lineas: 1 }\n", `spec.topes.normal: clave desconocida "lineas" (validas: palabras, criterios, citas_por_criterio)`},
		{"spec:\n  topes:\n    normal: { palabras: -1 }\n", "spec.topes.normal.palabras: -1 no es un entero mayor o igual que 0 (0 = sin tope)"},
		{"spec:\n  topes:\n    alto: { citas_por_criterio: ocho }\n", "spec.topes.alto.citas_por_criterio: ocho no es un entero mayor o igual que 0 (0 = sin tope)"},
		{"spec:\n  topes:\n    bajo: { criterios: 1.5 }\n", "spec.topes.bajo.criterios: 1.5 no es un entero mayor o igual que 0 (0 = sin tope)"},
		{"spec: 3\n", "spec debe ser un mapa"},
		{"spec:\n  topes: [1]\n", "spec.topes debe ser un mapa"},
		{"spec:\n  topes:\n    bajo: 7\n", "spec.topes.bajo debe ser un mapa"},
	} {
		for _, args := range [][]string{{"verify"}, {"verify", "--json"}, {"verify", "--spec", pprRuta}} {
			dir := veeRepo(t, c.yaml, veeGate("test", "touch ppr-corrio"))
			veeEscribir(t, dir, pprRuta, pprSpec(t, 60, 1), 0o644)
			qcGit(t, dir, "add", "-A")
			qcGit(t, dir, "commit", "-q", "-m", "spec")
			r := qcCorrer(t, bin, dir, qcSinCorrida, args...)
			if r.exit == 0 || !strings.Contains(r.todo(), "hoom.yaml invalido: "+c.want) {
				t.Fatalf("%s: %q con %q: sale distinto de 0 con %q: exit %d\n%s", ca, args, c.yaml, "hoom.yaml invalido: "+c.want, r.exit, r.todo())
			}
			if n := veeVeredictos(dir); n != 0 {
				t.Fatalf("%s: %q con %q: no queda veredicto: %d", ca, args, c.yaml, n)
			}
			if _, err := os.Stat(filepath.Join(dir, "ppr-corrio")); err == nil {
				t.Fatalf("%s: %q con %q: ningun gate corre", ca, args, c.yaml)
			}
		}
	}
}

// CA-792: un spec.topes valido con claves que faltan y un 0 (sin tope) carga
// como sin la seccion: el gate corre, queda un veredicto y el exit es el
// mismo.
func TestCA792_VerifyConSpecTopesValido(t *testing.T) {
	const ca = "CA-792"
	veeSaltar(t)
	bin := qcHoom(t)
	var exits []int
	for _, extra := range []string{"", "spec:\n  topes:\n    alto: { palabras: 0 }\n    normal: { citas_por_criterio: 0 }\n"} {
		dir := veeRepo(t, extra, veeGate("test", "touch ppr-corrio"))
		r := qcCorrer(t, bin, dir, qcSinCorrida, "verify")
		if strings.Contains(r.todo(), "hoom.yaml invalido") {
			t.Fatalf("%s: %q carga: %s", ca, extra, r.todo())
		}
		if _, err := os.Stat(filepath.Join(dir, "ppr-corrio")); err != nil || veeVeredictos(dir) != 1 {
			t.Fatalf("%s: %q: el gate corre y queda un veredicto (%v, %d)\n%s", ca, extra, err, veeVeredictos(dir), r.todo())
		}
		exits = append(exits, r.exit)
	}
	if exits[0] != exits[1] {
		t.Fatalf("%s: sin la seccion y con ella, el mismo exit: %v", ca, exits)
	}
}

// ---------------------------------------------------------------- CA-793

// CA-793: `hoom verify --spec` mide el spec sin aprobar: 801 palabras con
// una tarjeta normal es fail con la linea exacta (rojo, exit distinto de 0) y
// 800 pasa; 9 criterios fallan y 8 pasan; bajo y alto usan sus topes.
func TestCA793_VerifySpecMideContraElTope(t *testing.T) {
	const ca = "CA-793"
	veeSaltar(t)
	bin := qcHoom(t)
	for _, c := range []struct {
		nombre, item     string
		palabras, k      int
		falla, nota, sin string
	}{
		{"normal 801 palabras", "", 801, 1, pprLineaPalabras(801, "normal", 800), "", "criterios y el tope"},
		{"normal 800 palabras", "", 800, 1, "", "800 palabras de 800 · 1 criterios de 8 · riesgo normal · 1 ids CA-n", ""},
		{"normal 9 criterios", "", 120, 9, pprLineaCriterios(9, "normal", 8), "", "palabras y el tope"},
		{"normal 8 criterios", "", 120, 8, "", "120 palabras de 800 · 8 criterios de 8 · riesgo normal · 8 ids CA-n", ""},
		{"bajo 401 palabras", "riesgo: bajo\n", 401, 1, pprLineaPalabras(401, "bajo", 400), "", ""},
		{"bajo 5 criterios", "riesgo: bajo\n", 120, 5, pprLineaCriterios(5, "bajo", 4), "", ""},
		{"bajo 400 y 4", "riesgo: bajo\n", 400, 4, "", "400 palabras de 400 · 4 criterios de 4 · riesgo bajo · 4 ids CA-n", ""},
		{"alto 2000 y 15", "riesgo: alto\n", 2000, 15, "", "2000 palabras de 2000 · 15 criterios de 15 · riesgo alto · 15 ids CA-n", ""},
		{"alto 2001 palabras", "riesgo: alto\n", 2001, 1, pprLineaPalabras(2001, "alto", 2000), "", ""},
		{"alto 16 criterios", "riesgo: alto\n", 300, 16, pprLineaCriterios(16, "alto", 15), "", ""},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			dir := pprProyecto(t, "", c.item, c.palabras, c.k, 1)
			r, v := pprVerify(t, ca, bin, dir)
			lint := pprGateDe(t, ca, v, "spec_lint")
			if c.falla != "" {
				if lint.Status != hverdict.StatusFail || pprIndice(lint.OutputTail, c.falla) < 0 {
					t.Fatalf("%s: spec_lint en fail con\n%s\nfue %s:\n%s", ca, c.falla, lint.Status, lint.OutputTail)
				}
				if c.sin != "" && strings.Contains(lint.OutputTail, c.sin) {
					t.Fatalf("%s: solo una linea por medida que pasa: %q no va:\n%s", ca, c.sin, lint.OutputTail)
				}
				if v.Verdict != "red" || r.exit == 0 {
					t.Fatalf("%s: pasar el tope es rojo: %s exit %d", ca, v.Verdict, r.exit)
				}
				return
			}
			if lint.Status != hverdict.StatusPass || !strings.HasPrefix(lint.Notes, c.nota) {
				t.Fatalf("%s: el tope exacto pasa, con la nota %q: %s %q\n%s", ca, c.nota, lint.Status, lint.Notes, lint.OutputTail)
			}
		})
	}
}

// CA-793: sin tarjeta vale normal y la linea nombra spec.topes; con el tope
// de spec.topes del hoom.yaml que carga el comando; una tarjeta ilegible deja
// spec_lint en error con su texto, y spec_trace corre igual (aclaracion 4).
func TestCA793_VerifySpecSinTarjetaSpecTopesEIlegible(t *testing.T) {
	const ca = "CA-793"
	veeSaltar(t)
	bin := qcHoom(t)

	dir := pprProyecto(t, "", "-", 801, 9, 1)
	_, v := pprVerify(t, ca, bin, dir)
	lint := pprGateDe(t, ca, v, "spec_lint")
	for _, w := range []string{
		"el spec tiene 801 palabras y el tope con riesgo normal es 800: reduce el alcance o sube spec.topes.normal.palabras en hoom.yaml",
		"el spec tiene 9 criterios y el tope con riesgo normal es 8: reduce el alcance o sube spec.topes.normal.criterios en hoom.yaml",
	} {
		if lint.Status != hverdict.StatusFail || pprIndice(lint.OutputTail, w) < 0 {
			t.Fatalf("%s: sin tarjeta, normal y spec.topes:\n%s\nfue %s:\n%s", ca, w, lint.Status, lint.OutputTail)
		}
	}

	dir = pprProyecto(t, "spec:\n  topes:\n    normal: { palabras: 100 }\n", "", 101, 1, 1)
	_, v = pprVerify(t, ca, bin, dir)
	if lint := pprGateDe(t, ca, v, "spec_lint"); lint.Status != hverdict.StatusFail || pprIndice(lint.OutputTail, pprLineaPalabras(101, "normal", 100)) < 0 {
		t.Fatalf("%s: el tope sale del hoom.yaml que carga el comando: %s\n%s", ca, lint.Status, lint.OutputTail)
	}

	dir = pprProyecto(t, "", "riesgo: [alto\n", 120, 1, 1)
	r, v := pprVerify(t, ca, bin, dir)
	if lint := pprGateDe(t, ca, v, "spec_lint"); lint.Status != hverdict.StatusError || !strings.HasPrefix(lint.Notes, "no se puede leer la tarjeta precios: ") {
		t.Fatalf("%s: la tarjeta ilegible da error con su texto: %s %q", ca, lint.Status, lint.Notes)
	}
	if tr := pprGateDe(t, ca, v, "spec_trace"); tr.Status != hverdict.StatusPass {
		t.Fatalf("%s (aclaracion 4): spec_trace corre igual con la tarjeta ilegible: %+v", ca, tr)
	}
	if r.exit == 0 {
		t.Fatalf("%s: spec_lint en error no es verde: exit 0", ca)
	}
}

// ---------------------------------------------------------------- CA-794

// CA-794: un spec aprobado (con un tope 0 en su momento) que pasa el tope de
// hoy queda en pass con la nota del aprobado; cambiado despues de aprobarse,
// se mide y falla.
func TestCA794_VerifySpecAprobadoNoSeMideYEditadoSi(t *testing.T) {
	const ca = "CA-794"
	veeSaltar(t)
	bin := qcHoom(t)
	dir := pprProyecto(t, "spec:\n  topes:\n    normal: { palabras: 0 }\n", "", 801, 1, 1)
	if s := qcCorrer(t, bin, dir, qcSinCorrida, "spec", "approve", pprRuta); s.exit != 0 {
		t.Fatalf("%s: fixture: sin tope la aprobacion firma: exit %d\n%s", ca, s.exit, s.todo())
	}
	veeEscribir(t, dir, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+veeGate("test", "true"), 0o644)
	qcGit(t, dir, "commit", "-q", "-am", "vuelve el tope por defecto")

	_, v := pprVerify(t, ca, bin, dir)
	lint := pprGateDe(t, ca, v, "spec_lint")
	if lint.Status != hverdict.StatusPass || !strings.HasSuffix(lint.Notes, " · aprobado: el tope no se mide") ||
		!strings.HasPrefix(lint.Notes, "801 palabras de 800 · 1 criterios de 8 · riesgo normal · ") {
		t.Fatalf("%s: el aprobado que pasa su tope queda en pass con la nota: %s %q\n%s", ca, lint.Status, lint.Notes, lint.OutputTail)
	}

	veeEscribir(t, dir, pprRuta, pprSpec(t, 802, 1), 0o644)
	qcGit(t, dir, "commit", "-q", "-am", "el spec cambia despues de aprobarse")
	_, v = pprVerify(t, ca, bin, dir)
	if lint := pprGateDe(t, ca, v, "spec_lint"); lint.Status != hverdict.StatusFail || pprIndice(lint.OutputTail, pprLineaPalabras(802, "normal", 800)) < 0 {
		t.Fatalf("%s: cambiado despues de aprobarse se mide y falla: %s\n%s", ca, lint.Status, lint.OutputTail)
	}
}

// ---------------------------------------------------------------- CA-795

// CA-795: `hoom spec approve` con un spec que pasa su tope sale con 1, las
// lineas de spec_lint y ningun registro; despues de `hoom item set
// --tope-palabras` aprueba (exit 0, registro, status aprobado).
func TestCA795_SpecApproveSeNiegaYDespuesDeSubirElTope(t *testing.T) {
	const ca = "CA-795"
	veeSaltar(t)
	bin := qcHoom(t)
	root, wt := pprTarea(t, bin, 801)
	s := qcCorrer(t, bin, wt, qcSinCorrida, "spec", "approve", pprRuta)
	if s.exit != 1 || pprIndice(s.todo(), pprLineaPalabras(801, "normal", 800)) < 0 {
		t.Fatalf("%s: hoom spec approve sale con 1 y la linea de spec_lint: exit %d\n%s", ca, s.exit, s.todo())
	}
	if n := pprRegistros(root) + pprRegistros(wt); n != 0 {
		t.Fatalf("%s: la negativa no escribe registro: %d", ca, n)
	}
	if st := qcCorrer(t, bin, wt, qcSinCorrida, "spec", "status", pprRuta); st.exit != 1 {
		t.Fatalf("%s: tras la negativa sigue no-aprobado (exit 1): %d\n%s", ca, st.exit, st.todo())
	}
	if up := qcCorrer(t, bin, root, qcSinCorrida, "item", "set", pprSlug, "--tope-palabras", "801", "--motivo", "el intake es largo"); up.exit != 0 {
		t.Fatalf("%s: fixture: una persona sube el tope: exit %d\n%s", ca, up.exit, up.todo())
	}
	if s := qcCorrer(t, bin, wt, qcSinCorrida, "spec", "approve", pprRuta); s.exit != 0 {
		t.Fatalf("%s: con el tope subido aprueba: exit %d\n%s", ca, s.exit, s.todo())
	}
	if n := pprRegistros(root) + pprRegistros(wt); n == 0 {
		t.Fatalf("%s: la aprobacion escribe su registro", ca)
	}
	if st := qcCorrer(t, bin, wt, qcSinCorrida, "spec", "status", pprRuta); st.exit != 0 {
		t.Fatalf("%s: aprobado: status sale con 0: %d\n%s", ca, st.exit, st.todo())
	}
}

// CA-795: `hoom aprobar <slug> --si` igual: 1 sin registro, y aprueba
// despues de subir el tope de criterios y de palabras de la tarjeta.
func TestCA795_AprobarSeNiegaYDespuesDeSubirElTope(t *testing.T) {
	const ca = "CA-795"
	veeSaltar(t)
	bin := qcHoom(t)
	root, wt := pprTarea(t, bin, 801)
	s := qcCorrer(t, bin, root, qcSinCorrida, "aprobar", pprSlug, "--si")
	if s.exit != 1 || pprIndice(s.todo(), pprLineaPalabras(801, "normal", 800)) < 0 {
		t.Fatalf("%s: hoom aprobar sale con 1 y la linea de spec_lint: exit %d\n%s", ca, s.exit, s.todo())
	}
	if n := pprRegistros(root) + pprRegistros(wt); n != 0 {
		t.Fatalf("%s: la negativa de hoom aprobar no escribe registro: %d", ca, n)
	}
	// subir otro tope (criterios) no alcanza: sigue negandose
	if up := qcCorrer(t, bin, root, qcSinCorrida, "item", "set", pprSlug, "--tope-criterios", "20", "--motivo", "muchos criterios"); up.exit != 0 {
		t.Fatalf("%s: fixture: item set --tope-criterios: %d\n%s", ca, up.exit, up.todo())
	}
	if s := qcCorrer(t, bin, root, qcSinCorrida, "aprobar", pprSlug, "--si"); s.exit != 1 || pprRegistros(root)+pprRegistros(wt) != 0 {
		t.Fatalf("%s: con solo el tope de criterios subido sigue negandose: exit %d\n%s", ca, s.exit, s.todo())
	}
	if up := qcCorrer(t, bin, root, qcSinCorrida, "item", "set", pprSlug, "--tope-palabras", "0", "--motivo", "sin tope para esta"); up.exit != 0 {
		t.Fatalf("%s: fixture: item set --tope-palabras 0: %d\n%s", ca, up.exit, up.todo())
	}
	if s := qcCorrer(t, bin, root, qcSinCorrida, "aprobar", pprSlug, "--si"); s.exit != 0 || pprRegistros(root)+pprRegistros(wt) == 0 {
		t.Fatalf("%s: con el tope de palabras de la tarjeta en 0 aprueba y escribe: exit %d\n%s", ca, s.exit, s.todo())
	}
}

// ---------------------------------------------------------------- CA-796

// CA-796: cada negativa de `hoom item set --tope-*` sale con 1 y su texto,
// sin tocar el archivo de la tarjeta (ni el arbol de git).
func TestCA796_ItemSetTopeSeNiegaSinTocarElArchivo(t *testing.T) {
	const ca = "CA-796"
	veeSaltar(t)
	bin := qcHoom(t)
	root := vdtRepo(t, "")
	aehEscribir(t, root, pprItemRel, pprItemBase)
	qcGit(t, root, "add", "-A")
	qcGit(t, root, "commit", "-q", "-m", "item precios")
	p := filepath.Join(root, pprItemRel)
	antes := pprLeer(t, p)
	sinMotivo := `item set: cambiar el tope del spec pide --motivo "<por que>"`
	rol := "el tope del spec de una tarjeta lo cambia una persona, no un rol"
	for _, c := range []struct {
		nombre string
		env    map[string]string
		args   []string
		want   string
	}{
		{"sin motivo", qcSinCorrida, []string{"--tope-palabras", "900"}, sinMotivo},
		{"sin motivo, criterios", qcSinCorrida, []string{"--tope-criterios", "12"}, sinMotivo},
		{"motivo en blanco", qcSinCorrida, []string{"--tope-palabras", "900", "--motivo", "   "}, sinMotivo},
		{"motivo vacio", qcSinCorrida, []string{"--tope-criterios", "12", "--motivo", ""}, sinMotivo},
		{"negativo", qcSinCorrida, []string{"--tope-palabras", "-1", "--motivo", "x"}, "--tope-palabras tiene que ser un entero mayor o igual que 0"},
		{"no entero", qcSinCorrida, []string{"--tope-criterios", "ocho", "--motivo", "x"}, "--tope-criterios tiene que ser un entero mayor o igual que 0"},
		{"decimal", qcSinCorrida, []string{"--tope-palabras", "1.5", "--motivo", "x"}, "--tope-palabras tiene que ser un entero mayor o igual que 0"},
		{"motivo sin tope", qcSinCorrida, []string{"--motivo", "x"}, "item set: --motivo va con --tope-palabras o --tope-criterios"},
		{"con HOOM_ROLE", map[string]string{"HOOM_ROLE": "writer"}, []string{"--tope-palabras", "900", "--motivo", "x"}, rol},
		{"con HOOM_ROLE arquitecto", map[string]string{"HOOM_ROLE": "arquitecto"}, []string{"--tope-criterios", "12", "--motivo", "x"}, rol},
	} {
		s := qcCorrer(t, bin, root, c.env, append([]string{"item", "set", pprSlug}, c.args...)...)
		if s.exit != 1 || !strings.Contains(s.todo(), c.want) {
			t.Fatalf("%s: %s %q: exit 1 con %q: exit %d\n%s", ca, c.nombre, c.args, c.want, s.exit, s.todo())
		}
		if got := pprLeer(t, p); got != antes {
			t.Fatalf("%s: %s: ningun error toca el archivo:\nantes:\n%s\ndespues:\n%s", ca, c.nombre, antes, got)
		}
	}
	if s := e1Git(t, root, "status", "--porcelain", "--", pprItemRel); strings.TrimSpace(s) != "" {
		t.Fatalf("%s: la tarjeta queda como estaba: %q", ca, s)
	}
}

// CA-796: `hoom item set --tope-palabras --tope-criterios --motivo` escribe
// las cinco claves (tope_por de git, tope_en UTC RFC 3339), dice el slug y el
// comando para guardarlo; con --json, SetResult trae tope_palabras,
// tope_criterios y tope_motivo (aclaracion 6).
func TestCA796_ItemSetTopeEscribeLaTarjeta(t *testing.T) {
	const ca = "CA-796"
	veeSaltar(t)
	bin := qcHoom(t)
	root := vdtRepo(t, "")
	aehEscribir(t, root, pprItemRel, pprItemBase)
	qcGit(t, root, "add", "-A")
	qcGit(t, root, "commit", "-q", "-m", "item precios")
	desde := time.Now().UTC().Truncate(time.Second)
	s := qcCorrer(t, bin, root, qcSinCorrida, "item", "set", pprSlug, "--tope-palabras", "900", "--tope-criterios", "12", "--motivo", "el intake es largo")
	hasta := time.Now().UTC().Add(time.Second)
	if s.exit != 0 || !strings.Contains(s.todo(), "de "+pprSlug) || !strings.Contains(s.todo(), "hoom item save "+pprSlug) {
		t.Fatalf("%s: exit 0, el slug y hoom item save: exit %d\n%s", ca, s.exit, s.todo())
	}
	raw := pprLeer(t, filepath.Join(root, pprItemRel))
	for clave, want := range map[string]string{
		"tope_palabras": "900", "tope_criterios": "12", "tope_motivo": "el intake es largo",
		"tope_por": "Persona Git <persona@hoom.dev>", "titulo": "Precios",
	} {
		if v, ok := pprValor(raw, clave); !ok || v != want {
			t.Fatalf("%s: %s es %q, fue %q (%v):\n%s", ca, clave, want, v, ok, raw)
		}
	}
	v, ok := pprValor(raw, "tope_en")
	en, err := time.Parse(time.RFC3339, v)
	if !ok || err != nil || !strings.HasSuffix(v, "Z") || en.Before(desde) || en.After(hasta) {
		t.Fatalf("%s: tope_en en UTC, RFC 3339, ahora: %q (%v)\n%s", ca, v, err, raw)
	}

	j := qcCorrer(t, bin, root, qcSinCorrida, "item", "set", pprSlug, "--tope-palabras", "0", "--motivo", "sin tope", "--json")
	var res map[string]any
	if j.exit != 0 || json.Unmarshal([]byte(j.stdout), &res) != nil {
		t.Fatalf("%s: --json: exit 0 y un JSON: %d\n%s", ca, j.exit, j.todo())
	}
	if n, ok := res["tope_palabras"].(float64); !ok || n != 0 {
		t.Fatalf("%s: SetResult trae tope_palabras 0 (sin tope, distinto de no tenerlo): %v", ca, res)
	}
	if m, _ := res["tope_motivo"].(string); m != "sin tope" {
		t.Fatalf("%s: SetResult trae tope_motivo: %v", ca, res)
	}
	if n, ok := res["tope_criterios"].(float64); !ok || n != 12 {
		t.Fatalf("%s: SetResult trae el tope_criterios de la tarjeta despues del set (12): %v", ca, res)
	}
	if v, _ := pprValor(pprLeer(t, filepath.Join(root, pprItemRel)), "tope_palabras"); v != "0" {
		t.Fatalf("%s: tope_palabras: 0 queda escrito (0 es sin tope para la tarjeta): %q", ca, v)
	}
}

// ---------------------------------------------------------------- CA-797

// pprStatus corre `hoom spec status <precios>` y exige la linea want una sola
// vez, despues de otra linea de su mismo stream (la de hoy).
func pprStatus(t *testing.T, ca, caso, bin, dir, want string, exit int) {
	t.Helper()
	s := qcCorrer(t, bin, dir, qcSinCorrida, "spec", "status", pprRuta)
	if s.exit != exit {
		t.Fatalf("%s (%s): los exit codes de CA-12: %d, fue %d\n%s", ca, caso, exit, s.exit, s.todo())
	}
	stream := s.stdout
	i := pprIndice(stream, want)
	if i < 0 {
		stream = s.stderr
		i = pprIndice(stream, want)
	}
	if i < 1 || pprVeces(s.todo(), want) != 1 {
		t.Fatalf("%s (%s): una vez, despues de la linea de hoy:\n%s\nfue:\n%s", ca, caso, want, s.todo())
	}
}

// CA-797: la linea de `hoom spec status` con sus variantes, y los exit codes
// de CA-12 (0 solo aprobado; no-aprobado e invalidado, 1).
func TestCA797_SpecStatusConLaLineaDeLasMedidas(t *testing.T) {
	const ca = "CA-797"
	veeSaltar(t)
	bin := qcHoom(t)
	const cardTope = "tope_palabras: 500\ntope_motivo: \"el intake\"\ntope_por: \"Ana <ana@hoom.dev>\"\ntope_en: 2026-11-24T10:00:05Z\n"

	pprStatus(t, ca, "alto", bin, pprProyecto(t, "", "riesgo: alto\n", 120, 1, 1),
		"hoom spec: 120 palabras de 2000 · 1 criterios de 15 (riesgo alto)", 1)
	pprStatus(t, ca, "de la tarjeta", bin, pprProyecto(t, "", cardTope, 120, 1, 1),
		"hoom spec: 120 palabras de 500 de la tarjeta · 1 criterios de 8 (riesgo normal)", 1)
	pprStatus(t, ca, "sin tope", bin, pprProyecto(t, "spec:\n  topes:\n    bajo: { palabras: 0, criterios: 0 }\n", "riesgo: bajo\n", 120, 1, 1),
		"hoom spec: 120 palabras sin tope · 1 criterios sin tope (riesgo bajo)", 1)
	pprStatus(t, ca, "sin tarjeta", bin, pprProyecto(t, "", "-", 120, 1, 1),
		"hoom spec: 120 palabras de 800 · 1 criterios de 8 (riesgo normal, sin tarjeta)", 1)
	pprStatus(t, ca, "pasa su tope", bin, pprProyecto(t, "", "", 900, 9, 1),
		"hoom spec: 900 palabras de 800 · 9 criterios de 8 (riesgo normal)", 1)

	dir := pprProyecto(t, "", "riesgo: [alto\n", 120, 1, 1)
	s := qcCorrer(t, bin, dir, qcSinCorrida, "spec", "status", pprRuta)
	pre := "hoom spec: no se puede medir el tope: no se puede leer la tarjeta precios: "
	var hay int
	for _, l := range veeLineasNoVacias(s.todo()) {
		if strings.HasPrefix(strings.TrimSpace(l), pre) {
			hay++
		}
	}
	if s.exit != 1 || hay != 1 {
		t.Fatalf("%s (tarjeta ilegible): exit 1 y una linea %q...: exit %d\n%s", ca, pre, s.exit, s.todo())
	}

	dir = pprProyecto(t, "", "riesgo: alto\n", 120, 1, 1)
	if a := qcCorrer(t, bin, dir, qcSinCorrida, "spec", "approve", pprRuta); a.exit != 0 {
		t.Fatalf("%s: fixture: aprueba: %d\n%s", ca, a.exit, a.todo())
	}
	pprStatus(t, ca, "aprobado", bin, dir, "hoom spec: 120 palabras de 2000 · 1 criterios de 15 (riesgo alto) · aprobado: el tope no se mide", 0)
	veeEscribir(t, dir, pprRuta, pprSpec(t, 121, 1), 0o644)
	pprStatus(t, ca, "invalidado", bin, dir, "hoom spec: 121 palabras de 2000 · 1 criterios de 15 (riesgo alto)", 1)
}

// ---------------------------------------------------------------- CA-802

// CA-802 (y CA-801 por el binario): con una tarjeta normal y CA-1 citado en
// 11 lineas, `hoom verify --spec` imprime el aviso exacto una vez; con 10 no
// hay aviso; el estado del veredicto, el de spec_trace y el exit code son los
// mismos. Con --json el aviso no sale aparte (aclaracion 1) y la nota de
// spec_trace trae las citas y termina con el tope pasado.
func TestCA802_VerifyAvisoDeCitasSinCambiarElExit(t *testing.T) {
	const ca = "CA-802"
	veeSaltar(t)
	bin := qcHoom(t)
	aviso := "! 1 criterios pasan el tope de 10 citas en pruebas (riesgo normal): CA-1 11"

	con := pprProyecto(t, "", "", 120, 2, 11)
	sin := pprProyecto(t, "", "", 120, 2, 10)
	rc := qcCorrer(t, bin, con, qcSinCorrida, "verify", "--spec", pprRuta)
	rs := qcCorrer(t, bin, sin, qcSinCorrida, "verify", "--spec", pprRuta)
	if pprVeces(rc.todo(), aviso) != 1 {
		t.Fatalf("%s: con 11 citas, el aviso exacto una vez:\n%s\nfue:\n%s", ca, aviso, rc.todo())
	}
	if strings.Contains(rs.todo(), "criterios pasan el tope") {
		t.Fatalf("%s: con 10 citas (el tope exacto) no hay aviso:\n%s", ca, rs.todo())
	}
	if rc.exit != rs.exit {
		t.Fatalf("%s: el aviso no cambia el exit code: %d con aviso, %d sin", ca, rc.exit, rs.exit)
	}

	jc, vc := pprVerify(t, ca, bin, con)
	_, vs := pprVerify(t, ca, bin, sin)
	if strings.Contains(jc.stderr, aviso) {
		t.Fatalf("%s (aclaracion 1): con --json el aviso no sale aparte:\n%s", ca, jc.stderr)
	}
	tc, ts := pprGateDe(t, ca, vc, "spec_trace"), pprGateDe(t, ca, vs, "spec_trace")
	if tc.Status != hverdict.StatusPass || !strings.Contains(tc.Notes, " · citas por criterio: CA-1 11, CA-2 1") ||
		!strings.HasSuffix(tc.Notes, " · 1 pasan el tope de 10 citas") {
		t.Fatalf("CA-801/%s: la nota de spec_trace trae las citas y termina con el tope pasado: %s %q", ca, tc.Status, tc.Notes)
	}
	if ts.Status != tc.Status || strings.Contains(ts.Notes, "pasan el tope") || !strings.Contains(ts.Notes, " · citas por criterio: CA-1 10, CA-2 1") {
		t.Fatalf("CA-801/%s: sin pasar el tope, la nota trae las citas y nada mas: %s %q", ca, ts.Status, ts.Notes)
	}
	if vc.Verdict != vs.Verdict {
		t.Fatalf("%s: el aviso no cambia el estado del veredicto: %s con aviso, %s sin", ca, vc.Verdict, vs.Verdict)
	}
}

// CA-802: con alto y los valores por defecto (citas_por_criterio 0) no hay
// aviso aunque un criterio tenga 30 citas; con el tope de citas de alto
// puesto en hoom.yaml, si.
func TestCA802_VerifyAltoSinAvisoPorDefecto(t *testing.T) {
	const ca = "CA-802"
	veeSaltar(t)
	bin := qcHoom(t)
	dir := pprProyecto(t, "", "riesgo: alto\n", 120, 1, 30)
	r := qcCorrer(t, bin, dir, qcSinCorrida, "verify", "--spec", pprRuta)
	if strings.Contains(r.todo(), "criterios pasan el tope") {
		t.Fatalf("%s: con alto por defecto no hay aviso:\n%s", ca, r.todo())
	}
	_, v := pprVerify(t, ca, bin, dir)
	if tr := pprGateDe(t, ca, v, "spec_trace"); strings.Contains(tr.Notes, "pasan el tope") || !strings.Contains(tr.Notes, " · citas por criterio: CA-1 30") {
		t.Fatalf("%s: con alto la nota trae las citas, sin tope pasado: %q", ca, tr.Notes)
	}

	dir = pprProyecto(t, "spec:\n  topes:\n    alto: { citas_por_criterio: 5 }\n", "riesgo: alto\n", 120, 1, 30)
	r = qcCorrer(t, bin, dir, qcSinCorrida, "verify", "--spec", pprRuta)
	if w := "! 1 criterios pasan el tope de 5 citas en pruebas (riesgo alto): CA-1 30"; pprVeces(r.todo(), w) != 1 {
		t.Fatalf("%s: con el tope de alto en hoom.yaml, el aviso:\n%s\nfue:\n%s", ca, w, r.todo())
	}
}
