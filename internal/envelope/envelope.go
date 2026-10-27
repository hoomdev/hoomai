// Package envelope keeps the durable record of a `hoom agent` run. The
// envelope is what actually runs a role — five steps, six when the role runs
// blind — and until now it left no trace anybody could look at while it
// worked: its Result was printed and gone. From another terminal `hoom status`
// saw an anonymous run, and the Studio did not even see that.
//
// The record answers, at any moment and from any process, the question the
// terminal output can only answer to whoever is watching it: WHO is running,
// with WHICH provider, over WHICH spec, at WHICH step, and how it ended.
//
// It is telemetry, not evidence: it lives in .hoom/envelopes/, outside Git,
// outside the change candidate and outside the fingerprint — same contract as
// .hoom/runs/. Writing it can never break the envelope it describes, so every
// write is best-effort by design.
package envelope

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/providers"
)

// Status values of a record. Same vocabulary the envelope already prints.
const (
	StatusRunning        = "en curso"
	StatusDeliverable    = "entregable"
	StatusNotDeliverable = "no-entregable"
	// StatusNoDelivery: a role that writes finished its run without changing
	// anything but the evidence hoom itself generates. There is no new tree
	// to certify, so there is no verdict — and that is not the same as a red
	// one.
	StatusNoDelivery = "sin-entrega"
)

// DirName is where the records live, under .hoom/.
const DirName = "envelopes"

// Record is one envelope invocation, as far as it got.
type Record struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Provider string `json:"provider"`
	Task     string `json:"task,omitempty"`
	Dir      string `json:"dir"`
	Spec     string `json:"spec,omitempty"`
	Approval string `json:"approval,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	// Isolated says the role ran blind, and from which commit: the guarantee
	// the Spec D built is also part of the envelope's identity.
	Isolated     bool             `json:"isolated,omitempty"`
	IsolatedFrom string           `json:"isolated_from,omitempty"`
	VerdictID    string           `json:"verdict_id,omitempty"`
	Verdict      string           `json:"verdict,omitempty"`
	Usage        *providers.Usage `json:"usage,omitempty"`
	// Stage is the step the envelope is in or stopped at, with the same names
	// it prints: spec | aislar | run | scope | verify | check | ok.
	Stage     string    `json:"stage"`
	Step      int       `json:"step"`
	Steps     int       `json:"steps"`
	Status    string    `json:"status"`
	ExitCode  int       `json:"exit_code"`
	Note      string    `json:"note,omitempty"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// PID is the process that owns the envelope (brecha 4 of the cabin RFC):
	// with it, "interrupted" is a fact the moment that process dies instead
	// of an inference from a silent heartbeat. 0 in records of older hoom.
	PID int `json:"pid,omitempty"`
	// Pilot says the cabin's belt (auto: hasta-humano) launched this
	// envelope, not a person.
	Pilot   bool      `json:"piloto,omitempty"`
	EndedAt time.Time `json:"ended_at,omitempty"`
}

// Done reports a record that reached an end. Anything else is still "en
// curso" — which does NOT mean alive: whoever reads it decides how much to
// trust an old UpdatedAt, and hoom never closes what it did not see close.
func (r Record) Done() bool { return r.Status != "" && r.Status != StatusRunning }

// idRe is the shape of an envelope id: one file name, no separators, no
// dots that climb, nothing a shell would read.
var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// NewID mints a record id with the same shape as a run's: sortable by name
// and unique per invocation.
func NewID() string {
	raw := make([]byte, 3)
	rand.Read(raw)
	return time.Now().UTC().Format("20060102T150405") + "_" + hex.EncodeToString(raw)
}

func dir(root string) string { return filepath.Join(root, ".hoom", DirName) }

