// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// `hoom item add --riesgo` (CA-616) y el verbo nuevo `hoom item set <slug>
// --riesgo r [--json]` (CA-617), a nivel paquete y contra el binario. Escritos
// desde el spec y las firmas publicas, sin leer la implementacion.
package itemcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
)

const crFuera = `riesgo %q fuera del vocabulario (bajo, normal, alto)`

// crHead es el sha de HEAD de root.
func crHead(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// crCommitearItems commitea .hoom/items/ para que lo que haga set se vea en
// git status.
func crCommitearItems(t *testing.T, root string) {
	t.Helper()
	icGit(t, root, "add", "-A")
	icGit(t, root, "commit", "-q", "-m", "items")
}

// crHecho escribe un item cerrado por `hoom task done` y exige (guarda del
// fixture) que su tarjeta este en Hecho.
func crHecho(t *testing.T, root, slug string) {
	t.Helper()
	icEscribir(t, root, ".hoom/items/"+slug+".yaml", icItemYAML("Hecha", "2026-09-22T15:04:05Z",
		"hecho_en: 2026-09-23T10:00:00Z\ncommit_final: 3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c\n"))
	c, err := boardcmd.CardFor(root, "main", "high", slug, time.Now().UTC())
	if err != nil || c.Column != boardcmd.ColHecho {
		t.Fatalf("fixture: la tarjeta %s esta en Hecho: %q %v", slug, c.Column, err)
	}
}

// CA-616: Parse lleva --riesgo de add en Draft.Riesgo.
func TestCA616_ParseAddConRiesgo(t *testing.T) {
	for _, r := range []string{"bajo", "normal", "alto"} {
		req, err := Parse([]string{"add", "Arreglar login", "--riesgo", r})
		if err != nil || req.Sub != SubAdd || req.Draft.Riesgo != r {
			t.Fatalf("CA-616: --riesgo %s: %+v %v", r, req, err)
		}
	}
	req, err := Parse([]string{"add", "--riesgo", "alto", "Login", "--tipo", "bug"})
	if err != nil || req.Draft.Riesgo != "alto" || req.Draft.Titulo != "Login" || req.Draft.Tipo != "bug" {
		t.Fatalf("CA-616: --riesgo antes del titulo: %+v %v", req, err)
	}
	if req, err := Parse([]string{"add", "Sin"}); err != nil || req.Draft.Riesgo != "" {
		t.Fatalf("CA-616: sin --riesgo el borrador no lo trae: %+v %v", req, err)
	}
}

// CA-616: un valor fuera del vocabulario es error de uso (exit 2) con el
// texto de item.Parse.
func TestCA616_ParseAddRechazaOtroRiesgo(t *testing.T) {
	for _, r := range []string{"critico", "Alto", "", "medio"} {
		args := []string{"add", "X", "--riesgo", r}
		_, err := Parse(args)
		ue := icUso(t, "CA-616", args, err)
		if r != "" && !strings.Contains(ue.Error(), fmt.Sprintf(crFuera, r)) {
			t.Fatalf("CA-616: el texto es el de item.Parse %q: %v", fmt.Sprintf(crFuera, r), ue)
		}
	}
	icUso(t, "CA-616", []string{"add", "X", "--riesgo"}, func() error { _, err := Parse([]string{"add", "X", "--riesgo"}); return err }())
}

// CA-616: Execute add con --riesgo alto escribe la clave; con normal o sin el
// flag, no.
func TestCA616_ExecuteAddConRiesgo(t *testing.T) {
	root := icRepo(t)
	if _, _, err := icExec(t, root, "add", "Alto", "--slug", "alto", "--riesgo", "alto"); err != nil {
		t.Fatalf("CA-616: %v", err)
	}
	if _, _, err := icExec(t, root, "add", "Normal", "--slug", "normal", "--riesgo", "normal"); err != nil {
		t.Fatalf("CA-616: %v", err)
	}
	if _, _, err := icExec(t, root, "add", "Sin", "--slug", "sin"); err != nil {
		t.Fatalf("CA-616: %v", err)
	}
	raw, _ := os.ReadFile(item.Path(root, "alto"))
	if !bytes.Contains(raw, []byte("riesgo: alto\n")) {
		t.Fatalf("CA-616: --riesgo alto escribe riesgo: alto:\n%s", raw)
	}
	for _, s := range []string{"normal", "sin"} {
		raw, _ := os.ReadFile(item.Path(root, s))
		if bytes.Contains(raw, []byte("riesgo")) {
			t.Fatalf("CA-616: %s no lleva la clave:\n%s", s, raw)
		}
	}
	// el JSON de add trae el riesgo del item
	out, _, err := icExec(t, root, "add", "Bajo", "--slug", "bajo", "--riesgo", "bajo", "--json")
	if err != nil || !strings.Contains(out, `"riesgo"`) || !strings.Contains(out, `"bajo"`) {
		t.Fatalf("CA-616: el JSON de add trae el riesgo: %v\n%s", err, out)
	}
}

// CA-617: Parse de set: slug, --riesgo y --json.
func TestCA617_ParseSet(t *testing.T) {
	req, err := Parse([]string{"set", "precios", "--riesgo", "alto", "--json"})
	if err != nil || req.Sub != SubSet || req.Slug != "precios" || req.Riesgo != "alto" || !req.JSON {
		t.Fatalf("CA-617: %+v %v", req, err)
	}
	req, err = Parse([]string{"set", "--riesgo", "bajo", "precios"})
	if err != nil || req.Sub != SubSet || req.Slug != "precios" || req.Riesgo != "bajo" || req.JSON {
		t.Fatalf("CA-617: flags antes del slug: %+v %v", req, err)
	}
	for _, args := range [][]string{
		{"set", "precios"},                                    // sin --riesgo: el unico flag que cambia algo
		{"set", "precios", "--json"},                          // idem
		{"set", "precios", "--riesgo", ""},                    // vacio
		{"set", "precios", "--riesgo", "critico"},             // fuera del vocabulario
		{"set", "precios", "--riesgo"},                        // sin valor
		{"set", "--riesgo", "alto"},                           // sin slug
		{"set", "Foo Bar", "--riesgo", "alto"},                // slug que no es uno
		{"set", "a", "b", "--riesgo", "alto"},                 // dos slugs
		{"set", "precios", "--riesgo", "alto", "--tipo", "x"}, // otro flag
	} {
		_, err := Parse(args)
		icUso(t, "CA-617", args, err)
	}
	_, err = Parse([]string{"set", "precios", "--riesgo", "critico"})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf(crFuera, "critico")) {
		t.Fatalf("CA-617: un valor fuera del vocabulario lleva el texto de item.Parse: %v", err)
	}
}

