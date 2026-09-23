// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-328): el minimo de presupuesto de cada provider. Info trae
// min_budget_usd, que declara el adapter con la interfaz opcional
// BudgetFloor: 0.5 USD para claude y 0 para codex, gemini y opencode (no
// aceptan tope). Un provider que no implementa BudgetFloor declara 0, y
// JSONBytes (hoom providers --json, /api/providers) lo emite. Sin ningun CLI
// real: PATH apunta a binarios falsos.
package providers

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// acBins arma un directorio con binarios falsos de esos nombres.
func acBins(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func acCasi(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// acSinPiso: un provider con tope en USD que NO implementa BudgetFloor.
type acSinPiso struct{ name string }

func (p acSinPiso) Name() string { return p.name }
func (p acSinPiso) Bin() string  { return "hoom-ac-no-existe-" + p.name }
func (p acSinPiso) Capabilities() Capabilities {
	return Capabilities{SystemPrompt: true, Budget: true, Resume: true}
}
func (p acSinPiso) Command(req Request) (Invocation, error) {
	return Invocation{Bin: p.Bin(), Args: []string{req.Prompt}}, nil
}
func (p acSinPiso) Normalize(line string) []Event { return textEvents(line) }

// acConPiso: el mismo provider, declarando su minimo.
type acConPiso struct {
	acSinPiso
	piso float64
}

func (p acConPiso) MinBudgetUSD() float64 { return p.piso }

// acMinimos es lo que declara cada built-in.
var acMinimos = map[string]float64{"claude": 0.5, "codex": 0, "gemini": 0, "opencode": 0}

// CA-328: Detect trae min_budget_usd de cada provider: 0.5 para claude y 0
// para los que no aceptan tope. Lo declara el adapter: no depende de que el
// binario este en el PATH.
func TestCA328_DetectTraeElMinimoDeCadaProvider(t *testing.T) {
	for nombre, path := range map[string]string{
		"los cuatro instalados": acBins(t, "claude", "codex", "gemini", "opencode"),
		"ninguno instalado":     acBins(t),
	} {
		t.Setenv("PATH", path)
		infos := Detect()
		if len(infos) != len(acMinimos) {
			t.Fatalf("CA-328: (%s) Detect trae los cuatro providers: %+v", nombre, infos)
		}
		for _, in := range infos {
			want, ok := acMinimos[in.Name]
			if !ok {
				t.Fatalf("CA-328: (%s) provider inesperado %q", nombre, in.Name)
			}
			if !acCasi(in.MinBudgetUSD, want) {
				t.Fatalf("CA-328: (%s) min_budget_usd de %s es %v, fue %v", nombre, in.Name, want, in.MinBudgetUSD)
			}
		}
	}
}

// CA-328: providers.MinBudgetUSD(p) es el minimo que declara el adapter; un
// provider sin la capacidad budget declara 0 (el minimo no aplica).
func TestCA328_MinBudgetUSDDelAdapter(t *testing.T) {
	for _, p := range All() {
		want := acMinimos[p.Name()]
		if got := MinBudgetUSD(p); !acCasi(got, want) {
			t.Fatalf("CA-328: MinBudgetUSD(%s) es %v, fue %v", p.Name(), want, got)
		}
		if f, ok := p.(BudgetFloor); ok && !acCasi(f.MinBudgetUSD(), want) {
			t.Fatalf("CA-328: el BudgetFloor de %s declara %v, fue %v", p.Name(), want, f.MinBudgetUSD())
		}
		if !p.Capabilities().Budget && MinBudgetUSD(p) != 0 {
			t.Fatalf("CA-328: %s no acepta tope en USD, asi que su minimo es 0: %v", p.Name(), MinBudgetUSD(p))
		}
	}
	claude, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	if !claude.Capabilities().Budget || !acCasi(MinBudgetUSD(claude), 0.5) {
		t.Fatalf("CA-328: claude acepta tope y declara 0.5 USD de minimo: budget=%v min=%v",
			claude.Capabilities().Budget, MinBudgetUSD(claude))
	}
}

// CA-328: con un Registry propio, un provider que no implementa BudgetFloor
// declara 0 aunque acepte tope, y uno que lo implementa declara su valor:
// Detect lo lee de la interfaz, no de una tabla de nombres.
func TestCA328_RegistryPropioConYSinBudgetFloor(t *testing.T) {
	r := NewRegistry()
	for _, p := range []Provider{acSinPiso{name: "sin-piso"}, acConPiso{acSinPiso: acSinPiso{name: "con-piso"}, piso: 1.25},
		acConPiso{acSinPiso: acSinPiso{name: "claude-falso"}, piso: 0.75}} {
		if err := r.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]float64{"sin-piso": 0, "con-piso": 1.25, "claude-falso": 0.75}
	infos := r.Detect()
	if len(infos) != 3 {
		t.Fatalf("CA-328: Detect trae los tres providers del registry propio: %+v", infos)
	}
	for _, in := range infos {
		if !acCasi(in.MinBudgetUSD, want[in.Name]) {
			t.Fatalf("CA-328: min_budget_usd de %s es %v, fue %v", in.Name, want[in.Name], in.MinBudgetUSD)
		}
		if in.Installed {
			t.Fatalf("CA-328: el binario de %s no existe: installed false", in.Name)
		}
	}
	for _, p := range r.All() {
		if got := MinBudgetUSD(p); !acCasi(got, want[p.Name()]) {
			t.Fatalf("CA-328: MinBudgetUSD(%s) es %v, fue %v", p.Name(), want[p.Name()], got)
		}
	}
	if _, ok := Provider(acSinPiso{name: "x"}).(BudgetFloor); ok {
		t.Fatal("CA-328: el fixture sin piso no implementa BudgetFloor")
	}
}

// CA-328: JSONBytes (lo que emiten hoom providers --json y /api/providers)
// trae min_budget_usd en cada provider, tambien cuando es 0.
func TestCA328_JSONBytesEmiteMinBudget(t *testing.T) {
	t.Setenv("PATH", acBins(t, "claude", "codex"))
	raw, err := JSONBytes()
	if err != nil {
		t.Fatal(err)
	}
	var lista []map[string]any
	if err := json.Unmarshal(raw, &lista); err != nil {
		t.Fatalf("CA-328: JSONBytes es una lista JSON: %v\n%s", err, raw)
	}
	if len(lista) != len(acMinimos) {
		t.Fatalf("CA-328: JSONBytes trae los cuatro providers:\n%s", raw)
	}
	for _, p := range lista {
		name, _ := p["name"].(string)
		v, ok := p["min_budget_usd"].(float64)
		if !ok {
			t.Fatalf("CA-328: %s trae min_budget_usd como numero (0 incluido, sin omitir):\n%s", name, raw)
		}
		if !acCasi(v, acMinimos[name]) {
			t.Fatalf("CA-328: min_budget_usd de %s en JSON es %v, fue %v", name, acMinimos[name], v)
		}
	}
	if !strings.Contains(string(raw), `"min_budget_usd": 0.5`) {
		t.Fatalf("CA-328: claude emite \"min_budget_usd\": 0.5:\n%s", raw)
	}
}
