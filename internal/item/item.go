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
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"gopkg.in/yaml.v3"
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
	// Sesiones are the interactive sessions opened on the card's workspace
	// (`hoom cockpit --task`, "Abrir sesion" in the Studio): the DECLARED
	// writers `hoom review` reads. Written by a person's tool, never by an
	// agent (the envelope's floor forbids .hoom/items/).
	Sesiones []Sesion `yaml:"sesiones,omitempty" json:"sesiones,omitempty"`
}

// Sesion is one interactive session opened on the card's workspace.
type Sesion struct {
	Provider   string    `yaml:"provider" json:"provider"`
	AbiertaPor string    `yaml:"abierta_por,omitempty" json:"abierta_por"`
	AbiertaEn  time.Time `yaml:"abierta_en" json:"abierta_en"`
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
	var b strings.Builder
	guion := false
	for _, r := range strings.ToLower(titulo) {
		if plano, ok := sinAcento[r]; ok {
			r = plano
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if guion && b.Len() > 0 {
				b.WriteByte('-')
			}
			guion = false
			b.WriteRune(r)
			continue
		}
		guion = true
	}
	slug := b.String()
	if len(slug) > MaxSlug {
		slug = strings.TrimRight(slug[:MaxSlug], "-")
	}
	return slug
}

var sinAcento = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'ã': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
	'ñ': 'n', 'ç': 'c',
}

// slugRe is the shape `hoom task start` accepts.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidSlug reports whether s has the shape `hoom task start` accepts and
// fits in MaxSlug.
func ValidSlug(s string) bool {
	return len(s) <= MaxSlug && slugRe.MatchString(s)
}

// Dir is the directory of the items, under root.
func Dir(root string) string { return filepath.Join(root, ".hoom", DirName) }

// Path is the file of an item, under root.
func Path(root, slug string) string {
	return filepath.Join(Dir(root), slug+".yaml")
}

// RelPath is the file of an item relative to the project root, with slashes:
// the name the messages and the board use.
func RelPath(slug string) string { return ".hoom/" + DirName + "/" + slug + ".yaml" }

// Validate checks a draft against the vocabularies and returns it with its
// defaults applied and its slug resolved.
func Validate(d Draft) (Draft, error) {
	d.Titulo = strings.TrimSpace(d.Titulo)
	if d.Titulo == "" {
		return d, fmt.Errorf("titulo vacio")
	}
	d.Tipo = strings.ToLower(strings.TrimSpace(d.Tipo))
	if d.Tipo == "" {
		d.Tipo = DefaultTipo
	}
	if !contains(Tipos, d.Tipo) {
		return d, fmt.Errorf("tipo desconocido %q (validos: %s)", d.Tipo, strings.Join(Tipos, ", "))
	}
	d.Prioridad = strings.ToLower(strings.TrimSpace(d.Prioridad))
	if d.Prioridad == "" {
		d.Prioridad = DefaultPrioridad
	}
	if !contains(Prioridades, d.Prioridad) {
		return d, fmt.Errorf("prioridad desconocida %q (validas: %s)", d.Prioridad, strings.Join(Prioridades, ", "))
	}
	if d.PresupuestoUSD != nil && !(*d.PresupuestoUSD > 0) {
		return d, fmt.Errorf("presupuesto_usd tiene que ser mayor que 0 (para no declarar tope, no lo pases)")
	}
	d.Slug = strings.TrimSpace(d.Slug)
	if d.Slug == "" {
		d.Slug = Slugify(d.Titulo)
		if d.Slug == "" {
			return d, fmt.Errorf("del titulo no sale ningun slug; usa --slug")
		}
	}
	if !ValidSlug(d.Slug) {
		return d, fmt.Errorf("slug invalido %q: minusculas, numeros y guiones, hasta %d caracteres (ej: precios-por-region)", d.Slug, MaxSlug)
	}
	return d, nil
}

// Add creates the item's file. It fails, wrapping ErrExists, when the file is
// already there (and leaves it byte for byte as it was).
func Add(root string, d Draft) (Item, error) {
	d, err := Validate(d)
	if err != nil {
		return Item{}, err
	}
	it := Item{
		Slug: d.Slug, Titulo: d.Titulo, Tipo: d.Tipo, Prioridad: d.Prioridad,
		Pedido: strings.TrimSpace(d.Pedido), CreadoPor: gitx.Identity(root),
		CreadoEn: time.Now().UTC().Truncate(time.Second), PresupuestoUSD: d.PresupuestoUSD,
	}
	raw, err := encode(it)
	if err != nil {
		return Item{}, err
	}
	if err := os.MkdirAll(Dir(root), 0o755); err != nil {
		return Item{}, err
	}
	// O_EXCL: two `item add` of the same slug at once cannot both win, and
	// the loser never touches the winner's file.
	f, err := os.OpenFile(Path(root, it.Slug), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return Item{}, fmt.Errorf("%w: %s; elegi otro titulo o fija el slug con --slug", ErrExists, RelPath(it.Slug))
	}
	if err != nil {
		return Item{}, err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		os.Remove(Path(root, it.Slug))
		return Item{}, err
	}
	if err := f.Close(); err != nil {
		return Item{}, err
	}
	return it, nil
}

