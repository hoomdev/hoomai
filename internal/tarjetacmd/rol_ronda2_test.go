// Tests adversariales de la ronda 2 de la review de verbos-de-la-tarjeta en
// los verbos de la tarjeta: hallazgo a8e103 (CA-504, CA-511). hoom reanudar
// toma el rol de la interrupcion de la tarjeta; si, mientras la persona
// escribe el pedido en el editor, esa interrupcion pasa a ser la de otro rol
// (la del writer se cierra y queda la de un test-writer con el mismo
// provider y los mismos valores), lo que se mostro ya no es lo que se
// lanzaria: exit 1 con el motivo de faff7c, `rol` primero en la lista, y
// nada se lanza. Los fixtures son los de tarjetacmd_test.go y
// hallazgos_ronda1_test.go.
package tarjetacmd_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// r2JSONIndentado es v como lo escribe hoom en .hoom/ (JSON indentado).
func r2JSONIndentado(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw) + "\n"
}

// r2Propuesta es lo que la accion propone, como mapa JSON, sin las claves
// que se nombran (para comparar el resto).
func r2Propuesta(t *testing.T, a boardcmd.Action, sin ...string) map[string]any {
	t.Helper()
	raw, err := json.Marshal(lanzar.PropuestaDe(a))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range sin {
		delete(m, k)
	}
	return m
}

// hallazgo a8e103, CA-504 y CA-511: la interrupcion del writer se reemplaza
// por la de un test-writer mientras la persona escribe el pedido de hoom
// reanudar. Con la misma sesion (lo unico distinto es el rol), el motivo
// nombra solo rol; con otra sesion, rol y sesion. Nada se lanza.
func TestHallazgo_a8e103_CA511_ReanudarConLaInterrupcionReemplazada(t *testing.T) {
	for _, k := range []struct{ nombre, sesionTW, lista string }{
		{"el test-writer con la misma sesion", vdtSesion, "rol"},
		{"el test-writer con su propia sesion", "sesion-del-test-writer-789", "rol, sesion"},
	} {
		t.Run(k.nombre, func(t *testing.T) { r2ReanudarReemplazada(t, k.nombre, k.sesionTW, k.lista) })
	}
}

// r2ReanudarReemplazada: el editor de hoom reanudar cierra la interrupcion
// del writer y deja la de un test-writer con la sesion sesionTW; el motivo
// nombra lista.
func r2ReanudarReemplazada(t *testing.T, nombre, sesionTW, lista string) {
	ca := "hallazgo a8e103, CA-511 (" + nombre + ")"
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	wt := vdtInterrumpida(t, ca, root, "precios", true)
	antes := vdtAccion(t, ca, vdtCarta(t, root, "precios"), boardcmd.ActReanudar)
	if !antes.Enabled || antes.Role != "writer" || antes.ResumeID != vdtSesion {
		t.Fatalf("%s: fixture: reanudar habilitada, del writer, con su sesion: %+v", ca, antes)
	}

	// lo que el editor cambia mientras se escribe el pedido
	ahora := time.Now().UTC()
	muerto := vdtPIDMuerto(t)
	var viejo envelope.Record
	if err := json.Unmarshal([]byte(r1Leer(t, filepath.Join(root, ".hoom", "envelopes", "20260923T075959_env0ld.json"))), &viejo); err != nil {
		t.Fatal(err)
	}
	viejo.Status, viejo.ExitCode, viejo.UpdatedAt, viejo.EndedAt = envelope.StatusNotDeliverable, 1, ahora, ahora
	runTW, envTW := "20260923T090001_runtw1", "20260923T090000_envtw1"
	cambios := map[string]string{
		filepath.Join(root, ".hoom", "envelopes", viejo.ID+".json"): r2JSONIndentado(t, viejo),
		filepath.Join(root, ".hoom", "runs", runTW+".meta.json"): r2JSONIndentado(t, runcmd.Meta{ID: runTW, Provider: "claude",
			Role: "test-writer", Task: "precios", Dir: wt, CreatedAt: ahora.Add(-30 * time.Minute), Status: runcmd.StatusRunning,
			ExitCode: -1, PID: muerto, ProviderSessionID: sesionTW}),
		filepath.Join(root, ".hoom", "envelopes", envTW+".json"): r2JSONIndentado(t, envelope.Record{ID: envTW, Role: "test-writer",
			Provider: "claude", Task: "precios", Dir: wt, Spec: filepath.Join(wt, ".hoom", "specs", "precios.md"),
			Approval: "aprobado", RunID: runTW, Stage: "run", Step: 4, Steps: 6, Status: envelope.StatusRunning, ExitCode: -1,
			StartedAt: ahora.Add(-30 * time.Minute), UpdatedAt: ahora.Add(-20 * time.Minute), PID: muerto}),
	}
	ed := r1EditorQueCambia(t, "Segui desde el paso 3", cambios)
	s := vdtCorrer(t, ca, vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed}}, "reanudar", "precios")

	vdtExit(t, ca, s, 1)
	if !strings.Contains(s.err, r1Cambio(lista)) {
		t.Fatalf("%s: el motivo dice %q:\n%s", ca, r1Cambio(lista), s)
	}

	// fixture: la interrupcion ahora es la del test-writer y, salvo el rol
	// y la sesion, reanudar propone lo mismo que se mostro
	c := vdtCarta(t, root, "precios")
	if c.Interrupted == nil || c.Interrupted.EnvelopeID != envTW {
		t.Fatalf("%s: fixture: el editor dejo la interrupcion del test-writer (%s): %+v\n%s", ca, envTW, c.Interrupted, s)
	}
	despues := vdtAccion(t, ca, c, boardcmd.ActReanudar)
	if despues.Role != "test-writer" || despues.ResumeID != sesionTW {
		t.Fatalf("%s: fixture: reanudar ahora es del test-writer con la sesion %q: %+v", ca, sesionTW, despues)
	}
	if a, b := r2Propuesta(t, antes, "role", "resume_id"), r2Propuesta(t, despues, "role", "resume_id"); !reflect.DeepEqual(a, b) {
		t.Fatalf("%s: fixture: salvo rol y sesion, reanudar propone lo mismo:\n antes   %v\n despues %v", ca, a, b)
	}

	f.ninguna(t, ca, "reanudar con la interrupcion reemplazada")
	if n := len(vdtSobres(t, root)); n != 2 {
		t.Fatalf("%s: no se lanzo nada: solo los dos sobres interrumpidos, hay %d", ca, n)
	}
	if n := len(runcmd.Metas(root)); n != 2 {
		t.Fatalf("%s: no se lanzo nada: solo los dos runs interrumpidos, hay %d", ca, n)
	}
}
