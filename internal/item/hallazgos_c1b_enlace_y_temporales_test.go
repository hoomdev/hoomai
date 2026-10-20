//go:build !windows

// Tests de regresion de la review de los fixes de C1 sobre el item
// (.hoom/specs/items-y-columna-derivada.md):
//
//   - 20261020T202439_d2c506 (medium, resilience; CA-263 y CA-265): item.Add
//     publica el item con la variable de paquete enlazar (os.Link). En un
//     volumen que permite crear y renombrar archivos pero no hard links
//     (exFAT, vfat, ciertos mounts de red) el enlace falla con "no
//     soportado" (ENOTSUP/EOPNOTSUPP; en linux, link(2) devuelve EPERM
//     cuando el filesystem no admite hard links) y hoy Add devuelve ese
//     error: ningun item se puede crear. Add tiene que crear igual el item,
//     completo y legible por List, sin dejar temporales. Y con el enlace no
//     soportado sigue valiendo CA-265: si el item ya existe, ErrExists y el
//     archivo queda byte a byte igual; dos Add del mismo slug a la vez: uno
//     gana, el otro recibe ErrExists, y el archivo es el del ganador entero.
//
//   - 20261020T202732_6b1f72 (low, readability; CA-266): los temporales de
//     hoom en .hoom/items/ tienen la forma EXACTA "." + slug valido +
//     ".yaml.tmp-" + uno o mas digitos (asi los escriben item.Add y
//     hoomfs.AtomicWrite). List calla solo esos; cualquier otro archivo que
//     empiece con punto y contenga ".yaml.tmp-" avisa, como cualquier
//     archivo que no es un item. Hoy List calla todo lo que empieza con "."
//     y contiene ".yaml.tmp-".
//
// Estos tests cambian la variable de paquete enlazar: ninguno corre en
// paralelo.
package item

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/quick"
)

// c1bNoSoportados son los errores con que link(2) dice que el filesystem no
// admite hard links.
var c1bNoSoportados = []struct {
	nombre string
	errno  syscall.Errno
}{
	{"ENOTSUP", syscall.ENOTSUP},
	{"EOPNOTSUPP", syscall.EOPNOTSUPP},
	{"EPERM (linux: el filesystem no admite hard links)", syscall.EPERM},
}

// c1bSinEnlaces reemplaza enlazar por uno que falla como en un volumen sin
// hard links, y lo restaura al terminar el test. Devuelve cuantas veces se
// llamo.
func c1bSinEnlaces(t *testing.T, errno syscall.Errno) *int {
	t.Helper()
	orig := enlazar
	t.Cleanup(func() { enlazar = orig })
	var mu sync.Mutex
	llamadas := new(int)
	enlazar = func(viejo, nuevo string) error {
		mu.Lock()
		*llamadas++
		mu.Unlock()
		return &os.LinkError{Op: "link", Old: viejo, New: nuevo, Err: errno}
	}
	return llamadas
}

// c1bEntradas lista TODO lo que hay en .hoom/items/ (ocultos incluidos),
// ordenado.
func c1bEntradas(t *testing.T, root string) []string {
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
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// c1bSoloItems exige que en .hoom/items/ queden exactamente esos items y
// nada mas: ningun temporal ni parcial.
func c1bSoloItems(t *testing.T, ca, root string, slugs ...string) {
	t.Helper()
	var want []string
	for _, s := range slugs {
		want = append(want, s+".yaml")
	}
	sort.Strings(want)
	if got := c1bEntradas(t, root); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%s: en .hoom/items/ quedan solo %v, sin temporales: %v", ca, want, got)
	}
}

