package agentcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/materialgit"
	"github.com/hoomdev/hoomai/internal/providers"
)

// RuleConfigDeAgentes (contencion-del-writer, "La config de los agentes"): a
// role that executes changed what makes code run in the next session of an
// agent's CLI (ConfigAgentes):
//   - in the tree: `.claude/`, `.codex/`, `.mcp.json` and what each adapter
//     declares in providers.Estado.EnArbol, created, changed or removed;
//   - outside the tree: what each adapter declares in providers.Estado.Corre
//     (claude: the key `hooks` of `~/.claude/settings.json` and of
//     `~/.claude/settings.local.json`, and `mcpServers`, global and of each
//     project, of `~/.claude.json`; codex: the whole `~/.codex/config.toml`).
//
// That the CLI changes other keys of those JSON files (its counters, its
// history) is not it. It holds for every role that executes, with encierro
// or without it, in any project, and agents.<rol>.write of hoom.yaml cannot
// lift it. In the envelope it is compared after the run, before any git of
// hoom on the tree, and cuts like RuleMaterialDeGit, without running git
// status: the envelope closes in scope, no-entregable, exit 1, with no
// verify, its note is NotaConfigDeAgentes(rol), its Violation.Detail
// DetalleConfigDeAgentes and its Violation.Path what changed (as
// CambiosConfigAgentes names it), so its finding (high, author
// AutorGateDeScope) names it. The review compares it around each pass
// (reviewcmd.PasadaConConfigDeAgentes).
const RuleConfigDeAgentes = "config-de-agentes"

// DetalleConfigDeAgentes is the Detail of a RuleConfigDeAgentes violation.
const DetalleConfigDeAgentes = "la config de los agentes hace correr codigo en la proxima sesion: ningun rol que ejecuta la cambia"

// NotaConfigDeAgentes is the note of an envelope of role rol cut by
// RuleConfigDeAgentes:
// `el <rol> cambio la config de los agentes: no se emite veredicto sobre este arbol`.
func NotaConfigDeAgentes(rol string) string {
	return "el " + rol + " cambio la config de los agentes: no se emite veredicto sobre este arbol"
}

// DeConfigDeAgentes says whether f is the finding the scope gate registered
// for a RuleConfigDeAgentes violation in a run of role rol
// (DescripcionViolacion, author AutorGateDeScope, severity high), whatever its
// path. The pilot reads with it which envelopes changed the agents' config,
// and treats them as the ones that changed the git material
// (boardcmd.Sobre).
func DeConfigDeAgentes(f finding.Finding, rol string) bool {
	return deRegla(f, rol, RuleConfigDeAgentes, DetalleConfigDeAgentes)
}

// ConfigAgentes is the agents' config of a tree as RuleConfigDeAgentes
// compares it (FotoConfigAgentes): by path -> its Huella. A path inside the
// tree is relative to it, with "/" (each file and symlink under `.claude/`,
// `.codex/` and the directories of EnArbol is its own entry, a directory
// never is); a path outside it is
// absolute. For a file of providers.Estado.Corre with Claves, the Huella is
// that of the value of those keys only; without Claves, that of the whole
// file. ProtegidoAusente for what is not there, HuellaIlegible for what
// could not be read.
type ConfigAgentes map[string]string

// TopeFotoConfigAgentes and MaxLeidoConfigAgentes (ronda 1, decision 27)
// bound each photograph of the agents' config (FotoConfigAgentes): its time
// (the walks of `.claude/`, `.codex/` and EnArbol included), and the bytes
// of one JSON file of Corre it decodes. A photograph that passes its time is
// unfinished: its one entry FotoSinTerminar, and CambiosConfigAgentes against
// it gives [FotoSinTerminar]; a JSON file larger than MaxLeidoConfigAgentes
// is not decoded, and its Huella is materialgit's (Grande). Never hangs.
const (
	TopeFotoConfigAgentes       = 10 * time.Second
	MaxLeidoConfigAgentes int64 = 16 << 20
)

