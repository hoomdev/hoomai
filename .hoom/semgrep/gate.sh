#!/bin/sh
# Gate security de hoomai: semgrep con las reglas que fija
# .hoom/semgrep/paquetes.lock (archivos de repos de reglas en un commit fijo,
# cada uno con su sha256) mas las propias de .hoom/semgrep/reglas/.
# Exit 0 sin hallazgos y 1 con hallazgos, los de semgrep. 2: el gate no pudo
# correr exactamente las reglas del lock (lock invalido, otra version del
# motor, red, git o disco). Cualquier otro codigo es de semgrep (7: config
# invalida) o, en el CI, de docker (125).
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

sha() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$@"
	else
		shasum -a 256 "$@"
	fi
}

# git sin la configuracion del usuario ni la del sistema (un insteadOf que
# reescribe https a ssh no decide de donde se baja un repo publico), sin
# reemplazos y con rutas literales. Lo que corre no depende de git: cada
# archivo extraido se compara con su sha256 del lock (hallazgo 25bb92)
gitp() {
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_NO_REPLACE_OBJECTS=1 GIT_LITERAL_PATHSPECS=1 git "$@"
}

[ -r "$lock" ] || falla "no se puede leer $lock"
# una ultima linea sin salto la saltaria read y la leeria awk (hallazgo a32752)
[ -z "$(tail -c 1 "$lock")" ] || falla "$lock no termina en salto de linea"

