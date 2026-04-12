# Go Service Template

Шаблон для создания микросервисов на Go. Включает HTTP-сервер, observability (логи, метрики, трейсы), клиенты для PostgreSQL, Redis и Kafka, middleware-стек и инфраструктуру для тестирования.

## Структура проекта

```
cmd/service/main.go          # Точка входа. Связывает все компоненты через DI-фреймворк uber/fx
internal/                    # Пакеты, доступные только внутри модуля (Go convention)
  config/                    # Загрузка конфигурации: YAML-файл + переменные окружения
  server/                    # HTTP-сервер на fasthttp, регистрация маршрутов
  handler/                   # Обработчики запросов (бизнес-логика)
  middleware/                # Цепочка middleware: Recoverer → RequestID → Tracing → Metrics → Logger
  apperror/                  # Типизированные ошибки с маппингом на HTTP-статусы
  validator/                 # Декодирование и валидация JSON-тела запроса
  logger/                    # Структурированный JSON-логгер (slog)
  tracing/                   # OpenTelemetry: экспорт трейсов через OTLP/gRPC
  postgres/                  # Пул соединений pgxpool + миграции (golang-migrate)
  redis/                     # Клиент go-redis с OTel-инструментацией
  kafka/                     # Producer и Consumer с трейсингом + Event envelope
configs/                     # YAML-файлы конфигурации и настройки observability-стека
migrations/                  # SQL-миграции (golang-migrate)
docs/                        # Сгенерированная OpenAPI/Swagger спецификация (swaggo/swag)
Dockerfile                   # Multi-stage сборка, non-root пользователь
docker-compose.yml           # Сервис + PostgreSQL, Redis, Kafka, Jaeger, Prometheus, Grafana, Loki
Makefile                     # Команды для сборки, запуска, тестов, Docker, миграций, Swagger
```

> **Для тех, кто не работал с Go:** директория `internal/` -- это языковая конвенция. Go-компилятор запрещает импорт пакетов из `internal/` другим модулям. Это гарантирует инкапсуляцию.

## Компоненты

### config

**Проблема:** Сервису нужна конфигурация, которая работает одинаково локально (YAML-файл) и в Kubernetes (переменные окружения).

