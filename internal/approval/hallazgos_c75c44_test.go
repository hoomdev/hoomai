//go:build linux || darwin

// Regresion de los hallazgos c75c44, b2072a y 6d7433 (ronda 1 de
// proporcion-por-riesgo): Aprobar lee el spec UNA vez, y lo que mide, lo que
// decide que ya estaba aprobado y lo que firma son esos mismos bytes. El spec
// es un FIFO que entrega otro contenido en cada apertura: asi el spec "cambia
// entre lecturas" sin carreras, y cada apertura se cuenta.
package approval_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
)

func h75Corto() string {
	return "# Spec: x\n\n## Objetivo\n\nuno dos tres\n\n## Criterios de aceptacion\n\n- CA-1: algo\n"
}

// h75Largo pasa el tope de palabras de riesgo normal (800).
func h75Largo() string {
	return "# Spec: x\n\n## Objetivo\n\n" + strings.Repeat("palabra ", 900) + "\n\n## Criterios de aceptacion\n\n- CA-1: algo\n"
}

func h75Sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// h75Servir pone en ruta un FIFO que entrega contenidos[i] a la apertura i
// (el ultimo a las siguientes). Antes de escribir cada contenido deja en ruta
// un FIFO nuevo: el lector de esta apertura conserva el suyo y la siguiente
// apertura no se mezcla con ella. Devuelve las aperturas y el que lo para.
func h75Servir(t *testing.T, ruta string, contenidos ...string) (func() int32, func()) {
	t.Helper()
	tmp := t.TempDir()
	var nuevos int32
	fifo := func() error {
		p := filepath.Join(tmp, fmt.Sprintf("fifo%d", atomic.AddInt32(&nuevos, 1)))
		if err := syscall.Mkfifo(p, 0o600); err != nil {
			return err
		}
		return os.Rename(p, ruta)
	}
	_ = os.Remove(ruta)
	if err := fifo(); err != nil {
		t.Fatal(err)
	}
	var n int32
	parar, hecho := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(hecho)
		for i := 0; ; i++ {
			w, err := os.OpenFile(ruta, os.O_WRONLY, 0) // espera al proximo lector
			if err != nil {
				t.Errorf("abrir el FIFO: %v", err)
				return
			}
			select {
			case <-parar:
				_ = w.Close()
				return
			default:
			}
			atomic.AddInt32(&n, 1)
			if err := fifo(); err != nil {
				t.Errorf("FIFO nuevo: %v", err)
			}
			_, _ = w.WriteString(contenidos[min(i, len(contenidos)-1)])
			_ = w.Close()
		}
	}()
	return func() int32 { return atomic.LoadInt32(&n) }, func() {
		close(parar)
		// un lector propio destraba al escritor que espera
		r, err := os.OpenFile(ruta, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		<-hecho
		if err == nil {
			_ = r.Close()
		}
		_ = os.Remove(ruta)
	}
}

func h75Raiz(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(root, "docs", "x.md")
}

