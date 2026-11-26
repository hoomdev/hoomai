package riesgo

import (
	"fmt"
	"sort"
)

// TipoSeguridad is the tipo of a card (item.Tipos) whose effective riesgo is
// always Alto (rule 2 of Clasificador.Evaluar).
const TipoSeguridad = "seguridad"

// UmbralProducto is the threshold in lines of producto (Tamano.Producto,
// never insertions+deletions): past it, a Normal change gets the 4 lenses
// of `hoom review`, the board requires the review of a Bajo or Normal card
// (Alto always requires it), and `hoom verify` prints AvisoProductoGrande
// when the effective riesgo is Normal. reviewcmd.UmbralLineas is the same
// number.
const UmbralProducto = 400

// The motivos of the effective riesgo, exactly (Clasificador.Evaluar); %s
// is a riesgo of Vocabulario in MotivoDeclarado and a path in
// MotivoRutaDeRiesgo and MotivoFueraDeBajo.
const (
	// MotivoIlegible: rule 1, there is a task and its card cannot be read.
	MotivoIlegible = "no se pudo leer la tarjeta: riesgo alto"
	// MotivoSeguridad: rule 2, the card is of tipo seguridad.
	MotivoSeguridad = "tarjeta de tipo seguridad: riesgo alto"
	// MotivoDeclarado: rules 3, 6 and 8 (`declarado alto`, `declarado
	// bajo`, `declarado normal`); also the motivo of a card's riesgo when
	// its declared one wins over its verdict's (finding.RiesgoDeTarjeta).
	MotivoDeclarado = "declarado %s"
	// MotivoRutaDeRiesgo: rule 4, a file of class producto or prueba that
	// matches Rutas.DeRiesgo.
	MotivoRutaDeRiesgo = "el cambio toca %s: riesgo alto"
	// MotivoFueraDeBajo: rule 5, a Bajo card with a file of class producto
	// that does not match Rutas.DeBajoRiesgo.
	MotivoFueraDeBajo = "el cambio toca %s, fuera de las rutas de bajo riesgo: riesgo normal"
	// MotivoSinTarjeta: rule 7, there is no task.
	MotivoSinTarjeta = "sin tarjeta: riesgo normal"
)

// Tarjeta is what the rule of the effective riesgo reads of the task's card
// (item.TarjetaDeRiesgo reads it). The zero Tarjeta is "no task".
type Tarjeta struct {
	// Tarea is the task's slug; "" = there is no task, and then Ilegible,
	// Tipo and Riesgo are not read.
	Tarea string
	// Ilegible says that the task's card (its item) is there and cannot be
	// read.
	Ilegible bool
	// Tipo is the card's tipo (item.Tipos).
	Tipo string
	// Riesgo is the riesgo the card declares: one of Vocabulario, "" =
	// Normal. Any other value is read as Ilegible (it fails closed).
	Riesgo string
}

// Evaluacion is the riesgo of a change as hoom computed it: the key
// `riesgo` of the verdict, {"declarado": d, "efectivo": e, "motivo": m}.
type Evaluacion struct {
	// Declarado is the card's declared riesgo: Normal when the card does not
	// say it, when it cannot be read and when there is no task.
	Declarado string `json:"declarado"`
	// Efectivo is the riesgo hoom applies: one of Vocabulario, never lower
	// than Declarado.
	Efectivo string `json:"efectivo"`
	// Motivo says which rule gave Efectivo, exactly as the rule writes it;
	// never empty.
	Motivo string `json:"motivo"`
}

// LineaRiesgo is the line of the riesgo, exactly, with %s for the effective
// riesgo and its motivo: `riesgo: <efectivo> (<motivo>)`. `hoom verify`
// prints it in its header, `hoom review` in its output (indented), and
// `hoom status` prints the one of the newest verdict.
const LineaRiesgo = "riesgo: %s (%s)"

// Linea is e's line: LineaRiesgo with e.Efectivo and e.Motivo.
func (e Evaluacion) Linea() string {
	return fmt.Sprintf(LineaRiesgo, e.Efectivo, e.Motivo)
}

// The warnings of the header of `hoom verify`, exactly.
const (
	// AvisoMayorQueDeclarado, when the effective riesgo is higher than the
	// declared one, with %s for the effective riesgo, the declared one, the
	// task's slug and the effective riesgo again:
	// `! el riesgo efectivo (<efectivo>) es mayor que el declarado (<declarado>): hoom item set <slug> --riesgo <efectivo> lo sube`.
	AvisoMayorQueDeclarado = "! el riesgo efectivo (%s) es mayor que el declarado (%s): hoom item set %s --riesgo %s lo sube"
	// AvisoProductoGrande, when the effective riesgo is Normal and the
	// producto passes UmbralProducto.
	AvisoProductoGrande = "! mas de 400 lineas de producto: la review exige 2 lentes (risk y reliability)"
)

