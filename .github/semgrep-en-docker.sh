#!/bin/sh
# semgrep desde la imagen oficial fijada por digest (SEMGREP_IMAGEN), sobre el
# directorio actual. El CI lo instala como el comando semgrep del PATH: asi
# .hoom/semgrep/gate.sh corre igual en el CI y en una maquina con semgrep
# instalado.
set -eu
: "${SEMGREP_IMAGEN:?falta SEMGREP_IMAGEN (imagen de semgrep fijada por digest)}"
# El checkout entra solo de lectura y el contenedor no tiene red: semgrep
# solo lee el arbol y las reglas ya bajadas y comprobadas por gate.sh, y un
# defecto del scanner no puede tocar lo que despues se compila ni sacar nada
# afuera (hallazgo fb0761). Sin red tampoco busca versiones nuevas.
exec docker run --rm \
	--network=none \
	-e SEMGREP_ENABLE_VERSION_CHECK=0 \
	-v "$PWD:$PWD:ro" \
	-w "$PWD" \
	"$SEMGREP_IMAGEN" \
	semgrep "$@"
