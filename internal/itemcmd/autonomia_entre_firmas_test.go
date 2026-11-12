// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md
// (CA-641) sobre `hoom item set <slug> --auto hasta-humano|no
// --presupuesto-usd x`: Parse es puro (exit 2 sin ninguno de los tres flags
// y con un presupuesto que no es mayor que 0), SetCambios escribe en el
// mismo item que --riesgo (el del checkout principal si lo tiene), no
// commitea y se niega con los textos del contrato; la tarjeta lee auto y
// presupuesto_usd como el riesgo. La linea de texto de set no se fija
// (ACLARACION 3): solo el archivo, el exit code, el JSON y los errores.
package itemcmd

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
)

const (
	aeiFalta      = "falta --riesgo, --auto o --presupuesto-usd: es lo que set cambia"
	aeiMayorQue0  = "--presupuesto-usd tiene que ser un numero mayor que 0"
	aeiSinTope    = "--auto necesita --presupuesto-usd: el piloto automatico no corre sin tope"
	aeiPersona    = "el piloto automatico y el presupuesto de una tarjeta los cambia una persona, no un rol"
	aeiPersonaRsg = "el riesgo de una tarjeta lo cambia una persona, no un rol"
	aeiConTope    = "presupuesto_usd: 5\n"
)

func aeiStr(s string) *string   { return &s }
func aeiUSD(x float64) *float64 { return &x }
func aeiVal(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
func aeiItem(extra string) string { return icItemYAML("Precios", "2026-09-22T15:04:05Z", extra) }

// aeiRepo es el proyecto con el item precios commiteado: con tope en USD
// (conTope) o sin el.
func aeiRepo(t *testing.T, extra string) string {
	t.Helper()
	t.Setenv("HOOM_ROLE", "")
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", aeiItem(extra))
	crCommitearItems(t, root)
	return root
}

// aeiCargar lee el item del arbol dir.
func aeiCargar(t *testing.T, dir string) item.Item {
	t.Helper()
	it, err := item.Load(dir, "precios")
	if err != nil {
		t.Fatalf("fixture: el item de %s se lee: %v", dir, err)
	}
	return it
}

// aeiSinCommitear exige que set no haya commiteado en dir y que el item
// quede cambiado en el arbol de trabajo.
func aeiSinCommitear(t *testing.T, ca, dir, head string) {
	t.Helper()
	if crHead(t, dir) != head {
		t.Fatalf("%s: set no commitea en %s", ca, dir)
	}
	st, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if !strings.Contains(string(st), ".hoom/items/precios.yaml") {
		t.Fatalf("%s: el item de %s queda cambiado sin commitear: %q", ca, dir, st)
	}
}

// aeiIgualesSalvo exige que el item despues tenga las mismas claves que
// antes en todo lo que set no toca.
func aeiIgualesSalvo(t *testing.T, ca string, antes, despues item.Item) {
	t.Helper()
	if despues.Titulo != antes.Titulo || despues.Tipo != antes.Tipo || despues.Prioridad != antes.Prioridad ||
		despues.Pedido != antes.Pedido || despues.CreadoPor != antes.CreadoPor || !despues.CreadoEn.Equal(antes.CreadoEn) {
		t.Fatalf("%s: set cambia solo auto, presupuesto_usd y riesgo:\nantes   %+v\ndespues %+v", ca, antes, despues)
	}
}

// CA-641: Parse de set lleva --auto y --presupuesto-usd, solos o junto a
// --riesgo, con los flags antes o despues del slug.
func TestCA641_ParseSetConAutoYPresupuesto(t *testing.T) {
	for _, c := range []struct {
		args   []string
		riesgo string
		auto   string
		usd    *float64
		json   bool
	}{
		{[]string{"set", "precios", "--auto", "hasta-humano"}, "", item.AutoHastaHumano, nil, false},
		{[]string{"set", "precios", "--auto", "no"}, "", AutoNo, nil, false},
		{[]string{"set", "precios", "--auto=no", "--json"}, "", AutoNo, nil, true},
		{[]string{"set", "precios", "--presupuesto-usd", "2.5"}, "", "", aeiUSD(2.5), false},
		{[]string{"set", "--presupuesto-usd=0.01", "precios"}, "", "", aeiUSD(0.01), false},
		{[]string{"set", "precios", "--riesgo", "alto", "--auto", "hasta-humano", "--presupuesto-usd", "3"}, "alto", item.AutoHastaHumano, aeiUSD(3), false},
		{[]string{"set", "--auto", "hasta-humano", "--presupuesto-usd", "7", "--json", "precios"}, "", item.AutoHastaHumano, aeiUSD(7), true},
	} {
		req, err := Parse(c.args)
		if err != nil {
			t.Fatalf("CA-641: %q es valido: %v", c.args, err)
		}
		if req.Sub != SubSet || req.Slug != "precios" || req.Riesgo != c.riesgo || req.Auto != c.auto ||
			!reflect.DeepEqual(aeiVal(req.PresupuestoUSD), aeiVal(c.usd)) || req.JSON != c.json {
			t.Fatalf("CA-641: %q: riesgo %q auto %q presupuesto %v json %v; fue %+v (presupuesto %v)",
				c.args, c.riesgo, c.auto, aeiVal(c.usd), c.json, req, aeiVal(req.PresupuestoUSD))
		}
	}
	// AutoNo es lo que se escribe en la linea de comandos, nunca en el item
	if AutoNo != "no" {
		t.Fatalf("CA-641: --auto no: AutoNo es %q", AutoNo)
	}
}

// CA-641: sin ninguno de los tres flags que cambian algo, error de uso
// (exit 2) con el texto del contrato.
func TestCA641_SetSinNadaQueCambiarEsErrorDeUso(t *testing.T) {
	for _, args := range [][]string{
		{"set", "precios"},
		{"set", "precios", "--json"},
	} {
		_, err := Parse(args)
		icUso(t, "CA-641", args, err)
		if !strings.Contains(err.Error(), aeiFalta) {
			t.Fatalf("CA-641: %q dice %q: %v", args, aeiFalta, err)
		}
	}
}

// CA-641: --presupuesto-usd rechaza con exit 2 lo que no es un numero mayor
// que 0, con el texto del contrato; un --auto fuera del vocabulario o sin
// valor tambien es un error de uso.
func TestCA641_PresupuestoQueNoEsMayorQue0EsErrorDeUso(t *testing.T) {
	for _, v := range []string{"0", "-1", "-0.5", "0.0", "abc", "", "NaN", "Inf", "+Inf", "-Inf", "1,5", "5usd"} {
		args := []string{"set", "precios", "--presupuesto-usd=" + v}
		_, err := Parse(args)
		icUso(t, "CA-641", args, err)
		if !strings.Contains(err.Error(), aeiMayorQue0) {
			t.Fatalf("CA-641: %q dice %q: %v", args, aeiMayorQue0, err)
		}
		// junto a un --auto valido sigue siendo el mismo error
		args = []string{"set", "precios", "--auto", "hasta-humano", "--presupuesto-usd=" + v}
		_, err = Parse(args)
		icUso(t, "CA-641", args, err)
		if !strings.Contains(err.Error(), aeiMayorQue0) {
			t.Fatalf("CA-641: %q dice %q: %v", args, aeiMayorQue0, err)
		}
	}
	for _, args := range [][]string{
		{"set", "precios", "--presupuesto-usd", "0"},
		{"set", "precios", "--presupuesto-usd"},
		{"set", "precios", "--auto"},
		{"set", "precios", "--auto", ""},
		{"set", "precios", "--auto", "si"},
		{"set", "precios", "--auto", "hasta-humano", "--tipo", "bug"},
		{"set", "--auto", "hasta-humano"},
		{"set", "Foo Bar", "--auto", "hasta-humano"},
	} {
		_, err := Parse(args)
		icUso(t, "CA-641", args, err)
	}
}

// CA-641: --auto hasta-humano escribe auto en un item con presupuesto,
// conserva el resto, no commitea, y la segunda vez no escribe.
func TestCA641_AutoEnUnItemConPresupuesto(t *testing.T) {
	root := aeiRepo(t, "pedido: por region\n"+aeiConTope)
	head := crHead(t, root)
	antes := aeiCargar(t, root)

	res, err := SetCambios(root, "main", "high", "precios", item.Cambios{Auto: aeiStr(item.AutoHastaHumano)}, "")
	if err != nil {
		t.Fatalf("CA-641: %v", err)
	}
	if res.Slug != "precios" || !res.Cambio || res.Auto != item.AutoHastaHumano || res.AutoAntes != "" ||
		aeiVal(res.PresupuestoUSD) != 5.0 || aeiVal(res.PresupuestoAntes) != 5.0 || res.Antes != "normal" || res.Riesgo != "normal" {
		t.Fatalf("CA-641: el SetResult trae auto y presupuesto antes y despues: %+v", res)
	}
	despues := aeiCargar(t, root)
	if despues.Auto != item.AutoHastaHumano || aeiVal(despues.PresupuestoUSD) != 5.0 || despues.Riesgo != "normal" {
		t.Fatalf("CA-641: el item queda con auto hasta-humano y su tope: %+v", despues)
	}
	aeiIgualesSalvo(t, "CA-641", antes, despues)
	raw, _ := os.ReadFile(item.Path(root, "precios"))
	if !strings.Contains(string(raw), "auto: hasta-humano\n") {
		t.Fatalf("CA-641: el archivo lleva la clave auto:\n%s", raw)
	}
	aeiSinCommitear(t, "CA-641", root, head)

	// con el que ya tiene: Cambio false y el archivo intacto
	foto := ixFotoItems(t, root)
	st0, _ := os.Stat(item.Path(root, "precios"))
	time.Sleep(10 * time.Millisecond)
	res, err = SetCambios(root, "main", "high", "precios", item.Cambios{Auto: aeiStr(item.AutoHastaHumano)}, "")
	if err != nil || res.Cambio || res.Auto != item.AutoHastaHumano || res.AutoAntes != item.AutoHastaHumano {
		t.Fatalf("CA-641: con el auto que ya tenia no escribe: %+v %v", res, err)
	}
	st1, _ := os.Stat(item.Path(root, "precios"))
	if !ixMismos(foto, ixFotoItems(t, root)) || !st0.ModTime().Equal(st1.ModTime()) {
		t.Fatal("CA-641: con el auto que ya tenia no escribe")
	}
}

// CA-641: --auto hasta-humano en un item sin presupuesto se niega con el
// texto de item add y no escribe; con --presupuesto-usd en el mismo set, se
// escriben los dos.
func TestCA641_AutoSinPresupuestoSeNiega(t *testing.T) {
	root := aeiRepo(t, "")
	foto := ixFotoItems(t, root)
	for _, c := range []item.Cambios{
		{Auto: aeiStr(item.AutoHastaHumano)},
		{Auto: aeiStr(item.AutoHastaHumano), Riesgo: aeiStr("alto")},
	} {
		_, err := SetCambios(root, "main", "high", "precios", c, "")
		if err == nil || !errors.Is(err, item.ErrAutoSinPresupuesto) || !strings.Contains(err.Error(), aeiSinTope) {
			t.Fatalf("CA-641: auto sin tope se niega con item.ErrAutoSinPresupuesto: %v", err)
		}
		if !ixMismos(foto, ixFotoItems(t, root)) {
			t.Fatal("CA-641: auto sin tope no escribe nada (ni el riesgo que venia junto)")
		}
	}
	if item.ErrAutoSinPresupuesto.Error() != aeiSinTope {
		t.Fatalf("CA-641: el texto es el de item add: %q", item.ErrAutoSinPresupuesto)
	}

	res, err := SetCambios(root, "main", "high", "precios",
		item.Cambios{Auto: aeiStr(item.AutoHastaHumano), PresupuestoUSD: aeiUSD(3)}, "")
	if err != nil || !res.Cambio || res.Auto != item.AutoHastaHumano || aeiVal(res.PresupuestoUSD) != 3.0 ||
		res.AutoAntes != "" || res.PresupuestoAntes != nil {
		t.Fatalf("CA-641: auto con su tope en el mismo set: %+v %v", res, err)
	}
	it := aeiCargar(t, root)
	if it.Auto != item.AutoHastaHumano || aeiVal(it.PresupuestoUSD) != 3.0 {
		t.Fatalf("CA-641: %+v", it)
	}
}

// CA-641: --auto no quita la clave y deja el tope; --presupuesto-usd
// cambia el tope y deja auto; --auto no sin auto no escribe.
func TestCA641_AutoNoQuitaLaClaveYPresupuestoEscribeElTope(t *testing.T) {
	root := aeiRepo(t, aeiConTope+"auto: hasta-humano\n")
	head := crHead(t, root)
	antes := aeiCargar(t, root)

	res, err := SetCambios(root, "main", "high", "precios", item.Cambios{PresupuestoUSD: aeiUSD(7.5)}, "")
	if err != nil || !res.Cambio || aeiVal(res.PresupuestoUSD) != 7.5 || aeiVal(res.PresupuestoAntes) != 5.0 ||
		res.Auto != item.AutoHastaHumano || res.AutoAntes != item.AutoHastaHumano {
		t.Fatalf("CA-641: --presupuesto-usd 7.5: %+v %v", res, err)
	}
	it := aeiCargar(t, root)
	if aeiVal(it.PresupuestoUSD) != 7.5 || it.Auto != item.AutoHastaHumano {
		t.Fatalf("CA-641: el tope cambia y auto queda: %+v", it)
	}
	aeiIgualesSalvo(t, "CA-641", antes, it)

	// --auto no: el item del lado de Cambios es Auto ""
	res, err = SetCambios(root, "main", "high", "precios", item.Cambios{Auto: aeiStr("")}, "")
	if err != nil || !res.Cambio || res.Auto != "" || res.AutoAntes != item.AutoHastaHumano || aeiVal(res.PresupuestoUSD) != 7.5 {
		t.Fatalf("CA-641: --auto no: %+v %v", res, err)
	}
	raw, _ := os.ReadFile(item.Path(root, "precios"))
	if strings.Contains(string(raw), "auto:") || strings.Contains(string(raw), AutoNo+"\n") {
		t.Fatalf("CA-641: --auto no quita la clave (el item nunca guarda %q):\n%s", AutoNo, raw)
	}
	it = aeiCargar(t, root)
	if it.Auto != "" || aeiVal(it.PresupuestoUSD) != 7.5 {
		t.Fatalf("CA-641: sin auto y con su tope: %+v", it)
	}
	aeiSinCommitear(t, "CA-641", root, head)

	// otra vez --auto no: nada que cambiar
	foto := ixFotoItems(t, root)
	res, err = SetCambios(root, "main", "high", "precios", item.Cambios{Auto: aeiStr("")}, "")
	if err != nil || res.Cambio || !ixMismos(foto, ixFotoItems(t, root)) {
		t.Fatalf("CA-641: --auto no sin auto no escribe: %+v %v", res, err)
	}
	// un tope que no es mayor que 0 tampoco pasa por SetCambios
	for _, x := range []float64{0, -2, math.NaN(), math.Inf(1)} {
		if _, err := SetCambios(root, "main", "high", "precios", item.Cambios{PresupuestoUSD: aeiUSD(x)}, ""); err == nil {
			t.Fatalf("CA-641: SetCambios con presupuesto %v se niega", x)
		}
		if !ixMismos(foto, ixFotoItems(t, root)) {
			t.Fatalf("CA-641: SetCambios con presupuesto %v no escribe", x)
		}
	}
}

// CA-641: SetCambios se niega, sin escribir, en el orden del contrato: el
// rol primero (con el texto del piloto cuando toca auto o el tope, y el de
// Set cuando toca solo el riesgo), despues el item que no existe, despues
// Hecho y por ultimo el auto sin tope.
func TestCA641_SetSeNiegaEnElOrdenDelContrato(t *testing.T) {
	root := aeiRepo(t, "")
	icEscribir(t, root, ".hoom/items/contope.yaml", icItemYAML("Con tope", "2026-09-22T15:04:05Z", aeiConTope))
	crHecho(t, root, "hecha")
	foto := ixFotoItems(t, root)
	auto := item.Cambios{Auto: aeiStr(item.AutoHastaHumano)}
	for _, c := range []struct {
		slug, rol string
		c         item.Cambios
		want      string
	}{
		{"contope", "writer", auto, aeiPersona},
		{"contope", "refutador", item.Cambios{Auto: aeiStr("")}, aeiPersona},
		{"contope", "reviewer", item.Cambios{PresupuestoUSD: aeiUSD(9)}, aeiPersona},
		{"contope", "writer", item.Cambios{Riesgo: aeiStr("alto"), Auto: aeiStr(item.AutoHastaHumano)}, aeiPersona},
		{"contope", "writer", item.Cambios{Riesgo: aeiStr("alto"), PresupuestoUSD: aeiUSD(9)}, aeiPersona},
		{"contope", "writer", item.Cambios{Riesgo: aeiStr("alto")}, aeiPersonaRsg},
		{"nadie", "writer", auto, aeiPersona},
		{"precios", "test-writer", auto, aeiPersona}, // el rol antes que el auto sin tope
		{"hecha", "writer", auto, aeiPersona},
		{"nadie", "", auto, `no existe el item "nadie"`},
		{"hecha", "", auto, "la tarjeta ya esta en Hecho"}, // Hecho antes que el auto sin tope
		{"hecha", "", item.Cambios{PresupuestoUSD: aeiUSD(9)}, "la tarjeta ya esta en Hecho"},
		{"precios", "", auto, aeiSinTope},
	} {
		_, err := SetCambios(root, "main", "high", c.slug, c.c, c.rol)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("CA-641: SetCambios(%s, %+v, rol %q) se niega con %q: %v", c.slug, c.c, c.rol, c.want, err)
		}
		if c.want == aeiPersonaRsg && strings.Contains(err.Error(), aeiPersona) {
			t.Fatalf("CA-641: solo el riesgo lleva el texto de Set: %v", err)
		}
		if !ixMismos(foto, ixFotoItems(t, root)) {
			t.Fatalf("CA-641: SetCambios(%s, rol %q) no escribe", c.slug, c.rol)
		}
	}
}

