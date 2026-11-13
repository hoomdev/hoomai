// Ronda 1 del test-writer de contencion-del-writer: regresiones de la review
// de codex sobre lo puro del paquete encierro, en cualquier sistema:
// TrasAdoptar y el texto de SinJob (aclaracion 22), ValidarEscribe
// (aclaracion 26) y la lista AdministracionPrincipal (fab30f). Escrito desde
// REGRESIONES.md y el go doc, sin mirar la implementacion.
package encierro_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
)

// r1NivelIgual compara dos Info por lo que el contrato fija (el mecanismo
// con ninguno no se fija, aclaracion 7).
func r1NivelIgual(a, b encierro.Info) bool {
	if a.Nivel != b.Nivel || a.Motivo != b.Motivo || !cdwMismas(a.Falta, b.Falta) {
		return false
	}
	return a.Nivel == encierro.NivelNinguno || a.Mecanismo == b.Mecanismo
}

var r1SinJob = encierro.Info{Nivel: encierro.NivelNinguno, Falta: cdwTodasLasFaltas, Motivo: encierro.MotivoSinJob}

// ---------------------------------------------------------------- punto 2

// CA-676 y CA-677 (ids c5790e, 5380e1, 15f87d, c1983c, 185514; aclaracion
// 22): la tabla de TrasAdoptar. En Windows el nivel es el de NivelDe con Job
// enJob, y se mata en el acto solo sin el job y con exigir distinto de no (y
// de ""); en cualquier otro sistema, la Info cero y false.
func TestR1_c5790e_CA676_CA677_TrasAdoptarLaTabla(t *testing.T) {
	const ca = "CA-676"
	conJob := encierro.Info{Nivel: encierro.NivelParcial, Mecanismo: encierro.MecanismoJob,
		Falta: []string{encierro.FaltaEscrituras, encierro.FaltaIPC}, Motivo: encierro.MotivoSinEscrituras}
	for _, c := range []struct {
		sistema string
		enJob   bool
		exigir  string
		info    encierro.Info
		mata    bool
	}{
		{"windows", true, manifest.ExigirNo, conJob, false},
		{"windows", true, manifest.ExigirParcial, conJob, false},
		{"windows", true, manifest.ExigirCompleto, conJob, false},
		{"windows", true, "", conJob, false},
		{"windows", false, manifest.ExigirNo, r1SinJob, false},
		{"windows", false, "", r1SinJob, false},
		{"windows", false, manifest.ExigirParcial, r1SinJob, true},
		{"windows", false, manifest.ExigirCompleto, r1SinJob, true},
	} {
		info, mata := encierro.TrasAdoptar(c.sistema, c.enJob, c.exigir)
		if !r1NivelIgual(info, c.info) || mata != c.mata {
			t.Errorf("%s: TrasAdoptar(%q, %v, %q) = %+v, %v; quiero %+v, %v", ca, c.sistema, c.enJob, c.exigir, info, mata, c.info, c.mata)
		}
		tabla := encierro.NivelDe(encierro.Caso{Sistema: "windows", Tarea: true, Job: c.enJob})
		if !r1NivelIgual(info, tabla) {
			t.Errorf("CA-677: en Windows TrasAdoptar da el nivel de NivelDe con Job %v: %+v vs %+v", c.enJob, info, tabla)
		}
	}
	// CA-677 (propiedad sobre todo el dominio chico): fuera de Windows no hay
	// nada que corregir; en Windows mata exactamente sin job y con exigir.
	for _, sistema := range []string{"windows", "linux", "darwin", "freebsd", ""} {
		for _, enJob := range []bool{true, false} {
			for _, exigir := range []string{"", manifest.ExigirNo, manifest.ExigirParcial, manifest.ExigirCompleto} {
				info, mata := encierro.TrasAdoptar(sistema, enJob, exigir)
				quiero := sistema == "windows" && !enJob && exigir != "" && exigir != manifest.ExigirNo
				if mata != quiero {
					t.Errorf("%s: TrasAdoptar(%q, %v, %q) mata = %v; quiero %v", ca, sistema, enJob, exigir, mata, quiero)
				}
				if sistema != "windows" && (info.Nivel != "" || info.Mecanismo != "" || len(info.Falta) != 0 || info.Motivo != "") {
					t.Errorf("CA-677: fuera de Windows TrasAdoptar da la Info cero: %q %v %q -> %+v", sistema, enJob, exigir, info)
				}
			}
		}
	}
}

