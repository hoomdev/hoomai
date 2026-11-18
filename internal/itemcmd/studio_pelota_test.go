// Tests adversariales del spec .hoom/specs/studio-tu-siguiente-paso.md sobre
// `hoom item show <slug> --json` (CA-722): la "card" trae "pelota" siempre,
// nunca null, con grupo, paso (accion, etiqueta, texto), frase, ahora (frase,
// gris), sigue (frase, gris) y camino (8 entradas con nombre, estado y
// texto), y es la MISMA pelota que la tarjeta del tablero. Escritos desde el
// spec y las firmas publicas, sin leer la implementacion.
package itemcmd

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// spTipoJSON es el tipo JSON de un valor crudo.
func spTipoJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "invalido"
	}
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "?"
}

func spObjetoJSON(t *testing.T, ca, donde string, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	if tp := spTipoJSON(raw); tp != "object" {
		t.Fatalf("%s: %s es un objeto, fue %s: %s", ca, donde, tp, raw)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: %s: %v", ca, donde, err)
	}
	return m
}

func spClavesDe(m map[string]json.RawMessage) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// spCamposJSON exige cada campo con su tipo.
func spCamposJSON(t *testing.T, ca, donde string, m map[string]json.RawMessage, campos map[string]string) {
	t.Helper()
	for k, tp := range campos {
		v, ok := m[k]
		if !ok {
			t.Fatalf("%s: %s trae %q; no lo trae. Claves: %v", ca, donde, k, spClavesDe(m))
		}
		if got := spTipoJSON(v); got != tp {
			t.Fatalf("%s: %s.%s es %s (nunca null), fue %s: %s", ca, donde, k, tp, got, v)
		}
	}
}

// spPelotaDeCard lee y valida la forma de card.pelota en el JSON crudo de la
// tarjeta; devuelve la pelota cruda.
func spPelotaDeCard(t *testing.T, ca, donde string, card json.RawMessage) json.RawMessage {
	t.Helper()
	c := spObjetoJSON(t, ca, donde, card)
	p, ok := c["pelota"]
	if !ok {
		t.Fatalf("%s: %s trae \"pelota\" (siempre presente); no la trae. Claves: %v", ca, donde, spClavesDe(c))
	}
	if spTipoJSON(p) != "object" {
		t.Fatalf("%s: %s.pelota es un objeto, nunca null: %s", ca, donde, p)
	}
	obj := spObjetoJSON(t, ca, donde+".pelota", p)
	spCamposJSON(t, ca, donde+".pelota", obj, map[string]string{"grupo": "string", "paso": "object",
		"frase": "string", "ahora": "object", "sigue": "object", "camino": "array"})
	spCamposJSON(t, ca, donde+".pelota.paso", spObjetoJSON(t, ca, donde+".pelota.paso", obj["paso"]),
		map[string]string{"accion": "string", "etiqueta": "string", "texto": "string"})
	for _, k := range []string{"ahora", "sigue"} {
		spCamposJSON(t, ca, donde+".pelota."+k, spObjetoJSON(t, ca, donde+".pelota."+k, obj[k]),
			map[string]string{"frase": "string", "gris": "string"})
	}
	var camino []json.RawMessage
	if err := json.Unmarshal(obj["camino"], &camino); err != nil {
		t.Fatalf("%s: %s.pelota.camino: %v", ca, donde, err)
	}
	if len(camino) != 8 {
		t.Fatalf("%s: %s.pelota.camino tiene 8 entradas, tuvo %d: %s", ca, donde, len(camino), obj["camino"])
	}
	for _, e := range camino {
		spCamposJSON(t, ca, donde+".pelota.camino", spObjetoJSON(t, ca, donde+".pelota.camino", e),
			map[string]string{"nombre": "string", "estado": "string", "texto": "string"})
	}
	return p
}

// spPelota es la pelota decodificada (solo lo que este archivo compara).
type spPelota struct {
	Grupo  string `json:"grupo"`
	Frase  string `json:"frase"`
	Camino []struct {
		Nombre string `json:"nombre"`
		Estado string `json:"estado"`
	} `json:"camino"`
}

// spSpecTexto: spec con las 7 secciones (sin "Riesgos" si se pide).
func spSpecTexto(conRiesgos bool) string {
	s := "# Spec: precios\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n- CA-1: algo.\n\n## Decisiones\nx\n"
	if conRiesgos {
		s += "\n## Riesgos\nx\n"
	}
	return s
}

// spProyecto: un item por grupo facil de armar en disco, mas un item roto.
// Devuelve slug -> (columna esperada por el fixture, grupo del contrato).
func spProyecto(t *testing.T) (string, map[string][2]string) {
	t.Helper()
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/pedido.yaml", icItemYAML("Pedido nuevo", "2026-09-22T15:04:05Z", ""))
	icEscribir(t, root, ".hoom/items/arq.yaml", icItemYAML("Arquitectura", "2026-09-22T15:05:05Z", ""))
	icEscribir(t, root, ".hoom/specs/arq.md", spSpecTexto(false))
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:06:05Z", ""))
	icEscribir(t, root, ".hoom/specs/precios.md", spSpecTexto(true))
	icEscribir(t, root, ".hoom/items/lista.yaml", icItemYAML("Lista", "2026-09-22T15:07:05Z", "hecho_en: 2026-09-23T10:00:00Z\n"))
	icEscribir(t, root, ".hoom/items/roto.yaml", icItemYAML("Roto", "2026-09-22T15:08:05Z", "columna: hecho\n"))
	return root, map[string][2]string{
		"pedido":  {boardcmd.ColBacklog, "EN COLA"},
		"arq":     {boardcmd.ColArquitecto, "TE TOCA"},
		"precios": {boardcmd.ColTuAprobacion, "TE TOCA"},
		"lista":   {boardcmd.ColHecho, "HECHO"},
	}
}

