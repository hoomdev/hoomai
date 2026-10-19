// Tests de regresion de la sexta vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en runcmd. Escritos desde el spec
// (enmienda 4, seccion "Fuente telemetria") y la descripcion de los
// hallazgos, sin leer la implementacion. Los helpers de este archivo llevan
// el prefijo c4v6.
//
// El contrato es el de la enmienda 4 (decision de Henry), que reemplaza a la
// 3: una delegacion cuya linea pasaria de 1 MiB se recorta AL ESCRIBIRLA, no
// al leerla.
//
//   - Escritura: hoom escribe cada evento agent y agent_end de un run en una
//     linea de hasta 1 MiB, sin contar el salto. Si con su detail entero
//     pasaria, el detail queda en sus primeros 128 KiB, sin partir un
//     caracter, seguidos exactamente de
//     " [...] (detalle recortado al escribir la narracion: tenia <n> bytes)",
//     con <n> los bytes del detail original. ts, agente y tool_id quedan
//     enteros, y el run guarda en memoria el mismo evento que escribe.
//   - Lectura: ReadDelegations lee entera, en cualquier orden de claves, cada
//     linea de delegacion de hasta 1 MiB, y salta una mas larga, como salta
//     una linea mal formada o a medio escribir.
//   - Fuera del contrato: una delegacion cuyo agente o tool_id hacen pasar la
//     linea de 1 MiB aun recortada. Estos tests no le exigen nada.
//
// Hallazgos de la delta 2 de C4 que cubren:
//
//   - 20261019T191412_550f79: el tope de 1 MiB contaba el salto de linea; una
//     delegacion de exactamente 1 MiB tomaba la rama recortada.
//   - 20261019T191919_dae2e8: una linea de exactamente 1 MiB salia recortada
//     y sin tool_id, y la historia no podia emparejar su agent_end.
//   - 20261019T191925_7d07ce: una linea de mas de 1 MiB a medio escribir (sin
//     su salto) se volvia un subagente fantasma.
//
// Los runs son de verdad (Manager.Start con un binario falso en el PATH): un
// provider falso registrado en providers.Default que devuelve tal cual el
// evento que trae cada linea (asi un agent o un agent_end puede traer un
// detail de mas de 1 MiB, que el normalizador de Claude no deja pasar: corta
// la descripcion y el texto del fallo), y un claude falso con un subagente
// de nombre largo, que hace pasar de 1 MiB la linea de su agent_end.
package runcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/providers"
)

const (
	c4v6MiB = 1 << 20
	// c4v6Corte es lo que queda del detail de una delegacion recortada al
	// escribirla, antes del aviso.
	c4v6Corte = 128 << 10
)

// c4v6Aviso es lo que sigue al comienzo de un detail recortado al escribirlo.
func c4v6Aviso(n int) string {
	return fmt.Sprintf(" [...] (detalle recortado al escribir la narracion: tenia %d bytes)", n)
}

// c4v6Cabeza es el principio de s de a lo sumo n bytes que no parte un
// caracter (s es UTF-8 valido).
func c4v6Cabeza(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// c4v6Recortado es el detail de una delegacion como lo escribe hoom cuando su
// linea con el detail entero pasaria de 1 MiB.
func c4v6Recortado(orig string) string { return c4v6Cabeza(orig, c4v6Corte) + c4v6Aviso(len(orig)) }

// c4v6Mezcla arma un texto de casi n bytes (sin pasarse) con ASCII, comillas,
// barras y caracteres de 2, 3 y 4 bytes.
func c4v6Mezcla(n int) string {
	piezas := []string{"relleno ", `"comillas" `, "ñandú ", "漢字 ", "🙂 ", `\barra\ `, "é", "x"}
	var b strings.Builder
	for i := 0; b.Len() < n+8; i++ {
		b.WriteString(piezas[i%len(piezas)])
	}
	return c4v6Cabeza(b.String(), n)
}

// c4v6Escapes arma un texto de n bytes crudos que en JSON ocupa al menos el
// triple con cualquier codificador (con o sin escapar <, > y &): caracteres
// de control, comillas, barras, saltos y tabuladores.
func c4v6Escapes(n int) string {
	return strings.Repeat("\x01\x1f\"\\<&\n\t", n/8)
}

// c4v6LargoJSON es el largo de la linea del evento con el codificador de
// json.Marshal y con uno que no escapa <, > y &: el menor y el mayor.
func c4v6LargoJSON(t *testing.T, e Event) (menor, mayor int) {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		t.Fatal(err)
	}
	sin := b.Len() - 1 // sin el salto de Encode
	return min(len(raw), sin), max(len(raw), sin)
}

// El provider falso: cada linea de su salida es un evento en JSON, y
// Normalize lo devuelve tal cual (ts incluido). Una linea que no es un evento
// sale como text.
type c4v6Proveedor struct{}

const c4v6Nombre = "c4v6-falso"

func (c4v6Proveedor) Name() string { return c4v6Nombre }
func (c4v6Proveedor) Bin() string  { return c4v6Nombre }
func (c4v6Proveedor) Capabilities() providers.Capabilities {
	return providers.Capabilities{Structured: true}
}
func (c4v6Proveedor) Command(providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: c4v6Nombre}, nil
}
func (c4v6Proveedor) Normalize(line string) []providers.Event {
	var e providers.Event
	if err := json.Unmarshal([]byte(line), &e); err != nil || e.Kind == "" {
		return []providers.Event{{TS: time.Now().UTC(), Kind: "text", Detail: line}}
	}
	return []providers.Event{e}
}

var c4v6Registro sync.Once