// CA-676 (aclaracion 22): el texto de SinJob es el de una negativa de exigir
// con el motivo de Windows sin job, sobre el nivel real.
func TestR1_c5790e_CA676_ElTextoDeSinJob(t *testing.T) {
	const ca = "CA-676"
	for _, exigir := range []string{manifest.ExigirParcial, manifest.ExigirCompleto} {
		e := &encierro.SinJob{Info: r1SinJob, Exigir: exigir}
		want := "el encierro de este sistema es ninguno (falta: procesos, escrituras, ipc) y hoom.yaml exige " + exigir + ": windows no dejo crear el job"
		if e.Error() != want {
			t.Errorf("%s: SinJob.Error() = %q; quiero %q", ca, e.Error(), want)
		}
	}
	if encierro.MotivoSinJob != "windows no dejo crear el job" {
		t.Errorf("%s: MotivoSinJob: %q", ca, encierro.MotivoSinJob)
	}
	var _ error = &encierro.SinJob{}
}

// ---------------------------------------------------------------- punto 3

// r1Escribe arma el principal (un repo con .git adentro, sin correr git: la
// funcion no corre git) y un $HOME del test.
func r1Escribe(t *testing.T) (principal, comun, home string) {
	t.Helper()
	home = cdwDirPuro(t)
	t.Setenv("HOME", home)
	principal = filepath.Join(cdwDirPuro(t), "proyecto")
	comun = filepath.Join(principal, ".git")
	if err := os.MkdirAll(filepath.Join(comun, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	return principal, comun, home
}

func cdwDirPuro(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func r1TodoElDisco(prov, ruta string) string {
	return "encierro: " + prov + " escribe " + ruta + ": es todo el disco o todo el directorio personal: no puede ser escribible"
}

func r1DelPrincipal(prov, ruta string) string {
	return "encierro: " + prov + " escribe " + ruta + ": contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible"
}

// CA-690 y CA-678 (ids 425a75 y 03a36d; aclaracion 26): ValidarEscribe aplica
// al Escribe del provider las reglas de encierro.escribibles: `/` y el $HOME
// (tambien como "~/" y despues de filepath.Clean) son todo el disco o todo el
// directorio personal; el principal, lo que lo contiene o esta dentro, y el
// directorio comun o lo que esta dentro, no pueden ser escribibles. La ruta
// va como la declara el provider; gana la primera que rompe, en el orden del
// Escribe. Un Escribe valido (o vacio) da nil.
func TestR1_425a75_CA690_CA678_ValidarEscribe(t *testing.T) {
	const ca = "CA-690"
	principal, comun, home := r1Escribe(t)
	afuera := cdwDirPuro(t)
	const prov = "falso-r1"
	for _, c := range []struct {
		nombre  string
		escribe []string
		want    string
	}{
		{"todo el disco", []string{"/"}, r1TodoElDisco(prov, "/")},
		{"todo el disco sin limpiar", []string{"/./"}, r1TodoElDisco(prov, "/./")},
		{"el $HOME como ~/", []string{"~/"}, r1TodoElDisco(prov, "~/")},
		{"el $HOME absoluto", []string{home}, r1TodoElDisco(prov, home)},
		{"el $HOME con barra final", []string{home + "/"}, r1TodoElDisco(prov, home+"/")},
		{"el principal", []string{principal}, r1DelPrincipal(prov, principal)},
		{"dentro del principal", []string{filepath.Join(principal, ".hoom", "items")}, r1DelPrincipal(prov, filepath.Join(principal, ".hoom", "items"))},
		{"lo que contiene al principal", []string{filepath.Dir(principal)}, r1DelPrincipal(prov, filepath.Dir(principal))},
		{"el directorio comun", []string{comun}, r1DelPrincipal(prov, comun)},
		{"dentro del directorio comun", []string{filepath.Join(comun, "objects")}, r1DelPrincipal(prov, filepath.Join(comun, "objects"))},
		{"gana la primera que rompe", []string{afuera, filepath.Join(comun, "objects"), "/"}, r1DelPrincipal(prov, filepath.Join(comun, "objects"))},
		{"gana la primera que rompe (al reves)", []string{"~/", principal}, r1TodoElDisco(prov, "~/")},
		{"uno valido", []string{afuera, "~/.falso-r1/sesiones"}, ""},
		{"ninguno", nil, ""},
	} {
		err := encierro.ValidarEscribe(prov, c.escribe, principal, comun)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: %s: un Escribe valido da nil: %v", ca, c.nombre, err)
			}
			continue
		}
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: %s: ValidarEscribe(%q) = %v; quiero %q", ca, c.nombre, c.escribe, err, c.want)
		}
	}
}