// Write records the envelope's state. Best-effort by contract (CA-202): a
// record that cannot be written never breaks the envelope it describes, so
// callers may ignore the error. The one that cannot is the Started handshake
// (CA-335): it promises the first record is on disk.
func Write(root string, rec Record) error {
	if strings.TrimSpace(rec.ID) == "" {
		return nil
	}
	raw, err := registro(rec)
	if err != nil {
		return err
	}
	// la raiz del proyecto se crea si falta, como siempre; .hoom y
	// .hoom/envelopes se crean y se recorren por descriptores: un symlink a
	// otro proyecto no se sigue, ni en el primer intento (hallazgo 4f418f)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	r, err := hoomfs.AbrirOCrearRuta(root, ".hoom", DirName)
	if err != nil {
		return err
	}
	defer r.Close()
	// La regla queda escrita (CA-202) sin reescribir un archivo completo:
	// registrar telemetria no mueve el candidato (CA-203).
	hoomfs.EnsureIgnored(root, DirName)
	// Atomico: status y el Studio leen este archivo MIENTRAS se escribe, y un
	// sobre a medias leido como vacio desapareceria de la lista.
	esperarEscritura(root, rec.ID)
	if err := hoomfs.PublicarEn(r, rec.ID+".json", raw, 0o644); err != nil {
		return err
	}
	// el directorio en que se publico tiene que seguir siendo el de
	// root/.hoom/envelopes: uno que se movio mientras se escribia dejaria el
	// registro fuera de su lugar y Write diria que esta (hallazgo f89080)
	return hoomfs.SigueEn(root, r, ".hoom", DirName)
}

// registro is rec as hoom writes it: one format for the first try (Write)
// and every retry (hecho), from one place (finding 812a1b), stamped with the
// time of each call, so two tries write the same record with different
// UpdatedAt (finding a6d413).
func registro(rec Record) ([]byte, error) {
	rec.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// List returns the project's envelope records, newest first. A file that is
// unreadable, of another shape or without an id is skipped, and a project
// that never ran an envelope gets an empty list instead of an error: broken
// telemetry never breaks a command.
func List(root string) []Record {
	out, _ := ListErr(root)
	return out
}

// maxRecord caps one record file: hoom writes a few hundred bytes.
const maxRecord = 1 << 20

// ListErr is List for whoever must not take "unreadable" for "none" (the
// doctor, finding d0d331): an envelopes directory that exists and cannot be
// listed, or that is a symlink or not a directory, is an error, and so is a
// record it could not open or read (findings 9e7983 and a2b180) — the records
// it could read still come back. One that does not exist yet is no envelopes.
// The directory and every record are read by descriptors, never following a
// symlink: a record linked to one of another project would publish that
// project's telemetry as this one's (finding 3826b3). A file whose name is
// not a record id is not hoom's and is skipped. One whose name IS a record id
// and that is not a record (not JSON, truncated, without id) is unreadable,
// never "none": that is what a partial write leaves, and it would hide an
// orphan envelope from the doctor (findings 0cbf2e, 25ba18 -> 26eef9). A
// whole record whose id is not its file's name is no write of hoom's (a copy,
// a planted file): it is skipped, so it can neither reach a command nor stop
// the doctor (finding f34bc3).
func ListErr(root string) ([]Record, error) {
	var out []Record
	var sinLeer []string
	err := hoomfs.RecorrerDirectorio(root, []string{".hoom", DirName},
		func(n string) bool { return strings.HasSuffix(n, ".json") }, maxRecord,
		func(e hoomfs.Entrada) {
			if e.Err != nil {
				sinLeer = append(sinLeer, fmt.Sprintf("%s: %v", e.Nombre, e.Err))
				return
			}
			id := strings.TrimSuffix(e.Nombre, ".json")
			if !idRe.MatchString(id) {
				return
			}
			var rec Record
			if json.Unmarshal(e.Contenido, &rec) != nil || strings.TrimSpace(rec.ID) == "" {
				sinLeer = append(sinLeer, fmt.Sprintf("%s: no es un registro (JSON ilegible o sin id)", e.Nombre))
				return
			}
			// the file name is the address and the content only describes
			// it: a record whose id is not its file's name never reaches a
			// command the doctor proposes (finding f34bc3)
			if rec.ID != id {
				return
			}
			out = append(out, rec)
		})
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer .hoom/%s: %v", DirName, err)
	}
	// stable: records of the same instant keep the name order they were
	// read in (finding 3cb01b)
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if len(sinLeer) > 0 {
		return out, fmt.Errorf("no se pudieron leer %d registro(s) de .hoom/%s: %s", len(sinLeer), DirName, strings.Join(sinLeer, "; "))
	}
	return out, nil
}

// esperasCierre is how long a close that did not reach the disk waits before
// each new try of its own; past the last one it keeps trying at that pace.
var esperasCierre = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second}

// esperaCierre is the wait before try n+1 of one close.
func esperaCierre(n int) time.Duration {
	if n < len(esperasCierre) {
		return esperasCierre[n]
	}
	return esperasCierre[len(esperasCierre)-1]
}

