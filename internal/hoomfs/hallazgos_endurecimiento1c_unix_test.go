//go:build !windows

// Tercera ronda de endurecimiento-1 en hoomfs (ver
// hallazgos_endurecimiento1c_test.go): el caso del FIFO de
// RecorrerDirectorio, que solo se arma en unix. API nueva: sobre 51d6c22 no
// compila.
package hoomfs

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// CA-442 (hallazgos c85840 y b36303): un FIFO con nombre de registro, entre
// dos archivos regulares, no cuelga RecorrerDirectorio: llega con su Err y
// sin contenido, y los regulares de los costados llegan enteros, en orden de
// nombre. Reloj propio de 10 s; el FIFO se suelta al final.
func TestHallazgo_c85840_b36303_CA442_RecorrerDirectorioNoSeCuelgaEnUnFIFO(t *testing.T) {
	const ca = "CA-442 (hallazgos c85840 y b36303, FIFO en RecorrerDirectorio)"
	root := t.TempDir()
	dir := filepath.Join(root, ".hoom", "findings")
	e1cEscribir(t, filepath.Join(dir, "a.json"), `{"a":1}`)
	carreratest.Fifo(t, filepath.Join(dir, "b.json"))
	e1cEscribir(t, filepath.Join(dir, "c.json"), `{"c":3}`)

	type res struct {
		r   e1cRecorrido
		err error
	}
	listo := make(chan res, 1)
	go func() {
		r := e1cRecorrido{contenidos: map[string]string{}, errs: map[string]error{}}
		err := RecorrerDirectorio(root, []string{".hoom", "findings"}, e1cJSON, 1<<20, func(e Entrada) {
			r.nombres = append(r.nombres, e.Nombre)
			r.contenidos[e.Nombre] = string(e.Contenido)
			r.errs[e.Nombre] = e.Err
		})
		listo <- res{r, err}
	}()
	select {
	case <-time.After(10 * time.Second):
		t.Fatalf("%s: RecorrerDirectorio no volvio en 10 s: se quedo abriendo el FIFO b.json", ca)
	case x := <-listo:
		if x.err != nil {
			t.Fatalf("%s: un FIFO llega con su Err, no hace fallar el recorrido: %v", ca, x.err)
		}
		if got := x.r.nombres; len(got) != 3 || got[0] != "a.json" || got[1] != "b.json" || got[2] != "c.json" {
			t.Errorf("%s: llegan a.json, b.json (el FIFO, con su Err) y c.json, en orden de nombre: %v", ca, got)
		}
		if x.r.errs["b.json"] == nil || x.r.contenidos["b.json"] != "" {
			t.Errorf("%s: el FIFO llega con su Err y sin contenido: %q, Err %v", ca, x.r.contenidos["b.json"], x.r.errs["b.json"])
		}
		if x.r.contenidos["a.json"] != `{"a":1}` || x.r.contenidos["c.json"] != `{"c":3}` {
			t.Errorf("%s: los regulares de los costados llegan enteros: %q %q", ca, x.r.contenidos["a.json"], x.r.contenidos["c.json"])
		}
	}
}
