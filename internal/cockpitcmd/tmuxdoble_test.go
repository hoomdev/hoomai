// Soporte compartido de los tests de Abrir sesion cuando el cierre falla
// (CA-347, CA-348), escrito desde .hoom/specs/acciones-desde-la-tarjeta.md y
// el contrato de los hallazgos 9c1816, 57bd29 y 03481c (decision de Henry).
// Aca no hay tests: lo usan hallazgos_db8541_f7efdc_test.go (hf7Tmux y el test
// de f7efdc) y hallazgos_b59c30_e0001e_test.go (hb5Tmux y el test de la
// matriz). Antes tmuxSinSesion vivia en hallazgos_b59c30_e0001e_test.go y lo
// usaba el doble de hallazgos_db8541_f7efdc_test.go (hallazgo 344b2b, de
// legibilidad): lo compartido vive aca.
//
//   - Las respuestas de tmux de verdad que dan los dobles: tmuxSinSesion (la
//     sesion no esta) y tmuxClienteRoto (falla el propio cliente de tmux).
//     Las dos salen con codigo 1: por eso el codigo de salida no prueba nada
//     (9c1816).
//   - ciNoSabeSiSigueAbierta: que dice el error cuando Open creo la sesion,
//     algo fallo despues y el cierre `tmux kill-session -t =<sesion>` TAMBIEN
//     fallo. Un solo mensaje para todos los casos, este la sesion viva,
//     muerta o sin saberse.
//   - ciDiceQueLaCerro: la guarda de C1, con kill-session sano.
package cockpitcmd

import (
	"os/exec"
	"regexp"
	"strings"
)

// tmuxSinSesion es el error con el que tmux de verdad responde has-session o
// kill-session de una sesion que no existe: sale con codigo 1 y dice "can't
// find session: <sesion>" por stderr. Es el *exec.ExitError (ExitCode() == 1,
// con Stderr) de un proceso de verdad, no un error de Go armado a mano
// (b59c30, e0001e). Lo usan hb5Tmux y hf7Tmux.
func tmuxSinSesion(sesion string) error {
	_, err := exec.Command("/bin/sh", "-c", `printf "can't find session: %s\n" "$1" >&2; exit 1`, "sh", sesion).Output()
	return err
}

// tmuxClienteRotoMsg es lo que dice por stderr el cliente de tmux cuando se
// rompe su conexion con el servidor.
const tmuxClienteRotoMsg = "lost server"

// tmuxClienteRoto es el error con el que sale el propio cliente de tmux
// cuando falla (perdio el servidor): tambien codigo 1, como el de una sesion
// que no esta, aunque la sesion siga viva. Es el caso del hallazgo 9c1816: un
// *exec.ExitError con ExitCode() == 1 y la sesion VIVA. Un proceso de verdad,
// no un error de Go armado a mano.
func tmuxClienteRoto() error {
	_, err := exec.Command("/bin/sh", "-c", `printf '%s\n' "$1" >&2; exit 1`, "sh", tmuxClienteRotoMsg).Output()
	return err
}

// ciSinTildes pasa msg a minusculas sin tildes, para buscar frases.
func ciSinTildes(msg string) string {
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u").Replace(strings.ToLower(msg))
}

// ciIncierto son las maneras de decir que no se pudo saber si la sesion sigue
// abierta (sobre el mensaje sin tildes, en minusculas).
var ciIncierto = []string{
	"no se pudo saber", "no pude saber", "no se puede saber", "no se sabe", "no se si",
	"no se pudo confirmar", "no pude confirmar", "no se puede confirmar",
	"no se pudo comprobar", "no pude comprobar", "no se pudo verificar", "no pude verificar",
	"puede que siga", "quiza siga", "quizas siga", "tal vez siga", "podria seguir",
}

// ciIncierta dice si bajo (sin tildes, en minusculas) dice que no se pudo
// saber si la sesion sigue abierta.
func ciIncierta(bajo string) bool {
	for _, frase := range ciIncierto {
		if strings.Contains(bajo, frase) {
			return true
		}
	}
	return false
}

