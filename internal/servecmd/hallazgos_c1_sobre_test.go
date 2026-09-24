// Tests de regresion de la review cruzada de la cabina (C1, 2026-09-24),
// hallazgo 20260924T200412_151300 sobre `hoom serve`: el launch respondia
// 202 "el sobre esta corriendo" apoyado en un Started que se llamaba aunque
// el primer registro del sobre NO estuviera en disco (CA-335). Sin ese
// registro la tarjeta no tiene fantasma ni sobre que mostrar: el launch
// responde 409 JSON, nunca 202, y no arranca ningun run.
package servecmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// h3IDSobre es la forma de un id de sobre (la de un run): 20260924T200412_151300.
var h3IDSobre = regexp.MustCompile(`\d{8}T\d{6}_[0-9a-f]{6}`)

// h3Trampa vigila .hoom/envelopes/ y, en cuanto aparece algo con un id de
// sobre que no estaba (el log <id>.log que el Studio abre antes de lanzar,
// o un temporal del registro), planta un DIRECTORIO no vacio donde va
// <id>.json: el primer registro de ese sobre ya no se puede escribir. El
// Studio elige el id, asi que la trampa se arma mirando, no de antemano.
// parar devuelve el id visto y si la trampa llego antes que el registro.
func h3Trampa(dir string, previos map[string]bool) (parar func() (id string, gano bool)) {
	stop, done := make(chan struct{}), make(chan struct{})
	var id string
	var gano bool
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				runtime.Gosched()
				continue
			}
			for _, e := range entries {
				m := h3IDSobre.FindString(e.Name())
				if m == "" || previos[m] {
					continue
				}
				id = m
				p := filepath.Join(dir, m+".json")
				if os.Mkdir(p, 0o755) == nil {
					os.WriteFile(filepath.Join(p, "ocupado"), []byte("x\n"), 0o644)
					gano = true
				}
				return
			}
		}
	}()
	return func() (string, bool) {
		close(stop)
		<-done
		return id, gano
	}
}

// h3IDsEn son los ids de sobre que ya tienen algo en .hoom/envelopes/.
func h3IDsEn(dir string) map[string]bool {
	out := map[string]bool{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if m := h3IDSobre.FindString(e.Name()); m != "" {
			out[m] = true
		}
	}
	return out
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

// Hallazgo 20260924T200412_151300 (CA-335, CA-338): POST
// /api/board/{slug}/launch de pedir-writer cuando el primer registro del
// sobre no se puede escribir (un directorio en .hoom/envelopes/<id>.json)
// responde 409 JSON, nunca 202; el CLI de la IA no se invoca y no aparece
// ningun .hoom/runs/*.meta.json. Si la trampa llega tarde (el registro ya
// se escribio) el intento no prueba nada y se repite con otra tarjeta.
func TestHallazgo_151300_LaunchSinPrimerRegistroEs409(t *testing.T) {
	const ca = "CA-335"
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	envs := filepath.Join(root, ".hoom", envelope.DirName)
	if err := os.MkdirAll(envs, 0o755); err != nil {
		t.Fatal(err)
	}
	s := lnServidor(t, root, f)

	for intento := 0; intento < 8; intento++ {
		slug := fmt.Sprintf("sin-registro-%d", intento)
		wt := lnCartaWriter(t, ca, root, slug, "")
		llamadasAntes := len(f.llamadas(t, "claude"))
		metasAntes := h3Metas(t, root, wt)

		parar := h3Trampa(envs, h3IDsEn(envs))
		rec := lnLaunch(t, s, slug, s.Token(), lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec hasta que verify de verde."))
		id, gano := parar()
		if id == "" {
			t.Fatalf("%s: fixture: el launch no dejo nada con id de sobre en .hoom/envelopes/ (respuesta %d: %s)", ca, rec.Code, rec.Body.String())
		}
		if !gano {
			// el registro llego antes que la trampa: este intento no arma el caso
			t.Logf("%s: intento %d: el registro %s se escribio antes de la trampa; se repite", ca, intento, id)
			lnEsperarLanzamientos(t, ca, s)
			continue
		}

		if rec.Code == http.StatusAccepted {
			t.Fatalf("%s: hallazgo 151300: sin primer registro del sobre %s el launch nunca responde 202: %s", ca, id, rec.Body.String())
		}
		tbErrorJSON(t, ca, rec, http.StatusConflict)
		lnEsperarLanzamientos(t, ca, s)
		if n := len(f.llamadas(t, "claude")); n != llamadasAntes {
			t.Fatalf("%s: hallazgo 151300: sin primer registro no arranca ningun run: claude se invoco %d veces", ca, n-llamadasAntes)
		}
		if m := h3Metas(t, root, wt); len(m) != len(metasAntes) {
			t.Fatalf("%s: hallazgo 151300: sin primer registro no aparece ningun .hoom/runs/*.meta.json: antes %v, despues %v", ca, metasAntes, m)
		}
		for _, r := range envelope.List(root) {
			if r.ID == id {
				t.Fatalf("%s: no queda registro del sobre %s: %+v", ca, id, r)
			}
		}
		return
	}
	t.Fatalf("%s: fixture: en 8 intentos la trampa nunca llego antes que el primer registro", ca)
}