// c4v6Correr hace un run de verdad del provider falso cuya salida son los
// eventos evs, uno por linea, y devuelve root, el id del run y lo que el run
// tiene en memoria (Manager.Events).
func c4v6Correr(t *testing.T, evs []Event) (root, id string, mem []Event) {
	t.Helper()
	var errReg error
	c4v6Registro.Do(func() { errReg = providers.Default.Register(c4v6Proveedor{}) })
	if errReg != nil {
		t.Fatalf("fixture: registrar el provider falso: %v", errReg)
	}
	var salida bytes.Buffer
	for _, e := range evs {
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) >= 4*c4v6MiB-1024 {
			t.Fatalf("fixture: la linea del provider queda debajo de 4 MiB: %d bytes", len(raw))
		}
		salida.Write(raw)
		salida.WriteByte('\n')
	}
	return c4v6CorrerSalida(t, c4v6Nombre, salida.Bytes())
}

// c4v6CorrerSalida hace un run de verdad del provider nombre con un binario
// falso que imprime salida, y devuelve root, el id y lo que el run tiene en
// memoria.
func c4v6CorrerSalida(t *testing.T, nombre string, salida []byte) (root, id string, mem []Event) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "salida.jsonl")
	if err := os.WriteFile(p, salida, 0o644); err != nil {
		t.Fatal(err)
	}
	installFakeNamed(t, nombre, "cat '"+p+"'\nexit 0\n")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(root)
	info, err := m.Start(StartOptions{Provider: nombre, Prompt: "delega"})
	if err != nil {
		t.Fatalf("fixture: el run arranca: %v", err)
	}
	// la salida pesa decenas de MiB: con -race el run tarda unos segundos, y
	// el plazo de waitRun (10 s) queda justo; Wait con un plazo holgado
	listo := make(chan Run, 1)
	go func() { listo <- m.Wait(info.ID) }()
	select {
	case fin := <-listo:
		if fin.Status != StatusDone {
			t.Fatalf("fixture: el run del provider falso termina bien: %+v", fin)
		}
	case <-time.After(2 * time.Minute):
		t.Fatalf("fixture: el run del provider falso termina en 2 minutos")
	}
	_, mem, err = m.Events(info.ID, 0)
	if err != nil {
		t.Fatalf("fixture: Events del run: %v", err)
	}
	return root, info.ID, mem
}

// c4v6Linea es una linea del .jsonl del run tal como esta en el disco, sin
// el salto, con el evento que trae.
type c4v6Linea struct {
	raw []byte
	ev  Event
	ok  bool
}

// c4v6Disco lee el .jsonl del run linea por linea, sin pasar por runcmd.
func c4v6Disco(t *testing.T, root, id string) []c4v6Linea {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", id+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var out []c4v6Linea
	for _, l := range bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n")) {
		var e Event
		err := json.Unmarshal(l, &e)
		out = append(out, c4v6Linea{raw: l, ev: e, ok: err == nil})
	}
	return out
}

func c4v6Resumen(e Event) string {
	d := e.Detail
	if len(d) > 60 {
		d = fmt.Sprintf("%.60s... (%d bytes)", d, len(e.Detail))
	}
	a := e.Agent
	if len(a) > 40 {
		a = fmt.Sprintf("%.40s... (%d bytes)", a, len(e.Agent))
	}
	return fmt.Sprintf("%s %s agent=%q tool_id=%q detail=%q", e.TS.UTC().Format(time.RFC3339Nano), e.Kind, a, e.ToolID, d)
}

func c4v6Listado(ds []Event) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString("  " + c4v6Resumen(d) + "\n")
	}
	return b.String()
}

// c4v6Mismo compara todos los campos de dos eventos.
func c4v6Mismo(a, b Event) bool {
	return c4v4MismoEvento(a, b)
}

// c4v6ExigirDetalle exige el detail de una delegacion escrita por hoom: el
// original entero si no se recorta; si se recorta, un prefijo del original de
// entre 128 KiB - 3 y 128 KiB bytes que no parte un caracter (los primeros
// 128 KiB) seguido del aviso exacto con los bytes del original.
func c4v6ExigirDetalle(t *testing.T, ca string, got, orig Event, recorta bool) {
	t.Helper()
	d := got.Detail
	if !recorta {
		if d != orig.Detail {
			t.Errorf("%s: la delegacion %s entra en 1 MiB: se escribe con su detail entero (%d bytes); se escribio con %d bytes: %.80q",
				ca, c4v6Resumen(orig), len(orig.Detail), len(d), d)
		}
		return
	}
	aviso := c4v6Aviso(len(orig.Detail))
	if !strings.HasSuffix(d, aviso) {
		cola := d
		if len(cola) > 120 {
			cola = cola[len(cola)-120:]
		}
		t.Errorf("%s: el detail recortado de %s termina exactamente con el aviso %q; termina con %q (%d bytes)",
			ca, c4v6Resumen(orig), aviso, cola, len(d))
		return
	}
	pre := strings.TrimSuffix(d, aviso)
	switch want := c4v6Cabeza(orig.Detail, c4v6Corte); {
	case !strings.HasPrefix(orig.Detail, pre):
		t.Errorf("%s: el detail recortado de %s empieza con un prefijo del original: %.80q", ca, c4v6Resumen(orig), pre)
	case !utf8.ValidString(pre):
		t.Errorf("%s: el corte del detail de %s no parte un caracter (%d bytes, termina en % x)",
			ca, c4v6Resumen(orig), len(pre), pre[max(0, len(pre)-4):])
	case len(pre) < c4v6Corte-3 || len(pre) > c4v6Corte:
		t.Errorf("%s: el detail recortado de %s guarda entre %d y %d bytes del original; guardo %d",
			ca, c4v6Resumen(orig), c4v6Corte-3, c4v6Corte, len(pre))
	case pre != want:
		t.Errorf("%s: el detail recortado de %s guarda los primeros 128 KiB sin partir un caracter (%d bytes); guardo %d",
			ca, c4v6Resumen(orig), len(want), len(pre))
	}
}