**Решение:** [Viper](https://github.com/spf13/viper) загружает YAML-файл из `configs/`, затем перезаписывает значения переменными окружения. Структура `Config` содержит все параметры сервиса, включая секции `Postgres`, `Redis`, `Kafka`.

**Как расширить:** Добавить новое поле в структуру `Config`, задать default через `v.SetDefault()`, добавить в `config.local.yaml`.

**Как изменить:** Любой ключ конфигурации можно переопределить через переменную окружения. Вложенные ключи разделяются `_`: ключ `postgres.max_conns` -> переменная `POSTGRES_MAX_CONNS`.

### logger

**Проблема:** Логи должны быть структурированными (JSON) для парсинга в Loki/ELK и содержать trace_id для корреляции с трейсами.

**Решение:** Стандартный `log/slog` с JSON-хендлером. Уровень логирования настраивается через конфиг (`debug`, `info`, `warn`, `error`).

**Как изменить:** Уровень логирования -- ключ `log_level` в конфиге или переменная `LOG_LEVEL`.

### tracing

**Проблема:** Нужно отслеживать путь запроса через сервис и между сервисами для диагностики latency и ошибок.

**Решение:** OpenTelemetry с экспортом трейсов по OTLP/gRPC в Jaeger. Пропагация контекста через W3C TraceContext + Baggage. Трейсинг можно отключить через конфиг (`tracing_enabled: false`).

**Как работает trace_id:** Middleware `Tracing` создаёт span для каждого HTTP-запроса. `trace_id` из этого span-а добавляется в каждую строку лога middleware `Logger`. В Grafana можно перейти из лога прямо в трейс.

### middleware

**Проблема:** Каждый запрос должен проходить через одинаковую цепочку обработки: recovery от паник, присвоение request ID, трейсинг, сбор метрик, логирование.

**Решение:** Пять middleware, соединённых через `Chain()`:

| Порядок | Middleware | Что делает |
|---------|-----------|------------|
| 1 (внешний) | `Recoverer` | Ловит паники, логирует стек, возвращает 500 |
| 2 | `RequestID` | Генерирует или берёт из заголовка `X-Request-Id` |
| 3 | `Tracing` | Создаёт OTel span, извлекает входящий trace context |
| 4 | `Metrics` | Считает `http_requests_total` и `http_request_duration_seconds` |
| 5 (внутренний) | `Logger` | Логирует завершённый запрос с duration, status, trace_id |

> **Порядок важен:** `Recoverer` должен быть самым внешним, чтобы поймать панику в любом другом middleware.

**Metrics и кардинальность:** Middleware `Metrics` использует не сырой путь запроса (`/api/v1/playlists/123`), а шаблон маршрута (`/api/v1/playlists/:id`). Это предотвращает взрыв кардинальности меток в Prometheus.

### apperror

**Проблема:** Нужен единый формат ошибок для API и маппинг ошибок бизнес-логики на HTTP-статусы.

**Решение:** Типизированные ошибки с кодом, сообщением и HTTP-статусом:

```go
apperror.NewNotFound("плейлист не найден")     // 404
apperror.NewValidation("невалидный ID трека")   // 400
apperror.NewConflict("трек уже в плейлисте")    // 409
apperror.NewUnauthorized("токен истёк")          // 401
apperror.NewInternal("ошибка БД", err)           // 500
```

Ответ клиенту всегда в формате: `{"error": {"code": "NOT_FOUND", "message": "плейлист не найден"}}`.

**Как добавить новый тип ошибки:** Создать функцию-конструктор в `apperror.go` по аналогии с существующими.

### validator

**Проблема:** Каждый endpoint, принимающий JSON, должен декодировать тело и валидировать поля. Нельзя дублировать эту логику в каждом handler-е.

**Решение:** Функция `BindJSON(ctx, &dst)` в одном вызове декодирует JSON и валидирует структуру. При ошибке возвращает `apperror.Validation` с описанием проблемных полей.

```go
type CreatePlaylistRequest struct {
    Name string `json:"name" validate:"required,min=1,max=200"`
}

func (h *Handler) CreatePlaylist(ctx *fasthttp.RequestCtx) {
    var req CreatePlaylistRequest
    if err := validator.BindJSON(ctx, &req); err != nil {
        handler.WriteError(ctx, err)
        return
    }
    // req.Name гарантированно валиден
}
```

**Как добавить кастомную валидацию:** Использовать теги из [go-playground/validator](https://pkg.go.dev/github.com/go-playground/validator/v10#section-documentation), например `validate:"required,email"` или `validate:"gte=0,lte=100"`.

### handler

**Проблема:** Нужна стандартная точка входа для бизнес-логики с единообразной записью ответов.

**Решение:** Структура `Handler` с методами для каждого endpoint. Два хелпера для ответов:
- `WriteJSON(ctx, status, data)` -- успешный ответ
- `WriteError(ctx, appErr)` -- ответ с ошибкой в стандартном формате

Шаблон уже содержит `/healthz`, `/readyz` и `/api/v1/example`.

**Как добавить новый endpoint:** см. раздел "Добавление нового endpoint" ниже.

### server

**Проблема:** Нужно зарегистрировать маршруты, применить middleware, запустить сервер и корректно остановить его при shutdown.

**Решение:** Функция `Register()` создаёт fasthttp-сервер и привязывает его start/stop к жизненному циклу `fx`. Каждый маршрут оборачивается в `withRoute()` для передачи шаблона пути в middleware `Metrics`.

**Как добавить маршрут:**
```go
r.POST("/api/v1/playlists", withRoute("/api/v1/playlists", h.CreatePlaylist))
r.GET("/api/v1/playlists/:id", withRoute("/api/v1/playlists/:id", h.GetPlaylist))
```

> **Про fx:** `go.uber.org/fx` -- это DI-фреймворк. В `main.go` мы описываем, какие компоненты создать (`fx.Provide`) и что запустить (`fx.Invoke`). fx сам разрешает зависимости, создаёт объекты в правильном порядке и управляет shutdown.

### postgres

**Проблема:** Нужен пул соединений с PostgreSQL, интегрированный с трейсингом и проверкой здоровья.

**Решение:** `pgxpool` с настройкой через конфиг (DSN, размер пула, таймауты). Трейсинг через `otelpgx` -- каждый SQL-запрос автоматически создаёт span. Health check через `pool.Ping()`.

**Миграции:** Используется `golang-migrate`. SQL-файлы лежат в `migrations/`. Команды:
- `make migrate-create` -- создать новую миграцию
- `make migrate-up` -- применить все pending-миграции
- `make migrate-down` -- откатить последнюю миграцию

### redis

**Проблема:** Нужен Redis-клиент с пулом соединений, трейсингом и метриками.

**Решение:** `go-redis/v9` с инструментацией через `redisotel` (трейсы + метрики Redis-операций). Health check через `client.Ping()`.

**Как изменить:** Адрес, пароль, номер БД и размер пула -- через конфиг или переменные окружения.

### kafka

**Проблема:** Сервисы обмениваются событиями через Kafka. Нужны producer и consumer с трейсингом.

**Producer:** Обёртка над `segmentio/kafka-go`. При публикации автоматически инжектит OTel trace context в заголовки сообщения. Поддерживает `RequireAll` acks для надёжной доставки.

```go
err := producer.Publish(ctx, "playlist.updated", []byte(playlistID), eventBytes)
```

**Consumer:** Читает сообщения в consumer group, извлекает trace context из заголовков. При ошибке обработки -- логирует и коммитит offset (сообщение не блокирует очередь).

```go
consumer.Run(ctx, func(ctx context.Context, msg kafka.Message) error {
    // обработка сообщения
    return nil
})
```

**Event Envelope:** Обёртка для всех доменных событий с метаданными:

```json
{
  "type": "track.created",
  "version": 1,
  "timestamp": "2026-04-11T12:00:00Z",
  "payload": { "id": "...", "title": "...", "artist": "..." }
}
```

Поле `version` позволяет эволюционировать схему событий без breaking changes.

## Быстрый старт

```bash
# Поднять инфраструктуру (PostgreSQL, Redis, Kafka)
make docker-up-infra

# Сгенерировать Swagger-документацию
make swagger

# Запустить сервис локально
make run

# Запустить тесты
make test

# Поднять всё (сервис + инфра + observability)
make docker-up-all

# Открыть Swagger UI: http://localhost:8080/swagger/index.html
# Открыть Grafana:    http://localhost:3000
# Открыть Jaeger:     http://localhost:16686
```

## Добавление нового endpoint

1. **Handler-метод** в `internal/handler/handler.go`:

```go
func (h *Handler) GetPlaylist(ctx *fasthttp.RequestCtx) {
    id := ctx.UserValue("id").(string)
    // ... бизнес-логика ...
    WriteJSON(ctx, fasthttp.StatusOK, playlist)
}
```

2. **Swagger-аннотация** (перед методом handler-а):

```go
// GetPlaylist godoc
// @Summary     Получить плейлист
// @Description Возвращает плейлист по ID.
// @Tags        playlists
// @Produce     json
// @Param       id path string true "ID плейлиста"
// @Success     200 {object} Playlist
// @Failure     404 {object} apperror.Response
// @Router      /api/v1/playlists/{id} [get]
```

После добавления аннотаций выполните `make swagger` для перегенерации документации.

3. **Маршрут** в `internal/server/server.go`:

```go
r.GET("/api/v1/playlists/:id", withRoute("/api/v1/playlists/:id", h.GetPlaylist))
```

4. **Валидация** (если endpoint принимает JSON тело):

```go
var req CreatePlaylistRequest
if err := validator.BindJSON(ctx, &req); err != nil {
    WriteError(ctx, err)
    return
}
```

5. **Тест** в `internal/handler/handler_test.go`:

```go
func TestGetPlaylist(t *testing.T) {
    h := handler.New(slog.Default())
    ctx := &fasthttp.RequestCtx{}
    ctx.SetUserValue("id", "test-id")
    h.GetPlaylist(ctx)
    assert.Equal(t, http.StatusOK, ctx.Response.StatusCode())
}
```

## Создание нового сервиса из шаблона

1. Скопировать директорию `template/` в новую (например, `playlist-service/`)
2. В `go.mod` изменить `module service` на `module playlist-service`
3. Обновить все импорты: `service/internal/...` -> `playlist-service/internal/...`
4. В `config.local.yaml` изменить `service_name`
5. Добавить доменные пакеты (например, `internal/playlist/`, `internal/repository/`)
6. Добавить SQL-миграции в `migrations/`

## Справочник конфигурации

| Ключ YAML | Переменная окружения | По умолчанию | Описание |
|-----------|---------------------|-------------|----------|
| `service_name` | `SERVICE_NAME` | `service` | Имя сервиса (для логов и трейсов) |
| `http_port` | `HTTP_PORT` | `8080` | Порт HTTP-сервера |
| `log_level` | `LOG_LEVEL` | `info` | Уровень логирования: debug, info, warn, error |
| `shutdown_timeout` | `SHUTDOWN_TIMEOUT` | `15s` | Таймаут graceful shutdown |
| `tracing_enabled` | `TRACING_ENABLED` | `true` | Включить/выключить трейсинг |
| `otlp_endpoint` | `OTLP_ENDPOINT` | `localhost:4317` | Адрес OTLP-коллектора |
| `postgres.dsn` | `POSTGRES_DSN` | `postgres://postgres:postgres@localhost:5432/service?sslmode=disable` | DSN PostgreSQL |
| `postgres.max_conns` | `POSTGRES_MAX_CONNS` | `10` | Макс. соединений в пуле |
| `postgres.min_conns` | `POSTGRES_MIN_CONNS` | `2` | Мин. соединений в пуле |
| `redis.addr` | `REDIS_ADDR` | `localhost:6379` | Адрес Redis |
| `redis.password` | `REDIS_PASSWORD` | *(пусто)* | Пароль Redis |
| `redis.db` | `REDIS_DB` | `0` | Номер БД Redis |
| `redis.pool_size` | `REDIS_POOL_SIZE` | `10` | Размер пула Redis |
| `kafka.brokers` | `KAFKA_BROKERS` | `localhost:9092` | Адреса Kafka-брокеров |

## Makefile-цели

| Цель | Описание |
|------|----------|
| `make build` | Собрать бинарник в `./bin/` |
| `make run` | Запустить сервис локально |
| `make test` | Запустить тесты с `-race` |
| `make lint` | Запустить golangci-lint |
| `make swagger` | Сгенерировать/обновить OpenAPI-спецификацию и Swagger UI |
| `make docker-build` | Собрать Docker-образ |
| `make docker-up` | Поднять только сервис |
| `make docker-up-infra` | Поднять PostgreSQL + Redis + Kafka |
| `make docker-up-tracing` | Сервис + Jaeger |
| `make docker-up-metrics` | Сервис + Prometheus + Grafana |
| `make docker-up-logs` | Сервис + Loki + Promtail + Grafana |
| `make docker-up-all` | Всё вместе |
| `make docker-down` | Остановить все контейнеры |
| `make docker-logs` | Логи сервиса |
| `make migrate-up` | Применить миграции |
| `make migrate-down` | Откатить последнюю миграцию |
| `make migrate-create` | Создать новую миграцию |
