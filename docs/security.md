# Segurança

## Swagger não é publicado em produção

A decisão mais visível do projeto: a UI do Swagger e o `swagger.json` **não sobem junto com o deploy de produção**.

### O racional

Em um ataque real, grande parte do esforço inicial é **reconhecimento**: mapear a superfície da aplicação — quais rotas existem, quais verbos aceitam, quais parâmetros e formatos de payload são válidos, onde estão as operações de escrita e remoção. Ferramentas de fuzzing e scanners precisam adivinhar isso por força bruta, o que é lento, barulhento e detectável.

Um Swagger público em produção elimina essa barreira: entrega, de mão beijada, o inventário completo de rotas, schemas com tipos e exemplos, códigos de resposta e regras de validação. O atacante deixa de gastar tempo descobrindo a API e passa direto para a exploração — payloads certeiros na primeira tentativa.

### A implementação

O gateway decide em tempo de execução (`internal/gateway/config`):

| Ambiente | Comportamento |
|---|---|
| `APP_ENV != production` | Swagger em `/swagger/index.html` |
| `APP_ENV = production` | A rota `/swagger/*` **não é registrada** no router — responde `404` como qualquer rota inexistente |
| Override explícito | `SWAGGER_ENABLED=true/false` força qualquer um dos dois estados |

A documentação continua disponível para quem deve tê-la: no repositório (`api/openapi/`), nos ambientes internos de dev/staging e nesta pasta `docs/`.

### Isso não é "segurança por obscuridade"?

Seria, se fosse a única defesa. Esconder a documentação **não substitui** controles reais — e eles existem no projeto (abaixo). É **redução de superfície de reconhecimento**, um princípio de defesa em profundidade: não oferecer gratuitamente informação que só facilita o ataque. O mesmo motivo pelo qual não se expõe stack trace em produção.

## Demais controles

| Controle | Onde |
|---|---|
| Rate limiting por IP (429) | Gateway, `httprate`, configurável via `RATE_LIMIT_RPM` |
| Timeout por requisição | Gateway (`REQUEST_TIMEOUT`) + timeouts do `http.Server` |
| Payload máximo de 1 MiB | `http.MaxBytesReader` no decode do POST |
| JSON estrito | `DisallowUnknownFields` + rejeição de dados após o objeto |
| Validação de entrada na borda do domínio | `internal/movies/core/domain` (título, ano, dimensões) |
| Erros 5xx genéricos | Detalhes de banco/broker só nos logs; resposta expõe apenas `internal error` |
| Containers distroless non-root | Sem shell, sem package manager, filesystem mínimo |
| `securityContext` no Kubernetes | `runAsNonRoot`, `readOnlyRootFilesystem`, `capabilities: drop ALL`, seccomp `RuntimeDefault` |
| Secrets fora do código | Tudo via variável de ambiente; no K8s, `Secret`; nada commitado |
| Serviço Movies sem exposição pública | Só o gateway publica porta; gRPC fica na rede interna (Compose network / ClusterIP / rede privada do Railway) |
| Recover de panics | Middleware no gateway + interceptor no gRPC (panic vira 500/`Internal`, processo não morre) |

## Limitações conhecidas (e evoluções)

- **gRPC interno sem TLS** — aceitável em rede privada de confiança; a evolução natural é mTLS (service mesh ou certificados próprios);
- **MongoDB sem autenticação no Compose local** — conveniência de desenvolvimento; em produção use connection string com credenciais (o código já aceita qualquer `MONGO_URI`);
- **Rate limit em memória por réplica** — com N réplicas o limite efetivo é N× o configurado; a evolução é um contador compartilhado (Redis);
- **Sem autenticação de usuários** — fora do escopo do desafio; o desenho comporta um middleware de JWT/OAuth2 no gateway sem tocar no serviço Movies.
