package riesgo

// Rutas are the three lists of patterns of the section `rutas` of hoom.yaml
// (spec riesgo-efectivo-y-tamano), already resolved: a key the file does
// not give carries the default of the project's profile (profiles.Rutas,
// manifest.Rutas.Resolver). Each element is a pattern (Coincide). An empty
// or nil list matches no path: `rutas.de_riesgo: []` makes no path a risk
// path.
type Rutas struct {
	// DeRiesgo: a file of class producto or prueba that matches one makes
	// the effective riesgo Alto (rule 4 of Clasificador.Evaluar); a medium
	// finding whose file matches one blocks (finding.Bloquea).
	DeRiesgo []string
	// DeBajoRiesgo: the only files of class producto a Bajo card may touch
	// and stay Bajo (rule 5 of Clasificador.Evaluar).
	DeBajoRiesgo []string
	// Generadas: a file that matches one (and is not under .hoom/) is of
	// class generado: it adds to no number of Tamano and never raises the
	// riesgo.
	Generadas []string
}

// RutasBase are the defaults without a profile, the row «base» of the spec's
// table; every profile adds its own row to it (profiles.Rutas):
//
//   - DeRiesgo: `**/*<m>*` for each m of Marcas, in Marcas' order;
//   - DeBajoRiesgo: `**/*.md`, `**/*.txt`, `**/*.rst`, `**/*.adoc`,
//     `**/docs`, `**/LICENSE`, `**/NOTICE`, in that order;
//   - Generadas: none (nil).
//
// Each call returns new slices: changing them changes no later call.
func RutasBase() Rutas {
	return Rutas{}
}

// Coincide says whether the pattern patron matches the path ruta (relative
// to the root of the repository, with "/"). The syntax is the one of
// agents.<rol>.write.allow: `*`, `?` and `[...]` inside one segment (as
// path.Match reads them) and `**` for zero or more whole segments (several
// `**` in a row are one). Unlike the write territory, a pattern matches a
// path when it matches the path itself or one of its parent directories,
// and it does not distinguish upper from lower case: `**/*auth*` matches
// internal/oauth.go and everything under app/Http/Auth/ (for example
// app/Http/Auth/Login.php), and `vendor` matches vendor/a/b.go. A pattern
// that is not valid (PatronValido) matches nothing.
func Coincide(patron, ruta string) bool {
	return false
}

// CoincideAlguno says whether one of patrones matches ruta (Coincide); false
// for an empty or nil patrones.
func CoincideAlguno(patrones []string, ruta string) bool {
	return false
}

// PatronValido says whether p is a valid pattern of `rutas`, the check
// behind the error `rutas.<clave>: patron invalido "<p>"` of the manifest. A
// pattern is not valid when it is empty (or only blanks), when one of its
// segments is a malformed path.Match pattern (an unclosed `[`, a trailing
// `\`), or when it has an empty segment (it starts or ends with "/", or has
// "//"): none of those would ever match a path of the change.
func PatronValido(p string) bool {
	return false
}

// The classes of a file of the change (Clasificador.Clase).
const (
	// ClaseHoom: everything under .hoom/, whatever else it matches.
	ClaseHoom = "hoom"
	// ClaseGenerado: matches Rutas.Generadas.
	ClaseGenerado = "generado"
	// ClasePrueba: in the test-writer's territory (Clasificador.Pruebas).
	ClasePrueba = "prueba"
	// ClaseProducto: everything else, hoom.yaml included.
	ClaseProducto = "producto"
)

// Clasificador decides the class of each file of a change: the rutas of the
// project and the test-writer's territory. The zero Clasificador has no
// rutas and no territory: every file outside .hoom/ is producto.
// profiles.Clasificador builds the project's.
type Clasificador struct {
	// Rutas are the project's resolved rutas.
	Rutas Rutas
	// Pruebas is the test-writer's write territory: the patterns of
	// agents.test-writer.write.allow, or, without that key, its default
	// (agents.AllowPorDefecto(agents.ScopeTests)). They are matched the way
	// the envelope matches a write territory: against the whole path, case
	// sensitive, with no parent-directory rule (unlike Coincide). It may
	// include `.hoom/**`: a file under .hoom/ is hoom before it is prueba.
	Pruebas []string
}

// Clase is the class of the file ruta (relative to the root of the
// repository, with "/"): the first that holds, in this order:
//
//  1. ClaseHoom: ruta is under .hoom/;
//  2. ClaseGenerado: ruta matches c.Rutas.Generadas (CoincideAlguno);
//  3. ClasePrueba: ruta is in c.Pruebas;
//  4. ClaseProducto: anything else, hoom.yaml included.
func (c Clasificador) Clase(ruta string) string {
	return ""
}

// Tamano is the size of a change (spec riesgo-efectivo-y-tamano): the sum
// of the lines of its files of each class. A file of class generado adds to
// none. It is the key `tamano` of the verdict: {"producto": p, "pruebas": t,
// "hoom": h}.
type Tamano struct {
	Producto int `json:"producto"`
	Pruebas  int `json:"pruebas"`
	Hoom     int `json:"hoom"`
}

// LineaTamano is the line of the size, exactly, with %d for producto,
// pruebas and hoom: `tamano: producto <p> · pruebas <t> · hoom <h>`. `hoom
// verify` prints it in its header, and `hoom status` and the detail of
// `hoom tablero` print the one of the card's newest verdict.
const LineaTamano = "tamano: producto %d · pruebas %d · hoom %d"

// Linea is t's line: LineaTamano with its three numbers.
func (t Tamano) Linea() string {
	return ""
}

// Medir is the size of the change whose files and lines are lineas (path
// relative to the root of the repository, with "/" -> its lines, as
// gitx.LineasDelCambio gives them): each file's lines go to the number of
// its class (Clase), and a generado's to none. An empty path is left out.
// It does not change lineas.
func (c Clasificador) Medir(lineas map[string]int) Tamano {
	return Tamano{}
}
