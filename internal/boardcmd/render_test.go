// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-288): el texto de `hoom board`, su JSON y el parseo estricto del verbo.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/item"
)

func bdTablero(cards ...Card) Board {
	b := Board{Warnings: []string{}}
	for _, col := range Columns {
		bc := BoardColumn{ID: col.ID, Name: col.Name, Human: col.Human, Cards: []Card{}}
		for _, c := range cards {
			if c.Column == col.ID {
				bc.Cards = append(bc.Cards, c)
			}
		}
		b.Columns = append(b.Columns, bc)
	}
	return b
}

// CA-288: ParseArgs es estricto: sin operandos, flags conocidos, -h ayuda.
func TestCA288_ParseArgs(t *testing.T) {
	opt, err := ParseArgs(nil)
	if err != nil || opt.JSON {
		t.Fatalf("CA-288: 'hoom board' sin argumentos es valido: %+v %v", opt, err)
	}
	opt, err = ParseArgs([]string{"--json"})
	if err != nil || !opt.JSON {
		t.Fatalf("CA-288: --json: %+v %v", opt, err)
	}
	for _, args := range [][]string{{"x"}, {"--bogus"}, {"--json", "x"}, {"--", "x"}, {"list"}} {
		_, err := ParseArgs(args)
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) || ue.ExitCode() != 2 {
			t.Fatalf("CA-288: 'hoom board %s' es *cliargs.UsageError (exit 2), fue %T %v", strings.Join(args, " "), err, err)
		}
		for _, l := range strings.Split(UsageText, "\n") {
			if l = strings.TrimRight(l, " "); strings.TrimSpace(l) != "" && !strings.Contains(ue.Error(), l) {
				t.Fatalf("CA-288: el rechazo lleva el bloque de uso; falta %q", l)
			}
		}
	}
	for _, args := range [][]string{{"-h"}, {"--help"}} {
		if _, err := ParseArgs(args); !errors.Is(err, cliargs.ErrHelp) {
			t.Fatalf("CA-288: %v es ErrHelp, fue %v", args, err)
		}
	}
}

// CA-288: el texto: la cabecera con el conteo, las 8 columnas en orden con
// su conteo (tambien 0), las humanas con 'esperando humano', y por tarjeta
// el motivo, 'siguiente:' y una linea por subestado presente.
func TestCA288_RenderTexto(t *testing.T) {
	c1 := Derive(func() Evidence { ev := bdArbol(bdEv()); ev.SpecExists = false; return ev }())
	c1.Slug, c1.Item.Slug, c1.Item.Titulo = "uno", "uno", "Titulo uno"
	c2 := Derive(bdEv())
	c2.Running = &Running{EnvelopeID: "e1", Role: "writer", Provider: "claude", Stage: "run", Step: 2, Steps: 5, StartedAt: bdT0}
	c3 := Derive(func() Evidence { ev := bdEv(); ev.Approval = "no-aprobado"; return ev }())
	c3.Slug, c3.Item.Slug, c3.Item.Titulo = "tres", "tres", "Titulo tres"
	c3.Interrupted = &Interrupted{EnvelopeID: "e3", Stage: "run", Step: 3, Steps: 5, UpdatedAt: bdT0}
	c3.Red = &Red{Source: RedVerdict, ID: "v-rojo", Reason: "veredicto rojo: fallo el gate test"}
	c3.Unsynced = []string{".hoom/items/tres.yaml", ".hoom/specs/tres.md"}
	c3.Spend = Spend{CostUSD: bdF(1.2), Runs: 3, RunsWithoutCost: 1, InputTokens: 51000, OutputTokens: 3000, BudgetUSD: bdF(5)}
	var buf bytes.Buffer
	Render(&buf, bdTablero(c1, c2, c3))
	out := buf.String()
	lineas := strings.Split(out, "\n")
	if lineas[0] != "hoom board: 3 tarjetas (columna derivada de la evidencia; nada guardado)" {
		t.Fatalf("CA-288: la cabecera del tablero, fue %q", lineas[0])
	}
	pos := -1
	for _, col := range Columns {
		n := 0
		switch col.ID {
		case ColBacklog, ColTuAceptacion, ColTuAprobacion:
			n = 1
		}
		cab := col.Name + " (" + strconv.Itoa(n) + ")"
		if col.Human {
			cab += " - esperando humano"
		}
		i := strings.Index(out, cab)
		if i < 0 {
			t.Fatalf("CA-288: falta la columna %q en el texto:\n%s", cab, out)
		}
		if i < pos {
			t.Fatalf("CA-288: las columnas van en el orden 1..8 (%s fuera de orden):\n%s", col.Name, out)
		}
		pos = i
	}
	for _, want := range []string{
		"uno", "Titulo uno", "no hay spec: .hoom/specs/precios.md", "siguiente: hoom task start precios",
		"siguiente: hoom task done precios", "siguiente: hoom spec approve .hoom/specs/precios.md",
		"en curso", "interrumpido en el paso 3 de 5", "rojo: veredicto rojo: fallo el gate test",
		"sin sincronizar: 2 archivos", "gasto:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("CA-288: al texto le falta %q:\n%s", want, out)
		}
	}

	buf.Reset()
	Render(&buf, bdTablero())
	if got := strings.TrimSpace(buf.String()); got != `hoom board: sin items (crea uno con 'hoom item add "<titulo>"')` {
		t.Fatalf("CA-288: sin items el tablero lo dice en una linea, fue:\n%s", got)
	}
}

