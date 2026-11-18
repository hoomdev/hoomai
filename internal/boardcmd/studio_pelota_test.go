// Tests adversariales del spec .hoom/specs/studio-tu-siguiente-paso.md en
// internal/boardcmd: la regla de quien tiene la pelota, que se muda a
// boardcmd (CA-722..CA-727). Derive la calcula para cada tarjeta y la deja en
// el JSON como "pelota", siempre presente y nunca null.
//
// Escritos desde los dos specs (studio-tu-siguiente-paso.md y, para los
// textos, tablero-tu-siguiente-paso.md: "La caja TU SIGUIENTE PASO", "Los
// grupos", "El camino", "El rojo esperado", "El paso de una tarjeta" y "El
// detalle"), las firmas publicas y los fixtures de los tests de hoy (bdEv,
// acEv, acColumnas, piEv, piTarjetas, tspEv...), sin leer la implementacion.
//
// La pelota se lee SIEMPRE por JSON ("pelota" y sus claves), nunca por los
// nombres Go de sus campos: el spec fija Card.Pelota y los nombres JSON, no
// los nombres Go de los subcampos. Hoy la tarjeta no trae "pelota", asi que
// estos tests fallan en ejecucion con un mensaje que lo dice.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------------------
// La pelota tal como la trae el JSON de la tarjeta (contrato del spec).
// ---------------------------------------------------------------------------

type spPaso struct {
	Accion   string `json:"accion"`
	Etiqueta string `json:"etiqueta"`
	Texto    string `json:"texto"`
}

type spFG struct {
	Frase string `json:"frase"`
	Gris  string `json:"gris"`
}

type spEntrada struct {
	Nombre string `json:"nombre"`
	Estado string `json:"estado"`
	Texto  string `json:"texto"`
}

type spPelota struct {
	Grupo  string      `json:"grupo"`
	Paso   spPaso      `json:"paso"`
	Frase  string      `json:"frase"`
	Ahora  spFG        `json:"ahora"`
	Sigue  spFG        `json:"sigue"`
	Camino []spEntrada `json:"camino"`
}

// Los grupos del contrato, en el orden de su tabla.
const (
	spHecho     = "HECHO"
	spCorriendo = "CORRIENDO"
	spDetenida  = "DETENIDA"
	spEnCola    = "EN COLA"
	spTeToca    = "TE TOCA"
)

// spNombres son los nombres del camino, uno por columna de boardcmd.Columns,
// en su orden (los del tablero v3, "El camino").
var spNombres = []string{"Pedido", "Spec", "Tu firma", "Tests", "Código", "Review", "Tu aceptación", "Hecho"}

// spTipo es el tipo JSON de un valor crudo.
func spTipo(raw json.RawMessage) string {
	s := bytes.TrimSpace(raw)
	if len(s) == 0 {
		return "vacio"
	}
	switch s[0] {
	case '"':
		return "string"
	case '{':
		return "object"
	case '[':
		return "array"
	case 'n':
		return "null"
	case 't', 'f':
		return "bool"
	}
	return "number"
}

// spCampos exige que obj tenga cada clave con su tipo JSON.
func spCampos(t *testing.T, ca, nombre, donde string, obj map[string]json.RawMessage, quiero map[string]string) {
	t.Helper()
	for k, tipo := range quiero {
		v, ok := obj[k]
		if !ok {
			t.Fatalf("%s: [%s] %s trae la clave %q (no la trae): %v", ca, nombre, donde, k, spClaves(obj))
		}
		if got := spTipo(v); got != tipo {
			t.Fatalf("%s: [%s] %s.%s es un %s, fue %s: %s", ca, nombre, donde, k, tipo, got, v)
		}
	}
}

func spClaves(obj map[string]json.RawMessage) []string {
	var out []string
	for k := range obj {
		out = append(out, k)
	}
	return out
}

// spObjeto decodifica un objeto JSON en un mapa de valores crudos.
func spObjeto(t *testing.T, ca, nombre, donde string, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	if spTipo(raw) != "object" {
		t.Fatalf("%s: [%s] %s es un objeto JSON, fue %s: %s", ca, nombre, donde, spTipo(raw), raw)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("%s: [%s] %s es un objeto JSON: %v", ca, nombre, donde, err)
	}
	return obj
}

// spPelotaJSON lee "pelota" del JSON crudo de una tarjeta y exige su forma
// (CA-722): presente, nunca null, con grupo, paso (accion, etiqueta, texto),
// frase, ahora (frase, gris), sigue (frase, gris) y camino (nombre, estado,
// texto).
func spPelotaJSON(t *testing.T, ca, nombre string, raw []byte) spPelota {
	t.Helper()
	card := spObjeto(t, ca, nombre, "la tarjeta", raw)
	p, ok := card["pelota"]
	if !ok {
		t.Fatalf("%s: [%s] la tarjeta trae \"pelota\" en su JSON (CA-722: siempre presente); no la trae. Claves: %v",
			ca, nombre, spClaves(card))
	}
	if spTipo(p) == "null" {
		t.Fatalf("%s: [%s] \"pelota\" nunca es null (CA-722)", ca, nombre)
	}
	obj := spObjeto(t, ca, nombre, "pelota", p)
	spCampos(t, ca, nombre, "pelota", obj, map[string]string{
		"grupo": "string", "paso": "object", "frase": "string", "ahora": "object", "sigue": "object", "camino": "array"})
	spCampos(t, ca, nombre, "pelota.paso", spObjeto(t, ca, nombre, "pelota.paso", obj["paso"]),
		map[string]string{"accion": "string", "etiqueta": "string", "texto": "string"})
	for _, k := range []string{"ahora", "sigue"} {
		spCampos(t, ca, nombre, "pelota."+k, spObjeto(t, ca, nombre, "pelota."+k, obj[k]),
			map[string]string{"frase": "string", "gris": "string"})
	}
	var camino []json.RawMessage
	if err := json.Unmarshal(obj["camino"], &camino); err != nil {
		t.Fatalf("%s: [%s] pelota.camino es una lista: %v", ca, nombre, err)
	}
	for i, e := range camino {
		donde := fmt.Sprintf("pelota.camino[%d]", i)
		spCampos(t, ca, nombre, donde, spObjeto(t, ca, nombre, donde, e),
			map[string]string{"nombre": "string", "estado": "string", "texto": "string"})
	}
	var out spPelota
	if err := json.Unmarshal(p, &out); err != nil {
		t.Fatalf("%s: [%s] pelota se decodifica: %v", ca, nombre, err)
	}
	return out
}

// spPelotaDe serializa la tarjeta (como la emite hoom board --json) y lee su
// pelota.
func spPelotaDe(t *testing.T, ca, nombre string, c Card) spPelota {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("%s: [%s] la tarjeta se serializa: %v", ca, nombre, err)
	}
	return spPelotaJSON(t, ca, nombre, raw)
}