// FotoConfigAgentesConTope is FotoConfigAgentes within tope and maxLeido
// bytes of each JSON file.
func FotoConfigAgentesConTope(dir string, tope time.Duration, maxLeido int64) ConfigAgentes {
	limite := time.Now().Add(tope)
	sinTerminar := ConfigAgentes{FotoSinTerminar: FotoSinTerminar}
	foto := ConfigAgentes{}
	enArbol := []string{".claude/", ".codex/", ".mcp.json"}
	var corre []providers.Corre
	for _, p := range providers.Default.All() {
		ce, ok := p.(providers.ConEstado)
		if !ok {
			continue
		}
		e := ce.Estado()
		enArbol = append(enArbol, e.EnArbol...)
		corre = append(corre, e.Corre...)
	}
	vistos := map[string]bool{}
	for _, rel := range enArbol {
		rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
		if rel == "" || vistos[rel] {
			continue
		}
		vistos[rel] = true
		if !fotoEnArbol(foto, dir, rel, limite) {
			return sinTerminar
		}
	}
	home, _ := os.UserHomeDir()
	porArchivo := map[string][]string{}
	enteros := map[string]bool{}
	var orden []string
	for _, c := range corre {
		a := expandirHome(c.Archivo, home)
		if a == "" {
			continue
		}
		if _, ok := porArchivo[a]; !ok && !enteros[a] {
			orden = append(orden, a)
		}
		if len(c.Claves) == 0 {
			enteros[a] = true
			continue
		}
		porArchivo[a] = append(porArchivo[a], c.Claves...)
	}
	for _, a := range orden {
		if time.Now().After(limite) {
			return sinTerminar
		}
		if enteros[a] {
			foto[filepath.ToSlash(a)] = materialgit.Huella(a)
			continue
		}
		foto[filepath.ToSlash(a)] = huellaDeClaves(a, porArchivo[a], maxLeido)
	}
	if time.Now().After(limite) {
		return sinTerminar
	}
	return foto
}

// FotoConfigAgentes photographs the agents' config of the tree dir: in the
// tree, `.claude/`, `.codex/`, `.mcp.json` and the EnArbol of every provider
// of providers.Default that implements providers.ConEstado; outside it, the
// Corre of each of them ("~/" being this process's $HOME). It reads files
// only, never follows a symlink, and runs no git. A JSON value that is null,
// an empty object or an empty list reads as absent: claude writes
// `"mcpServers": {}` for each new project, which makes nothing run. It is
// bounded by TopeFotoConfigAgentes and MaxLeidoConfigAgentes.
func FotoConfigAgentes(dir string) ConfigAgentes {
	return FotoConfigAgentesConTope(dir, TopeFotoConfigAgentes, MaxLeidoConfigAgentes)
}

// fotoEnArbol adds to foto the entry rel of the tree dir: a directory (rel
// ending in "/") by what is under it, each file and symlink by its path
// relative to dir; a file by itself. Never followed. A directory is never an
// entry of its own: a new empty one changes no config, and one removed or
// replaced shows by its files or by the symlink (or file) in its place. A
// directory that cannot be read is HuellaIlegible, by its path with "/". It
// says whether it ended before limite.
func fotoEnArbol(foto ConfigAgentes, dir, rel string, limite time.Time) bool {
	esDir := strings.HasSuffix(rel, "/")
	p := filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(rel, "/")))
	fi, err := os.Lstat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if !esDir {
			foto[rel] = ProtegidoAusente
		}
		return true
	case err != nil:
		foto[rel] = HuellaIlegible
		return true
	case !esDir || !fi.IsDir():
		foto[strings.TrimSuffix(rel, "/")] = materialgit.Huella(p)
		return true
	}
	termino := true
	_ = filepath.WalkDir(p, func(q string, e fs.DirEntry, err error) error {
		if time.Now().After(limite) {
			termino = false
			return filepath.SkipAll
		}
		r, rerr := filepath.Rel(dir, q)
		if rerr != nil {
			return nil
		}
		n := filepath.ToSlash(r)
		if err != nil {
			foto[n+"/"] = HuellaIlegible
			return nil
		}
		if e.IsDir() {
			return nil
		}
		foto[n] = materialgit.Huella(q)
		return nil
	})
	return termino
}

