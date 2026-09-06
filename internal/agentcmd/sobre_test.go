// Tests del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md
// (CA-201, CA-203, CA-204, CA-205): el sobre deja rastro. Lo que antes se
// imprimia y se iba ahora queda en .hoom/envelopes/, que es lo que status y el
// Studio pueden mirar MIENTRAS el sobre corre.
package agentcmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gitx"
)

func sobres(t *testing.T, root string) []envelope.Record {
	t.Helper()
	return envelope.List(root)
}

// CA-204: el sobre registra cada transicion y cierra el registro con la misma
// verdad que imprime; el registro existe aunque corte en el paso 1.
func TestCA204_ElSobreDejaRastro(t *testing.T) {
	// (a) corte en el paso 1: un spec sin aprobar no gasta un token, pero SI
	// deja registro — un sobre que no llego a correr nada es exactamente el
	// que hay que poder mirar despues
	root := repo(t)
	fakeProvider(t, "claude", "exit 0\n")
	spec := specDemo(t, root)
	res, err := Run(root, "main", Options{Role: "writer", Spec: spec, Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	recs := sobres(t, root)
	if len(recs) != 1 {
		t.Fatalf("CA-204: un sobre, un registro: %+v", recs)
	}
	rec := recs[0]
	if rec.ID != res.EnvelopeID || rec.ID == "" {
		t.Fatalf("CA-204: el Result nombra su registro: %q vs %q", res.EnvelopeID, rec.ID)
	}
	if rec.Stage != "spec" || rec.Status != envelope.StatusNotDeliverable || rec.ExitCode != 1 {
		t.Fatalf("CA-204: el registro cierra donde cerro el sobre: %+v", rec)
	}
	if rec.Role != "writer" || rec.Provider != "claude" || rec.Dir != root || rec.Steps != 5 {
		t.Fatalf("CA-204: el registro identifica el sobre: %+v", rec)
	}
	if rec.Spec == "" || rec.Approval == "" || rec.Note == "" {
		t.Fatalf("CA-204: spec, aprobacion y motivo viajan: %+v", rec)
	}
	if rec.RunID != "" {
		t.Fatalf("CA-204: no hubo run, no hay run_id: %+v", rec)
	}
	if rec.StartedAt.IsZero() || rec.EndedAt.IsZero() || rec.UpdatedAt.IsZero() {
		t.Fatalf("CA-204: cuando arranco, cuando se movio y cuando termino: %+v", rec)
	}

	// (b) camino completo: el registro pasa por los pasos y cierra entregable
	root2 := repo(t)
	fakeProvider(t, "claude", "exit 0\n")
	res2, err := Run(root2, "main", Options{Role: "writer", Prompt: "no toques nada"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	rec2 := sobres(t, root2)[0]
	if rec2.RunID == "" || rec2.RunID != res2.RunID {
		t.Fatalf("CA-204: el registro nombra el run: %+v", rec2)
	}
	if rec2.Stage != res2.Stage || rec2.Status != res2.Status || rec2.ExitCode != res2.ExitCode {
		t.Fatalf("CA-204: registro y Result dicen lo mismo: %+v vs %+v", rec2, res2)
	}
	if rec2.VerdictID == "" || rec2.Verdict != res2.Verdict {
		t.Fatalf("CA-204: el veredicto viaja en el registro: %+v", rec2)
	}
	if rec2.Step != rec2.Steps {
		t.Fatalf("CA-204: un sobre que llego al final esta en su ultimo paso: %+v", rec2)
	}
}

// CA-205: el latido mueve UpdatedAt mientras el run narra, sin escribir un
// archivo por evento.
func TestCA205_ElLatidoMueveElRegistro(t *testing.T) {
	root := repo(t)
	// el provider tarda: hay narracion durante mas de un latido
	fakeProvider(t, "claude", "printf 'hola\\n'\nsleep 6\nprintf 'chau\\n'\nexit 0\n")
	antes := time.Now().UTC()
	if _, err := Run(root, "main", Options{Role: "writer", Prompt: "trabaja"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	rec := sobres(t, root)[0]
	if !rec.UpdatedAt.After(antes) {
		t.Fatalf("CA-205: el registro se movio mientras el run hablaba: %+v", rec)
	}
	// un archivo por sobre, no uno por evento ni por latido
	entries, err := os.ReadDir(filepath.Join(root, ".hoom", envelope.DirName))
	if err != nil || len(entries) != 1 {
		t.Fatalf("CA-205: el latido reescribe UN archivo: %v %v", entries, err)
	}
}

// CA-203: el registro es telemetria, no evidencia: fuera de Git, fuera del
// candidato de cambio y fuera de la huella.
func TestCA203_ElRegistroNoEnsuciaLaEvidencia(t *testing.T) {
	root := repo(t)
	// un proyecto ya inicializado esconde su telemetria (hoom init la escribe);
	// agregar la regla la primera vez SI toca un archivo del repo, y por eso se
	// mide despues de que la regla existe, que es la vida normal del proyecto
	write(t, root, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\nenvelopes/\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "telemetria escondida")
	antes := gitx.Snapshot(root, "main")

	envelope.Write(root, envelope.Record{ID: "20260906T230000_aaa", Role: "writer",
		Provider: "claude", Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning})

	despues := gitx.Snapshot(root, "main")
	if antes.ChangeFingerprint != despues.ChangeFingerprint {
		t.Fatalf("CA-203: escribir un sobre no puede mover la huella: %s -> %s",
			antes.ChangeFingerprint, despues.ChangeFingerprint)
	}
	for _, f := range despues.ChangedFiles {
		if strings.HasPrefix(f, ".hoom/envelopes/") {
			t.Fatalf("CA-203: el registro entro en el candidato de cambio: %v", despues.ChangedFiles)
		}
	}
	gi, err := os.ReadFile(filepath.Join(root, ".hoom", ".gitignore"))
	if err != nil || !strings.Contains(string(gi), "envelopes/") {
		t.Fatalf("CA-203: .hoom/.gitignore debe esconder envelopes/: %q %v", gi, err)
	}
	// y tampoco se le cobra al rol: es narracion de hoom, no del agente
	if !hoomOwn(".hoom/envelopes/20260906T230000_aaa.json") {
		t.Fatal("CA-203: el registro es narracion de hoom, no escritura del rol")
	}
}

// CA-201: el sobre dice lo que costo el run, en el texto y en el JSON.
func TestCA201_ElSobreDiceElGasto(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", `printf '{"type":"system","subtype":"init","session_id":"s-1"}\n'`+"\n"+
		`printf '{"type":"result","subtype":"success","result":"listo","num_turns":2,"total_cost_usd":0.25,`+
		`"usage":{"input_tokens":10,"output_tokens":5}}\n'`+"\nexit 0\n")

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "writer", Prompt: "trabaja"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Usage == nil || res.Usage.CostUSD == nil || *res.Usage.CostUSD != 0.25 || res.Usage.Turns != 2 {
		t.Fatalf("CA-201: el Result trae el gasto del run: %+v", res.Usage)
	}
	if !strings.Contains(buf.String(), "gasto:") || !strings.Contains(buf.String(), "0.25") {
		t.Fatalf("CA-201: el paso run imprime el gasto:\n%s", buf.String())
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"cost_usd":0.25`) || !strings.Contains(string(raw), `"envelope_id"`) {
		t.Fatalf("CA-201: --json incluye usage y el id del registro: %s", raw)
	}
	if rec := sobres(t, root)[0]; rec.Usage == nil || rec.Usage.Turns != 2 {
		t.Fatalf("CA-204: y el registro tambien lo guarda: %+v", rec.Usage)
	}
}