// Cerrar writes the closing record of an envelope. Its owner's PID keeps an
// open record alive (CA-333): a close that never reaches the disk would leave
// the envelope "en curso" for as long as that process lives, which for one
// the Studio launched is the whole life of the server (finding de4843). So a
// close that fails is not dropped: it joins the pending closes, each retried
// at its own pace (esperasCierre) until it lands, its record is gone or is no
// longer a regular file, or the process ends — and then the dead PID is what
// says the envelope stopped. A close that lands takes an older pending close
// of the same record out of the queue, so a retry never writes back a stale
// state (finding b7417b). The write and what it does to the queue happen
// under the record's lock, so two Cerrar of one record settle the queue in
// the order they wrote (findings 4647ba, 22806c). Cerrar returns its own
// try's error; the record stays best-effort for the envelope (CA-202).
func Cerrar(root string, rec Record) error {
	if strings.TrimSpace(rec.ID) == "" {
		return Write(root, rec)
	}
	path := filepath.Join(dir(root), rec.ID+".json")
	unlock := pendientes.escritura(path)
	defer unlock()
	err := Write(root, rec)
	if err != nil {
		pendientes.agregar(path, root, rec)
	} else {
		pendientes.quitar(path)
	}
	return err
}

// cierres is the set of closes still to write, one per record file, and the
// worker that schedules them. Each try runs on its own goroutine, so a write
// the file system leaves hanging holds back only its own record, never the
// others (finding 1e0674); the worker itself only sleeps until the next one
// is due or a new close arrives (finding eafa2d). Writes of one record file
// go one at a time (escritura), so a try still in flight can never land
// after a newer close. At most maxEnVuelo tries of one project are in flight
// at once: the rest of that project wait for one to end (finding da912f),
// and a project whose writes hang never holds back another's (finding
// ab66c7).
type cierres struct {
	mu         sync.Mutex
	porArchivo map[string]*cierre
	candados   map[string]*candado
	enVuelo    map[string]int // tries in flight, by project root
	trabajando bool
	despertar  chan struct{}
	seq        uint64
}

// maxEnVuelo bounds the tries in flight of one project. Its writes hang for
// one cause (its .hoom, its file system), so past this many hanging at once
// another goroutine would only hang too; other projects keep their own.
const maxEnVuelo = 4

// candado is the write lock of one record file, with how many hold it or
// wait for it: the last one out takes it out of the map, so the map is the
// size of the writes going on, not of every record ever closed (findings
// 92fa84, 8bc636).
type candado struct {
	mu   sync.Mutex
	usos int
}

// cierre is one pending close.
type cierre struct {
	root    string
	rec     Record
	seq     uint64 // tells this close from a newer one of the same record
	intento int    // tries already made
	proximo time.Time
	enVuelo bool
}

var pendientes = &cierres{porArchivo: map[string]*cierre{}, candados: map[string]*candado{}, enVuelo: map[string]int{}, despertar: make(chan struct{}, 1)}

// escritura takes the write lock of the record at path and returns its
// release. Lock order: a record's lock before c.mu, never the other way.
func (c *cierres) escritura(path string) func() {
	c.mu.Lock()
	k, ok := c.candados[path]
	if !ok {
		k = &candado{}
		c.candados[path] = k
	}
	k.usos++
	c.mu.Unlock()
	k.mu.Lock()
	return func() {
		k.mu.Unlock()
		c.mu.Lock()
		k.usos--
		if k.usos == 0 {
			delete(c.candados, path)
		}
		c.mu.Unlock()
	}
}

// agregar queues the close of the record at path, replacing an older one of
// the same record, and makes sure the worker sees it.
func (c *cierres) agregar(path, root string, rec Record) {
	c.mu.Lock()
	c.seq++
	c.porArchivo[path] = &cierre{root: root, rec: rec, seq: c.seq, proximo: time.Now().Add(esperaCierre(0))}
	if !c.trabajando {
		c.trabajando = true
		go c.trabajar()
	}
	c.mu.Unlock()
	c.avisar()
}

// quitar drops the pending close of the record at path: a newer close
// landed.
func (c *cierres) quitar(path string) {
	c.mu.Lock()
	delete(c.porArchivo, path)
	c.mu.Unlock()
}

// avisar wakes the worker without ever blocking.
func (c *cierres) avisar() {
	select {
	case c.despertar <- struct{}{}:
	default:
	}
}