// CA-641: Execute lee $HOOM_ROLE: --auto y --presupuesto-usd se niegan con
// el texto del piloto, tambien junto a --riesgo (ACLARACION 3).
func TestCA641_ExecuteConHoomRoleSeNiega(t *testing.T) {
	root := aeiRepo(t, aeiConTope)
	foto := ixFotoItems(t, root)
	t.Setenv("HOOM_ROLE", "writer")
	for _, args := range [][]string{
		{"set", "precios", "--auto", "hasta-humano"},
		{"set", "precios", "--auto", "no"},
		{"set", "precios", "--presupuesto-usd", "9"},
		{"set", "precios", "--riesgo", "alto", "--auto", "hasta-humano"},
		{"set", "precios", "--riesgo", "alto", "--presupuesto-usd", "9", "--json"},
	} {
		out, _, err := icExec(t, root, args...)
		if err == nil || !strings.Contains(err.Error(), aeiPersona) {
			t.Fatalf("CA-641: hoom item %q con HOOM_ROLE se niega con %q: %v\n%s", args, aeiPersona, err, out)
		}
		if !ixMismos(foto, ixFotoItems(t, root)) {
			t.Fatalf("CA-641: hoom item %q con HOOM_ROLE no escribe", args)
		}
	}
}

// CA-641: el JSON de set lleva auto y presupuesto_usd despues del set solo
// cuando la tarjeta los tiene; --riesgo solo da el JSON de antes.
func TestCA641_ElJSONDeSet(t *testing.T) {
	root := aeiRepo(t, aeiConTope)
	for _, c := range []struct {
		args []string
		want map[string]any
	}{
		{[]string{"set", "precios", "--auto", "hasta-humano", "--json"},
			map[string]any{"slug": "precios", "antes": "normal", "riesgo": "normal", "auto": "hasta-humano", "presupuesto_usd": 5.0}},
		{[]string{"set", "precios", "--presupuesto-usd", "2.5", "--riesgo", "alto", "--json"},
			map[string]any{"slug": "precios", "antes": "normal", "riesgo": "alto", "auto": "hasta-humano", "presupuesto_usd": 2.5}},
		{[]string{"set", "precios", "--auto", "no", "--json"},
			map[string]any{"slug": "precios", "antes": "alto", "riesgo": "alto", "presupuesto_usd": 2.5}},
		{[]string{"set", "precios", "--riesgo", "bajo", "--json"},
			map[string]any{"slug": "precios", "antes": "alto", "riesgo": "bajo"}},
	} {
		out, _, err := icExec(t, root, c.args...)
		var js map[string]any
		if err != nil || json.Unmarshal([]byte(out), &js) != nil || !reflect.DeepEqual(js, c.want) {
			t.Fatalf("CA-641: hoom item %q --json es %v; fue %v (%v)\n%s", c.args, c.want, js, err, out)
		}
	}
	it := aeiCargar(t, root)
	if it.Auto != "" || aeiVal(it.PresupuestoUSD) != 2.5 || it.Riesgo != "bajo" {
		t.Fatalf("CA-641: el item despues de los cuatro sets: %+v", it)
	}
}

