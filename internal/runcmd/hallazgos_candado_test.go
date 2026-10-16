// Tests de regresion de los hallazgos 20261014T180914_b6e55b (Descartar,
// CA-346) y 20261014T180925_ac2feb (Guardar, CA-344) de la review de C3
// (.hoom/specs/acciones-desde-la-tarjeta.md), del lado de runcmd. Escritos
// desde el contrato publico (go doc LockArbol, EsperaArbol), sin leer la
// implementacion: Descartar y Guardar miran "no hay un run activo" y despues
// actuan. Para que ningun run se registre en ese medio, registrar un run en
// un arbol (Manager.Start y Manager.Input) toma el candado de ese arbol
// (runcmd.LockArbol), esperando hasta 30 s, y no lo retiene mientras trabaja.
//
// Reusa installFake, initRepo, git y allDetails de runcmd_test.go.
package runcmd

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// kcTomar toma el candado del arbol dir como lo toman Descartar y Guardar, y
// devuelve como soltarlo; si nadie lo suelta antes, se suelta al final.
func kcTomar(t *testing.T, dir string) func() {
	t.Helper()
	suelta, err := LockArbol(dir, time.Second)
	if err != nil {
		t.Fatalf("CA-346: el test toma el candado del arbol %s: %v", dir, err)
	}
	var una sync.Once
	soltar := func() { una.Do(suelta) }
	t.Cleanup(soltar)
	return soltar
}

// kcCorriendo lista los runs que algo registro como en curso: en la memoria
// del manager o en un sidecar de root/.hoom/runs.
func kcCorriendo(m *Manager, root string) []string {
	var ids []string
	for _, r := range m.List() {
		if r.Status == StatusRunning {
			ids = append(ids, "memoria:"+r.ID)
		}
	}
	for _, meta := range Metas(root) {
		if meta.Status == StatusRunning {
			ids = append(ids, "sidecar:"+meta.ID)
		}
	}
	return ids
}

// kcVuelta es lo que devolvio un Start o un Input lanzado en una goroutine,
// y cuando volvio.
type kcVuelta struct {
	run    Run
	err    error
	cuando time.Time
}

// kcLlamada es un Start o un Input corriendo en otra goroutine. Lo que
// devuelve se recoge una sola vez; al final del test se suelta el candado,
// se recoge si hace falta y el run que haya quedado se cancela (despues de
// que su subproceso dijo marca, para que la cancelacion lo alcance).
type kcLlamada struct {
	ch  chan kcVuelta
	got *kcVuelta
}

func (l *kcLlamada) recoger(espera time.Duration) (kcVuelta, bool) {
	if l.got != nil {
		return *l.got, true
	}
	if espera <= 0 {
		select {
		case v := <-l.ch:
			l.got = &v
			return v, true
		default:
			return kcVuelta{}, false
		}
	}
	select {
	case v := <-l.ch:
		l.got = &v
		return v, true
	case <-time.After(espera):
		return kcVuelta{}, false
	}
}

