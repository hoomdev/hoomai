package costcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// NotaVentana is said by every output that shows a window figure: the
// window is the account's, not the run's.
const NotaVentana = "(ventana de la cuenta durante los runs)"

// JSONBytes renders the report as indented JSON, with the schema the spec
// fixes: empty lists and maps are [] and {}, a missing figure is null.
func JSONBytes(r Reporte) ([]byte, error) {
	if r.Tareas == nil {
		r.Tareas = []Tarea{}
	}
	for i := range r.Tareas {
		t := &r.Tareas[i]
		if t.PorProvider == nil {
			t.PorProvider = map[string]Gasto{}
		}
		if t.PorRol == nil {
			t.PorRol = []GastoRol{}
		}
		if t.RondasReview == nil {
			t.RondasReview = []Ronda{}
		}
		for j := range t.RondasReview {
			if t.RondasReview[j].Lentes == nil {
				t.RondasReview[j].Lentes = []string{}
			}
			if t.RondasReview[j].Hallazgos == nil {
				t.RondasReview[j].Hallazgos = map[string]int{"high": 0, "medium": 0, "low": 0}
			}
		}
	}
	return json.MarshalIndent(r, "", "  ")
}

// Render writes the report for humans: one line per task, or the breakdown
// of each task when the report was built for one slug (detalle).
func Render(w io.Writer, r Reporte, detalle bool) {
	if len(r.Tareas) == 0 {
		fmt.Fprintln(w, "ninguna tarea tiene runs ni reviews")
	}
	for i, t := range r.Tareas {
		if detalle {
			if i > 0 {
				fmt.Fprintln(w)
			}
			renderDetalle(w, t)
			continue
		}
		renderLinea(w, t)
	}
	if r.Ilegibles > 0 {
		fmt.Fprintf(w, "%d runs ilegibles\n", r.Ilegibles)
	}
}

// renderLinea is the task in one line; the window note goes on the line, so
// the list stays one line per task.
func renderLinea(w io.Writer, t Tarea) {
	parts := []string{t.Slug, estadoTexto(t), fmt.Sprintf("rondas %d", t.Rondas)}
	conVentana := false
	for _, p := range provsDe(t.PorProvider) {
		g := t.PorProvider[p]
		parts = append(parts, p+": "+gastoTexto(p, g))
		conVentana = conVentana || g.Ventanas != nil
	}
	line := strings.Join(parts, " · ")
	if conVentana {
		line += " " + NotaVentana
	}
	fmt.Fprintln(w, line)
}

func renderDetalle(w io.Writer, t Tarea) {
	fmt.Fprintf(w, "%s · %s · rondas %d\n", t.Slug, estadoTexto(t), t.Rondas)
	if t.Estado == EstadoAceptada {
		v, c := "sin dato", "sin dato"
		if t.Veredicto != nil {
			v = *t.Veredicto
		}
		if t.CommitFinal != nil {
			c = *t.CommitFinal
		}
		fmt.Fprintf(w, "veredicto %s · commit_final %s\n", v, c)
	}
	fmt.Fprintln(w, "por rol")
	if len(t.PorRol) == 0 {
		fmt.Fprintln(w, "  sin runs")
	}
	for _, r := range t.PorRol {
		rol := r.Rol
		if rol == "" {
			rol = "sin rol"
		}
		fmt.Fprintf(w, "  %s · %s: %s\n", rol, r.Provider, gastoTexto(r.Provider, r.Gasto))
	}
	fmt.Fprintln(w, "por ronda de review")
	if len(t.RondasReview) == 0 {
		fmt.Fprintln(w, "  ninguna")
	}
	for _, r := range t.RondasReview {
		if r.SinRonda {
			fmt.Fprintf(w, "  reviewer (sin ronda) · %s: %s\n", r.Provider, gastoPtrTexto(r.Provider, r.Gasto))
			continue
		}
		id := ""
		if r.ID != nil {
			id = *r.ID
		}
		cob := r.Cobertura
		if cob == "" {
			cob = "sin dato"
		}
		fmt.Fprintf(w, "  %s · %s · lentes %s · high %d medium %d low %d · %s: %s\n", id, cob,
			strings.Join(r.Lentes, ","), r.Hallazgos["high"], r.Hallazgos["medium"], r.Hallazgos["low"],
			r.Provider, gastoPtrTexto(r.Provider, r.Gasto))
	}
	fmt.Fprintln(w, "total")
	if len(t.PorProvider) == 0 {
		fmt.Fprintln(w, "  sin runs")
	}
	for _, p := range provsDe(t.PorProvider) {
		fmt.Fprintf(w, "  %s: %s\n", p, gastoTexto(p, t.PorProvider[p]))
	}
	fmt.Fprintln(w, NotaVentana)
}

