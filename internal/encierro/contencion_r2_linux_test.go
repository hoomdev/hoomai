//go:build linux

// Ronda 2 del test-writer de contencion-del-writer: regresiones de la review
// delta de codex sobre el encierro de Linux, directo con Envolver, Arrancar y
// Comprobar: con Windows simulado y el job creado, Comprobar nunca sube el
// nivel de un plan que ya era ninguno por otra razon (59fa6c; no necesita
// bwrap: un plan ninguno o de job no se envuelve), y la gracia cuenta desde
// el SIGTERM del comando, no desde la cancelacion (f1f7f5; con bwrap, que
// falla sin el: aclaracion 1).
package encierro_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// r2MismoNivel compara nivel, mecanismo, falta (en orden) y motivo.
func r2MismoNivel(a, b encierro.Info) bool {
	return a.Nivel == b.Nivel && a.Mecanismo == b.Mecanismo && a.Motivo == b.Motivo &&
		strings.Join(a.Falta, ",") == strings.Join(b.Falta, ",")
}

// ---------------------------------------------------------------- 59fa6c

// CA-674 y CA-677 (id 59fa6c): con ReemplazarAdopcion simulando Windows con
// el job creado, Comprobar de un plan que ya era ninguno por otra razon (sin
// tarea, o un provider que no declara su estado) devuelve ese mismo nivel con
// su motivo, sin error, y el comando sigue corriendo: nunca sube el nivel.
// Control: un plan parcial por el job, con el job creado, queda parcial.
func TestR2_59fa6c_CA674_CA677_ComprobarConElJobNuncaSubeElNivel(t *testing.T) {
	const ca = "CA-677"
	restaurar := encierro.ReemplazarAdopcion("windows", func(int) bool { return true })
	defer restaurar()
	const prov = "r2-sin-estado"
	sinTarea := encierro.NivelDe(encierro.Caso{Sistema: "windows", Tarea: false, Job: true})
	sinEstado := encierro.NivelDe(encierro.Caso{Sistema: "windows", Tarea: true, Provider: prov, Job: true})
	parcial := encierro.NivelDe(encierro.Caso{Sistema: "windows", Tarea: true, Job: true})
	if sinTarea.Nivel != encierro.NivelNinguno || sinTarea.Motivo != encierro.MotivoSinTarea ||
		sinEstado.Nivel != encierro.NivelNinguno || sinEstado.Motivo != encierro.MotivoSinEstado(prov) ||
		parcial.Nivel != encierro.NivelParcial || parcial.Mecanismo != encierro.MecanismoJob {
		t.Fatalf("%s: fixture: la tabla de NivelDe: %+v %+v %+v", ca, sinTarea, sinEstado, parcial)
	}
	for _, c := range []struct {
		nombre string
		info   encierro.Info
		exigir string
	}{
		{"sin tarea", sinTarea, manifest.ExigirNo},
		{"provider sin estado", sinEstado, manifest.ExigirNo},
		{"control: parcial por el job", parcial, manifest.ExigirParcial},
	} {
		tok := cdwToken()
		t.Cleanup(func() { cdwMatar(tok) })
		v := cdwVistaSimple(t)
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "exec sh -c 'while :; do sleep 1; done' "+tok)
		cmd.Dir = v.Dir
		cmd.WaitDelay = 20 * time.Second
		p := encierro.Plan{Info: c.info, Vista: v, Exigir: c.exigir}
		if err := encierro.Envolver(cmd, p); err != nil {
			cancel()
			t.Fatalf("%s: %s: Envolver: %v", ca, c.nombre, err)
		}
		if err := encierro.Arrancar(cmd, 2*time.Second); err != nil {
			cancel()
			t.Fatalf("%s: %s: Arrancar: %v", ca, c.nombre, err)
		}
		if !cdwAparece(tok, 1, 10*time.Second) {
			cancel()
			t.Fatalf("%s: %s: fixture: el comando arranco", ca, c.nombre)
		}
		got, err := encierro.Comprobar(cmd, p)
		if err != nil || !r2MismoNivel(got, c.info) {
			t.Errorf("%s: %s: con el job creado, Comprobar conserva el nivel y el motivo del plan (%+v), sin error: %+v %v", ca, c.nombre, c.info, got, err)
		}
		time.Sleep(200 * time.Millisecond)
		if len(cdwConToken(tok)) == 0 {
			t.Errorf("%s: %s: con el job creado el comando sigue corriendo", ca, c.nombre)
		}
		cancel()
		_ = cmd.Wait()
		encierro.Liberar(cmd)
		cdwNadieVivo(t, ca, c.nombre+", al cancelar", tok, 5*time.Second)
	}
}

// ---------------------------------------------------------------- f1f7f5

// CA-679 (id f1f7f5, medium; de mejor esfuerzo): la gracia cuenta desde que
// el comando recibe su SIGTERM, no desde la cancelacion. Un comando
// encerrado con trap de TERM que duerme 0.6 s y deja su marca, con gracia de
// 1 s y cancelado en seguida despues de Arrancar, deja su marca siempre. Diez
// veces en el mismo test.
func TestR2_f1f7f5_CA679_LaGraciaCuentaDesdeElSIGTERM(t *testing.T) {
	const ca = "CA-679"
	info := cdwInfo(t, ca)
	script := "trap 'sleep 0.6; echo term > \"$PWD/term\"; exit 0' TERM\n" +
		"while :; do sleep 0.05; done\n"
	for vuelta := 0; vuelta < 10; vuelta++ {
		v := cdwVistaSimple(t)
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
		cmd.Dir = v.Dir
		cmd.WaitDelay = 20 * time.Second
		if err := encierro.Envolver(cmd, encierro.Plan{Info: info, Vista: v}); err != nil {
			cancel()
			t.Fatalf("%s: Envolver: %v", ca, err)
		}
		desde := time.Now()
		if err := encierro.Arrancar(cmd, time.Second); err != nil {
			cancel()
			t.Fatalf("%s: Arrancar: %v", ca, err)
		}
		cancel()
		_ = cmd.Wait()
		encierro.Liberar(cmd)
		dur := time.Since(desde)
		if _, ok := cdwLeer(filepath.Join(v.Dir, "term")); !ok {
			t.Fatalf("%s: vuelta %d: con gracia de 1 s contada desde su SIGTERM, el trap de 0.6 s deja su marca (termino a los %s de arrancar)", ca, vuelta, dur)
		}
		if dur > 10*time.Second {
			t.Fatalf("%s: vuelta %d: termina en un tiempo acotado: %s", ca, vuelta, dur)
		}
	}
}