// ---------------------------------------------------------------------------
// El oraculo: la regla del contrato, escrita desde el spec sobre los campos
// JSON de la tarjeta (column, column_name, running, interrupted, red, plain,
// actions, item).
// ---------------------------------------------------------------------------

// spGrupo es la tabla "grupo" del contrato: el primero que se cumple.
func spGrupo(c Card) string {
	switch {
	case c.Column == ColHecho:
		return spHecho
	case c.Running != nil:
		return spCorriendo
	case c.Interrupted != nil || (c.Red != nil && !c.Red.Esperado):
		return spDetenida
	case c.Column == ColBacklog:
		return spEnCola
	}
	return spTeToca
}

// spEtiquetaMin es la etiqueta en minuscula de las acciones de la regla 4,
// para el texto de una accion que la tarjeta no tiene.
var spEtiquetaMin = map[string]string{ActAprobar: "aprobar spec", ActIntegrar: "integrar"}

// spPasoDe son las 6 reglas del paso del contrato.
func spPasoDe(c Card) spPaso {
	switch {
	case c.Column == ColHecho:
		return spPaso{Texto: "la tarjeta ya esta en Hecho"}
	case c.Running != nil:
		return spPaso{Texto: "el " + c.Running.Role + " esta trabajando: espera a que termine"}
	case c.Interrupted != nil:
		return spPaso{Texto: "el trabajo se corto: reanudalo o pedilo de nuevo"}
	}
	id := ""
	switch c.Column {
	case ColTuAprobacion:
		id = ActAprobar
	case ColTuAceptacion:
		id = ActIntegrar
	default:
		if len(c.Actions) > 0 && (IsRoleAction(c.Actions[0].ID) || c.Actions[0].ID == ActGuardar) {
			id = c.Actions[0].ID
		}
	}
	if id == "" {
		return spPaso{Texto: "en " + c.ColumnName + " no hay nada que hacer ahora: " + c.Plain}
	}
	var a *Action
	for i := range c.Actions {
		if c.Actions[i].ID == id {
			a = &c.Actions[i]
			break
		}
	}
	if a == nil {
		return spPaso{Texto: "en " + c.ColumnName + " no hay " + spEtiquetaMin[id]}
	}
	if !a.Enabled {
		return spPaso{Texto: a.Why}
	}
	return spPaso{Accion: a.ID, Etiqueta: a.Label}
}

// spTitulo es el titulo del item, o el slug si esta vacio.
func spTitulo(c Card) string {
	if c.Item.Titulo == "" {
		return c.Slug
	}
	return c.Item.Titulo
}

// spFraseDe es la tabla de frases del contrato.
func spFraseDe(c Card, grupo string, p spPaso) string {
	t := spTitulo(c)
	switch grupo {
	case spDetenida:
		return "Decidí cómo sigue «" + t + "»."
	case spTeToca:
		switch p.Accion {
		case ActAprobar:
			return "Firmá el spec de «" + t + "»."
		case ActIntegrar:
			return "Aceptá «" + t + "»."
		case ActPedirArquitecto:
			return "Pedile el spec al arquitecto para «" + t + "»."
		case ActPedirTestWriter:
			return "Pedile los tests al test-writer para «" + t + "»."
		case ActPedirWriter:
			return "Pedile el código al writer para «" + t + "»."
		case ActPedirReviewer:
			return "Pedile la review al reviewer para «" + t + "»."
		case ActReanudar:
			return "Reanudá el trabajo de «" + t + "»."
		case ActGuardar:
			return "Guardá el trabajo de «" + t + "»."
		}
		return "Mirá «" + t + "»."
	}
	return ""
}

// spFraseDelRol es la frase de AHORA con running, segun su rol.
func spFraseDelRol(rol string) string {
	switch rol {
	case "arquitecto":
		return "El arquitecto está escribiendo el spec."
	case "test-writer":
		return "El test-writer está escribiendo los tests."
	case "writer":
		return "El writer está escribiendo el código."
	case "reviewer":
		return "El reviewer está revisando el cambio."
	}
	return "El " + rol + " está trabajando."
}

// spGrisCorriendo es la linea gris de AHORA con running:
// `<provider> · empezo <HH:MM> · <stage> <step>/<steps>` (el paso de la v2).
// La hora es HH:MM; el spec no dice en que huso, asi que solo se pide la forma.
func spGrisCorriendo(r *Running) *regexp.Regexp {
	paso := r.Stage + " " + strconv.Itoa(r.Step) + "/" + strconv.Itoa(r.Steps)
	return regexp.MustCompile("^" + regexp.QuoteMeta(r.Provider) + ` · empezo \d{2}:\d{2} · ` + regexp.QuoteMeta(paso) + "$")
}

// spAhoraDe es la tabla de AHORA del tablero v3, sin los casos del piloto del
// tablero, con `ya no corre` para interrupted y red.reason para un rojo no
// esperado. Con running devuelve la frase; la linea gris se mira con
// spGrisCorriendo.
func spAhoraDe(c Card) spFG {
	switch {
	case c.Running != nil:
		return spFG{Frase: spFraseDelRol(c.Running.Role)}
	case c.Interrupted != nil:
		return spFG{Frase: fmt.Sprintf("El trabajo del %s se cortó en el paso %d de %d.", c.Interrupted.Role,
			c.Interrupted.Step, c.Interrupted.Steps), Gris: "ya no corre"}
	case c.Red != nil && !c.Red.Esperado:
		return spFG{Frase: c.Red.Plain, Gris: c.Red.Reason}
	case c.Red != nil && c.Red.Esperado:
		return spFG{Frase: "Los tests están escritos y fallan, como se espera.", Gris: "falta el código"}
	case c.Column == ColTuAprobacion:
		return spFG{Frase: "El spec está listo y espera tu firma.", Gris: c.Plain}
	case c.Column == ColTuAceptacion:
		return spFG{Frase: "Está lista para que la pruebes y la aceptes.", Gris: c.Plain}
	case c.Column == ColBacklog:
		return spFG{Frase: "Todavía no empezó.", Gris: c.Plain}
	case c.Column == ColHecho:
		return spFG{Frase: "Terminada.", Gris: c.Plain}
	}
	return spFG{Frase: c.Plain}
}

// spSigueTabla es la tabla de SIGUE: por columna, {con piloto, sin piloto}.
var spSigueTabla = map[string][2]string{
	ColBacklog:      {"el arquitecto escribe el spec; despues te toca firmarlo", "el arquitecto escribe el spec; despues te toca firmarlo"},
	ColArquitecto:   {"te toca firmar el spec", "te toca firmar el spec"},
	ColTuAprobacion: {"al firmar, el piloto pide los tests y sigue solo hasta tu aceptación", "al firmar, te toca pedir los tests al test-writer"},
	ColTestWriter:   {"el piloto pide el código al writer y sigue hasta tu aceptación", "despues, te toca pedir el código al writer"},
	ColWriter:       {"el piloto pide la review; si algo bloquea, hace una ronda de hallazgos", "despues, te toca pedir la review al reviewer"},
	ColReview:       {"si nada bloquea, te toca aceptar; si algo bloquea, el piloto hace una ronda de hallazgos", "si nada bloquea, te toca aceptar"},
	ColTuAceptacion: {"al aceptar, hoom integra y la tarjeta pasa a Hecho", "al aceptar, hoom integra y la tarjeta pasa a Hecho"},
	ColHecho:        {"nada: la tarjeta esta terminada", "nada: la tarjeta esta terminada"},
}

