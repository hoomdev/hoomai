// Tests adversariales del spec .hoom/specs/studio-tu-siguiente-paso.md
// (CA-722, la parte HTTP): /api/board trae pelota en cada tarjeta y en el
// tablero, la card de /api/board/{slug} trae la misma pelota, nunca null, y
// /api/board sigue dando los mismos bytes que hoom board --json. Sobre el
// JSON que sirve el Studio se comprueban tambien las tablas del contrato que
// se pueden leer en la misma tarjeta (grupo, paso, frase, camino, orden, caja
// y cuenta: CA-723..CA-728), con tres proyectos: el tablero de C1
// (tbTablero), el de las tarjetas en curso (runningtest) y uno vacio.
package servecmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/runningtest"
)

// Los cinco grupos, en el orden de la lista del tablero v3.
var stGrupos = []string{"TE TOCA", "CORRIENDO", "DETENIDA", "EN COLA", "HECHO"}

// Los nombres del camino, uno por columna de boardcmd.Columns.
var stNombresCamino = []string{"Pedido", "Spec", "Tu firma", "Tests", "Código", "Review", "Tu aceptación", "Hecho"}

type stPasoJSON struct {
	Accion   string `json:"accion"`
	Etiqueta string `json:"etiqueta"`
	Texto    string `json:"texto"`
}

type stLineaJSON struct {
	Frase string `json:"frase"`
	Gris  string `json:"gris"`
}

type stEntradaJSON struct {
	Nombre string `json:"nombre"`
	Estado string `json:"estado"`
	Texto  string `json:"texto"`
}

type stPelotaJSON struct {
	Grupo  string          `json:"grupo"`
	Paso   stPasoJSON      `json:"paso"`
	Frase  string          `json:"frase"`
	Ahora  stLineaJSON     `json:"ahora"`
	Sigue  stLineaJSON     `json:"sigue"`
	Camino []stEntradaJSON `json:"camino"`
}

type stCardJSON struct {
	Slug       string `json:"slug"`
	Column     string `json:"column"`
	ColumnName string `json:"column_name"`
	Item       struct {
		Titulo string `json:"titulo"`
	} `json:"item"`
	Running *struct {
		Role string `json:"role"`
	} `json:"running"`
	Interrupted *struct {
		Role string `json:"role"`
	} `json:"interrupted"`
	Red *struct {
		Reason   string `json:"reason"`
		Esperado bool   `json:"esperado"`
	} `json:"red"`
	Actions []struct {
		ID      string `json:"id"`
		Label   string `json:"label"`
		Enabled bool   `json:"enabled"`
		Why     string `json:"why"`
	} `json:"actions"`
	Pelota json.RawMessage `json:"pelota"`
}

type stBoardJSON struct {
	Columns []struct {
		ID    string       `json:"id"`
		Cards []stCardJSON `json:"cards"`
	} `json:"columns"`
	Warnings []string        `json:"warnings"`
	Pelota   json.RawMessage `json:"pelota"`
}

type stPelotaBoardJSON struct {
	Orden  []string       `json:"orden"`
	Caja   string         `json:"caja"`
	Cuenta map[string]int `json:"cuenta"`
}

// stTarjetaRevisada es una tarjeta del JSON con su pelota ya revisada.
type stTarjetaRevisada struct {
	card    stCardJSON
	pelota  stPelotaJSON
	columna int // indice de su columna en boardcmd.Columns
	enCol   int // posicion dentro de su columna en el JSON del tablero
}

func stEsCadena(raw json.RawMessage) bool {
	r := bytes.TrimSpace(raw)
	return len(r) > 0 && r[0] == '"'
}

func stObjeto(t *testing.T, ca, donde string, raw json.RawMessage, claves ...string) map[string]json.RawMessage {
	t.Helper()
	if r := bytes.TrimSpace(raw); len(r) == 0 || string(r) == "null" {
		t.Fatalf("%s: %s esta presente y nunca es null: %s", ca, donde, raw)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: %s es un objeto JSON: %v\n%s", ca, donde, err, raw)
	}
	for _, k := range claves {
		v, ok := m[k]
		if !ok {
			t.Fatalf("%s: a %s le falta %q: %s", ca, donde, k, raw)
		}
		if string(bytes.TrimSpace(v)) == "null" {
			t.Fatalf("%s: %s.%s nunca es null: %s", ca, donde, k, raw)
		}
	}
	return m
}

