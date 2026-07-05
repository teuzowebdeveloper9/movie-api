# Trade-offs e decisões técnicas

Registro das principais escolhas, o que foi ganho e o que foi conscientemente sacrificado.

## Plataforma de deploy: Railway

**Escolha:** Railway, deployando as imagens Docker geradas pelo CI (GHCR).

**Por quê:** eu queria um deploy simples e direto — o Railway permite subir um serviço apontando para uma imagem Docker de um registry em poucos cliques/chamadas, com rede privada entre serviços, injeção automática de `PORT`, domínio público gerenciado e provisionamento de MongoDB e RabbitMQ na própria plataforma. Isso elimina a necessidade de gerenciar cluster, load balancer e certificados para um projeto deste porte.

**Trade-off:** menos controle fino que um Kubernetes gerenciado (EKS/GKE). Mitigação: os manifests Kubernetes completos estão em `deploy/k8s`, então a migração para um cluster é questão de `kubectl apply -k`.

## Monorepo com dois binários

**Escolha:** um único módulo Go com `cmd/gateway` e `cmd/movies`.

**Ganho:** o código gerado do Protobuf (`gen/`) é compartilhado sem publicar módulo ou duplicar contrato; CI única; refatorações atômicas.

**Sacrifício:** em uma organização grande, times donos de serviços diferentes normalmente preferem repositórios separados com o contrato publicado como módulo versionado. Para o escopo do desafio, o monorepo entrega os mesmos containers independentes com muito menos atrito.

## chi em vez de gin/echo

**Escolha:** `go-chi/chi` como router HTTP.

**Por quê:** o chi é 100% compatível com `net/http` (handlers padrão, middlewares padrão), minimalista e não impõe conventions de framework. Combina com a proposta hexagonal de manter frameworks nas bordas — trocar de router não afeta nada além de `internal/gateway/router`.

**Sacrifício:** gin traz mais baterias inclusas (binding, validação). Aqui o binding manual é trivial e a validação pertence ao domínio, não ao framework.

## IDs: UUID string em vez de ObjectID

**Escolha:** UUIDs gerados pela aplicação, armazenados como `_id` string no Mongo e `id` no DynamoDB.

**Ganho:** o mesmo ID funciona em qualquer backend de persistência (requisito para o port ser realmente plugável) e pode ser gerado **antes** da escrita — essencial no fluxo assíncrono, onde o cliente recebe `Location: /movies/{id}` no `202` antes de o registro existir.

**Sacrifício:** ObjectID é menor e ordenável por tempo. Irrelevante nesta escala.

## Escrita assíncrona com resposta 202

**Escolha:** no modo event-driven, POST/DELETE publicam evento e respondem `202 Accepted` + `Location`; um consumer aplica a escrita.

**Ganho:** desacopla a disponibilidade da API da disponibilidade do banco; absorve picos; demonstra o padrão pedido no diferencial. A publicação usa **publisher confirms** — o 202 só sai depois de o broker aceitar o evento.

**Sacrifício:** consistência eventual — um GET imediatamente após o 202 pode responder 404. É a semântica correta do 202 e está documentada. O DELETE verifica a existência antes de enfileirar para que erros óbvios (404) sejam síncronos.

## Consumer com DLQ em vez de retry infinito

**Escolha:** eventos inválidos (poison messages) vão para uma Dead Letter Queue; falhas transitórias (banco fora) são reenfileiradas com pausa.

**Ganho:** a fila nunca trava atrás de uma mensagem podre e nenhum evento é perdido silenciosamente.

**Sacrifício:** sem contador de tentativas persistente, uma falha classificada como transitória que na verdade é permanente pode reciclar por mais tempo que o ideal. Evolução: retry com backoff + limite via header `x-death`.

## Listagem no DynamoDB via Query em GSI ordenada

**Escolha:** `GET /movies` no driver DynamoDB consulta a GSI `title-sort-index` (partition key constante `gsi_pk`, sort key `title_sort` = título minúsculo + id). Os itens já chegam do servidor na ordem da listagem e a leitura para assim que a janela da página (`offset + page_size`) enche; os filtros de título/gênero/ano viram `FilterExpression` server-side com a mesma semântica de `ListFilter.Matches`. `EnsureTable` cria a GSI junto com a tabela e, em tabelas pré-existentes, adiciona o índice via `UpdateTable` e faz backfill dos itens antigos (a GSI é esparsa: itens sem `gsi_pk` ficariam invisíveis nela).

**Ganho:** sem Scan e sem ordenação em memória — no caso comum (primeiras páginas) a leitura é O(janela) em vez de O(tabela), com memória O(janela).

**Sacrifício:** o total exato ainda exige uma segunda Query com `Select=COUNT` percorrendo a partição — inerente ao contrato "página numérica + total", mas sem transferir dados de item. A partition key constante concentra a GSI em uma única partição: suficiente para os 28k registros do desafio; em escala real a chave seria fragmentada (`MOVIE#0..N`) ou a paginação migraria para cursor por `LastEvaluatedKey`. Leituras de GSI são eventualmente consistentes — o que o fluxo de escrita assíncrona já implica.

## Código gerado versionado no repositório

**Escolha:** `gen/` (protoc) e `api/openapi` (swag) são commitados.

**Ganho:** `go build` e CI funcionam sem `protoc`/`swag` instalados; builds reproduzíveis; diffs de contrato visíveis em code review.

**Sacrifício:** risco de dessincronizar contrato e código gerado. Mitigação: `make proto` e `make swagger` regeneram tudo em um comando.

## Rate limiting em memória

**Escolha:** `httprate` com contadores em memória, chave = IP do cliente.

**Sacrifício:** por réplica (N réplicas ⇒ limite efetivo N×) e, atrás de proxy sem configuração de IP confiável, clientes de um mesmo proxy compartilham bucket. Para o desafio é suficiente; a evolução (contador em Redis + `ClientIPFromXFFTrustedProxies` com os CIDRs do edge) está isolada no router.

## gRPC interno sem TLS

**Escolha:** `insecure.NewCredentials()` entre gateway e Movies.

**Por quê:** a comunicação acontece exclusivamente em rede privada (network do Compose, ClusterIP no K8s, private networking no Railway); o serviço Movies não expõe porta pública em nenhum ambiente.

**Evolução:** mTLS via service mesh (Linkerd/Istio) ou certificados gerenciados, sem mudança de código relevante (troca de credentials no dial/listen).

## Seed embutido no serviço

**Escolha:** o próprio Movies Service faz o seed idempotente do `movies.json` na inicialização (`SEED_ENABLED=true`), com o dataset embutido na imagem.

**Ganho:** cumpre o requisito de "subir com um único comando" já com dados utilizáveis, em qualquer ambiente (Compose, K8s, Railway), sem job separado.

**Sacrifício:** em produção real, carga de dados costuma ser um job de migração separado do ciclo de vida do serviço. O seed é desligável por env var.

## Swagger fora de produção

Ver [security.md](security.md) — resumo: reduzir superfície de reconhecimento; a documentação vive no repositório e nos ambientes internos, não no endpoint público.
