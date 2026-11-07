package providers

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Ventana is one plan window as the provider reported it at one moment
// (spec costo-por-spec): how much of it the account had used and when it
// resets. It is the account's window, not the run's: anything else spending
// on the same account at the same time moves it too.
type Ventana struct {
	// Nombre: "5h", "semana", or "<n>min" for a window hoom has no name for.
	Nombre string
	// Usado is the percent used, 0..100, as the provider gave it (a fraction
	// is converted, never rounded).
	Usado float64
	// ReiniciaEn is when the window resets; zero = not reported.
	ReiniciaEn time.Time
	// Plan is the plan the provider named ("plus"); "" = not reported.
	Plan string
}

// Medicion is what a provider said about a run beyond its narration: the
// model and effort it ran with, and every report of its plan windows.
type Medicion struct {
	Model  string
	Effort string
	// Muestras holds one snapshot per report, oldest first; each snapshot
	// lists the windows that report carried.
	Muestras [][]Ventana
}

// Medidor is implemented by an adapter whose CLI reports which model ran and
// how the plan windows moved. runcmd reads it beside Normalize, so the
// narration events stay exactly as they are.
type Medidor interface {
	// MedirLinea reads one stdout line of the CLI (Claude: system/init and
	// rate_limit_event). A line that says nothing gives the zero Medicion.
	MedirLinea(line string) Medicion
	// MedirSesion reads what the CLI wrote in its own session file about the
	// session sessionID, keeping only the reports dated at or after desde
	// (Codex: turn_context and token_count). A CLI without such a file gives
	// the zero Medicion and no error.
	MedirSesion(sessionID string, desde time.Time) (Medicion, error)
}

// CodexHome is where Codex keeps its state: $CODEX_HOME, or ~/.codex.
func CodexHome() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// MedirLinea reads Claude's system/init (the model) and rate_limit_event
// (the windows). The cheap test first: every other line of the stream —
// tool results of megabytes among them — is not decoded a second time.
func (claude) MedirLinea(line string) Medicion {
	if !strings.Contains(line, `"rate_limit_event"`) && !strings.Contains(line, `"init"`) {
		return Medicion{}
	}
	var msg struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		Model     string `json:"model"`
		RateLimit *struct {
			UnifiedWindows map[string]struct {
				Utilization *float64 `json:"utilization"`
				ResetsAt    *float64 `json:"resetsAt"`
			} `json:"unifiedWindows"`
		} `json:"rate_limit_info"`
	}
	if json.Unmarshal([]byte(line), &msg) != nil {
		return Medicion{}
	}
	switch {
	case msg.Type == "system" && msg.Subtype == "init":
		return Medicion{Model: strings.TrimSpace(msg.Model)}
	case msg.Type == "rate_limit_event" && msg.RateLimit != nil:
		var snap []Ventana
		for key, w := range msg.RateLimit.UnifiedWindows {
			nombre, ok := claudeVentanas[key]
			// a fraction outside 0..1 is no percent of a window: that sample
			// is "sin dato", never a figure that poisons the totals
			if !ok || w.Utilization == nil || !finito(*w.Utilization) || *w.Utilization < 0 || *w.Utilization > 1 {
				continue
			}
			v := Ventana{Nombre: nombre, Usado: *w.Utilization * 100}
			if w.ResetsAt != nil {
				v.ReiniciaEn = epoch(*w.ResetsAt)
			}
			snap = append(snap, v)
		}
		if len(snap) == 0 {
			return Medicion{}
		}
		OrdenarVentanas(snap)
		return Medicion{Muestras: [][]Ventana{snap}}
	}
	return Medicion{}
}

// claudeVentanas names the unifiedWindows hoom knows. Another key is not
// guessed: it is left out.
var claudeVentanas = map[string]string{"five_hour": "5h", "seven_day": "semana"}

// MedirSesion: Claude leaves no session file hoom reads.
func (claude) MedirSesion(sessionID string, desde time.Time) (Medicion, error) {
	return Medicion{}, nil
}

// MedirLinea: Codex does not report its windows in the exec --json stream.
func (codex) MedirLinea(line string) Medicion { return Medicion{} }

// threadRe is the shape of a Codex thread id: it goes into a glob pattern,
// so anything that is not a plain id is refused before it gets there.
var threadRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// maxLineaSesion caps one line of a Codex session file. A longer line is a
// message (never a token_count): it is skipped, and the reading goes on.
const maxLineaSesion = 4 << 20

// The bounds of one MedirSesion: the run's close waits for it, so a huge
// thread, a file that keeps growing or a directory full of look-alikes ends
// the reading with "sin dato" instead of holding the run in running.
var limitesSesion = struct {
	bytes    int64
	archivos int
	plazo    time.Duration
}{bytes: 256 << 20, archivos: 8, plazo: 10 * time.Second}

