// Tests de regresion de la review de 4 lentes de la cabina C3 sobre
// .hoom/specs/acciones-desde-la-tarjeta.md, seccion "El minimo del provider"
// (CA-328):
//
//   - 20261014T180437_78a674 (providers): "Un provider sin la capacidad
//     budget declara 0: no acepta tope, asi que el minimo no aplica". Un
//     provider con Capabilities.Budget false que igual implementa
//     BudgetFloor (por error o por compatibilidad) publica min_budget_usd 0,
//     en el Info de Detect y en MinBudgetUSD. Hoy publica su piso. Guarda: uno
//     CON la capacidad budget publica el piso que declara.
//
// Con un Registry propio y binarios que no existen: ningun CLI real.
package providers

import (
	"encoding/json"
	"math"
	"testing"
	"testing/quick"
)

// h3Falso es un provider de prueba con o sin la capacidad budget que SIEMPRE
// implementa BudgetFloor con su piso.
type h3Falso struct {
	name string
	tope bool
	piso float64
}

func (p h3Falso) Name() string { return p.name }
func (p h3Falso) Bin() string  { return "hoom-h3-no-existe-" + p.name }
func (p h3Falso) Capabilities() Capabilities {
	return Capabilities{SystemPrompt: true, Resume: true, Budget: p.tope}
}
func (p h3Falso) Command(req Request) (Invocation, error) {
	return Invocation{Bin: p.Bin(), Args: []string{req.Prompt}}, nil
}
func (p h3Falso) Normalize(string) []Event { return nil }
func (p h3Falso) MinBudgetUSD() float64    { return p.piso }

// h3Publicado registra los providers en un Registry propio y devuelve, por
// nombre, el min_budget_usd que publica su Info (Detect, lo que emite el
// JSON) y el de MinBudgetUSD.
func h3Publicado(t *testing.T, ps ...Provider) (info, fn map[string]float64) {
	t.Helper()
	r := NewRegistry()
	for _, p := range ps {
		if _, ok := p.(BudgetFloor); !ok {
			t.Fatalf("CA-328: fixture: %s implementa BudgetFloor", p.Name())
		}
		if err := r.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	info, fn = map[string]float64{}, map[string]float64{}
	infos := r.Detect()
	if len(infos) != len(ps) {
		t.Fatalf("CA-328: Detect trae los %d providers del registry propio: %+v", len(ps), infos)
	}
	for _, in := range infos {
		info[in.Name] = in.MinBudgetUSD
	}
	for _, p := range r.All() {
		fn[p.Name()] = MinBudgetUSD(p)
	}
	return info, fn
}

// h3JSON es el min_budget_usd que trae el JSON del Info de p (lo que emiten
// hoom providers --json y /api/providers), con p solo en un Registry propio.
func h3JSON(t *testing.T, p Provider) (float64, string) {
	t.Helper()
	r := NewRegistry()
	if err := r.Register(p); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(r.Detect())
	if err != nil {
		t.Fatal(err)
	}
	var lista []map[string]any
	if err := json.Unmarshal(raw, &lista); err != nil || len(lista) != 1 {
		t.Fatalf("CA-328: el Info se emite como lista JSON: %v\n%s", err, raw)
	}
	v, ok := lista[0]["min_budget_usd"].(float64)
	if !ok {
		t.Fatalf("CA-328: el JSON trae min_budget_usd como numero (0 incluido, sin omitir):\n%s", raw)
	}
	return v, string(raw)
}

// CA-328 (hallazgo 78a674): un provider sin la capacidad budget que declara
// un piso de 0.75 publica min_budget_usd 0 (el minimo no aplica): en el Info
// de Detect, en MinBudgetUSD y en el JSON.
func TestHallazgo_78a674_SinCapacidadBudgetElMinimoEsCero(t *testing.T) {
	sinTope := h3Falso{name: "sin-tope", tope: false, piso: 0.75}
	info, fn := h3Publicado(t, sinTope)
	if info["sin-tope"] != 0 {
		t.Errorf("CA-328 (hallazgo 78a674): un provider sin la capacidad budget publica min_budget_usd 0 aunque implemente BudgetFloor (0.75): Info dice %v", info["sin-tope"])
	}
	if fn["sin-tope"] != 0 {
		t.Errorf("CA-328 (hallazgo 78a674): MinBudgetUSD de un provider sin la capacidad budget es 0 aunque implemente BudgetFloor (0.75): fue %v", fn["sin-tope"])
	}
	if v, raw := h3JSON(t, sinTope); v != 0 {
		t.Errorf("CA-328 (hallazgo 78a674): el JSON de un provider sin la capacidad budget trae \"min_budget_usd\": 0:\n%s", raw)
	}
}

// CA-328 (hallazgo 78a674), guarda: un provider CON la capacidad budget y
// piso 0.5 publica 0.5 en el Info, en MinBudgetUSD y en el JSON.
func TestHallazgo_78a674_GuardaConCapacidadBudgetPublicaSuPiso(t *testing.T) {
	conTope := h3Falso{name: "con-tope", tope: true, piso: 0.5}
	info, fn := h3Publicado(t, conTope)
	if !acCasi(info["con-tope"], 0.5) || !acCasi(fn["con-tope"], 0.5) {
		t.Fatalf("CA-328: un provider con la capacidad budget publica su piso 0.5: Info %v, MinBudgetUSD %v", info["con-tope"], fn["con-tope"])
	}
	if v, raw := h3JSON(t, conTope); !acCasi(v, 0.5) {
		t.Fatalf("CA-328: el JSON de un provider con la capacidad budget trae \"min_budget_usd\": 0.5:\n%s", raw)
	}
}

// CA-328 (hallazgo 78a674), propiedad: para cualquier piso p >= 0, el
// provider sin la capacidad budget publica 0 y el que la tiene publica p.
func TestHallazgo_78a674_PropiedadElPisoSoloAplicaConBudget(t *testing.T) {
	prop := func(crudo uint32) bool {
		piso := float64(crudo%100000) / 100 // de 0 a 999.99 USD
		info, fn := h3Publicado(t,
			h3Falso{name: "sin-tope", tope: false, piso: piso},
			h3Falso{name: "con-tope", tope: true, piso: piso})
		ok := info["sin-tope"] == 0 && fn["sin-tope"] == 0 &&
			math.Abs(info["con-tope"]-piso) < 1e-9 && math.Abs(fn["con-tope"]-piso) < 1e-9
		if !ok {
			t.Logf("CA-328 (hallazgo 78a674): piso %v: sin-tope publica %v/%v (quiero 0), con-tope %v/%v (quiero %v)",
				piso, info["sin-tope"], fn["sin-tope"], info["con-tope"], fn["con-tope"], piso)
		}
		return ok
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("CA-328 (hallazgo 78a674): el piso de un provider sin la capacidad budget no aplica: %v", err)
	}
}