// c4v6Caso es una delegacion que el provider emite y lo que hoom debe escribir.
type c4v6Caso struct {
	nombre  string
	ev      Event
	recorta bool
}

// c4v6ExigirEscritura exige, para un run cuya salida trajo los eventos de
// casos (mas otros), lo que dice la enmienda 4 de lo que hoom escribe:
//   - cada linea de delegacion del .jsonl mide a lo sumo 1 MiB sin el salto;
//   - cada delegacion de casos esta una vez, con su ts, su agente y su tool_id
//     enteros, y su detail entero o recortado segun el caso;
//   - lo que el run tiene en memoria es lo que escribio, evento por evento;
//   - ReadDelegations trae entera cada delegacion escrita, como ReadEvents.
func c4v6ExigirEscritura(t *testing.T, ca, root, id string, mem []Event, casos []c4v6Caso) {
	t.Helper()
	disco := c4v6Disco(t, root, id)
	var escritas []Event
	var largas []string
	porClave := map[string][]Event{}
	for i, l := range disco {
		if !l.ok {
			t.Errorf("%s: la linea %d del .jsonl es un evento JSON valido: %.120q", ca, i, l.raw)
			continue
		}
		if l.ev.Kind != "agent" && l.ev.Kind != "agent_end" {
			continue
		}
		escritas = append(escritas, l.ev)
		porClave[l.ev.Kind+" "+l.ev.ToolID] = append(porClave[l.ev.Kind+" "+l.ev.ToolID], l.ev)
		if len(l.raw) > c4v6MiB {
			largas = append(largas, fmt.Sprintf("  %s: %d bytes (%d de mas)", c4v6Resumen(l.ev), len(l.raw), len(l.raw)-c4v6MiB))
		}
	}
	if len(largas) > 0 {
		t.Errorf("%s: hoom escribe cada delegacion en una linea de hasta 1 MiB sin el salto; %d de %d pasan de 1 MiB (las primeras):\n%s",
			ca, len(largas), len(escritas), strings.Join(largas[:min(3, len(largas))], "\n"))
	}
	for _, c := range casos {
		got := porClave[c.ev.Kind+" "+c.ev.ToolID]
		cc := ca + ", " + c.nombre
		if len(got) != 1 {
			t.Errorf("%s: la delegacion %s se escribe exactamente una vez; hay %d:\n%s", cc, c4v6Resumen(c.ev), len(got), c4v6Listado(got))
			continue
		}
		g := got[0]
		if !g.TS.Equal(c.ev.TS) || g.Agent != c.ev.Agent || g.ToolID != c.ev.ToolID || g.SessionID != c.ev.SessionID {
			t.Errorf("%s: la delegacion se escribe con su ts, su agente y su tool_id enteros: quiere %s, se escribio %s",
				cc, c4v6Resumen(c.ev), c4v6Resumen(g))
		}
		c4v6ExigirDetalle(t, cc, g, c.ev, c.recorta)
	}

	// lo que el run tiene en memoria es lo que escribio
	if len(mem) != len(disco) {
		t.Errorf("%s: el run guarda en memoria los mismos eventos que escribe: %d en memoria, %d lineas en el .jsonl",
			ca, len(mem), len(disco))
	} else {
		for i := range mem {
			if !c4v6Mismo(mem[i], disco[i].ev) {
				t.Errorf("%s: el run guarda en memoria el mismo evento que escribe (evento %d):\n  memoria %s\n  disco   %s",
					ca, i, c4v6Resumen(mem[i]), c4v6Resumen(disco[i].ev))
			}
		}
	}

	// lo que hoom escribe, la historia lo lee entero
	ds, err := ReadDelegations(root, id)
	if err != nil {
		t.Errorf("%s: ReadDelegations lee el run: %v", ca, err)
		return
	}
	ok := len(ds) == len(escritas)
	for i := 0; ok && i < len(ds); i++ {
		ok = c4v6Mismo(ds[i], escritas[i])
	}
	if !ok {
		t.Errorf("%s: ReadDelegations trae enteras las %d delegaciones que escribio hoom, en orden; trajo %d:\n%squiere:\n%s",
			ca, len(escritas), len(ds), c4v6Listado(ds), c4v6Listado(escritas))
	}
}

