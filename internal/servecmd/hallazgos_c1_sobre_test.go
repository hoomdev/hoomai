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
	"sync"
	"testing"
	"time"

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
//
// Hallazgo 20261005T155443_0fcace: lo que parar informa no puede depender de
// si al que mira le toco un turno mientras el launch corria.
//   - parar mira el directorio UNA VEZ MAS antes de volver, asi que un id de
//     sobre que el launch dejo en disco se informa siempre. Esa ultima mirada
//     solo informa, no planta: el launch ya respondio y una trampa puesta
//     despues no le cambia nada; el intento se repite (gano false), igual que
//     cuando el registro llega antes.
//   - miran h3Miradores a la vez. La ventana entre el log y el registro es de
//     una fraccion de milisegundo, y con la maquina cargada (16 procesos del
//     test a la vez, GOMAXPROCS=2) uno solo la perdia en 4 de cada 10
//     intentos: los 8 intentos seguidos, en 1 corrida de 384. El primero mira
//     sin parar, como antes, y los demas cada h3Pausa (bajo carga llegan mas
//     seguido que el que nunca suelta la CPU): entre los seis pierden
//     alrededor de 1 intento de cada 20, y nunca mas de 2 seguidos en 768
//     corridas.
func h3Trampa(dir string, previos map[string]bool) (parar func() (id string, gano bool)) {
	var (
		mu   sync.Mutex // id y gano, entre los que miran
		id   string
		gano bool
		wg   sync.WaitGroup
	)
	stop := make(chan struct{})
	// mirar lee el directorio una vez y dice si hay algo con un id de sobre
	// que no estaba; si lo hay lo anota y, con plantar, intenta la trampa.
	mirar := func(plantar bool) bool {
		entries, err := os.ReadDir(dir)
		if err != nil {
			runtime.Gosched()
			return false
		}
		for _, e := range entries {
			m := h3IDSobre.FindString(e.Name())
			if m == "" || previos[m] {
				continue
			}
			mu.Lock()
			defer mu.Unlock()
			id = m
			p := filepath.Join(dir, m+".json")
			if plantar && os.Mkdir(p, 0o755) == nil {
				os.WriteFile(filepath.Join(p, "ocupado"), []byte("x\n"), 0o644)
				gano = true
			}
			return true
		}
		return false
	}
	for i := 0; i < h3Miradores; i++ {
		pausa := h3Pausa
		if i == 0 {
			pausa = 0 // el primero mira sin parar
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !mirar(true) {
				select {
				case <-stop:
					return
				default:
				}
				if pausa > 0 {
					time.Sleep(pausa)
				}
			}
		}()
	}
	return func() (string, bool) {
		close(stop)
		wg.Wait() // desde aca nadie mas toca id ni gano
		if id == "" {
			mirar(false) // la ultima mirada: solo informa
		}
		return id, gano
	}
}

// h3Miradores son las goroutines de h3Trampa que miran a la vez, y h3Pausa
// lo que espera cada una entre dos miradas (menos la primera, que no espera).
const (
	h3Miradores = 6
	h3Pausa     = 50 * time.Microsecond
)

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