// Evaluar is the effective riesgo of a change whose card is t and whose
// files are archivos (paths relative to the root of the repository, with
// "/", in any order: the keys of gitx.LineasDelCambio, or the changed_files
// of a verdict older than riesgo-efectivo-y-tamano). The first rule that
// holds wins:
//
//	#  condition                                                   efectivo  motivo
//	1  t has a task and Ilegible                                   Alto      MotivoIlegible
//	2  t's Tipo is TipoSeguridad                                   Alto      MotivoSeguridad
//	3  t declares Alto                                             Alto      `declarado alto`
//	4  a file of class producto or prueba matches c.Rutas.DeRiesgo Alto      `el cambio toca <ruta>: riesgo alto`
//	5  t declares Bajo and a file of class producto does not
//	   match c.Rutas.DeBajoRiesgo                                  Normal    `el cambio toca <ruta>, fuera de las rutas de bajo riesgo: riesgo normal`
//	6  t declares Bajo                                             Bajo      `declarado bajo`
//	7  there is no task                                            Normal    MotivoSinTarjeta
//	8  anything else                                               Normal    `declarado normal`
//
// <ruta> is the first file that meets the condition in byte order, as it
// was given. A file of class hoom or generado never raises the riesgo: a
// Bajo card that only touches pruebas, .hoom/ or documentation stays Bajo.
// Without files (outside a git repository) rules 4 and 5 never hold. The
// result's Declarado is t's (Normal without a task) and its Efectivo is
// never lower than it. An empty path is left out, and archivos is not
// changed.
func (c Clasificador) Evaluar(t Tarjeta, archivos []string) Evaluacion {
	tarea := t.Tarea != ""
	ilegible := tarea && (t.Ilegible || Validar(t.Riesgo) != nil)
	declarado := Normal
	if tarea && !ilegible && t.Riesgo != "" {
		declarado = t.Riesgo
	}
	e := Evaluacion{Declarado: declarado}
	switch {
	case ilegible:
		e.Efectivo, e.Motivo = Alto, MotivoIlegible
		return e
	case tarea && t.Tipo == TipoSeguridad:
		e.Efectivo, e.Motivo = Alto, MotivoSeguridad
		return e
	case declarado == Alto:
		e.Efectivo, e.Motivo = Alto, fmt.Sprintf(MotivoDeclarado, Alto)
		return e
	}
	orden := make([]string, 0, len(archivos))
	for _, f := range archivos {
		if f != "" {
			orden = append(orden, f)
		}
	}
	sort.Strings(orden)
	for _, f := range orden {
		if cl := c.Clase(f); (cl == ClaseProducto || cl == ClasePrueba) && CoincideAlguno(c.Rutas.DeRiesgo, f) {
			e.Efectivo, e.Motivo = Alto, fmt.Sprintf(MotivoRutaDeRiesgo, f)
			return e
		}
	}
	if declarado == Bajo {
		for _, f := range orden {
			if c.Clase(f) == ClaseProducto && !CoincideAlguno(c.Rutas.DeBajoRiesgo, f) {
				e.Efectivo, e.Motivo = Normal, fmt.Sprintf(MotivoFueraDeBajo, f)
				return e
			}
		}
		e.Efectivo, e.Motivo = Bajo, fmt.Sprintf(MotivoDeclarado, Bajo)
		return e
	}
	if !tarea {
		e.Efectivo, e.Motivo = Normal, MotivoSinTarjeta
		return e
	}
	e.Efectivo, e.Motivo = Normal, fmt.Sprintf(MotivoDeclarado, Normal)
	return e
}

// Mayor is the higher of the riesgos a and b in the order of Vocabulario
// ("" is Normal, and Mayor never returns ""); a value outside Vocabulario
// counts as Alto, and then Mayor returns Alto. The riesgo of a
// card is the higher of the effective riesgo of its newest verdict and its
// declared one now (finding.RiesgoDeTarjeta).
func Mayor(a, b string) string {
	if nivelDe(b) > nivelDe(a) {
		a = b
	}
	return Vocabulario[nivelDe(a)]
}

// nivelDe is r's place in Vocabulario: "" is Normal, and a value outside it
// counts as Alto (it fails closed).
func nivelDe(r string) int {
	switch r {
	case Bajo:
		return 0
	case "", Normal:
		return 1
	}
	return 2
}
