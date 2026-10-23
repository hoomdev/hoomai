#!/bin/sh
# Gate security de hoomai: semgrep con las reglas que fija
# .hoom/semgrep/paquetes.lock (repos de reglas en un commit fijo) mas las
# propias de .hoom/semgrep/reglas/.
# Exit 0 sin hallazgos, 1 con hallazgos, 2 si las reglas o el motor no son
# los que fija el lock.
set -eu
lock=.hoom/semgrep/paquetes.lock
cache=.hoom/cache/semgrep

falla() {
	echo "gate security: $*" >&2
	exit 2
}
mal() {
	falla "linea invalida en $lock: $*"
}

# git sin la configuracion del usuario ni la del sistema (un insteadOf que
# reescribe https a ssh no decide de donde se baja un repo publico) y con
# rutas literales: una ruta del lock nunca es un comodin ni una magia de git
gitp() {
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_LITERAL_PATHSPECS=1 git "$@"
}

# 1. el lock entero se valida antes de bajar nada
version=""
repos=" "
while read -r a b c d e f; do
	case "$a" in
	'' | '#'*) ;;
	semgrep)
		[ -n "$b" ] && [ -z "$c" ] || mal "$a $b $c"
		version=$b
		;;
	repo)
		# repo <nombre> <carpeta> <url> <commit>
		[ -n "$e" ] && [ -z "$f" ] || mal "repo $b $c $d $e $f"
		case "$b" in '' | *[!A-Za-z0-9_-]*) mal "repo $b: nombre" ;; esac
		case "$c" in .) ;; '' | *[!A-Za-z0-9_-]*) mal "repo $b: carpeta $c" ;; esac
		case "$d" in https://*) ;; *) mal "repo $b: url $d" ;; esac
		case "$e" in *[!0-9a-f]*) mal "repo $b: commit $e" ;; esac
		[ "${#e}" -eq 40 ] || mal "repo $b: commit $e"
		case "$repos" in *" $b "*) mal "repo $b repetido" ;; esac
		repos="$repos$b "
		;;
	*)
		# <nombre> <ruta>: el repo va declarado antes que sus archivos
		case "$repos" in *" $a "*) ;; *) mal "$a $b (repo no declarado)" ;; esac
		[ -n "$b" ] && [ -z "$c" ] || mal "$a $b $c"
		case "$b" in /* | *..* | *[!A-Za-z0-9_./-]*) mal "$a $b" ;; esac
		case "$b" in *.yaml | *.yml) ;; *) mal "$a $b (no es .yaml ni .yml)" ;; esac
		;;
	esac
done <"$lock"
[ -n "$version" ] || falla "$lock no fija la version de semgrep"
[ "$repos" != " " ] || falla "$lock no declara ningun repo de reglas"
if [ -n "$(grep -vE '^(#|$)' "$lock" | sort | uniq -d)" ]; then
	falla "$lock tiene lineas repetidas"
fi

tiene=$(semgrep --version 2>/dev/null | tail -n 1)
[ "$tiene" = "$version" ] || falla "semgrep $tiene instalado y el lock fija $version ($lock)"

# 2. una carpeta propia por corrida, dentro del proyecto: el semgrep del CI
# corre en docker y solo ve el directorio actual, y dos verify a la vez no se
# pisan. Las de corridas que murieron sin limpiar se borran al dia.
mkdir -p "$cache" || falla "no se pudo crear $cache"
find "$cache" -maxdepth 1 -name 'corrida.*' -mmin +1440 -exec rm -rf {} + 2>/dev/null || true
corrida=$(mktemp -d "$cache/corrida.XXXXXX") || falla "no se pudo crear la carpeta de la corrida"
trap 'rm -rf "$corrida"' EXIT
trap 'exit 2' HUP INT TERM

# 3. cada repo en su commit: el clon vive en el cache y solo se baja lo que
# falta. Cada archivo queda en <carpeta>/<ruta sin extension>/<archivo>, para
# que semgrep nombre la regla igual que el registro y los nosemgrep valgan.
set -f # las rutas se separan por espacios, nunca se expanden
while read -r a b c d e f; do
	[ "$a" = repo ] || continue
	nombre=$b carpeta=$c url=$d commit=$e
	clon="$cache/$nombre.git"
	if ! gitp --git-dir="$clon" cat-file -e "$commit^{commit}" 2>/dev/null; then
		gitp init -q --bare "$clon" || falla "no se pudo crear $clon"
		intentos=0
		# reintentos: el commit se comprueba igual, asi que reintentar no
		# afloja nada y una caida breve de la red no pone rojo el gate
		until gitp --git-dir="$clon" fetch -q --depth 1 "$url" "$commit"; do
			intentos=$((intentos + 1))
			[ "$intentos" -lt 5 ] || falla "no se pudo bajar el commit $commit de $url"
			sleep 3
		done
		gitp --git-dir="$clon" cat-file -e "$commit^{commit}" 2>/dev/null ||
			falla "$url no entrego el commit $commit"
	fi
	rutas=$(awk -v n="$nombre" '$1 == n { print $2 }' "$lock")
	# sin rutas, git archive entregaria el repo entero
	[ -n "$rutas" ] || falla "el repo $nombre no tiene archivos en $lock"
	plano="$corrida/plano/$nombre"
	mkdir -p "$plano"
	# shellcheck disable=SC2086 # rutas validadas en el paso 1
	gitp --git-dir="$clon" archive -o "$corrida/$nombre.tar" "$commit" -- $rutas ||
		falla "el commit $commit de $nombre no tiene todas las rutas de $lock"
	tar -xf "$corrida/$nombre.tar" -C "$plano" || falla "no se pudo extraer $nombre"
	for ruta in $rutas; do
		destino="$corrida/reglas/$carpeta/${ruta%.*}"
		mkdir -p "$destino" && mv "$plano/$ruta" "$destino/" || falla "no se pudo ubicar $nombre $ruta"
	done
done <"$lock"
set +f

# 4. semgrep sin exec: la carpeta de la corrida se borra al salir
st=0
semgrep scan --error --quiet --metrics=off --config "$corrida/reglas" --config .hoom/semgrep/reglas/ . || st=$?
exit "$st"