// spSigueDe es SIGUE: la frase por columna y piloto, y sin piloto, en Tu
// aprobacion, Test-writer, Writer y Review, la linea gris que dice como
// encenderlo.
func spSigueDe(c Card) spFG {
	piloto := c.Item.Auto == item.AutoHastaHumano
	fila := spSigueTabla[c.Column]
	if piloto {
		return spFG{Frase: fila[0]}
	}
	out := spFG{Frase: fila[1]}
	switch c.Column {
	case ColTuAprobacion, ColTestWriter, ColWriter, ColReview:
		out.Gris = "con el piloto seguiria sola: hoom item set " + c.Slug + " --auto hasta-humano"
	}
	return out
}

// spCaminoDe son las 8 entradas del camino.
func spCaminoDe(c Card, grupo string) []spEntrada {
	idx := -1
	for i, col := range Columns {
		if col.ID == c.Column {
			idx = i
		}
	}
	out := make([]spEntrada, 0, len(Columns))
	for i := range Columns {
		e := spEntrada{Nombre: spNombres[i]}
		switch {
		case grupo == spHecho || i < idx:
			e.Estado = "hecho"
		case i == idx:
			e.Estado = "actual"
			switch grupo {
			case spTeToca:
				e.Texto = "te toca a vos"
			case spCorriendo:
				e.Texto = c.Running.Role + ", ahora"
			case spDetenida:
				e.Texto = "se detuvo"
			case spEnCola:
				e.Texto = "sin empezar"
			}
		default:
			e.Estado = "falta"
		}
		out = append(out, e)
	}
	return out
}

// spTeclas son los nombres de las teclas del tablero que ningun texto de
// pelota contiene (CA-726): `R `, `enter`, ` n ` y `P `, como palabras (una
// "R " al final de "ERROR " o el "enter" de "entera" no son teclas).
var spTeclas = []*regexp.Regexp{
	regexp.MustCompile(`(^|[^\p{L}\p{N}])R `),
	regexp.MustCompile(`(^|[^\p{L}\p{N}])enter([^\p{L}\p{N}]|$)`),
	regexp.MustCompile(` n `),
	regexp.MustCompile(`(^|[^\p{L}\p{N}])P `),
}

// spTextos son todos los textos de una pelota.
func spTextos(p spPelota) map[string]string {
	out := map[string]string{"grupo": p.Grupo, "paso.accion": p.Paso.Accion, "paso.etiqueta": p.Paso.Etiqueta,
		"paso.texto": p.Paso.Texto, "frase": p.Frase, "ahora.frase": p.Ahora.Frase, "ahora.gris": p.Ahora.Gris,
		"sigue.frase": p.Sigue.Frase, "sigue.gris": p.Sigue.Gris}
	for i, e := range p.Camino {
		out[fmt.Sprintf("camino[%d].nombre", i)] = e.Nombre
		out[fmt.Sprintf("camino[%d].estado", i)] = e.Estado
		out[fmt.Sprintf("camino[%d].texto", i)] = e.Texto
	}
	return out
}

// ---------------------------------------------------------------------------
// Comparadores por criterio.
// ---------------------------------------------------------------------------

func spMiraGrupo(t *testing.T, ca, nombre string, c Card, p spPelota) {
	t.Helper()
	if quiero := spGrupo(c); p.Grupo != quiero {
		t.Fatalf("%s: [%s] pelota.grupo es %q, fue %q (columna %s, running=%v, interrupted=%v, red=%+v)",
			ca, nombre, quiero, p.Grupo, c.Column, c.Running != nil, c.Interrupted != nil, c.Red)
	}
}

func spMiraPaso(t *testing.T, ca, nombre string, c Card, p spPelota) {
	t.Helper()
	if quiero := spPasoDe(c); p.Paso != quiero {
		t.Fatalf("%s: [%s] pelota.paso es %+v, fue %+v (columna %s, acciones %v)", ca, nombre, quiero, p.Paso, c.Column, acIDs(c))
	}
	// accion es el id de una accion habilitada de la tarjeta o "" (nunca
	// Seguir solo, que no es una accion de la tarjeta)
	if p.Paso.Accion != "" {
		a, ok := c.Action(p.Paso.Accion)
		if !ok || !a.Enabled {
			t.Fatalf("%s: [%s] pelota.paso.accion %q es una accion habilitada de la tarjeta (acciones %v)", ca, nombre, p.Paso.Accion, acIDs(c))
		}
		if p.Paso.Etiqueta != a.Label || p.Paso.Texto != "" {
			t.Fatalf("%s: [%s] con paso, etiqueta es su label %q y texto \"\": %+v", ca, nombre, a.Label, p.Paso)
		}
	} else if p.Paso.Etiqueta != "" || p.Paso.Texto == "" {
		t.Fatalf("%s: [%s] sin paso, accion y etiqueta son \"\" y texto dice por que: %+v", ca, nombre, p.Paso)
	}
}

func spMiraFrase(t *testing.T, ca, nombre string, c Card, p spPelota) {
	t.Helper()
	if quiero := spFraseDe(c, spGrupo(c), spPasoDe(c)); p.Frase != quiero {
		t.Fatalf("%s: [%s] pelota.frase es %q, fue %q (grupo %s, paso %+v)", ca, nombre, quiero, p.Frase, spGrupo(c), spPasoDe(c))
	}
}

func spMiraAhora(t *testing.T, ca, nombre string, c Card, p spPelota) {
	t.Helper()
	quiero := spAhoraDe(c)
	if p.Ahora.Frase != quiero.Frase {
		t.Fatalf("%s: [%s] pelota.ahora.frase es %q, fue %q", ca, nombre, quiero.Frase, p.Ahora.Frase)
	}
	if c.Running != nil {
		if c.Running.Provider != "" && c.Running.Steps > 0 {
			if re := spGrisCorriendo(c.Running); !re.MatchString(p.Ahora.Gris) {
				t.Fatalf("%s: [%s] con running, pelota.ahora.gris es `<provider> · empezo <HH:MM> · <stage> <step>/<steps>` (%s), fue %q",
					ca, nombre, re, p.Ahora.Gris)
			}
		}
		return
	}
	if p.Ahora.Gris != quiero.Gris {
		t.Fatalf("%s: [%s] pelota.ahora.gris es %q, fue %q", ca, nombre, quiero.Gris, p.Ahora.Gris)
	}
}

