package spec

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Criterion is one acceptance criterion as written: its id and the statement
// of its own bullet in the criteria section ("" when the id is only cited
// elsewhere in the spec).
type Criterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

var (
	headingRe   = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	bulletRe    = regexp.MustCompile(`^\s*[-*]\s+`)
	criterionRe = regexp.MustCompile("^\\s*[-*]\\s+[*`]*(CA-\\d+)[*`]*\\s*:\\s*(.*)$")
	spacesRe    = regexp.MustCompile(`\s+`)
)

// Criteria returns one Criterion per id of Lint, in Lint's order. The text
// is the bullet that states the criterion inside the "criterios de
// aceptacion" section, continuation lines joined, without its id prefix and
// without the verifica marker.
func Criteria(path string) ([]Criterion, error) {
	ids, _, _, err := Lint(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	texts := map[string]string{}
	level := 0 // heading level of the criteria section; 0 = outside
	cur := ""  // id of the bullet being read
	var buf []string
	flush := func() {
		if cur != "" {
			if _, seen := texts[cur]; !seen {
				t := verifRe.ReplaceAllString(strings.Join(buf, " "), "")
				texts[cur] = strings.TrimSpace(spacesRe.ReplaceAllString(t, " "))
			}
		}
		cur, buf = "", nil
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if h := headingRe.FindStringSubmatch(line); h != nil {
			flush()
			switch {
			case strings.Contains(normalize(h[2]), "criterios de aceptacion"):
				level = len(h[1])
			case level > 0 && len(h[1]) <= level:
				level = 0
			}
			continue
		}
		if level == 0 {
			continue
		}
		if m := criterionRe.FindStringSubmatch(line); m != nil {
			flush()
			cur, buf = m[1], []string{m[2]}
			continue
		}
		if strings.TrimSpace(line) == "" || bulletRe.MatchString(line) {
			flush()
			continue
		}
		if cur != "" {
			buf = append(buf, strings.TrimSpace(line))
		}
	}
	flush()
	out := make([]Criterion, 0, len(ids))
	for _, id := range ids {
		out = append(out, Criterion{ID: id, Text: texts[id]})
	}
	return out, nil
}

// TokenIndex is one pass over the test files of a tree: for every whole
// CA-n token, the files that cite it.
type TokenIndex struct {
	Scanned int // test files read
	files   map[string][]string
}

// IndexTokens reads the test files of root once, with the same filter as
// Trace, and records which files cite each whole token. A token never
// matches inside a longer one: the regexp takes every digit.
func IndexTokens(root string) (TokenIndex, error) {
	idx := TokenIndex{files: map[string][]string{}}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !isTestFile(rel) {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil || info.Size() > 2<<20 {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		idx.Scanned++
		rel = filepath.ToSlash(rel)
		seen := map[string]bool{}
		for _, tok := range caRe.FindAllString(string(raw), -1) {
			if !seen[tok] {
				seen[tok] = true
				idx.files[tok] = append(idx.files[tok], rel)
			}
		}
		return nil
	})
	if err != nil {
		return TokenIndex{}, err
	}
	for _, fs := range idx.files {
		sort.Strings(fs)
	}
	return idx, nil
}

// Files returns the test files citing id, relative to the indexed root,
// slash-separated and sorted. Never nil.
func (x TokenIndex) Files(id string) []string {
	return append([]string{}, x.files[id]...)
}

// Missing returns the ids no test file cites, in the given order.
func (x TokenIndex) Missing(ids []string) []string {
	var out []string
	for _, id := range ids {
		if len(x.files[id]) == 0 {
			out = append(out, id)
		}
	}
	return out
}