// CA-617: set cambia solo esa clave, no commitea e imprime la linea del
// contrato; con --json, slug, antes y riesgo.
func TestCA617_ExecuteSet(t *testing.T) {
	t.Setenv("HOOM_ROLE", "")
	root := icRepo(t)
	cinco := 5.0
	if _, err := item.Add(root, item.Draft{Titulo: "Precios", Slug: "precios", Pedido: "por region", PresupuestoUSD: &cinco, Tipo: "bug"}); err != nil {
		t.Fatal(err)
	}
	crCommitearItems(t, root)
	head := crHead(t, root)
	antes, _ := item.Load(root, "precios")

	out, _, err := icExec(t, root, "set", "precios", "--riesgo", "alto")
	if err != nil {
		t.Fatalf("CA-617: %v", err)
	}
	if want := "riesgo de precios: normal -> alto - guardalo con: hoom item save precios"; !strings.Contains(out, want) {
		t.Fatalf("CA-617: imprime %q:\n%s", want, out)
	}
	despues, err := item.Load(root, "precios")
	if err != nil || despues.Riesgo != "alto" || despues.Pedido != antes.Pedido || despues.Tipo != "bug" ||
		despues.Titulo != antes.Titulo || despues.CreadoPor != antes.CreadoPor || !despues.CreadoEn.Equal(antes.CreadoEn) ||
		despues.PresupuestoUSD == nil || *despues.PresupuestoUSD != 5 {
		t.Fatalf("CA-617: cambia solo el riesgo:\nantes   %+v\ndespues %+v (%v)", antes, despues, err)
	}
	if crHead(t, root) != head {
		t.Fatal("CA-617: set no commitea")
	}
	st, _ := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if !strings.Contains(string(st), ".hoom/items/precios.yaml") {
		t.Fatalf("CA-617: el item queda cambiado sin commitear: %q", st)
	}

	// --json
	out, _, err = icExec(t, root, "set", "precios", "--riesgo", "bajo", "--json")
	if err != nil {
		t.Fatalf("CA-617: %v", err)
	}
	var js map[string]any
	if err := json.Unmarshal([]byte(out), &js); err != nil {
		t.Fatalf("CA-617: --json es JSON: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(js, map[string]any{"slug": "precios", "antes": "alto", "riesgo": "bajo"}) {
		t.Fatalf("CA-617: el JSON es {slug, antes, riesgo}: %v", js)
	}

	// a normal: el archivo queda sin la clave
	out, _, err = icExec(t, root, "set", "precios", "--riesgo", "normal")
	if err != nil || !strings.Contains(out, "riesgo de precios: bajo -> normal - guardalo con: hoom item save precios") {
		t.Fatalf("CA-617: %v\n%s", err, out)
	}
	raw, _ := os.ReadFile(item.Path(root, "precios"))
	if bytes.Contains(raw, []byte("riesgo")) {
		t.Fatalf("CA-617: con --riesgo normal el archivo queda sin la clave:\n%s", raw)
	}
}

// CA-617: con el riesgo que ya tenia imprime `ya es` y no escribe, en texto
// y en JSON.
func TestCA617_ExecuteSetYaEs(t *testing.T) {
	t.Setenv("HOOM_ROLE", "")
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", "riesgo: alto\n"))
	p := item.Path(root, "precios")
	raw0, _ := os.ReadFile(p)
	st0, _ := os.Stat(p)
	time.Sleep(10 * time.Millisecond)
	out, _, err := icExec(t, root, "set", "precios", "--riesgo", "alto")
	if err != nil || !strings.Contains(out, "riesgo de precios: ya es alto") {
		t.Fatalf("CA-617: %v\n%s", err, out)
	}
	if strings.Contains(out, "->") {
		t.Fatalf("CA-617: sin cambio no imprime la linea de cambio:\n%s", out)
	}
	out, _, err = icExec(t, root, "set", "precios", "--riesgo", "alto", "--json")
	var js map[string]any
	if err != nil || json.Unmarshal([]byte(out), &js) != nil ||
		!reflect.DeepEqual(js, map[string]any{"slug": "precios", "antes": "alto", "riesgo": "alto"}) {
		t.Fatalf("CA-617: --json sin cambio: %v %v\n%s", js, err, out)
	}
	raw1, _ := os.ReadFile(p)
	st1, _ := os.Stat(p)
	if !bytes.Equal(raw0, raw1) || !st0.ModTime().Equal(st1.ModTime()) {
		t.Fatal("CA-617: con el riesgo que ya tenia no escribe")
	}
	// un item sin la clave y --riesgo normal: ya es normal
	icEscribir(t, root, ".hoom/items/sin.yaml", icItemYAML("Sin", "2026-09-22T15:04:05Z", ""))
	out, _, err = icExec(t, root, "set", "sin", "--riesgo", "normal")
	if err != nil || !strings.Contains(out, "riesgo de sin: ya es normal") {
		t.Fatalf("CA-617: %v\n%s", err, out)
	}
}

// CA-617: Set se niega, sin escribir, dentro de una corrida, sin item y en
// Hecho, con los textos del contrato, en ese orden.
func TestCA617_SetSeNiega(t *testing.T) {
	t.Setenv("HOOM_ROLE", "")
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	crHecho(t, root, "hecha")
	foto := ixFotoItems(t, root)

	casos := []struct {
		slug, rol, want string
	}{
		{"precios", "writer", "el riesgo de una tarjeta lo cambia una persona, no un rol"},
		{"precios", "refutador", "el riesgo de una tarjeta lo cambia una persona, no un rol"},
		{"nadie", "writer", "el riesgo de una tarjeta lo cambia una persona, no un rol"}, // el rol va primero
		{"nadie", "", `no existe el item "nadie"`},
		{"hecha", "", "la tarjeta ya esta en Hecho"},
		{"hecha", "reviewer", "el riesgo de una tarjeta lo cambia una persona, no un rol"},
	}
	for _, c := range casos {
		_, err := Set(root, "main", "high", c.slug, "alto", c.rol)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("CA-617: Set(%s, rol %q) se niega con %q: %v", c.slug, c.rol, c.want, err)
		}
		if !ixMismos(foto, ixFotoItems(t, root)) {
			t.Fatalf("CA-617: Set(%s, rol %q) no escribe", c.slug, c.rol)
		}
	}
	// Execute lee HOOM_ROLE
	t.Setenv("HOOM_ROLE", "writer")
	if _, _, err := icExec(t, root, "set", "precios", "--riesgo", "alto"); err == nil ||
		!strings.Contains(err.Error(), "el riesgo de una tarjeta lo cambia una persona, no un rol") {
		t.Fatalf("CA-617: Execute lee $HOOM_ROLE: %v", err)
	}
	if !ixMismos(foto, ixFotoItems(t, root)) {
		t.Fatal("CA-617: dentro de una corrida no escribe")
	}
	// Set da el SetResult del contrato
	t.Setenv("HOOM_ROLE", "")
	res, err := Set(root, "main", "high", "precios", "bajo", "")
	if err != nil || res != (SetResult{Slug: "precios", Antes: "normal", Riesgo: "bajo", Cambio: true}) {
		t.Fatalf("CA-617: %+v %v", res, err)
	}
	res, err = Set(root, "main", "high", "precios", "bajo", "")
	if err != nil || res != (SetResult{Slug: "precios", Antes: "bajo", Riesgo: "bajo", Cambio: false}) {
		t.Fatalf("CA-617: con el que ya tenia: %+v %v", res, err)
	}
}

