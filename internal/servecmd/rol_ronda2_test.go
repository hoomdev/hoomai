// Tests adversariales de la ronda 2 de la review de verbos-de-la-tarjeta en
// el Studio: hallazgo a8e103 (CA-504). Lo que el dialogo mostro incluye el
// rol que la accion lanza: POST /launch con una vista de otro rol (o sin
// rol) es el 409 de faff7c con `rol` primero en la lista, sin efectos. El
// caso real: relanzar toma su rol de la interrupcion de la tarjeta, y si
// mientras se escribe el pedido la interrupcion pasa a ser la de un
// test-writer con el mismo provider y los mismos valores, solo el rol lo
// distingue (relanzar no tiene sesion). El dialogo manda el rol en la vista
// (vistaDe de acciones.js). Los CLIs son los falsos de lanzar_test.go, en un
// PATH minimo.
package servecmd

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// hallazgo a8e103, CA-504: POST /launch con una vista igual a lo que la
// accion propone salvo el rol (otro rol, o sin la clave role) es un 409 que
// nombra `rol`, primero y antes del orden de faff7c; nada se lanza.
func TestHallazgo_a8e103_CA504_LaunchConVistaDeOtroRolEs409(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	f.cli(t, "codex", false)
	root := raProyectoConReview(t, "agents:\n  writer:\n    model: m-yaml\n    effort: high\n    timeout: 45m\n")
	lnCartaWriter(t, "hallazgo a8e103", root, "precios", "presupuesto_usd: 5\n")
	a, ok := lnAccion(lnCarta(t, root, "precios"), boardcmd.ActPedirWriter)
	if !ok || !a.Enabled || a.Role != "writer" {
		t.Fatalf("hallazgo a8e103: fixture: pedir-writer habilitada, del writer: %+v", a)
	}
	base := r1Vista(a)
	s := lnServidor(t, root, f)
	casos := []struct {
		nombre string
		cambia func(v map[string]any)
		lista  string
	}{
		{"sin la clave role (las seis de la ronda 1)", func(v map[string]any) { delete(v, "role") }, "rol"},
		{"otro rol", func(v map[string]any) { v["role"] = "test-writer" }, "rol"},
		{"role vacio", func(v map[string]any) { v["role"] = "" }, "rol"},
		{"rol y esfuerzo", func(v map[string]any) { v["role"] = "reviewer"; v["effort"] = "low" }, "rol, esfuerzo"},
		{"los siete", func(v map[string]any) {
			v["timeout_s"] = 3600
			v["resume_id"] = "otra-sesion"
			v["budget_usd"] = 4.5
			v["effort"] = "low"
			v["model"] = "m-otro"
			v["provider"] = "codex"
			v["role"] = "arquitecto"
		}, "rol, provider, modelo, esfuerzo, presupuesto, tiempo limite, sesion"},
	}
	for _, c := range casos {
		v := r1Copia(base)
		c.cambia(v)
		ca := "hallazgo a8e103, CA-504 (" + c.nombre + ")"
		antes := lnHuellas(t, root)
		rec := lnLaunch(t, s, "precios", s.Token(), r1ConVista(lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec."), v))
		lnEsperarLanzamientos(t, ca, s)
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s: una vista de otro rol es un 409, fue %d: %s", ca, rec.Code, rec.Body.String())
		}
		if got, want := vdtMotivoStudio(t, ca, rec), r1Cambio(c.lista); got != want {
			t.Fatalf("%s: el motivo es\n  %q\nfue\n  %q", ca, want, got)
		}
		lnSinEfectos(t, ca, "una vista de otro rol", antes, lnHuellas(t, root), false)
	}
	lnSinLlamadas(t, "hallazgo a8e103", "una vista de otro rol", f)
}

// r2Registro reescribe el registro de un sobre como lo dejaria otro proceso.
func r2Registro(t *testing.T, root string, rec envelope.Record) {
	t.Helper()
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tbEscribir(t, root, ".hoom/envelopes/"+rec.ID+".json", string(raw)+"\n")
}

// r2SinRol es la vista sin la clave role.
func r2SinRol(v map[string]any) map[string]any {
	out := r1Copia(v)
	delete(out, "role")
	return out
}

// hallazgo a8e103, CA-504: la carrera real de relanzar. La persona abre el
// dialogo de Volver a lanzar sobre la interrupcion del writer; mientras
// escribe el pedido, esa interrupcion se cierra y aparece la de un
// test-writer con el mismo provider y los mismos valores. Lo unico que
// cambio es el rol (relanzar no tiene sesion que los distinga): el POST con
// lo que se mostro es un 409 (rol) y no lanza al test-writer.
func TestHallazgo_a8e103_CA504_RelanzarConLaInterrupcionReemplazada(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	wt, viejo, crudo := lnCartaInterrumpida(t, "hallazgo a8e103", root, "cortada")
	a, ok := lnAccion(lnCarta(t, root, "cortada"), boardcmd.ActRelanzar)
	if !ok || !a.Enabled || a.Role != "writer" {
		t.Fatalf("hallazgo a8e103: fixture: relanzar habilitada, con el rol del writer interrumpido: %+v", a)
	}
	vista := r1Vista(a)
	s := lnServidor(t, root, f)

	// la interrupcion del writer se cierra y queda la de un test-writer
	var rec envelope.Record
	if err := json.Unmarshal(crudo, &rec); err != nil {
		t.Fatal(err)
	}
	ahora := time.Now().UTC()
	rec.Status, rec.ExitCode, rec.UpdatedAt, rec.EndedAt = envelope.StatusNotDeliverable, 1, ahora, ahora
	r2Registro(t, root, rec)
	muerto := lnPIDMuerto(t)
	runTW, envTW := "20260923T090001_runtw1", "20260923T090000_envtw1"
	lnSidecar(t, root, runcmd.Meta{ID: runTW, Provider: "claude", Role: "test-writer", Task: "cortada", Dir: wt,
		CreatedAt: ahora.Add(-30 * time.Minute), Status: runcmd.StatusRunning, ExitCode: -1, PID: muerto})
	r2Registro(t, root, envelope.Record{ID: envTW, Role: "test-writer", Provider: "claude", Task: "cortada", Dir: wt,
		Spec: filepath.Join(wt, ".hoom", "specs", "cortada.md"), Approval: "aprobado", RunID: runTW,
		Stage: "run", Step: 4, Steps: 6, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: ahora.Add(-30 * time.Minute), UpdatedAt: ahora.Add(-20 * time.Minute), PID: muerto})
	c := lnCarta(t, root, "cortada")
	if c.Interrupted == nil || c.Interrupted.EnvelopeID != envTW {
		t.Fatalf("hallazgo a8e103: fixture: la interrupcion ahora es la del test-writer (%s), no %s: %+v", envTW, viejo, c.Interrupted)
	}
	a2, ok := lnAccion(c, boardcmd.ActRelanzar)
	if !ok || !a2.Enabled || a2.Role != "test-writer" {
		t.Fatalf("hallazgo a8e103: fixture: relanzar ahora lanza al test-writer, habilitada: %+v", a2)
	}
	if ahoraV := r1Vista(a2); !reflect.DeepEqual(r2SinRol(ahoraV), r2SinRol(vista)) {
		t.Fatalf("hallazgo a8e103: fixture: salvo el rol, relanzar propone lo mismo que se mostro:\n mostrado %v\n ahora    %v", vista, ahoraV)
	}

	// primero la vista sin role (lo que mandaba el dialogo de la ronda 1, que
	// no distinguia las dos interrupciones), despues la del dialogo de hoy
	for _, k := range []struct {
		nombre string
		vista  map[string]any
	}{
		{"la vista de seis claves, sin role", r2SinRol(vista)},
		{"la vista con el rol que se mostro (writer)", vista},
	} {
		ca := "hallazgo a8e103, CA-504 (relanzar, " + k.nombre + ")"
		antes := lnHuellas(t, root)
		resp := lnLaunch(t, s, "cortada", s.Token(), r1ConVista(lnCuerpo("relanzar", "claude", "", nil, "Implementa el spec desde el arbol como quedo."), r1Copia(k.vista)))
		lnEsperarLanzamientos(t, ca, s)
		if resp.Code != http.StatusConflict {
			t.Fatalf("%s: la interrupcion cambio de rol mientras se escribia el pedido: 409, fue %d: %s", ca, resp.Code, resp.Body.String())
		}
		if got := vdtMotivoStudio(t, ca, resp); got != r1Cambio("rol") {
			t.Fatalf("%s: el motivo es %q: %q", ca, r1Cambio("rol"), got)
		}
		lnSinEfectos(t, ca, "relanzar con otra interrupcion", antes, lnHuellas(t, root), false)
		lnSinLlamadas(t, ca, "relanzar con otra interrupcion", f)
	}
}

// hallazgo a8e103: el POST del dialogo de rol manda el rol en la vista:
// vistaDe de acciones.js arma la clave role (comprobacion sobre el fuente,
// como faff7c).
func TestHallazgo_a8e103_AccionesJSMandaElRol(t *testing.T) {
	_, _, acc := atUI(t)
	js := c2SinComentarios(acc)
	cuerpo := h2Cuerpo(js, "vistaDe")
	if cuerpo == "" {
		t.Fatal("hallazgo a8e103: acciones.js declara la funcion vistaDe")
	}
	if !regexp.MustCompile(`\brole\s*:`).MatchString(cuerpo) {
		t.Fatalf("hallazgo a8e103: vistaDe arma la clave role (role: ...):\n%s", cuerpo)
	}
	if !strings.Contains(js, "vistaDe(") {
		t.Fatal("hallazgo a8e103: acciones.js usa vistaDe")
	}
}
