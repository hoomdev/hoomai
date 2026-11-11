package riesgo

// The three riesgos a card may declare.
const (
	// Bajo: one lens (reliability) whatever the size of the change, unless
	// the change touches a risk path (then it is Normal).
	Bajo = "bajo"
	// Normal: the rule as it was before the riesgo existed. It is the
	// absence of the key: no verb of hoom ever writes `riesgo: normal`.
	Normal = "normal"
	// Alto: the 4 lenses always, a review always required (unless the
	// change is empty or only documentation), and the mediums of a complete
	// review block.
	Alto = "alto"
)

// Vocabulario is every riesgo a card may declare, in this order: Bajo,
// Normal, Alto.
var Vocabulario = []string{Bajo, Normal, Alto}

// Marcas are the substrings that make a path a risk path, matched against
// the path in lower case. They are the lens rule's marks, unchanged.
var Marcas = []string{
	"auth", "login", "password", "passwd", "secret", "token", "cred",
	"crypt", "sign", "pago", "payment", "billing", "invoice", "dte",
	"permission", "permiso", "sandbox", "sudo",
}

// Ruta is the first of files, in their order, whose path in lower case
// contains one of Marcas, returned as it was given; "" when none does (an
// empty or nil files included).
func Ruta(files []string) string {
	return ""
}

// Efectivo is the riesgo hoom applies for a card that declares declarado and
// whose whole change is files:
//
//   - declarado Bajo and Ruta(files) not empty: Normal, with the motivo
//     `el cambio toca <ruta>: riesgo normal` (<ruta> is Ruta(files));
//   - any other case: declarado ("" gives Normal) and an empty motivo.
//
// The riesgo goes up by itself and never down by itself: Normal and Alto are
// returned as they are, with or without a risk path. A declarado outside
// Vocabulario is returned as it is too: validating it is Validar's job.
func Efectivo(declarado string, files []string) (efectivo, motivo string) {
	return "", ""
}

// Validar says whether r is a riesgo a card may declare: nil for "" (the
// missing key, Normal) and for each value of Vocabulario; for anything else
// the error whose text is exactly
// `riesgo "<r>" fuera del vocabulario (bajo, normal, alto)` (r quoted as %q
// quotes it), the text item.Parse gives.
func Validar(r string) error {
	return nil
}