// CA-641: desde el worktree de la tarea, set escribe en el item del checkout
// principal cuando lo tiene (y no en el de la tarea), sin commitear en
// ninguno de los dos; y la tarjeta derivada desde el worktree lee auto y
// presupuesto_usd de ese item, aunque el de la tarea diga otra cosa.
func TestCA641_DesdeElWorktreeEscribeYLeeElItemDelPrincipal(t *testing.T) {
	t.Setenv("HOOM_ROLE", "")
	root, wt := crR4Tarea(t, true)
	crR4PATH(t)
	headP, headW := crHead(t, root), crHead(t, wt)
	fotoW := ixFotoItems(t, wt)

	res, err := SetCambios(wt, "main", "high", crR4Slug,
		item.Cambios{Auto: aeiStr(item.AutoHastaHumano), PresupuestoUSD: aeiUSD(4)}, "")
	if err != nil || !res.Cambio {
		t.Fatalf("CA-641: set desde el worktree: %+v %v", res, err)
	}
	if p := aeiCargar(t, root); p.Auto != item.AutoHastaHumano || aeiVal(p.PresupuestoUSD) != 4.0 {
		t.Fatalf("CA-641: escribe el item del checkout principal: %+v", p)
	}
	if !ixMismos(fotoW, ixFotoItems(t, wt)) {
		t.Fatal("CA-641: no toca el item del arbol de la tarea cuando el principal lo tiene")
	}
	aeiSinCommitear(t, "CA-641", root, headP)
	if crHead(t, wt) != headW {
		t.Fatal("CA-641: set no commitea en el worktree")
	}

	for _, dir := range []string{wt, root} {
		c, err := boardcmd.CardFor(dir, "main", "high", crR4Slug, time.Now().UTC())
		if err != nil || c.Item.Auto != item.AutoHastaHumano || aeiVal(c.Item.PresupuestoUSD) != 4.0 {
			t.Fatalf("CA-641: la tarjeta desde %s lee auto y tope del principal: %+v %v", dir, c.Item, err)
		}
	}

	// el item de la tarea dice otra cosa: manda el del principal
	icEscribir(t, wt, crR4ItemRel, aeiItem("presupuesto_usd: 9\nauto: hasta-humano\n"))
	if _, err := SetCambios(wt, "main", "high", crR4Slug, item.Cambios{Auto: aeiStr("")}, ""); err != nil {
		t.Fatalf("CA-641: --auto no desde el worktree: %v", err)
	}
	if p := aeiCargar(t, root); p.Auto != "" || aeiVal(p.PresupuestoUSD) != 4.0 {
		t.Fatalf("CA-641: --auto no quita la clave del principal: %+v", p)
	}
	if w := aeiCargar(t, wt); w.Auto != item.AutoHastaHumano || aeiVal(w.PresupuestoUSD) != 9.0 {
		t.Fatalf("CA-641: el item de la tarea queda como estaba: %+v", w)
	}
	c, err := boardcmd.CardFor(wt, "main", "high", crR4Slug, time.Now().UTC())
	if err != nil || c.Item.Auto != "" || aeiVal(c.Item.PresupuestoUSD) != 4.0 {
		t.Fatalf("CA-641: la tarjeta desde el worktree lee del principal, no de la tarea: %+v %v", c.Item, err)
	}
}