func estadoTexto(t Tarea) string {
	switch t.Estado {
	case EstadoAceptada:
		if t.AceptadaEn != nil {
			return "aceptada " + t.AceptadaEn.UTC().Format("2006-01-02 15:04") + " UTC"
		}
		return "aceptada"
	case EstadoEnCurso:
		return "en curso"
	}
	return "cerrada sin aceptar"
}

func provsDe(m map[string]Gasto) []string {
	out := make([]string, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func gastoPtrTexto(provider string, g *Gasto) string {
	if g == nil {
		return "sin runs en el registro (anterior a costo-por-spec)"
	}
	return gastoTexto(provider, *g)
}

// gastoTexto is one provider's figures, in the strings the package doc fixes.
func gastoTexto(provider string, g Gasto) string {
	parts := []string{plural(g.Runs, "run", "runs")}
	if g.EnCurso > 0 {
		parts = append(parts, fmt.Sprintf("%d en curso", g.EnCurso))
	}
	parts = append(parts,
		cifra("nuevos", g.Tokens.Nuevos), cifra("cache leido", g.Tokens.CacheLeido),
		cifra("salida", g.Tokens.Salida), cifra("razonamiento", g.Tokens.Razonamiento))
	terminados := g.Runs - g.EnCurso
	if g.CostoUSD == nil {
		parts = append(parts, provider+" no informa costo")
	} else {
		usd := fmt.Sprintf("%.2f USD equivalente API", *g.CostoUSD)
		if g.SinCosto > 0 {
			usd += fmt.Sprintf(", costo: sin dato en %d de %d runs", g.SinCosto, g.Runs)
		}
		parts = append(parts, usd)
	}
	if g.Ventanas == nil {
		if terminados > 0 {
			parts = append(parts, "ventana: sin dato")
		}
	} else {
		nombres := make([]string, 0, len(g.Ventanas))
		for n := range g.Ventanas {
			nombres = append(nombres, n)
		}
		sort.Slice(nombres, func(i, j int) bool { return providers.MenorVentana(nombres[i], nombres[j]) })
		for _, n := range nombres {
			parts = append(parts, n+" "+puntosTexto(g.Ventanas[n]))
		}
		if g.SinVentana > 0 {
			parts = append(parts, fmt.Sprintf("ventana: sin dato en %d de %d runs", g.SinVentana, terminados))
		}
	}
	if g.DuracionMS != nil {
		parts = append(parts, "tiempo "+runcmd.Duracion(time.Duration(*g.DuracionMS)*time.Millisecond))
	}
	return strings.Join(parts, " · ")
}

func cifra(label string, v *int) string {
	if v == nil {
		return label + " sin dato"
	}
	return label + " " + strconv.Itoa(*v)
}

// puntosTexto: "<x> puntos" with at most one decimal, "<1 punto" under one
// point, "al menos " in front when the window reset.
func puntosTexto(m Movimiento) string {
	if m.Puntos == nil {
		return "sin dato"
	}
	s := ""
	if m.AlMenos {
		s = "al menos "
	}
	if *m.Puntos < 1 {
		return s + "<1 punto"
	}
	x := math.Round(*m.Puntos*10) / 10
	return s + strconv.FormatFloat(x, 'f', -1, 64) + " puntos"
}

func plural(n int, uno, varios string) string {
	if n == 1 {
		return "1 " + uno
	}
	return fmt.Sprintf("%d %s", n, varios)
}
