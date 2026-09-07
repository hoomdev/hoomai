// Tests de regresion de la review cruzada (Codex, 2026-09-06) sobre Spec D + Spec E.
package envelope

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// hrLectura es lo que un lector concurrente vio del sidecar.
type hrLectura struct {
	lecturas, vacias, parciales, ausentes int
	muestra                               string // primera lectura parcial, recortada
}

func (l *hrLectura) sumar(o hrLectura) {
	l.lecturas += o.lecturas
	l.vacias += o.vacias
	l.parciales += o.parciales
	l.ausentes += o.ausentes
	if l.muestra == "" {
		l.muestra = o.muestra
	}
}

// Hallazgos 20260906T235816_0293b4 y 20260907T000658_19274d: Write reescribe
// el unico sidecar del sobre con os.WriteFile, que TRUNCA antes de escribir
// (sin temporal, fsync ni rename). Un status o Studio concurrente puede leer
// JSON vacio o parcial, List lo omite y el sobre desaparece en vez de quedar
// visible; una caida a mitad del write destruye el ultimo estado durable.
//
// Propiedad: mientras una goroutine escribe muchas transiciones del mismo
// registro, dos lectores leen el archivo en bucle; toda lectura debe ser el
// JSON completo de ese registro (el archivo existe completo desde ANTES de
// que arranquen los lectores, asi que una lectura vacia o un archivo ausente
// tambien son roturas) y al final no queda ningun temporal al lado.
func TestHallazgo_0293b4_WriteEsAtomicoParaUnLectorConcurrente(t *testing.T) {
	root := t.TempDir()
	rec := Record{ID: "20260906T230000_a70a1c", Role: "writer", Provider: "claude", Stage: "spec",
		Step: 1, Steps: 5, Status: StatusRunning, ExitCode: -1, StartedAt: time.Now().UTC(),
		// una nota larga: el JSON pesa lo suficiente para que "vacio o parcial"
		// sea una ventana observable y no un instante
		Note: strings.Repeat("el sobre narra lo que hace mientras lo hace. ", 400)}
	Write(root, rec)
	path := filepath.Join(dir(root), rec.ID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("0293b4: el primer Write debe dejar el registro: %v", err)
	}

	const escrituras = 4000
	etapas := []string{"spec", "aislar", "run", "scope", "verify", "check", "ok"}
	fin := make(chan struct{})
	go func() {
		defer close(fin)
		for i := 0; i < escrituras; i++ {
			r := rec
			r.Stage, r.Step = etapas[i%len(etapas)], i%len(etapas)+1
			Write(root, r)
		}
	}()

	leer := func() hrLectura {
		var l hrLectura
		for {
			select {
			case <-fin:
				return l
			default:
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				l.ausentes++
				continue
			}
			l.lecturas++
			if len(raw) == 0 {
				l.vacias++
				continue
			}
			var got Record
			if json.Unmarshal(raw, &got) != nil || got.ID != rec.ID {
				l.parciales++
				if l.muestra == "" {
					l.muestra = string(raw[:min(len(raw), 80)])
				}
			}
		}
	}
	resultados := make([]hrLectura, 2)
	var wg sync.WaitGroup
	for i := range resultados {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resultados[i] = leer()
		}(i)
	}
	wg.Wait()

	var total hrLectura
	for _, r := range resultados {
		total.sumar(r)
	}
	if total.vacias+total.parciales+total.ausentes > 0 {
		t.Fatalf("0293b4: de %d lecturas concurrentes del sidecar, %d lo vieron vacio, %d parcial y %d ausente: Write trunca antes de escribir en vez de escribir a un temporal y renombrar (muestra parcial: %q)",
			total.lecturas, total.vacias, total.parciales, total.ausentes, total.muestra)
	}
	entries, err := os.ReadDir(dir(root))
	if err != nil {
		t.Fatal(err)
	}
	var nombres []string
	for _, e := range entries {
		nombres = append(nombres, e.Name())
	}
	if len(nombres) != 1 || nombres[0] != rec.ID+".json" {
		t.Fatalf("0293b4: al terminar solo puede quedar el sidecar, sin temporales: %v", nombres)
	}
	ultima := etapas[(escrituras-1)%len(etapas)]
	if got := List(root); len(got) != 1 || got[0].Stage != ultima {
		t.Fatalf("0293b4: el ultimo estado escrito es el que queda legible (esperaba stage %q): %+v", ultima, got)
	}
}