// CA-362 (hallazgos 550f79, dae2e8 y 7d07ce; enmienda 4): un run de verdad
// cuyo provider emite delegaciones (cada agent con su agent_end) de todos los
// tamanos. hoom las escribe en lineas de hasta 1 MiB:
//   - un detail de 1.5 MiB (ASCII, comillas, barras y caracteres de 2, 3 y 4
//     bytes) se recorta: sus primeros 128 KiB sin partir un caracter y el
//     aviso exacto con los bytes del original;
//   - un detail de 400 KiB crudos que escapado en JSON pasa de 1 MiB
//     (caracteres de control, comillas, barras, <, &) tambien se recorta;
//   - un caracter de 2, 3 o 4 bytes justo alrededor del corte de 128 KiB (en
//     cada posicion que lo cruza, y terminando o empezando justo en el corte)
//     no se parte: el corte queda antes del caracter;
//   - un detail de 200 KiB cuya linea entra se escribe entero, igual que uno
//     de tamano normal;
//   - un text de 1.5 MiB no es una delegacion: esta regla no lo recorta.
//
// ts, agente y tool_id quedan enteros; lo que el run tiene en memoria
// (Manager.Events) es lo que escribio; y ReadDelegations trae enteras todas
// las delegaciones que escribio.
func TestHallazgo_550f79_dae2e8_7d07ce_CA362_HoomEscribeLaDelegacionEnUnMiB(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 21, 0, 0, 123456789, time.UTC)
	seg := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Second) }
	var casos []c4v6Caso
	var evs []Event
	k := 0
	par := func(nombre, agente, tool, detAgent, detFin string, recorta bool) {
		a := Event{TS: seg(k), Kind: "agent", Agent: agente, ToolID: tool, Detail: detAgent}
		f := Event{TS: seg(k + 1), Kind: "agent_end", Agent: agente, ToolID: tool, Detail: detFin}
		k += 2
		evs = append(evs, a, Event{TS: seg(k), Kind: "text", Detail: "sigo"}, f)
		k++
		casos = append(casos, c4v6Caso{nombre + " (agent)", a, recorta}, c4v6Caso{nombre + " (agent_end)", f, recorta})
	}

	par("delegacion normal", "Explore", "toolu_NORMAL", "Agent: buscar precios", "Explore termino", false)
	par("detail de 1.5 MiB", "Enorme", "toolu_ENORME",
		"Agent: "+c4v6Mezcla(3*c4v6MiB/2), "Enorme fallo: "+c4v6Mezcla(3*c4v6MiB/2), true)
	par("detail de 400 KiB que escapado pasa de 1 MiB", "Escapado", "toolu_ESCAPADO",
		"Agent: "+c4v6Escapes(400<<10), "Escapado fallo: "+c4v6Escapes(400<<10), true)
	par("detail de 200 KiB que entra", "Entra", "toolu_ENTRA",
		"Agent: "+c4v6Mezcla(200<<10), "Entra termino: "+c4v6Mezcla(200<<10), false)
	for _, r := range []string{"ñ", "漢", "🙂"} {
		for inicio := c4v6Corte - len(r); inicio <= c4v6Corte; inicio++ {
			// el caracter r empieza en el byte inicio del detail, en el agent
			// y en el agent_end; el resto lleva la linea apenas por encima de
			// 1 MiB
			runaEn := func(prefijo string) string {
				return prefijo + strings.Repeat("x", inicio-len(prefijo)) + r + c4v6Mezcla(c4v6MiB-c4v6Corte+16<<10)
			}
			detAgent, detFin := runaEn("Agent: "), runaEn("Runas fallo: ")
			corte := c4v6Corte
			if inicio < c4v6Corte && inicio+len(r) > c4v6Corte {
				corte = inicio
			}
			for _, d := range []string{detAgent, detFin} {
				if got := len(c4v6Cabeza(d, c4v6Corte)); got != corte || d[inicio:inicio+len(r)] != r {
					t.Fatalf("fixture: con %q en el byte %d el corte queda en %d, no en %d", r, inicio, got, corte)
				}
			}
			tool := fmt.Sprintf("toolu_RUNA_%d_%d", len(r), c4v6Corte-inicio)
			par(fmt.Sprintf("%q de %d bytes en el byte %d (corte en %d)", r, len(r), inicio, corte), "Runas", tool,
				detAgent, detFin, true)
		}
	}
	texto := Event{TS: seg(k), Kind: "text", Detail: "un text enorme: " + c4v6Mezcla(3*c4v6MiB/2)}
	evs = append(evs, texto, Event{TS: seg(k + 1), Kind: "agent", Agent: "general-purpose", ToolID: "toolu_ULTIMO", Detail: "Agent: sin cierre"})
	casos = append(casos, c4v6Caso{"delegacion sin cierre despues del text enorme", evs[len(evs)-1], false})

	// los casos son los que dicen ser
	for _, c := range casos {
		menor, mayor := c4v6LargoJSON(t, c.ev)
		switch {
		case c.recorta && menor <= c4v6MiB:
			t.Fatalf("fixture: %s: con el detail entero la linea pasa de 1 MiB con cualquier codificador: %d bytes", c.nombre, menor)
		case !c.recorta && mayor > c4v6MiB:
			t.Fatalf("fixture: %s: con el detail entero la linea entra en 1 MiB: %d bytes", c.nombre, mayor)
		case c.recorta && len(c4v6Recortado(c.ev.Detail))*6+len(c.ev.Agent)+len(c.ev.ToolID)+200 > c4v6MiB:
			t.Fatalf("fixture: %s: recortada, la linea entra en 1 MiB (dentro del contrato)", c.nombre)
		}
	}
	if c := casos[4]; len(c.ev.Detail) >= c4v6MiB || c.ev.Agent != "Escapado" {
		t.Fatalf("fixture: el detail que pasa de 1 MiB escapado mide menos de 1 MiB crudo: %d bytes", len(c.ev.Detail))
	}

	root, id, mem := c4v6Correr(t, evs)
	ca := "CA-362 (hallazgos 550f79, dae2e8 y 7d07ce, enmienda 4)"
	c4v6ExigirEscritura(t, ca, root, id, mem, casos)

	// el text enorme no es una delegacion: esta regla no lo recorta
	var textos []Event
	for _, l := range c4v6Disco(t, root, id) {
		if l.ok && l.ev.Kind == "text" && strings.HasPrefix(l.ev.Detail, "un text enorme: ") {
			textos = append(textos, l.ev)
		}
	}
	if len(textos) != 1 || textos[0].Detail != texto.Detail || strings.Contains(textos[0].Detail, "detalle recortado al escribir") {
		t.Errorf("%s: un text de %d bytes no es una delegacion: se escribe entero, sin el aviso del recorte; se escribio %s",
			ca, len(texto.Detail), c4v6Listado(textos))
	}
}

