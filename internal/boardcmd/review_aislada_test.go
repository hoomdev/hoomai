// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-408) en el tablero: la tarjeta trae el modelo y el esfuerzo del
// reviewer del MISMO registro de review del que sale el reviewer; "" cuando
// el registro no los dice (registros viejos incluidos). El tablero no suma el
// gasto del registro.
package boardcmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

func raRegistro(id string, en time.Duration, cuenta bool, provider, model, effort string) reviewcmd.Record {
	r := tbRegistro(id, en, cuenta, provider, "claude", reviewcmd.CrossYes)
	r.Model, r.Effort = model, effort
	return r
}

func raModelo(t *testing.T, ev Evidence, reviewer, model, effort string) {
	t.Helper()
	p := Derive(ev).Providers
	if p.Reviewer != reviewer || p.ReviewerModel != model || p.ReviewerEffort != effort {
		t.Fatalf("CA-408: providers debe ser {reviewer %q, reviewer_model %q, reviewer_effort %q}, fue %+v",
			reviewer, model, effort, p)
	}
}

// CA-408: con un registro que cuenta, modelo y esfuerzo salen de el, aunque
// haya uno mas nuevo que no cuenta con otros valores.
func TestCA408_ModeloYEsfuerzoDelRegistroQueCuenta(t *testing.T) {
	ev := bdEv()
	ev.Reviews = []reviewcmd.Record{
		raRegistro("r-nuevo-una-lente", 40*time.Minute, false, "gemini", "otro-modelo", "low"),
		raRegistro("r-cuenta", 35*time.Minute, true, "codex", "gpt-5.6-sol", "xhigh"),
	}
	if c := Derive(ev); c.Evidence.ReviewID != "r-cuenta" {
		t.Fatalf("CA-408: fixture: r-cuenta es el registro que cuenta: %+v", c.Evidence)
	}
	raModelo(t, ev, "codex", "gpt-5.6-sol", "xhigh")
}

// CA-408: si ninguno cuenta, del mas nuevo (por fecha); un vacio en el
// registro es "" (sin registrar), no el valor de otro registro.
func TestCA408_ModeloYEsfuerzoDelMasNuevoYVacios(t *testing.T) {
	ev := bdEv()
	ev.Reviews = []reviewcmd.Record{
		raRegistro("r-medio", 30*time.Minute, false, "codex", "m-medio", "e-medio"),
		raRegistro("r-nuevo", 50*time.Minute, false, "claude", "opus", ""),
		raRegistro("r-viejo", 10*time.Minute, false, "gemini", "m-viejo", "e-viejo"),
	}
	raModelo(t, ev, "claude", "opus", "")

	// un registro viejo, sin modelo ni esfuerzo: los dos vacios
	ev = bdEv()
	ev.Reviews = []reviewcmd.Record{tbRegistro("r-viejo", 35*time.Minute, true, "codex", "claude", reviewcmd.CrossYes)}
	raModelo(t, ev, "codex", "", "")

	// sin registro no hay reviewer, ni modelo ni esfuerzo
	raModelo(t, bdEv(), "", "", "")
}

// CA-408: el JSON de la tarjeta trae providers.reviewer_model y
// providers.reviewer_effort; el tablero no suma el gasto del registro.
func TestCA408_JSONDeLaTarjeta(t *testing.T) {
	ev := bdEv()
	r := raRegistro("r-cuenta", 35*time.Minute, true, "codex", "gpt-5.6-sol", "high")
	r.Usage = []reviewcmd.LensUsage{{Lens: "risk", InputTokens: 999999, CachedTokens: 1, OutputTokens: 777777, Turns: 3}}
	ev.Reviews = []reviewcmd.Record{r}
	c := Derive(ev)
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Providers map[string]any `json:"providers"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Providers["reviewer_model"] != "gpt-5.6-sol" || m.Providers["reviewer_effort"] != "high" {
		t.Fatalf("CA-408: la tarjeta trae providers.reviewer_model y reviewer_effort: %v", m.Providers)
	}
	if strings.Contains(string(raw), "999999") || strings.Contains(string(raw), "777777") {
		t.Fatalf("CA-408/CA-334: el tablero no suma ni muestra el gasto del registro de review: %s", raw)
	}
}