func spMiraSigue(t *testing.T, ca, nombre string, c Card, p spPelota) {
	t.Helper()
	if quiero := spSigueDe(c); p.Sigue != quiero {
		t.Fatalf("%s: [%s] pelota.sigue es %+v, fue %+v (columna %s, auto %q)", ca, nombre, quiero, p.Sigue, c.Column, c.Item.Auto)
	}
}

func spMiraTeclas(t *testing.T, ca, nombre string, p spPelota) {
	t.Helper()
	for campo, s := range spTextos(p) {
		for _, re := range spTeclas {
			if re.MatchString(s) {
				t.Fatalf("%s: [%s] ningun texto de pelota nombra una tecla del tablero (%s): pelota.%s = %q", ca, nombre, re, campo, s)
			}
		}
	}
}

func spMiraCamino(t *testing.T, ca, nombre string, c Card, p spPelota) {
	t.Helper()
	if len(p.Camino) != 8 {
		t.Fatalf("%s: [%s] pelota.camino tiene siempre 8 entradas, tuvo %d: %+v", ca, nombre, len(p.Camino), p.Camino)
	}
	if quiero := spCaminoDe(c, spGrupo(c)); !reflect.DeepEqual(p.Camino, quiero) {
		t.Fatalf("%s: [%s] pelota.camino es\n %+v\nfue\n %+v", ca, nombre, quiero, p.Camino)
	}
}

// spMiraTodo compara la pelota entera con el contrato.
func spMiraTodo(t *testing.T, nombre string, c Card, p spPelota) {
	t.Helper()
	spMiraGrupo(t, "CA-723", nombre, c, p)
	spMiraPaso(t, "CA-724", nombre, c, p)
	spMiraFrase(t, "CA-725", nombre, c, p)
	spMiraAhora(t, "CA-726", nombre, c, p)
	spMiraSigue(t, "CA-726", nombre, c, p)
	spMiraTeclas(t, "CA-726", nombre, p)
	spMiraCamino(t, "CA-727", nombre, c, p)
}

// ---------------------------------------------------------------------------
// Fixtures: Evidence literales (los de los tests de hoy y unos pocos mas).
// ---------------------------------------------------------------------------

type spFix struct {
	nombre string
	ev     Evidence
}

// spCorriendo suma un sobre abierto de rol cuyo dueno vive (run 3/5, claude).
func spConRun(ev Evidence, rol string) Evidence {
	ev.Envelopes = append([]EnvelopeState{acSobreVivo(rol, "claude", "run", 3, 5)}, ev.Envelopes...)
	return ev
}

// spRojoVeredicto pone un veredicto rojo (gate test) mas nuevo que el verde:
// un rojo que no es el esperado (no hay sobre del test-writer).
func spRojoVeredicto(ev Evidence) Evidence {
	v := bdVeredicto("v-rojo", bdT0.Add(40*time.Minute), "huella-1", 50, 10, []verdict.GateResult{
		{Name: "spec_lint", Required: true, Status: verdict.StatusPass},
		{Name: "test", Required: true, Status: verdict.StatusFail},
	})
	ev.Verdict, ev.GreenVerdicts = v, nil
	return ev
}

// spRojoSobre suma un sobre cerrado no entregable, mas nuevo que el veredicto:
// un rojo que no es el esperado.
func spRojoSobre(ev Evidence) Evidence {
	ev.Envelopes = append(ev.Envelopes, tbSobreCerrado("env-rojo", "writer", envelope.StatusNotDeliverable, "run", "se corto el agente"))
	return ev
}

// spEsperado es la tarjeta del rojo esperado del test-writer (CA-703), en
// Writer; con piloto o sin piloto.
func spEsperado(conPiloto bool) Evidence {
	v := aeRojoTest("v-tw-rojo")
	ev := tspEv(v, tspTW("verify", v))
	if !conPiloto {
		ev.Item.Auto = ""
	}
	return ev
}

// spColumna es acColumnas por id de columna (la primera forma de cada una).
func spColumna(t *testing.T, col string) Evidence {
	t.Helper()
	for _, cs := range acColumnas() {
		if cs.col == col {
			return cs.ev
		}
	}
	t.Fatalf("fixture: acColumnas no tiene la columna %s", col)
	return Evidence{}
}

// spCard deriva y exige la columna del fixture.
func spCard(t *testing.T, nombre string, ev Evidence, col string) Card {
	t.Helper()
	c := Derive(ev)
	if c.Column != col {
		t.Fatalf("fixture [%s]: cae en %q, se esperaba %q (missing=%q)", nombre, c.Column, col, c.Missing)
	}
	return c
}

// spTodas son todas las tarjetas de los fixtures, ya derivadas: las del
// piloto (9 formas x 3 topes x 4 extras), las de acciones (ocho columnas con y
// sin espacio propio), las del rojo esperado y casos sueltos de cada fila de
// las tablas.
func spTodas(t *testing.T) []piTarjeta {
	t.Helper()
	out := piTarjetas(t)
	for _, cs := range append(acColumnas(), acColumnasArbol()...) {
		out = append(out, piTarjeta{"acciones " + cs.nombre, Derive(cs.ev)})
		ev := cs.ev
		ev.Item.Auto = item.AutoHastaHumano
		out = append(out, piTarjeta{"acciones con piloto " + cs.nombre, Derive(ev)})
		ev = cs.ev
		ev.Item.Titulo = ""
		out = append(out, piTarjeta{"acciones sin titulo " + cs.nombre, Derive(ev)})
		ev = cs.ev
		ev.Uncommitted = acSucios()
		out = append(out, piTarjeta{"acciones con cambios " + cs.nombre, Derive(ev)})
	}
	for _, f := range spSueltos() {
		out = append(out, piTarjeta{f.nombre, Derive(f.ev)})
	}
	return out
}

// spSueltos son los casos de cada fila de las tablas que los fixtures de hoy
// no cubren.
func spSueltos() []spFix {
	cuando := bdT0.Add(48 * time.Hour)
	var out []spFix
	add := func(nombre string, ev Evidence) { out = append(out, spFix{nombre, ev}) }
	for _, rol := range []string{"arquitecto", "test-writer", "writer", "reviewer", "refutador"} {
		add("corriendo "+rol, spConRun(acWriter(), rol))
	}
	add("corriendo y en rojo", spConRun(spRojoVeredicto(acEv()), "writer"))
	add("corriendo en backlog", func() Evidence { ev := acEv(); ev.SpecExists = false; return spConRun(ev, "arquitecto") }())
	add("interrumpida en backlog", func() Evidence {
		ev := acEv()
		ev.SpecExists = false
		return acInterrumpido(ev, "arquitecto", "claude")
	}())
	add("interrumpida y en rojo", acInterrumpido(spRojoVeredicto(acEv()), "writer", "claude"))
	add("rojo del veredicto", spRojoVeredicto(acEv()))
	add("rojo del sobre en backlog", func() Evidence { ev := acEv(); ev.SpecExists = false; return spRojoSobre(ev) }())
	add("rojo del sobre en tu aprobacion", func() Evidence {
		ev := acEv()
		ev.Approval = approval.StatusNotApproved
		return spRojoSobre(ev)
	}())
	add("rojo del sobre en hecho", func() Evidence { ev := acEv(); ev.Item.HechoEn = &cuando; return spRojoSobre(ev) }())
	add("corriendo en hecho", func() Evidence { ev := acEv(); ev.Item.HechoEn = &cuando; return spConRun(ev, "writer") }())
	add("rojo esperado con piloto", spEsperado(true))
	add("rojo esperado sin piloto", spEsperado(false))
	add("writer sin proveedores", func() Evidence { ev := acWriter(); ev.Providers = nil; return ev }())
	add("tu aprobacion sin titulo", func() Evidence {
		ev := acEv()
		ev.Approval = approval.StatusNotApproved
		ev.Item.Titulo = ""
		return ev
	}())
	add("review sin accion con cambios", func() Evidence {
		ev := tbListo(acEv(), taskcmd.ReadySinGuardar, tbMsgSinGuardar)
		ev.Uncommitted = acSucios()
		return ev
	}())
	return out
}

