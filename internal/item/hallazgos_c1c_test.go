// Tests de regresion del hallazgo 20260925T044719_d23526 (reliability) de la
// review cruzada de C1 (.hoom/specs/items-y-columna-derivada.md), escritos
// desde el contrato:
//
// CA-266: "un YAML roto, una clave desconocida, un tipo invalido o un nombre
// de archivo que no es slug se omiten con un aviso que nombra el archivo".
// item.List calla SOLO los temporales que hoom mismo escribe en .hoom/items/
// (CA-263, escrituras atomicas): nombres que empiezan con "." y contienen
// ".yaml.tmp-" (.facturas.yaml.tmp-1234567). Cualquier otro archivo que no
// sea <slug>.yaml, oculto o no, se omite CON un aviso que lo nombra.
package item

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/quick"
	"time"
)

// c1cTemporal dice si el nombre es un temporal de hoom segun el contrato.
func c1cTemporal(nombre string) bool {
	return strings.HasPrefix(nombre, ".") && strings.Contains(nombre, ".yaml.tmp-")
}

// c1cResetItems deja .hoom/items/ vacio.
func c1cResetItems(t *testing.T, root string) {
	t.Helper()
	if err := os.RemoveAll(Dir(root)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
}

// c1cDesde mapea bytes arbitrarios a un nombre sobre el alfabeto dado, de 1 a
// max caracteres (minusculas: sin choques en un sistema de archivos que no
// distingue mayusculas).
func c1cDesde(raw []byte, alfabeto string, max int) string {
	var b strings.Builder
	for i, c := range raw {
		if i == max {
			break
		}
		b.WriteByte(alfabeto[int(c)%len(alfabeto)])
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}

// Hallazgo 20260925T044719_d23526 (reliability). CA-266: los nombres que se
// PARECEN a un temporal de hoom pero no cumplen la forma exacta (empieza con
// "." y contiene ".yaml.tmp-") no son temporales: se omiten con un aviso que
// los nombra. Los temporales de verdad se callan aunque su item no exista.
func TestCA266_HallazgoD23526_ListAvisaPorLosCasiTemporales(t *testing.T) {
	root := itRepo(t)
	itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z", ""))
	parcial := itItemValido("Precios", "2026-09-22T15:04:05Z", "")[:25]
	conAviso := map[string]string{
		".precios.yaml.tmp":      parcial, // sin guion
		".precios.yaml.tmp123":   parcial, // sin guion
		".precios.yaml.tmp_123":  parcial, // guion bajo
		".precios.yml.tmp-123":   parcial, // .yml
		".otro.YAML.TMP-123":     parcial, // mayusculas
		".precios.yaml-tmp-123":  parcial,
		".precios.tmp-123":       parcial,
		"precios.yaml.tmp-123":   parcial, // sin el punto inicial
		".precios.yaml.swp":      "b0VIM 9.1",
		".precios.yaml~":         itItemValido("Precios viejo", "2026-09-22T15:04:05Z", ""),
		".precios.yaml.bak":      itItemValido("Precios viejo", "2026-09-22T15:04:05Z", ""),
		".cabina.yaml.orig":      "",
		".hoom-items-README.txt": "notas",
	}
	temporales := map[string]string{
		".precios.yaml.tmp-1":          parcial,
		".precios.yaml.tmp-4294967295": itItemValido("Precios", "2026-09-22T15:04:05Z", ""),
		".sin-item.yaml.tmp-123":       "titulo: [a medio escribir",
	}
	for n, c := range conAviso {
		itEscribir(t, root, ".hoom/items/"+n, c)
	}
	for n, c := range temporales {
		itEscribir(t, root, ".hoom/items/"+n, c)
	}
	items, warns, err := List(root)
	if err != nil {
		t.Fatalf("CA-266: un archivo que no es item nunca es fatal: %v", err)
	}
	if len(items) != 1 || items[0].Slug != "precios" {
		t.Errorf("CA-266: solo precios.yaml es un item: %+v", items)
	}
	var nombres []string
	for n := range conAviso {
		nombres = append(nombres, n)
	}
	sort.Strings(nombres)
	for _, n := range nombres {
		if !h1Avisa(warns, n) {
			t.Errorf("CA-266: hallazgo d23526: %q no es <slug>.yaml ni un temporal de hoom: se omite con un aviso que lo nombra: %q", n, warns)
		}
	}
	for n := range temporales {
		if h1Avisa(warns, n) {
			t.Errorf("CA-266: hallazgo d23526: %q es un temporal de hoom: se calla: %q", n, warns)
		}
	}
}

// Hallazgo 20260925T044719_d23526 (reliability). CA-266, como propiedad:
// para cualquier nombre oculto que NO contiene ".yaml.tmp-", List lo omite
// con un aviso que lo nombra; para cualquier ".<slug>.yaml.tmp-<n>", List
// calla; y ninguno de los dos es nunca un item.
func TestCA266_HallazgoD23526_PropiedadOcultosAvisanTemporalesNo(t *testing.T) {
	root := itRepo(t)
	sufijos := []string{"", ".yaml", ".yml", ".lock", ".tmp", ".yaml.tmp", ".yaml.tmp1", ".json", "~"}
	var fallas []string
	prop := func(raw []byte, suf uint8, n uint32, contenido []byte) bool {
		oculto := "." + c1cDesde(raw, "abcdefghijklmnopqrstuvwxyz0123456789-_.", 24) + sufijos[int(suf)%len(sufijos)]
		temporal := "." + c1cDesde(raw, "abcdefghijklmnopqrstuvwxyz0123456789", 40) + ".yaml.tmp-" + strconv.FormatUint(uint64(n), 10)
		if c1cTemporal(oculto) || oculto == "." || oculto == ".." || oculto == temporal {
			return true
		}
		c1cResetItems(t, root)
		itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z", ""))
		itEscribir(t, root, ".hoom/items/"+oculto, string(contenido))
		itEscribir(t, root, ".hoom/items/"+temporal, string(contenido))
		items, warns, err := List(root)
		switch {
		case err != nil:
			fallas = append(fallas, fmt.Sprintf("%q/%q: List fallo: %v", oculto, temporal, err))
		case len(items) != 1 || items[0].Slug != "precios":
			fallas = append(fallas, fmt.Sprintf("%q/%q: aparecieron items de mas: %d", oculto, temporal, len(items)))
		case !h1Avisa(warns, oculto):
			fallas = append(fallas, fmt.Sprintf("%q no avisa (warnings %q)", oculto, warns))
		case h1Avisa(warns, temporal):
			fallas = append(fallas, fmt.Sprintf("el temporal %q avisa (warnings %q)", temporal, warns))
		default:
			return true
		}
		return false
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		muestra := fallas
		if len(muestra) > 5 {
			muestra = muestra[:5]
		}
		t.Fatalf("CA-266: hallazgo d23526: List avisa por todo oculto que no es un temporal de hoom y calla los temporales:\n  %s\n(%v)",
			strings.Join(muestra, "\n  "), err)
	}
}

// GUARDA (verde en la base). Hallazgo 20260925T044719_d23526. CA-266 y
// CA-263: los temporales que hoom REALMENTE escribe (Add, AddSession,
// MarkDone y UnmarkDone, con un pedido grande que alarga la escritura) caen
// en la forma que List calla: un List que corre a la vez nunca avisa.
// Protege el recorte del silencio: si hoom escribiera en .hoom/items/ otro
// archivo propio (un lock, otro temporal), el tablero avisaria en cada
// escritura.
func TestCA266_HallazgoD23526_ListDuranteEscriturasDeHoomNoAvisa(t *testing.T) {
	root := itRepo(t)
	pedido := strings.Repeat("que el catalogo muestre precios por region y moneda\n", 8000) // ~400 KB
	commit := "3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c"
	at := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

	var mu sync.Mutex
	avisos := map[string]bool{}
	listas := 0
	listo := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-listo:
				return
			default:
			}
			_, warns, err := List(root)
			mu.Lock()
			listas++
			if err != nil {
				avisos["error: "+err.Error()] = true
			}
			for _, w := range warns {
				avisos[w] = true
			}
			mu.Unlock()
		}
	}()
	var errs []string
	for i := 0; i < 8; i++ {
		slug := fmt.Sprintf("escritura-%02d", i)
		if _, err := Add(root, Draft{Titulo: "Escritura", Slug: slug, Pedido: pedido}); err != nil {
			errs = append(errs, fmt.Sprintf("Add %s: %v", slug, err))
			continue
		}
		if _, err := AddSession(root, slug, "claude", at); err != nil {
			errs = append(errs, fmt.Sprintf("AddSession %s: %v", slug, err))
		}
		if _, err := MarkDone(root, slug, commit, at); err != nil {
			errs = append(errs, fmt.Sprintf("MarkDone %s: %v", slug, err))
		}
		if _, err := UnmarkDone(root, slug, commit); err != nil {
			errs = append(errs, fmt.Sprintf("UnmarkDone %s: %v", slug, err))
		}
	}
	close(listo)
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("CA-263: fixture: las escrituras de hoom funcionan: %s", strings.Join(errs, "; "))
	}
	var vistos []string
	for w := range avisos {
		vistos = append(vistos, w)
	}
	sort.Strings(vistos)
	if len(vistos) > 0 {
		t.Fatalf("CA-266: hallazgo d23526: un List que corre durante las escrituras de hoom no avisa por sus temporales (%d listas, %d avisos distintos):\n  %s",
			listas, len(vistos), strings.Join(vistos, "\n  "))
	}
	if _, warns, err := List(root); err != nil || len(warns) != 0 {
		t.Fatalf("CA-266: al terminar las escrituras no queda nada que avisar: %q %v", warns, err)
	}
}
