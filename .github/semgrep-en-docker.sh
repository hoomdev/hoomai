#!/bin/sh
# semgrep desde la imagen oficial fijada por digest (SEMGREP_IMAGEN), sobre el
# directorio actual. El CI lo instala como el comando semgrep del PATH: asi
# .hoom/semgrep/gate.sh corre igual en el CI y en una maquina con semgrep
# instalado.
set -eu
: "${SEMGREP_IMAGEN:?falta SEMGREP_IMAGEN (imagen de semgrep fijada por digest)}"
exec docker run --rm \
	-v "$PWD:$PWD" \
	-w "$PWD" \
	"$SEMGREP_IMAGEN" \
	semgrep "$@"
