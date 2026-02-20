# kafkar — Observabilidade completa com Kafka + OpenTelemetry

Projeto de estudo que demonstra **observabilidade de ponta a ponta** em um sistema distribuído com Kafka e Go: traces distribuídos, métricas de negócio, logs estruturados, baggage e tratamento de erros — tudo visível no Grafana e/ou no Kibana APM.

---

## Fluxo do sistema

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Cliente HTTP                                │
└────────────────────────────┬────────────────────────────────────────┘
                             │ POST /orders
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         order-api :8080                             │
│                                                                     │
│  OrderController.Create                                             │
│    ├─ valida body                                                   │
│    ├─ injeta customer_id no Baggage OTEL                            │
│    └─ OrderUseCase.PlaceOrder                                       │
│         ├─ cria span filho "PlaceOrder"                             │
│         ├─ ~20% chance → retorna ErrPaymentDeclined (HTTP 402)      │
│         ├─ publica Order no tópico "orders"                         │
│         └─ registra métricas: orders_created_total, order_value_cents│
└─────────────────────────┬───────────────────────────────────────────┘
                          │ Kafka: tópico "orders" (3 partições)
                          ▼
┌─────────────────────────────────────────────────────────────────────┐
│                           consumer                                  │
│                                                                     │
│  processOrder                                                       │
│    ├─ extrai trace context dos headers Kafka                        │
│    ├─ extrai customer_id do Baggage                                 │
│    ├─ simula processamento (50–500ms)                               │
│    ├─ registra métricas: messages_consumed_total, processing_time   │
│    └─ publica Payment no tópico "payments"                          │
└─────────────────────────┬───────────────────────────────────────────┘
                          │ Kafka: tópico "payments" (3 partições)
                          ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        payment-api :8081                            │
│                                                                     │
│  POST /payments/confirm (chamado por quem consome "payments")       │
│  PaymentController.Confirm                                          │
│    └─ PaymentUseCase.ConfirmPayment                                 │
│         ├─ cria span filho "ConfirmPayment"                         │
│         ├─ recupera customer_id do Baggage                          │
│         └─ registra métrica: payments_confirmed_total               │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Stack de observabilidade

```
┌──────────────────────────────────────────────────────────────────────┐
│                       Aplicações Go                                  │
│   order-api │ payment-api │ consumer │ producer                     │
│                   OpenTelemetry SDK                                  │
│        traces + metrics + logs (via OTLP gRPC)                      │
└────────────────────────────┬─────────────────────────────────────────┘
                             │ OTLP gRPC :4317
                             ▼
┌──────────────────────────────────────────────────────────────────────┐
│                     otel-collector                                   │
│                                                                      │
│  receivers:  otlp (grpc + http)                                      │
│  processors: resource/remove-noise, transform/remove-scope, batch    │
│  exporters:                                                          │
│    ├─ otlphttp/tempo   → Tempo :4318      (traces)                   │
│    ├─ prometheusremotewrite → Prometheus  (metrics)                  │
│    ├─ loki             → Loki :3100       (logs)                     │
│    └─ otlphttp/apm     → APM Server :8200 (traces+metrics+logs)      │
└──────┬──────────┬─────────────┬──────────────┬───────────────────────┘
       │          │             │              │
       ▼          ▼             ▼              ▼
    Tempo      Prometheus     Loki        APM Server
       │          │             │              │
       └──────────┴──────────── ┼──────────────┘
                                │
                                ▼
                   Grafana :3001  /  Kibana :5601
```

---

## Camadas do trace

Cada requisição gera uma árvore de spans visível no Tempo e no APM:

```
[HTTP POST /orders]                           ← otelfiber (order-api)
  └─ [PlaceOrder]                             ← order/usecase.go
       └─ [publish orders]                    ← kafka/producer.go
            └─ [receive orders]               ← kafka/consumer.go
                 └─ [publish payments]        ← kafka/producer.go
```

Spans de erro (~20% das requisições) aparecem em vermelho com status `Error` e o atributo `customer_id` propagado via Baggage em todos os níveis.

---

## Estrutura do projeto