// Hallazgo 20261020T202439_d2c506. CA-263 y CA-264: con un enlace que el
// filesystem no soporta, Add crea igual el item: completo (con un pedido
// grande), legible por Load y por List sin avisos, y sin temporales.
func TestCA263_HallazgoD2c506_AddSinHardLinksCreaElItem(t *testing.T) {
	// ~200 KB con saltos de linea adentro y sin espacios en los bordes: Add
	// recorta los bordes del pedido por cualquier camino (regla vieja del
	// item), y aca se exige que llegue entero, byte a byte.
	pedido := strings.Repeat("que el catalogo muestre precios por region y moneda\n", 4000) + "fin del pedido"
	presupuesto := 7.5
	for _, c := range c1bNoSoportados {
		t.Run(c.nombre, func(t *testing.T) {
			root := itRepo(t)
			c1bSinEnlaces(t, c.errno)
			d := Draft{Titulo: "Precios por region", Tipo: "bug", Prioridad: "alta", Pedido: pedido, PresupuestoUSD: &presupuesto}
			it, err := Add(root, d)
			if err != nil {
				t.Fatalf("CA-263: hallazgo d2c506: en un volumen sin hard links (%s) Add crea igual el item: %v", c.nombre, err)
			}
			if it.Slug != "precios-por-region" {
				t.Fatalf("CA-263: el slug sale del titulo: %q", it.Slug)
			}
			got, err := Load(root, it.Slug)
			if err != nil {
				t.Fatalf("CA-263: hallazgo d2c506: el item creado sin hard links se lee: %v", err)
			}
			if got.Titulo != d.Titulo || got.Tipo != "bug" || got.Prioridad != "alta" || got.Pedido != pedido ||
				got.PresupuestoUSD == nil || *got.PresupuestoUSD != presupuesto || got.CreadoPor == "" || got.CreadoEn.IsZero() {
				t.Fatalf("CA-264: hallazgo d2c506: el item queda completo: titulo %q tipo %q prioridad %q pedido %d/%d bytes presupuesto %v creado %q %v",
					got.Titulo, got.Tipo, got.Prioridad, len(got.Pedido), len(pedido), got.PresupuestoUSD, got.CreadoPor, got.CreadoEn)
			}
			items, warns, err := List(root)
			if err != nil || len(warns) != 0 || len(items) != 1 || items[0].Slug != it.Slug || items[0].Pedido != pedido {
				t.Fatalf("CA-266: hallazgo d2c506: List lee el item sin avisos: %d items, avisos %q, err %v", len(items), warns, err)
			}
			c1bSoloItems(t, "CA-263: hallazgo d2c506", root, it.Slug)
		})
	}
}

// GUARDA (verde en la base). Hallazgo 20261020T202439_d2c506. CA-265: con el
// item ya creado, Add devuelve ErrExists por el camino normal (hard links
// soportados), el archivo queda byte a byte igual y no quedan temporales.
func TestCA265_HallazgoD2c506_AddExistenteConEnlaceNormal(t *testing.T) {
	root := itRepo(t)
	path := itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z", "pedido: el original\n"))
	antes, _ := os.ReadFile(path)
	_, err := Add(root, Draft{Titulo: "Otro", Slug: "precios", Pedido: "el nuevo"})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("CA-265: el item ya existe: Add envuelve ErrExists, fue %v", err)
	}
	if raw, _ := os.ReadFile(path); !bytes.Equal(raw, antes) {
		t.Fatalf("CA-265: el item existente queda byte a byte igual:\n%s", raw)
	}
	c1bSoloItems(t, "CA-265", root, "precios")
}

// Hallazgo 20261020T202439_d2c506. CA-265: con el enlace no soportado, un
// Add sobre un item que ya existe tambien devuelve ErrExists (no el error
// del enlace), deja el archivo byte a byte igual y no deja temporales.
func TestCA265_HallazgoD2c506_AddExistenteSinHardLinks(t *testing.T) {
	for _, c := range c1bNoSoportados {
		t.Run(c.nombre, func(t *testing.T) {
			root := itRepo(t)
			path := itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z", "pedido: el original\n"))
			antes, _ := os.ReadFile(path)
			c1bSinEnlaces(t, c.errno)
			_, err := Add(root, Draft{Titulo: "Otro", Slug: "precios", Pedido: "el nuevo"})
			if !errors.Is(err, ErrExists) {
				t.Fatalf("CA-265: hallazgo d2c506: sin hard links (%s), el item ya existe: Add envuelve ErrExists, fue %v", c.nombre, err)
			}
			if raw, _ := os.ReadFile(path); !bytes.Equal(raw, antes) {
				t.Fatalf("CA-265: hallazgo d2c506: el item existente queda byte a byte igual:\n%s", raw)
			}
			c1bSoloItems(t, "CA-265: hallazgo d2c506", root, "precios")
		})
	}
}

