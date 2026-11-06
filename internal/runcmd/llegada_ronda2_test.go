// Tests adversariales de la ronda 2 de la review de verbos-de-la-tarjeta en
// el run: hallazgo d4e3bf (CA-495). Al vencer el tiempo limite, la eleccion
// entre "seguia narrando" y "sin narracion" lee cuando LLEGO por ultima vez
// la salida del provider, nunca cuando se cerro una linea. Lo ultimo que
// escribe un provider sin salto de linea (una linea gigante de mas de
// MaxLinea, una corta, o el pedido de permiso `Permitir escribir? [s/N] `
// por stderr) solo se entrega cuando se cierra la tuberia, despues del
// tiempo limite; aun asi lleva la hora en que llegaron sus bytes: el
// detalle no dice "ultimo evento hace 0s". Los textos son los de CA-495
// (verbos_de_la_tarjeta_test.go) y los CLIs son falsos.
package runcmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// r2GiganteSinSalto escribe un archivo con una linea de mas de MaxLinea
// bytes (que no es JSON) SIN salto de linea al final y devuelve su ruta.
func r2GiganteSinSalto(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gigante-sin-salto.txt")
	if err := os.WriteFile(p, []byte("r2-gigante "+strings.Repeat("g", MaxLinea+32<<10)), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// r2Narrando3s es el detalle "seguia narrando" del writer con un tiempo
// limite de 3 s; el grupo 1 son los segundos desde el ultimo evento.
var r2Narrando3s = regexp.MustCompile(`^timeout tras 3s: la CLI seguia narrando \(ultimo evento hace ([0-9]+)s\), no esperaba un permiso\. ` +
	`Accion: subi el tiempo limite \(--timeout o agents\.writer\.timeout en hoom\.yaml\) o pedi menos por run$`)

// hallazgo d4e3bf, CA-495: un provider que escribe enseguida un fragmento
// sin salto de linea y despues calla hasta pasar su tiempo limite (3 s)
// sigue dentro de la ventana de 60 s, pero su ultimo evento fue hace al
// menos 1 s, no 0 s: la hora es la de la llegada de sus bytes, no la del
// cierre de la tuberia. Los cuatro casos corren a la vez, cada uno en su
// proyecto: el CLI falso corre el r2caso.sh de su carpeta (el run corre en la
// raiz del proyecto, CA-22).
func TestHallazgo_d4e3bf_CA495_LoQueLlegoSinSaltoLlevaSuHora(t *testing.T) {
	gigante := r2GiganteSinSalto(t)
	installFake(t, "exec /bin/sh ./r2caso.sh\n")
	casos := []struct{ nombre, script, rastro string }{
		{"stdout, una linea gigante sin salto", "cat '" + gigante + "'\nexec sleep 60\n", ""},
		{"stderr, una linea gigante sin salto", "cat '" + gigante + "' >&2\nexec sleep 60\n", ""},
		{"stdout, una linea corta sin salto", "printf 'pensando en voz alta'\nexec sleep 60\n", "pensando en voz alta"},
		{"stderr, el pedido de permiso sin salto", "printf 'Permitir escribir? [s/N] ' >&2\nexec sleep 60\n", "Permitir escribir? [s/N]"},
	}
	type res struct {
		fin Run
		evs []Event
		err error
	}
	out := make([]res, len(casos))
	var wg sync.WaitGroup
	for i, c := range casos {
		root := vdtRaiz(t)
		if err := os.WriteFile(filepath.Join(root, "r2caso.sh"), []byte(c.script), 0o644); err != nil {
			t.Fatal(err)
		}
		m := NewManager(root)
		info, err := m.Start(StartOptions{Provider: "claude", Prompt: "trabaja", Role: "writer", Timeout: 3 * time.Second})
		if err != nil {
			t.Fatalf("hallazgo d4e3bf (%s): fixture: el run arranca: %v", c.nombre, err)
		}
		wg.Add(1)
		go func(i int, m *Manager, id string) {
			defer wg.Done()
			listo := make(chan Run, 1)
			go func() { listo <- m.Wait(id) }()
			select {
			case out[i].fin = <-listo:
			case <-time.After(40 * time.Second):
				m.Cancel(id)
				out[i].fin = <-listo
			}
			_, out[i].evs, out[i].err = m.Events(id, 0)
		}(i, m, info.ID)
	}
	wg.Wait()
	for i, c := range casos {
		r := out[i]
		t.Run(c.nombre, func(t *testing.T) {
			ca := "hallazgo d4e3bf, CA-495 (" + c.nombre + ")"
			if r.err != nil {
				t.Fatalf("%s: Events del run: %v", ca, r.err)
			}
			todo := allDetails(r.evs)
			if c.rastro != "" && !strings.Contains(todo, c.rastro) {
				t.Fatalf("%s: fixture: el fragmento sin salto llega a la narracion:\n%.2000s", ca, todo)
			}
			d := vdtDetalleTimeout(r.evs)
			sub := r2Narrando3s.FindStringSubmatch(d)
			if r.fin.Status != StatusError || sub == nil {
				t.Fatalf("%s: un provider que escribio al empezar y callo 3 s vence con\n  %s\nfue\n  %q\n%.2000s", ca,
					"timeout tras 3s: la CLI seguia narrando (ultimo evento hace <s>s), no esperaba un permiso. Accion: subi el tiempo limite (--timeout o agents.writer.timeout en hoom.yaml) o pedi menos por run",
					d, todo)
			}
			if n, _ := strconv.Atoi(sub[1]); n < 1 {
				t.Fatalf("%s: el fragmento llego al empezar y el run vencio a los 3 s: su ultimo evento fue hace al menos 1 s, no %d s (la hora es la de la llegada, no la del cierre):\n  %q", ca, n, d)
			}
		})
	}
}

// r2Mudo es el detalle "sin narracion"; el grupo 2 son los minutos y el 3
// los segundos desde la ultima narracion.
var r2Mudo = regexp.MustCompile(`^timeout tras [0-9hms]+ sin narracion desde hace (([0-9]+)m)?([0-9]+)s: ` +
	`la CLI puede estar esperando un permiso interactivo\. Accion: preconfigura sus permisos headless$`)

// hallazgo d4e3bf, CA-495, el caso del contrato entero: el provider pide
// permiso por stderr sin salto de linea (`printf 'Permitir escribir? [s/N] '
// >&2`) y espera; con un tiempo limite de 65 s, lo unico que escribio llego
// hace mas de 60 s, asi que el detalle es "sin narracion" (preconfigura sus
// permisos), no "seguia narrando (ultimo evento hace 0s)".
//
// Tarda unos 66 s. Para no sumarlos a la serie del paquete, el run arranca
// con el PATH del CLI falso puesto con os.Setenv (t.Setenv impide
// t.Parallel), el PATH vuelve en cuanto el CLI falso esta corriendo, y el
// test sigue con t.Parallel: espera el tiempo limite mientras corre el
// resto del paquete. Con -short no corre.
func TestHallazgo_d4e3bf_CA495_PermisoSinSaltoYSilencioEsSinNarracion(t *testing.T) {
	if testing.Short() {
		t.Skip("hallazgo d4e3bf: tarda unos 66 s (la ventana de CA-495 es de 60 s)")
	}
	bin := t.TempDir()
	marca := filepath.Join(t.TempDir(), "arranco")
	falso := "#!/bin/sh\n: > '" + marca + "'\nprintf 'Permitir escribir? [s/N] ' >&2\nexec sleep 600\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(falso), 0o755); err != nil {
		t.Fatal(err)
	}
	root := vdtRaiz(t)
	m := NewManager(root)

	viejo, habia := os.LookupEnv("PATH")
	devolver := func() {
		if habia {
			os.Setenv("PATH", viejo)
		} else {
			os.Unsetenv("PATH")
		}
	}
	os.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "trabaja", Role: "writer", Timeout: 65 * time.Second})
	arranco := false
	if err == nil {
		for limite := time.Now().Add(20 * time.Second); time.Now().Before(limite); time.Sleep(20 * time.Millisecond) {
			if _, e := os.Stat(marca); e == nil {
				arranco = true
				break
			}
		}
	}
	devolver()
	if err != nil {
		t.Fatalf("hallazgo d4e3bf: fixture: el run arranca: %v", err)
	}
	if !arranco {
		m.Cancel(info.ID)
		t.Fatal("hallazgo d4e3bf: fixture: el CLI falso no arranco en 20 s")
	}

	t.Parallel()

	listo := make(chan Run, 1)
	go func() { listo <- m.Wait(info.ID) }()
	var fin Run
	select {
	case fin = <-listo:
	case <-time.After(3 * time.Minute):
		m.Cancel(info.ID)
		t.Fatal("hallazgo d4e3bf: el run no cerro en 3 minutos")
	}
	_, evs, err := m.Events(info.ID, 0)
	if err != nil {
		t.Fatalf("hallazgo d4e3bf: Events del run: %v", err)
	}
	todo := allDetails(evs)
	if !strings.Contains(todo, "Permitir escribir? [s/N]") {
		t.Fatalf("hallazgo d4e3bf: fixture: el pedido de permiso llega a la narracion:\n%s", todo)
	}
	d := vdtDetalleTimeout(evs)
	sub := r2Mudo.FindStringSubmatch(d)
	if fin.Status != StatusError || sub == nil {
		t.Fatalf("hallazgo d4e3bf, CA-495: un pedido de permiso sin salto y 65 s de silencio vence con\n  %s\nfue\n  %q\n%s",
			"timeout tras <d> sin narracion desde hace <m>: la CLI puede estar esperando un permiso interactivo. Accion: preconfigura sus permisos headless",
			d, todo)
	}
	minutos, _ := strconv.Atoi(sub[2])
	seg, _ := strconv.Atoi(sub[3])
	if hace := time.Duration(minutos)*time.Minute + time.Duration(seg)*time.Second; hace < VentanaNarrando {
		t.Fatalf("hallazgo d4e3bf, CA-495: sin narracion desde hace mas de 60 s (desde que llego el pedido de permiso), no %s: %q", hace, d)
	}
}