// ===========================================================================
// CA-722: la pelota esta en el JSON de la tarjeta, siempre, nunca null.
// ===========================================================================

// CA-722: toda tarjeta que da Derive trae "pelota" en su JSON, como objeto,
// con grupo, paso (accion, etiqueta, texto), frase, ahora (frase, gris), sigue
// (frase, gris) y camino (nombre, estado, texto), en todas las columnas y en
// todos los estados de los fixtures.
func TestCA722_LaTarjetaTraePelotaEnSuJSON(t *testing.T) {
	for _, f := range spTodas(t) {
		spPelotaDe(t, "CA-722", f.nombre, f.c)
	}
}

// CA-722: la pelota no es un adorno del JSON: un grupo, un camino y un paso
// que existen en cada tarjeta (grupo uno de los cinco, camino de 8).
func TestCA722_PelotaNuncaVacia(t *testing.T) {
	validos := map[string]bool{spHecho: true, spCorriendo: true, spDetenida: true, spEnCola: true, spTeToca: true}
	for _, f := range spTodas(t) {
		p := spPelotaDe(t, "CA-722", f.nombre, f.c)
		if !validos[p.Grupo] {
			t.Fatalf("CA-722: [%s] pelota.grupo es uno de los cinco grupos, fue %q", f.nombre, p.Grupo)
		}
		if len(p.Camino) != 8 {
			t.Fatalf("CA-722: [%s] pelota.camino tiene sus 8 entradas, tuvo %d", f.nombre, len(p.Camino))
		}
		if p.Paso.Accion == "" && p.Paso.Texto == "" {
			t.Fatalf("CA-722: [%s] pelota.paso tiene una accion o dice por que no: %+v", f.nombre, p.Paso)
		}
	}
}

// ===========================================================================
// CA-723: el grupo, una prueba por fila de su tabla.
// ===========================================================================

func spGrupoEs(t *testing.T, nombre string, c Card, quiero string) {
	t.Helper()
	p := spPelotaDe(t, "CA-723", nombre, c)
	if p.Grupo != quiero {
		t.Fatalf("CA-723: [%s] pelota.grupo es %q, fue %q", nombre, quiero, p.Grupo)
	}
}

// CA-723 fila 1: la columna es Hecho, gane lo que gane (running, rojo).
func TestCA723_GrupoHecho(t *testing.T) {
	cuando := bdT0.Add(48 * time.Hour)
	ev := acEv()
	ev.Item.HechoEn = &cuando
	spGrupoEs(t, "hecho", spCard(t, "hecho", ev, ColHecho), spHecho)

	c := spCard(t, "hecho con running", spConRun(ev, "writer"), ColHecho)
	if c.Running != nil {
		spGrupoEs(t, "hecho con running", c, spHecho)
	}
	c = spCard(t, "hecho con rojo", spRojoSobre(ev), ColHecho)
	if c.Red != nil {
		spGrupoEs(t, "hecho con rojo", c, spHecho)
	}
}

// CA-723 fila 2: tiene running, en cualquier columna que no es Hecho, y antes
// que el rojo y que Backlog.
func TestCA723_GrupoCorriendo(t *testing.T) {
	for _, rol := range []string{"arquitecto", "test-writer", "writer", "reviewer"} {
		c := spCard(t, "corriendo "+rol, spConRun(acWriter(), rol), ColWriter)
		if c.Running == nil {
			t.Fatalf("fixture [corriendo %s]: la tarjeta tiene running", rol)
		}
		spGrupoEs(t, "corriendo "+rol, c, spCorriendo)
	}
	c := Derive(spConRun(spRojoVeredicto(acEv()), "writer"))
	if c.Running == nil || c.Red == nil || c.Red.Esperado {
		t.Fatalf("fixture [corriendo y en rojo]: running y un rojo no esperado: running=%v red=%+v", c.Running, c.Red)
	}
	spGrupoEs(t, "corriendo y en rojo (no esperado)", c, spCorriendo)

	ev := acEv()
	ev.SpecExists = false
	c = spCard(t, "corriendo en backlog", spConRun(ev, "arquitecto"), ColBacklog)
	spGrupoEs(t, "corriendo en backlog", c, spCorriendo)

	for _, f := range piTarjetas(t) {
		if f.c.Running != nil && f.c.Column != ColHecho {
			spGrupoEs(t, "piloto "+f.nombre, f.c, spCorriendo)
		}
	}
}

// CA-723 fila 3a: tiene interrupted (en cualquier columna salvo Hecho, aunque
// este en Backlog).
func TestCA723_GrupoDetenidaPorInterrupted(t *testing.T) {
	c := spCard(t, "interrumpida", acInterrumpido(acWriter(), "writer", "claude"), ColWriter)
	if c.Interrupted == nil || c.Running != nil {
		t.Fatalf("fixture [interrumpida]: interrupted sin running: %+v %+v", c.Interrupted, c.Running)
	}
	spGrupoEs(t, "interrumpida", c, spDetenida)

	ev := acEv()
	ev.SpecExists = false
	c = spCard(t, "interrumpida en backlog", acInterrumpido(ev, "arquitecto", "claude"), ColBacklog)
	spGrupoEs(t, "interrumpida en backlog (DETENIDA va antes que EN COLA)", c, spDetenida)

	ev = acEv()
	ev.Approval = approval.StatusNotApproved
	c = spCard(t, "interrumpida en tu aprobacion", acInterrumpido(ev, "arquitecto", "claude"), ColTuAprobacion)
	spGrupoEs(t, "interrumpida en tu aprobacion", c, spDetenida)
}

