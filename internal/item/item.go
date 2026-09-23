// Package item is the card of the visual cabin as a FILE a person writes:
// .hoom/items/<slug>.yaml, one file per item, versioned in Git with the same
// pattern as .hoom/findings/ — two people adding items on two machines never
// write the same file. An item holds what a person decides (title, kind,
// priority, the request for the architect, a budget) and nothing that could
// move the card: its column is a function of the evidence (see boardcmd), so
// a key like `columna:` makes the file invalid instead of being obeyed.
//
// The slug is the file name and the card's only identity for its whole life:
// it is also the spec's name (.hoom/specs/<slug>.md) and the task's
// (`hoom task start <slug>`).
package item

import (
	"errors"
	"time"
)

// DirName is where the items live, under .hoom/.
const DirName = "items"

// MaxSlug bounds a slug's length.
const MaxSlug = 64

// Vocabularies, in the order the usage prints them.
var (
	Tipos       = []string{"feature", "bug", "refactor", "seguridad", "docs", "test"}
	Prioridades = []string{"alta", "media", "baja"}
)

// Defaults of `hoom item add`.
const (
	DefaultTipo      = "feature"
	DefaultPrioridad = "media"
)

// Item is one card. Slug is the file name: outside the YAML, inside the JSON.
// hecho_en and commit_final are written by `hoom task done`, never by an
// agent (the envelope's floor forbids any change under .hoom/items/).
type Item struct {
	Slug           string     `yaml:"-" json:"slug"`
	Titulo         string     `yaml:"titulo" json:"titulo"`
	Tipo           string     `yaml:"tipo" json:"tipo"`
	Prioridad      string     `yaml:"prioridad" json:"prioridad"`
	Pedido         string     `yaml:"pedido,omitempty" json:"pedido"`
	CreadoPor      string     `yaml:"creado_por" json:"creado_por"`
	CreadoEn       time.Time  `yaml:"creado_en" json:"creado_en"`
	PresupuestoUSD *float64   `yaml:"presupuesto_usd,omitempty" json:"presupuesto_usd,omitempty"`
	HechoEn        *time.Time `yaml:"hecho_en,omitempty" json:"hecho_en,omitempty"`
	CommitFinal    string     `yaml:"commit_final,omitempty" json:"commit_final,omitempty"`
}

// Draft is what `hoom item add` asks for before hoom stamps it (identity,
// time). Empty Tipo/Prioridad take the defaults; empty Slug is Slugify(Titulo).
type Draft struct {
	Titulo         string
	Tipo           string
	Prioridad      string
	Pedido         string
	Slug           string
	PresupuestoUSD *float64
}

// ErrExists is wrapped by Add when the item's file is already there.
var ErrExists = errors.New("el item ya existe")

// Slugify turns a title into a slug: lower case, no accents, every run of
// characters outside [a-z0-9] becomes one '-', no '-' at either end, at most
// MaxSlug characters. "" means the title yields no slug.
func Slugify(titulo string) string {
	return ""
}

// ValidSlug reports whether s has the shape `hoom task start` accepts and
// fits in MaxSlug.
func ValidSlug(s string) bool {
	return false
}

// Path is the file of an item, under root.
func Path(root, slug string) string {
	return ""
}

// Validate checks a draft against the vocabularies and returns it with its
// defaults applied and its slug resolved.
func Validate(d Draft) (Draft, error) {
	return Draft{}, nil
}

// Add creates the item's file. It fails, wrapping ErrExists, when the file is
// already there (and leaves it byte for byte as it was).
func Add(root string, d Draft) (Item, error) {
	return Item{}, nil
}

// Parse reads an item's bytes strictly: an unknown key, a missing required
// field or a value outside its vocabulary is an error.
func Parse(slug string, raw []byte) (Item, error) {
	return Item{}, nil
}

// Load reads one item by slug.
func Load(root, slug string) (Item, error) {
	return Item{}, nil
}

// List returns the valid items of root sorted by creado_en and then slug,
// plus one warning per invalid file (never fatal).
func List(root string) ([]Item, []string, error) {
	return nil, nil, nil
}

// MarkDone records the close of a task on its item: hecho_en (UTC, seconds)
// and commit_final, keeping every other field. It reports whether it wrote:
// no item, or an item already done, is (false, nil); an unreadable or invalid
// item is an error.
func MarkDone(root, slug, commit string, at time.Time) (bool, error) {
	return false, nil
}