```
kafkar/
├── cmd/
│   ├── order-api/main.go      # API HTTP Fiber, porta 8080
│   ├── payment-api/main.go    # API HTTP Fiber, porta 8081
│   ├── consumer/main.go       # Consome "orders", publica "payments"
│   ├── load-gen/main.go       # Automatic load generator
│   └── producer/main.go       # Publica eventos genéricos (referência)
│
├── internal/
│   ├── kafka/
│   │   ├── producer.go        # Wrapper kafka.Writer + propagação de trace
│   │   ├── consumer.go        # Wrapper kafka.Reader + extração de trace
│   │   └── admin.go           # Criação de tópicos
│   ├── order/
│   │   ├── usecase.go         # PlaceOrder: erro ~20%, publica no Kafka
│   │   └── controller.go      # Handler Fiber + injeção de Baggage
│   ├── payment/
│   │   ├── usecase.go         # ConfirmPayment: loga e registra métrica
│   │   └── controller.go      # Handler Fiber
│   ├── models/
│   │   ├── order.go           # Order{ID, CustomerID, Items, TotalCents}
│   │   ├── payment.go         # Payment{OrderID, Status, ConfirmedAt}
│   │   └── event.go           # Event genérico (producer de referência)
│   └── telemetry/
│       ├── logger.go          # Setup OTLP: traces + metrics + logs
│       └── metrics.go         # Contadores e histogramas
│
├── monitoring/
│   ├── otel-collector-config.yaml          # Sem elastic
│   ├── otel-collector-config.elastic.yaml  # Com APM Server
│   ├── tempo-config.yaml
│   ├── loki-config.yaml
│   ├── prometheus.yaml
│   ├── grafana-datasources.yaml
│   ├── apm-server.yml
│   └── kibana.yml
│
├── docker-compose.yml          # Stack sem Elastic
├── docker-compose.elastic.yml  # Stack completa com Elastic + Kibana + APM
├── Dockerfile
└── Makefile
```

---

## Métricas instrumentadas

| Métrica | Tipo | Labels | Descrição |
|---|---|---|---|
| `orders_created_total` | Counter | `status` (ok/declined/error) | Pedidos criados |
| `order_value_cents` | Histogram | — | Valor do pedido em centavos |
| `payments_confirmed_total` | Counter | — | Pagamentos confirmados |
| `messages_consumed_total` | Counter | `customer_id` | Mensagens consumidas do Kafka |
| `message_processing_duration_seconds` | Histogram | `customer_id` | Tempo de processamento |
| `messages_published_total` | Counter | — | Mensagens publicadas no Kafka |

---

## Como rodar

### Stack sem Elastic (Grafana + Tempo + Loki + Prometheus)

```bash
docker compose up --build
```

### Stack completa com Elastic (+ Kibana + APM Server)

```bash
docker compose -f docker-compose.elastic.yml up --build
```

### Testar manualmente

```bash
# Criar um pedido (80% sucesso, 20% payment declined)
curl -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"c1","items":["item-a","item-b"],"total_cents":9900}'

# Confirmar pagamento
curl -X POST http://localhost:8081/payments/confirm \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"<uuid>","customer_id":"c1","total_cents":9900}'

# Health checks
curl http://localhost:8080/health
curl http://localhost:8081/health
```

### Load generator (`load-gen`)

O serviço `load-gen` sobe junto com o compose e bate na `order-api` automaticamente a cada 2 segundos com clientes e valores aleatórios — não é necessário fazer nada manualmente para ver traces no Grafana/Kibana.

| Variável | Default | Descrição |
|---|---|---|
| `ORDER_API_ADDR` | `http://localhost:8080` | Endereço da order-api |
| `INTERVAL_MS` | `2000` | Intervalo entre requests em ms |

Para ajustar a cadência sem rebuild:

```bash
INTERVAL_MS=500 docker compose up --build   # 2 req/s → 2/s
```

### UIs

| Serviço | URL | Credenciais |
|---|---|---|
| Grafana | http://localhost:3001 | anônimo (admin) |
| Kibana | http://localhost:5601 | elastic / AxaUcAn4mLfLKRABJbH- |
| Prometheus | http://localhost:9090 | — |
| Elasticsearch | http://localhost:9200 | elastic / AxaUcAn4mLfLKRABJbH- |
| APM Server | http://localhost:8200 | — |

---

## Conceitos demonstrados

| Conceito | Onde |
|---|---|
| Trace distribuído Kafka | `internal/kafka/producer.go` + `consumer.go` |
| Span customizado por camada | `internal/order/usecase.go`, `internal/payment/usecase.go` |
| Baggage OTEL (customer_id) | `internal/order/controller.go` → consumer → payment |
| Span de erro (~20%) | `internal/order/usecase.go` |
| Middleware HTTP automático | `otelfiber.Middleware()` em ambas as APIs |
| Métricas de negócio | `internal/telemetry/metrics.go` |
| Logs estruturados + OTLP | `internal/telemetry/logger.go` (zap + otelzap bridge) |
| Fan-out de exportadores | `monitoring/otel-collector-config.elastic.yaml` |
| Consumer group + commit manual | `internal/kafka/consumer.go` |
| Graceful shutdown | `cmd/*/main.go` |
