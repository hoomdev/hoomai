#!/bin/sh
# Gate security de hoomai: semgrep con reglas fijadas por
# .hoom/semgrep/paquetes.lock mas las propias de .hoom/semgrep/reglas/.
# Exit 0 sin hallazgos, 1 con hallazgos, 2 si las reglas o el motor no son
# los que fija el lock.
set -eu
lock=.hoom/semgrep/paquetes.lock
cache=.hoom/cache/semgrep
mkdir -p "$cache"

sha() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

configs=""
while read -r nombre valor; do
	case "$nombre" in
	'' | '#'*) continue ;;
	semgrep)
		tiene=$(semgrep --version 2>/dev/null | tail -n 1)
		if [ "$tiene" != "$valor" ]; then
			echo "gate security: semgrep $tiene instalado y el lock fija $valor ($lock)" >&2
			exit 2
		fi
		;;
	p/*)
		archivo="$cache/$(echo "$nombre" | tr '/' '_')-$valor.yml"
		if [ ! -f "$archivo" ] || [ "$(sha "$archivo")" != "$valor" ]; then
			# un temporal propio por corrida: dos verify a la vez no se pisan, y
			# el nombre final solo aparece con el hash ya comprobado
			tmp=$(mktemp "$archivo.XXXXXX")
			# reintentos: el hash se comprueba igual, asi que reintentar no
			# afloja nada y una caida breve del registro no pone rojo el gate
			curl -fsSL --retry 4 --retry-delay 3 --retry-all-errors "https://semgrep.dev/c/$nombre" -o "$tmp" || {
				echo "gate security: no se pudo bajar $nombre del registro de semgrep" >&2
				rm -f "$tmp"
				exit 2
			}
			bajado=$(sha "$tmp")
			if [ "$bajado" != "$valor" ]; then
				echo "gate security: el registro cambio $nombre (sha256 $bajado; el lock fija $valor). Revisa las reglas nuevas y actualiza $lock" >&2
				rm -f "$tmp"
				exit 2
			fi
			mv "$tmp" "$archivo"
		fi
		configs="$configs --config $archivo"
		;;
	*)
		echo "gate security: linea desconocida en $lock: $nombre" >&2
		exit 2
		;;
	esac
done <"$lock"

# shellcheck disable=SC2086 # configs son rutas sin espacios armadas arriba
exec semgrep scan --error --quiet --metrics=off $configs --config .hoom/semgrep/reglas/ .
