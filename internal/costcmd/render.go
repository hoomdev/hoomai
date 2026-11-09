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
	"unicode"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// NotaVentana is said by every output that shows a window figure: the
// window is the account's, not the run's.
const NotaVentana = "(ventana de la cuenta durante los runs)"

// JSONBytes renders the report as indented JSON, with the schema the spec
// fixes: empty lists and maps are [] and {}, a missing figure is null.
func JSONBytes(r Reporte) ([]byte, error) {
	// a copy all the way down to what gets filled in: the caller's report is
	// never changed by serializing it
	r.Tareas = append([]Tarea{}, r.Tareas...)
	r.Avisos = append([]string{}, r.Avisos...)
	for i := range r.Tareas {
		t := &r.Tareas[i]
		t.RondasReview = append([]Ronda(nil), t.RondasReview...)
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
// of each task when the report was built for one slug (detalle). When the
// report has Hoy (the report of every task), its first line is
// LineaHoy(*r.Hoy), before everything else ("ninguna tarea tiene runs ni
// reviews" included).
func Render(w io.Writer, r Reporte, detalle bool) {
	s := salida{w}
	if r.Hoy != nil {
		s.linea(LineaHoy(*r.Hoy))
	}
	if len(r.Tareas) == 0 {
		s.linea("ninguna tarea tiene runs ni reviews")
	}
	for i, t := range r.Tareas {
		if detalle {
			if i > 0 {
				s.linea("")
			}
			renderDetalle(s, t)
			continue
		}
		renderLinea(s, t)
	}
	if r.Ilegibles > 0 {
		s.lineaf("%d runs ilegibles", r.Ilegibles)
	}
	for _, a := range r.Avisos {
		s.lineaf("aviso: %s", a)
	}
}

// fueraTexto is what came after the acceptance, "" when nothing did.
func fueraTexto(t Tarea) string {
	f := t.DespuesDeAceptada
	if f == nil || (f.Runs == 0 && f.Rondas == 0) {
		return ""
	}
	return fmt.Sprintf("despues de aceptada: %s y %s, fuera del costo",
		plural(f.Runs, "run", "runs"), plural(f.Rondas, "ronda", "rondas"))
}

// renderLinea is the task in one line; the window note goes on the line, so
// the list stays one line per task.
func renderLinea(s salida, t Tarea) {
	parts := []string{t.Slug, estadoTexto(t), fmt.Sprintf("rondas %d", t.Rondas)}
	conVentana := false
	for _, p := range provsDe(t.PorProvider) {
		g := t.PorProvider[p]
		parts = append(parts, p+": "+gastoTexto(p, g))
		conVentana = conVentana || g.Ventanas != nil
	}
	if f := fueraTexto(t); f != "" {
		parts = append(parts, f)
	}
	line := strings.Join(parts, " · ")
	if conVentana {
		line += " " + NotaVentana
	}
	s.linea(line)
}

func renderDetalle(s salida, t Tarea) {
	s.lineaf("%s · %s · rondas %d", t.Slug, estadoTexto(t), t.Rondas)
	if t.Estado == EstadoAceptada {
		v, c := "sin dato", "sin dato"
		if t.Veredicto != nil {
			v = *t.Veredicto
		}
		if t.CommitFinal != nil {
			c = *t.CommitFinal
		}
		s.lineaf("veredicto %s · commit_final %s", v, c)
		if f := fueraTexto(t); f != "" {
			s.linea(f)
		}
	}
	s.linea("por rol")
	if len(t.PorRol) == 0 {
		s.linea("  sin runs")
	}
	for _, r := range t.PorRol {
		rol := r.Rol
		if rol == "" {
			rol = "sin rol"
		}
		s.lineaf("  %s · %s: %s", rol, r.Provider, gastoTexto(r.Provider, r.Gasto))
	}
	s.linea("por ronda de review")
	if len(t.RondasReview) == 0 {
		s.linea("  ninguna")
	}
	for _, r := range t.RondasReview {
		if r.SinRonda {
			s.lineaf("  reviewer (sin ronda) · %s: %s", r.Provider, gastoPtrTexto(r.Provider, r.Gasto))
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
		hall := fmt.Sprintf("high %d medium %d low %d", r.Hallazgos["high"], r.Hallazgos["medium"], r.Hallazgos["low"])
		if r.HallazgosSinDato > 0 {
			hall += fmt.Sprintf(" · hallazgos sin dato %d", r.HallazgosSinDato)
		}
		s.lineaf("  %s · %s · lentes %s · %s · %s: %s", id, cob,
			strings.Join(r.Lentes, ","), hall, r.Provider, gastoPtrTexto(r.Provider, r.Gasto))
	}
	s.linea("total")
	if len(t.PorProvider) == 0 {
		s.linea("  sin runs")
	}
	for _, p := range provsDe(t.PorProvider) {
		s.lineaf("  %s: %s", p, gastoTexto(p, t.PorProvider[p]))
	}
	s.linea(NotaVentana)
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
	if g.SinUso > 0 && g.SinUso < g.Runs {
		parts = append(parts, fmt.Sprintf("tokens: sin dato en %d de %d runs", g.SinUso, g.Runs))
	}
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
	if x == 1 {
		return s + "1 punto"
	}
	return s + strconv.FormatFloat(x, 'f', -1, 64) + " puntos"
}

func plural(n int, uno, varios string) string {
	if n == 1 {
		return "1 " + uno
	}
	return fmt.Sprintf("%d %s", n, varios)
}

// salida prints the report one line at a time: each line is built whole
// and then escaped whole, so the only newline a terminal gets is the one
// salida puts at its end. A name that comes from the evidence (a slug, a
// lens, a provider, a window) can neither hide text nor make up a line:
// every control or direction character in it — newline included — and every
// byte that is no UTF-8 goes out escaped (\n, \x1b, \u202e).
type salida struct{ w io.Writer }

// linea prints texto as one line, as it is: a % in it is just a %.
func (s salida) linea(texto string) {
	fmt.Fprintln(s.w, escapar(texto))
}

// lineaf prints one line formatted with fmt.Sprintf(format, args...).
func (s salida) lineaf(format string, args ...any) {
	s.linea(fmt.Sprintf(format, args...))
}

// escapar is s with its control and direction characters, and its bytes that
// are no UTF-8, written as their Go escapes.
func escapar(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, "\\x%02x", s[i])
		case unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r):
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