// crCorrer corre el binario con HOOM_TASK fuera y HOOM_ROLE = rol.
func crCorrer(t *testing.T, bin, dir, rol string, args ...string) ixSalida {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOOM_TASK=") && !strings.HasPrefix(kv, "HOOM_ROLE=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	if rol != "" {
		cmd.Env = append(cmd.Env, "HOOM_ROLE="+rol)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("no pude ejecutar hoom %v: %v", args, err)
	}
	return ixSalida{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

// CA-616 (binario): un --riesgo fuera del vocabulario sale con exit 2 y el
// texto de item.Parse, sin crear el item.
func TestCA616_E2EAddRiesgoInvalidoExit2(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	antes := ixFotoItems(t, root)
	r := crCorrer(t, bin, root, "", "item", "add", "Login", "--riesgo", "critico")
	if r.exit != 2 || !strings.Contains(r.stderr, fmt.Sprintf(crFuera, "critico")) {
		t.Fatalf("CA-616: exit 2 con el texto de item.Parse: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	if !ixMismos(antes, ixFotoItems(t, root)) {
		t.Fatal("CA-616: no crea el item")
	}
	r = crCorrer(t, bin, root, "", "item", "add", "Login", "--riesgo", "alto")
	if r.exit != 0 {
		t.Fatalf("CA-616: --riesgo alto: %d\n%s", r.exit, r.stderr)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".hoom", "items", "login.yaml"))
	if !bytes.Contains(raw, []byte("riesgo: alto\n")) {
		t.Fatalf("CA-616: %s", raw)
	}
}

// CA-617 (binario): exit 2 sin --riesgo o fuera del vocabulario; exit 1 con
// HOOM_ROLE, sin item o en Hecho; exit 0 y la linea del contrato al cambiar.
func TestCA617_E2ESetExitCodes(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	crHecho(t, root, "hecha")
	foto := ixFotoItems(t, root)
	for _, c := range []struct {
		rol  string
		args []string
		exit int
		want string
	}{
		{"", []string{"item", "set", "precios"}, 2, ""},
		{"", []string{"item", "set", "precios", "--riesgo", "critico"}, 2, fmt.Sprintf(crFuera, "critico")},
		{"writer", []string{"item", "set", "precios", "--riesgo", "alto"}, 1, "el riesgo de una tarjeta lo cambia una persona, no un rol"},
		{"", []string{"item", "set", "nadie", "--riesgo", "alto"}, 1, `no existe el item "nadie"`},
		{"", []string{"item", "set", "hecha", "--riesgo", "alto"}, 1, "la tarjeta ya esta en Hecho"},
	} {
		r := crCorrer(t, bin, root, c.rol, c.args...)
		if r.exit != c.exit || !strings.Contains(r.stderr, c.want) {
			t.Fatalf("CA-617: hoom %v (HOOM_ROLE=%q): exit %d con %q; fue %d\nstdout: %s\nstderr: %s",
				c.args, c.rol, c.exit, c.want, r.exit, r.stdout, r.stderr)
		}
		if !ixMismos(foto, ixFotoItems(t, root)) {
			t.Fatalf("CA-617: hoom %v no escribe", c.args)
		}
	}
	r := crCorrer(t, bin, root, "", "item", "set", "precios", "--riesgo", "alto")
	if r.exit != 0 || !strings.Contains(r.stdout, "riesgo de precios: normal -> alto - guardalo con: hoom item save precios") {
		t.Fatalf("CA-617: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	r = crCorrer(t, bin, root, "", "item", "set", "precios", "--riesgo", "alto")
	if r.exit != 0 || !strings.Contains(r.stdout, "riesgo de precios: ya es alto") {
		t.Fatalf("CA-617: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
}
