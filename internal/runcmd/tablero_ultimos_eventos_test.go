// runcmd.UltimosEventos (spec tablero-en-la-terminal, CA-523): los eventos
// de los ultimos max bytes de la narracion de un run, sin la linea cortada
// por el principio de la ventana ni una ultima linea a medio escribir; un id
// que no es un archivo regular de los runs es "run no encontrado".
package runcmd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/runcmd"
)

var tueT0 = time.Date(2026, 11, 8, 11, 0, 0, 0, time.UTC)

// tueLinea es el evento i como una linea JSON de exactamente 100 bytes.
func tueLinea(i int) string {
	ts := tueT0.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
	base := fmt.Sprintf(`{"ts":"%s","kind":"text","detail":"evento %05d`, ts, i)
	return base + strings.Repeat("x", 99-len(base)-2) + `"}` + "\n"
}

func tueLineas(desde, hasta int) string {
	var b strings.Builder
	for i := desde; i < hasta; i++ {
		b.WriteString(tueLinea(i))
	}
	return b.String()
}

// tueRun escribe la narracion del run id en root.
func tueRun(t *testing.T, id, contenido string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".hoom", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// tueDetalles son los numeros de evento de los eventos.
func tueDetalles(evs []runcmd.Event) string {
	var s []string
	for _, e := range evs {
		s = append(s, strings.TrimRight(strings.TrimPrefix(e.Detail, "evento "), "x"))
	}
	return strings.Join(s, ",")
}

func TestCA523_UltimosEventosVentana(t *testing.T) {
	const id = "20261108T110000_ultimo"
	diez := tueLineas(0, 10) // 1000 bytes; la linea k empieza en 100k
	casos := []struct {
		nombre    string
		contenido string
		max       int64
		quiere    string
	}{
		{"archivo corto: todo", tueLineas(0, 3), 64 << 10, "00000,00001,00002"},
		{"max igual al tamano: todo", diez, 1000, "00000,00001,00002,00003,00004,00005,00006,00007,00008,00009"},
		{"la linea cortada por la ventana queda afuera", diez, 250, "00008,00009"},
		{"una linea que empieza justo en la ventana no esta cortada", diez, 300, "00007,00008,00009"},
		{"ultima linea a medio escribir queda afuera", tueLineas(0, 3) + `{"ts":"2026-11-08T11:00`, 64 << 10, "00000,00001,00002"},
		{"ultima linea completa sin salto de linea cuenta", tueLineas(0, 2) + strings.TrimSuffix(tueLinea(2), "\n"), 64 << 10, "00000,00001,00002"},
		{"narracion vacia", "", 64 << 10, ""},
	}
	for _, c := range casos {
		root := tueRun(t, id, c.contenido)
		evs, err := runcmd.UltimosEventos(root, id, c.max)
		if err != nil {
			t.Fatalf("CA-523: %s: UltimosEventos: %v", c.nombre, err)
		}
		if got := tueDetalles(evs); got != c.quiere {
			t.Fatalf("CA-523: %s: eventos %q, se esperaban %q", c.nombre, got, c.quiere)
		}
	}
}

// CA-523, caso limite: un log de varios MB: solo los eventos de los ultimos
// 64 KB, con su hora y su kind.
func TestCA523_UltimosEventosVariosMB(t *testing.T) {
	const id = "20261108T110000_grande"
	root := tueRun(t, id, tueLineas(0, 40000)) // 4 000 000 bytes
	evs, err := runcmd.UltimosEventos(root, id, 64<<10)
	if err != nil {
		t.Fatalf("CA-523: UltimosEventos: %v", err)
	}
	// la ventana empieza en 3 934 464: la linea 39344 queda cortada
	if len(evs) != 655 || !strings.HasPrefix(evs[0].Detail, "evento 39345") || !strings.HasPrefix(evs[654].Detail, "evento 39999") {
		t.Fatalf("CA-523: de 4 MB, los 655 eventos de los ultimos 64 KB: %d", len(evs))
	}
	if !evs[0].TS.Equal(tueT0.Add(39345*time.Second)) || evs[0].Kind != "text" {
		t.Fatalf("CA-523: cada evento con su hora y su kind: %+v", evs[0])
	}
}

// CA-523: un id invalido, que trepa, que no existe o que es un symlink es
// "run no encontrado".
func TestCA523_UltimosEventosRunNoEncontrado(t *testing.T) {
	const id = "20261108T110000_bueno"
	root := tueRun(t, id, tueLineas(0, 3))
	afuera := filepath.Join(t.TempDir(), "secreto.jsonl")
	if err := os.WriteFile(afuera, []byte(tueLineas(0, 3)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(afuera, filepath.Join(root, ".hoom", "runs", "20261108T110000_enlace.jsonl")); err != nil {
		t.Skipf("sin symlinks: %v", err)
	}
	for _, mal := range []string{"../bueno", "..", "", "20261108T110000_no-existe", "20261108T110000_enlace", "a/b"} {
		evs, err := runcmd.UltimosEventos(root, mal, 64<<10)
		if err == nil || !strings.Contains(err.Error(), "run no encontrado") || len(evs) != 0 {
			t.Fatalf("CA-523: UltimosEventos(%q) es 'run no encontrado': %v, %d eventos", mal, err, len(evs))
		}
	}
	if evs, err := runcmd.UltimosEventos(root, id, 64<<10); err != nil || len(evs) != 3 {
		t.Fatalf("CA-523: el id bueno se lee: %v, %d", err, len(evs))
	}
}
