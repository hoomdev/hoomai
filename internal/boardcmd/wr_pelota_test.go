package boardcmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// Tests propios del writer (studio-tu-siguiente-paso): lo que el diseño de
// pelota.go agrega donde el spec deja lugar.

// OrdenPelota ordena TE TOCA por el lugar en que cada columna llega en cards
// (la ultima del tablero primero), no por la tabla Columns: un tablero que
// trae sus columnas en otro orden se lista por el suyo, como hoom tablero.
func TestWR_OrdenPelotaSigueElOrdenDelTablero(t *testing.T) {
	cards := []*Card{
		{Slug: "w", Column: ColWriter},
		{Slug: "b", Column: ColBacklog},
		{Slug: "a1", Column: ColArquitecto},
		{Slug: "a2", Column: ColArquitecto},
		{Slug: "h", Column: ColHecho},
	}
	grupo := func(c *Card) string {
		switch c.Column {
		case ColBacklog:
			return GrupoEnCola
		case ColHecho:
			return GrupoHecho
		}
		return GrupoTeToca
	}
	var got []string
	for _, c := range OrdenPelota(cards, grupo) {
		got = append(got, c.Slug)
	}
	if strings.Join(got, ",") != "a1,a2,w,b,h" {
		t.Fatalf("TE TOCA va desde la columna que llega ultima (Arquitecto) a la primera (Writer), en su orden; despues EN COLA y HECHO: %v", got)
	}
	if n := len(OrdenPelota(nil, grupo)); n != 0 {
		t.Fatalf("sin tarjetas no hay orden: %d", n)
	}
}

// Una pelota o un tablero que Derive y Build no hicieron siguen sin null:
// camino y orden salen [].
func TestWR_PelotaSinNull(t *testing.T) {
	raw, err := json.Marshal(struct {
		P Pelota      `json:"p"`
		B PelotaBoard `json:"b"`
	}{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "null") || !strings.Contains(s, `"camino":[]`) || !strings.Contains(s, `"orden":[]`) {
		t.Fatalf("la pelota vacia no tiene null: %s", s)
	}
	for _, k := range []string{`"te_toca":0`, `"corriendo":0`, `"detenida":0`, `"en_cola":0`, `"hecho":0`} {
		if !strings.Contains(s, k) {
			t.Fatalf("la cuenta trae sus cinco claves: %s", s)
		}
	}
}

// La piel escribe solo los textos fijos: el titulo de una frase va como
// viene, entre las comillas de la piel.
func TestWR_PielNoTocaElTitulo(t *testing.T) {
	c := &Card{Slug: "precios", Column: ColTuAprobacion}
	c.Item.Titulo = "Precios por región"
	p := Piel{Fijo: strings.ToUpper, Citar: func(t string) string { return `"` + t + `"` }}
	if got := FraseDe(c, GrupoTeToca, ActAprobar, p); got != `FIRMÁ EL SPEC DE "Precios por región".` {
		t.Fatalf("la piel escribe el texto fijo y deja el titulo: %q", got)
	}
	if got := FraseDe(c, GrupoTeToca, ActAprobar, Piel{}); got != "Firmá el spec de «Precios por región»." {
		t.Fatalf("la piel del JSON deja el texto como es: %q", got)
	}
}