func stCadenas(t *testing.T, ca, donde string, m map[string]json.RawMessage, claves ...string) {
	t.Helper()
	for _, k := range claves {
		if !stEsCadena(m[k]) {
			t.Fatalf("%s: %s.%s es una cadena: %s", ca, donde, k, m[k])
		}
	}
}

// stRevisarForma exige la forma de pelota del contrato (CA-722): grupo,
// paso (accion, etiqueta, texto), frase, ahora (frase, gris), sigue (frase,
// gris) y camino, con 8 entradas (nombre, estado, texto), todo presente y
// sin null.
func stRevisarForma(t *testing.T, ca, donde string, raw json.RawMessage) stPelotaJSON {
	t.Helper()
	m := stObjeto(t, ca, donde+".pelota", raw, "grupo", "paso", "frase", "ahora", "sigue", "camino")
	stCadenas(t, ca, donde+".pelota", m, "grupo", "frase")
	paso := stObjeto(t, ca, donde+".pelota.paso", m["paso"], "accion", "etiqueta", "texto")
	stCadenas(t, ca, donde+".pelota.paso", paso, "accion", "etiqueta", "texto")
	for _, k := range []string{"ahora", "sigue"} {
		l := stObjeto(t, ca, donde+".pelota."+k, m[k], "frase", "gris")
		stCadenas(t, ca, donde+".pelota."+k, l, "frase", "gris")
	}
	var camino []json.RawMessage
	if err := json.Unmarshal(m["camino"], &camino); err != nil {
		t.Fatalf("%s: %s.pelota.camino es una lista: %v\n%s", ca, donde, err, m["camino"])
	}
	if len(camino) != 8 {
		t.Fatalf("CA-727: %s.pelota.camino tiene siempre 8 entradas, tiene %d", donde, len(camino))
	}
	for i, e := range camino {
		em := stObjeto(t, ca, donde+".pelota.camino[]", e, "nombre", "estado", "texto")
		stCadenas(t, ca, donde+".pelota.camino["+stNombresCamino[i]+"]", em, "nombre", "estado", "texto")
	}
	var p stPelotaJSON
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func stColumna(id string) int {
	for i, c := range boardcmd.Columns {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func stRango(grupo string) int {
	for i, g := range stGrupos {
		if g == grupo {
			return i
		}
	}
	return -1
}

func stTitulo(c stCardJSON) string {
	titulo := strings.Join(strings.Fields(c.Item.Titulo), " ")
	if titulo == "" {
		return c.Slug
	}
	return titulo
}

// stFraseTeToca es la tabla de frases del contrato para TE TOCA (CA-725).
func stFraseTeToca(accion, titulo string) string {
	q := "«" + titulo + "»"
	switch accion {
	case boardcmd.ActAprobar:
		return "Firmá el spec de " + q + "."
	case boardcmd.ActIntegrar:
		return "Aceptá " + q + "."
	case boardcmd.ActPedirArquitecto:
		return "Pedile el spec al arquitecto para " + q + "."
	case boardcmd.ActPedirTestWriter:
		return "Pedile los tests al test-writer para " + q + "."
	case boardcmd.ActPedirWriter:
		return "Pedile el código al writer para " + q + "."
	case boardcmd.ActPedirReviewer:
		return "Pedile la review al reviewer para " + q + "."
	case boardcmd.ActReanudar:
		return "Reanudá el trabajo de " + q + "."
	case boardcmd.ActGuardar:
		return "Guardá el trabajo de " + q + "."
	}
	return "Mirá " + q + "."
}

// stRevisarTarjeta exige, sobre una tarjeta del JSON que sirve el Studio, las
// reglas del contrato que se leen en la misma tarjeta.
func stRevisarTarjeta(t *testing.T, donde string, c stCardJSON, p stPelotaJSON) {
	t.Helper()
	// CA-723: la tabla de grupos, en su orden
	quiere := "TE TOCA"
	switch {
	case c.Column == boardcmd.ColHecho:
		quiere = "HECHO"
	case c.Running != nil:
		quiere = "CORRIENDO"
	case c.Interrupted != nil || (c.Red != nil && !c.Red.Esperado):
		quiere = "DETENIDA"
	case c.Column == boardcmd.ColBacklog:
		quiere = "EN COLA"
	}
	if p.Grupo != quiere {
		t.Fatalf("CA-723: %s (columna %s, running=%v, interrupted=%v, red=%v) esta en %q, no en %q",
			donde, c.Column, c.Running != nil, c.Interrupted != nil, c.Red != nil, quiere, p.Grupo)
	}

	// CA-724: con paso, la accion es una accion habilitada de la tarjeta, con
	// su label y sin texto; sin paso, etiqueta vacia y el texto dice por que
	if p.Paso.Accion != "" {
		hay := false
		for _, a := range c.Actions {
			if a.ID == p.Paso.Accion {
				hay = true
				if !a.Enabled {
					t.Fatalf("CA-724: %s: paso.accion %q es una accion HABILITADA de la tarjeta (why %q)", donde, a.ID, a.Why)
				}
				if p.Paso.Etiqueta != a.Label {
					t.Fatalf("CA-724: %s: paso.etiqueta es el label de la accion (%q), es %q", donde, a.Label, p.Paso.Etiqueta)
				}
			}
		}
		if !hay {
			t.Fatalf("CA-724: %s: paso.accion %q es una accion de la tarjeta: %+v", donde, p.Paso.Accion, c.Actions)
		}
		if p.Paso.Texto != "" {
			t.Fatalf("CA-724: %s: con paso habilitado el texto es \"\", es %q", donde, p.Paso.Texto)
		}
	} else {
		if p.Paso.Etiqueta != "" || p.Paso.Texto == "" {
			t.Fatalf("CA-724: %s: sin paso, etiqueta es \"\" y texto dice por que: %+v", donde, p.Paso)
		}
	}
	switch {
	case c.Column == boardcmd.ColHecho:
		if p.Paso.Accion != "" || p.Paso.Texto != "la tarjeta ya esta en Hecho" {
			t.Fatalf("CA-724: %s: en Hecho no hay paso, con \"la tarjeta ya esta en Hecho\": %+v", donde, p.Paso)
		}
	case c.Running != nil:
		if p.Paso.Accion != "" {
			t.Fatalf("CA-724: %s: con running no hay paso: %+v", donde, p.Paso)
		}
		if c.Running.Role != "" && p.Paso.Texto != "el "+c.Running.Role+" esta trabajando: espera a que termine" {
			t.Fatalf("CA-724: %s: con running el texto es %q, es %q", donde,
				"el "+c.Running.Role+" esta trabajando: espera a que termine", p.Paso.Texto)
		}
	case c.Interrupted != nil:
		if p.Paso.Accion != "" || p.Paso.Texto != "el trabajo se corto: reanudalo o pedilo de nuevo" {
			t.Fatalf("CA-724: %s: con interrupted no hay paso, con \"el trabajo se corto: reanudalo o pedilo de nuevo\": %+v", donde, p.Paso)
		}
	case c.Column == boardcmd.ColTuAprobacion || c.Column == boardcmd.ColTuAceptacion:
		elegida, minuscula := boardcmd.ActAprobar, "aprobar spec"
		if c.Column == boardcmd.ColTuAceptacion {
			elegida, minuscula = boardcmd.ActIntegrar, "integrar"
		}
		var tiene, habilitada bool
		var why string
		for _, a := range c.Actions {
			if a.ID == elegida {
				tiene, habilitada, why = true, a.Enabled, a.Why
			}
		}
		switch {
		case tiene && habilitada:
			if p.Paso.Accion != elegida {
				t.Fatalf("CA-724: %s: en %s el paso es %q: %+v", donde, c.ColumnName, elegida, p.Paso)
			}
		case tiene:
			if p.Paso.Accion != "" || p.Paso.Texto != why {
				t.Fatalf("CA-724: %s: con %q deshabilitada, sin paso y su why (%q): %+v", donde, elegida, why, p.Paso)
			}
		default:
			if w := "en " + c.ColumnName + " no hay " + minuscula; p.Paso.Accion != "" || p.Paso.Texto != w {
				t.Fatalf("CA-724: %s: sin la accion %q, sin paso y %q: %+v", donde, elegida, w, p.Paso)
			}
		}
	}

	// CA-725: la frase de la caja
	fraseQuiere := ""
	switch p.Grupo {
	case "DETENIDA":
		fraseQuiere = "Decidí cómo sigue «" + stTitulo(c) + "»."
	case "TE TOCA":
		fraseQuiere = stFraseTeToca(p.Paso.Accion, stTitulo(c))
	}
	if p.Frase != fraseQuiere {
		t.Fatalf("CA-725: %s (%s, paso %q): la frase es %q, es %q", donde, p.Grupo, p.Paso.Accion, fraseQuiere, p.Frase)
	}

	// CA-726: con interrupted, gris es "ya no corre"; con un rojo no esperado
	// (y sin running ni interrupted), gris es red.reason
	if p.Grupo == "DETENIDA" && c.Interrupted != nil && p.Ahora.Gris != "ya no corre" {
		t.Fatalf("CA-726: %s: con interrupted, ahora.gris es \"ya no corre\", es %q", donde, p.Ahora.Gris)
	}
	if p.Grupo == "DETENIDA" && c.Interrupted == nil && c.Red != nil && p.Ahora.Gris != c.Red.Reason {
		t.Fatalf("CA-726: %s: con un rojo no esperado, ahora.gris es red.reason (%q), es %q", donde, c.Red.Reason, p.Ahora.Gris)
	}

	// CA-727: el camino
	col := stColumna(c.Column)
	if col < 0 {
		t.Fatalf("CA-727: %s: columna desconocida %q", donde, c.Column)
	}
	textoActual := map[string]string{"TE TOCA": "te toca a vos", "DETENIDA": "se detuvo", "EN COLA": "sin empezar"}
	if c.Running != nil && c.Running.Role != "" {
		textoActual["CORRIENDO"] = c.Running.Role + ", ahora"
	}
	for i, e := range p.Camino {
		if e.Nombre != stNombresCamino[i] {
			t.Fatalf("CA-727: %s: la entrada %d del camino es %q, es %q", donde, i+1, stNombresCamino[i], e.Nombre)
		}
		estado := "falta"
		switch {
		case p.Grupo == "HECHO" || i < col:
			estado = "hecho"
		case i == col:
			estado = "actual"
		}
		if e.Estado != estado {
			t.Fatalf("CA-727: %s (columna %s): la entrada %q esta %q, esta %q", donde, c.Column, e.Nombre, estado, e.Estado)
		}
		if estado != "actual" && e.Texto != "" {
			t.Fatalf("CA-727: %s: texto solo en la entrada actual: %q tiene %q", donde, e.Nombre, e.Texto)
		}
		if estado == "actual" {
			if w, ok := textoActual[p.Grupo]; ok && e.Texto != w {
				t.Fatalf("CA-727: %s: la entrada actual (%s, %s) dice %q, dice %q", donde, e.Nombre, p.Grupo, w, e.Texto)
			}
		}
	}
}

// stRevisarTablero lee /api/board, exige la forma de pelota en cada tarjeta
// y en el tablero, y las reglas que se leen en el mismo JSON. Devuelve las
// tarjetas por slug y la pelota del tablero.
func stRevisarTablero(t *testing.T, ca string, raw []byte) (map[string]stTarjetaRevisada, stPelotaBoardJSON) {
	t.Helper()
	var b stBoardJSON
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("%s: /api/board es JSON: %v", ca, err)
	}
	tarjetas := map[string]stTarjetaRevisada{}
	for _, col := range b.Columns {
		for k, c := range col.Cards {
			p := stRevisarForma(t, ca, "la tarjeta "+c.Slug+" de /api/board", c.Pelota)
			if stRango(p.Grupo) < 0 {
				t.Fatalf("CA-723: el grupo de %s es uno de %v: %q", c.Slug, stGrupos, p.Grupo)
			}
			stRevisarTarjeta(t, "la tarjeta "+c.Slug+" de /api/board", c, p)
			tarjetas[c.Slug] = stTarjetaRevisada{card: c, pelota: p, columna: stColumna(c.Column), enCol: k}
		}
	}

	// el tablero: orden, caja y cuenta (CA-728)
	m := stObjeto(t, ca, "/api/board.pelota", b.Pelota, "orden", "caja", "cuenta")
	if r := bytes.TrimSpace(m["orden"]); len(r) == 0 || r[0] != '[' {
		t.Fatalf("CA-728: pelota.orden es una lista (nunca null): %s", m["orden"])
	}
	if !stEsCadena(m["caja"]) {
		t.Fatalf("CA-728: pelota.caja es una cadena: %s", m["caja"])
	}
	cuenta := stObjeto(t, ca, "/api/board.pelota.cuenta", m["cuenta"], "te_toca", "corriendo", "detenida", "en_cola", "hecho")
	for k, v := range cuenta {
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			t.Fatalf("CA-728: pelota.cuenta.%s es un numero: %s", k, v)
		}
	}
	var pb stPelotaBoardJSON
	if err := json.Unmarshal(b.Pelota, &pb); err != nil {
		t.Fatal(err)
	}

	// orden: todas las tarjetas, una vez cada una, y ningun warning
	if len(pb.Orden) != len(tarjetas) {
		t.Fatalf("CA-728: pelota.orden tiene todas las tarjetas (%d): %v", len(tarjetas), pb.Orden)
	}
	vistas := map[string]bool{}
	for _, s := range pb.Orden {
		if _, ok := tarjetas[s]; !ok || vistas[s] {
			t.Fatalf("CA-728: pelota.orden son los slugs de las tarjetas, una vez cada uno: %v", pb.Orden)
		}
		vistas[s] = true
	}
	for _, w := range b.Warnings {
		for _, s := range pb.Orden {
			if strings.Contains(w, ".hoom/items/"+s+".yaml") {
				t.Fatalf("CA-728: un item invalido (%q) no entra en orden: %v", w, pb.Orden)
			}
		}
	}
	// el orden de la lista: TE TOCA (de la columna mas cerca de Hecho a la
	// mas lejos), CORRIENDO, DETENIDA, EN COLA y HECHO, y dentro de cada
	// columna el orden del tablero
	for i := 1; i < len(pb.Orden); i++ {
		a, z := tarjetas[pb.Orden[i-1]], tarjetas[pb.Orden[i]]
		ra, rz := stRango(a.pelota.Grupo), stRango(z.pelota.Grupo)
		if ra > rz {
			t.Fatalf("CA-728: en orden, %s (%s) va antes que %s (%s): %v", z.card.Slug, z.pelota.Grupo, a.card.Slug, a.pelota.Grupo, pb.Orden)
		}
		if ra == rz && a.pelota.Grupo == "TE TOCA" && a.columna < z.columna {
			t.Fatalf("CA-728: TE TOCA va de la columna mas cerca de Hecho a la mas lejos: %s (%s) antes que %s (%s): %v",
				z.card.Slug, z.card.Column, a.card.Slug, a.card.Column, pb.Orden)
		}
		if ra == rz && a.pelota.Grupo != "TE TOCA" && a.columna > z.columna {
			t.Fatalf("CA-728: en %s van en el orden del tablero (por columna, de Backlog a Hecho): %s (%s) antes que %s (%s): %v",
				a.pelota.Grupo, z.card.Slug, z.card.Column, a.card.Slug, a.card.Column, pb.Orden)
		}
		if ra == rz && a.columna == z.columna && a.enCol > z.enCol {
			t.Fatalf("CA-728: dentro de la columna %s, el orden del tablero: %s antes que %s: %v", a.card.Column, z.card.Slug, a.card.Slug, pb.Orden)
		}
	}

	// caja: la primera de TE TOCA, si no la primera de DETENIDA, si no ""
	caja := ""
	for _, g := range []string{"TE TOCA", "DETENIDA"} {
		for _, s := range pb.Orden {
			if caja == "" && tarjetas[s].pelota.Grupo == g {
				caja = s
			}
		}
	}
	if pb.Caja != caja {
		t.Fatalf("CA-728: pelota.caja es %q, es %q (orden %v)", caja, pb.Caja, pb.Orden)
	}

	// cuenta: cuantas tarjetas tiene cada grupo
	clave := map[string]string{"TE TOCA": "te_toca", "CORRIENDO": "corriendo", "DETENIDA": "detenida", "EN COLA": "en_cola", "HECHO": "hecho"}
	quiere := map[string]int{"te_toca": 0, "corriendo": 0, "detenida": 0, "en_cola": 0, "hecho": 0}
	for _, tr := range tarjetas {
		quiere[clave[tr.pelota.Grupo]]++
	}
	for k, n := range quiere {
		if pb.Cuenta[k] != n {
			t.Fatalf("CA-728: pelota.cuenta.%s es %d, es %d (%v)", k, n, pb.Cuenta[k], pb.Cuenta)
		}
	}
	return tarjetas, pb
}

func stMismosBytesQueElCLI(t *testing.T, ca, dir string, rec []byte, blockOn string) {
	t.Helper()
	b, err := boardcmd.Build(dir, "main", blockOn, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	cli, err := boardcmd.JSONBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(rec), bytes.TrimSpace(cli)) {
		t.Fatalf("%s: /api/board son los bytes de hoom board --json:\napi: %s\ncli: %s", ca, rec, cli)
	}
	if !bytes.Contains(cli, []byte(`"pelota"`)) {
		t.Fatalf("%s: hoom board --json trae pelota: %.300s...", ca, cli)
	}
}

// CA-722: /api/board trae pelota en cada tarjeta y en el tablero, con la
// forma del contrato, y siguen siendo los bytes de hoom board --json. Sobre
// el tablero de C1 (una tarjeta por columna y un item invalido, CA-266): los
// grupos (CA-723), el orden, la caja y la cuenta (CA-728) son los del
// contrato.
func TestCA722_ApiBoardTraePelota(t *testing.T) {
	dir, _ := tbTablero(t)
	s := newServer(t, dir)
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-722: GET /api/board responde 200: %d %s", rec.Code, rec.Body.String())
	}
	stMismosBytesQueElCLI(t, "CA-722", dir, rec.Body.Bytes(), tbBlockOn)
	tarjetas, pb := stRevisarTablero(t, "CA-722", rec.Body.Bytes())

	// CA-723: el grupo de cada fixture de C1
	grupos := map[string]string{
		"sin-spec": "EN COLA", "cerrada": "HECHO",
		"incompleto": "TE TOCA", "sin-firma": "TE TOCA", "sin-tests": "TE TOCA",
		"sin-veredicto": "TE TOCA", "en-review": "TE TOCA", "lista": "TE TOCA",
	}
	for slug, g := range grupos {
		tr, ok := tarjetas[slug]
		if !ok {
			t.Fatalf("CA-722: la tarjeta %s esta en /api/board", slug)
		}
		if tr.pelota.Grupo != g {
			t.Fatalf("CA-723: %s (columna %s) esta en %s, esta en %s", slug, tr.card.Column, g, tr.pelota.Grupo)
		}
	}
	// CA-728: TE TOCA de tu-aceptacion a arquitecto, despues EN COLA y HECHO
	orden := []string{"lista", "en-review", "sin-veredicto", "sin-tests", "sin-firma", "incompleto", "sin-spec", "cerrada"}
	if strings.Join(pb.Orden, ",") != strings.Join(orden, ",") {
		t.Fatalf("CA-728: pelota.orden es %v, es %v", orden, pb.Orden)
	}
	if pb.Caja != "lista" {
		t.Fatalf("CA-728: la caja es la primera de TE TOCA (lista), es %q", pb.Caja)
	}
	cuenta := map[string]int{"te_toca": 6, "corriendo": 0, "detenida": 0, "en_cola": 1, "hecho": 1}
	for k, n := range cuenta {
		if pb.Cuenta[k] != n {
			t.Fatalf("CA-728: cuenta.%s es %d, es %d", k, n, pb.Cuenta[k])
		}
	}
	for _, s := range pb.Orden {
		if s == "malo" {
			t.Fatalf("CA-728: el item invalido no entra en orden: %v", pb.Orden)
		}
	}
	// CA-725: en EN COLA y en HECHO la frase es ""; CA-727: en HECHO las 8 son hecho
	if f := tarjetas["sin-spec"].pelota.Frase; f != "" {
		t.Fatalf("CA-725: en EN COLA la frase es \"\", es %q", f)
	}
	for _, e := range tarjetas["cerrada"].pelota.Camino {
		if e.Estado != "hecho" {
			t.Fatalf("CA-727: en HECHO las 8 entradas son hecho: %+v", tarjetas["cerrada"].pelota.Camino)
		}
	}
}

