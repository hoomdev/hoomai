package hoomfs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Hallazgo 20260925T041933_51d902 (CA-349, CA-343: el candado que serializa
// MarkDone/AddSession y task done). Soltar un candado nunca suelta el de otro:
// ni un unlock repetido ni uno tardio dejan entrar a un tercero mientras el
// dueño actual lo tiene.
func TestLock_SoltarNoSueltaElCandadoDeOtro(t *testing.T) {
	path := LockPath(t.TempDir(), "s")
	soltarA, err := Lock(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(path, 50*time.Millisecond); !errors.Is(err, ErrLocked) {
		t.Fatalf("51d902: con A adentro, B espera y no entra: %v", err)
	}
	soltarA()
	soltarB, err := Lock(path, time.Second)
	if err != nil {
		t.Fatalf("51d902: A solto, B entra: %v", err)
	}
	soltarA() // tardio o repetido: no es de A
	if _, err := Lock(path, 50*time.Millisecond); !errors.Is(err, ErrLocked) {
		t.Fatalf("51d902: el unlock tardio de A no deja entrar a un tercero con B adentro: %v", err)
	}
	soltarB()
	if soltar, err := Lock(path, time.Second); err != nil {
		t.Fatalf("51d902: B solto, entra el siguiente: %v", err)
	} else {
		soltar()
	}
}

// Hallazgo 20260925T041933_51d902 (CA-349). Muchos a la vez: uno solo adentro.
func TestLock_UnoSoloAdentro(t *testing.T) {
	path := LockPath(t.TempDir(), "s")
	var adentro, maximo int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			soltar, err := Lock(path, 10*time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			n := atomic.AddInt32(&adentro, 1)
			for {
				m := atomic.LoadInt32(&maximo)
				if n <= m || atomic.CompareAndSwapInt32(&maximo, m, n) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt32(&adentro, -1)
			soltar()
		}()
	}
	wg.Wait()
	if maximo != 1 {
		t.Fatalf("51d902: %d adentro a la vez", maximo)
	}
}

// Hallazgo 20260925T041933_51d902 (CA-349). Un dueño que muere sin soltar no
// deja el candado tomado: no hay "vencido" que adivinar.
func TestLock_ElDuenoQueMuereLoSuelta(t *testing.T) {
	if p := os.Getenv("HOOMFS_LOCK_HELPER"); p != "" {
		if _, err := Lock(p, time.Second); err != nil {
			os.Exit(3)
		}
		os.Exit(0) // muere con el candado tomado
	}
	path := LockPath(t.TempDir(), "s")
	cmd := exec.Command(os.Args[0], "-test.run=^TestLock_ElDuenoQueMuereLoSuelta$")
	cmd.Env = append(os.Environ(), "HOOMFS_LOCK_HELPER="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("el proceso dueño no tomo el candado: %v\n%s", err, out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("el proceso dueño dejo su archivo de candado: %v", err)
	}
	soltar, err := Lock(path, time.Second)
	if err != nil {
		t.Fatalf("51d902: el dueño murio, el candado es libre: %v", err)
	}
	soltar()
}

// LockPath vive bajo .hoom/cache/locks/: local e ignorado, nunca junto a la
// evidencia (.hoom/items/ queda con sus items y nada mas).
func TestLock_LockPathEnCache(t *testing.T) {
	got := filepath.ToSlash(LockPath("/r", "item-x"))
	if got != "/r/.hoom/cache/locks/item-x.lock" {
		t.Fatalf("LockPath = %s", got)
	}
}
