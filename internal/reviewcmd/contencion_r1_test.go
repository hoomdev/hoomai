// Ronda 1 del test-writer de contencion-del-writer: el Escribe de un
// provider en la review (ids 425a75 y 03a36d; aclaracion 26), en cualquier
// sistema. Un provider falso con ConEstado, registrado en providers.Default,
// cuyo Escribe pone el test; su CLI lo arma fakeProvider (cliFalso: drena
// su stdin). Este archivo no escribe shebangs.
package reviewcmd

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/providers"
)

const r1RevNombre = "r1-revisa"

type r1RevProv struct{}

var (
	r1RevMu      sync.Mutex
	r1RevEscribe []string
	r1RevOnce    sync.Once
	r1RevErr     error
	_            providers.ConEstado = r1RevProv{}
)

func (r1RevProv) Name() string { return r1RevNombre }
func (r1RevProv) Bin() string  { return r1RevNombre }
func (r1RevProv) Capabilities() providers.Capabilities {
	return providers.Capabilities{Structured: true, SessionID: true, Model: true, SystemPrompt: true, Tools: true,
		ReadOnly: true, NoExec: true, Unattended: true, MaxTurns: true, Budget: true, Effort: true, Isolation: true}
}
func (p r1RevProv) Command(req providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: p.Bin(), Stdin: req.Prompt}, nil
}
func (r1RevProv) Normalize(string) []providers.Event { return nil }
func (r1RevProv) Estado() providers.Estado {
	r1RevMu.Lock()
	defer r1RevMu.Unlock()
	return providers.Estado{Escribe: append([]string(nil), r1RevEscribe...)}
}

// r1RevPoner registra el provider falso (una vez) y deja su Escribe en
// escribe hasta que termina el test.
func r1RevPoner(t *testing.T, escribe []string) {
	t.Helper()
	r1RevOnce.Do(func() { r1RevErr = providers.Default.Register(r1RevProv{}) })
	if r1RevErr != nil {
		t.Fatalf("fixture: registrar %s: %v", r1RevNombre, r1RevErr)
	}
	r1RevMu.Lock()
	r1RevEscribe = append([]string(nil), escribe...)
	r1RevMu.Unlock()
	t.Cleanup(func() {
		r1RevMu.Lock()
		r1RevEscribe = nil
		r1RevMu.Unlock()
	})
}

// CA-690 y CA-678 (id 425a75; aclaracion 26): con la seccion, la review con
// un provider cuyo Escribe es `/`, el $HOME, el checkout principal o algo
// dentro del directorio comun se niega antes de su primera pasada: exit 1,
// con el error de encierro.ValidarEscribe, sin que su CLI corra, sin run y
// sin registro. Un Escribe valido corre como hoy.
func TestR1_425a75_CA690_CA678_LaReviewValidaElEscribeDelProvider(t *testing.T) {
	const ca = "CA-690"
	const todo = ": es todo el disco o todo el directorio personal: no puede ser escribible"
	const dentro = ": contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible"
	for _, c := range []struct {
		nombre  string
		escribe func(root, home, comun string) string
		cola    string
	}{
		{"todo el disco", func(r, h, c string) string { return "/" }, todo},
		{"el $HOME como ~/", func(r, h, c string) string { return "~/" }, todo},
		{"el $HOME absoluto", func(r, h, c string) string { return h }, todo},
		{"el checkout principal", func(r, h, c string) string { return r }, dentro},
		{"dentro del directorio comun", func(r, h, c string) string { return filepath.Join(c, "objects") }, dentro},
	} {
		root, wt, opt := cdwTarea(t, "encierro:\n  exigir: no\n")
		opt.Provider = r1RevNombre
		home, comun := os.Getenv("HOME"), cdwComun(t, wt)
		ruta := c.escribe(root, home, comun)
		r1RevPoner(t, []string{ruta})
		want := "encierro: " + r1RevNombre + " escribe " + ruta + c.cola
		if err := encierro.ValidarEscribe(r1RevNombre, []string{ruta}, root, comun); err == nil || err.Error() != want {
			t.Fatalf("%s: %s: encierro.ValidarEscribe da el texto del contrato: %v; quiero %q", ca, c.nombre, err, want)
		}
		marca := filepath.Join(t.TempDir(), "invocado")
		fakeProvider(t, r1RevNombre, "echo x >> '"+marca+"'\nexit 0\n")
		antes := len(h3Metas(t, root, wt))
		res, out, err := cdwRevisar(t, root, opt)
		if res.ExitCode != 1 || !(strings.Contains(out, want) || (err != nil && strings.Contains(err.Error(), want))) {
			t.Fatalf("%s: %s: la review se niega con exit 1 y %q: %v %+v\n%s", ca, c.nombre, want, err, res, out)
		}
		if cdwExiste(marca) || len(res.Passes) != 0 && res.Passes[0].RunID != "" || len(h3Metas(t, root, wt)) != antes {
			t.Fatalf("%s: %s: ninguna pasada corre: %+v", ca, c.nombre, res)
		}
		cdwSinRegistros(t, ca, c.nombre, root, wt)
	}

	// control: un Escribe valido corre como hoy (la pasada lanza su CLI)
	root, _, opt := cdwTarea(t, "encierro:\n  exigir: no\n")
	opt.Provider = r1RevNombre
	r1RevPoner(t, []string{t.TempDir(), "~/.r1-revisa/sesiones"})
	marca := filepath.Join(t.TempDir(), "invocado")
	fakeProvider(t, r1RevNombre, "echo x >> '"+marca+"'\nexit 0\n")
	res, out, err := cdwRevisar(t, root, opt)
	if !cdwExiste(marca) || strings.Contains(out, "no puede ser escribible") || (err != nil && strings.Contains(err.Error(), "no puede ser escribible")) {
		t.Fatalf("%s: control: con un Escribe valido la pasada corre: %v %+v\n%s", ca, err, res, out)
	}
}