// c4v6Claude es la salida de un claude falso: delega en dos subagentes de
// nombre largo (subagent_type de unos 660 KiB): el primero termina bien y el
// segundo falla.
func c4v6Claude(agenteOK, agenteErr string) []byte {
	toolUse := func(id, agente string) string {
		in, _ := json.Marshal(map[string]any{"description": "mapea el modulo", "prompt": "mapea", "subagent_type": agente, "run_in_background": false})
		return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id +
			`","name":"Agent","input":` + string(in) + `}]},"parent_tool_use_id":null,"session_id":"s-c4v6"}`
	}
	resultado := func(id string, err bool, texto string) string {
		e := ""
		if err {
			e = `,"is_error":true`
		}
		return `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"` + id + `","type":"tool_result","content":"` +
			texto + `"` + e + `}]},"parent_tool_use_id":null,"session_id":"s-c4v6"}`
	}
	return []byte(strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s-c4v6"}`,
		toolUse("toolu_LARGO_OK", agenteOK),
		resultado("toolu_LARGO_OK", false, "el mapa del modulo"),
		toolUse("toolu_LARGO_ERR", agenteErr),
		resultado("toolu_LARGO_ERR", true, "el subagente se quedo sin turnos"),
		`{"type":"result","subtype":"success","result":"ok","num_turns":1}`,
	}, "\n") + "\n")
}

// CA-362 (hallazgos 550f79, dae2e8 y 7d07ce; enmienda 4), con el claude de
// CA-374: un run de Claude que delega en subagentes de nombre largo (unos
// 660 KiB). El agent_end de cada uno ("<agente> termino" y "<agente> fallo:
// ...") pasaria de 1 MiB con su detail entero: hoom lo escribe recortado, con
// el agente y el tool_id enteros; su agent (detail corto) entra y se escribe
// entero. Lo que el run tiene en memoria es lo escrito, y ReadDelegations
// trae las cuatro delegaciones enteras, emparejables por tool_id. El detail
// original es el que da el normalizador de Claude para la misma salida.
func TestHallazgo_550f79_dae2e8_7d07ce_CA362_RunDeClaudeConSubagenteDeNombreLargo(t *testing.T) {
	agenteOK := "hoom-" + strings.Repeat("largo-ñ-", 75000)
	agenteErr := "hoom-" + strings.Repeat("otro-漢-", 68000)
	salida := c4v6Claude(agenteOK, agenteErr)

	// la referencia: el normalizador de Claude de un run, con la misma salida
	p, err := providers.Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := p.(providers.Correlating)
	if !ok {
		t.Fatal("fixture: el provider claude implementa providers.Correlating (CA-373)")
	}
	norm := c.NewNormalizer()
	var ref []Event
	for _, l := range strings.Split(strings.TrimSuffix(string(salida), "\n"), "\n") {
		for _, e := range norm(l) {
			if e.Kind == "agent" || e.Kind == "agent_end" {
				ref = append(ref, e)
			}
		}
	}
	if len(ref) != 4 || ref[0].Agent != agenteOK || ref[1].Kind != "agent_end" || ref[1].ToolID != "toolu_LARGO_OK" ||
		ref[3].Agent != agenteErr || ref[3].ToolID != "toolu_LARGO_ERR" || !strings.HasPrefix(ref[3].Detail, agenteErr+" fallo") {
		t.Fatalf("fixture: el normalizador de Claude da agent y agent_end de cada subagente de nombre largo:\n%s", c4v6Listado(ref))
	}
	var casos []c4v6Caso
	for i, e := range ref {
		menor, mayor := c4v6LargoJSON(t, e)
		recorta := e.Kind == "agent_end"
		if (recorta && menor <= c4v6MiB) || (!recorta && mayor > c4v6MiB) {
			t.Fatalf("fixture: solo la linea del agent_end pasa de 1 MiB con el detail entero: %s mide %d", c4v6Resumen(e), menor)
		}
		casos = append(casos, c4v6Caso{fmt.Sprintf("delegacion %d", i), e, recorta})
	}

	root, id, mem := c4v6CorrerSalida(t, "claude", salida)
	ca := "CA-362 (hallazgos 550f79, dae2e8 y 7d07ce, enmienda 4, run de Claude)"

	// ts: el normalizador de la referencia corrio en otro momento; el del
	// disco es el de la memoria (c4v6ExigirEscritura) y no es cero
	disco := c4v6Disco(t, root, id)
	for i := range casos {
		for _, l := range disco {
			if l.ok && l.ev.Kind == casos[i].ev.Kind && l.ev.ToolID == casos[i].ev.ToolID {
				if l.ev.TS.IsZero() {
					t.Errorf("%s: la delegacion %s se escribe con su ts", ca, c4v6Resumen(l.ev))
				}
				casos[i].ev.TS = l.ev.TS
			}
		}
	}
	c4v6ExigirEscritura(t, ca, root, id, mem, casos)
}

// Los ordenes de las claves de una linea de delegacion escrita a mano.
type c4v6Orden int

const (
	c4v6OrdenHoom    c4v6Orden = iota // como json.Marshal: ts, kind, agent, detail, tool_id
	c4v6OrdenInverso                  // tool_id, detail, agent, kind, ts
)

func (o c4v6Orden) String() string {
	if o == c4v6OrdenInverso {
		return "claves en otro orden"
	}
	return "claves como las escribe hoom"
}