// expandirHome makes a path of providers.Estado absolute: "~/" is home; a
// relative one, or "~/" without a home, is "".
func expandirHome(p, home string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		if home == "" {
			return ""
		}
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	if !filepath.IsAbs(p) {
		return ""
	}
	return filepath.Clean(p)
}

// huellaDeClaves is the Huella of the JSON file a restricted to claves
// (providers.Corre.Claves): the sha256 of the canonical JSON of each concrete
// key path that matches and holds something (null, {} and [] are nothing),
// sorted. A file that is not there, or holds none of them, is the Huella of
// nothing; one that is not a regular file is materialgit's Huella; one that
// is not valid JSON is the whole file's.
func huellaDeClaves(a string, claves []string, maxLeido int64) string {
	fi, err := os.Lstat(a)
	if errors.Is(err, fs.ErrNotExist) {
		return huellaDeValores(nil)
	}
	if err != nil {
		return HuellaIlegible
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxLeido {
		return materialgit.Huella(a) // not decoded: too large to read whole
	}
	r, err := os.OpenRoot(filepath.Dir(a))
	if err != nil {
		return HuellaIlegible
	}
	defer r.Close()
	f, err := hoomfs.AbrirRegularEn(r, filepath.Base(a), fi)
	if err != nil {
		return HuellaIlegible
	}
	defer f.Close()
	var doc any
	dec := json.NewDecoder(io.LimitReader(f, maxLeido))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return "json-invalido:" + materialgit.Huella(a)
	}
	valores := map[string]string{}
	for _, c := range claves {
		extraer(doc, strings.Split(c, "."), "", valores)
	}
	return huellaDeValores(valores)
}

// extraer walks v along the keys camino ("*" = any one key), and keeps in out,
// by the concrete path it took, the canonical JSON of each value it reaches
// that holds something.
func extraer(v any, camino []string, hecho string, out map[string]string) {
	if len(camino) == 0 {
		if vacio(v) {
			return
		}
		b, err := json.Marshal(v) // maps are marshalled with sorted keys
		if err != nil {
			out[hecho] = HuellaIlegible
			return
		}
		out[hecho] = string(b)
		return
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	k := camino[0]
	if k != "*" {
		if hijo, ok := obj[k]; ok {
			extraer(hijo, camino[1:], hecho+"\x00"+k, out)
		}
		return
	}
	for kk, hijo := range obj {
		extraer(hijo, camino[1:], hecho+"\x00"+kk, out)
	}
}

// vacio says whether a JSON value makes nothing run: null, {} or [].
func vacio(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	}
	return false
}

// huellaDeValores is the sha256 of the sorted key paths and values.
func huellaDeValores(valores map[string]string) string {
	ks := make([]string, 0, len(valores))
	for k := range valores {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	h := sha256.New()
	for _, k := range ks {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(valores[k]))
		h.Write([]byte{0})
	}
	return "claves:" + hex.EncodeToString(h.Sum(nil))
}

// CambiosConfigAgentes lists, sorted, the paths whose entry is not the same
// in antes and despues: changed, appeared or gone; [FotoSinTerminar] when
// either photograph did not end within its bound. nil when none.
func CambiosConfigAgentes(antes, despues ConfigAgentes) []string {
	if _, ok := antes[FotoSinTerminar]; ok {
		return []string{FotoSinTerminar}
	}
	if _, ok := despues[FotoSinTerminar]; ok {
		return []string{FotoSinTerminar}
	}
	var out []string
	for p, h := range antes {
		if d, ok := despues[p]; !ok || d != h {
			out = append(out, p)
		}
	}
	for p := range despues {
		if _, ok := antes[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
