// Tests adversariales del spec .hoom/specs/verify-args-estrictos.md (CA-223):
// el rechazo del gate desconocido nace en verifycmd.Run, asi que la puerta del
// Studio hereda la MISMA validacion. Una sola validacion para las dos puertas:
// si el 400 fuera un chequeo propio del Studio, el mensaje no coincidiria con
// el del CLI y este test lo delata.
package servecmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/live"
)

func sinArtefactos(t *testing.T, ca, dir string) {
	t.Helper()
	if entradas, err := os.ReadDir(filepath.Join(dir, ".hoom", "verdicts")); err == nil && len(entradas) > 0 {
		t.Fatalf("%s: un pedido rechazado no escribe veredicto, hay %d", ca, len(entradas))
	}
	if _, err := os.Stat(filepath.Join(dir, ".hoom", "cache", live.FileName)); err == nil {
		t.Fatalf("%s: un pedido rechazado no emite eventos vivos", ca)
	}
}

// CA-223: POST /api/verify con un gate desconocido responde 400 con la razon y
// no escribe veredicto ni eventos. El cuerpo trae el nombre desconocido Y la
// lista de gates del proyecto: es el mensaje del CLI, no uno inventado aca.
func TestCA223_StudioRechazaGateDesconocido(t *testing.T) {
	dir := newProject(t) // declara un solo gate: test
	s := newServer(t, dir)

	casos := []struct {
		cuerpo string
		nombra string
	}{
		{`{"gates":["noexiste"]}`, "noexiste"},
		{`{"gates":["spec_lint"]}`, "spec_lint"}, // sintetico: tampoco por el Studio
		{`{"gates":["ratchet"],"full":true}`, "ratchet"},
		{`{"gates":["test","noexiste"]}`, "noexiste"}, // un nombre malo envenena
		{`{"gates":[""]}`, ""},                        // nombre vacio
		{`{"gates":[","]}`, ","},                      // la coma no es un gate
		{`{"gates":["TEST"]}`, "TEST"},                // exacto, no case-insensitive
	}
	for _, c := range casos {
		rec := doPOST(t, s, "/api/verify", s.Token(), []byte(c.cuerpo), "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("CA-223: %s esperaba 400, obtuve %d: %s", c.cuerpo, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if c.nombra != "" && !strings.Contains(body, c.nombra) {
			t.Fatalf("CA-223: el cuerpo debe nombrar el gate desconocido %q: %s", c.nombra, body)
		}
		if !strings.Contains(body, "gate desconocido") {
			t.Fatalf("CA-223: el cuerpo debe traer la razon del CLI: %s", body)
		}
		if !strings.Contains(body, "test") {
			t.Fatalf("CA-223: el cuerpo debe listar los gates del proyecto (misma validacion que el CLI): %s", body)
		}
		sinArtefactos(t, "CA-223", dir)
	}
}

// CA-223: el 400 no es un portazo generico. Con gates validos —o sin gates— el
// Studio sigue corriendo verify y persistiendo su veredicto.
func TestCA223_StudioSigueAceptandoLoValido(t *testing.T) {
	dir := newProject(t)
	s := newServer(t, dir)

	rec := doPOST(t, s, "/api/verify", s.Token(), []byte(`{"gates":["test"]}`), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-223: 'test' esta declarado; esperaba 200, obtuve %d: %s", rec.Code, rec.Body.String())
	}
	var v struct {
		Partial bool `json:"partial"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if !v.Partial {
		t.Fatalf("CA-223: una seleccion de gates sigue dando veredicto PARCIAL: %s", rec.Body.String())
	}
	if rec := doPOST(t, s, "/api/verify", s.Token(), []byte(`{}`), "application/json"); rec.Code != http.StatusOK {
		t.Fatalf("CA-223: sin gates la corrida completa sigue viva: %d %s", rec.Code, rec.Body.String())
	}
	entradas, err := os.ReadDir(filepath.Join(dir, ".hoom", "verdicts"))
	if err != nil || len(entradas) != 2 {
		t.Fatalf("CA-223: las dos corridas validas persisten su veredicto (err=%v, n=%d)", err, len(entradas))
	}
}
