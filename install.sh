#!/bin/sh
# hoomAI installer — https://github.com/oceansbits/hoomai
# Uso:  curl -fsSL https://raw.githubusercontent.com/oceansbits/hoomai/main/install.sh | sh
set -e

REPO="${HOOM_REPO:-oceansbits/hoomai}"
INSTALL_DIR="${HOOM_INSTALL_DIR:-}"

info()  { printf '\033[32mhoomAI:\033[0m %s\n' "$1"; }
error() { printf '\033[31mhoomAI:\033[0m %s\n' "$1" >&2; exit 1; }

# --- detectar plataforma ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  *) error "sistema no soportado: $OS (usa Windows PowerShell: irm .../install.ps1 | iex)" ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) error "arquitectura no soportada: $ARCH" ;;
esac

BIN="hoom-$OS-$ARCH"
URL="https://github.com/$REPO/releases/latest/download/$BIN"
SUMS_URL="https://github.com/$REPO/releases/latest/download/checksums.txt"

# --- elegir directorio de instalacion ---
if [ -z "$INSTALL_DIR" ]; then
  if [ -w /usr/local/bin ]; then
    INSTALL_DIR=/usr/local/bin
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi
mkdir -p "$INSTALL_DIR"

# --- descargar ---
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
info "descargando $BIN (release mas reciente de $REPO)..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$TMP/hoom" "$URL" || error "no se pudo descargar $URL
¿Existe un release publicado? Alternativa: go install github.com/$REPO/cmd/hoom@latest"
  curl -fsSL -o "$TMP/checksums.txt" "$SUMS_URL" 2>/dev/null || true
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$TMP/hoom" "$URL" || error "no se pudo descargar $URL"
  wget -qO "$TMP/checksums.txt" "$SUMS_URL" 2>/dev/null || true
else
  error "se necesita curl o wget"
fi

# --- verificar checksum si es posible ---
if [ -s "$TMP/checksums.txt" ]; then
  SUM_TOOL=""
  command -v sha256sum >/dev/null 2>&1 && SUM_TOOL="sha256sum"
  [ -z "$SUM_TOOL" ] && command -v shasum >/dev/null 2>&1 && SUM_TOOL="shasum -a 256"
  if [ -n "$SUM_TOOL" ]; then
    EXPECTED=$(grep " $BIN\$" "$TMP/checksums.txt" | awk '{print $1}')
    ACTUAL=$($SUM_TOOL "$TMP/hoom" | awk '{print $1}')
    if [ -n "$EXPECTED" ] && [ "$EXPECTED" != "$ACTUAL" ]; then
      error "checksum invalido: esperado $EXPECTED, obtenido $ACTUAL"
    fi
    [ -n "$EXPECTED" ] && info "checksum verificado"
  fi
fi

# --- instalar ---
chmod +x "$TMP/hoom"
mv "$TMP/hoom" "$INSTALL_DIR/hoom"
info "instalado en $INSTALL_DIR/hoom"

# macOS: quitar cuarentena de Gatekeeper si aplica
if [ "$OS" = "darwin" ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$INSTALL_DIR/hoom" 2>/dev/null || true
fi

# --- PATH ---
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) info "AVISO: $INSTALL_DIR no esta en tu PATH. Agrega a tu shell:"
     printf '    export PATH="%s:$PATH"\n' "$INSTALL_DIR" ;;
esac

"$INSTALL_DIR/hoom" version
info "listo. Siguiente paso: cd a tu proyecto y ejecuta 'hoom init'"
