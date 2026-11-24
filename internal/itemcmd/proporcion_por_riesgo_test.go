// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// `hoom item set <slug> --tope-palabras|--tope-criterios N --motivo "..."`
// (CA-796): Parse lleva los flags; SetCambios escribe el tope en el item con
// su motivo, no commitea y se niega, sin tocar nada y en el orden del
// contrato, con un rol, con un valor negativo, sin motivo y con un motivo
// sin tope. Escritos desde el spec, las aclaraciones de la ronda 0 y las
// firmas publicas, sin leer la implementacion.
package itemcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/item"
)

const (
	pprPersonaTope = "el tope del spec de una tarjeta lo cambia una persona, no un rol"
	pprErrPal      = "--tope-palabras tiene que ser un entero mayor o igual que 0"
	pprErrCri      = "--tope-criterios tiene que ser un entero mayor o igual que 0"
)

func pprInt(n int) *int { return &n }
func pprIntVal(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// CA-796: Parse de set lleva --tope-palabras, --tope-criterios y --motivo,
// solos o juntos, con los flags antes o despues del slug; 0 vale.
func TestCA796_ParseSetConTope(t *testing.T) {
	const ca = "CA-796"
	for _, c := range []struct {
		args     []string
		pal, cri *int
		motivo   *string
	}{
		{[]string{"set", "precios", "--tope-palabras", "900", "--motivo", "el intake es largo"}, pprInt(900), nil, aeiStr("el intake es largo")},
		{[]string{"set", "precios", "--tope-criterios=12", "--motivo=x"}, nil, pprInt(12), aeiStr("x")},
		{[]string{"set", "--tope-palabras", "0", "--tope-criterios", "0", "--motivo", "sin tope", "precios"}, pprInt(0), pprInt(0), aeiStr("sin tope")},
		{[]string{"set", "precios", "--tope-palabras", "1500"}, pprInt(1500), nil, nil},
		{[]string{"set", "precios", "--tope-palabras", "1500", "--motivo", ""}, pprInt(1500), nil, aeiStr("")},
	} {
		req, err := Parse(c.args)
		if err != nil {
			t.Fatalf("%s: %q es valido para Parse: %v", ca, c.args, err)
		}
		var motivo any
		if req.Motivo != nil {
			motivo = *req.Motivo
		}
		var want any
		if c.motivo != nil {
			want = *c.motivo
		}
		if req.Sub != SubSet || req.Slug != "precios" || pprIntVal(req.TopePalabras) != pprIntVal(c.pal) ||
			pprIntVal(req.TopeCriterios) != pprIntVal(c.cri) || motivo != want {
			t.Fatalf("%s: %q: palabras %v criterios %v motivo %v; fue %+v", ca, c.args, pprIntVal(c.pal), pprIntVal(c.cri), want, req)
		}
	}
	// junto a --riesgo y --json
	req, err := Parse([]string{"set", "precios", "--riesgo", "alto", "--tope-palabras", "2500", "--motivo", "m", "--json"})
	if err != nil || req.Riesgo != "alto" || pprIntVal(req.TopePalabras) != 2500 || !req.JSON {
		t.Fatalf("%s: --tope-palabras junto a --riesgo y --json: %+v %v", ca, req, err)
	}
}

// CA-796: un valor que no es un entero >= 0 es un error con el texto exacto.
func TestCA796_ParseTopeQueNoEsEntero(t *testing.T) {
	const ca = "CA-796"
	for _, v := range []string{"-1", "1.5", "ocho", "", "1e3", "+-2", "9x"} {
		for flag, want := range map[string]string{"--tope-palabras": pprErrPal, "--tope-criterios": pprErrCri} {
			args := []string{"set", "precios", flag + "=" + v, "--motivo", "x"}
			_, err := Parse(args)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s: %q dice %q: %v", ca, args, want, err)
			}
		}
	}
	if _, err := Parse([]string{"set", "precios", "--tope-palabras"}); err == nil {
		t.Fatalf("%s: --tope-palabras sin valor es un error", ca)
	}
}

