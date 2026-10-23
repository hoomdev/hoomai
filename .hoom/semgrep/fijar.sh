#!/bin/sh
# Rehace los sha256 de .hoom/semgrep/paquetes.lock desde una bajada nueva de
# cada repo en el commit que declara, extraida igual que la extrae gate.sh
# (git archive, solo archivos regulares, ni uno de mas ni uno de menos). Las
# rutas las valida gate.sh. Es parte de actualizar las reglas, que es un acto
# deliberado: cambiar el commit o la lista, correr esto, revisar el diff del
# lock y de las reglas, y correr el gate.
set -eu
unset CDPATH
lock=.hoom/semgrep/paquetes.lock

falla() {
	echo "fijar: $*" >&2
	exit 2
}

sha() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$@"
	else
		shasum -a 256 "$@"
	fi
}
gitp() {
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_NO_REPLACE_OBJECTS=1 GIT_LITERAL_PATHSPECS=1 git "$@"
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
set -f
: >"$tmp/hashes"
while read -r a b c d e f; do
	[ "$a" = repo ] || continue
	nombre=$b url=$d commit=$e
	rutas=$(awk -v n="$nombre" '$1 == n { print $2 }' "$lock")
	[ -n "$rutas" ] || falla "el repo $nombre no tiene archivos en $lock"
	gitp init -q --bare "$tmp/$nombre.git"
	gitp --git-dir="$tmp/$nombre.git" fetch -q --depth 1 "$url" "$commit"
	mkdir -p "$tmp/plano/$nombre"
	# shellcheck disable=SC2086 # rutas sin espacios ni comodines (las valida gate.sh)
	gitp --git-dir="$tmp/$nombre.git" archive -o "$tmp/$nombre.tar" "$commit" -- $rutas
	tar -xf "$tmp/$nombre.tar" -C "$tmp/plano/$nombre"
	# lo mismo que exige gate.sh: un symlink o algo que no es un archivo no
	# se fija (hallazgo 404f93)
	[ -z "$(find "$tmp/plano/$nombre" ! -type f ! -type d)" ] || falla "$nombre trae algo que no es un archivo regular"
	[ "$(find "$tmp/plano/$nombre" -type f | wc -l | tr -d ' ')" = "$(printf '%s\n' "$rutas" | wc -l | tr -d ' ')" ] ||
		falla "$nombre no trae exactamente los archivos del lock"
	# shellcheck disable=SC2086
	(cd "$tmp/plano/$nombre" && sha -- $rutas) | awk -v n="$nombre" '{ print n, $2, $1 }' >>"$tmp/hashes"
done <"$lock"
set +f

# un sha256 por archivo del lock, o nada se toca (hallazgo acaf6e)
[ "$(wc -l <"$tmp/hashes" | tr -d ' ')" = "$(awk '!/^[[:space:]]*(#|$)/ && $1 != "semgrep" && $1 != "repo"' "$lock" | wc -l | tr -d ' ')" ] ||
	falla "no salio un sha256 por cada archivo del lock"
# cada linea de archivo con su sha256 nuevo; el resto del lock queda igual
awk 'FILENAME == ARGV[1] { h[$1 " " $2] = $3; next }
	/^[[:space:]]*(#|$)/ || $1 == "semgrep" || $1 == "repo" { print; next }
	{ k = $1 " " $2; if (!(k in h)) { print "fijar: sin sha256 para " k > "/dev/stderr"; exit 2 }; print $1, $2, h[k] }' \
	"$tmp/hashes" "$lock" >"$tmp/lock"
cp "$tmp/lock" "$lock"
echo "fijar: $(wc -l <"$tmp/hashes" | tr -d ' ') archivos con su sha256 en $lock; revisa el diff"
