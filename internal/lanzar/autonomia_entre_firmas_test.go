// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md
// (CA-661) sobre la funcion de lanzamiento compartida: todo sobre que lanza
// Lanzar lleva en su registro la columna de la tarjeta al lanzar y el pedido
// que recibio el rol; lo que el piloto pone en el Request (Pilot, Regla,
// Intento, Ronda, Hallazgos, Bloqueantes) llega al registro tal cual, y sin
// eso el registro no trae esas claves. CLI falso (r1Claude), nunca uno real.
package lanzar_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/lanzar"
)

// aelLanzar lanza req en la tarjeta precios y espera a que el trabajo
// termine; devuelve el registro del sobre como mapa crudo y como Record.
func aelLanzar(t *testing.T, req lanzar.Request) (map[string]any, envelope.Record) {
	t.Helper()
	r1Claude(t)
	root, m := vdtProyecto(t)
	if req.Pilot {
		// aclaracion 13: el piloto solo lanza sobre un item con
		// `auto: hasta-humano` (y su presupuesto_usd).
		item := filepath.Join(root, ".hoom", "items", "precios.yaml")
		raw, err := os.ReadFile(item)
		if err != nil {
			t.Fatal(err)
		}
		vdtEscribir(t, root, ".hoom/items/precios.yaml", string(raw)+"presupuesto_usd: 5\nauto: hasta-humano\n")
		vdtGit(t, root, "add", "-A")
		vdtGit(t, root, "commit", "-q", "-m", "piloto automatico en precios")
	}
	antes, err := boardcmd.CardFor(root, "main", "", "precios", time.Now().UTC())
	if err != nil || antes.Column != boardcmd.ColBacklog {
		t.Fatalf("fixture: la tarjeta precios esta en Backlog: %q %v", antes.Column, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, "precios", req)
	if err != nil || tr == nil || tr.EnvelopeID == "" {
		t.Fatalf("fixture: el lanzamiento arranca: %v %+v", err, tr)
	}
	tr.Wait()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, tr.EnvelopeID+".json"))
	if err != nil {
		t.Fatalf("CA-661: el registro del sobre esta en .hoom/envelopes/: %v", err)
	}
	var crudo map[string]any
	if err := json.Unmarshal(raw, &crudo); err != nil {
		t.Fatalf("CA-661: el registro es JSON: %v\n%s", err, raw)
	}
	for _, r := range envelope.List(root) {
		if r.ID == tr.EnvelopeID {
			return crudo, r
		}
	}
	t.Fatalf("CA-661: envelope.List trae el sobre %s", tr.EnvelopeID)
	return nil, envelope.Record{}
}

// CA-661: un lanzamiento de una persona (sin Pilot) lleva columna y pedido,
// y ninguna clave del piloto.
func TestCA661_TodoSobreDeLanzarLlevaColumnaYPedido(t *testing.T) {
	pedido := "Escribi el spec de precios por region, con su moneda."
	crudo, rec := aelLanzar(t, lanzar.Request{Action: boardcmd.ActPedirArquitecto, Pedido: pedido})
	if crudo["columna"] != boardcmd.ColBacklog || rec.Columna != boardcmd.ColBacklog {
		t.Fatalf("CA-661: el sobre lleva la columna de la tarjeta al lanzar (backlog): %v %q", crudo["columna"], rec.Columna)
	}
	p, _ := crudo["pedido"].(string)
	if !strings.Contains(p, pedido) || rec.Pedido != p {
		t.Fatalf("CA-661: el sobre lleva el pedido que recibio el rol: %q", p)
	}
	for _, k := range []string{"piloto", "regla", "intento", "ronda", "hallazgos", "bloqueantes"} {
		if v, ok := crudo[k]; ok {
			t.Fatalf("CA-661: un lanzamiento de una persona no lleva %q: %v", k, v)
		}
	}
}

// CA-661: lo que el piloto pone en el Request llega al registro con sus
// claves, junto a columna, pedido y piloto: true.
func TestCA661_LoDelPilotoLlegaAlRegistro(t *testing.T) {
	pedido := "Escribi el spec de precios por region."
	hallazgos := []string{"20261112T010101_f00001", "20261112T010101_f00002"}
	crudo, rec := aelLanzar(t, lanzar.Request{Action: boardcmd.ActPedirArquitecto, Pedido: pedido, Pilot: true,
		Regla: boardcmd.ReglaRondaTestWriter, Intento: 2, Ronda: 1, Hallazgos: hallazgos, Bloqueantes: 2})
	want := map[string]any{"piloto": true, "columna": boardcmd.ColBacklog, "regla": boardcmd.ReglaRondaTestWriter,
		"intento": 2.0, "ronda": 1.0, "hallazgos": []any{hallazgos[0], hallazgos[1]}, "bloqueantes": 2.0}
	for k, v := range want {
		if !reflect.DeepEqual(crudo[k], v) {
			t.Fatalf("CA-661: el registro trae %q = %v; fue %v\n%v", k, v, crudo[k], crudo)
		}
	}
	if p, _ := crudo["pedido"].(string); !strings.Contains(p, pedido) {
		t.Fatalf("CA-661: tambien con el piloto el sobre lleva su pedido: %q", p)
	}
	if !rec.Pilot || rec.Regla != boardcmd.ReglaRondaTestWriter || rec.Intento != 2 || rec.Ronda != 1 ||
		!reflect.DeepEqual(rec.Hallazgos, hallazgos) || rec.Bloqueantes != 2 {
		t.Fatalf("CA-661: Record: %+v", rec)
	}

	// solo la regla: las demas claves no aparecen en cero
	crudo, _ = aelLanzar(t, lanzar.Request{Action: boardcmd.ActPedirArquitecto, Pedido: pedido, Pilot: true,
		Regla: boardcmd.ReglaCadena})
	if crudo["regla"] != boardcmd.ReglaCadena || crudo["piloto"] != true {
		t.Fatalf("CA-661: todo lanzamiento del piloto lleva piloto: true y su regla: %v", crudo)
	}
	for _, k := range []string{"intento", "ronda", "hallazgos", "bloqueantes"} {
		if v, ok := crudo[k]; ok {
			t.Fatalf("CA-661: la regla cadena no lleva %q: %v", k, v)
		}
	}
}