// CA-796: SetCambios escribe el tope con su motivo, no commitea, y el
// SetResult lo trae (JSON tope_palabras, tope_criterios, tope_motivo).
func TestCA796_SetCambiosEscribeElTope(t *testing.T) {
	const ca = "CA-796"
	root := aeiRepo(t, "")
	head := crHead(t, root)
	antes := aeiCargar(t, root)
	res, err := SetCambios(root, "main", "high", "precios",
		item.Cambios{TopePalabras: pprInt(1500), TopeCriterios: pprInt(12), TopeMotivo: aeiStr("el intake es largo")}, "")
	if err != nil {
		t.Fatalf("%s: SetCambios con tope: %v", ca, err)
	}
	if !res.Cambio || pprIntVal(res.TopePalabras) != 1500 || pprIntVal(res.TopeCriterios) != 12 || res.TopeMotivo != "el intake es largo" ||
		res.TopePalabrasAntes != nil || res.TopeCriteriosAntes != nil {
		t.Fatalf("%s: el SetResult trae los topes despues y antes: %+v", ca, res)
	}
	despues := aeiCargar(t, root)
	if pprIntVal(despues.TopePalabras) != 1500 || pprIntVal(despues.TopeCriterios) != 12 || despues.TopeMotivo != "el intake es largo" ||
		despues.TopePor != "hoom test <test@hoom.dev>" || despues.TopeEn == nil {
		t.Fatalf("%s: el item lleva el tope, el motivo, quien y cuando: %+v", ca, despues)
	}
	aeiIgualesSalvo(t, ca, antes, despues)
	if despues.Riesgo != antes.Riesgo || despues.RiesgoPor != "" {
		t.Fatalf("%s: el tope no toca el riesgo: %+v", ca, despues)
	}
	aeiSinCommitear(t, ca, root, head)
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`"tope_palabras":1500`, `"tope_criterios":12`, `"tope_motivo":"el intake es largo"`, `"slug":"precios"`} {
		if !strings.Contains(string(raw), w) {
			t.Fatalf("%s: el JSON de set trae %s: %s", ca, w, raw)
		}
	}
	// el segundo set trae los de antes
	res, err = SetCambios(root, "main", "high", "precios", item.Cambios{TopePalabras: pprInt(0), TopeMotivo: aeiStr("sin tope")}, "")
	if err != nil || pprIntVal(res.TopePalabrasAntes) != 1500 || pprIntVal(res.TopePalabras) != 0 || pprIntVal(res.TopeCriterios) != 12 {
		t.Fatalf("%s: el segundo set trae antes 1500 y despues 0: %+v %v", ca, res, err)
	}
	// un set sin tope emite el JSON de antes, sin tope_
	root2 := aeiRepo(t, "")
	res, err = SetCambios(root2, "main", "high", "precios", item.Cambios{Riesgo: aeiStr("alto")}, "")
	raw, _ = json.Marshal(res)
	if err != nil || strings.Contains(string(raw), "tope_") {
		t.Fatalf("%s: un set sin tope no trae tope_ en el JSON: %s %v", ca, raw, err)
	}
}

