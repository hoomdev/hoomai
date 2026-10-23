#!/bin/sh
# Rehace los sha256 de .hoom/semgrep/paquetes.lock desde una bajada nueva de
# cada repo en el commit que declara, extraida igual que la extrae gate.sh. Es
# parte de actualizar las reglas, que es un acto deliberado: cambiar el commit
# o la lista, correr esto y revisar el diff del lock y de las reglas.
set -eu
lock=.hoom/semgrep/paquetes.lock

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
	[ -n "$rutas" ] || { echo "fijar: el repo $nombre no tiene archivos en $lock" >&2; exit 2; }
	gitp init -q --bare "$tmp/$nombre.git"
	gitp --git-dir="$tmp/$nombre.git" fetch -q --depth 1 "$url" "$commit"
	mkdir -p "$tmp/plano/$nombre"
	# shellcheck disable=SC2086 # rutas sin espacios ni comodines (las valida gate.sh)
	gitp --git-dir="$tmp/$nombre.git" archive -o "$tmp/$nombre.tar" "$commit" -- $rutas
	tar -xf "$tmp/$nombre.tar" -C "$tmp/plano/$nombre"
	# shellcheck disable=SC2086
	(cd "$tmp/plano/$nombre" && sha $rutas) | awk -v n="$nombre" '{ print n, $2, $1 }' >>"$tmp/hashes"
done <"$lock"
set +f

# cada linea de archivo con su sha256 nuevo; el resto del lock queda igual
awk 'NR == FNR { h[$1 " " $2] = $3; next }
	/^(#|$)/ || $1 == "semgrep" || $1 == "repo" { print; next }
	{ k = $1 " " $2; if (!(k in h)) { print "fijar: sin sha256 para " k > "/dev/stderr"; exit 2 }; print $1, $2, h[k] }' \
	"$tmp/hashes" "$lock" >"$tmp/lock"
cp "$tmp/lock" "$lock"
echo "fijar: $(wc -l <"$tmp/hashes" | tr -d ' ') archivos con su sha256 en $lock; revisa el diff"