# 1. el lock entero se valida antes de bajar nada
version=""
repos=" "
carpetas=" "
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
		case "$carpetas" in *" $c "*) mal "repo $b: la carpeta $c ya es de otro repo" ;; esac
		repos="$repos$b "
		carpetas="$carpetas$c "
		;;
	*)
		# <nombre> <ruta> <sha256>
		case "$repos" in *" $a "*) ;; *) mal "$a $b (repo no declarado)" ;; esac
		[ -n "$c" ] && [ -z "$d" ] || mal "$a $b $c $d"
		case "$b" in /* | *..* | *[!A-Za-z0-9_./-]*) mal "$a $b" ;; esac
		case "$b" in *.yaml | *.yml) ;; *) mal "$a $b (no es .yaml ni .yml)" ;; esac
		# semgrep no carga como reglas los tests ni los .fixed: el lock fijaria
		# una regla que nunca corre
		case "$b" in *.test.yaml | *.test.yml | *.fixed.*) mal "$a $b (semgrep no lo carga como reglas)" ;; esac
		case "$c" in *[!0-9a-f]*) mal "$a $b: sha256 $c" ;; esac
		[ "${#c}" -eq 64 ] || mal "$a $b: sha256 $c"
		;;
	esac
done <"$lock"
[ -n "$version" ] || falla "$lock no fija la version de semgrep"
[ "$repos" != " " ] || falla "$lock no declara ningun repo de reglas"
if [ -n "$(awk '!/^(#|$)/ { print $1, $2 }' "$lock" | sort | uniq -d)" ]; then
	falla "$lock tiene lineas repetidas"
fi

tiene=$(semgrep --version 2>/dev/null | tail -n 1)
[ "$tiene" = "$version" ] || falla "semgrep $tiene instalado y el lock fija $version ($lock)"

# 2. una carpeta propia por corrida, dentro del proyecto: el semgrep del CI
# corre en docker y solo ve el directorio actual, y dos verify a la vez no se
# pisan. semgrep la lee hasta el final, asi que la de una corrida que termino
# bien queda y se borra una hora despues, como lo que dejo una corrida que
# murio a medias.
mkdir -p "$cache" || falla "no se pudo crear $cache"
find "$cache" -maxdepth 1 \( -name 'corrida.*' -o -name 'clon.*' -o -name '*.viejo.*' \) -mmin +60 \
	-exec rm -rf {} + 2>/dev/null || true
corrida=$(mktemp -d "$cache/corrida.XXXXXX") || falla "no se pudo crear la carpeta de la corrida"
trap 'rm -rf "$corrida"' EXIT
trap 'exit 2' HUP INT TERM

# extraer <clon>: los archivos de $nombre en $commit, del clon a $plano, y
# solo si son exactamente los del lock: archivos regulares, ni uno de mas ni
# uno de menos, cada uno con su sha256. 1 si algo no cuadra o git no pudo.
extraer() {
	rm -rf "$plano" && mkdir -p "$plano" || return 1
	gitp --git-dir="$1" cat-file -e "$commit^{commit}" 2>/dev/null || return 1
	# shellcheck disable=SC2086 # rutas validadas en el paso 1
	gitp --git-dir="$1" archive -o "$corrida/$nombre.tar" "$commit" -- $rutas 2>/dev/null || return 1
	tar -xf "$corrida/$nombre.tar" -C "$plano" || return 1
	[ -z "$(find "$plano" ! -type f ! -type d)" ] || return 1
	[ "$(find "$plano" -type f | wc -l | tr -d ' ')" = "$cuantas" ] || return 1
	# shellcheck disable=SC2086
	(cd "$plano" && sha $rutas) | LC_ALL=C sort >"$corrida/$nombre.tiene" || return 1
	cmp -s "$corrida/$nombre.quiere" "$corrida/$nombre.tiene"
}

# 3. cada repo: del clon del cache si entrega lo que fija el lock; si no (no
# esta, le falta el commit, quedo a medias o no cuadra), de un clon propio de
# esta corrida, que despues queda como el del cache (hallazgos 9ea31f,
# 5471e6). Cada archivo queda en <carpeta>/<ruta sin extension>/<archivo>:
# asi el id de la regla termina en el del registro y los nosemgrep, que se
# comparan por ese final, valen.
set -f # las rutas se separan por espacios, nunca se expanden
while read -r a b c d e f; do
	[ "$a" = repo ] || continue
	nombre=$b carpeta=$c url=$d commit=$e
	rutas=$(awk -v n="$nombre" '$1 == n { print $2 }' "$lock")
	# sin rutas, git archive entregaria el repo entero
	[ -n "$rutas" ] || falla "el repo $nombre no tiene archivos en $lock"
	cuantas=$(printf '%s\n' "$rutas" | wc -l | tr -d ' ')
	awk -v n="$nombre" '$1 == n { print $3 "  " $2 }' "$lock" | LC_ALL=C sort >"$corrida/$nombre.quiere"
	plano="$corrida/plano/$nombre"
	clon="$cache/$nombre.git"
	if [ -d "$clon" ] && extraer "$clon"; then
		:
	else
		nuevo=$(mktemp -d "$cache/clon.XXXXXX") || falla "no se pudo crear un clon para $nombre"
		gitp init -q --bare "$nuevo" || falla "no se pudo crear un clon para $nombre"
		intentos=0
		# reintentos: lo bajado se comprueba igual, asi que reintentar no
		# afloja nada; una transferencia que se arrastra se corta (hallazgo
		# 860bb5)
		until gitp -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=30 --git-dir="$nuevo" \
			fetch -q --depth 1 "$url" "$commit"; do
			intentos=$((intentos + 1))
			if [ "$intentos" -ge 3 ]; then
				rm -rf "$nuevo"
				falla "no se pudo bajar el commit $commit de $url"
			fi
			sleep 3
		done
		if ! extraer "$nuevo"; then
			rm -rf "$nuevo"
			falla "el commit $commit de $url no entrega los archivos que fija $lock (falta una ruta o cambio un sha256)"
		fi
		# queda como el clon del cache para la proxima corrida; si otra lo
		# publico antes, se descarta este
		if [ -e "$clon" ]; then
			mv "$clon" "$clon.viejo.$$" 2>/dev/null || true
			rm -rf "$clon.viejo.$$"
		fi
		if [ -e "$clon" ] || ! mv "$nuevo" "$clon" 2>/dev/null; then
			rm -rf "$nuevo"
		fi
	fi
	for ruta in $rutas; do
		destino="$corrida/reglas/$carpeta/${ruta%.*}"
		[ ! -e "$destino/${ruta##*/}" ] || falla "dos reglas en el mismo lugar: $carpeta/${ruta%.*}"
		mkdir -p "$destino" && mv "$plano/$ruta" "$destino/" || falla "no se pudo ubicar $nombre $ruta"
	done
done <"$lock"
set +f

# 4. semgrep con exec: si vence el plazo del gate, la senal le llega a semgrep
# y no a un sh que lo deja huerfano (hallazgo 860bb5). La carpeta de la
# corrida queda: semgrep la esta leyendo.
trap - EXIT HUP INT TERM
exec semgrep scan --error --quiet --metrics=off --config "$corrida/reglas" --config .hoom/semgrep/reglas/ .