// CA-723 fila 3b: tiene red y red.esperado es false (del veredicto o del
// sobre), aunque este en Backlog o en una columna humana.
func TestCA723_GrupoDetenidaPorRojoNoEsperado(t *testing.T) {
	casos := []struct {
		nombre string
		ev     Evidence
		col    string
	}{
		{"rojo del veredicto", spRojoVeredicto(acEv()), ColWriter},
		{"rojo del sobre en backlog", func() Evidence { ev := acEv(); ev.SpecExists = false; return spRojoSobre(ev) }(), ColBacklog},
		{"rojo del sobre en tu aprobacion", func() Evidence {
			ev := acEv()
			ev.Approval = approval.StatusNotApproved
			return spRojoSobre(ev)
		}(), ColTuAprobacion},
		{"rojo del sobre en tu aceptacion", spRojoSobre(acEv()), ColTuAceptacion},
	}
	for _, cs := range casos {
		c := spCard(t, cs.nombre, cs.ev, cs.col)
		if c.Red == nil || c.Red.Esperado || c.Running != nil || c.Interrupted != nil {
			t.Fatalf("fixture [%s]: un rojo no esperado, sin running ni interrupted: red=%+v", cs.nombre, c.Red)
		}
		spGrupoEs(t, cs.nombre, c, spDetenida)
	}
}

// CA-723 fila 4: la columna es Backlog, sin running, interrupted ni rojo.
func TestCA723_GrupoEnCola(t *testing.T) {
	c := spCard(t, "backlog", spColumna(t, ColBacklog), ColBacklog)
	spGrupoEs(t, "backlog", c, spEnCola)
	ev := bdArbol(acEv())
	ev.SpecExists = false
	spGrupoEs(t, "backlog sin espacio propio", spCard(t, "backlog sin espacio propio", ev, ColBacklog), spEnCola)
}

// CA-723 fila 5: cualquier otra: las seis columnas del medio sin running,
// interrupted ni rojo no esperado, tengan paso o no (deshabilitado, sin
// accion).
func TestCA723_GrupoTeToca(t *testing.T) {
	for _, cs := range append(acColumnas(), acColumnasArbol()...) {
		if cs.col == ColBacklog || cs.col == ColHecho {
			continue
		}
		c := spCard(t, cs.nombre, cs.ev, cs.col)
		if c.Running != nil || c.Interrupted != nil || (c.Red != nil && !c.Red.Esperado) {
			continue
		}
		spGrupoEs(t, cs.nombre, c, spTeToca)
	}
	c := spCard(t, "writer sin proveedores", func() Evidence { ev := acWriter(); ev.Providers = nil; return ev }(), ColWriter)
	spGrupoEs(t, "writer sin proveedores (paso deshabilitado)", c, spTeToca)
}

// CA-723: una tarjeta con red.esperado y sin running esta en TE TOCA, no en
// DETENIDA (con piloto y sin piloto, el rojo del veredicto y el del sobre).
func TestCA723_RojoEsperadoEsTeToca(t *testing.T) {
	v := aeRojoTest("v-tw-rojo")
	casos := []spFix{
		{"rojo esperado con piloto", spEsperado(true)},
		{"rojo esperado sin piloto", spEsperado(false)},
		{"rojo esperado del sobre, sin veredicto de la tarjeta", tspEv(nil, tspTW("verify", v))},
		{"rojo esperado cerrado en check", tspEv(v, tspTW("check", v))},
	}
	for _, f := range casos {
		c := Derive(f.ev)
		if c.Red == nil || !c.Red.Esperado || c.Running != nil {
			t.Fatalf("fixture [%s]: rojo esperado sin running: red=%+v running=%v", f.nombre, c.Red, c.Running)
		}
		spGrupoEs(t, f.nombre, c, spTeToca)
	}
}

// CA-723: el grupo de cada tarjeta de todos los fixtures es el primero que se
// cumple de la tabla.
func TestCA723_GrupoEnTodosLosFixtures(t *testing.T) {
	for _, f := range spTodas(t) {
		spMiraGrupo(t, "CA-723", f.nombre, f.c, spPelotaDe(t, "CA-723", f.nombre, f.c))
	}
}

// ===========================================================================
// CA-724: el paso, una prueba por regla, con sus textos exactos.
// ===========================================================================

func spPasoEs(t *testing.T, nombre string, c Card, quiero spPaso) {
	t.Helper()
	p := spPelotaDe(t, "CA-724", nombre, c)
	if p.Paso != quiero {
		t.Fatalf("CA-724: [%s] pelota.paso es %+v, fue %+v (acciones %v)", nombre, quiero, p.Paso, acIDs(c))
	}
}

// CA-724 regla 1: en Hecho no hay paso: `la tarjeta ya esta en Hecho`.
func TestCA724_Regla1Hecho(t *testing.T) {
	ev := spColumna(t, ColHecho)
	spPasoEs(t, "hecho", spCard(t, "hecho", ev, ColHecho), spPaso{Texto: "la tarjeta ya esta en Hecho"})
	ev.Uncommitted = acSucios() // con guardar en sus acciones, igual no hay paso
	spPasoEs(t, "hecho con cambios sin guardar", spCard(t, "hecho con cambios", ev, ColHecho), spPaso{Texto: "la tarjeta ya esta en Hecho"})
}

// CA-724 regla 2: con running no hay paso:
// `el <rol> esta trabajando: espera a que termine`.
func TestCA724_Regla2Running(t *testing.T) {
	for _, rol := range []string{"writer", "reviewer", "arquitecto"} {
		c := spCard(t, "corriendo "+rol, spConRun(acWriter(), rol), ColWriter)
		spPasoEs(t, "corriendo "+rol, c, spPaso{Texto: "el " + rol + " esta trabajando: espera a que termine"})
	}
	ev := acEv()
	ev.Approval = approval.StatusNotApproved
	c := spCard(t, "corriendo en tu aprobacion", spConRun(ev, "arquitecto"), ColTuAprobacion)
	spPasoEs(t, "corriendo en tu aprobacion (antes que aprobar)", c, spPaso{Texto: "el arquitecto esta trabajando: espera a que termine"})
}

// CA-724 regla 3: con interrupted no hay paso:
// `el trabajo se corto: reanudalo o pedilo de nuevo` (sin nombrar la tecla R).
func TestCA724_Regla3Interrupted(t *testing.T) {
	const quiero = "el trabajo se corto: reanudalo o pedilo de nuevo"
	c := spCard(t, "interrumpida", acInterrumpido(acWriter(), "writer", "claude"), ColWriter)
	if _, ok := c.Action(ActReanudar); !ok {
		t.Fatalf("fixture [interrumpida]: tiene reanudar entre sus acciones: %v", acIDs(c))
	}
	spPasoEs(t, "interrumpida", c, spPaso{Texto: quiero})
	ev := acEv()
	ev.Approval = approval.StatusNotApproved
	c = spCard(t, "interrumpida en tu aprobacion", acInterrumpido(ev, "arquitecto", "claude"), ColTuAprobacion)
	spPasoEs(t, "interrumpida en tu aprobacion (antes que aprobar)", c, spPaso{Texto: quiero})
}