// h75SinAprobar: el contenido de ruta no tiene aprobacion vigente.
func h75SinAprobar(t *testing.T, hallazgo, root, ruta, contenido string) {
	t.Helper()
	if err := os.WriteFile(ruta, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
	if st, _, err := approval.Status(root, ruta); err != nil || st == approval.StatusApproved {
		t.Fatalf("hallazgo %s: el spec largo (900 palabras, tope 800) no queda aprobado sin medirse: %s %v", hallazgo, st, err)
	}
}

// hallazgos c75c44 y b2072a: el spec corto esta aprobado y pasa a ser el
// largo entre la lectura que lo ve aprobado y la que firma. Aprobar lee una
// vez: es el corto, ya aprobado, sin registro para el largo.
func TestHallazgo_c75c44_AprobadoNoFirmaLoQueNoLeyo(t *testing.T) {
	root, ruta := h75Raiz(t)
	if err := os.WriteFile(ruta, []byte(h75Corto()), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := approval.Aprobar(nil, root, ruta); err != nil {
		t.Fatalf("fixture: el corto se aprueba: %v", err)
	}
	aperturas, parar := h75Servir(t, ruta, h75Corto(), h75Largo())
	rec, already, err := approval.Aprobar(nil, root, ruta)
	parar()
	if err != nil {
		t.Fatalf("hallazgo c75c44: el corto ya aprobado se aprueba otra vez sin error: %v", err)
	}
	if !already || rec.SHA256 != h75Sha(h75Corto()) {
		t.Fatalf("hallazgo c75c44: firma lo que leyo (el corto, ya aprobado): already=%v sha256 %s", already, rec.SHA256)
	}
	if n := aperturas(); n != 1 {
		t.Fatalf("hallazgo c75c44: Aprobar lee el spec una vez, lo abrio %d", n)
	}
	h75SinAprobar(t, "b2072a", root, ruta, h75Largo())
}

// hallazgo 6d7433: el spec nunca se aprobo; si cambia entre lo que se mide y
// lo que se firma, la firma es de lo medido. Con una sola lectura, el corto
// se mide y se firma el corto.
func TestHallazgo_6d7433_SinAprobarFirmaLoQueMidio(t *testing.T) {
	root, ruta := h75Raiz(t)
	// las cuatro primeras aperturas son el corto: antes del arreglo, la
	// quinta (la de Approve) era el largo
	aperturas, parar := h75Servir(t, ruta, h75Corto(), h75Corto(), h75Corto(), h75Corto(), h75Largo())
	rec, already, err := approval.Aprobar(nil, root, ruta)
	parar()
	if err != nil {
		t.Fatalf("hallazgo 6d7433: el corto pasa su tope y se aprueba: %v", err)
	}
	if already || rec.SHA256 != h75Sha(h75Corto()) {
		t.Fatalf("hallazgo 6d7433: firma lo que midio (el corto): already=%v sha256 %s", already, rec.SHA256)
	}
	if n := aperturas(); n != 1 {
		t.Fatalf("hallazgo 6d7433: Aprobar lee el spec una vez, lo abrio %d", n)
	}
	h75SinAprobar(t, "6d7433", root, ruta, h75Largo())

	// y al reves: si lo que lee es el largo, se niega y no escribe registro
	root2, ruta2 := h75Raiz(t)
	_, parar2 := h75Servir(t, ruta2, h75Largo(), h75Corto())
	_, _, err = approval.Aprobar(nil, root2, ruta2)
	parar2()
	var neg *approval.Negativa
	if !errors.As(err, &neg) {
		t.Fatalf("hallazgo 6d7433: lo que lee es el largo y pasa su tope: es una *Negativa, fue %v", err)
	}
	if ents, _ := os.ReadDir(filepath.Join(root2, ".hoom", "approvals")); len(ents) != 0 {
		t.Fatalf("hallazgo 6d7433: la negativa no escribe registro, hay %d", len(ents))
	}
}

// hallazgo c75c44 (variante latente): una ruta relativa es del root, para
// medirla y para firmarla, aunque hoom corra en otro directorio donde la
// misma ruta relativa es otro archivo.
func TestHallazgo_c75c44_RutaRelativaEsDelRootAlFirmar(t *testing.T) {
	root, ruta := h75Raiz(t)
	if err := os.WriteFile(ruta, []byte(h75Corto()), 0o644); err != nil {
		t.Fatal(err)
	}
	otro := t.TempDir()
	if err := os.MkdirAll(filepath.Join(otro, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otro, "docs", "x.md"), []byte(h75Largo()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(otro)
	rec, _, err := approval.Aprobar(nil, root, "docs/x.md")
	if err != nil {
		t.Fatalf("hallazgo c75c44: el docs/x.md del root (el corto) se aprueba: %v", err)
	}
	if rec.SHA256 != h75Sha(h75Corto()) || rec.Spec != "docs/x.md" {
		t.Fatalf("hallazgo c75c44: firma el docs/x.md del root (el corto) con su ruta del root: %+v", rec)
	}
	if st, _, err := approval.Status(root, ruta); err != nil || st != approval.StatusApproved {
		t.Fatalf("hallazgo c75c44: el spec del root queda aprobado: %s %v", st, err)
	}
}
