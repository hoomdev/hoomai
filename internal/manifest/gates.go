package manifest

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/riesgo"
)

// ValidarGates (gates-por-ruta) checks the gates: section of a YAML document
// (a hoom.yaml, or an embedded profile) with the strictness Load applies to
// it, before anything decodes it: each gate's keys are those of GateKeys
// (ErrGateClaveDesconocida) and its aplica_en is a non-empty list of valid
// patterns (ErrAplicaEnLista, ErrAplicaEnPatron). The checks go in the
// file's order, gate by gate and key by key: the first error found is the
// one given, without any prefix (Load adds "hoom.yaml invalido: ", the
// embedded profiles theirs).
//
// It reads the document the way yaml.v3's decoder does, at every level (the
// document, gates: and each gate): a merge key (<<) brings, where it is, the
// keys of the map or maps it names, at any depth (findings 13e26e and
// dc272d), and a key that is an alias is the key it points to. What comes
// from a merge is checked even where the decoder would let a key of the map
// itself win over it: stricter, never laxer.
//
// What it does not judge stays as today: a document that is not YAML is the
// parser's error, a gates: or a gate that is not a map, and a value of
// required, cmd, diff_cmd, notes or partial of the wrong type, are left to
// the decoder that reads the document afterwards, with today's errors. nil
// for a document without gates:.
func ValidarGates(raw []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	raiz := &doc
	if raiz.Kind == yaml.DocumentNode && len(raiz.Content) > 0 {
		raiz = raiz.Content[0]
	}
	v := validador{visto: map[visita]bool{}}
	return v.pares(raiz, rolRaiz, func(clave, valor *yaml.Node) error {
		if clave.Value != "gates" {
			return nil
		}
		return v.pares(valor, rolGates, func(nombre, gate *yaml.Node) error {
			return v.pares(gate, rolGate, func(clave, valor *yaml.Node) error {
				return validarClave(nombre.Value, clave.Value, valor)
			})
		})
	})
}

// rol is what a map is to ValidarGates: the document, its gates:, or a gate.
type rol int

const (
	rolRaiz rol = iota
	rolGates
	rolGate
)

// visita is a map walked in a rol.
type visita struct {
	n   *yaml.Node
	rol rol
}

// validador holds, for the whole document, the maps already walked in each
// rol: one that comes again (merged twice, by two gates, or a cycle, which
// the decoder then rejects) was checked or is being checked, so it adds
// nothing. The walk stays linear in the size of the document, without a cap
// on the depth.
type validador struct {
	visto map[visita]bool
}

// pares calls f with each key and value of the map n, in the file's order,
// the way the decoder reads them: a merge key brings, where it is, the
// pairs of the map (or the maps of the sequence) it names, merges of theirs
// included; a key that is an alias comes as the node it points to. A map
// already walked in r gives nothing; so does anything that is not a map (the
// decoder gives its error).
func (v validador) pares(n *yaml.Node, r rol, f func(clave, valor *yaml.Node) error) error {
	n = sinAlias(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	k := visita{n, r}
	if v.visto[k] {
		return nil
	}
	v.visto[k] = true
	for i := 0; i+1 < len(n.Content); i += 2 {
		clave, valor := n.Content[i], n.Content[i+1]
		if esMerge(clave) {
			fuente := sinAlias(valor)
			fuentes := []*yaml.Node{fuente}
			if fuente != nil && fuente.Kind == yaml.SequenceNode {
				fuentes = fuente.Content
			}
			for _, m := range fuentes {
				if err := v.pares(m, r, f); err != nil {
					return err
				}
			}
			continue
		}
		if c := sinAlias(clave); c != nil {
			clave = c
		}
		if err := f(clave, valor); err != nil {
			return err
		}
	}
	return nil
}

// esMerge is yaml.v3's merge key: a plain << (or one tagged !!merge); a
// quoted "<<", or an alias to one, is an ordinary key.
func esMerge(clave *yaml.Node) bool {
	return clave.Kind == yaml.ScalarNode && clave.Value == "<<" && clave.ShortTag() == "!!merge"
}

// validarClave is one key of gates.<g>: one of GateKeys, and aplica_en a
// valid one.
func validarClave(nombre, clave string, valor *yaml.Node) error {
	if !containsKey(GateKeys, clave) {
		return fmt.Errorf(ErrGateClaveDesconocida, nombre, clave)
	}
	if clave == "aplica_en" {
		return validarAplicaEn(nombre, valor)
	}
	return nil
}

// validarAplicaEn is gates.<g>.aplica_en: a non-empty list of strings, each a
// valid pattern of `rutas:` (riesgo.PatronValido). null, an empty list, a
// scalar, a map, or an element that is not a string is ErrAplicaEnLista.
// The elements are judged in their order, like those of `rutas:`.
func validarAplicaEn(nombre string, n *yaml.Node) error {
	n = sinAlias(n)
	if n == nil || n.Kind != yaml.SequenceNode || len(n.Content) == 0 {
		return fmt.Errorf(ErrAplicaEnLista, nombre)
	}
	for _, e := range n.Content {
		e = sinAlias(e)
		if e == nil || e.Kind != yaml.ScalarNode || e.ShortTag() != "!!str" {
			return fmt.Errorf(ErrAplicaEnLista, nombre)
		}
		if !riesgo.PatronValido(e.Value) {
			return fmt.Errorf(ErrAplicaEnPatron, nombre, e.Value)
		}
	}
	return nil
}

// sinAlias is the node an alias points to (n itself when it is not one).
// yaml.v3 anchors only scalars, sequences and maps, never an alias, so an
// alias points to a node that is not one: the loop takes one step at most,
// and needs no cap.
func sinAlias(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}