// trabajar launches every close that is due and sleeps until the next one,
// until none is left.
func (c *cierres) trabajar() {
	for {
		c.mu.Lock()
		if len(c.porArchivo) == 0 {
			c.trabajando = false
			c.mu.Unlock()
			return
		}
		ahora := time.Now()
		siguiente := ahora.Add(esperaCierre(len(esperasCierre)))
		for path, ci := range c.porArchivo {
			vencido := !ci.proximo.After(ahora)
			switch {
			case ci.enVuelo:
			case vencido && c.enVuelo[ci.root] < maxEnVuelo:
				ci.enVuelo = true
				c.enVuelo[ci.root]++
				go c.intentar(path, ci.seq, ci.root)
			case vencido:
				// no slot: the try that ends wakes the worker
			case ci.proximo.Before(siguiente):
				siguiente = ci.proximo
			}
		}
		c.mu.Unlock()
		select {
		case <-time.After(time.Until(siguiente)):
		case <-c.despertar:
		}
	}
}

// intentar makes one try of the close seq of the record at path, settles it
// or schedules its next try, and frees its slot of proyecto's tries in
// flight.
func (c *cierres) intentar(path string, seq uint64, proyecto string) {
	unlock := c.escritura(path)
	c.mu.Lock()
	ci, ok := c.porArchivo[path]
	vigente := ok && ci.seq == seq
	var root string
	var rec Record
	if vigente {
		root, rec = ci.root, ci.rec
	}
	c.mu.Unlock()
	listo := !vigente || hecho(root, rec)
	unlock()

	c.mu.Lock()
	if c.enVuelo[proyecto]--; c.enVuelo[proyecto] <= 0 {
		delete(c.enVuelo, proyecto)
	}
	if ci, ok := c.porArchivo[path]; ok && ci.seq == seq {
		if listo {
			delete(c.porArchivo, path)
		} else {
			ci.enVuelo = false
			ci.intento++
			ci.proximo = time.Now().Add(esperaCierre(ci.intento))
		}
	}
	c.mu.Unlock()
	c.avisar()
}

// hecho tries one pending close and says whether it is settled: written, or
// nothing left to close — the record is gone, or something else took its
// name (a directory, a symlink: not this close's to overwrite). A failure to
// look at it is not settled: storage that fails once may come back (finding
// be5b7a). A retry never creates the directories back: a project removed
// under it stays removed.
//
// The retry walks to the record by descriptors and publishes in that very
// directory (hoomfs.ReemplazarSiSigue): a .hoom or envelopes swapped for a
// symlink after the first try never takes the write to another project
// (finding 33f0e0 -> 28f494), and a record deleted after it was looked at is
// not written back (findings 9e157f, 3fd1d4 -> 332b11). The retry does not
// touch .hoom/.gitignore: the first try already did.
func hecho(root string, rec Record) bool {
	for _, p := range []string{filepath.Join(root, ".hoom"), dir(root)} {
		st, err := os.Lstat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return true
		case err != nil:
			return false
		case !st.IsDir():
			return true
		}
	}
	r, err := hoomfs.AbrirRuta(root, ".hoom", DirName)
	if err != nil {
		// it changed since the look: the next try sees what it is
		return false
	}
	defer r.Close()
	name := rec.ID + ".json"
	antes, err := r.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true
	case err != nil:
		return false
	case !antes.Mode().IsRegular():
		return true
	}
	raw, err := registro(rec)
	if err != nil {
		return true // it never will: not a close to retry
	}
	esperarEscritura(root, rec.ID)
	err = hoomfs.ReemplazarSiSigue(r, name, raw, 0o644, antes)
	return err == nil || errors.Is(err, hoomfs.ErrCambio)
}

// antesDeEscribir, when set, runs right before every write of a record (the
// first try in Write, every retry in hecho), with its project and id. It
// exists only for the tests: no write of a record blocks on a FIFO anymore
// (findings c6c300, 28f494), and what Cerrar and its worker promise about a
// write that hangs (findings 1e0674, da912f, ab66c7, 4647ba) still needs one
// to be shown. Atomic, because the worker outlives a test.
var antesDeEscribir atomic.Pointer[func(root, id string)]

// esperarEscritura runs antesDeEscribir, if set.
func esperarEscritura(root, id string) {
	if f := antesDeEscribir.Load(); f != nil {
		(*f)(root, id)
	}
}