// CA-724 regla 4: en Tu aprobacion el paso es aprobar; en Tu aceptacion,
// integrar, con su label y texto "".
func TestCA724_Regla4AprobarEIntegrar(t *testing.T) {
	c := spCard(t, "tu aprobacion", spColumna(t, ColTuAprobacion), ColTuAprobacion)
	acHabilitada(t, "CA-724 (fixture)", acAccion(t, "CA-724", c, ActAprobar))
	spPasoEs(t, "tu aprobacion", c, spPaso{Accion: ActAprobar, Etiqueta: "Aprobar spec"})

	c = spCard(t, "tu aceptacion", spColumna(t, ColTuAceptacion), ColTuAceptacion)
	acHabilitada(t, "CA-724 (fixture)", acAccion(t, "CA-724", c, ActIntegrar))
	spPasoEs(t, "tu aceptacion", c, spPaso{Accion: ActIntegrar, Etiqueta: "Integrar"})

	// con piloto: la firma sigue siendo de la persona
	ev := spColumna(t, ColTuAprobacion)
	ev.Item.Auto = item.AutoHastaHumano
	spPasoEs(t, "tu aprobacion con piloto", spCard(t, "tu aprobacion con piloto", ev, ColTuAprobacion),
		spPaso{Accion: ActAprobar, Etiqueta: "Aprobar spec"})
}

// CA-724 regla 5: si no, la primera accion de la tarjeta cuando es de rol o
// guardar, con su label.
func TestCA724_Regla5PrimeraAccionDeRolOGuardar(t *testing.T) {
	casos := []struct {
		nombre string
		ev     Evidence
		col    string
		paso   spPaso
	}{
		{"backlog", spColumna(t, ColBacklog), ColBacklog, spPaso{Accion: ActPedirArquitecto, Etiqueta: "Pedir al arquitecto"}},
		{"arquitecto", spColumna(t, ColArquitecto), ColArquitecto, spPaso{Accion: ActPedirArquitecto, Etiqueta: "Pedir al arquitecto"}},
		{"test-writer", acTestWriter(), ColTestWriter, spPaso{Accion: ActPedirTestWriter, Etiqueta: "Pedir al test-writer"}},
		{"writer", acWriter(), ColWriter, spPaso{Accion: ActPedirWriter, Etiqueta: "Pedir al writer"}},
		{"review exigida", acGrande(acEv()), ColReview, spPaso{Accion: ActPedirReviewer, Etiqueta: "Pedir al reviewer"}},
		{"review con hallazgos", acConHallazgos(acEv(), "f-a"), ColReview, spPaso{Accion: ActPedirWriter, Etiqueta: "Pedir al writer"}},
		{"review sin accion con cambios", func() Evidence {
			ev := tbListo(acEv(), taskcmd.ReadySinGuardar, tbMsgSinGuardar)
			ev.Uncommitted = acSucios()
			return ev
		}(), ColReview, spPaso{Accion: ActGuardar, Etiqueta: "Guardar"}},
	}
	for _, cs := range casos {
		c := spCard(t, cs.nombre, cs.ev, cs.col)
		if len(c.Actions) == 0 || c.Actions[0].ID != cs.paso.Accion {
			t.Fatalf("fixture [%s]: la primera accion es %s: %v", cs.nombre, cs.paso.Accion, acIDs(c))
		}
		acHabilitada(t, "CA-724 (fixture "+cs.nombre+")", c.Actions[0])
		spPasoEs(t, cs.nombre, c, cs.paso)
	}
}

// CA-724 regla 6: sin ninguna de esas, no hay paso:
// `en <column_name> no hay nada que hacer ahora: <plain>`.
func TestCA724_Regla6NadaQueHacer(t *testing.T) {
	ev := tbListo(acEv(), taskcmd.ReadySinGuardar, tbMsgSinGuardar)
	c := spCard(t, "review sin accion", ev, ColReview)
	for _, a := range c.Actions {
		if IsRoleAction(a.ID) || a.ID == ActGuardar {
			t.Fatalf("fixture [review sin accion]: sin acciones de rol ni guardar: %v", acIDs(c))
		}
	}
	if c.Plain == "" {
		t.Fatal("fixture [review sin accion]: la tarjeta tiene plain")
	}
	spPasoEs(t, "review sin accion", c, spPaso{Texto: "en Review no hay nada que hacer ahora: " + c.Plain})

	c = spCard(t, "review sin accion sin espacio propio", bdArbol(acEv()), ColReview)
	spPasoEs(t, "review sin accion sin espacio propio", c, spPaso{Texto: "en Review no hay nada que hacer ahora: " + c.Plain})
}

// CA-724: con la accion elegida deshabilitada no hay paso, y texto es su why
// (sin proveedores, sin espacio propio, con el presupuesto agotado).
func TestCA724_PasoDeshabilitadoDaSuWhy(t *testing.T) {
	casos := []struct {
		nombre string
		ev     Evidence
		col    string
		id     string
	}{
		{"writer sin proveedores", func() Evidence { ev := acWriter(); ev.Providers = nil; return ev }(), ColWriter, ActPedirWriter},
		{"writer sin espacio propio", bdArbol(acWriter()), ColWriter, ActPedirWriter},
		{"test-writer sin espacio propio", bdArbol(acTestWriter()), ColTestWriter, ActPedirTestWriter},
		{"writer con el presupuesto agotado", piEv("writer", piAgotado, piNada), ColWriter, ActPedirWriter},
	}
	for _, cs := range casos {
		c := spCard(t, cs.nombre, cs.ev, cs.col)
		a := acAccion(t, "CA-724 (fixture "+cs.nombre+")", c, cs.id)
		if a.Enabled || a.Why == "" || c.Actions[0].ID != cs.id {
			t.Fatalf("fixture [%s]: la primera accion %s esta deshabilitada con why: %+v (%v)", cs.nombre, cs.id, a, acIDs(c))
		}
		spPasoEs(t, cs.nombre, c, spPaso{Texto: a.Why})
	}
}

// CA-724: el paso nunca es Seguir solo: con el piloto encendido en
// Test-writer, Writer y Review (donde el tablero ofreceria Seguir solo) el
// paso es el de la regla siguiente, la accion de rol de la tarjeta.
func TestCA724_NuncaSeguirSolo(t *testing.T) {
	for _, cs := range []struct {
		forma, id, label string
	}{
		{"test-writer", ActPedirTestWriter, "Pedir al test-writer"},
		{"writer", ActPedirWriter, "Pedir al writer"},
		{"review-exigida", ActPedirReviewer, "Pedir al reviewer"},
	} {
		c := piCard(t, cs.forma, piConTope, piNada)
		if c.Item.Auto != item.AutoHastaHumano {
			t.Fatalf("fixture [%s]: con piloto", cs.forma)
		}
		spPasoEs(t, "piloto en "+cs.forma, c, spPaso{Accion: cs.id, Etiqueta: cs.label})
	}
}