// CA-690 (id 03a36d; aclaracion 26): el estado de codex sigue a CODEX_HOME
// (providers.CodexHome). Con la funcion, nunca con la CLI real: un
// CODEX_HOME que es el $HOME, o que esta dentro del principal, no puede ser
// escribible; uno aparte si.
func TestR1_03a36d_CA690_ValidarEscribeConCodexHome(t *testing.T) {
	const ca = "CA-690"
	principal, comun, home := r1Escribe(t)
	aparte := filepath.Join(cdwDirPuro(t), "codex-home")
	for _, c := range []struct {
		nombre, codexHome string
		want              func(ruta string) string
	}{
		{"CODEX_HOME es el $HOME", home, func(r string) string { return r1TodoElDisco("codex", r) }},
		{"CODEX_HOME dentro del principal", filepath.Join(principal, ".codex"), func(r string) string { return r1DelPrincipal("codex", r) }},
		{"CODEX_HOME dentro del directorio comun", filepath.Join(comun, "codex"), func(r string) string { return r1DelPrincipal("codex", r) }},
		{"CODEX_HOME aparte", aparte, nil},
	} {
		t.Setenv("CODEX_HOME", c.codexHome)
		ruta := providers.CodexHome()
		if filepath.Clean(ruta) != filepath.Clean(c.codexHome) {
			t.Fatalf("%s: %s: fixture: CodexHome sigue a CODEX_HOME: %q", ca, c.nombre, ruta)
		}
		err := encierro.ValidarEscribe("codex", []string{ruta}, principal, comun)
		if c.want == nil {
			if err != nil {
				t.Errorf("%s: %s: un CODEX_HOME aparte es escribible: %v", ca, c.nombre, err)
			}
			continue
		}
		if err == nil || err.Error() != c.want(ruta) {
			t.Errorf("%s: %s: ValidarEscribe = %v; quiero %q", ca, c.nombre, err, c.want(ruta))
		}
	}
}

// ---------------------------------------------------------------- punto 1

// CA-678 (id fab30f): la lista de la administracion del principal trae, por
// lo menos, el index y el HEAD que el punto nombra, relativos al directorio
// comun, sin repetidos.
func TestR1_fab30f_CA678_LaAdministracionDelPrincipal(t *testing.T) {
	const ca = "CA-678"
	vistos := map[string]bool{}
	for _, e := range encierro.AdministracionPrincipal {
		if e == "" || filepath.IsAbs(e) || vistos[e] {
			t.Errorf("%s: cada entrada es una ruta relativa al directorio comun, una vez: %q", ca, e)
		}
		vistos[e] = true
	}
	for _, e := range []string{"HEAD", "index"} {
		if !vistos[e] {
			t.Errorf("%s: AdministracionPrincipal trae %s: %q", ca, e, encierro.AdministracionPrincipal)
		}
	}
	if vistos["worktrees"] || vistos["refs"] || vistos["objects"] || vistos["packed-refs"] {
		t.Errorf("%s: refs/, objects/, packed-refs y worktrees/ no son la administracion del principal: %q", ca, encierro.AdministracionPrincipal)
	}
}
