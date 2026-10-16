// Package cierretmuxtest verifica, SOLO para tests, que dice el error de Abrir
// sesion (.hoom/specs/acciones-desde-la-tarjeta.md, CA-347 y CA-348) cuando
// la apertura creo la sesion de tmux, algo fallo despues y el cierre `tmux
// kill-session -t =<sesion>` TAMBIEN fallo: el contrato de los hallazgos
// 9c1816, 57bd29 y 03481c (decision de Henry). tmux sale con codigo 1 tanto
// si la sesion no esta como si falla su propio cliente, asi que nada de lo
// que responda prueba si la sesion sigue abierta: el error no dice que la
// cerro, no afirma como hecho que la sesion ya no existe ni que quedo
// abierta, dice que no se pudo saber si sigue abierta y muestra el comando de
// cierre `tmux kill-session -t '=<sesion>'`, con el destino entre comillas.
// Y, con kill-session sano, el contrato de C1: el error dice que la cerro.
//
// Existe solo para tests, como net/http/httptest: ningun codigo de
// produccion lo importa. Es un paquete y no un _test.go porque el mismo
// verificador lo usan internal/cockpitcmd (Open) e internal/servecmd (el
// Studio y la CLI de punta a punta): antes cada uno tenia el suyo y eran dos
// analizadores de lenguaje natural que mover juntos (hallazgo 9accd8). La
// matriz de frases que acepta y rechaza vive en mensaje_test.go, aca mismo
// (hallazgo 1c0773). Cada paquete conserva lo propio en su envoltorio
// incumplimientosDelCierreFallido, y lo prueba en su test del verificador:
// cockpitcmd exige ademas que el error conserve el del paso que fallo y busca
// el comando de cierre tal cual (strings.Contains), y servecmd lo busca con
// el nombre exacto de la sesion (contieneElAviso).
package cierretmuxtest

import (
	"regexp"
	"strings"
)

// sinTildes pasa msg a minusculas sin tildes, para buscar frases.
func sinTildes(msg string) string {
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u").Replace(strings.ToLower(msg))
}

// incierto son las maneras de decir que no se pudo saber si la sesion sigue
// abierta (sobre el mensaje sin tildes, en minusculas).
var incierto = []string{
	"no se pudo saber", "no pude saber", "no se puede saber", "no se sabe", "no se si",
	"no se pudo confirmar", "no pude confirmar", "no se puede confirmar",
	"no se pudo comprobar", "no pude comprobar", "no se pudo verificar", "no pude verificar",
	"puede que siga", "quiza siga", "quizas siga", "tal vez siga", "podria seguir",
}

// incierta dice si bajo (sin tildes, en minusculas) dice que no se pudo saber
// si la sesion sigue abierta.
func incierta(bajo string) bool {
	for _, frase := range incierto {
		if strings.Contains(bajo, frase) {
			return true
		}
	}
	return false
}

// Lo que puede ir justo antes de una frase para que no sea una afirmacion: un
// condicional ("si ...", prefijoSi) o, para lo que la apertura hizo, tambien
// una negacion ("no la cerre", prefijoSiONo). Entre medio, a lo sumo "la
// sesion <nombre> " y un adverbio; o el segundo miembro de una disyuncion en
// la misma frase ("si sigue abierta o ya no existe").
var (
	prefijoSi    = regexp.MustCompile(`(^|[^a-z])si ((la|esa) sesion )?(\S+ )?((todavia|aun|ya) )?$|(^|[^a-z])si [^.;:]* o $`)
	prefijoSiONo = regexp.MustCompile(`(^|[^a-z])(si|no) ((la|esa) sesion )?(\S+ )?((todavia|aun|ya) )?$|(^|[^a-z])si [^.;:]* o $`)
)

// afirma dice si bajo afirma frase como un hecho: alguna aparicion de frase
// que no sigue a uno de los prefijos de permitido.
func afirma(bajo, frase string, permitido *regexp.Regexp) bool {
	for i := 0; ; {
		j := strings.Index(bajo[i:], frase)
		if j < 0 {
			return false
		}
		ini := i + j
		if !permitido.MatchString(bajo[:ini]) {
			return true
		}
		i = ini + len(frase)
	}
}

// hechos son las frases con las que el error afirmaria que la sesion ya no
// existe o que quedo abierta: ninguna vale como hecho (solo como condicional,
// "si sigue abierta").
var hechos = []string{"ya no existe", "no existe mas", "quedo abierta", "sigue abierta", "sigue viva", "quedo viva"}

// cerro son las frases con las que el error dice que la apertura cerro la
// sesion.
var cerro = []string{"la cerre", "cerre la sesion", "se cerro la sesion", "la sesion se cerro"}

// IncumplimientosDelCierreFallido devuelve en que incumple msg el contrato de
// 9c1816, 57bd29 y 03481c: msg es el error de una apertura que creo la sesion
// sesion, fallo despues y no pudo cerrarla con `tmux kill-session -t
// =<sesion>`. No es un predicado: devuelve la lista de incumplimientos,
// vacia si msg cumple todo y, si no, con al menos uno por cada regla que
// rompe, en el orden de abajo. Este la sesion viva, muerta o sin saberse, el
// error:
//
//   - no dice que la cerro (kill-session fallo);
//   - NO afirma como hecho que la sesion ya no existe ni que quedo abierta;
//   - dice que no se pudo saber si sigue abierta;
//   - muestra el comando de cierre `tmux kill-session -t '=<sesion>'`, con el
//     destino entre comillas, segun contiene(msg, comando): cada paquete dice
//     como lo busca. Que corra tal cual en sh y en zsh lo exige aparte
//     cierreEjecutable, en cada paquete.
func IncumplimientosDelCierreFallido(msg, sesion string, contiene func(msg, aviso string) bool) []string {
	bajo := sinTildes(msg)
	var mal []string
	for _, frase := range cerro {
		if afirma(bajo, frase, prefijoSiONo) {
			mal = append(mal, "no dice que la cerro (contiene "+frase+"): kill-session fallo")
		}
	}
	for _, frase := range hechos {
		if afirma(bajo, frase, prefijoSi) {
			mal = append(mal, "no afirma como hecho que la sesion "+frase+": el codigo de salida de tmux no prueba nada (9c1816)")
		}
	}
	if !incierta(bajo) {
		mal = append(mal, "dice que no se pudo saber si la sesion sigue abierta")
	}
	if aviso := "tmux kill-session -t '=" + sesion + "'"; !contiene(msg, aviso) {
		mal = append(mal, "muestra el comando de cierre con el destino entre comillas: "+aviso)
	}
	return mal
}

// DiceQueNoSabe dice si msg dice (con o sin tildes) que no se pudo saber si
// la sesion sigue abierta.
func DiceQueNoSabe(msg string) bool {
	return incierta(sinTildes(msg))
}

// DiceQueLaCerro dice si msg dice que la apertura cerro la sesion (sin
// negarlo): el contrato de C1 cuando kill-session sale bien, para que el
// proximo intento la arme entera.
func DiceQueLaCerro(msg string) bool {
	bajo := sinTildes(msg)
	for _, frase := range cerro {
		if afirma(bajo, frase, prefijoSiONo) {
			return true
		}
	}
	return false
}