// errTopeSesion is a session reading cut by its bounds.
var errTopeSesion = errors.New("la sesion de codex supera el tope de lectura de hoom: sin dato")

// presupuesto is what is left of one MedirSesion's bounds.
type presupuesto struct {
	bytes int64
	hasta time.Time
}

func (p *presupuesto) gastar(n int) error {
	p.bytes -= int64(n)
	if p.bytes < 0 || time.Now().After(p.hasta) {
		return errTopeSesion
	}
	return nil
}

// MedirSesion reads the Codex session files of thread sessionID under
// CodexHome()/sessions: the files whose session_meta.id IS the thread (a
// file can carry it in the name and be another's). It reads only session_meta, turn_context
// and token_count — never the messages — and only the reports dated at or
// after desde. No file, or an unreadable one, is an error with the zero
// Medicion: the run that asks closes all the same.
func (codex) MedirSesion(sessionID string, desde time.Time) (Medicion, error) {
	if !threadRe.MatchString(sessionID) {
		return Medicion{}, fmt.Errorf("thread de codex invalido %q", sessionID)
	}
	home := CodexHome()
	if home == "" {
		return Medicion{}, errors.New("sin CODEX_HOME ni directorio personal")
	}
	files, err := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*-"+sessionID+".jsonl"))
	if err != nil {
		return Medicion{}, err
	}
	// the names start with the session's date, so the order is the order the
	// files were opened in: a resumed thread may leave a second file whose
	// session_meta names the same thread, and its reports come after
	sort.Strings(files)
	if len(files) > limitesSesion.archivos {
		return Medicion{}, errTopeSesion
	}
	budget := &presupuesto{bytes: limitesSesion.bytes, hasta: time.Now().Add(limitesSesion.plazo)}
	lastErr := fmt.Errorf("no hay archivo de sesion de codex para el thread %s", sessionID)
	var out Medicion
	encontrado := false
	for _, f := range files {
		med, suyo, err := leerSesionCodex(f, sessionID, desde, budget)
		if errors.Is(err, errTopeSesion) {
			return Medicion{}, err
		}
		if err != nil {
			lastErr = err
			continue
		}
		if !suyo {
			continue
		}
		encontrado = true
		if med.Model != "" {
			out.Model = med.Model
		}
		if med.Effort != "" {
			out.Effort = med.Effort
		}
		out.Muestras = append(out.Muestras, med.Muestras...)
	}
	if !encontrado {
		return Medicion{}, lastErr
	}
	return out, nil
}

