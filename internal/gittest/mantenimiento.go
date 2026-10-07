package gittest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SinMantenimiento es lo que el .git/config de un repo de prueba lleva para
// que git no lance nada por su cuenta. maintenance.auto=false apaga el
// lanzamiento (git 2.29 en adelante); gc.auto=0 es lo mismo para un git
// anterior, que lanza `git gc --auto`, y deja sin nada que hacer al gc que
// alguien lance a mano con --auto. Los worktrees comparten el .git/config de
// su repo, asi que lo heredan; un `git clone` NO: al clon hay que apagarselo
// (ApagarMantenimiento).
const SinMantenimiento = "[maintenance]\n\tauto = false\n[gc]\n\tauto = 0\n"

// ApagarMantenimiento anexa SinMantenimiento al .git/config de root. Es un
// archivo, no un proceso: no cuesta un git mas por repo (reviewcmd corre con
// -race cerca del limite de tiempo de go test). El git(t, dir, "init", ...)
// de cada paquete lo llama solo.
func ApagarMantenimiento(t testing.TB, root string) {
	t.Helper()
	cfg := filepath.Join(root, ".git", "config")
	// sin O_CREATE: si git no dejo su config, esto no es un repo
	f, err := os.OpenFile(cfg, os.O_WRONLY|os.O_APPEND, 0)
	if err == nil {
		_, err = f.WriteString(SinMantenimiento)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		t.Fatalf("fixture: apagar el mantenimiento automatico de git en %s: %v", root, err)
	}
}

