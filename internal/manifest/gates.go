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
// embedded profiles theirs). The keys a gate gets from a merge (<<) are
// checked as its own, at any depth (findings 13e26e and dc272d).
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
		raiz = sinAlias(raiz.Content[0])
	}
	if raiz == nil || raiz.Kind != yaml.MappingNode {
		return nil
	}
	visto := map[*yaml.Node]bool{}
	for i := 0; i+1 < len(raiz.Content); i += 2 {
		if raiz.Content[i].Value != "gates" {
			continue
		}
		gates := sinAlias(raiz.Content[i+1])
		if gates == nil || gates.Kind != yaml.MappingNode {
			return nil // today's decoder gives its error
		}
		for j := 0; j+1 < len(gates.Content); j += 2 {
			if err := validarGate(gates.Content[j].Value, gates.Content[j+1], visto); err != nil {
				return err
			}
		}
	}
	return nil
}

// validarGate is one gates.<g>: its keys and its aplica_en. A merge key
// (<<) brings the keys of the map (or maps) it names, merges of theirs
// included, which are checked as the gate's own, at any depth. visto, shared
// by the whole document, holds the maps already walked: one that comes again
// (merged by two gates, twice in a chain, or a cycle, which the decoder then
// rejects) was checked or is being checked, so it adds nothing, and the walk
// stays linear in the size of the document, without a cap on the depth.
func validarGate(nombre string, n *yaml.Node, visto map[*yaml.Node]bool) error {
	n = sinAlias(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil // null is a gate without keys; anything else is the decoder's
	}
	if visto[n] {
		return nil
	}
	visto[n] = true
	for i := 0; i+1 < len(n.Content); i += 2 {
		clave, valor := n.Content[i], n.Content[i+1]
		if clave.Kind == yaml.ScalarNode && clave.ShortTag() == "!!merge" {
			v := sinAlias(valor)
			fuentes := []*yaml.Node{v}
			if v != nil && v.Kind == yaml.SequenceNode {
				fuentes = v.Content
			}
			for _, f := range fuentes {
				if err := validarGate(nombre, f, visto); err != nil {
					return err
				}
			}
			continue
		}
		if !containsKey(GateKeys, clave.Value) {
			return fmt.Errorf(ErrGateClaveDesconocida, nombre, clave.Value)
		}
		if clave.Value == "aplica_en" {
			if err := validarAplicaEn(nombre, valor); err != nil {
				return err
			}
		}
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