// c4v6Comienzo es el comienzo del detail de las lineas escritas a mano, como
// va en el JSON (con comillas escapadas y caracteres de varios bytes).
const c4v6Comienzo = `Agent: ñandú 漢字 🙂 \"comillas\" `

// c4v6LineaDeLargo arma una linea de delegacion compacta de exactamente
// largo bytes sin el salto, con el detail relleno de ASCII.
func c4v6LineaDeLargo(t *testing.T, largo int, orden c4v6Orden, ts time.Time, kind, agente, tool string) string {
	t.Helper()
	tsS := ts.UTC().Format(time.RFC3339Nano)
	cab := `{"ts":"` + tsS + `","kind":"` + kind + `","agent":"` + agente + `","detail":"` + c4v6Comienzo
	cola := `","tool_id":"` + tool + `"}`
	if orden == c4v6OrdenInverso {
		cab = `{"tool_id":"` + tool + `","detail":"` + c4v6Comienzo
		cola = `","agent":"` + agente + `","kind":"` + kind + `","ts":"` + tsS + `"}`
	}
	n := largo - len(cab) - len(cola)
	if n < 0 {
		t.Fatalf("fixture: una linea de %d bytes no entra la cabeza y la cola", largo)
	}
	relleno := strings.Repeat("abcdefghij", n/10+1)[:n]
	l := cab + relleno + cola
	if len(l) != largo {
		t.Fatalf("fixture: la linea mide %d bytes, no %d", len(l), largo)
	}
	return l
}

// c4v6Log escribe el log del run id con las lineas dadas tal cual; cada una
// lleva su salto salvo que sinSaltoFinal y sea la ultima.
func c4v6Log(t *testing.T, root, id string, sinSaltoFinal bool, lineas ...string) {
	t.Helper()
	p := filepath.Join(root, ".hoom", "runs", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	for i, l := range lineas {
		b.WriteString(l)
		if !sinSaltoFinal || i < len(lineas)-1 {
			b.WriteByte('\n')
		}
	}
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// c4v6JSON es la linea de un evento como la escribe json.Marshal.
func c4v6JSON(t *testing.T, e Event) string {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// c4v6Filtradas es ReadEvents filtrado por agent y agent_end.
func c4v6Filtradas(t *testing.T, root, id string) []Event {
	t.Helper()
	evs, err := ReadEvents(root, id)
	if err != nil {
		t.Fatalf("fixture: ReadEvents lee el log: %v", err)
	}
	var out []Event
	for _, e := range evs {
		if e.Kind == "agent" || e.Kind == "agent_end" {
			out = append(out, e)
		}
	}
	return out
}

func c4v6Iguales(a, b []Event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !c4v6Mismo(a[i], b[i]) {
			return false
		}
	}
	return true
}

// c4v6Vecinas arma un log del run id con una linea de delegacion en el medio
// (borde) entre delegaciones de tamano normal, y devuelve ReadEvents filtrado
// y las delegaciones vecinas.
func c4v6Vecinas(t *testing.T, root, id, borde string) (ref, vecinas []Event) {
	t.Helper()
	t0 := time.Date(2026, 10, 19, 22, 0, 0, 0, time.UTC)
	m := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Minute) }
	vecinas = []Event{
		{TS: m(1), Kind: "agent", Agent: "Explore", ToolID: "toolu_1", Detail: "Agent: antes del borde"},
		{TS: m(3), Kind: "agent_end", Agent: "Explore", ToolID: "toolu_1", Detail: "Explore termino"},
		{TS: m(4), Kind: "agent", Agent: "general-purpose", ToolID: "toolu_3", Detail: "Agent: despues del borde"},
	}
	c4v6Log(t, root, id, false,
		c4v6JSON(t, Event{TS: m(0), Kind: "start", Detail: "init"}),
		c4v6JSON(t, vecinas[0]),
		borde,
		c4v6JSON(t, vecinas[1]),
		c4v6JSON(t, vecinas[2]),
		c4v6JSON(t, Event{TS: m(5), Kind: "end", Detail: "listo"}),
	)
	return c4v6Filtradas(t, root, id), vecinas
}

// CA-362 (hallazgos 550f79 y dae2e8; enmienda 4): una linea de delegacion
// compacta de exactamente 1 MiB (y de 1 MiB - 1) mas su salto se lee ENTERA:
// el tope de 1 MiB no cuenta el salto. ReadDelegations la trae con su
// tool_id, su agente, su ts y su detail iguales a los de ReadEvents, entre
// sus vecinas, para un agent y un agent_end, con las claves como las escribe
// hoom y en otro orden.
func TestHallazgo_550f79_dae2e8_CA362_LineaDeHastaUnMiBSeLeeEntera(t *testing.T) {
	ts := time.Date(2026, 10, 19, 22, 2, 0, 0, time.UTC)
	for _, largo := range []int{c4v6MiB, c4v6MiB - 1} {
		for _, kind := range []string{"agent", "agent_end"} {
			for _, orden := range []c4v6Orden{c4v6OrdenHoom, c4v6OrdenInverso} {
				nombre := fmt.Sprintf("%s de %d bytes, %s", kind, largo, orden)
				t.Run(nombre, func(t *testing.T) {
					ca := "CA-362 (hallazgos 550f79 y dae2e8, " + nombre + ")"
					root := t.TempDir()
					const id = "20261019T220000_borde0"
					ref, vecinas := c4v6Vecinas(t, root, id, c4v6LineaDeLargo(t, largo, orden, ts, kind, "Borde", "toolu_BORDE"))
					if len(ref) != 4 || ref[1].Kind != kind || ref[1].ToolID != "toolu_BORDE" || ref[1].Agent != "Borde" ||
						!ref[1].TS.Equal(ts) || !strings.HasPrefix(ref[1].Detail, "Agent: ñandú 漢字 🙂 \"comillas\" abcdefghij") ||
						!c4v6Mismo(ref[0], vecinas[0]) {
						t.Fatalf("fixture: ReadEvents lee la linea de %d bytes entre sus vecinas:\n%s", largo, c4v6Listado(ref))
					}
					ds, err := ReadDelegations(root, id)
					if err != nil {
						t.Fatalf("%s: ReadDelegations lee el log: %v", ca, err)
					}
					if !c4v6Iguales(ds, ref) {
						t.Errorf("%s: una linea de delegacion de hasta 1 MiB sin contar el salto se lee entera, con su tool_id y su detail, como la lee ReadEvents; trajo %d:\n%squiere:\n%s",
							ca, len(ds), c4v6Listado(ds), c4v6Listado(ref))
					}
				})
			}
		}
	}
}