// Hallazgo 20261020T202439_d2c506. CA-265 y CA-263: con el enlace no
// soportado, dos Add del mismo slug a la vez: exactamente uno crea el item,
// el otro recibe ErrExists, y el archivo es el item COMPLETO del ganador
// (nunca uno pisado por el otro ni una mezcla). Sin temporales al final.
func TestCA265_HallazgoD2c506_DosAddALaVezSinHardLinks(t *testing.T) {
	for _, c := range c1bNoSoportados {
		t.Run(c.nombre, func(t *testing.T) {
			root := itRepo(t)
			c1bSinEnlaces(t, c.errno)
			const rondas = 25
			var slugs []string
			for ronda := 0; ronda < rondas; ronda++ {
				slug := fmt.Sprintf("mismo-%02d", ronda)
				slugs = append(slugs, slug)
				var wg sync.WaitGroup
				largada := make(chan struct{})
				errs := make([]error, 2)
				for i := 0; i < 2; i++ {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						<-largada
						_, errs[i] = Add(root, Draft{Titulo: fmt.Sprintf("Titulo %d", i), Slug: slug,
							Pedido: strings.Repeat(string(rune('a'+i)), 64<<10*(i+1))})
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
						t.Fatalf("CA-265: hallazgo d2c506: sin hard links, dos Add con el slug %q crearon el item a la vez", slug)
					case !errors.Is(err, ErrExists):
						t.Fatalf("CA-265: hallazgo d2c506: sin hard links, el perdedor recibe ErrExists, fue %v", err)
					}
				}
				if ganador == -1 {
					t.Fatalf("CA-265: hallazgo d2c506: sin hard links, uno de los dos Add con el slug %q crea el item: %v", slug, errs)
				}
				it, err := Load(root, slug)
				want := strings.Repeat(string(rune('a'+ganador)), 64<<10*(ganador+1))
				if err != nil || it.Titulo != fmt.Sprintf("Titulo %d", ganador) || it.Pedido != want {
					t.Fatalf("CA-265: hallazgo d2c506: el archivo es el item entero del ganador %d: titulo %q, pedido %d bytes (esperaba %d), err %v",
						ganador, it.Titulo, len(it.Pedido), len(want), err)
				}
			}
			c1bSoloItems(t, "CA-263: hallazgo d2c506", root, slugs...)
		})
	}
}

// c1bAvisa dice si algun aviso nombra EXACTAMENTE el archivo nombre de
// .hoom/items/: la ruta completa seguida de ":" (o del final del aviso), asi
// un nombre que es prefijo de otro (.precios.yaml.tmp-0 de
// .precios.yaml.tmp-0x1f) no se da por avisado con el aviso del otro.
func c1bAvisa(warns []string, nombre string) bool {
	ruta := ".hoom/items/" + nombre
	for _, w := range warns {
		for i := strings.Index(w, ruta); i >= 0; {
			fin := i + len(ruta)
			if fin == len(w) || w[fin] == ':' {
				return true
			}
			j := strings.Index(w[i+1:], ruta)
			if j < 0 {
				break
			}
			i += 1 + j
		}
	}
	return false
}

// c1bTemporalExacto es la forma del contrato: "." + slug valido +
// ".yaml.tmp-" + uno o mas digitos ASCII. La particion es unica: los digitos
// van al final y ".yaml.tmp-" no contiene digitos.
func c1bTemporalExacto(nombre string) bool {
	const marca = ".yaml.tmp-"
	if !strings.HasPrefix(nombre, ".") {
		return false
	}
	i := strings.LastIndex(nombre, marca)
	if i < 0 {
		return false
	}
	slug, digitos := nombre[1:i], nombre[i+len(marca):]
	if digitos == "" {
		return false
	}
	for _, r := range digitos {
		if r < '0' || r > '9' {
			return false
		}
	}
	return ValidSlug(slug)
}

