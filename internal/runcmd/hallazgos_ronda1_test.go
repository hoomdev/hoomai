// Tests adversariales de la ronda 1 de la review de verbos-de-la-tarjeta en
// el run: hallazgo 829af2 (CA-495). Una linea del provider que hoom descarta
// por pasar de MaxLinea sigue siendo el provider narrando: un provider que
// escribe lineas gigantes hasta que vence su tiempo limite recibe el mensaje
// "seguia narrando", no el "sin narracion" que manda a mirar los permisos.
// Los textos y el fixture de tiempo limite corto son los de CA-495
// (verbos_de_la_tarjeta_test.go).
package runcmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// r1Gigantes escribe en dir un archivo con una linea de mas de MaxLinea
// bytes (que no es JSON) y devuelve su ruta.
func r1Gigantes(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gigante.txt")
	if err := os.WriteFile(p, []byte("r1-gigante "+strings.Repeat("g", MaxLinea+32<<10)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// hallazgo 829af2, CA-495: un provider que, hasta que vence el tiempo
// limite, solo escribe lineas de mas de MaxLinea (que hoom descarta) estaba
// narrando: el detalle es "seguia narrando", por stdout y por stderr.
func TestHallazgo_829af2_CA495_LineasDescartadasSonNarracion(t *testing.T) {
	gigante := r1Gigantes(t)
	for _, k := range []struct{ nombre, script string }{
		{"stdout", "while true; do cat '" + gigante + "'; sleep 0.1; done\n"},
		{"stderr", "while true; do cat '" + gigante + "' >&2; sleep 0.1; done\n"},
	} {
		t.Run(k.nombre, func(t *testing.T) { r1Narrando(t, k.nombre, k.script) })
	}
}

// r1Narrando corre el provider de script con un tiempo limite de 2 s y
// exige el detalle "seguia narrando" del writer.
func r1Narrando(t *testing.T, por, script string) {
	t.Helper()
	installFake(t, script)
	root := vdtRaiz(t)
	m := NewManager(root)
	fin, evs, _ := vdtCorrer(t, "CA-495", m, StartOptions{Provider: "claude", Prompt: "trabaja", Role: "writer", Timeout: 2 * time.Second}, 40*time.Second)
	if por == "stdout" && !strings.Contains(allDetails(evs), "aviso: hoom descarto una linea") {
		t.Fatalf("hallazgo 829af2 (%s): fixture: hoom descarto lineas de mas de MaxLinea:\n%.2000s", por, allDetails(evs))
	}
	d := vdtDetalleTimeout(evs)
	if fin.Status != StatusError || !regexp.MustCompile(vdtNarrando+`--timeout o agents\.writer\.timeout en hoom\.yaml`+vdtFinal).MatchString(d) {
		t.Fatalf("hallazgo 829af2, CA-495 (%s): un provider que escribio lineas descartadas hasta el final seguia narrando:\n  quiero %s\n  fue    %q",
			por, "timeout tras 2s: la CLI seguia narrando (ultimo evento hace <s>s), no esperaba un permiso. Accion: subi el tiempo limite (--timeout o agents.writer.timeout en hoom.yaml) o pedi menos por run", d)
	}
}
