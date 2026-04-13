# Playback Session Service

Сервис управления сессиями воспроизведения — хранит текущее состояние прослушивания пользователя (трек, позиция, статус, устройство) и обеспечивает синхронизацию между устройствами с отставанием не более 10 секунд. Часть backend-системы музыкального стримингового сервиса (аналог Spotify).

## API

| Method | Path | Description | Status codes |
|--------|------|-------------|-------------|
| POST | `/api/v1/sessions` | Начать сессию воспроизведения | 201, 400, 401 |
| GET | `/api/v1/sessions/current` | Получить текущую сессию | 200, 401, 404 |
| PUT | `/api/v1/sessions/heartbeat` | Обновить позицию (каждые ≤5 с) | 204, 400, 401 |
| PUT | `/api/v1/sessions/pause` | Поставить на паузу | 200, 400, 401, 404 |
| PUT | `/api/v1/sessions/resume` | Возобновить воспроизведение | 200, 400, 401, 404 |

Все запросы требуют заголовок `X-User-ID: <user_id>`.

## Быстрый старт

```bash
# Поднять инфраструктуру (PostgreSQL, Redis, Kafka)
make docker-up-infra

# Применить миграции
make migrate-up

# Запустить сервис
make run

# Тесты
make test

# Swagger UI: http://localhost:8080/swagger/index.html
```

## Технологии

| Компонент | Технология |
|-----------|-----------|
| Язык | Go 1.26 |
| HTTP-сервер | fasthttp |
| DI | uber/fx |
| БД | PostgreSQL 17 (pgxpool) |
| Кэш | Redis 7 (go-redis) |
| Очередь | Apache Kafka 3.7 (segmentio/kafka-go) |
| Конфигурация | Viper |
| Трейсинг | OpenTelemetry + Jaeger |
| Метрики | Prometheus + Grafana |
| Логи | slog (JSON) + Loki |
| CI | GitHub Actions |

## Архитектура

```
HTTP Request
    │
    ▼
Handler (X-User-ID auth, JSON validation)
    │
    ▼
Service (бизнес-логика)
    ├── Repository.SetCache()  ──► Redis  (fast path, ≤5 s lag)
    └── Producer.Publish()     ──► Kafka  ──► HeartbeatConsumer ──► PostgreSQL
                                    │ (при недоступности Kafka)
                                    └── Repository.Upsert()    ──► PostgreSQL (fallback)
```

**Cross-device sync:** `GET /current` читает из Redis (актуальная позиция), при промахе — из PostgreSQL. Новое устройство получает позицию с отставанием ≤ 5 секунд.

**Degraded mode:** при недоступном Redis все операции прозрачно продолжают работу через PostgreSQL.

## Kafka-топики

| Топик | Продюсер | Консьюмер |
|-------|---------|----------|
| `playback.heartbeat` | Playback Session Service | Playback Session Service (async DB persist) |

## Справочник конфигурации

| Ключ YAML | Переменная окружения | По умолчанию | Описание |
|-----------|---------------------|-------------|----------|
| `service_name` | `SERVICE_NAME` | `playback-service` | Имя сервиса |
| `http_port` | `HTTP_PORT` | `8080` | Порт HTTP-сервера |
| `log_level` | `LOG_LEVEL` | `info` | Уровень: debug, info, warn, error |
| `shutdown_timeout` | `SHUTDOWN_TIMEOUT` | `15s` | Таймаут graceful shutdown |
| `tracing_enabled` | `TRACING_ENABLED` | `true` | Включить трейсинг |
| `otlp_endpoint` | `OTLP_ENDPOINT` | `localhost:4317` | Адрес OTLP-коллектора (Jaeger) |
| `postgres.dsn` | `POSTGRES_DSN` | `postgres://postgres:postgres@localhost:5432/playback?sslmode=disable` | DSN PostgreSQL |
| `postgres.max_conns` | `POSTGRES_MAX_CONNS` | `10` | Макс. соединений в пуле |
| `postgres.min_conns` | `POSTGRES_MIN_CONNS` | `2` | Мин. соединений в пуле |
| `redis.addr` | `REDIS_ADDR` | `localhost:6379` | Адрес Redis |
| `redis.pool_size` | `REDIS_POOL_SIZE` | `10` | Размер пула Redis |
| `kafka.brokers` | `KAFKA_BROKERS` | `localhost:9092` | Адреса Kafka-брокеров (producer) |
| `kafka_consumer.brokers` | `KAFKA_CONSUMER_BROKERS` | `localhost:9092` | Адреса Kafka-брокеров (consumer) |
| `kafka_consumer.group_id` | `KAFKA_CONSUMER_GROUP_ID` | `playback-session` | Consumer group ID |
| `kafka_consumer.topic` | `KAFKA_CONSUMER_TOPIC` | `playback.heartbeat` | Топик для heartbeat |

## Makefile-цели

| Цель | Описание |
|------|----------|
| `make build` | Собрать бинарник в `./bin/` |
| `make run` | Запустить сервис локально |
| `make test` | Запустить тесты |
| `make lint` | Запустить golangci-lint |
| `make swagger` | Сгенерировать/обновить OpenAPI-спецификацию |
| `make docker-build` | Собрать Docker-образ |
| `make docker-up-infra` | Поднять PostgreSQL + Redis + Kafka |
| `make docker-up-tracing` | Инфра + Jaeger |
| `make docker-up-metrics` | Инфра + Prometheus + Grafana |
| `make docker-up-logs` | Инфра + Loki + Promtail + Grafana |
| `make docker-up-all` | Всё вместе |
| `make docker-down` | Остановить все контейнеры |
| `make migrate-up` | Применить миграции |
| `make migrate-down` | Откатить последнюю миграцию |
| `make migrate-create` | Создать новую миграцию |
