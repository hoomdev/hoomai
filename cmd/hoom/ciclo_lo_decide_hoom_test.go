// Tests adversariales del spec .hoom/specs/ciclo-lo-decide-hoom.md sobre el
// binario (CA-755): `hoom pedir refutador -m` es el error de uso, exit 2, y el
// rol desconocido nombra los cinco roles de hoom pedir; ninguno corre un CLI
// de IA. Usa los fixtures de verbos_de_la_tarjeta_test.go.
package main

import (
	"strings"
	"testing"
)

const cldMsgConM = "el refutador arma su propio pedido: no lleva -m"

// CA-755: hoom pedir refutador con -m es exit 2 con el texto del contrato,
// con el slug o con la tarjeta fijada, sin correr el CLI.
func TestCA755_ElBinarioPedirRefutadorConMEsExit2(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude", "codex")
	dir, _ := vdtConTarjetaFijada(t, bin)
	for _, args := range [][]string{
		{"pedir", "refutador", "precios", "-m", "Refuta todos los abiertos"},
		{"pedir", "refutador", "-m", "Refuta todos los abiertos"},
	} {
		s := qcCorrer(t, bin, dir, nil, args...)
		if s.exit != 2 || !strings.Contains(s.stderr, cldMsgConM) {
			t.Fatalf("CA-755: hoom %s es exit 2 con %q: %d\n%s", strings.Join(args, " "), cldMsgConM, s.exit, s.todo())
		}
	}
	for _, cli := range []string{"claude", "codex"} {
		if ll := vdtLlamadas(t, argv, cli); len(ll) != 0 {
			t.Fatalf("CA-755: hoom pedir refutador -m no corre %s: %q", cli, ll)
		}
	}
}

// CA-755: el rol desconocido es exit 2 y nombra arquitecto, test-writer,
// writer, refutador, reviewer.
func TestCA755_ElBinarioRolDesconocidoNombraLosCinco(t *testing.T) {
	bin := qcHoom(t)
	argv := vdtFalso(t, "claude")
	dir, _ := vdtConTarjetaFijada(t, bin)
	for _, rol := range []string{"scout", "refutar"} {
		want := `rol desconocido "` + rol + `" (arquitecto, test-writer, writer, refutador, reviewer); para otros roles: hoom agent`
		s := qcCorrer(t, bin, dir, nil, "pedir", rol, "precios", "-m", "Haz algo")
		if s.exit != 2 || !strings.Contains(s.stderr, want) {
			t.Fatalf("CA-755: hoom pedir %s es exit 2 con %q: %d\n%s", rol, want, s.exit, s.todo())
		}
	}
	if ll := vdtLlamadas(t, argv, "claude"); len(ll) != 0 {
		t.Fatalf("CA-755: un rol desconocido no corre el CLI: %q", ll)
	}
}
