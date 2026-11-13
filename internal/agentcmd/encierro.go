package agentcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/materialgit"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/ratchet"
	"github.com/hoomdev/hoomai/internal/verdict"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

// EstadoDe is what prov declares of the state its CLI keeps
// (providers.ConEstado): the zero Estado when it declares none.
func EstadoDe(prov providers.Provider) providers.Estado {
	if ce, ok := prov.(providers.ConEstado); ok {
		return ce.Estado()
	}
	return providers.Estado{}
}

// ComunGit is the git common directory of the tree dir, "" when git does not
// say (outside a repository).
func ComunGit(dir string) string {
	l, err := materialgit.Ubicar(dir, time.Now().Add(10*time.Second))
	if err != nil {
		return ""
	}
	return l.Comun
}

// NombresDeGates are the names of the gates of m (the tree's hoom.yaml
// merged with its profile: the ones verify runs), sorted.
func NombresDeGates(m *manifest.Manifest) []string {
	out := make([]string, 0, len(m.Gates))
	for g := range m.Gates {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// NiegaEncierro is why an envelope of role rol, or a review, does not run
// under the policy pol with level info: the error of encierro.Validar (the
// writable paths against the main checkout root and the git common
// directory of the tree dir, gates_sin_encierro against m's gates), or
// encierro.NotaExigir when the level does not meet exigir. "" = it runs.
func NiegaEncierro(root, dir string, m *manifest.Manifest, pol manifest.EncierroPolicy, info encierro.Info, rol string) string {
	return NiegaEncierroCon(root, dir, m, pol, info, rol, nil)
}

// NiegaEncierroCon is NiegaEncierro for a role that runs on the provider
// prov (nil = none): the Escribe it declares goes through
// encierro.ValidarEscribe too, after encierro.Validar and before
// encierro.exigir (ronda 1, 425a75).
func NiegaEncierroCon(root, dir string, m *manifest.Manifest, pol manifest.EncierroPolicy, info encierro.Info, rol string, prov providers.Provider) string {
	comun := ComunGit(dir)
	if err := encierro.Validar(pol, root, comun, NombresDeGates(m)); err != nil {
		return err.Error()
	}
	if prov != nil {
		if err := encierro.ValidarEscribe(prov.Name(), EstadoDe(prov).Escribe, root, comun); err != nil {
			return err.Error()
		}
	}
	if !encierro.Alcanza(info.Nivel, pol.Exigir) {
		return encierro.NotaExigir(info, pol.Exigir, rol)
	}
	return ""
}

// encierroNiega is NiegaEncierroCon for the envelope of role rol on prov.
func encierroNiega(root, dir string, m *manifest.Manifest, pol manifest.EncierroPolicy, info encierro.Info, rol string, prov providers.Provider) string {
	return NiegaEncierroCon(root, dir, m, pol, info, rol, prov)
}

// fotosPrevias are the photographs an envelope of a role that executes
// takes before its run, for the comparisons it makes before any git of hoom
// on the tree: the agents' config of the run tree (and, blind, of the task's
// tree) and git-ajeno.
type fotosPrevias struct {
	runDir, dir      string
	config, configDe ConfigAgentes
	ajeno            GitAjeno
}

// tomarPrevias photographs, before the run, the agents' config of runDir
// (and of dir when the run is blind) and git-ajeno.
func tomarPrevias(runDir, dir string) *fotosPrevias {
	p := &fotosPrevias{runDir: runDir, dir: dir, config: FotoConfigAgentes(runDir), ajeno: FotoGitAjeno(runDir)}
	if dir != runDir {
		p.configDe = FotoConfigAgentes(dir)
	}
	return p
}

// comparar looks again, after the run and before any git of hoom on the
// tree, at what tomarPrevias photographed and, when pol.MaterialGit is not
// nil (the writer's envelope), at the git material of dir. When something
// changed it gives the scope result of the cut, and ok: those violations,
// plus what the gate can still say without git — the evidence's disk floor,
// the closings and raises, hoom.yaml and the ratchet, the protected tests on
// disk — between before (and, blind, beforeReal) and a photograph of the
// disk alone (tomarSinGit). Its findings are registered (high,
// AutorGateDeScope) with no fingerprint: computing one would run git on the
// tree. ok false (and the zero result) when nothing changed.
func (p *fotosPrevias) comparar(dir, task string, role agents.Role, before, beforeReal Snapshot, blind bool, pol Policy) (ScopeResult, bool) {
	var mat []string
	if pol.MaterialGit != nil && role.Scope == agents.ScopeCodigo {
		mat = CambiosMaterialGit(pol.MaterialGit, FotoMaterialGit(dir))
	}
	conf := CambiosConfigAgentes(p.config, FotoConfigAgentes(p.runDir))
	if p.configDe != nil {
		conf = unir(conf, CambiosConfigAgentes(p.configDe, FotoConfigAgentes(p.dir)))
	}
	ajeno := CambiosGitAjeno(p.ajeno)
	if len(mat)+len(conf)+len(ajeno) == 0 {
		return ScopeResult{}, false
	}
	after := tomarSinGit(p.runDir, before)
	sc := CheckScope(before, after, pol)
	arboles := []Fotos{{Antes: before, Despues: after}}
	if blind {
		arboles = append(arboles, Fotos{Antes: beforeReal, Despues: tomarSinGit(dir, beforeReal)})
	}
	if role.Slug != finding.RolQueRefuta {
		sc = sinCierres(sc, role, arboles...)
	}
	sc = sinSubidas(sc, role, arboles...)
	sc = pisoEnDisco(sc, arboles...)
	sc = sinTocarProtegidos(sc, dir, role, pol.Protegidos, nil)
	sc = conRegla(sc, mat, RuleMaterialDeGit, DetalleMaterialDeGit)
	sc = conRegla(sc, conf, RuleConfigDeAgentes, DetalleConfigDeAgentes)
	sc = conRegla(sc, ajeno, RuleGitAjeno, DetalleGitAjeno)
	return registrarViolaciones(dir, task, role, sc, ""), true
}

// tomarSinGit photographs the tree root like Take but without git: the
// evidence's disk floor, hoom.yaml and the ratchet. Its Touched is before's
// (nothing listed, so nothing more counts as touched) and it has no
// Listado: the comparison before any git of hoom on the tree uses it.
func tomarSinGit(root string, before Snapshot) Snapshot {
	s := Snapshot{Touched: map[string]string{}, Evidence: map[string]bool{}, Huellas: map[string]string{}, Directorios: map[string]bool{}}
	for p, h := range before.Touched {
		s.Touched[p] = h
	}
	switch r, err := os.OpenRoot(root); {
	case err == nil:
		s.fotoEvidencia(r)
		r.Close()
	case !errors.Is(err, fs.ErrNotExist):
		s.sinEvidenciaLegible()
	}
	if raw, err := os.ReadFile(filepath.Join(root, manifest.FileName)); err == nil {
		sum := sha256.Sum256(raw)
		s.Manifest = hex.EncodeToString(sum[:])
	}
	rf, err := ratchet.Load(root)
	if err != nil {
		rf = &ratchet.File{}
	}
	s.Ratchet = rf
	return s
}

// notaDelCorte is the note of an envelope whose scope cuts (Cuts): the first
// that applies of the tampering note, the isolation note,
// NotaTestsDelTestWriter, NotaMaterialDeGit, NotaConfigDeAgentes(rol) and
// NotaGitAjeno(rol).
func notaDelCorte(sc ScopeResult, rol string) string {
	switch {
	case sc.Broken():
		return "el aislamiento se rompio: no se emite veredicto sobre este arbol"
	case sc.Tampering:
	case sc.has(RuleTestsDelTestWriter):
		// the test-writer's tests: with the evidence also moved, the note
		// stays the tampering one
		return NotaTestsDelTestWriter
	case sc.has(RuleMaterialDeGit):
		return NotaMaterialDeGit
	case sc.has(RuleConfigDeAgentes):
		return NotaConfigDeAgentes(rol)
	case sc.has(RuleGitAjeno):
		return NotaGitAjeno(rol)
	}
	return "manipulacion de la evidencia: no se emite veredicto sobre este arbol"
}

// registrarViolaciones registers, for each violation of sc that has no
// finding yet, the scope gate's high finding (DescripcionViolacion), with
// huella as its fingerprint.
func registrarViolaciones(dir, task string, role agents.Role, sc ScopeResult, huella string) ScopeResult {
	for i, v := range sc.Violations {
		if v.FindingID != "" {
			continue
		}
		if f, err := finding.Register(dir, "", finding.Draft{Severity: "high", Lens: "risk", File: v.Path,
			Description: DescripcionViolacion(v, role.Slug), Author: AutorGateDeScope, Task: task, Huella: &huella}); err == nil {
			sc.Violations[i].FindingID = f.ID
		}
	}
	return sc
}

// registrarCorte adds to sc one violation of regla (with detalle) for each
// path of rutas and registers its finding: the cut of the writer's envelope
// whose verify changed the git material.
func registrarCorte(dir, task string, role agents.Role, sc ScopeResult, rutas []string, regla, detalle string) ScopeResult {
	sc = conRegla(sc, rutas, regla, detalle)
	return registrarViolaciones(dir, task, role, sc, "")
}

// MaterialDelVeredicto lists the paths of the gate verifycmd.GateMaterialDeGit
// of v when it is in fail (one per line of its output), nil otherwise.
func MaterialDelVeredicto(v *verdict.Verdict) []string {
	return rutasDelGate(v, verifycmd.GateMaterialDeGit)
}

// rutasDelGate lists the paths of the gate named gate of v when it is in
// fail (one per line of its output; FotoSinTerminar when it has none), nil
// otherwise.
func rutasDelGate(v *verdict.Verdict, gate string) []string {
	if v == nil {
		return nil
	}
	for _, g := range v.Gates {
		if g.Name != gate || g.Status != verdict.StatusFail {
			continue
		}
		var out []string
		for _, l := range strings.Split(g.OutputTail, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
		if len(out) == 0 {
			out = []string{FotoSinTerminar}
		}
		return out
	}
	return nil
}

// GitAjenoDelVeredicto (ronda 1, aclaracion 20) lists the paths of the gate
// verifycmd.GateGitAjeno of v when it is in fail (one per line of its
// output), nil otherwise: the writer's envelope then closes in verify with
// the git-ajeno finding (DeGitAjeno, so the pilot marks TocoGitAjeno), and
// the pilot's cerrar-hallazgos stops with its red-verdict text before its
// save.
func GitAjenoDelVeredicto(v *verdict.Verdict) []string {
	return rutasDelGate(v, verifycmd.GateGitAjeno)
}

// unir is the sorted union of a and b, without repeats.
func unir(a, b []string) []string {
	visto := map[string]bool{}
	var out []string
	for _, x := range append(append([]string(nil), a...), b...) {
		if !visto[x] {
			visto[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