// leerSesionCodex reads one session file: whether its session_meta names
// the thread and, if so, what its reports since desde said.
func leerSesionCodex(path, thread string, desde time.Time, budget *presupuesto) (Medicion, bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Medicion{}, false, err
	}
	if !fi.Mode().IsRegular() {
		return Medicion{}, false, fmt.Errorf("%s no es un archivo regular", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return Medicion{}, false, err
	}
	defer f.Close()
	var med Medicion
	suyo, visto := false, false
	br := bufio.NewReaderSize(f, 64<<10)
	for {
		line, largo, leidos, err := leerLineaAcotada(br, maxLineaSesion)
		if berr := budget.gastar(leidos); berr != nil {
			return Medicion{}, false, berr
		}
		if !largo && len(line) > 0 {
			switch codexTipo(line) {
			case "session_meta":
				// the first session_meta says whose file this is; a later one
				// (a fork that carries its parent's) does not change it
				var m struct {
					Payload struct {
						ID string `json:"id"`
					} `json:"payload"`
				}
				if !visto && json.Unmarshal(line, &m) == nil {
					visto = true
					if m.Payload.ID != thread {
						return Medicion{}, false, nil // another thread's file
					}
					suyo = true
				}
			case "turn_context":
				var m struct {
					Timestamp time.Time `json:"timestamp"`
					Payload   struct {
						Model  string `json:"model"`
						Effort string `json:"effort"`
					} `json:"payload"`
				}
				if json.Unmarshal(line, &m) == nil && !m.Timestamp.Before(desde) {
					if v := strings.TrimSpace(m.Payload.Model); v != "" {
						med.Model = v
					}
					if v := strings.TrimSpace(m.Payload.Effort); v != "" {
						med.Effort = v
					}
				}
			case "token_count":
				var m struct {
					Timestamp time.Time `json:"timestamp"`
					Payload   struct {
						RateLimits *struct {
							Primary   *codexVentana `json:"primary"`
							Secondary *codexVentana `json:"secondary"`
							PlanType  *string       `json:"plan_type"`
						} `json:"rate_limits"`
					} `json:"payload"`
				}
				if json.Unmarshal(line, &m) == nil && !m.Timestamp.Before(desde) && m.Payload.RateLimits != nil {
					rl := m.Payload.RateLimits
					plan := ""
					if rl.PlanType != nil {
						plan = strings.TrimSpace(*rl.PlanType)
					}
					var snap []Ventana
					for _, w := range []*codexVentana{rl.Primary, rl.Secondary} {
						if v, ok := w.ventana(plan); ok {
							snap = append(snap, v)
						}
					}
					if len(snap) > 0 {
						OrdenarVentanas(snap)
						med.Muestras = append(med.Muestras, snap)
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return Medicion{}, false, err
		}
	}
	if !suyo {
		return Medicion{}, false, nil
	}
	return med, true, nil
}

// codexTipo is the kind of a session line, read from its first bytes only:
// `{"timestamp":"...","type":"<t>"` and, for an event_msg, the payload's own
// type. A message line is never decoded.
func codexTipo(line []byte) string {
	head := line
	if len(head) > 160 {
		head = head[:160]
	}
	h := string(head)
	switch {
	case strings.Contains(h, `"type":"session_meta"`):
		return "session_meta"
	case strings.Contains(h, `"type":"turn_context"`):
		return "turn_context"
	case strings.Contains(h, `"type":"event_msg"`) && strings.Contains(h, `"payload":{"type":"token_count"`):
		return "token_count"
	}
	return ""
}

// codexVentana is one of rate_limits.primary/secondary.
type codexVentana struct {
	UsedPercent   *float64 `json:"used_percent"`
	WindowMinutes *int     `json:"window_minutes"`
	ResetsAt      *float64 `json:"resets_at"`
}

func (w *codexVentana) ventana(plan string) (Ventana, bool) {
	if w == nil || w.UsedPercent == nil || w.WindowMinutes == nil || !finito(*w.UsedPercent) ||
		*w.UsedPercent < 0 || *w.UsedPercent > 100 || *w.WindowMinutes <= 0 {
		return Ventana{}, false
	}
	v := Ventana{Nombre: NombreVentana(*w.WindowMinutes), Usado: *w.UsedPercent, Plan: plan}
	if w.ResetsAt != nil {
		v.ReiniciaEn = epoch(*w.ResetsAt)
	}
	return v, true
}

// NombreVentana names a window by its length: 300 minutes is "5h", 10080 is
// "semana", anything else "<n>min".
func NombreVentana(minutos int) string {
	switch minutos {
	case 300:
		return "5h"
	case 10080:
		return "semana"
	}
	return strconv.Itoa(minutos) + "min"
}

// OrdenarVentanas puts windows in the order hoom shows them: 5h, semana,
// then the rest by name.
func OrdenarVentanas(vs []Ventana) {
	sort.SliceStable(vs, func(i, j int) bool { return MenorVentana(vs[i].Nombre, vs[j].Nombre) })
}

// MenorVentana is the order of window names: 5h, semana, then by name.
func MenorVentana(a, b string) bool {
	ra, rb := rangoVentana(a), rangoVentana(b)
	if ra != rb {
		return ra < rb
	}
	return a < b
}

func rangoVentana(n string) int {
	switch n {
	case "5h":
		return 0
	case "semana":
		return 1
	}
	return 2
}

// epoch turns a provider's epoch seconds into UTC; a value that is no time
// at all (zero, negative, past year 9999) is "not reported".
func epoch(s float64) time.Time {
	if !finito(s) || s <= 0 || s > 253402300799 {
		return time.Time{}
	}
	sec, frac := math.Modf(s)
	return time.Unix(int64(sec), int64(frac*1e9)).UTC()
}

func finito(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// leerLineaAcotada reads one line of br without its newline. A line longer
// than max is read to its end and reported as largo, with no content: the
// reader never holds it whole. leidos is every byte it consumed.
func leerLineaAcotada(br *bufio.Reader, max int) ([]byte, bool, int, error) {
	var buf []byte
	largo := false
	leidos := 0
	for {
		frag, err := br.ReadSlice('\n')
		leidos += len(frag)
		if !largo {
			if len(buf)+len(frag) > max+1 {
				largo, buf = true, nil
			} else {
				buf = append(buf, frag...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if len(buf) > 0 && buf[len(buf)-1] == '\n' {
			buf = buf[:len(buf)-1]
		}
		return buf, largo, leidos, err
	}
}

// EntradaNueva is the input of u that did not come from the cache, in the
// accounting of provider: Codex counts the cached tokens inside
// input_tokens, while Claude's InputTokens (input plus cache creation, as
// the adapter stores it) never had them. A provider whose accounting hoom
// does not know gives false: no figure rather than a guessed one.
func EntradaNueva(provider string, u Usage) (int, bool) {
	switch provider {
	case "claude":
		return u.InputTokens, true
	case "codex":
		return max(u.InputTokens-u.CachedTokens, 0), true
	}
	return 0, false
}
