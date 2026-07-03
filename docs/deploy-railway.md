# Deploy no Railway

**API em produção:** <https://gateway-production-7813.up.railway.app/movies>

```bash
curl "https://gateway-production-7813.up.railway.app/movies?page=1&page_size=5"
curl -i https://gateway-production-7813.up.railway.app/swagger/index.html   # 404: Swagger não sobe em produção
```

## Por que Railway

Eu queria um deploy **simples**, sem gerenciar infraestrutura, e que aceitasse diretamente as **imagens Docker geradas pelo CI**. O Railway atende exatamente isso:

- serviços criados a partir de uma imagem de registry (GHCR) em poucos passos;
- rede privada entre os serviços do mesmo projeto (o gRPC do Movies nunca é exposto publicamente);
- injeção automática de `PORT` (o gateway já a respeita);
- MongoDB e RabbitMQ provisionados na própria plataforma;
- domínio público gerenciado com TLS para o gateway.

Trade-offs da escolha em [trade-offs.md](trade-offs.md).

## Topologia no Railway

```
┌──────────────────────── projeto movie-api ────────────────────────┐
│                                                                   │
│  gateway (imagem GHCR)  ──gRPC/rede privada──►  movies (imagem GHCR)  │
│      │ domínio público                              │             │
│      ▼                                              ├──► MongoDB  │
│  https://<app>.up.railway.app                       └──► RabbitMQ │
└───────────────────────────────────────────────────────────────────┘
```

## Imagens

Publicadas pelo pipeline de CI a cada push na `main`:

```
ghcr.io/teuzowebdeveloper9/movie-api/gateway:latest
ghcr.io/teuzowebdeveloper9/movie-api/movies:latest
```

## Variáveis de ambiente por serviço

### movies

| Variável | Valor |
|---|---|
| `DB_DRIVER` | `mongo` |
| `MONGO_URI` | connection string do MongoDB do Railway (referência `${{MongoDB.MONGO_URL}}`) |
| `MONGO_DATABASE` | `movies` |
| `RABBITMQ_URL` | URL AMQP do RabbitMQ do Railway |
| `ASYNC_WRITES` | `true` |
| `SEED_ENABLED` | `true` |
| `GRPC_PORT` | `50051` |

### gateway

| Variável | Valor |
|---|---|
| `APP_ENV` | `production` ← **Swagger desabilitado** (ver [security.md](security.md)) |
| `MOVIES_GRPC_ADDR` | `movies.railway.internal:50051` (rede privada) |
| `RATE_LIMIT_RPM` | `300` |

Somente o gateway recebe domínio público. O Swagger não é publicado em produção — a especificação continua disponível no repositório (`api/openapi/`).

## Deploy contínuo

O job `deploy` do CI ([.github/workflows/ci.yml](../.github/workflows/ci.yml)) dispara um redeploy dos serviços via API GraphQL do Railway após publicar as novas imagens. Para ativá-lo, configure no GitHub:

- Secret `RAILWAY_TOKEN` — token de projeto (Railway → Settings → Tokens);
- Variables `RAILWAY_ENVIRONMENT_ID`, `RAILWAY_MOVIES_SERVICE_ID`, `RAILWAY_GATEWAY_SERVICE_ID`.

Sem essas credenciais o job encerra sem erro (skip explícito), mantendo o pipeline verde.

## Passo a passo manual (reproduzível)

1. Crie um projeto no Railway;
2. Adicione um serviço **MongoDB** (template oficial) e um **RabbitMQ** (template);
3. Crie o serviço **movies**: Source = Docker Image → `ghcr.io/teuzowebdeveloper9/movie-api/movies:latest`; configure as variáveis da tabela acima; não gere domínio público;
4. Crie o serviço **gateway**: Source = Docker Image → `ghcr.io/teuzowebdeveloper9/movie-api/gateway:latest`; configure as variáveis; gere o domínio público;
5. Teste:

```bash
curl https://<dominio-do-gateway>/movies
curl -i https://<dominio-do-gateway>/swagger/index.html   # 404: produção não publica Swagger
```

> As imagens no GHCR precisam estar com visibilidade **pública** (Package settings → Change visibility) para o Railway conseguir puxá-las sem credenciais de registry.