// CA-641: con el item solo en la rama de la tarea, set escribe en el arbol
// de la tarea y la tarjeta lo lee de alli.
func TestCA641_ConElItemSoloEnLaTareaEscribeYLeeAlli(t *testing.T) {
	t.Setenv("HOOM_ROLE", "")
	root, wt := crR4Tarea(t, false)
	crR4PATH(t)
	headW := crHead(t, wt)
	res, err := SetCambios(wt, "main", "high", crR4Slug,
		item.Cambios{PresupuestoUSD: aeiUSD(6), Auto: aeiStr(item.AutoHastaHumano)}, "")
	if err != nil || !res.Cambio {
		t.Fatalf("CA-641: %+v %v", res, err)
	}
	if w := aeiCargar(t, wt); w.Auto != item.AutoHastaHumano || aeiVal(w.PresupuestoUSD) != 6.0 {
		t.Fatalf("CA-641: escribe el item de la tarea: %+v", w)
	}
	if _, err := os.Stat(item.Path(root, crR4Slug)); !os.IsNotExist(err) {
		t.Fatalf("CA-641: no crea el item en el principal: %v", err)
	}
	aeiSinCommitear(t, "CA-641", wt, headW)
	c, err := boardcmd.CardFor(wt, "main", "high", crR4Slug, time.Now().UTC())
	if err != nil || c.Item.Auto != item.AutoHastaHumano || aeiVal(c.Item.PresupuestoUSD) != 6.0 {
		t.Fatalf("CA-641: la tarjeta lee el item de la tarea: %+v %v", c.Item, err)
	}
}

