package providers

import (
	"os"
	"path/filepath"
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

// MedirLinea: esqueleto (costo-por-spec), sin comportamiento todavia.
func (claude) MedirLinea(line string) Medicion { return Medicion{} }

// MedirSesion: Claude no deja un archivo de sesion que hoom lea.
func (claude) MedirSesion(sessionID string, desde time.Time) (Medicion, error) {
	return Medicion{}, nil
}

// MedirLinea: Codex no informa la ventana en el stream de exec --json.
func (codex) MedirLinea(line string) Medicion { return Medicion{} }

// MedirSesion: esqueleto (costo-por-spec), sin comportamiento todavia.
func (codex) MedirSesion(sessionID string, desde time.Time) (Medicion, error) {
	return Medicion{}, nil
}