// Lo que puede ir justo antes de una frase para que no sea una afirmacion: un
// condicional ("si ...", ciPrefijoSi) o, para lo que Open hizo, tambien una
// negacion ("no la cerre", ciPrefijoSiONo). Entre medio, a lo sumo "la sesion
// <nombre> " y un adverbio; o el segundo miembro de una disyuncion en la
// misma frase ("si sigue abierta o ya no existe").
var (
	ciPrefijoSi    = regexp.MustCompile(`(^|[^a-z])si ((la|esa) sesion )?(\S+ )?((todavia|aun|ya) )?$|(^|[^a-z])si [^.;:]* o $`)
	ciPrefijoSiONo = regexp.MustCompile(`(^|[^a-z])(si|no) ((la|esa) sesion )?(\S+ )?((todavia|aun|ya) )?$|(^|[^a-z])si [^.;:]* o $`)
)

// ciAfirma dice si bajo afirma frase como un hecho: alguna aparicion de frase
// que no sigue a uno de los prefijos de permitido.
func ciAfirma(bajo, frase string, permitido *regexp.Regexp) bool {
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

// ciHechos son las frases con las que el error afirmaria que la sesion ya no
// existe o que quedo abierta: ninguna vale como hecho (solo como condicional,
// "si sigue abierta").
var ciHechos = []string{"ya no existe", "no existe mas", "quedo abierta", "sigue abierta", "sigue viva", "quedo viva"}

// ciCerro son las frases con las que el error dice que Open cerro la sesion.
var ciCerro = []string{"la cerre", "cerre la sesion", "se cerro la sesion", "la sesion se cerro"}

// ciNoSabeSiSigueAbierta exige el contrato nuevo (hallazgos 9c1816, 57bd29 y
// 03481c, decision de Henry) sobre msg, el error de Open cuando creo la sesion
// sesion, algo fallo despues y el cierre `tmux kill-session -t =<sesion>`
// TAMBIEN fallo: tmux sale con codigo 1 tanto si la sesion no esta como si
// falla su propio cliente, asi que nada de lo que responda prueba si la sesion
// sigue abierta. Este la sesion viva, muerta o sin saberse, el error:
//
//   - conserva original, el error del paso que fallo;
//   - no dice que la cerro (kill-session fallo);
//   - NO afirma como hecho que la sesion ya no existe ni que quedo abierta;
//   - dice que no se pudo saber si sigue abierta;
//   - muestra el comando de cierre `tmux kill-session -t '=<sesion>'`, con el
//     destino entre comillas (que corra tal cual en sh y en zsh lo exige
//     aparte cierreEjecutable).
//
// Devuelve lo que esta mal ([] si nada).
func ciNoSabeSiSigueAbierta(msg, original, sesion string) []string {
	bajo := ciSinTildes(msg)
	var mal []string
	if !strings.Contains(msg, original) {
		mal = append(mal, "conserva el error del paso que fallo ("+original+")")
	}
	for _, frase := range ciCerro {
		if ciAfirma(bajo, frase, ciPrefijoSiONo) {
			mal = append(mal, "no dice que la cerro (contiene "+frase+"): kill-session fallo")
		}
	}
	for _, frase := range ciHechos {
		if ciAfirma(bajo, frase, ciPrefijoSi) {
			mal = append(mal, "no afirma como hecho que la sesion "+frase+": el codigo de salida de tmux no prueba nada (9c1816)")
		}
	}
	if !ciIncierta(bajo) {
		mal = append(mal, "dice que no se pudo saber si la sesion sigue abierta")
	}
	if aviso := "tmux kill-session -t '=" + sesion + "'"; !strings.Contains(msg, aviso) {
		mal = append(mal, "muestra el comando de cierre con el destino entre comillas: "+aviso)
	}
	return mal
}

// ciDiceQueLaCerro dice si msg dice que Open cerro la sesion (sin negarlo):
// el contrato de C1 cuando kill-session sale bien, para que el proximo intento
// la arme entera.
func ciDiceQueLaCerro(msg string) bool {
	bajo := ciSinTildes(msg)
	for _, frase := range ciCerro {
		if ciAfirma(bajo, frase, ciPrefijoSiONo) {
			return true
		}
	}
	return false
}