// CA-641 (binario): exit 2 sin nada que cambiar y con un presupuesto que no
// es mayor que 0; exit 1 con auto sin tope y con HOOM_ROLE; exit 0 al
// cambiar, sin commitear; nada se escribe en los que se niegan.
func TestCA641_E2ESetAutoExitCodes(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", aeiItem(""))
	icEscribir(t, root, ".hoom/items/contope.yaml", icItemYAML("Con tope", "2026-09-22T15:04:05Z", aeiConTope))
	crCommitearItems(t, root)
	head := crHead(t, root)
	foto := ixFotoItems(t, root)
	for _, c := range []struct {
		rol  string
		args []string
		exit int
		want string
	}{
		{"", []string{"item", "set", "precios"}, 2, aeiFalta},
		{"", []string{"item", "set", "precios", "--json"}, 2, aeiFalta},
		{"", []string{"item", "set", "precios", "--presupuesto-usd", "0"}, 2, aeiMayorQue0},
		{"", []string{"item", "set", "precios", "--presupuesto-usd=-1"}, 2, aeiMayorQue0},
		{"", []string{"item", "set", "precios", "--presupuesto-usd", "abc"}, 2, aeiMayorQue0},
		{"", []string{"item", "set", "precios", "--auto", "hasta-humano"}, 1, aeiSinTope},
		{"", []string{"item", "set", "precios", "--auto", "hasta-humano", "--riesgo", "alto"}, 1, aeiSinTope},
		{"writer", []string{"item", "set", "contope", "--auto", "hasta-humano"}, 1, aeiPersona},
		{"writer", []string{"item", "set", "contope", "--presupuesto-usd", "3"}, 1, aeiPersona},
		{"reviewer", []string{"item", "set", "contope", "--riesgo", "alto", "--auto", "no"}, 1, aeiPersona},
		{"", []string{"item", "set", "nadie", "--auto", "hasta-humano"}, 1, `no existe el item "nadie"`},
	} {
		r := crCorrer(t, bin, root, c.rol, c.args...)
		if r.exit != c.exit || !strings.Contains(r.stderr, c.want) {
			t.Fatalf("CA-641: hoom %v (HOOM_ROLE=%q): exit %d con %q; fue %d\nstdout: %s\nstderr: %s",
				c.args, c.rol, c.exit, c.want, r.exit, r.stdout, r.stderr)
		}
		if !ixMismos(foto, ixFotoItems(t, root)) {
			t.Fatalf("CA-641: hoom %v no escribe", c.args)
		}
	}

	r := crCorrer(t, bin, root, "", "item", "set", "contope", "--auto", "hasta-humano")
	if r.exit != 0 {
		t.Fatalf("CA-641: --auto en un item con tope: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	raw, _ := os.ReadFile(item.Path(root, "contope"))
	if !strings.Contains(string(raw), "auto: hasta-humano\n") || !strings.Contains(string(raw), aeiConTope) {
		t.Fatalf("CA-641: el archivo lleva auto y su tope:\n%s", raw)
	}
	r = crCorrer(t, bin, root, "", "item", "set", "precios", "--auto", "hasta-humano", "--presupuesto-usd", "2", "--json")
	var js map[string]any
	if r.exit != 0 || json.Unmarshal([]byte(r.stdout), &js) != nil ||
		!reflect.DeepEqual(js, map[string]any{"slug": "precios", "antes": "normal", "riesgo": "normal", "auto": "hasta-humano", "presupuesto_usd": 2.0}) {
		t.Fatalf("CA-641: auto con tope en el mismo set: %d %v\n%s%s", r.exit, js, r.stdout, r.stderr)
	}
	r = crCorrer(t, bin, root, "", "item", "set", "contope", "--auto", "no")
	raw, _ = os.ReadFile(item.Path(root, "contope"))
	if r.exit != 0 || strings.Contains(string(raw), "auto:") || !strings.Contains(string(raw), aeiConTope) {
		t.Fatalf("CA-641: --auto no quita la clave y deja el tope: %d\n%s\n%s", r.exit, raw, r.stderr)
	}
	aeiSinCommitear(t, "CA-641", root, head)
}