// ExigirMantenimientoApagado lee cfg (el .git/config de un repo) y devuelve
// un error si no deja apagado el mantenimiento automatico de git:
// maintenance.auto en false Y gc.auto en 0, y de cada clave manda la ultima
// aparicion, como en git. Lee el archivo y nada mas: ningun proceso.
//
// Es a proposito mas estricta que git: exige que el fixture lo haya dejado
// escrito en claro en el repo. Un valor con comillas o con un comentario al
// lado, una continuacion de linea o un include no los interpreta: se niega.
// Lo que nunca hace es dar por apagado lo que git lee prendido.
func ExigirMantenimientoApagado(cfg string) error {
	const pide = "el repo tiene que traer apagado el mantenimiento automatico de git " +
		"(maintenance.auto = false y gc.auto = 0 en su .git/config): git(t, dir, \"init\", ...) lo deja asi, " +
		"y a un clon se lo apaga gittest.ApagarMantenimiento"
	raw, err := os.ReadFile(cfg)
	if err != nil {
		return fmt.Errorf("%s: %w", pide, err)
	}
	valor := map[string]string{} // seccion.clave -> el ultimo valor, tal cual
	seccion := ""
	for i, l := range strings.Split(string(raw), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == '#' || l[0] == ';' {
			continue
		}
		if strings.HasSuffix(l, `\`) {
			return fmt.Errorf("%s: %s:%d: una linea que sigue en la siguiente (\\) no se interpreta", pide, cfg, i+1)
		}
		if l[0] == '[' {
			fin := strings.IndexByte(l, ']')
			if fin < 0 {
				return fmt.Errorf("%s: %s:%d: seccion sin cerrar", pide, cfg, i+1)
			}
			seccion = strings.ToLower(l[1:fin])
			if seccion == "include" || strings.HasPrefix(seccion, "includeif") {
				return fmt.Errorf("%s: %s:%d: un include no se sigue", pide, cfg, i+1)
			}
			if l = strings.TrimSpace(l[fin+1:]); l == "" {
				continue
			}
		}
		clave, v, hayValor := strings.Cut(l, "=")
		if !hayValor {
			v = "true" // una clave sola es true para git
		}
		valor[seccion+"."+strings.ToLower(strings.TrimSpace(clave))] = strings.TrimSpace(v)
	}
	mostrar := func(k string) string {
		if v, ok := valor[k]; ok {
			return fmt.Sprintf("%s = %q", k, v)
		}
		return k + " sin definir"
	}
	m, hayM := valor["maintenance.auto"]
	g, hayG := valor["gc.auto"]
	apagado := false
	for _, no := range []string{"false", "no", "off", "0"} {
		apagado = apagado || (hayM && strings.EqualFold(m, no))
	}
	if !apagado || !hayG || g != "0" {
		return fmt.Errorf("%s: %s trae %s y %s", pide, cfg, mostrar("maintenance.auto"), mostrar("gc.auto"))
	}
	return nil
}

// ConfigDeMantenimiento es una forma de escribir (o no) el apagado en un
// .git/config: Anexo, despues de lo que deja `git init`. ApagadoParaGit es lo
// que git tiene que leer; Aceptado, lo que ExigirMantenimientoApagado tiene
// que decir.
type ConfigDeMantenimiento struct {
	Caso           string
	Anexo          string
	ApagadoParaGit bool
	Aceptado       bool
}

// ConfigsDeMantenimiento son las formas con las que se prueba a
// ExigirMantenimientoApagado, y a quien se apoye en ella para negarse (la
// copia de plantillas de agentcmd). Que git lea cada una como dice
// ApagadoParaGit lo prueba el test de este paquete.
func ConfigsDeMantenimiento() []ConfigDeMantenimiento {
	return []ConfigDeMantenimiento{
		{"lo que anexa el fixture", SinMantenimiento, true, true},
		{"anexado dos veces", SinMantenimiento + SinMantenimiento, true, true},
		{"sin espacios y en una linea con su seccion", "[maintenance] auto=false\n[gc] auto=0\n", true, true},
		{"mayusculas y no/off", "[MAINTENANCE]\n\tAUTO = No\n[Gc]\n\tAuto = 0\n", true, true},
		{"maintenance.auto en 0", "[maintenance]\n\tauto = 0\n[gc]\n\tauto = 0\n", true, true},
		{"fin de linea CRLF", "[maintenance]\r\n\tauto = false\r\n[gc]\r\n\tauto = 0\r\n", true, true},
		{"otras claves alrededor", "[maintenance]\n\tstrategy = none\n\tauto = false\n[gc]\n\tautoDetach = true\n\tauto = 0\n\tpruneExpire = now\n", true, true},
		// lo que git lee apagado y ExigirMantenimientoApagado, mas estricta, no interpreta
		{"con un comentario al lado", "[maintenance]\n\tauto = false # apagado\n[gc]\n\tauto = 0 ; apagado\n", true, false},
		{"con comillas", "[maintenance]\n\tauto = \"false\"\n[gc]\n\tauto = \"0\"\n", true, false},
		{"valor vacio (false para git)", "[maintenance]\n\tauto =\n[gc]\n\tauto = 0\n", true, false},
		// lo que git lee prendido: se niega siempre
		{"nada", "", false, false},
		{"solo maintenance.auto", "[maintenance]\n\tauto = false\n", false, false},
		{"solo gc.auto", "[gc]\n\tauto = 0\n", false, false},
		{"despues alguien lo prende", SinMantenimiento + "[maintenance]\n\tauto = true\n", false, false},
		{"despues alguien sube gc.auto", SinMantenimiento + "[gc]\n\tauto = 6700\n", false, false},
		{"prendido en mayusculas, despues", SinMantenimiento + "[Maintenance]\n\tAUTO = TRUE\n", false, false},
		{"la clave sola (true para git)", "[maintenance]\n\tauto\n[gc]\n\tauto = 0\n", false, false},
		{"comentado", "#[maintenance]\n#\tauto = false\n;[gc]\n;\tauto = 0\n", false, false},
		{"las claves comentadas", "[maintenance]\n\t# auto = false\n[gc]\n\t; auto = 0\n", false, false},
		{"en una subseccion", "[maintenance \"x\"]\n\tauto = false\n[gc \"x\"]\n\tauto = 0\n", false, false},
		{"en otra seccion", "[core]\n\tauto = false\n[gcx]\n\tauto = 0\n", false, false},
		{"otra clave que empieza igual", "[maintenance]\n\tautoDetach = false\n[gc]\n\tautoPackLimit = 0\n", false, false},
		{"la linea de arriba sigue en esta", "[maintenance]\n\tstrategy = none\\\n\tauto = false\n[gc]\n\tauto = 0\n", false, false},
		{"gc.auto en 1", "[maintenance]\n\tauto = false\n[gc]\n\tauto = 1\n", false, false},
	}
}