// CA-362 (hallazgos 550f79 y dae2e8; enmienda 4): una linea de delegacion de
// 1 MiB + 1 (y de 1 MiB + 2 y 3 MiB) no es como la escribe hoom: ReadDelegations
// la salta entera, sin sacar nada de ella (ni recortada ni sin tool_id), y
// trae sus vecinas exactas y en orden. ReadEvents si la entiende: es JSON
// valido, la salta solo la historia.
func TestHallazgo_550f79_dae2e8_CA362_LineaDeMasDeUnMiBSeSalta(t *testing.T) {
	ts := time.Date(2026, 10, 19, 22, 2, 0, 0, time.UTC)
	for _, largo := range []int{c4v6MiB + 1, c4v6MiB + 2, 3 * c4v6MiB} {
		for _, kind := range []string{"agent", "agent_end"} {
			for _, orden := range []c4v6Orden{c4v6OrdenHoom, c4v6OrdenInverso} {
				nombre := fmt.Sprintf("%s de %d bytes, %s", kind, largo, orden)
				t.Run(nombre, func(t *testing.T) {
					ca := "CA-362 (hallazgos 550f79 y dae2e8, " + nombre + ")"
					root := t.TempDir()
					const id = "20261019T220000_borde1"
					ref, vecinas := c4v6Vecinas(t, root, id, c4v6LineaDeLargo(t, largo, orden, ts, kind, "Borde", "toolu_BORDE"))
					if len(ref) != 4 || ref[1].ToolID != "toolu_BORDE" || ref[1].Kind != kind {
						t.Fatalf("fixture: ReadEvents lee la linea de %d bytes (JSON valido):\n%s", largo, c4v6Listado(ref))
					}
					ds, err := ReadDelegations(root, id)
					if err != nil {
						t.Fatalf("%s: ReadDelegations lee el log: %v", ca, err)
					}
					if !c4v6Iguales(ds, vecinas) {
						t.Errorf("%s: una linea de delegacion de mas de 1 MiB se salta sin tapar a sus vecinas: trae solo las %d vecinas, exactas y en orden; trajo %d:\n%squiere:\n%s",
							ca, len(vecinas), len(ds), c4v6Listado(ds), c4v6Listado(vecinas))
					}
				})
			}
		}
	}
}

// CA-362 (hallazgos 550f79 y dae2e8; enmienda 4), la ventana donde el
// refutador vio perderse el tool_id: para cada largo de 1 MiB - 16 a
// 1 MiB + 64 bytes (agent y agent_end, con las claves como las escribe hoom
// y en otro orden, alternados), la linea de hasta 1 MiB sale entera, como la
// lee ReadEvents, y la de mas de 1 MiB no sale; nada sale sin su tool_id, y
// las vecinas salen siempre exactas y en orden.
func TestHallazgo_550f79_dae2e8_CA362_VentanaAlrededorDeUnMiB(t *testing.T) {
	ts := time.Date(2026, 10, 19, 22, 2, 0, 0, time.UTC)
	var malos []string
	for largo := c4v6MiB - 16; largo <= c4v6MiB+64; largo++ {
		kind := []string{"agent", "agent_end"}[largo%2]
		orden := []c4v6Orden{c4v6OrdenHoom, c4v6OrdenInverso}[(largo/2)%2]
		root := t.TempDir()
		id := fmt.Sprintf("20261019T220000_v%05d", largo-c4v6MiB+16)
		ref, vecinas := c4v6Vecinas(t, root, id, c4v6LineaDeLargo(t, largo, orden, ts, kind, "Borde", "toolu_BORDE"))
		if len(ref) != 4 || ref[1].ToolID != "toolu_BORDE" {
			t.Fatalf("fixture: ReadEvents lee la linea de %d bytes:\n%s", largo, c4v6Listado(ref))
		}
		want := vecinas
		if largo <= c4v6MiB {
			want = ref
		}
		ds, err := ReadDelegations(root, id)
		if err != nil {
			t.Fatalf("CA-362: ReadDelegations lee el log de %d bytes: %v", largo, err)
		}
		if !c4v6Iguales(ds, want) {
			que := "se salta"
			if largo <= c4v6MiB {
				que = "sale entera"
			}
			var sinTool []string
			for _, d := range ds {
				if d.ToolID == "" {
					sinTool = append(sinTool, c4v6Resumen(d))
				}
			}
			malos = append(malos, fmt.Sprintf("  %d bytes (%+d), %s, %s: %s; trajo %d delegaciones, %d sin tool_id %v",
				largo, largo-c4v6MiB, kind, orden, que, len(ds), len(sinTool), sinTool))
		}
	}
	if len(malos) > 0 {
		t.Errorf("CA-362 (hallazgos 550f79 y dae2e8): alrededor de 1 MiB la linea de hasta 1 MiB sale entera y la de mas no sale, sin tapar a sus vecinas; fallan %d largos:\n%s",
			len(malos), strings.Join(malos, "\n"))
	}
}

