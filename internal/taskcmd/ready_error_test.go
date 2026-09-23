// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-307): taskcmd.Ready devuelve *taskcmd.ReadyError con el Kind de la
// condicion que fallo, y su Error() es BYTE A BYTE el mensaje de antes: task
// done y el missing de C1 no cambian. El tablero lee el Kind, nunca parsea el
// mensaje.
package taskcmd

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// reKind exige que err sea un *ReadyError (tambien envuelto) con el kind y el
// mensaje exactos, y que task done sin --force devuelva el mismo texto.
func reKind(t *testing.T, root, slug, kind, msg string) {
	t.Helper()
	err := Ready(root, slug, "main")
	if err == nil {
		t.Fatalf("CA-307: Ready debe rechazar con kind %q, fue nil", kind)
	}
	var re *ReadyError
	if !errors.As(err, &re) {
		t.Fatalf("CA-307: Ready devuelve un *taskcmd.ReadyError, fue %T: %v", err, err)
	}
	if re.Kind != kind {
		t.Fatalf("CA-307: el kind debe ser %q, fue %q (%v)", kind, re.Kind, err)
	}
	if err.Error() != msg {
		t.Fatalf("CA-307: el mensaje es byte a byte el de antes:\nquiere: %q\nfue:    %q", msg, err.Error())
	}
	if re.Error() != msg {
		t.Fatalf("CA-307: ReadyError.Error() es el mensaje de antes: %q", re.Error())
	}
	// envuelto sigue siendo el mismo kind
	var re2 *ReadyError
	if !errors.As(fmt.Errorf("envuelto: %w", err), &re2) || re2.Kind != kind {
		t.Fatalf("CA-307: errors.As encuentra el ReadyError aunque venga envuelto")
	}
	derr := Done(root, slug, "main", false)
	if derr == nil || derr.Error() != msg {
		t.Fatalf("CA-307: task done rechaza con el mismo mensaje de siempre:\nquiere: %q\nfue:    %v", msg, derr)
	}
}

// CA-307: los seis kinds, cada uno con el mensaje que Ready daba antes de
// tipar el error. Con todo en orden, Ready es nil (un nil de verdad, no un
// *ReadyError nil dentro de la interfaz).
func TestCA307_ReadyErrorKindsYMensajes(t *testing.T) {
	const slug = "precios"
	root, wt := rdTarea(t, slug)

	reKind(t, root, "no-existe", ReadySinTarea,
		`la tarea "no-existe" no existe (mira 'hoom task list')`)

	rdEscribir(t, wt, "precios.go", "package app\n")
	reKind(t, root, slug, ReadySinGuardar,
		fmt.Sprintf("la tarea %q tiene cambios sin commitear (incluidos posibles veredictos).\n  Accion: commitea todo dentro de %s y repite 'hoom task done %s'", slug, wt, slug))
	rdCommit(t, wt, "codigo")

	reKind(t, root, slug, ReadySinVeredicto,
		fmt.Sprintf("la tarea %q no tiene veredictos. Accion: ejecuta 'hoom verify' dentro del worktree", slug))

	rdVeredicto(t, wt, time.Now().UTC(), verdict.StatusPass, true, "")
	rdCommit(t, wt, "parcial")
	reKind(t, root, slug, ReadySoloParciales,
		fmt.Sprintf("la tarea %q solo tiene veredictos PARCIALES (--gate), que no son referencia. Accion: ejecuta 'hoom verify' completo dentro del worktree", slug))

	rojo := rdVeredicto(t, wt, time.Now().UTC().Add(time.Second), verdict.StatusFail, false, "")
	rdCommit(t, wt, "rojo")
	reKind(t, root, slug, ReadyRojo,
		fmt.Sprintf("el ultimo veredicto de %q es ROJO (%s). Accion: corrige y re-ejecuta 'hoom verify' en el worktree", slug, rojo.ID))

	rdVeredicto(t, wt, time.Now().UTC().Add(2*time.Second), verdict.StatusPass, false, "otra-huella")
	rdCommit(t, wt, "verde de otra huella")
	actual := gitx.Snapshot(wt, "main").ChangeFingerprint
	reKind(t, root, slug, ReadyHuella,
		fmt.Sprintf("el arbol de %q cambio despues del ultimo veredicto verde (huella %s vs %s).\n  Accion: re-ejecuta 'hoom verify' dentro del worktree y commitea", slug, actual, "otra-huella"))

	rdVeredicto(t, wt, time.Now().UTC().Add(3*time.Second), verdict.StatusPass, false, "")
	rdCommit(t, wt, "verde")
	if err := Ready(root, slug, "main"); err != nil {
		t.Fatalf("CA-307: limpio, verde y con la huella, Ready es nil (sin *ReadyError nil adentro): %#v", err)
	}
}

// CA-307: los kinds son las cadenas del contrato, y el tipo es un error.
func TestCA307_ReadyErrorKindsDelContrato(t *testing.T) {
	quiere := map[string]string{
		ReadySinTarea: "sin-tarea", ReadySinGuardar: "sin-guardar", ReadySinVeredicto: "sin-veredicto",
		ReadySoloParciales: "solo-parciales", ReadyRojo: "rojo", ReadyHuella: "huella",
	}
	for got, want := range quiere {
		if got != want {
			t.Fatalf("CA-307: kind %q debe ser %q", got, want)
		}
	}
	if len(quiere) != 6 {
		t.Fatalf("CA-307: son seis kinds distintos: %v", quiere)
	}
	var err error = &ReadyError{Kind: ReadyRojo, Msg: "texto de siempre"}
	if err.Error() != "texto de siempre" {
		t.Fatalf("CA-307: Error() devuelve el mensaje tal cual: %q", err.Error())
	}
}
