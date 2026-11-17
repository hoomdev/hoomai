package tablero

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/finding"
)

// signo is the sign of segment i of the card's meter: ✋ on the first falta
// when the card needs your decision.
func (m *Modelo) signo(c *boardcmd.Card, i int) string {
	seg := c.Meter[i]
	switch seg.State {
	case boardcmd.SegHecho:
		return m.s.hecho
	case boardcmd.SegNoAplica:
		return m.s.noAplica
	}
	if c.NeedsDecision {
		primera := true
		for _, s := range c.Meter[:i] {
			if s.State == boardcmd.SegFalta {
				primera = false
				break
			}
		}
		if primera {
			return m.s.decision
		}
	}
	return m.s.falta
}

// ---------------------------------------------------------------- the Hallazgos view

func (m *Modelo) hallazgoElegido() (finding.Item, bool) {
	for _, h := range m.hallazgos {
		if h.ID == m.sel4 {
			return h, true
		}
	}
	return finding.Item{}, false
}

func (m *Modelo) elegir4(i int) {
	if len(m.hallazgos) == 0 {
		m.sel4, m.idx4 = "", 0
		return
	}
	i = max(0, min(i, len(m.hallazgos)-1))
	m.sel4, m.idx4 = m.hallazgos[i].ID, i
}

func (m *Modelo) acomodar4() {
	for i, h := range m.hallazgos {
		if h.ID == m.sel4 {
			m.idx4 = i
			return
		}
	}
	m.elegir4(m.idx4)
}

// estiloSeveridad is a severity's color.
func estiloSeveridad(s string) estilo {
	switch s {
	case "high":
		return rojo
	case "medium":
		return amarillo
	case "low":
		return gris
	}
	return normal
}

// donde is a finding's place as the rows write it: the base name of its file
// and ":<linea>", or "-" without a file.
func donde(f finding.Item) string {
	archivo, n := archivoYLinea(f)
	if archivo == "" {
		return "-"
	}
	if i := strings.LastIndexAny(archivo, `/\`); i >= 0 {
		archivo = archivo[i+1:]
	}
	if n > 0 {
		archivo += ":" + strconv.Itoa(n)
	}
	return archivo
}

// contenido4 is the Hallazgos view's list for an inner width wi.
func (m *Modelo) contenido4(wi int) []linea {
	if !m.hallLeidos {
		return []linea{lin(" leyendo los hallazgos...")}
	}
	if len(m.hallazgos) == 0 {
		return []linea{lin(" sin hallazgos abiertos")}
	}
	lugar := wi - 10
	tw := lugar / 2
	dw := lugar - tw - 1
	var out []linea
	for i, h := range m.hallazgos {
		marca := " "
		if i == m.idx4 {
			marca = m.s.sel
		}
		tarea := h.Task
		if tarea == "" {
			tarea = "-"
		}
		l := lin(" "+marca+" ").masCon(h.Severity, estiloSeveridad(h.Severity)).
			mas(strings.Repeat(" ", max(0, 6-celdas(h.Severity))) + " " + rellenar(cortar(tarea, tw, m.s.corte), tw) + " " +
				cortar(donde(h), dw, m.s.corte))
		out = append(out, l)
	}
	return out
}

// archivoYLinea is the finding's file and its line (0 without one).
func archivoYLinea(f finding.Item) (string, int) {
	archivo := f.File
	if i := strings.LastIndex(archivo, ":"); i >= 0 && digitos(archivo[i+1:]) {
		n, _ := strconv.Atoi(archivo[i+1:])
		return archivo[:i], n
	}
	if archivo == "" {
		return "", 0
	}
	if i := strings.Index(f.Description, archivo+":"); i >= 0 {
		resto := f.Description[i+len(archivo)+1:]
		j := 0
		for j < len(resto) && resto[j] >= '0' && resto[j] <= '9' {
			j++
		}
		if j > 0 {
			n, _ := strconv.Atoi(resto[:j])
			return archivo, n
		}
	}
	return archivo, 0
}

func digitos(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- the project

// lineaCheck is the check as the header and the project's detail write it,
// without a margin, in pieces.
func (m *Modelo) lineaCheck() linea {
	p := m.proyecto
	switch {
	case p.Check.OK:
		l := lin("check ").masCon("VERDE", verde)
		v := p.VeredictoDelCheck
		if v == nil && p.Veredicto != nil && p.Veredicto.ID == p.Check.VerdictID {
			v = p.Veredicto // the newest is the one the check refers to
		}
		if v != nil {
			l = l.mas(" · verify hace " + hace(m.ahora.Sub(v.CreatedAt)))
		}
		return l
	case p.Check.Reason == checkcmd.ReasonNoVerdict:
		return lin("sin veredicto todavia")
	}
	return lin("check ").masCon("ROJO", rojo).mas(": " + checkcmd.Motivo(p.Check))
}

// contenido1 is the first four lines of the project's detail (the v2's [1]).
func (m *Modelo) contenido1() []linea {
	fijada := " sin tarjeta fijada"
	if m.fijada != "" {
		fijada = " fijada: " + m.fijada
	}
	p := m.proyecto
	if p == nil {
		return []linea{lin(" " + m.c.Proyecto), lin(" leyendo el proyecto..."), lin(""), lin(fijada)}
	}
	uno := " " + m.c.Proyecto + " · sin git"
	if p.Git {
		uno = " " + m.c.Proyecto + " · rama " + p.Rama
	}
	hoy := " hoy sin trabajos"
	if p.Hoy.Runs > 0 {
		hoy = fmt.Sprintf(" hoy %.2f USD en %d trabajos", p.Hoy.CostoUSD, p.Hoy.Runs)
		if p.Hoy.SinCosto > 0 {
			hoy += fmt.Sprintf(" · %d sin dato", p.Hoy.SinCosto)
		}
	}
	return []linea{lin(uno), append(lin(" "), m.lineaCheck()...), lin(hoy), lin(fijada)}
}

// ---------------------------------------------------------------- times

// hace is a time ago: "<n>s" below a minute, else the biggest whole unit
// among m, h and d.
func hace(d time.Duration) string {
	s := segundos(d)
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm", s/60)
	case s < 86400:
		return fmt.Sprintf("%dh", s/3600)
	}
	return fmt.Sprintf("%dd", s/86400)
}

// reloj is a duration as m:ss below an hour and h:mm:ss from an hour on.
func reloj(d time.Duration) string {
	s := segundos(d)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// segundos are the whole seconds of d (0 when negative).
func segundos(d time.Duration) int {
	if d < 0 {
		return 0
	}
	return int(d / time.Second)
}

// gasto is the card's cost with two decimals, or "sin dato de costo".
func gasto(s boardcmd.Spend) string {
	if s.CostUSD == nil {
		return "sin dato de costo"
	}
	return fmt.Sprintf("%.2f USD", *s.CostUSD)
}

// etapa is "<stage> <step>/<steps>", or the stage alone when steps is 0.
func etapa(stage string, step, steps int) string {
	if steps == 0 {
		return stage
	}
	return strings.TrimSpace(fmt.Sprintf("%s %d/%d", stage, step, steps))
}