// CA-362 (hallazgo 7d07ce; enmienda 4): un log que termina en una linea de
// delegacion a medio escribir (cortada y sin su salto: un append en curso o
// un write corto) no da ningun evento por ella, y ReadDelegations trae todas
// las anteriores exactas y en orden (una de ellas larga, escrita como hoom:
// detail recortado y linea de hasta 1 MiB). Para un agent y un agent_end
// (este del tool_id de una delegacion anterior abierta, que no debe cerrar),
// de mas de 1 MiB y de menos, cortados en el detail, en medio de un caracter
// de varios bytes, justo antes de la llave final y apenas despues del agente.
func TestHallazgo_7d07ce_CA362_LineaAMedioEscribirNoSale(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 23, 0, 0, 0, time.UTC)
	m := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Minute) }
	largaOrig := "Agent: " + c4v6Mezcla(3*c4v6MiB/2)
	anteriores := []Event{
		{TS: m(1), Kind: "agent", Agent: "Explore", ToolID: "toolu_1", Detail: "Agent: buscar precios"},
		{TS: m(2), Kind: "agent", Agent: "Larga", ToolID: "toolu_LARGA", Detail: c4v6Recortado(largaOrig)},
		{TS: m(3), Kind: "agent_end", Agent: "Explore", ToolID: "toolu_1", Detail: "Explore termino"},
		{TS: m(4), Kind: "agent_end", Agent: "Larga", ToolID: "toolu_LARGA", Detail: c4v6Recortado("Larga fallo: " + largaOrig)},
		{TS: m(5), Kind: "agent", Agent: "Abierta", ToolID: "toolu_ABIERTA", Detail: "Agent: sigue trabajando"},
	}
	for _, e := range anteriores {
		if menor, _ := c4v6LargoJSON(t, e); menor > c4v6MiB {
			t.Fatalf("fixture: la delegacion %s esta escrita como hoom: linea de hasta 1 MiB (%d)", c4v6Resumen(e), menor)
		}
	}
	type corte struct {
		nombre string
		largo  int                    // largo de la linea entera, sin el salto
		hasta  func(linea string) int // donde se corta
	}
	cortes := []corte{
		{"linea de 3 MiB cortada a los 2 MiB", 3 * c4v6MiB, func(string) int { return 2 * c4v6MiB }},
		{"linea de 1.5 MiB cortada en 1 MiB + 10", 3 * c4v6MiB / 2, func(string) int { return c4v6MiB + 10 }},
		{"linea de 1.5 MiB cortada justo antes de la llave final", 3 * c4v6MiB / 2, func(l string) int { return len(l) - 1 }},
		{"linea de 500 KiB cortada a los 400 KiB", 500 << 10, func(string) int { return 400 << 10 }},
		{"linea de 500 KiB cortada en medio de un caracter", 500 << 10, func(l string) int { return strings.Index(l, "🙂") + 2 }},
		{"linea de 300 KiB cortada justo antes de la llave final", 300 << 10, func(l string) int { return len(l) - 1 }},
		{"linea cortada apenas despues del agente", 300 << 10, func(l string) int { return strings.Index(l, `,"detail"`) }},
	}
	for _, kind := range []string{"agent", "agent_end"} {
		for _, c := range cortes {
			nombre := kind + ", " + c.nombre
			t.Run(nombre, func(t *testing.T) {
				ca := "CA-362 (hallazgo 7d07ce, " + nombre + ")"
				agente, tool := "Cortada", "toolu_CORTADA"
				if kind == "agent_end" {
					agente, tool = "Abierta", "toolu_ABIERTA"
				}
				linea := c4v6LineaDeLargo(t, c.largo, c4v6OrdenHoom, m(6), kind, agente, tool)
				hasta := c.hasta(linea)
				if hasta <= 0 || hasta >= len(linea) {
					t.Fatalf("fixture: el corte cae dentro de la linea: %d de %d", hasta, len(linea))
				}
				var lineas []string
				lineas = append(lineas, c4v6JSON(t, Event{TS: m(0), Kind: "start", Detail: "init"}))
				for _, e := range anteriores {
					lineas = append(lineas, c4v6JSON(t, e))
				}
				lineas = append(lineas, linea[:hasta])
				root := t.TempDir()
				const id = "20261019T230000_cortad"
				c4v6Log(t, root, id, true, lineas...)
				if ref := c4v6Filtradas(t, root, id); !c4v6Iguales(ref, anteriores) {
					t.Fatalf("fixture: ReadEvents salta la linea a medio escribir y lee las anteriores:\n%s", c4v6Listado(ref))
				}
				ds, err := ReadDelegations(root, id)
				if err != nil {
					t.Fatalf("%s: ReadDelegations lee el log: %v", ca, err)
				}
				if !c4v6Iguales(ds, anteriores) {
					t.Errorf("%s: la linea a medio escribir del final no da ningun evento (ni un subagente fantasma ni el cierre de %s), y las %d anteriores salen exactas y en orden; trajo %d:\n%squiere:\n%s",
						ca, tool, len(anteriores), len(ds), c4v6Listado(ds), c4v6Listado(anteriores))
				}
			})
		}
	}
}
