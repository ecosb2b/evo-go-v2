#!/usr/bin/env bash
# Roda `go test` dentro de um container, para máquinas sem Go instalado.
#
# O .dockerignore exclui *_test.go do contexto de build, então os testes não
# rodam via `docker build`: aqui o repositório é montado direto no container.
# Os volumes evogo-gomod e evogo-gocache guardam módulos e cache de build entre
# execuções.
#
# Uso:  scripts/go-test.sh [args do go test]   (padrão: ./pkg/...)
#       scripts/go-test.sh build ./...          (roda go build/vet em vez de test)
#       scripts/go-test.sh -race ./pkg/whatsmeow/service/ -run TestRuntime
set -euo pipefail

GO_IMAGE="${GO_IMAGE:-golang:1.25.0-alpine}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Git Bash no Windows: converte o caminho para o formato que o Docker entende
# e impede o MSYS de reescrever os caminhos do container.
if command -v cygpath >/dev/null 2>&1; then
	ROOT="$(cygpath -w "$ROOT")"
	export MSYS_NO_PATHCONV=1
fi

GO_SUBCMD=test
case "${1:-}" in
build | vet) GO_SUBCMD="$1"; shift ;;
esac

if [ "$#" -eq 0 ]; then
	set -- ./pkg/...
fi

docker run --rm \
	-v "$ROOT:/src" \
	-v evogo-gomod:/go/pkg/mod \
	-v evogo-gocache:/root/.cache/go-build \
	-w /src \
	-e CGO_ENABLED=1 \
	-e GO_SUBCMD="$GO_SUBCMD" \
	"$GO_IMAGE" \
	sh -c 'apk add --no-cache build-base libjpeg-turbo-dev libwebp-dev git >/dev/null && go "$GO_SUBCMD" "$@"' -- "$@"