// spTarjetas arma el tablero y exige la columna de cada caso (lo que el
// fixture quiere probar) antes de mirar ninguna pelota; devuelve las
// tarjetas por slug. El item roto no es tarjeta.
func spTarjetas(t *testing.T, root string, casos map[string][2]string) map[string]boardcmd.Card {
	t.Helper()
	b, err := boardcmd.Build(root, "main", "high", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]boardcmd.Card{}
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			out[c.Slug] = c
		}
	}
	if _, ok := out["roto"]; ok || len(b.Warnings) == 0 {
		t.Fatalf("fixture: el item roto va a warnings y no es tarjeta: %v", b.Warnings)
	}
	for slug, cg := range casos {
		c, ok := out[slug]
		if !ok {
			t.Fatalf("fixture: el tablero no tiene la tarjeta %s", slug)
		}
		if c.Column != cg[0] || c.Running != nil || c.Interrupted != nil || c.Red != nil {
			t.Fatalf("fixture: %s esta en %s (se esperaba %s), sin trabajo vivo, cortado ni rojo: running=%v interrupted=%v red=%+v",
				slug, c.Column, cg[0], c.Running != nil, c.Interrupted != nil, c.Red)
		}
	}
	return out
}

var spNombresCamino = []string{"Pedido", "Spec", "Tu firma", "Tests", "Código", "Review", "Tu aceptación", "Hecho"}

// CA-722: `hoom item show <slug> --json` (Execute) trae card.pelota con todos
// sus campos, nunca null, igual a la pelota de esa tarjeta en el tablero; el
// grupo es el del contrato y el camino tiene los 8 nombres en orden.
func TestCA722_ItemShowJSONTraePelota(t *testing.T) {
	root, casos := spProyecto(t)
	tarjetas := spTarjetas(t, root, casos)
	for slug, cg := range casos {
		out, _, err := icExec(t, root, "show", slug, "--json")
		if err != nil {
			t.Fatalf("CA-722: show %s --json: %v", slug, err)
		}
		var js map[string]json.RawMessage
		if err := json.Unmarshal([]byte(out), &js); err != nil {
			t.Fatalf("CA-722: show %s --json es JSON: %v\n%s", slug, err, out)
		}
		card, ok := js["card"]
		if !ok {
			t.Fatalf("CA-722: show %s --json trae card: %s", slug, out)
		}
		p := spPelotaDeCard(t, "CA-722", "hoom item show "+slug+" --json: card", card)
		var got spPelota
		if err := json.Unmarshal(p, &got); err != nil {
			t.Fatal(err)
		}
		if got.Grupo != cg[1] {
			t.Fatalf("CA-722: [%s] pelota.grupo es %q, fue %q", slug, cg[1], got.Grupo)
		}
		for i, e := range got.Camino {
			if e.Nombre != spNombresCamino[i] {
				t.Fatalf("CA-722: [%s] camino[%d].nombre es %q, fue %q", slug, i, spNombresCamino[i], e.Nombre)
			}
		}
		raw, err := json.Marshal(tarjetas[slug])
		if err != nil {
			t.Fatal(err)
		}
		var a, b any
		_ = json.Unmarshal(p, &a)
		_ = json.Unmarshal(spPelotaDeCard(t, "CA-722", "la tarjeta "+slug+" del tablero", raw), &b)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("CA-722: [%s] show y board dan la misma pelota:\nshow:  %s\nboard: %v", slug, p, b)
		}
	}
	// CA-725 en el show: la frase de Tu aprobacion con el titulo
	out, _, err := icExec(t, root, "show", "precios", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var js map[string]json.RawMessage
	_ = json.Unmarshal([]byte(out), &js)
	var got spPelota
	_ = json.Unmarshal(spPelotaDeCard(t, "CA-722", "show precios", js["card"]), &got)
	if got.Frase != "Firmá el spec de «Precios»." {
		t.Fatalf("CA-722, CA-725: la frase de Tu aprobacion en el show: %q", got.Frase)
	}
}

// CA-722: el binario real: `hoom item show <slug> --json` trae card.pelota.
func TestCA722_E2EItemShowJSONTraePelota(t *testing.T) {
	bin := ixBinario(t)
	root, casos := spProyecto(t)
	spTarjetas(t, root, casos)
	for slug, cg := range casos {
		r := ixCorrer(t, bin, root, "item", "show", slug, "--json")
		if r.exit != 0 {
			t.Fatalf("CA-722: hoom item show %s --json sale 0: %d\n%s%s", slug, r.exit, r.stdout, r.stderr)
		}
		var js map[string]json.RawMessage
		if err := json.Unmarshal([]byte(r.stdout), &js); err != nil {
			t.Fatalf("CA-722: hoom item show %s --json es JSON: %v\n%s", slug, err, r.stdout)
		}
		p := spPelotaDeCard(t, "CA-722", "hoom item show "+slug+" --json (binario): card", js["card"])
		var got spPelota
		_ = json.Unmarshal(p, &got)
		if got.Grupo != cg[1] {
			t.Fatalf("CA-722: [%s] el binario da pelota.grupo %q, fue %q", slug, cg[1], got.Grupo)
		}
	}
}
