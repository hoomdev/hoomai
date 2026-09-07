// Tests del writer para los hallazgos 20260907T000658_31bc43 (cada poll
// releia el log completo) y 20260907T001832_31542d (el id del sidecar
// elegia la ruta) de la review cruzada sobre Spec D + Spec E.
package runcmd

import (
	"os"
	"path/filepath"
	"testing"
)

// 20260907T000658_31bc43: leer desde un offset devuelve solo lo nuevo, y una
// linea a medio escribir queda para la proxima lectura.
func TestReadEventsFromIncremental(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(runsDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(runsDir(root), "a.jsonl")
	dos := "{\"kind\":\"start\",\"detail\":\"uno\"}\n{\"kind\":\"text\",\"detail\":\"dos\"}\n"
	if err := os.WriteFile(log, []byte(dos), 0o644); err != nil {
		t.Fatal(err)
	}
	evs, next, err := ReadEventsFrom(root, "a", 0)
	if err != nil || len(evs) != 2 || next != int64(len(dos)) {
		t.Fatalf("primera lectura: %d eventos, next=%d err=%v", len(evs), next, err)
	}
	f, _ := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("{\"kind\":\"text\",\"detail\":\"parci")
	f.Close()
	evs, next2, err := ReadEventsFrom(root, "a", next)
	if err != nil || len(evs) != 0 || next2 != next {
		t.Fatalf("una linea a medias no se consume: %d eventos, next=%d (antes %d) err=%v", len(evs), next2, next, err)
	}
	f, _ = os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("al\"}\n")
	f.Close()
	evs, next3, err := ReadEventsFrom(root, "a", next2)
	if err != nil || len(evs) != 1 || evs[0].Detail != "parcial" {
		t.Fatalf("la linea completada llega entera: %+v err=%v", evs, err)
	}
	if st, _ := os.Stat(log); next3 != st.Size() {
		t.Fatalf("el offset final es el tamano del archivo: %d vs %d", next3, st.Size())
	}
	if all, err := ReadEvents(root, "a"); err != nil || len(all) != 3 {
		t.Fatalf("ReadEvents sigue leyendo todo: %d err=%v", len(all), err)
	}
}

// 20260907T001832_31542d: un id es un nombre de archivo, jamas una ruta.
func TestValidIDRechazaRutas(t *testing.T) {
	for _, bad := range []string{"", "../../evil", "a/b", "..", ".", "x\\y", " con espacio", ".oculto"} {
		if validID(bad) {
			t.Fatalf("validID(%q) debe ser false", bad)
		}
	}
	for _, ok := range []string{"20260906T230000_zzz", "viejo", "run-1.x", "A1_b"} {
		if !validID(ok) {
			t.Fatalf("validID(%q) debe ser true", ok)
		}
	}
}
