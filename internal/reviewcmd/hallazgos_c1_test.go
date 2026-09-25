// Tests de regresion de la review cruzada de la cabina (C1, 2026-09-24)
// sobre `hoom review`:
//
//   - 20260924T200412_151300 (CA-335, CA-202): con Started (el Studio), si el
//     primer registro del sobre no se puede escribir, Started no se llama,
//     Run devuelve un error que lo dice y no arranca ninguna pasada. Sin
//     Started (la CLI) el registro sigue siendo best-effort.
//   - 20260924T195800_5d9276 (CA-293): una review que llega al final pero no
//     puede escribir su registro en .hoom/reviews/ no es "revisado": sin
//     registro no hay rastro de que se reviso.
//
// Providers falsos en el PATH: ningun CLI de IA real.
package reviewcmd

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
)

// h3Bloquear planta un DIRECTORIO (no vacio) donde va
// .hoom/envelopes/<id>.json del proyecto: el registro del sobre de la
// review no se puede escribir.
func h3Bloquear(t *testing.T, root, id string) {
	t.Helper()
	p := filepath.Join(root, ".hoom", envelope.DirName, id+".json")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "ocupado"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// h3Reviewer instala un claude falso que deja una marca FUERA del proyecto
// cada vez que lo invocan. Devuelve la ruta de la marca.
func h3Reviewer(t *testing.T) string {
	t.Helper()
	marca := filepath.Join(t.TempDir(), "claude-invocado")
	fakeProvider(t, "claude", "echo invocado >> '"+marca+"'\nexit 0\n")
	return marca
}

func h3Invocado(marca string) bool {
	_, err := os.Stat(marca)
	return err == nil
}

// h3Metas lista los sidecars .hoom/runs/*.meta.json de cada dir.
func h3Metas(t *testing.T, dirs ...string) []string {
	t.Helper()
	var out []string
	for _, d := range dirs {
		m, err := filepath.Glob(filepath.Join(d, ".hoom", "runs", "*.meta.json"))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m...)
	}
	return out
}

// h3Tarea arma el proyecto de la review con Started: la tarea precios con
// un cambio de codigo y un writer observado en codex, para que revise
// claude (cruzada) con UNA lente.
func h3Tarea(t *testing.T) (root, wt string, opt Options) {
	t.Helper()
	root = cbRepo(t)
	wt = cbTarea(t, root, "precios")
	cbMeta(t, root, wt, "20260924T190000_h3w001", "codex", "writer")
	return root, wt, Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "claude"}
}

// Hallazgo 20260924T200412_151300 (CA-335, CA-202): con Started no nil y
// EnvelopeID preasignado, si el primer registro del sobre de la review no se
// puede escribir, Started nunca se llama, Run devuelve un error que nombra el
// registro del sobre y no arranca ninguna pasada: el reviewer no se invoca,
// no aparece ningun .hoom/runs/*.meta.json nuevo y no hay registro de review.
func TestHallazgo_151300_ReviewConStartedNoArrancaSinPrimerRegistro(t *testing.T) {
	root, wt, opt := h3Tarea(t)
	marca := h3Reviewer(t)
	metasAntes := h3Metas(t, root, wt)
	s := &cbStarted{root: root, id: "20260924T200412_h3rv01"}
	h3Bloquear(t, root, s.id)
	opt.EnvelopeID, opt.Started = s.id, s.fn

	res, err := Run(root, "main", opt, io.Discard)
	if calls, _, _, _ := s.snapshot(); calls != 0 {
		t.Fatalf("CA-335: hallazgo 151300: el primer registro no esta en disco, asi que Started no se llama (llamadas: %d, res %+v)", calls, res)
	}
	if err == nil {
		t.Fatalf("CA-335: hallazgo 151300: sin primer registro la review con Started devuelve un error, no un Result: %+v", res)
	}
	if msg := strings.ToLower(err.Error()); !strings.Contains(msg, "registro") || !strings.Contains(msg, "sobre") {
		t.Fatalf("CA-335: hallazgo 151300: el error nombra el registro del sobre: %q", err)
	}
	if h3Invocado(marca) {
		t.Fatal("CA-335: hallazgo 151300: sin primer registro no arranca ninguna pasada: el reviewer se invoco")
	}
	if m := h3Metas(t, root, wt); len(m) != len(metasAntes) {
		t.Fatalf("CA-335: hallazgo 151300: sin primer registro no aparece ningun .hoom/runs/*.meta.json nuevo: antes %v, despues %v", metasAntes, m)
	}
	if got := rcRegistros(t, wt); len(got) != 0 {
		t.Fatalf("CA-293: una review que no corrio no deja registro de review: %v", got)
	}
	if recs := envelope.List(root); len(recs) != 0 {
		t.Fatalf("CA-335: no queda registro de sobre: %+v", recs)
	}
}

