// Tests de regresion de los hallazgos de la review cruzada de la cabina
// (spec C1 .hoom/specs/items-y-columna-derivada.md y C3
// .hoom/specs/acciones-desde-la-tarjeta.md), escritos desde el contrato:
//
//   - 20260924T195800_b7cb4a (medium): presupuesto_usd .inf, -.inf o .nan
//     es un item invalido (Parse falla, List lo omite con aviso), y nunca
//     llega a un JSON que no se puede emitir.
//   - 20260924T200412_ff6cf4 (medium): MarkDone y AddSession a la vez sobre
//     el mismo item nunca pierden la escritura del otro.
//   - 20260924T195800_a91dba (medium, real low): crear un item nunca deja un
//     archivo parcial visible con el nombre final; List ignora los ocultos
//     (.algo) de .hoom/items/ sin aviso; un slug existente da ErrExists y
//     el archivo queda igual.
package item

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// h1Visibles lista los nombres NO ocultos de .hoom/items/, ordenados.
func h1Visibles(t *testing.T, root string) []string {
	t.Helper()
	entradas, err := os.ReadDir(Dir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []string
	for _, e := range entradas {
		if !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// h1Avisa dice si algun aviso nombra el archivo.
func h1Avisa(warns []string, archivo string) bool {
	for _, w := range warns {
		if strings.Contains(w, archivo) {
			return true
		}
	}
	return false
}

// h1NoFinitos son las formas YAML de un presupuesto que no es un numero
// finito.
var h1NoFinitos = map[string]string{
	"inf":           ".inf",
	"inf-mas":       "+.inf",
	"inf-mayuscula": ".Inf",
	"inf-grito":     ".INF",
	"menos-inf":     "-.inf",
	"nan":           ".nan",
	"nan-mayuscula": ".NaN",
	"nan-grito":     ".NAN",
}

// Hallazgo 20260924T195800_b7cb4a (medium). CA-266 y CA-264 (presupuesto_usd
// es un numero > 0): un presupuesto infinito o NaN hace invalido al item.
// Guarda probable: -.inf ya es <= 0.
func TestCA266_H1Hallazgo195800b7_ParseRechazaPresupuestoNoFinito(t *testing.T) {
	var nombres []string
	for n := range h1NoFinitos {
		nombres = append(nombres, n)
	}
	sort.Strings(nombres)
	for _, n := range nombres {
		raw := itItemValido("Precios", "2026-09-22T15:04:05Z", "presupuesto_usd: "+h1NoFinitos[n]+"\n")
		it, err := Parse("precios", []byte(raw))
		if err == nil {
			p := "nil"
			if it.PresupuestoUSD != nil {
				p = fmt.Sprint(*it.PresupuestoUSD)
			}
			t.Errorf("CA-266: presupuesto_usd: %s (%s) hace invalido al item; Parse lo acepto con %s", h1NoFinitos[n], n, p)
		}
	}
	// guarda: un presupuesto finito y positivo sigue valiendo
	if it, err := Parse("precios", []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", "presupuesto_usd: 1e300\n"))); err != nil || it.PresupuestoUSD == nil {
		t.Fatalf("CA-266 (guarda): un presupuesto finito grande es valido: %v", err)
	}
}

// Hallazgo 20260924T195800_b7cb4a (medium). CA-266: List omite los items con
// presupuesto no finito con un aviso que nombra el archivo, devuelve los
// validos, y lo que devuelve se puede emitir como JSON (hoy: "json:
// unsupported value: +Inf").
func TestCA266_H1Hallazgo195800b7_ListOmiteElPresupuestoNoFinito(t *testing.T) {
	root := itRepo(t)
	itEscribir(t, root, ".hoom/items/bueno.yaml", itItemValido("Bueno", "2026-09-22T09:00:00Z", "presupuesto_usd: 5\n"))
	itEscribir(t, root, ".hoom/items/otro-bueno.yaml", itItemValido("Otro", "2026-09-22T10:00:00Z", ""))
	for n, v := range h1NoFinitos {
		itEscribir(t, root, ".hoom/items/"+n+".yaml", itItemValido("Roto "+n, "2026-09-22T11:00:00Z", "presupuesto_usd: "+v+"\n"))
	}
	items, warns, err := List(root)
	if err != nil {
		t.Fatalf("CA-266: un item invalido nunca es fatal: %v", err)
	}
	var slugs []string
	for _, it := range items {
		slugs = append(slugs, it.Slug)
	}
	if strings.Join(slugs, ",") != "bueno,otro-bueno" {
		t.Errorf("CA-266: List devuelve solo los validos (bueno,otro-bueno), fue %v", slugs)
	}
	for n := range h1NoFinitos {
		if !h1Avisa(warns, n+".yaml") {
			t.Errorf("CA-266: falta el aviso que nombra %s.yaml (presupuesto_usd: %s): %q", n, h1NoFinitos[n], warns)
		}
	}
	if _, err := json.Marshal(map[string]any{"warnings": warns, "items": items}); err != nil {
		t.Fatalf("CA-266: lo que devuelve List se emite como JSON: %v", err)
	}
}

// Hallazgo 20260924T195800_b7cb4a (medium). CA-264: `--presupuesto-usd` es
// un numero > 0; inf y NaN (que strconv.ParseFloat acepta) no lo son, y un
// rechazo no crea nada. (Extiende el hallazgo a la otra puerta: sin esto,
// `hoom item add --presupuesto-usd inf` escribe un item que List rechaza.)
func TestCA264_H1Hallazgo195800b7_ValidateRechazaPresupuestoNoFinito(t *testing.T) {
	root := itRepo(t)
	for _, v := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		v := v
		if _, err := Validate(Draft{Titulo: "x", PresupuestoUSD: &v}); err == nil {
			t.Errorf("CA-264: Validate rechaza presupuesto_usd %v", v)
		}
		if _, err := Add(root, Draft{Titulo: "Presupuesto raro", PresupuestoUSD: &v}); err == nil {
			t.Errorf("CA-264: Add rechaza presupuesto_usd %v", v)
		}
		if got := h1Visibles(t, root); len(got) != 0 {
			t.Fatalf("CA-264: un presupuesto %v rechazado no crea nada en .hoom/items/: %v", v, got)
		}
	}
}

// Hallazgo 20260924T200412_ff6cf4 (medium). CA-349 y CA-348 (AddSession
// agrega la sesion sin tocar las demas claves) y CA-289 (MarkDone escribe
// hecho_en y commit_final conservando lo demas): corriendo a la vez sobre el
// mismo item, ninguna pierde la escritura de la otra. 100 items nuevos.
func TestCA349_H1Hallazgo200412_MarkDoneYAddSessionConcurrentes(t *testing.T) {
	root := itRepo(t)
	const n = 100
	commit := "3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c"
	at := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	var perdidas []string
	for i := 0; i < n; i++ {
		slug := fmt.Sprintf("carrera-%03d", i)
		itEscribir(t, root, ".hoom/items/"+slug+".yaml", itItemValido("Carrera", "2026-09-22T15:04:05Z", "pedido: correr a la vez\n"))
		var wg sync.WaitGroup
		largada := make(chan struct{})
		var wDone, wSes bool
		var eDone, eSes error
		wg.Add(2)
		go func() { defer wg.Done(); <-largada; wDone, eDone = MarkDone(root, slug, commit, at) }()
		go func() { defer wg.Done(); <-largada; wSes, eSes = AddSession(root, slug, "claude", at) }()
		close(largada)
		wg.Wait()
		if eDone != nil || eSes != nil || !wDone || !wSes {
			perdidas = append(perdidas, fmt.Sprintf("%s: MarkDone=(%v,%v) AddSession=(%v,%v)", slug, wDone, eDone, wSes, eSes))
			continue
		}
		it, err := Load(root, slug)
		if err != nil {
			perdidas = append(perdidas, fmt.Sprintf("%s: el item quedo ilegible: %v", slug, err))
			continue
		}
		var falta []string
		if it.HechoEn == nil || it.CommitFinal != commit {
			falta = append(falta, "hecho_en/commit_final")
		}
		if len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
			falta = append(falta, fmt.Sprintf("la sesion (%d)", len(it.Sesiones)))
		}
		if it.Titulo != "Carrera" || it.Pedido != "correr a la vez" {
			falta = append(falta, "las demas claves")
		}
		if len(falta) > 0 {
			perdidas = append(perdidas, slug+": se perdio "+strings.Join(falta, " y "))
		}
	}
	if len(perdidas) > 0 {
		muestra := perdidas
		if len(muestra) > 5 {
			muestra = muestra[:5]
		}
		t.Fatalf("CA-349: MarkDone y AddSession a la vez no pierden la escritura de la otra: %d de %d items perdieron algo, p. ej.:\n  %s",
			len(perdidas), n, strings.Join(muestra, "\n  "))
	}
}

// Hallazgo 20260924T200412_ff6cf4 (medium). CA-348 y CA-349: dos AddSession a
// la vez (dos sesiones abiertas en el mismo instante) dejan las dos
// entradas. (Extiende el hallazgo: la misma lectura-modificacion-escritura.)
func TestCA348_H1Hallazgo200412_DosAddSessionConcurrentes(t *testing.T) {
	root := itRepo(t)
	const n = 100
	at := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	perdidas := 0
	var ejemplo string
	for i := 0; i < n; i++ {
		slug := fmt.Sprintf("sesiones-%03d", i)
		itEscribir(t, root, ".hoom/items/"+slug+".yaml", itItemValido("Sesiones", "2026-09-22T15:04:05Z", ""))
		var wg sync.WaitGroup
		largada := make(chan struct{})
		for _, p := range []string{"claude", "codex"} {
			wg.Add(1)
			go func(p string) { defer wg.Done(); <-largada; _, _ = AddSession(root, slug, p, at) }(p)
		}
		close(largada)
		wg.Wait()
		it, err := Load(root, slug)
		if err != nil || len(it.Sesiones) != 2 {
			perdidas++
			if ejemplo == "" {
				ejemplo = fmt.Sprintf("%s: %d sesiones, err %v", slug, len(it.Sesiones), err)
			}
		}
	}
	if perdidas > 0 {
		t.Fatalf("CA-348: dos sesiones abiertas a la vez quedan las dos: %d de %d items perdieron una (p. ej. %s)", perdidas, n, ejemplo)
	}
}

// Hallazgo 20260924T195800_a91dba. CA-266 (List avisa por cada archivo que
// no es un item) y CA-263 (las escrituras son atomicas: sus temporales son
// ocultos): los archivos ocultos de .hoom/items/ se ignoran SIN aviso, sea
// cual sea su nombre o contenido. Guarda: los no ocultos que no son items
// siguen avisando.
func TestCA266_H1Hallazgoa91dba_ListIgnoraOcultosSinAviso(t *testing.T) {
	root := itRepo(t)
	itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z", ""))
	ocultos := map[string]string{
		".algo.yaml.tmp":             "titulo: [a medio escribir",
		".precios.yaml.tmp123456":    itItemValido("Precios", "2026-09-22T15:04:05Z", "")[:20],
		".slug.lock":                 "",
		".precios.lock":              "12345\n",
		".oculto.yaml":               itItemValido("Oculto", "2026-09-22T15:04:05Z", ""),
		".DS_Store":                  "\x00\x01basura",
		".tmp-precios.yaml-98765432": "",
	}
	for nombre, cuerpo := range ocultos {
		itEscribir(t, root, ".hoom/items/"+nombre, cuerpo)
	}
	items, warns, err := List(root)
	if err != nil {
		t.Fatalf("CA-266: List no falla por archivos ocultos: %v", err)
	}
	if len(items) != 1 || items[0].Slug != "precios" {
		t.Errorf("CA-266: un oculto nunca es un item: %+v", items)
	}
	if len(warns) != 0 {
		t.Errorf("CA-266: los archivos ocultos de .hoom/items/ se ignoran sin aviso, hubo %d: %q", len(warns), warns)
	}

	// guarda: lo que no es oculto y no es un item sigue avisando
	itEscribir(t, root, ".hoom/items/x.yml", itItemValido("X", "2026-09-22T10:00:00Z", ""))
	itEscribir(t, root, ".hoom/items/Foo Bar.yaml", itItemValido("Foo", "2026-09-22T10:00:00Z", ""))
	_, warns, err = List(root)
	if err != nil || !h1Avisa(warns, "x.yml") || !h1Avisa(warns, "Foo Bar.yaml") {
		t.Fatalf("CA-266 (guarda): x.yml y 'Foo Bar.yaml' siguen avisando: %q %v", warns, err)
	}
}

// Hallazgo 20260924T195800_a91dba. CA-263 (todas las escrituras son
// atomicas): mientras Add crea un item, quien lee .hoom/items/<slug>.yaml ve
// que no existe o un item COMPLETO y valido; nunca un archivo vacio ni a
// medio escribir con el nombre final. Un pedido grande agranda la ventana.
func TestCA263_H1Hallazgoa91dba_AddNuncaDejaUnParcialVisible(t *testing.T) {
	root := itRepo(t)
	pedido := strings.Repeat("que el catalogo muestre precios por region y moneda\n", 20000) // ~1 MB
	const n = 60
	parciales := 0
	var ejemplo string
	for i := 0; i < n; i++ {
		slug := fmt.Sprintf("parcial-%03d", i)
		path := Path(root, slug)
		listo := make(chan struct{})
		visto := make(chan string, 1)
		go func() {
			for {
				select {
				case <-listo:
					visto <- ""
					return
				default:
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				if _, perr := Parse(slug, raw); perr != nil {
					visto <- fmt.Sprintf("%d bytes visibles con el nombre final: %v", len(raw), perr)
					return
				}
			}
		}()
		_, err := Add(root, Draft{Titulo: "Parcial", Slug: slug, Pedido: pedido})
		close(listo)
		v := <-visto
		if err != nil {
			t.Fatalf("CA-263: Add %s: %v", slug, err)
		}
		if v != "" {
			parciales++
			if ejemplo == "" {
				ejemplo = slug + ": " + v
			}
		}
	}
	if parciales > 0 {
		t.Fatalf("CA-263: crear un item nunca deja un parcial visible con el nombre final: %d de %d altas lo dejaron ver (p. ej. %s)", parciales, n, ejemplo)
	}
}

// GUARDA (verde en la base). Hallazgo 20260924T195800_a91dba. CA-265: con el
// archivo ya ahi (aunque sea un item invalido, y con temporales ocultos al
// lado) Add envuelve ErrExists y el archivo queda byte a byte igual; no
// aparece ningun archivo visible nuevo.
func TestCA265_H1Hallazgoa91dba_AddExistenteNoCambiaNada(t *testing.T) {
	root := itRepo(t)
	for nombre, cuerpo := range map[string]string{
		"precios": itItemValido("Precios", "2026-09-22T15:04:05Z", "pedido: el original\n"),
		"roto":    "titulo: [no es un item valido\n",
		"vacio":   "",
	} {
		path := itEscribir(t, root, ".hoom/items/"+nombre+".yaml", cuerpo)
		itEscribir(t, root, ".hoom/items/."+nombre+".yaml.tmp", "resto de otra escritura")
		antes := h1Visibles(t, root)
		_, err := Add(root, Draft{Titulo: "Otro titulo", Slug: nombre, Pedido: "el nuevo"})
		if !errors.Is(err, ErrExists) {
			t.Fatalf("CA-265: el slug %q ya existe: Add envuelve ErrExists, fue %v", nombre, err)
		}
		if raw, _ := os.ReadFile(path); !bytes.Equal(raw, []byte(cuerpo)) {
			t.Fatalf("CA-265: el item %q queda byte a byte igual:\n%s", nombre, raw)
		}
		if despues := h1Visibles(t, root); strings.Join(despues, ",") != strings.Join(antes, ",") {
			t.Fatalf("CA-265: un alta rechazada no deja archivos visibles: %v -> %v", antes, despues)
		}
	}
}

// GUARDA (verde en la base). Hallazgo 20260924T195800_a91dba. CA-265 y
// CA-263: ocho Add a la vez con el mismo slug: exactamente uno crea el item,
// los demas reciben ErrExists, y el archivo final es el item COMPLETO del
// ganador (nunca una mezcla ni uno pisado por otro).
func TestCA265_H1Hallazgoa91dba_AddConcurrenteMismoSlug(t *testing.T) {
	root := itRepo(t)
	for ronda := 0; ronda < 20; ronda++ {
		slug := fmt.Sprintf("mismo-%02d", ronda)
		const k = 8
		var wg sync.WaitGroup
		largada := make(chan struct{})
		errs := make([]error, k)
		for i := 0; i < k; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-largada
				_, errs[i] = Add(root, Draft{Titulo: fmt.Sprintf("Titulo %d", i), Slug: slug, Pedido: strings.Repeat("x", 4096*(i+1))})
			}(i)
		}
		close(largada)
		wg.Wait()
		ganador := -1
		for i, err := range errs {
			switch {
			case err == nil && ganador == -1:
				ganador = i
			case err == nil:
				t.Fatalf("CA-265: dos Add con el mismo slug %q crearon el item (%d y %d)", slug, ganador, i)
			case !errors.Is(err, ErrExists):
				t.Fatalf("CA-265: el perdedor recibe ErrExists, fue %v", err)
			}
		}
		if ganador == -1 {
			t.Fatalf("CA-265: uno de los Add con el slug %q crea el item: %v", slug, errs)
		}
		it, err := Load(root, slug)
		if err != nil || it.Titulo != fmt.Sprintf("Titulo %d", ganador) || len(it.Pedido) != 4096*(ganador+1) {
			t.Fatalf("CA-265: el archivo final es el item completo del ganador %d: titulo %q, pedido %d bytes, err %v",
				ganador, it.Titulo, len(it.Pedido), err)
		}
	}
	if got := h1Visibles(t, root); len(got) != 20 {
		t.Fatalf("CA-263: quedan solo los 20 items visibles, sin restos: %v", got)
	}
}