// CA-722: la card de /api/board/{slug} trae pelota, nunca null, con la forma
// del contrato, y es la misma pelota que la tarjeta tiene en /api/board.
func TestCA722_ApiBoardSlugTraePelota(t *testing.T) {
	dir, _ := tbTablero(t)
	s := newServer(t, dir)
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-722: GET /api/board responde 200: %d %s", rec.Code, rec.Body.String())
	}
	tarjetas, _ := stRevisarTablero(t, "CA-722", rec.Body.Bytes())
	if len(tarjetas) == 0 {
		t.Fatal("CA-722: fixture: el tablero de C1 tiene tarjetas")
	}
	for slug, tr := range tarjetas {
		rec := tbGET(t, s, "/api/board/"+slug)
		if rec.Code != http.StatusOK {
			t.Fatalf("CA-722: GET /api/board/%s responde 200: %d %s", slug, rec.Code, rec.Body.String())
		}
		var d struct {
			Card stCardJSON `json:"card"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
			t.Fatalf("CA-722: el detalle de %s es JSON: %v", slug, err)
		}
		p := stRevisarForma(t, "CA-722", "la card de /api/board/"+slug, d.Card.Pelota)
		stRevisarTarjeta(t, "la card de /api/board/"+slug, d.Card, p)
		var a, b any
		_ = json.Unmarshal(d.Card.Pelota, &a)
		_ = json.Unmarshal(tr.card.Pelota, &b)
		sa, _ := json.Marshal(a)
		sb, _ := json.Marshal(b)
		if string(sa) != string(sb) {
			t.Fatalf("CA-722: la pelota de la card de /api/board/%s es la de /api/board:\ndetalle: %s\ntablero: %s", slug, sa, sb)
		}
	}
}

// CA-722: con tarjetas en curso (runningtest: un sobre en run, un run suelto
// y un sobre sin run, vivos), /api/board y /api/board/{slug} traen pelota y
// los mismos bytes que el CLI; las tres estan en CORRIENDO (CA-723), sin
// paso (CA-724), con la frase vacia (CA-725), y como no hay TE TOCA ni
// DETENIDA la caja es "" (CA-728).
func TestCA722_ApiBoardConTarjetasEnCurso(t *testing.T) {
	root, _ := runningtest.Proyecto(t)
	s := newServer(t, root)
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-722: GET /api/board responde 200: %d %s", rec.Code, rec.Body.String())
	}
	// el blockOn por defecto del manifiesto de runningtest
	b, err := boardcmd.Build(root, "main", s.m.FindingsBlockOn(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	cli, err := boardcmd.JSONBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(cli, []byte(`"pelota"`)) {
		t.Fatalf("CA-722: hoom board --json trae pelota: %.300s...", cli)
	}
	tarjetas, pb := stRevisarTablero(t, "CA-722", rec.Body.Bytes())
	for _, slug := range []string{"con-sobre", "suelto", "sin-run"} {
		tr, ok := tarjetas[slug]
		if !ok {
			t.Fatalf("CA-722: la tarjeta %s esta en /api/board", slug)
		}
		if tr.pelota.Grupo != "CORRIENDO" {
			t.Fatalf("CA-723: %s tiene running: esta en CORRIENDO, esta en %s", slug, tr.pelota.Grupo)
		}
		if tr.pelota.Paso.Accion != "" || tr.pelota.Frase != "" {
			t.Fatalf("CA-724/CA-725: %s corre: sin paso y con la frase vacia: %+v", slug, tr.pelota)
		}
		rec := tbGET(t, s, "/api/board/"+slug)
		if rec.Code != http.StatusOK {
			t.Fatalf("CA-722: GET /api/board/%s responde 200: %d %s", slug, rec.Code, rec.Body.String())
		}
		var d struct {
			Card stCardJSON `json:"card"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		stRevisarForma(t, "CA-722", "la card de /api/board/"+slug, d.Card.Pelota)
	}
	if pb.Caja != "" || pb.Cuenta["corriendo"] != 3 {
		t.Fatalf("CA-728: tres en CORRIENDO y ninguna TE TOCA ni DETENIDA: caja \"\" y corriendo 3: %+v", pb)
	}
}

