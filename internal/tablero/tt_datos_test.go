// Los datos reales (CA-521, CA-523, CA-530, CA-531): DatosDe sobre un
// proyecto en disco (git con gittest.EntornoSolo, ningun provider): el tablero
// de boardcmd.Build, la tarjeta fijada, el detalle, la historia y la cinta
// del Sobre con solo los ultimos MaxSobre bytes de la narracion.
package tablero_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

func ttGit(t *testing.T, env []string, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func ttArchivo(t *testing.T, root, rel, contenido string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ttProyecto es un proyecto hoom en disco con el item "datos" y un run
// suelto de esa tarea, cerrado, con una narracion de 2000 lineas de 100
// bytes (200 000 bytes).
func ttProyecto(t *testing.T) (string, string) {
	t.Helper()
	// git solo con su configuracion, sin tocar el entorno del proceso
	env := gittest.EntornoSolo(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ttGit(t, env, root, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	ttGit(t, env, root, "config", "user.email", "test@hoom.dev")
	ttGit(t, env, root, "config", "user.name", "hoom test")
	ttArchivo(t, root, "hoom.yaml", ttYAML)
	ttArchivo(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	ttArchivo(t, root, "app.go", "package app\n")
	ttGit(t, env, root, "add", "-A")
	ttGit(t, env, root, "commit", "-q", "-m", "inicial")
	ttArchivo(t, root, ".hoom/items/datos.yaml", "titulo: Datos de prueba\ntipo: feature\nprioridad: media\n"+
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n")
	runID := "20261108T110000_datos1"
	meta := runcmd.Meta{ID: runID, Provider: "claude", Role: "writer", Task: "datos", Dir: root,
		CreatedAt: ttAhora.Add(-time.Hour), EndedAt: ttAhora.Add(-30 * time.Minute), Status: runcmd.StatusDone}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	ttArchivo(t, root, ".hoom/runs/"+runID+".meta.json", string(raw)+"\n")
	ttArchivo(t, root, ".hoom/runs/"+runID+".jsonl", ttNarracion(2000))
	return root, runID
}

// ttNarracion son n lineas JSON de exactamente 100 bytes (con su "\n"): el
// evento i dice "evento <i con 5 digitos>" y va a las 11:00:00 + i segundos.
func ttNarracion(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		ts := ttAhora.Add(-time.Hour + time.Duration(i)*time.Second).UTC().Format(time.RFC3339)
		base := fmt.Sprintf(`{"ts":"%s","kind":"text","detail":"evento %05d`, ts, i)
		b.WriteString(base + strings.Repeat("x", 99-len(base)-2) + `"}` + "\n")
	}
	return b.String()
}

// ttDatosDe es DatosDe, que tiene que dar unos Datos.
func ttDatosDe(t *testing.T, root string) tablero.Datos {
	t.Helper()
	d := tablero.DatosDe(root, "main", "high")
	if d == nil {
		t.Fatalf("CA-530: DatosDe da unos Datos")
	}
	return d
}

// CA-530, CA-531: DatosDe lee el tablero de boardcmd.Build y la tarjeta
// fijada de tarjetacmd.ArchivoFijada.
func TestCA530_DatosDeTableroYFijada(t *testing.T) {
	root, _ := ttProyecto(t)
	d := ttDatosDe(t, root)
	b, err := d.Tablero()
	if err != nil {
		t.Fatalf("CA-530: Tablero: %v", err)
	}
	quiere, err := boardcmd.Build(root, "main", "high", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	slugs := func(b boardcmd.Board) string {
		var s []string
		for _, c := range b.Columns {
			s = append(s, c.ID+":")
			for _, k := range c.Cards {
				s = append(s, k.Slug)
			}
		}
		return strings.Join(s, " ")
	}
	if slugs(b) != slugs(quiere) || !strings.Contains(slugs(b), "datos") {
		t.Fatalf("CA-530: el tablero es el de boardcmd.Build:\n got %s\nwant %s", slugs(b), slugs(quiere))
	}
	if f := d.Fijada(); f != "" {
		t.Fatalf("CA-531: sin hoom usar no hay tarjeta fijada: %q", f)
	}
	ttArchivo(t, root, ".hoom/cache/tarjeta", "datos\n")
	if f := d.Fijada(); f != "datos" {
		t.Fatalf("CA-531: la tarjeta fijada es la de .hoom/cache/tarjeta: %q", f)
	}
}

// CA-521, CA-523: el detalle es el de boardcmd.DetailFor sin diff, la
// historia solo cuando se pide, y el Sobre trae solo los eventos de los
// ultimos MaxSobre bytes de la narracion del run mas nuevo (caso limite: un
// log de varios MB).
func TestCA523_DatosDeDetalleYSobre(t *testing.T) {
	root, runID := ttProyecto(t)
	d := ttDatosDe(t, root)
	det := d.Detalle("datos", false)
	if det.Err != nil || det.Slug != "datos" || det.Detalle.Card.Slug != "datos" {
		t.Fatalf("CA-521: el detalle de la tarjeta: slug %q, card %q, err %v", det.Slug, det.Detalle.Card.Slug, det.Err)
	}
	if det.Detalle.Diff != nil {
		t.Fatalf("CA-521: el detalle va sin diff")
	}
	if len(det.Historia.Entries) != 0 {
		t.Fatalf("CA-521: sin la pestana Historia no se lee la historia: %d entradas", len(det.Historia.Entries))
	}
	if det.Sobre.RunID != runID || det.Sobre.Err != nil {
		t.Fatalf("CA-523: el Sobre es la narracion del run mas nuevo %s: %q (%v)", runID, det.Sobre.RunID, det.Sobre.Err)
	}
	// 200 000 bytes; la ventana empieza en 134 464: la linea 1344 (134 400)
	// queda cortada y afuera; de la 1345 a la 1999, 655 eventos.
	ev := det.Sobre.Eventos
	if len(ev) != 655 || !strings.HasPrefix(ev[0].Detail, "evento 01345") || !strings.HasPrefix(ev[len(ev)-1].Detail, "evento 01999") {
		var primero string
		if len(ev) > 0 {
			primero = ev[0].Detail
		}
		t.Fatalf("CA-523: el Sobre lee solo los ultimos %d bytes: %d eventos, el primero %q", tablero.MaxSobre, len(ev), primero)
	}
	con := d.Detalle("datos", true)
	tl, err := boardcmd.TimelineFor(root, "main", "high", "datos", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(con.Historia.Entries) != len(tl.Entries) || len(tl.Entries) == 0 {
		t.Fatalf("CA-521: con la Historia, la de boardcmd.TimelineFor: %d entradas, se esperaban %d", len(con.Historia.Entries), len(tl.Entries))
	}
	if mal := d.Detalle("no-existe", false); mal.Err == nil {
		t.Fatalf("CA-521: el detalle de una tarjeta que no existe es un error")
	}
}

// CA-523: varios MB de narracion: el Sobre sigue trayendo solo la ventana.
func TestCA523_DatosDeLogDeVariosMB(t *testing.T) {
	root, runID := ttProyecto(t)
	ttArchivo(t, root, ".hoom/runs/"+runID+".jsonl", ttNarracion(40000)) // 4 MB
	det := ttDatosDe(t, root).Detalle("datos", false)
	if n := len(det.Sobre.Eventos); n < 600 || n > 656 {
		t.Fatalf("CA-523: con 4 MB de narracion el Sobre trae solo los eventos de los ultimos 64 KB: %d", n)
	}
	if ev := det.Sobre.Eventos; !strings.HasPrefix(ev[len(ev)-1].Detail, "evento 39999") {
		t.Fatalf("CA-523: el ultimo evento es el del final del log: %q", ev[len(ev)-1].Detail)
	}
}

// CA-523 (hallazgos 5d0044, 92edb2): la fila de trabajo mas nueva de la
// tarjeta es un run con su sidecar pero sin su narracion .jsonl (ni en la
// raiz ni en la carpeta de la evidencia: un run de otra maquina). El Sobre
// queda sin eventos y sin Err, y el detalle tampoco es un error.
func TestCA523_DatosDeRunSinNarracion(t *testing.T) {
	root, runID := ttProyecto(t)
	if err := os.Remove(filepath.Join(root, ".hoom", "runs", runID+".jsonl")); err != nil {
		t.Fatal(err)
	}
	// guarda: la fila mas nueva de la tarjeta es ese run (DetailFor es la
	// capa de abajo, ya hecha)
	quiere, err := boardcmd.DetailFor(root, "main", "high", "datos", time.Now(), false)
	if err != nil {
		t.Fatalf("CA-523 (guarda): DetailFor: %v", err)
	}
	if len(quiere.Work) == 0 || (quiere.Work[0].ID != runID && quiere.Work[0].RunID != runID) {
		t.Fatalf("CA-523 (guarda): la fila mas nueva de la tarjeta es el run %s: %+v", runID, quiere.Work)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", "runs", runID+".meta.json")); err != nil {
		t.Fatalf("CA-523 (guarda): el sidecar del run sigue: %v", err)
	}
	det := ttDatosDe(t, root).Detalle("datos", false)
	if det.Err != nil {
		t.Fatalf("CA-523: un run sin narracion no hace fallar el detalle: %v", det.Err)
	}
	if det.Detalle.Card.Slug != "datos" {
		t.Fatalf("CA-523: el detalle es el de la tarjeta: %q", det.Detalle.Card.Slug)
	}
	if det.Sobre.Err != nil || len(det.Sobre.Eventos) != 0 {
		t.Fatalf("CA-523: sin narracion el Sobre queda sin eventos y sin Err: %d eventos, %v", len(det.Sobre.Eventos), det.Sobre.Err)
	}
}