// Hallazgo 20260924T200412_151300 (CA-202), GUARDA: con Started nil (la
// CLI) nada cambia aunque el registro del sobre no se pueda escribir: la
// review corre su pasada y termina revisado, como siempre.
func TestHallazgo_151300_ReviewSinStartedSigueBestEffort(t *testing.T) {
	root, wt, opt := h3Tarea(t)
	marca := h3Reviewer(t)
	metasAntes := h3Metas(t, root, wt)
	opt.EnvelopeID = "20260924T200412_h3rv02"
	h3Bloquear(t, root, opt.EnvelopeID)

	res, err := Run(root, "main", opt, io.Discard)
	if err != nil {
		t.Fatalf("CA-202: sin Started el registro del sobre es best-effort: la review no falla por no poder escribirlo: %v", err)
	}
	if !h3Invocado(marca) || len(h3Metas(t, root, wt)) <= len(metasAntes) {
		t.Fatalf("CA-202: sin Started la pasada corre y deja su sidecar: %+v", res)
	}
	if res.Status != "revisado" || res.ExitCode != 0 || res.RecordID == "" {
		t.Fatalf("CA-202: la review termina como siempre (revisado, exit 0, con registro de review): %+v", res)
	}
}

// Hallazgo 20260924T195800_5d9276 (CA-293, CA-334): una review que llega al
// final pero no puede escribir su registro en .hoom/reviews/ (directorio de
// solo lectura) termina no-entregable con exit 1 y sin record_id, y la
// salida dice que no pudo escribir el registro. Hoy termina revisado con
// exit 0: "revisado" sin el rastro que CA-293 promete.
func TestHallazgo_5d9276_ReviewSinRegistroDeReviewEsNoEntregable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("CA-293: corriendo como root un directorio 0555 no frena la escritura")
	}
	root := repo(t)
	write(t, root, ".hoom/specs/precios.md", "# Spec: precios\n")
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")
	metaDeRun(t, root, "20260924T190000_h3w002", "claude", "writer", time.Now())
	reviews := filepath.Join(root, ".hoom", RecordsDir)
	if err := os.MkdirAll(reviews, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(reviews, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(reviews, 0o755) })

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Spec: ".hoom/specs/precios.md"}, &out)
	if err != nil {
		t.Fatalf("CA-293: no poder escribir el registro es un resultado de la review, no un armado roto: %v\n%s", err, out.String())
	}
	if len(res.Passes) != 1 || res.Cross != CrossYes {
		t.Fatalf("CA-293: fixture: la review llego al final con su pasada cruzada: %+v\n%s", res, out.String())
	}
	if res.Status != "no-entregable" || res.ExitCode != 1 {
		t.Fatalf("CA-293: hallazgo 5d9276: sin registro en .hoom/reviews/ la review es no-entregable con exit 1, no %q/%d\n%s",
			res.Status, res.ExitCode, out.String())
	}
	if res.RecordID != "" {
		t.Fatalf("CA-293: hallazgo 5d9276: sin registro escrito no hay record_id: %q", res.RecordID)
	}
	if raw, _ := json.Marshal(res); strings.Contains(string(raw), "record_id") {
		t.Fatalf("CA-293: sin registro, record_id se omite del JSON: %s", raw)
	}
	if !strings.Contains(strings.ToLower(out.String()), "escribir el registro") {
		t.Fatalf("CA-293: hallazgo 5d9276: la salida dice que no pudo escribir el registro:\n%s", out.String())
	}
	if got := rcRegistros(t, root); len(got) != 0 {
		t.Fatalf("CA-293: nada quedo en .hoom/reviews/: %v", got)
	}
	// el sobre de la review dice lo mismo que su Result (CA-334: cierra
	// no-entregable cuando la review lo es)
	for _, r := range envelope.List(root) {
		if r.Role == "reviewer" && r.Status != envelope.StatusNotDeliverable {
			t.Fatalf("CA-334: hallazgo 5d9276: el registro del sobre de una review no-entregable cierra no-entregable: %+v", r)
		}
	}
}

// Hallazgo 20260924T195800_5d9276 (CA-293), GUARDA: con .hoom/reviews/
// escribible la misma review sigue terminando revisado, con registro y
// record_id.
func TestHallazgo_5d9276_ConReviewsEscribibleSigueRevisado(t *testing.T) {
	root := repo(t)
	write(t, root, ".hoom/specs/precios.md", "# Spec: precios\n")
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")
	metaDeRun(t, root, "20260924T190000_h3w003", "claude", "writer", time.Now())
	if err := os.MkdirAll(filepath.Join(root, ".hoom", RecordsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Spec: ".hoom/specs/precios.md"}, &out)
	if err != nil {
		t.Fatalf("CA-293: %v\n%s", err, out.String())
	}
	if res.Status != "revisado" || res.ExitCode != 0 || res.RecordID == "" {
		t.Fatalf("CA-293: con .hoom/reviews/ escribible la review termina revisado con record_id: %+v\n%s", res, out.String())
	}
	if got := rcRegistros(t, root); len(got) != 1 || got[0] != res.RecordID+".json" {
		t.Fatalf("CA-293: un registro, el del record_id: %v", got)
	}
}