// Hallazgo 20261020T202732_6b1f72. CA-266: los nombres que empiezan con "." y
// contienen ".yaml.tmp-" pero no son un temporal de hoom (el slug no es
// valido, o lo que sigue no son digitos) se omiten con un aviso que los
// nombra, y nunca son items. Los temporales exactos se callan.
func TestCA266_Hallazgo6b1f72_ListAvisaPorLoQueNoEsUnTemporalExacto(t *testing.T) {
	root := itRepo(t)
	itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z", ""))
	parcial := itItemValido("Precios", "2026-09-22T15:04:05Z", "")[:25]
	conAviso := []string{
		".notas.yaml.tmp-respaldo.yaml", // los tres del hallazgo
		".NO SLUG.yaml.tmp-x",
		".x.json.yaml.tmp-",
		".precios.yaml.tmp-",                                   // sin digitos
		".precios.yaml.tmp-12a",                                // digitos y algo mas
		".precios.yaml.tmp-123.yaml",                           // digitos y una extension
		".precios.yaml.tmp- 124",                               // espacio antes de los digitos
		".precios.yaml.tmp-+125",                               // signo
		".precios.yaml.tmp-0x1f",                               // hexadecimal
		".precios.yaml.tmp-１２",                                 // digitos de ancho completo (unicode)
		"..yaml.tmp-126",                                       // slug vacio
		".Precios.yaml.tmp-127",                                // slug con mayuscula
		".pre_cios.yaml.tmp-128",                               // slug con guion bajo
		".pre cios.yaml.tmp-129",                               // slug con espacio
		".a.yaml.tmp-1.yaml.tmp-2",                             // el "slug" seria a.yaml.tmp-1
		"." + strings.Repeat("a", MaxSlug+1) + ".yaml.tmp-130", // slug de mas de MaxSlug
	}
	temporales := []string{
		".mi-item.yaml.tmp-123456", // la guarda del hallazgo
		".precios.yaml.tmp-0",
		".sin-item.yaml.tmp-00000000000000000000131",         // muchos digitos, ceros a la izquierda
		"." + strings.Repeat("b", MaxSlug) + ".yaml.tmp-132", // slug de MaxSlug justo
	}
	for _, n := range conAviso {
		if c1bTemporalExacto(n) {
			t.Fatalf("fixture: %q no deberia tener la forma de un temporal de hoom", n)
		}
		itEscribir(t, root, ".hoom/items/"+n, parcial)
	}
	for _, n := range temporales {
		if !c1bTemporalExacto(n) {
			t.Fatalf("fixture: %q deberia tener la forma de un temporal de hoom", n)
		}
		itEscribir(t, root, ".hoom/items/"+n, parcial)
	}
	items, warns, err := List(root)
	if err != nil {
		t.Fatalf("CA-266: un archivo que no es item nunca es fatal: %v", err)
	}
	if len(items) != 1 || items[0].Slug != "precios" {
		t.Errorf("CA-266: solo precios.yaml es un item: %+v", items)
	}
	for _, n := range conAviso {
		if !c1bAvisa(warns, n) {
			t.Errorf("CA-266: hallazgo 6b1f72: %q no es un temporal de hoom (\".\" + slug + \".yaml.tmp-\" + digitos): se omite con un aviso que lo nombra; avisos %q", n, warns)
		}
	}
	for _, n := range temporales {
		if c1bAvisa(warns, n) {
			t.Errorf("CA-266: hallazgo 6b1f72: %q es un temporal de hoom: se calla; avisos %q", n, warns)
		}
	}
}

// Hallazgo 20261020T202732_6b1f72. CA-266, como propiedad: para cualquier
// nombre "." + P + ".yaml.tmp-" + S, List calla si P es un slug valido y S
// son uno o mas digitos ASCII, y avisa nombrandolo en cualquier otro caso.
// Nunca es un item.
func TestCA266_Hallazgo6b1f72_PropiedadSoloLaFormaExactaSeCalla(t *testing.T) {
	root := itRepo(t)
	const alfabetoSlug = "abcdef0123456789-abcdef0123456789-X_. " // mayormente valido
	const alfabetoSufijo = "0123456789012345678901234567890123456789a-.x"
	var fallas []string
	callados, avisados := 0, 0
	prop := func(rawSlug, rawSufijo []byte) bool {
		slug := c1cDesde(rawSlug, alfabetoSlug, 12)
		sufijo := "" // de 0 a 7 caracteres
		if len(rawSufijo) > 0 && rawSufijo[0]%8 != 0 {
			sufijo = c1cDesde(rawSufijo[1:], alfabetoSufijo, int(rawSufijo[0]%8))
		}
		nombre := "." + slug + ".yaml.tmp-" + sufijo
		callar := c1bTemporalExacto(nombre)
		c1cResetItems(t, root)
		itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z", ""))
		itEscribir(t, root, ".hoom/items/"+nombre, "titulo: [a medio escribir")
		items, warns, err := List(root)
		switch {
		case err != nil:
			fallas = append(fallas, fmt.Sprintf("%q: List fallo: %v", nombre, err))
		case len(items) != 1 || items[0].Slug != "precios":
			fallas = append(fallas, fmt.Sprintf("%q: aparecieron items de mas: %d", nombre, len(items)))
		case callar && c1bAvisa(warns, nombre):
			fallas = append(fallas, fmt.Sprintf("el temporal exacto %q avisa (avisos %q)", nombre, warns))
		case !callar && !c1bAvisa(warns, nombre):
			fallas = append(fallas, fmt.Sprintf("%q no es un temporal exacto y no avisa (avisos %q)", nombre, warns))
		default:
			if callar {
				callados++
			} else {
				avisados++
			}
			return true
		}
		return false
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 300}); err != nil {
		muestra := fallas
		if len(muestra) > 6 {
			muestra = muestra[:6]
		}
		t.Fatalf("CA-266: hallazgo 6b1f72: List calla solo la forma exacta de los temporales de hoom:\n  %s\n(%v)",
			strings.Join(muestra, "\n  "), err)
	}
	if callados == 0 || avisados == 0 {
		t.Fatalf("fixture: la propiedad tiene que probar los dos lados: %d callados, %d avisados", callados, avisados)
	}
}