// CA-796: las negativas, sin escribir nada, en el orden del contrato: el rol
// primero (tambien junto a --riesgo y en una tarjeta que no existe), luego
// las de item.Cambiar antes de buscar el item, luego las de hoy.
func TestCA796_SetCambiosSeNiegaEnOrden(t *testing.T) {
	const ca = "CA-796"
	root := aeiRepo(t, "")
	crHecho(t, root, "hecha")
	foto := ixFotoItems(t, root)
	tope := item.Cambios{TopePalabras: pprInt(1500), TopeMotivo: aeiStr("m")}
	for _, c := range []struct {
		slug, rol string
		c         item.Cambios
		want      string
		is        error
	}{
		{"precios", "writer", tope, pprPersonaTope, nil},
		{"precios", "test-writer", item.Cambios{TopeCriterios: pprInt(20), TopeMotivo: aeiStr("m")}, pprPersonaTope, nil},
		{"precios", "arquitecto", item.Cambios{Riesgo: aeiStr("alto"), TopePalabras: pprInt(1), TopeMotivo: aeiStr("m")}, pprPersonaTope, nil},
		{"precios", "writer", item.Cambios{TopePalabras: pprInt(-1), TopeMotivo: aeiStr("m")}, pprPersonaTope, nil},
		{"precios", "writer", item.Cambios{TopePalabras: pprInt(900)}, pprPersonaTope, nil},
		{"nadie", "writer", tope, pprPersonaTope, nil},
		{"hecha", "writer", tope, pprPersonaTope, nil},
		{"precios", "", item.Cambios{TopePalabras: pprInt(-1), TopeMotivo: aeiStr("m")}, "tope_palabras tiene que ser un entero mayor o igual que 0", nil},
		{"precios", "", item.Cambios{TopePalabras: pprInt(900)}, "", item.ErrTopeSinMotivo},
		{"precios", "", item.Cambios{TopePalabras: pprInt(900), TopeMotivo: aeiStr("   ")}, "", item.ErrTopeSinMotivo},
		{"precios", "", item.Cambios{TopeMotivo: aeiStr("m")}, "", item.ErrMotivoSinTope},
		{"nadie", "", item.Cambios{TopePalabras: pprInt(900)}, "", item.ErrTopeSinMotivo},
		{"nadie", "", item.Cambios{TopeCriterios: pprInt(-2), TopeMotivo: aeiStr("m")}, "tope_criterios tiene que ser un entero mayor o igual que 0", nil},
		{"hecha", "", item.Cambios{TopeMotivo: aeiStr("m")}, "", item.ErrMotivoSinTope},
		{"nadie", "", tope, `no existe el item "nadie"`, nil},
		{"hecha", "", tope, "la tarjeta ya esta en Hecho", nil},
	} {
		_, err := SetCambios(root, "main", "high", c.slug, c.c, c.rol)
		ok := err != nil
		if c.is != nil {
			ok = ok && errors.Is(err, c.is)
		} else {
			ok = ok && strings.Contains(err.Error(), c.want)
		}
		if !ok {
			t.Fatalf("%s: SetCambios(%s, %+v, rol %q) se niega con %q %v: %v", ca, c.slug, c.c, c.rol, c.want, c.is, err)
		}
		if !ixMismos(foto, ixFotoItems(t, root)) {
			t.Fatalf("%s: SetCambios(%s, rol %q) no escribe", ca, c.slug, c.rol)
		}
	}
}

// CA-796: Execute lee $HOOM_ROLE y se niega con el texto del tope; sin rol,
// imprime una linea con «de <slug>» y «hoom item save <slug>» (aclaracion 6).
func TestCA796_ExecuteConYSinHoomRole(t *testing.T) {
	const ca = "CA-796"
	root := aeiRepo(t, "")
	foto := ixFotoItems(t, root)
	req, err := Parse([]string{"set", "precios", "--tope-palabras", "1500", "--motivo", "largo"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOOM_ROLE", "writer")
	var out, errb bytes.Buffer
	err = Execute(root, "main", "high", req, &out, &errb)
	if err == nil || !strings.Contains(err.Error()+errb.String(), pprPersonaTope) {
		t.Fatalf("%s: con HOOM_ROLE se niega con %q: %v %s", ca, pprPersonaTope, err, errb.String())
	}
	if !ixMismos(foto, ixFotoItems(t, root)) {
		t.Fatalf("%s: con HOOM_ROLE no escribe", ca)
	}
	t.Setenv("HOOM_ROLE", "")
	out.Reset()
	errb.Reset()
	if err := Execute(root, "main", "high", req, &out, &errb); err != nil {
		t.Fatalf("%s: sin HOOM_ROLE escribe: %v %s", ca, err, errb.String())
	}
	if !strings.Contains(out.String(), "de precios") || !strings.Contains(out.String(), "hoom item save precios") {
		t.Fatalf("%s: la salida nombra la tarjeta y como guardarla: %q", ca, out.String())
	}
	if it := aeiCargar(t, root); pprIntVal(it.TopePalabras) != 1500 || it.TopeMotivo != "largo" {
		t.Fatalf("%s: Execute escribe el tope: %+v", ca, it)
	}
}
