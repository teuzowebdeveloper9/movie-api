# Kubernetes

Manifests completos em [`deploy/k8s`](../deploy/k8s), organizados com Kustomize.

## Conteúdo

| Arquivo | Recursos |
|---|---|
| `namespace.yaml` | Namespace `movie-api` |
| `mongodb.yaml` | StatefulSet com PVC (1Gi) + headless Service, com autenticação habilitada |
| `rabbitmq.yaml` | Deployment + Service |
| `movies.yaml` | ConfigMap + Deployment (2 réplicas) + Service ClusterIP + HPA |
| `gateway.yaml` | ConfigMap + Deployment (2 réplicas) + Service + Ingress + HPA |
| `secrets/*.env.example` | Modelos das credenciais — os `.env` reais ficam fora do git |

Destaques:

- **Probes:** o Movies usa o probe gRPC nativo do Kubernetes (servidor implementa o gRPC Health Checking Protocol); o gateway usa `httpGet` em `/healthz` (liveness) e `/readyz` (readiness — verifica o Movies antes de receber tráfego);
- **Segurança:** `runAsNonRoot`, `readOnlyRootFilesystem`, `capabilities: drop ALL` e seccomp `RuntimeDefault` nos dois serviços;
- **Autoscaling:** HPA por CPU (70%, 2–5 réplicas) para gateway e Movies;
- **Configuração:** ConfigMaps para valores não sensíveis; credenciais (RabbitMQ e MongoDB) em Secrets gerados pelo `secretGenerator` do Kustomize a partir de arquivos locais fora do git — **nenhuma senha versionada**. O sufixo de hash nos nomes dos Secrets provoca rolling update automático quando uma credencial muda;
- **MongoDB com auth:** o mongod sobe com `--auth` (usuário root criado via `MONGO_INITDB_ROOT_*`); o serviço Movies conecta com a URI autenticada vinda do Secret;
- **Produção de verdade:** o gateway roda com `APP_ENV=production` — Swagger desabilitado.

## Executando localmente (kind ou minikube)

Pré-requisitos: `kubectl` e um cluster local ([kind](https://kind.sigs.k8s.io/) ou [minikube](https://minikube.sigs.k8s.io/)) com um ingress controller nginx.

```bash
kind create cluster --name movie-api

make k8s-secrets   # gera deploy/k8s/secrets/{rabbitmq,mongodb}.env com senhas aleatórias

kubectl apply -k deploy/k8s
# ou: make k8s-apply

kubectl -n movie-api get pods -w
```

Sem os arquivos de credenciais o `kubectl apply -k` falha de propósito — é o que garante que nenhum ambiente sobe com senha default.

As imagens vêm do GHCR (`ghcr.io/teuzowebdeveloper9/movie-api/{gateway,movies}:latest`), publicadas pelo CI.

Acesso sem ingress:

```bash
kubectl -n movie-api port-forward svc/gateway 8080:80
curl http://localhost:8080/movies
```

Com ingress nginx, adicione `movie-api.local` ao `/etc/hosts` apontando para o IP do ingress e acesse `http://movie-api.local/movies`.

Limpeza:

```bash
kubectl delete -k deploy/k8s   # ou: make k8s-delete
kind delete cluster --name movie-api
```

## Notas de produção

- O MongoDB roda como StatefulSet de nó único por simplicidade; em produção usaria um operador (MongoDB Community Operator) ou banco gerenciado;
- Os Secrets são gerados localmente pelo Kustomize a partir de arquivos fora do git — em um cluster real com GitOps, viriam de um cofre (External Secrets, Sealed Secrets, Vault), mantendo o mesmo contrato (`secretKeyRef`);
- As variáveis `MONGO_INITDB_ROOT_*` criam o usuário apenas com o volume vazio: para rotacionar a senha de um volume existente, atualize também o usuário no MongoDB (`db.changeUserPassword`) — com cofre + operador isso vira um fluxo automatizado;
- `imagePullPolicy: Always` + tag `latest` simplificam o desafio; em produção, tags imutáveis por SHA/semver (o CI já as publica).
