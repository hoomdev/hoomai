// Package hoomfs is the single source of truth for what under .hoom is
// LOCAL — telemetry and scaffolding that never travel in Git nor enter the
// change candidate — and for the one way hoom writes those files: atomically.
// Five packages used to carry their own copy of the local list; a directory
// added in one and forgotten in another meant a run charged to the role, a
// fingerprint that moved, or an untracked file blocking a task.
package hoomfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Local names the directories under .hoom/ that stay on this machine: the
// live cache, the narration of runs, task worktrees, blind trees and
// envelope records. Evidence (verdicts, findings, approvals) is NOT here:
// it travels.
var Local = []string{"cache", "worktrees", "runs", "isolated", "envelopes"}

// IsLocal reports whether a slash path relative to the project root points
// inside a local directory.
func IsLocal(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, d := range Local {
		if rel == ".hoom/"+d || strings.HasPrefix(rel, ".hoom/"+d+"/") {
			return true
		}
	}
	return false
}

// GitignoreBody is the canonical content of .hoom/.gitignore.
func GitignoreBody() string {
	var b strings.Builder
	for _, d := range Local {
		b.WriteString(d + "/\n")
	}
	return b.String()
}

// EnsureIgnored completes .hoom/.gitignore with the named local directories
// it lacks (all of them when none is named) and returns what it added. A file
// that already has them is never rewritten, not even byte-identically:
// registering telemetry must not move the candidate. Callers that only need
// their own entry ask for it by name, so opening a quarantine never edits the
// file for a directory it does not use.
func EnsureIgnored(root string, names ...string) (added []string, err error) {
	if len(names) == 0 {
		names = Local
	}
	gi := filepath.Join(root, ".hoom", ".gitignore")
	raw, _ := os.ReadFile(gi)
	have := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	for _, d := range names {
		if !have[d+"/"] {
			added = append(added, d+"/")
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(gi), 0o755); err != nil {
		return nil, err
	}
	content := string(raw)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(added, "\n") + "\n"
	return added, AtomicWrite(gi, []byte(content), 0o644)
}

// crearTemporal creates, next to path, the hidden temp file a whole write
// goes through: "." + the name of path + ".tmp-" + the digits os.CreateTemp
// puts in place of *, the one form TemporalDe recognizes (finding 6b1f72).
// It never ends in the target's extension, so listings that filter by
// suffix never pick it up.
func crearTemporal(path string) (*os.File, error) {
	return os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
}

// TemporalDe says whether name is a temp file of AtomicWrite or
// PublicarNuevo, exactly as they write it, and the name of the file it will
// become. Anything else is not one of hoom's writes in progress.
func TemporalDe(name string) (base string, ok bool) {
	rest, ok := strings.CutPrefix(name, ".")
	i := strings.LastIndex(rest, ".tmp-")
	if !ok || i <= 0 {
		return "", false
	}
	digitos := rest[i+len(".tmp-"):]
	if digitos == "" || strings.Trim(digitos, "0123456789") != "" {
		return "", false
	}
	return rest[:i], true
}

// AtomicWrite writes data to path through a temp file in the same directory
// and a rename: a concurrent reader sees the previous complete file or the
// new complete file, never a truncated one, and a crash mid-write leaves the
// previous content in place.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := crearTemporal(path)
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// PublicarNuevo creates path with data, whole or not at all, and never
// replaces a file already there: then it fails with an error that is
// os.ErrExist. The data goes to a temp file that is closed and then linked
// to path, so path appears complete; on a filesystem without hard links
// (exFAT, some network mounts) path is created with O_EXCL instead and
// removed again if the write fails (findings c6ddec, d2c506). enlazar is
// os.Link, or a test's stand-in.
func PublicarNuevo(path string, data []byte, mode os.FileMode, enlazar func(oldname, newname string) error) error {
	tmp, err := crearTemporal(path)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), mode)
	}
	if err != nil {
		return err
	}
	if err := enlazar(tmp.Name(), path); err == nil || errors.Is(err, os.ErrExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// ErrLocked: another process holds the lock and did not let go in time.
var ErrLocked = errors.New("otro proceso tiene tomado")

// Lock takes an exclusive lock on the file at path, across processes (the
// CLI and the Studio) and goroutines, waiting up to wait. It is the kernel's
// lock (flock, LockFileEx): a holder that dies releases it with its process,
// so there is no stale lock to guess, and releasing (the returned func) only
// closes this holder's descriptor. The file stays; it lives under
// .hoom/cache/locks/ (LockPath), local and ignored.
func Lock(path string, wait time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		ok, err := tryLock(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if ok {
			return func() { f.Close() }, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("%w %s", ErrLocked, path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// LockPath is where the lock called name of the project at root lives.
func LockPath(root, name string) string {
	return filepath.Join(root, ".hoom", "cache", "locks", name+".lock")
}
