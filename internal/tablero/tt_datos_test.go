// Los datos reales (CA-521, CA-523, CA-530, CA-531; en la v2 CA-556, CA-558,
// CA-559, CA-560 y CA-562): DatosDe sobre un proyecto en disco (git con
// gittest.EntornoSolo, ningun provider): el tablero de boardcmd.Build, la
// tarjeta fijada, el detalle con su historia y la cinta del Sobre con solo
// los ultimos MaxSobre bytes de la narracion, el proyecto ([1]), los
// hallazgos abiertos ([4]) y las lineas del archivo de un hallazgo por el
// os.Root del proyecto.
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
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/costcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/providers"
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

// CA-521, CA-523, CA-556: el detalle es el de boardcmd.DetailFor sin diff,
// con la historia de boardcmd.TimelineFor, y el Sobre trae solo los eventos
// de los ultimos MaxSobre bytes de la narracion del run mas nuevo (caso
// limite: un log de varios MB). Re-expresado por CA-556: [0] de una tarjeta
// es un solo documento con su Historia, asi que el Detalle la trae siempre
// (en la v1 solo cuando su pestana la pedia).
func TestCA523_CA556_DatosDeDetalleYSobre(t *testing.T) {
	root, runID := ttProyecto(t)
	d := ttDatosDe(t, root)
	det := d.Detalle("datos")
	if det.Err != nil || det.Slug != "datos" || det.Detalle.Card.Slug != "datos" {
		t.Fatalf("CA-521: el detalle de la tarjeta: slug %q, card %q, err %v", det.Slug, det.Detalle.Card.Slug, det.Err)
	}
	if det.Detalle.Diff != nil {
		t.Fatalf("CA-521: el detalle va sin diff")
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
	tl, err := boardcmd.TimelineFor(root, "main", "high", "datos", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(det.Historia.Entries) != len(tl.Entries) || len(tl.Entries) == 0 {
		t.Fatalf("CA-556: el detalle trae la historia de boardcmd.TimelineFor: %d entradas, se esperaban %d", len(det.Historia.Entries), len(tl.Entries))
	}
	if mal := d.Detalle("no-existe"); mal.Err == nil {
		t.Fatalf("CA-521: el detalle de una tarjeta que no existe es un error")
	}
}

// CA-523: varios MB de narracion: el Sobre sigue trayendo solo la ventana.
func TestCA523_DatosDeLogDeVariosMB(t *testing.T) {
	root, runID := ttProyecto(t)
	ttArchivo(t, root, ".hoom/runs/"+runID+".jsonl", ttNarracion(40000)) // 4 MB
	det := ttDatosDe(t, root).Detalle("datos")
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
	det := ttDatosDe(t, root).Detalle("datos")
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

// ---------------------------------------------------------------- v2: proyecto, hallazgos y archivo

// ttRunDeHoy escribe en root el sidecar de un run que empezo ahora (en la
// hora local), con costo o sin el. (CA-561, hallazgo 27e287: CreatedAt es
// ahora mismo, no un segundo antes, que entre las 00:00:00 y las 00:00:01
// caeria ayer.)
func ttRunDeHoy(t *testing.T, root, id string, costo *float64) {
	t.Helper()
	ahora := time.Now()
	meta := runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: "datos", Dir: root,
		CreatedAt: ahora, EndedAt: ahora, Status: runcmd.StatusDone}
	if costo != nil {
		meta.Usage = &providers.Usage{InputTokens: 10, OutputTokens: 1, CostUSD: costo, Turns: 1}
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	ttArchivo(t, root, ".hoom/runs/"+id+".meta.json", string(raw)+"\n")
}

// CA-560, CA-562: Proyecto lee la rama de gitx ("main" en el repositorio de
// prueba), el check de hoom status (sin veredicto: ReasonNoVerdict y
// Veredicto nil, lo que [1] dice "sin veredicto todavia"), el gasto de hoy de
// costcmd.HoyEn con el reloj local y los problemas de boardcmd.Doctor; sin
// error.
func TestCA560_CA562_DatosDeProyecto(t *testing.T) {
	root, _ := ttProyecto(t)
	ahora := time.Now()
	if ahora.Hour() == 23 && ahora.Minute() == 59 {
		t.Skip("a un minuto de medianoche el dia de hoy puede cambiar entre dos lecturas")
	}
	ttRunDeHoy(t, root, ahora.UTC().Format("20060102T150405")+"_a1b2c3", ttF(0.75))
	p := ttDatosDe(t, root).Proyecto()
	if p.Err != nil {
		t.Fatalf("CA-562: el proyecto se lee sin error: %v", p.Err)
	}
	if !p.Git || p.Rama != "main" {
		t.Fatalf("CA-560: con git, la rama de gitx (main): git %v, rama %q", p.Git, p.Rama)
	}
	if p.Check.OK || p.Check.Reason != checkcmd.ReasonNoVerdict || p.Veredicto != nil {
		t.Fatalf("CA-560: sin veredicto el check dice no-verdict y no hay ultimo veredicto: %+v, %+v", p.Check, p.Veredicto)
	}
	hoy, err := costcmd.HoyEn(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p.Hoy != hoy || p.Hoy.Runs < 1 {
		t.Fatalf("CA-560: el gasto de hoy es el de costcmd.HoyEn con el reloj local: %+v, se esperaba %+v", p.Hoy, hoy)
	}
	doc, err := boardcmd.Doctor(root, "main", "high", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var got, quiere []string
	for _, x := range p.Problemas {
		got = append(got, x.ID+" "+x.Slug)
	}
	for _, x := range doc.Problems {
		quiere = append(quiere, x.ID+" "+x.Slug)
	}
	if strings.Join(got, "|") != strings.Join(quiere, "|") {
		t.Fatalf("CA-560: los problemas son los de boardcmd.Doctor: %q, se esperaban %q", got, quiere)
	}
}

// CA-560, caso limite: sin repositorio git, Proyecto dice Git false ([1]:
// "sin git") y no es un error.
func TestCA560_DatosDeProyectoSinGit(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ttArchivo(t, root, "hoom.yaml", ttYAML)
	ttArchivo(t, root, ".hoom/items/solo.yaml", "titulo: Solo\ntipo: feature\nprioridad: media\n"+
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n")
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	p := ttDatosDe(t, root).Proyecto()
	if p.Git {
		t.Fatalf("CA-560: sin repositorio, Git es false (\"sin git\"): %+v", p)
	}
}

// CA-559: Hallazgos son los de finding.List(root, base, true): los abiertos
// del proyecto, del mas viejo al mas nuevo.
func TestCA559_DatosDeHallazgos(t *testing.T) {
	root, _ := ttProyecto(t)
	d := ttDatosDe(t, root)
	if h := d.Hallazgos(); h.Err != nil || len(h.Items) != 0 {
		t.Fatalf("CA-559: sin hallazgos, la lista esta vacia y sin error: %+v", h)
	}
	if _, err := finding.Add(root, "main", "high", "risk", "app.go:1", "el primero", "hoom test"); err != nil {
		t.Fatal(err)
	}
	if _, err := finding.Add(root, "main", "low", "style", "app.go", "el segundo", "hoom test"); err != nil {
		t.Fatal(err)
	}
	quiere, _, err := finding.List(root, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	h := d.Hallazgos()
	if h.Err != nil || len(h.Items) != 2 || len(quiere) != 2 {
		t.Fatalf("CA-559: los dos hallazgos abiertos: %+v", h)
	}
	for i := range quiere {
		if h.Items[i].ID != quiere[i].ID || h.Items[i].Status != finding.StatusOpen {
			t.Fatalf("CA-559: los hallazgos son los de finding.List con openOnly, en su orden: %v", h.Items)
		}
	}
}

// ttLineasNumeradas son n lineas "linea <i>", con su salto.
func ttLineasNumeradas(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "linea %d\n", i)
	}
	return b.String()
}

// CA-559: Archivo lee por el os.Root del proyecto las lineas de
// max(1, linea-ContextoArchivo) a linea+ContextoArchivo (menos al final del
// archivo), sin sus saltos, con Desde la primera; con linea 0, las primeras
// 2*ContextoArchivo+1. Un archivo que no esta es Err (y no Fuera).
func TestCA559_DatosDeArchivo(t *testing.T) {
	root, _ := ttProyecto(t)
	ttArchivo(t, root, "internal/x/lineas.go", ttLineasNumeradas(40))
	d := ttDatosDe(t, root)
	k := tablero.ContextoArchivo
	casos := []struct {
		linea, desde, hasta int
	}{
		{20, 20 - k, 20 + k},
		{0, 1, 2*k + 1},
		{3, 1, 3 + k},
		{38, 38 - k, 40},
		{1, 1, 1 + k},
	}
	for _, c := range casos {
		a := d.Archivo("internal/x/lineas.go", c.linea)
		if a.Err != nil || a.Fuera {
			t.Fatalf("CA-559: la linea %d se lee: %+v", c.linea, a)
		}
		if a.Ruta != "internal/x/lineas.go" || a.Linea != c.linea || a.Desde != c.desde {
			t.Fatalf("CA-559: la lectura dice su ruta, su linea y Desde=%d: %+v", c.desde, a)
		}
		var quiere []string
		for i := c.desde; i <= c.hasta; i++ {
			quiere = append(quiere, fmt.Sprintf("linea %d", i))
		}
		if strings.Join(a.Lineas, "|") != strings.Join(quiere, "|") {
			t.Fatalf("CA-559: la linea %d trae las lineas %d..%d sin sus saltos: %q", c.linea, c.desde, c.hasta, a.Lineas)
		}
	}
	if a := d.Archivo("internal/x/no-existe.go", 3); a.Err == nil || a.Fuera {
		t.Fatalf("CA-559: un archivo que no esta es un error de lectura (no fuera del proyecto): %+v", a)
	}
	// CRLF: los saltos de Windows tampoco quedan
	ttArchivo(t, root, "crlf.go", "uno\r\ndos\r\ntres\r\n")
	if a := d.Archivo("crlf.go", 2); a.Err != nil || strings.Join(a.Lineas, "|") != "uno|dos|tres" {
		t.Fatalf("CA-559: las lineas van sin sus saltos (tambien \\r\\n): %q (%v)", a.Lineas, a.Err)
	}
}
