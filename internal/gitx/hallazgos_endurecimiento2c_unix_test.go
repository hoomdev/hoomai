//go:build !windows

// Tercera ronda de endurecimiento-2 en gitx (ver
// hallazgos_endurecimiento2c_test.go), el caso que se arma con un git falso
// (un script de sh), en un proceso hijo:
//
//   - 20261026T21*_0738fe (high): TouchedErr y HuellaConTope corrian git sobre
//     dir y, solo despues del fallo, volvian a resolver dir para decidir si
//     estaba fuera de un repo. Si el enlace dir se cambiaba en el medio, git
//     habia fallado en un repo roto pero la decision miraba otro lugar,
//     fuera de todo repo, y el fallo se convertia en una lista vacia (o
//     ninguna huella) sin error. Ahora el directorio real se resuelve una
//     vez, antes de git, y git y la busqueda de .git usan ese mismo camino.
//
// Como se fuerza la carrera, de forma determinista y de afuera: un git falso
// al frente del PATH corre el git de verdad y, la primera vez que este
// falla, cambia el enlace dir para que apunte a un directorio fuera de todo
// repo, y sale con el mismo error. El cambio queda exactamente entre la
// corrida de git y lo que se decida despues. Cambiar el PATH es global al
// proceso: el caso corre en un proceso hijo (este mismo binario de test,
// re-ejecutado con -test.run y HOOM_E2C_GIT_HIJO), que deja un informe; el
// padre lo juzga.
package gitx

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	e2cEnvHijo = "HOOM_E2C_GIT_HIJO"
	e2cEnvDir  = "HOOM_E2C_GIT_DIR"
)

// e2cResultado es lo que el hijo vio en una llamada.
type e2cResultado struct {
	Err       string
	Lista     map[string]string
	Huella    string
	Disparado bool // el git falso cambio el enlace
	Apunta    string
}

// e2cInforme es lo que el hijo le deja al padre.
type e2cInforme struct {
	Touched, Huella e2cResultado
}

// e2cArmar arma, en el hijo, un repo con .git/config corrupto, un enlace
// (fuera de todo repo) a su sub/, un directorio fuera de todo repo, y un git
// falso que la primera vez que el git de verdad falla cambia el enlace para
// que apunte a ese directorio. Devuelve el enlace, la marca que dice que el
// git falso se disparo y el directorio del git falso.
func e2cArmar(t *testing.T, real string) (alias, marca, bin string) {
	t.Helper()
	repo := e2Repo(t)
	e2Roturas[1].romper(t, repo) // config corrupto
	alias = e2bAlias(t, filepath.Join(repo, "sub"), false)
	e2GitFallaEn(t, alias)
	afuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin, marca = t.TempDir(), filepath.Join(t.TempDir(), "disparado")
	s := "#!/bin/sh\n" +
		"'" + real + "' \"$@\"\n" +
		"c=$?\n" +
		"if [ $c -ne 0 ] && [ ! -e '" + marca + "' ]; then\n" +
		"  : > '" + marca + "'\n" +
		"  rm -f '" + alias + "' && ln -s '" + afuera + "' '" + alias + "'\n" +
		"fi\n" +
		"exit $c\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return alias, marca, bin
}

// e2cHijo corre en el proceso hijo: arma el caso dos veces (una para
// TouchedErr, otra para HuellaConTope), con el git falso al frente del PATH,
// y deja el informe en dir.
func e2cHijo(t *testing.T, dir string) {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("sin git en el PATH")
	}
	real, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	var inf e2cInforme
	for _, cual := range []string{"TouchedErr", "HuellaConTope"} {
		alias, marca, bin := e2cArmar(t, real)
		viejo := os.Getenv("PATH")
		os.Setenv("PATH", bin+string(os.PathListSeparator)+viejo)
		var r e2cResultado
		if cual == "TouchedErr" {
			m, err := TouchedErr(alias, "main")
			r.Lista = m
			if err != nil {
				r.Err = err.Error()
			}
		} else {
			h, err := HuellaConTope(alias, 30*time.Second)
			r.Huella = h
			if err != nil {
				r.Err = err.Error()
			}
		}
		os.Setenv("PATH", viejo)
		_, e := os.Stat(marca)
		r.Disparado = e == nil
		r.Apunta, _ = os.Readlink(alias)
		if cual == "TouchedErr" {
			inf.Touched = r
		} else {
			inf.Huella = r
		}
	}
	raw, err := json.MarshalIndent(inf, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "informe.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// CA-133, CA-442 (hallazgo 0738fe): dir es un enlace, puesto fuera de todo
// repo, a sub/ de un repo con .git/config corrupto; el git falso cambia el
// enlace para que apunte fuera de todo repo apenas el git de verdad falla
// (entre la corrida de git y la decision de "fuera de un repo"). Como git
// fallo en el repo roto, TouchedErr devuelve error, no una lista vacia; y
// HuellaConTope, error y ninguna huella. Guarda: el git falso se disparo (si
// no, el caso no se inyecto). Corre en un proceso hijo porque cambia el PATH.
func TestHallazgo_0738fe_CA133_UnEnlaceCambiadoDespuesDeGitNoConvierteElFalloEnNada(t *testing.T) {
	const ca = "CA-133, CA-442 (hallazgo 0738fe, enlace cambiado despues de git)"
	if os.Getenv(e2cEnvHijo) == "1" {
		e2cHijo(t, os.Getenv(e2cEnvDir))
		return
	}
	dir := t.TempDir()
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelar()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1", "-test.v", "-test.timeout=3m")
	cmd.Env = append(os.Environ(), e2cEnvHijo+"=1", e2cEnvDir+"="+dir)
	salida, err := cmd.CombinedOutput()
	if strings.Contains(string(salida), "--- SKIP") {
		t.Skipf("%s: el hijo salteo el caso:\n%s", ca, salida)
	}
	if err != nil {
		t.Fatalf("%s: fixture: el proceso hijo fallo (%v):\n%s", ca, err, salida)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "informe.json"))
	if err != nil {
		t.Fatalf("%s: fixture: el hijo no dejo su informe (%v):\n%s", ca, err, salida)
	}
	var inf e2cInforme
	if err := json.Unmarshal(raw, &inf); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		cual string
		res  e2cResultado
	}{{"TouchedErr", inf.Touched}, {"HuellaConTope", inf.Huella}} {
		if !r.res.Disparado {
			t.Fatalf("%s: fixture: %s no corrio un git que fallara: el git falso no cambio el enlace y el caso no se inyecto", ca, r.cual)
		}
		t.Logf("%s: %s: el git falso cambio el enlace a %s; error %q", ca, r.cual, r.res.Apunta, r.res.Err)
		if r.res.Err == "" {
			t.Errorf("%s: git fallo en el repo roto y el enlace se cambio despues para apuntar fuera de todo repo: %s devuelve error, no %v%q sin error (el gate de scope lo toma por \"nada cambio\")",
				ca, r.cual, r.res.Lista, r.res.Huella)
		}
		if r.res.Huella != "" {
			t.Errorf("%s: %s no da huella cuando git fallo: %q", ca, r.cual, r.res.Huella)
		}
	}
}
