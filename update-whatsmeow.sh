#!/usr/bin/env bash
# Atualiza a dependencia go.mau.fi/whatsmeow para a versao mais recente,
# usando um container Go de uso unico (nao precisa de Go instalado no Windows).
# Uso:  bash update-whatsmeow.sh
set -e
export MSYS_NO_PATHCONV=1

echo ">>> Versao atual do whatsmeow:"
grep 'go.mau.fi/whatsmeow ' go.mod || true
echo ""
echo ">>> Atualizando (go get + go mod tidy) num container Go..."
docker run --rm \
  -v "$(pwd):/app" \
  -w /app \
  golang:1.25.0-alpine \
  sh -c "apk add --no-cache git >/dev/null 2>&1 && \
         go get go.mau.fi/whatsmeow@latest && \
         go mod tidy"

echo ""
echo ">>> Nova versao do whatsmeow:"
grep 'go.mau.fi/whatsmeow ' go.mod
echo ""
echo ">>> Diff do go.mod:"
git diff go.mod | grep whatsmeow || echo "(sem mudanca — ja estava na ultima)"