// CA-288: JSONBytes emite {"columns": [...], "warnings": [...]} con las 8
// columnas y ninguna lista null.
func TestCA288_JSONBytes(t *testing.T) {
	c := Derive(bdEv())
	b := bdTablero(c)
	b.Warnings = []string{"item invalido .hoom/items/malo.yaml: clave desconocida \"columna\""}
	raw, err := JSONBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	var js struct {
		Columns []struct {
			ID    string           `json:"id"`
			Name  string           `json:"name"`
			Human bool             `json:"human"`
			Cards []map[string]any `json:"cards"`
		} `json:"columns"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(raw, &js); err != nil {
		t.Fatalf("CA-288: JSONBytes es JSON: %v\n%s", err, raw)
	}
	if len(js.Columns) != 8 || len(js.Warnings) != 1 {
		t.Fatalf("CA-288: 8 columnas y los avisos: %s", raw)
	}
	for i, col := range js.Columns {
		if col.ID != Columns[i].ID || col.Name != Columns[i].Name || col.Human != Columns[i].Human {
			t.Fatalf("CA-288: columna %d: %+v", i, col)
		}
	}
	if len(js.Columns[6].Cards) != 1 || js.Columns[6].Cards[0]["slug"] != bdSlug {
		t.Fatalf("CA-288: la tarjeta en tu-aceptacion: %+v", js.Columns[6])
	}
	var crudo map[string][]map[string]any
	_ = json.Unmarshal(raw, &crudo)
	for _, col := range crudo["columns"] {
		if _, ok := col["cards"].([]any); !ok {
			t.Fatalf("CA-288: cards de %v es una lista (no null): %s", col["id"], raw)
		}
	}
	// la tarjeta trae el item con sus claves (y pedido aunque este vacio)
	c2 := Derive(func() Evidence {
		ev := bdEv()
		ev.Item = item.Item{Slug: bdSlug, Titulo: "x", Tipo: "feature", Prioridad: "media", CreadoPor: "y", CreadoEn: bdT0}
		return ev
	}())
	raw, _ = json.Marshal(c2)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	it, _ := m["item"].(map[string]any)
	if _, ok := it["pedido"]; !ok || it["slug"] != bdSlug {
		t.Fatalf("CA-288: el item de la tarjeta trae slug y pedido: %v", it)
	}
	for _, k := range []string{"presupuesto_usd", "hecho_en", "commit_final"} {
		if _, ok := it[k]; ok {
			t.Fatalf("CA-288: el item sin %s no lo emite: %v", k, it)
		}
	}
}