// claves are the only keys an item may carry, in the order hoom writes them.
var claves = []string{"titulo", "tipo", "prioridad", "pedido", "creado_por", "creado_en",
	"presupuesto_usd", "hecho_en", "commit_final"}

// Parse reads an item's bytes strictly: an unknown key, a missing required
// field or a value outside its vocabulary is an error.
func Parse(slug string, raw []byte) (Item, error) {
	if !ValidSlug(slug) {
		return Item{}, fmt.Errorf("el nombre %q no es un slug (minusculas, numeros y guiones)", slug)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Item{}, fmt.Errorf("yaml ilegible: %v", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return Item{}, fmt.Errorf("un item es un mapa de claves (titulo, tipo, prioridad, ...)")
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i].Value; !contains(claves, k) {
			return Item{}, fmt.Errorf("clave desconocida %q (un item no guarda columna ni estado: la columna sale de la evidencia; validas: %s)",
				k, strings.Join(claves, ", "))
		}
	}
	var it Item
	if err := m.Decode(&it); err != nil {
		return Item{}, fmt.Errorf("valor invalido: %v", strings.TrimPrefix(err.Error(), "yaml: unmarshal errors:\n  "))
	}
	it.Slug = slug
	switch {
	case strings.TrimSpace(it.Titulo) == "":
		return Item{}, fmt.Errorf("falta titulo")
	case !contains(Tipos, it.Tipo):
		return Item{}, fmt.Errorf("tipo %q fuera del vocabulario (%s)", it.Tipo, strings.Join(Tipos, ", "))
	case !contains(Prioridades, it.Prioridad):
		return Item{}, fmt.Errorf("prioridad %q fuera del vocabulario (%s)", it.Prioridad, strings.Join(Prioridades, ", "))
	case strings.TrimSpace(it.CreadoPor) == "":
		return Item{}, fmt.Errorf("falta creado_por")
	case it.CreadoEn.IsZero():
		return Item{}, fmt.Errorf("falta creado_en")
	case it.PresupuestoUSD != nil && !(*it.PresupuestoUSD > 0):
		return Item{}, fmt.Errorf("presupuesto_usd tiene que ser mayor que 0")
	}
	it.CreadoEn = it.CreadoEn.UTC()
	if it.HechoEn != nil {
		t := it.HechoEn.UTC()
		it.HechoEn = &t
	}
	return it, nil
}

// Load reads one item by slug.
func Load(root, slug string) (Item, error) {
	raw, err := os.ReadFile(Path(root, slug))
	if err != nil {
		return Item{}, err
	}
	return Parse(slug, raw)
}

// List returns the valid items of root sorted by creado_en and then slug,
// plus one warning per invalid file (never fatal). A project without items
// gets empty, non-nil slices.
func List(root string) ([]Item, []string, error) {
	items, warnings := []Item{}, []string{}
	entries, err := os.ReadDir(Dir(root))
	if os.IsNotExist(err) {
		return items, warnings, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		rel := ".hoom/" + DirName + "/" + name
		slug, ok := strings.CutSuffix(name, ".yaml")
		if !ok {
			warnings = append(warnings, fmt.Sprintf("item ignorado %s: un item es <slug>.yaml", rel))
			continue
		}
		raw, err := os.ReadFile(filepath.Join(Dir(root), name))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("item ilegible %s: %v", rel, err))
			continue
		}
		it, err := Parse(slug, raw)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("item invalido %s: %v", rel, err))
			continue
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].CreadoEn.Equal(items[j].CreadoEn) {
			return items[i].CreadoEn.Before(items[j].CreadoEn)
		}
		return items[i].Slug < items[j].Slug
	})
	return items, warnings, nil
}

// MarkDone records the close of a task on its item: hecho_en (UTC, seconds)
// and commit_final, keeping every other field. It reports whether it wrote:
// no item, or an item already done, is (false, nil); an unreadable or invalid
// item is an error.
func MarkDone(root, slug, commit string, at time.Time) (bool, error) {
	raw, err := os.ReadFile(Path(root, slug))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	it, err := Parse(slug, raw)
	if err != nil {
		return false, err
	}
	if it.HechoEn != nil {
		return false, nil
	}
	t := at.UTC().Truncate(time.Second)
	it.HechoEn, it.CommitFinal = &t, strings.TrimSpace(commit)
	out, err := encode(it)
	if err != nil {
		return false, err
	}
	if err := hoomfs.AtomicWrite(Path(root, slug), out, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// encode writes an item in the canonical key order (the struct's).
func encode(it Item) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(it); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// AddSession appends one session to the item's sesiones, stamped with the git
// identity of root, keeping every other field. It reports whether it wrote:
// no item is (false, nil); an unreadable or invalid item is an error.
func AddSession(root, slug, provider string, at time.Time) (bool, error) {
	return false, fmt.Errorf("sin implementar") // esqueleto
}
