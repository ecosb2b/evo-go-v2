---
name: release
description: This skill should be used when the user wants to "cut a release", "publish a new version", "gerar uma imagem nova", "subir a versão", "fazer deploy", "lançar 0.7.x", or otherwise publish a new container image of this fork to ghcr.io/ecosb2b/evo-go-v2. It covers bumping VERSION, tagging, verifying the published digest, writing the GitHub release, and updating CHANGELOG-FORK.md.
version: 1.0.0
---

# Publicar uma release do evo-go-v2

Publica uma imagem nova em `ghcr.io/ecosb2b/evo-go-v2` e registra a release.

## Como a publicação funciona neste repositório

A imagem nasce de **tag**, não de push na `main`.

| Ação | Publica imagem? |
|---|---|
| `git push origin main` | não |
| `git push origin <tag>` | **sim** |

O workflow [`.github/workflows/publish_docker_image.yml`](../../../.github/workflows/publish_docker_image.yml)
dispara em tags no formato `N.N.N` (sem prefixo `v`) e falha de propósito se o
nome da tag divergir do arquivo `VERSION`.

## Passo a passo

### 1. Verificar o ponto de partida

```bash
git rev-parse --abbrev-ref HEAD     # tem que estar em main
git status -sb                       # tem que estar sincronizada e limpa
cat VERSION                          # versão atual
git describe --tags --abbrev=0       # última tag
```

`VERSION` e a última tag devem ser iguais. Se divergirem, alguém publicou fora do
fluxo — investigue antes de seguir.

### 2. Validar que compila

Não existe workflow de CI: **nenhum teste roda automaticamente**, e o `docker build`
executa `go build`, que ignora arquivos `_test.go`. Então valide localmente antes
de marcar a tag.

```bash
docker build --target build -t evo-go-release-check .
```

O Docker Desktop pode não estar rodando mesmo com o CLI respondendo — se der
`failed to connect to the docker API`, inicie-o e espere o daemon.

Nunca canalize o build para `tail` sem `set -o pipefail`: o exit code passa a ser
o do `tail` e um build quebrado parece verde.

### 3. Subir o VERSION

```bash
echo "0.7.4" > VERSION
git add VERSION
git commit -m "chore: release 0.7.4"
git push origin main
```

Este push **não** publica nada.

### 4. Marcar e empurrar a tag

```bash
git tag -a 0.7.4 -m "release 0.7.4"
git push origin 0.7.4
```

Aqui a build dispara.

### 5. Acompanhar

```bash
gh run list --repo ecosb2b/evo-go-v2 --limit 1
gh run watch <RUN_ID> --repo ecosb2b/evo-go-v2 --exit-status
```

Confirme a conclusão real, não só o exit code:

```bash
gh run view <RUN_ID> --repo ecosb2b/evo-go-v2 --json conclusion
```

### 6. Capturar o digest publicado

```bash
gh run view <RUN_ID> --repo ecosb2b/evo-go-v2 --log \
  | grep "pushing manifest for ghcr"
```

Confirme puxando de verdade, em vez de confiar no log:

```bash
docker pull ghcr.io/ecosb2b/evo-go-v2:0.7.4
docker image inspect ghcr.io/ecosb2b/evo-go-v2:0.7.4 --format '{{index .RepoDigests 0}}'
```

### 7. Registrar

Atualize [`CHANGELOG-FORK.md`](../../../CHANGELOG-FORK.md) — **nunca** o
`CHANGELOG.md`, que é do upstream e conflita a cada sincronização.

Cada entrada leva imagem, digest, commit e o que mudou.

Depois crie a release:

```bash
gh release create 0.7.4 --repo ecosb2b/evo-go-v2 \
  --title "0.7.4 — <resumo>" --notes-file notas.md
```

Commits só de `.md` **não** disparam build (há `paths-ignore` no workflow), então
atualizar o changelog depois é seguro.

## Armadilhas já encontradas neste repositório

**Digest muda a cada rebuild.** O Dockerfile faz `COPY . .`, então qualquer arquivo
do repositório entra no contexto de build. A 0.7.3 foi publicada duas vezes por
causa disso — a segunda ao adicionar o changelog. O `paths-ignore` cobre `.md` e
os diretórios não-código, mas mudanças no próprio workflow ainda republicam.

**`latest` é móvel.** Sempre oriente fixar a tag de versão no compose. Um
`docker compose pull` com `latest` troca a imagem embaixo de um servidor em
produção sem aviso.

**`docs/docs.go` é código.** É Go gerado pelo `swag` e vai para o binário — não
inclua em `paths-ignore`.

**Instâncias em `connected = false` não voltam sozinhas.** O `ConnectOnStartup`
filtra por `connected = true`, então instâncias que caíram ficam invisíveis para
ele. Depois do deploy elas precisam de um `POST /instance/connect` cada uma. Sempre
mencione isso ao entregar uma release.

## O que dizer ao usuário no fim

Entregue: a tag da imagem, o digest, o link da release, e estes três itens.

Atualizar o servidor (o deploy roda no ambiente do usuário, não daqui):

```bash
docker compose pull && docker compose up -d
```

Conferir que o servidor pegou a imagem certa — o digest tem que bater com o
publicado:

```bash
docker image inspect ghcr.io/ecosb2b/evo-go-v2:<versão> --format '{{index .RepoDigests 0}}'
```

Acompanhar o problema que originou este fork:

```sql
SELECT count(*), state FROM pg_stat_activity
WHERE datname = 'evogo_auth' GROUP BY state;
```

Deve estabilizar abaixo de 20 e recuar sozinho quando a carga cai. Antes das
correções crescia sem parar até `FATAL: sorry, too many clients already`.

## Rollback

As tags anteriores continuam publicadas. Basta apontar o compose para a versão
anterior e `docker compose up -d`.

Digests conhecidos estão em `CHANGELOG-FORK.md`. Atenção: a tag `0.7.2` foi
reescrita duas vezes antes de adotarmos versionamento próprio — o changelog
registra qual digest é qual.
