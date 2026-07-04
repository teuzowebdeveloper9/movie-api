# Referência da API

Base URL local: `http://localhost:8080`

Todas as respostas são JSON. Erros seguem o envelope:

```json
{
  "error": {
    "code": "not_found",
    "message": "movie not found"
  }
}
```

| Código HTTP | `error.code` | Quando |
|---|---|---|
| 400 | `invalid_request` / `invalid_body` | Query inválida, JSON malformado, campo desconhecido, validação de domínio |
| 404 | `not_found` | Filme inexistente |
| 404 | `route_not_found` | Rota inexistente |
| 405 | `method_not_allowed` | Método não suportado na rota |
| 409 | `conflict` | ID duplicado |
| 429 | — | Rate limit excedido |
| 503 | `upstream_unavailable` | Serviço Movies fora do ar |
| 504 | `upstream_timeout` | Serviço Movies não respondeu a tempo |
| 500 | `internal` | Erro inesperado (detalhes só nos logs) |

---

## GET /movies

Lista paginada de filmes.

| Query param | Tipo | Padrão | Descrição |
|---|---|---|---|
| `page` | int | 1 | Página (1-based) |
| `page_size` | int | 20 | Itens por página (máx. 100) |
| `title` | string | — | Busca por substring do título, case-insensitive |
| `genre` | string | — | Filtro exato de gênero, case-insensitive |
| `year` | int | — | Filtro por ano de lançamento |

O dataset oficial (`movies.json`, 28.451 filmes) traz `id`, `title` e `year`; os ids originais são preservados no seed. Os demais campos (`cast`, `genres`, `href`, `extract`, `thumbnail`…) são opcionais e aparecem na resposta quando o filme os possui (ex.: criados via `POST`). O filtro `genre` se aplica a esses filmes.

```bash
curl "http://localhost:8080/movies?page=1&page_size=2&title=matrix"
```

```json
{
  "data": [
    {
      "id": "3495874",
      "title": "SoulMatrix (2014)",
      "year": 2014,
      "cast": [],
      "genres": [],
      "created_at": "2026-07-03T12:00:00Z",
      "updated_at": "2026-07-03T12:00:00Z"
    },
    {
      "id": "328832",
      "title": "The Animatrix (2003)",
      "year": 2003,
      "cast": [],
      "genres": [],
      "created_at": "2026-07-03T12:00:00Z",
      "updated_at": "2026-07-03T12:00:00Z"
    }
  ],
  "meta": {
    "page": 1,
    "page_size": 2,
    "total": 5,
    "total_pages": 3
  }
}
```

---

## GET /movies/{id}

```bash
# ids do movies.json são preservados; filmes criados via POST recebem UUID
curl http://localhost:8080/movies/8
```

`200 OK` com o filme, ou `404`:

```bash
curl -i http://localhost:8080/movies/nao-existe
```

```json
{"error":{"code":"not_found","message":"movie not found"}}
```

---

## POST /movies

Cadastra um filme. Somente `title` e `year` são obrigatórios (`year` entre 1888 e o ano atual + 10). Campos desconhecidos são rejeitados.

```bash
curl -i -X POST http://localhost:8080/movies \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Blade Runner",
    "year": 1982,
    "cast": ["Harrison Ford", "Rutger Hauer"],
    "genres": ["Science Fiction", "Thriller"],
    "href": "Blade_Runner",
    "extract": "Um blade runner precisa caçar replicantes fugitivos em Los Angeles.",
    "thumbnail": "https://example.com/blade-runner.jpg",
    "thumbnail_width": 220,
    "thumbnail_height": 330
  }'
```

**Modo event-driven (padrão no Compose)** — `202 Accepted` + header `Location`:

```http
HTTP/1.1 202 Accepted
Location: /movies/5f1b9db2-...
```

```json
{
  "id": "5f1b9db2-...",
  "status": "accepted",
  "message": "movie creation accepted for asynchronous processing"
}
```

**Modo síncrono** (`ASYNC_WRITES=false`) — `201 Created` + `Location` + o filme criado no corpo.

Validação — `400 Bad Request`:

```bash
curl -s -X POST http://localhost:8080/movies \
  -H "Content-Type: application/json" \
  -d '{"title": "", "year": 1800}'
```

```json
{"error":{"code":"invalid_request","message":"invalid movie: title must not be empty; year must be between 1888 and 2036"}}
```

---

## DELETE /movies/{id}

```bash
curl -i -X DELETE http://localhost:8080/movies/9a2cbe19-9c4d-4b41-8d5c-1c2f36bfb70c
```

- **Modo event-driven** — `202 Accepted` com o mesmo envelope de aceite (a existência do filme é verificada antes de enfileirar; ID inexistente responde `404`);
- **Modo síncrono** — `204 No Content`;
- ID inexistente — `404`.

---

## Health checks

```bash
curl http://localhost:8080/healthz   # liveness do gateway
curl http://localhost:8080/readyz    # readiness: verifica o Movies via gRPC health
```

---

## Swagger

Com `APP_ENV != production`:

```
http://localhost:8080/swagger/index.html
```

A especificação também está versionada em [`api/openapi/swagger.json`](../api/openapi/swagger.json) e [`api/openapi/swagger.yaml`](../api/openapi/swagger.yaml). Em produção a UI não é publicada — ver [security.md](security.md).