// CA-722 (caso limite "un tablero sin tarjetas"): sin items, /api/board trae
// pelota con orden [], caja "" y la cuenta con sus cinco ceros (CA-728), sin
// ningun null, y los mismos bytes que el CLI.
func TestCA722_ApiBoardVacioTraePelota(t *testing.T) {
	dir := tbProyecto(t)
	s := newServer(t, dir)
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-722: GET /api/board sin items responde 200: %d %s", rec.Code, rec.Body.String())
	}
	stMismosBytesQueElCLI(t, "CA-722", dir, rec.Body.Bytes(), tbBlockOn)
	_, pb := stRevisarTablero(t, "CA-722", rec.Body.Bytes())
	var crudo struct {
		Pelota struct {
			Orden json.RawMessage `json:"orden"`
		} `json:"pelota"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &crudo); err != nil {
		t.Fatal(err)
	}
	if string(bytes.Join(bytes.Fields(crudo.Pelota.Orden), nil)) != "[]" {
		t.Fatalf("CA-728: un tablero vacio da orden: [], da %s", crudo.Pelota.Orden)
	}
	if pb.Caja != "" || len(pb.Cuenta) != 5 {
		t.Fatalf("CA-728: un tablero vacio da caja \"\" y la cuenta con sus cinco claves: %+v", pb)
	}
	for k, n := range pb.Cuenta {
		if n != 0 {
			t.Fatalf("CA-728: un tablero vacio da cinco ceros: cuenta.%s = %d", k, n)
		}
	}
	if strings.Contains(rec.Body.String(), "null") {
		t.Fatalf("CA-722: sin items ningun campo es null: %s", rec.Body.String())
	}
}

// CA-742 (caso limite "el hash #tarjeta/ de un slug que existia y ya no"): un
// item que se borra deja de tener tarjeta: /api/board/{slug} pasa de 200 a
// 404 JSON, que es lo que la pantalla Tarjeta convierte en "no existe la
// tarjeta «<slug>»".
func TestCA742_TarjetaBorradaEs404(t *testing.T) {
	dir := tbProyecto(t)
	tbItem(t, dir, "se-va", "")
	s := newServer(t, dir)
	if rec := tbGET(t, s, "/api/board/se-va"); rec.Code != http.StatusOK {
		t.Fatalf("CA-742: fixture: GET /api/board/se-va responde 200: %d %s", rec.Code, rec.Body.String())
	}
	if err := os.Remove(filepath.Join(dir, ".hoom", "items", "se-va.yaml")); err != nil {
		t.Fatal(err)
	}
	tbErrorJSON(t, "CA-742", tbGET(t, s, "/api/board/se-va"), http.StatusNotFound)
}
