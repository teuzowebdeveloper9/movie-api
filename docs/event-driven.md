# Event-Driven: escritas assíncronas com RabbitMQ

Diferencial implementado: **POST e DELETE processados de forma assíncrona via mensageria**, mantendo o modo síncrono disponível por configuração.

## Ativação

| Configuração | Efeito |
|---|---|
| `RABBITMQ_URL` definido + `ASYNC_WRITES=true` | Modo event-driven (padrão no Compose) |
| `RABBITMQ_URL` vazio ou `ASYNC_WRITES=false` | Escritas síncronas (201/204) |

A decisão vive em um único lugar: o composition root injeta (ou não) o `EventPublisher` no serviço. Handlers, domínio e repositórios não mudam.

## Topologia

```
                          movie.create.requested
                        ┌────────────────────────┐
  Publisher ──► movies.events (topic) ──► movies.write-requests ──► Consumer
                        └────────────────────────┘        │
                          movie.delete.requested          │ nack (poison)
                                                          ▼
                              movies.events.dlx ──► movies.write-requests.dlq
```

| Objeto | Nome | Tipo |
|---|---|---|
| Exchange de eventos | `movies.events` | topic, durável |
| Fila de trabalho | `movies.write-requests` | durável, com DLX |
| Dead letter exchange | `movies.events.dlx` | fanout, durável |
| Dead letter queue | `movies.write-requests.dlq` | durável |
| Routing keys | `movie.create.requested`, `movie.delete.requested` | — |

As declarações são idempotentes e executadas por publisher e consumer — quem subir primeiro cria a topologia.

## Formato do evento

```json
{
  "event_id": "b8a7...",
  "type": "movie.create.requested",
  "occurred_at": "2026-07-03T12:00:00Z",
  "payload": {
    "id": "5f1b9db2-...",
    "title": "Blade Runner",
    "year": 1982,
    "cast": ["Harrison Ford"],
    "genres": ["Science Fiction"],
    "created_at": "2026-07-03T12:00:00Z",
    "updated_at": "2026-07-03T12:00:00Z"
  }
}
```

Mensagens são persistentes (`delivery_mode=2`) e publicadas com **publisher confirms**: o serviço só responde `ACCEPTED` (e o gateway só devolve `202`) depois de o broker confirmar a posse do evento.

## Fluxo do POST assíncrono

1. Gateway recebe o JSON, valida o formato e chama `CreateMovie` via gRPC;
2. O núcleo valida as regras de negócio e gera o UUID **antes** de publicar — validação continua síncrona: entrada inválida responde `400` na hora;
3. `movie.create.requested` é publicado e confirmado pelo broker;
4. Gateway responde `202 Accepted` + `Location: /movies/{id}`;
5. O consumer (goroutine do próprio Movies Service) recebe o evento e chama `ApplyCreate`, que persiste no repositório.

No DELETE, a existência do filme é verificada antes de enfileirar — ID inexistente responde `404` síncrono.

## Semântica e garantias

- **Consistência eventual:** um GET logo após o `202` pode responder `404`; o header `Location` indica onde o recurso estará;
- **At-least-once + idempotência:** redeliveries são tolerados — criar um ID já existente e deletar um ID já removido são tratados como sucesso (`ApplyCreate`/`ApplyDelete`);
- **Poison messages:** evento malformado ou inválido é rejeitado sem requeue e cai na DLQ, nunca travando a fila;
- **Falhas transitórias** (banco indisponível): nack com requeue após pausa de 1s, com `prefetch=1` para preservar ordem simples;
- **Reconexão:** consumer e publisher redialam com backoff exponencial se a conexão com o broker cair.

## Observação prática

```bash
docker compose up -d --build
curl -i -X POST http://localhost:8080/movies -H "Content-Type: application/json" \
  -d '{"title":"Akira","year":1988,"genres":["Animation"]}'
```

- A resposta é `202` com `Location`;
- Instantes depois, `curl http://localhost:8080/movies/{id}` responde `200`;
- Management UI em <http://localhost:15672> (movies/movies) mostra filas, taxas e a DLQ.