func kcLanzar(t *testing.T, m *Manager, soltar func(), marca string, f func() (Run, error)) *kcLlamada {
	t.Helper()
	l := &kcLlamada{ch: make(chan kcVuelta, 1)}
	t.Cleanup(func() {
		soltar()
		v, ok := l.recoger(35 * time.Second)
		if !ok || v.err != nil {
			return
		}
		hasta := time.Now().Add(5 * time.Second)
		for time.Now().Before(hasta) {
			if _, evs, err := m.Events(v.run.ID, 0); err != nil || strings.Contains(allDetails(evs), marca) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		m.Cancel(v.run.ID)
		m.Wait(v.run.ID)
	})
	go func() {
		r, err := f()
		l.ch <- kcVuelta{r, err, time.Now()}
	}()
	return l
}

// kcSinRegistrar exige, durante ~300 ms con el candado tomado, que la
// llamada no vuelva, que el arbol no figure ocupado y que no haya ningun run
// en curso registrado. Despues suelta el candado y devuelve lo que la
// llamada devolvio, que tiene que volver despues de soltarlo y sin error.
func kcSinRegistrar(t *testing.T, que string, m *Manager, root string, soltar func(), l *kcLlamada) kcVuelta {
	t.Helper()
	limite := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(limite) {
		if v, ok := l.recoger(0); ok {
			t.Fatalf("hallazgos b6e55b/ac2feb (CA-346, CA-344): con el candado del arbol tomado, %s no registra el run: volvio antes de soltarlo con %+v, %v", que, v.run, v.err)
		}
		if id, busy := m.Busy(root); busy {
			t.Fatalf("hallazgos b6e55b/ac2feb (CA-346, CA-344): con el candado del arbol tomado, %s no registra el run: Busy ya ve %s", que, id)
		}
		if ids := kcCorriendo(m, root); len(ids) > 0 {
			t.Fatalf("hallazgos b6e55b/ac2feb (CA-346, CA-344): con el candado del arbol tomado, %s no registra el run: ya hay en curso %v", que, ids)
		}
		time.Sleep(20 * time.Millisecond)
	}
	soltado := time.Now()
	soltar()
	v, ok := l.recoger(10 * time.Second)
	if !ok {
		t.Fatalf("hallazgos b6e55b/ac2feb (CA-346, CA-344): suelto el candado, %s no volvio en 10 s", que)
	}
	if v.err != nil {
		t.Fatalf("hallazgos b6e55b/ac2feb (CA-346, CA-344): suelto el candado, %s registra el run: %v", que, v.err)
	}
	if v.cuando.Before(soltado) {
		t.Fatalf("hallazgos b6e55b/ac2feb (CA-346, CA-344): %s volvio (%s) antes de que se soltara el candado (%s)", que, v.cuando.Format(time.StampMicro), soltado.Format(time.StampMicro))
	}
	return v
}

// kcRegistrado exige que el run id quede registrado en curso: Busy lo ve y
// su sidecar dice running.
func kcRegistrado(t *testing.T, que string, m *Manager, root, id string) {
	t.Helper()
	if got, busy := m.Busy(root); !busy || got != id {
		t.Fatalf("hallazgos b6e55b/ac2feb (CA-346, CA-344): suelto el candado, el run de %s queda registrado: Busy dice %q, %v (quiero %s)", que, got, busy, id)
	}
	if meta, ok := ReadMeta(root, id); !ok || meta.Status != StatusRunning {
		t.Fatalf("hallazgos b6e55b/ac2feb (CA-346, CA-344): suelto el candado, el sidecar del run de %s dice running: %+v (existe: %v)", que, meta, ok)
	}
}

// Hallazgos 20261014T180914_b6e55b y 20261014T180925_ac2feb (CA-346, CA-344):
// con el candado de un arbol tomado (como lo toman Descartar y Guardar entre
// "no hay run activo" y su accion), Manager.Start sobre ese arbol no registra
// el run mientras siga tomado (no vuelve, no aparece en Busy ni hay sidecar
// running) y lo registra en cuanto se suelta: vuelve despues de soltarlo y el
// run queda en curso.
func TestHallazgo_b6e55b_ac2feb_CA346_CA344_StartEsperaElCandadoDelArbol(t *testing.T) {
	installFake(t, "echo arranco\nsleep 30\n")
	root := initRepo(t)
	m := NewManager(root)
	soltar := kcTomar(t, root)

	l := kcLanzar(t, m, soltar, "arranco", func() (Run, error) {
		return m.Start(StartOptions{Provider: "claude", Prompt: "trabajo largo"})
	})
	v := kcSinRegistrar(t, "Start", m, root, soltar, l)
	kcRegistrado(t, "Start", m, root, v.run.ID)
}

// Hallazgos 20261014T180914_b6e55b y 20261014T180925_ac2feb (CA-346, CA-344):
// lo mismo con Manager.Input, que tambien registra un run en el arbol: con el
// candado tomado, la continuacion de un run terminado no se registra hasta
// que se suelta.
func TestHallazgo_b6e55b_ac2feb_CA346_CA344_InputEsperaElCandadoDelArbol(t *testing.T) {
	ya := filepath.Join(t.TempDir(), "ya-corrio")
	// la primera invocacion termina enseguida; las siguientes trabajan largo
	installFake(t, "if [ -f '"+ya+"' ]; then echo sigo; sleep 30; else touch '"+ya+"'; echo listo; fi\n")
	root := initRepo(t)
	m := NewManager(root)
	primero, err := m.Start(StartOptions{Provider: "claude", Prompt: "primer turno"})
	if err != nil {
		t.Fatal(err)
	}
	if final := m.Wait(primero.ID); final.Status == StatusRunning {
		t.Fatalf("CA-346: fixture: el primer turno termina: %+v", final)
	}
	soltar := kcTomar(t, root)

	l := kcLanzar(t, m, soltar, "sigo", func() (Run, error) {
		return m.Input(primero.ID, "segundo turno")
	})
	v := kcSinRegistrar(t, "Input", m, root, soltar, l)
	kcRegistrado(t, "Input", m, root, v.run.ID)
}

// Hallazgos 20261014T180914_b6e55b y 20261014T180925_ac2feb (guarda): el
// candado es POR arbol. Con el de otro arbol del mismo repo tomado (un
// worktree), Start en la raiz no espera.
func TestHallazgo_b6e55b_ac2feb_GuardaOtroArbolNoFrenaStart(t *testing.T) {
	installFake(t, "echo arranco\nsleep 30\n")
	root := initRepo(t)
	otro := filepath.Join(t.TempDir(), "otro-arbol")
	git(t, root, "worktree", "add", "-q", "-b", "hoom/otro", otro)
	kcTomar(t, otro)
	m := NewManager(root)

	desde := time.Now()
	r, err := m.Start(StartOptions{Provider: "claude", Prompt: "trabajo largo"})
	tardo := time.Since(desde)
	if err != nil {
		t.Fatalf("hallazgos b6e55b/ac2feb: guarda: el candado de otro arbol no frena a Start en la raiz: %v", err)
	}
	t.Cleanup(func() {
		hasta := time.Now().Add(5 * time.Second)
		for time.Now().Before(hasta) {
			if _, evs, err := m.Events(r.ID, 0); err != nil || strings.Contains(allDetails(evs), "arranco") {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		m.Cancel(r.ID)
		m.Wait(r.ID)
	})
	if tardo >= time.Second {
		t.Fatalf("hallazgos b6e55b/ac2feb: guarda: Start en la raiz no espera el candado de otro arbol: tardo %v", tardo)
	}
	kcRegistrado(t, "Start con el candado de otro arbol tomado", m, root, r.ID)
}

// Hallazgos 20261014T180914_b6e55b y 20261014T180925_ac2feb (guarda):
// LockArbol sobre un directorio que no es un repo de git no falla (no hay
// con que correr una carrera) y no escribe nada adentro del arbol.
func TestHallazgo_b6e55b_ac2feb_GuardaCandadoFueraDeUnRepo(t *testing.T) {
	dir := t.TempDir()
	suelta, err := LockArbol(dir, time.Second)
	if err != nil {
		t.Fatalf("hallazgos b6e55b/ac2feb: guarda: LockArbol sobre un directorio que no es un repo no falla: %v", err)
	}
	if suelta == nil {
		t.Fatal("hallazgos b6e55b/ac2feb: guarda: LockArbol devuelve como soltarlo")
	}
	suelta()
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 0 {
		var nombres []string
		for _, e := range entradas {
			nombres = append(nombres, e.Name())
		}
		t.Fatalf("hallazgos b6e55b/ac2feb: guarda: el candado no escribe nada adentro del arbol: quedo %s", strings.Join(nombres, ", "))
	}
}