// CA-724: en todos los fixtures el paso sigue las 6 reglas, y accion es el id
// de una accion habilitada de la tarjeta o "".
func TestCA724_PasoEnTodosLosFixtures(t *testing.T) {
	for _, f := range spTodas(t) {
		spMiraPaso(t, "CA-724", f.nombre, f.c, spPelotaDe(t, "CA-724", f.nombre, f.c))
	}
}

// ===========================================================================
// CA-725: la frase, una prueba por fila de su tabla.
// ===========================================================================

func spFraseEs(t *testing.T, nombre string, c Card, quiero string) {
	t.Helper()
	p := spPelotaDe(t, "CA-725", nombre, c)
	if p.Frase != quiero {
		t.Fatalf("CA-725: [%s] pelota.frase es %q, fue %q (grupo %q, paso %+v)", nombre, quiero, p.Frase, p.Grupo, p.Paso)
	}
}

// CA-725: DETENIDA (por interrupted y por rojo no esperado):
// `Decidí cómo sigue «<titulo>».`
func TestCA725_FraseDetenida(t *testing.T) {
	spFraseEs(t, "interrumpida", spCard(t, "interrumpida", acInterrumpido(acWriter(), "writer", "claude"), ColWriter),
		"Decidí cómo sigue «Precios por region».")
	spFraseEs(t, "rojo no esperado", spCard(t, "rojo", spRojoVeredicto(acEv()), ColWriter),
		"Decidí cómo sigue «Precios por region».")
	ev := acEv()
	ev.Approval = approval.StatusNotApproved
	spFraseEs(t, "rojo no esperado en tu aprobacion (tenga paso o no)", spCard(t, "rojo en tu aprobacion", spRojoSobre(ev), ColTuAprobacion),
		"Decidí cómo sigue «Precios por region».")
}

// CA-725: TE TOCA con cada paso de la tabla.
func TestCA725_FraseTeTocaPorPaso(t *testing.T) {
	casos := []struct {
		nombre string
		ev     Evidence
		col    string
		frase  string
	}{
		{"aprobar", spColumna(t, ColTuAprobacion), ColTuAprobacion, "Firmá el spec de «Precios por region»."},
		{"integrar", spColumna(t, ColTuAceptacion), ColTuAceptacion, "Aceptá «Precios por region»."},
		{"pedir-arquitecto", spColumna(t, ColArquitecto), ColArquitecto, "Pedile el spec al arquitecto para «Precios por region»."},
		{"pedir-test-writer", acTestWriter(), ColTestWriter, "Pedile los tests al test-writer para «Precios por region»."},
		{"pedir-writer", acWriter(), ColWriter, "Pedile el código al writer para «Precios por region»."},
		{"pedir-writer en review con hallazgos", acConHallazgos(acEv(), "f-a"), ColReview, "Pedile el código al writer para «Precios por region»."},
		{"pedir-reviewer", acGrande(acEv()), ColReview, "Pedile la review al reviewer para «Precios por region»."},
		{"guardar", func() Evidence {
			ev := tbListo(acEv(), taskcmd.ReadySinGuardar, tbMsgSinGuardar)
			ev.Uncommitted = acSucios()
			return ev
		}(), ColReview, "Guardá el trabajo de «Precios por region»."},
		{"rojo esperado sin piloto (pide el codigo)", spEsperado(false), ColWriter, "Pedile el código al writer para «Precios por region»."},
	}
	for _, cs := range casos {
		c := spCard(t, cs.nombre, cs.ev, cs.col)
		spFraseEs(t, cs.nombre, c, cs.frase)
	}
}

// CA-725: TE TOCA sin paso (deshabilitado o sin nada que hacer):
// `Mirá «<titulo>».`
func TestCA725_FraseTeTocaSinPaso(t *testing.T) {
	spFraseEs(t, "writer sin proveedores", spCard(t, "writer sin proveedores",
		func() Evidence { ev := acWriter(); ev.Providers = nil; return ev }(), ColWriter), "Mirá «Precios por region».")
	spFraseEs(t, "writer sin espacio propio", spCard(t, "writer sin espacio propio", bdArbol(acWriter()), ColWriter),
		"Mirá «Precios por region».")
	spFraseEs(t, "review sin accion", spCard(t, "review sin accion", tbListo(acEv(), taskcmd.ReadySinGuardar, tbMsgSinGuardar), ColReview),
		"Mirá «Precios por region».")
}

// CA-725: en CORRIENDO, EN COLA y HECHO la frase es "".
func TestCA725_FraseVaciaFueraDeTeTocaYDetenida(t *testing.T) {
	spFraseEs(t, "corriendo", spCard(t, "corriendo", spConRun(acWriter(), "writer"), ColWriter), "")
	spFraseEs(t, "en cola", spCard(t, "en cola", spColumna(t, ColBacklog), ColBacklog), "")
	spFraseEs(t, "hecho", spCard(t, "hecho", spColumna(t, ColHecho), ColHecho), "")
	ev := spColumna(t, ColHecho)
	spFraseEs(t, "hecho en rojo", spCard(t, "hecho en rojo", spRojoSobre(ev), ColHecho), "")
}

// CA-725: una tarjeta sin titulo usa su slug en la frase.
func TestCA725_SinTituloUsaElSlug(t *testing.T) {
	ev := spColumna(t, ColTuAprobacion)
	ev.Item.Titulo = ""
	spFraseEs(t, "tu aprobacion sin titulo", spCard(t, "sin titulo", ev, ColTuAprobacion), "Firmá el spec de «precios».")
	ev = acInterrumpido(acWriter(), "writer", "claude")
	ev.Item.Titulo = ""
	spFraseEs(t, "detenida sin titulo", spCard(t, "detenida sin titulo", ev, ColWriter), "Decidí cómo sigue «precios».")
	ev = acWriter()
	ev.Providers, ev.Item.Titulo = nil, ""
	spFraseEs(t, "sin paso sin titulo", spCard(t, "sin paso sin titulo", ev, ColWriter), "Mirá «precios».")
}

// CA-725: el titulo va en una linea: un titulo con salto de linea no parte la
// frase (el spec no dice como se junta, solo que queda en una linea).
func TestCA725_TituloEnUnaLinea(t *testing.T) {
	ev := spColumna(t, ColTuAprobacion)
	ev.Item.Titulo = "Precios\npor region"
	p := spPelotaDe(t, "CA-725", "titulo con salto", spCard(t, "titulo con salto", ev, ColTuAprobacion))
	if strings.ContainsAny(p.Frase, "\n\r") || !strings.HasPrefix(p.Frase, "Firmá el spec de «Precios") || !strings.HasSuffix(p.Frase, "».") {
		t.Fatalf("CA-725: la frase lleva el titulo en una linea: %q", p.Frase)
	}
}

// CA-725: en todos los fixtures la frase es la de la tabla para su grupo y su
// paso.
func TestCA725_FraseEnTodosLosFixtures(t *testing.T) {
	for _, f := range spTodas(t) {
		spMiraFrase(t, "CA-725", f.nombre, f.c, spPelotaDe(t, "CA-725", f.nombre, f.c))
	}
}
