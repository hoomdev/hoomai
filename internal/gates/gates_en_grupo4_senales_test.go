//go:build !windows

// Ronda 4 de gates-en-grupo, hallazgo 20261027T171326_1e64a4 (medium): ante
// una senal, el manejador de procesos esperaba primero la gracia de los
// grupos con gracia (los gates) y solo despues mataba los grupos sin gracia
// (el provider de un run). En hoom serve un provider y un verify corren a la
// vez: un Ctrl-C dejaba al provider vivo hasta 10 s, editando archivos,
// mientras un gate que ignora TERM gastaba su gracia.
//
// El hoom es este mismo binario corriendo TestAyudanteHoomConUnGate con un
// provider arrancado con procesos.Arrancar (sin gracia) y un gate (con
// gracia) que ignora TERM. Escrito desde el contrato (go doc
// ./internal/procesos: Arrancar, ArrancarConGracia, Senales) y el hallazgo,
// sin leer la implementacion.
package gates

import (
	"testing"
	"time"
)

// Hallazgo 1e64a4: hoom que recibe Ctrl-C o SIGTERM con un provider (grupo
// sin gracia) y un gate (grupo con gracia que ignora TERM) mata AL INSTANTE
// el grupo del provider: muerto en menos de 1 s, mientras el gate todavia
// esta en su gracia. Una segunda senal termina todo.
func TestHallazgo1e64a4_SenalAHoomMataAlInstanteElGrupoSinGracia(t *testing.T) {
	const etiqueta = "hallazgo 1e64a4"
	for _, caso := range gegSenalesAHoom {
		t.Run(caso.nombre, func(t *testing.T) {
			d := t.TempDir()
			provider := gegEscribir(d, "provider.pid", "$$") +
				"sleep 300 &\n" +
				gegEscribir(d, "provider_hijo.pid", "$!") +
				"wait\n"
			gate := "trap '' TERM\n" + // lo heredan sus hijos
				gegEscribir(d, "lider.pid", "$$") +
				"sleep 300 >/dev/null 2>&1 &\n" +
				gegEscribir(d, "hijo.pid", "$!") +
				"wait\n"
			h := gegArrancarHoom(t, gegConUnGate(d, gate, etiqueta, gegEnvAyudanteProvider+"="+provider))
			prov, provHijo := gegLeerPID(t, d, "provider.pid"), gegLeerPID(t, d, "provider_hijo.pid")
			lider, hijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

			enviada := time.Now()
			h.senal(t, caso.senal, caso.terminal)
			for _, pid := range []int{prov, provHijo} {
				if !gegMuere(pid, time.Until(enviada.Add(time.Second))) {
					t.Fatalf("%s: hoom que recibe %v mata al instante el grupo sin gracia (el provider): a 1 s %d sigue vivo (provider %d vivo=%v, su hijo %d vivo=%v; el gate sigue en su gracia: %v)", etiqueta, caso.senal, pid, prov, gegVivo(prov), provHijo, gegVivo(provHijo), gegVivo(lider))
				}
			}
			if !gegVivo(lider) || !gegVivo(hijo) {
				t.Fatalf("%s: el gate que ignora SIGTERM sigue en su gracia cuando el provider ya murio (sh %d vivo=%v, hijo %d vivo=%v)", etiqueta, lider, gegVivo(lider), hijo, gegVivo(hijo))
			}
			if h.termino(0) {
				t.Fatalf("%s: hoom espera la gracia del gate: termino a %v de la senal", etiqueta, time.Since(enviada))
			}

			h.senal(t, caso.senal, caso.terminal)
			if !h.termino(3 * time.Second) {
				t.Fatalf("%s: una segunda %v corta la gracia y hoom termina", etiqueta, caso.senal)
			}
			for _, pid := range []int{lider, hijo} {
				if !gegMuere(pid, 2*time.Second) {
					t.Fatalf("%s: con la segunda %v el gate muere: %d sigue vivo", etiqueta, caso.senal, pid)
				}
			}
		})
	}
}
