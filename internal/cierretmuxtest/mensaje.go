// Package cierretmuxtest verifica, SOLO para tests, si el error de Abrir
// sesion (.hoom/specs/acciones-desde-la-tarjeta.md, CA-347 y CA-348) dice que
// la apertura cerro la sesion de tmux que habia creado. Con la ENMIENDA 6
// (hallazgo bde7e4), cuando despues de crear la sesion falla el split-window
// o el registro en el item, Abrir sesion no la cierra (cerrarla por su nombre
// no prueba que sea la que creo), asi que su error no puede decir que la
// cerro: lo exige incumplimientosEnmienda6, en internal/cockpitcmd y en
// internal/servecmd, con DiceQueLaCerro.
//
// Existe solo para tests, como net/http/httptest: ningun codigo de produccion
// lo importa. Es un paquete y no un _test.go porque el mismo analizador lo
// usan internal/cockpitcmd (Open) e internal/servecmd (el Studio y la CLI de
// punta a punta): antes cada uno tenia el suyo (hallazgo 9accd8). La matriz
// de frases que reconoce y rechaza vive en mensaje_test.go, aca mismo
// (hallazgo 1c0773). El verificador del cierre fallido
// (IncumplimientosDelCierreFallido y DiceQueNoSabe, hallazgos 9c1816, 57bd29
// y 03481c) se borro: con la enmienda 6 Abrir sesion ya no corre ese cierre.
package cierretmuxtest

import (
	"regexp"
	"strings"
)

// sinTildes pasa msg a minusculas sin tildes, para buscar frases.
func sinTildes(msg string) string {
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u").Replace(strings.ToLower(msg))
}

// prefijoSiONo es lo que puede ir justo antes de "la cerre" para que no sea
// una afirmacion: un condicional ("si la cerre") o una negacion ("no la
// cerre"). Entre medio, a lo sumo "la sesion <nombre> " y un adverbio; o el
// segundo miembro de una disyuncion en la misma frase ("si sigue abierta o
// ya la cerre").
var prefijoSiONo = regexp.MustCompile(`(^|[^a-z])(si|no) ((la|esa) sesion )?(\S+ )?((todavia|aun|ya) )?$|(^|[^a-z])si [^.;:]* o $`)

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

// cerro son las frases con las que el error dice que la apertura cerro la
// sesion.
var cerro = []string{"la cerre", "cerre la sesion", "se cerro la sesion", "la sesion se cerro"}

// DiceQueLaCerro dice si msg dice que la apertura cerro la sesion (sin
// negarlo, con o sin tildes). Con la enmienda 6 el error de Abrir sesion no
// puede decirlo: la apertura no cierra la sesion que creo.
func DiceQueLaCerro(msg string) bool {
	bajo := sinTildes(msg)
	for _, frase := range cerro {
		if afirma(bajo, frase, prefijoSiONo) {
			return true
		}
	}
	return false
}
