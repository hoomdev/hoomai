package agentcmd

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// ProtegidosGit are a task's protected tests as git saw them in the task's
// tree BEFORE the run (FotoGitProtegidos), by path. The disk alone does not
// say whether a test changed: a writer may commit another version and put
// the original bytes back on disk, or mark the path so git stops seeing its
// differences (finding 123262). nil = not taken: Gate then judges the disk
// alone.
type ProtegidosGit map[string]EntradaGit

// EntradaGit is one protected test in git: Indice and Head are its index
// entry and its HEAD entry ("<mode> <blob>", every stage of an index in
// conflict joined with ","; "" when absent; GitIlegible when git did not
// answer), Marca its tag in `git ls-files -v` ("H", "S" skip-worktree, a
// lowercase tag assume-unchanged; "" when not in the index), and Disco the
// blob its disk content stages as (`git hash-object`; "" when it is not a
// regular file on disk).
type EntradaGit struct {
	Indice, Head, Marca, Disco string
}

// GitIlegible is an EntradaGit field git could not answer.
const GitIlegible = "?"

// FotoGitProtegidos photographs the protected tests rutas in git, in the
// tree dir: the index (with its tags), HEAD and the blob of what the disk
// has. An empty rutas is an empty ProtegidosGit.
func FotoGitProtegidos(dir string, rutas []string, disco Protegidos) ProtegidosGit {
	foto := fotoGit(dir, rutas)
	var regulares []string
	for _, p := range rutas {
		if h := disco[p]; h != "" && h != ProtegidoAusente && h != HuellaIlegible && !strings.HasPrefix(h, HuellaNoRegular) {
			regulares = append(regulares, p)
		}
	}
	if len(regulares) == 0 {
		return foto
	}
	out, err := gitEn(dir, append([]string{"hash-object", "--"}, regulares...)...)
	shas := strings.Fields(string(out))
	if err != nil || len(shas) != len(regulares) {
		return foto
	}
	for i, p := range regulares {
		e := foto[p]
		e.Disco = shas[i]
		foto[p] = e
	}
	return foto
}

// fotoGit is the index and HEAD part of FotoGitProtegidos.
func fotoGit(dir string, rutas []string) ProtegidosGit {
	foto := ProtegidosGit{}
	if len(rutas) == 0 {
		return foto
	}
	for _, p := range rutas {
		foto[p] = EntradaGit{}
	}
	if out, err := gitEn(dir, append([]string{"--literal-pathspecs", "ls-files", "-s", "-v", "-z", "--"}, rutas...)...); err != nil {
		for _, p := range rutas {
			foto[p] = EntradaGit{Indice: GitIlegible, Marca: GitIlegible}
		}
	} else {
		// "<tag> <mode> <blob> <stage>\t<path>"
		for _, rec := range registros(out) {
			meta, p, ok := strings.Cut(rec, "\t")
			campos := strings.Fields(meta)
			e, esta := foto[p]
			if !ok || !esta || len(campos) != 4 {
				continue
			}
			ent := campos[1] + " " + campos[2]
			if e.Indice != "" {
				ent = e.Indice + "," + ent
			} else {
				e.Marca = campos[0]
			}
			e.Indice = ent
			foto[p] = e
		}
	}
	if out, err := gitEn(dir, append([]string{"--literal-pathspecs", "ls-tree", "-z", "HEAD", "--"}, rutas...)...); err != nil {
		for _, p := range rutas {
			e := foto[p]
			e.Head = GitIlegible
			foto[p] = e
		}
	} else {
		// "<mode> <type> <blob>\t<path>"
		for _, rec := range registros(out) {
			meta, p, ok := strings.Cut(rec, "\t")
			campos := strings.Fields(meta)
			e, esta := foto[p]
			if !ok || !esta || len(campos) != 3 {
				continue
			}
			e.Head = campos[0] + " " + campos[2]
			foto[p] = e
		}
	}
	return foto
}

// cambioEnGit says whether the protected test went from antes to despues in
// git: marked so git hides its differences (skip-worktree or
// assume-unchanged) when it was not, or with an index or HEAD entry that is
// neither the one before nor the blob of the disk content photographed
// before (committing the test-writer's bytes as they were is not a change).
func cambioEnGit(antes, despues EntradaGit) bool {
	if despues.Marca != antes.Marca && esconde(despues.Marca) {
		return true
	}
	return entradaCambio(antes.Indice, despues.Indice, antes.Disco) ||
		entradaCambio(antes.Head, despues.Head, antes.Disco)
}

func entradaCambio(antes, despues, disco string) bool {
	if despues == antes {
		return false
	}
	_, blob, ok := strings.Cut(despues, " ")
	return !ok || disco == "" || blob != disco
}

// esconde says whether a `git ls-files -v` tag hides the path's
// differences: S (skip-worktree) or a lowercase tag (assume-unchanged).
func esconde(tag string) bool {
	if tag == "" || tag == GitIlegible {
		return false
	}
	r := []rune(tag)[0]
	return tag == "S" || unicode.IsLower(r)
}

// registros splits git's -z output.
func registros(out []byte) []string {
	var recs []string
	for _, r := range bytes.Split(out, []byte{0}) {
		if len(r) > 0 {
			recs = append(recs, string(r))
		}
	}
	return recs
}

// topeGitProtegidos bounds each git command of the photograph: one that does
// not answer is GitIlegible, never a wait without end.
const topeGitProtegidos = 30 * time.Second

// gitEn runs git in dir, untranslated, bounded by topeGitProtegidos.
func gitEn(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), topeGitProtegidos)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "LC_ALL=C", "LANGUAGE=C")
	return cmd.Output()
}
