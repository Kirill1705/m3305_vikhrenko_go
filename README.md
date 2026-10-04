# TripGo — лабораторная работа 1

HTTP-сервис поездок. Реализована основная часть и оба бонуса

## Требования и запуск

Нужны Go **1.27.1**, Make, Docker и `tripgoctl`.

Установка утилиты курса:

```sh
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL \
  https://github.com/course-go-autumn-2026/course-infra/releases/latest/download/install-tripgoctl \
  -o /tmp/install-tripgoctl
sh /tmp/install-tripgoctl --install-dir "$HOME/.local/bin" v0.1.2
export PATH="$HOME/.local/bin:$PATH"
tripgoctl doctor
```

В корне клонированного репозитория:

```sh
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect
make migrate
make run
```

запуск `make run`
запуск с переопределением параметров .env `make run HTTP_ADDR=127.0.0.1:8081`.


### Docker

```sh
docker network create --driver bridge \
  --opt com.docker.network.bridge.enable_ip_masquerade=true \
  --opt com.docker.network.driver.mtu=1500 \
  --ipv6 --subnet 172.30.250.0/24 --subnet fc00:f853:ccd:e793::/64 kind
```

## Конфигурация

Все настройки читаются из генерируемого .env

| Переменная | Значение примера | Назначение |
|---|---|---|
| `HTTP_ADDR` | `:8080` | Адрес HTTP-сервера |
| `HTTP_READ_TIMEOUT` | `5s` | Чтение запроса |
| `HTTP_READ_HEADER_TIMEOUT` | `2s` | Чтение заголовков |
| `HTTP_WRITE_TIMEOUT` | `10s` | Запись ответа |
| `HTTP_IDLE_TIMEOUT` | `60s` | Ожидание следующего запроса keep-alive |
| `HTTP_MAX_BODY_BYTES` | `1048576` | Максимальный размер тела запроса |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SHUTDOWN_TIMEOUT` | `10s` | Общий таймаут HTTP + БД |
| `DATABASE_URL` | из `.env` | Строка подключения PostgreSQL |
| `DATABASE_MAX_CONNS` | `10` | Максимальный размер пула |
| `DATABASE_MIN_CONNS` | `2` | Минимальный размер пула, от 0 до максимума |
| `DATABASE_MAX_CONN_LIFETIME` | `30m` | Время жизни соединения |
| `DATABASE_CONNECT_TIMEOUT` | `5s` | Таймаут первоначального соединения |
| `DATABASE_QUERY_TIMEOUT` | `3s` | Таймаут запроса в БД |
| `IDEMPOTENCY_TTL` | `24h` | Срок действия ключа создания поездки |

## API и проверка вручную

Возьмите адрес из `HTTP_ADDR`: для `:8080` это `http://localhost:8080`.
При переопределении порта измените `BASE_URL` соответственно.

```sh
export BASE_URL=http://localhost:8080
curl -i "$BASE_URL/health"
curl -i "$BASE_URL/ready"

TRIP=$(curl -fsS "$BASE_URL/api/v1/trips" \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
       "driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
       "start_point":{"latitude":59.9398,"longitude":30.3146},
       "end_point":{"latitude":59.9290,"longitude":30.3626},
       "price":1450}')
printf '%s\n' "$TRIP" | jq
TRIP_ID=$(printf '%s' "$TRIP" | jq -r .id)
curl -i "$BASE_URL/api/v1/trips/$TRIP_ID"
curl -i -X POST "$BASE_URL/api/v1/trips/$TRIP_ID/finish"
curl -i -X POST "$BASE_URL/api/v1/trips/$TRIP_ID/finish"
```

Создание возвращает `201` и `Location`, получение и первое завершение — `200`,
повторное завершение — `409 trip_completed`. Повторное создание на занятого
водителя возвращает `409 driver_busy`; после завершения водитель свободен.
Невалидный запрос — `400 invalid_request`, отсутствующая поездка —
`404 trip_not_found`, внутренняя ошибка — `500 internal_error`.
Ошибки API имеют формат RFC 9457 (`application/problem+json`) с `type`,
`title`, `status`, `detail`, `instance`, `code`. Внутренние ошибки доступны
только в логах.

`/health` всегда возвращает `200 {"status":"ok"}` без запроса в БД.
`/ready` делает Ping и возвращает `200 {"status":"ok"}` либо
`503 {"status":"unavailable"}`; оба ответа имеют `application/json`.
Это можно проверить, остановив и снова запустив окружение при работающем сервисе.

## Команды разработки

```sh
make generate
make build
make test
go vet ./...
make migrate-status
make migrate-down
make migrate
```

Для полного отката трёх миграций в пустом тестовом окружении вызовите
`make migrate-down` трижды: сначала удалится таблица ключей, затем история и поездки.
Миграции применяются отдельной командой;
запуск сервиса схему не меняет.

Набор unit-тестов и линтер по программе курса добавляются в ЛР2.
В ЛР1 цель `make test` проверяет сборку пакетов с `-race`.
При приёмке также проверяются реальные HTTP-запросы, 20 конкурентных созданий
(один `201`, остальные `409`), конкурентное завершение (один `200`),
атомарность истории, откат при ошибке и панике, вложенные транзакции,
readiness при остановке БД и graceful shutdown с активным запросом.

## Бонус: идемпотентность создания

Передайте UUID в заголовке `Idempotency-Key`. Ключ выбирает клиент:

```sh
curl -i "$BASE_URL/api/v1/trips" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: 7f1c2a54-6d0e-4a3b-9f21-0c8e5b3a77d1' \
  -d '{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
       "driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
       "start_point":{"latitude":59.9398,"longitude":30.3146},
       "end_point":{"latitude":59.9290,"longitude":30.3626},
       "price":1450}'
```

Ключи хранятся в PostgreSQL, в `trip_idempotency_keys` (третья миграция).
Ключ, поездка, история и сохранённый ответ фиксируются одной транзакцией.
Конкурентные запросы синхронизируются уникальным ключом таблицы; несколько
экземпляров сервиса используют одну и ту же защиту. Ошибка создания откатывает
также захват ключа: неуспешный запрос не мешает следующей попытке.

Тело сравнивается по SHA-256 от JSON проверенного DTO. Порядок полей и пробелы
не меняют команду, а изменение пассажира, водителя, координат или цены создаёт
конфликт. Сохраняется снимок успешного ответа создания, поэтому повтор после
завершения поездки возвращает исходный снимок

## Бонус: Docker-образ

`Dockerfile` использует многостадийную сборку: Go 1.27.1 компилирует статический
бинарник с `CGO_ENABLED=0`, затем бинарник и CA-сертификаты копируются в `scratch`.

Измеренный размер итогового образа около 14 Мб.

После `tripgoctl connect` и `make migrate`:

```sh
make docker-build
make docker-run
curl -i "$BASE_URL/ready"
docker stop tripgo-lab1
```

`make docker-run` сначала обновляет образ, затем передаёт настройки из окружения
Make. `.env.example`, `.env` и аргументы `make` имеют тот же приоритет, что при
локальном запуске. Например, для параллельного запуска с локальным сервисом:
`make docker-run HTTP_ADDR=127.0.0.1:8081`.

## Структура

```text
cmd/trip-service/main.go          сборка зависимостей, запуск и остановка
internal/
  config/config.go               чтение и проверка env
  database/
    pool.go                      настройка pgxpool
    db.go                        общий DBTX
    transaction_manager.go       BEGIN / COMMIT / ROLLBACK, транзакция в context
  httpapi/handler.go             HTTP, валидация, JSON
  trip/
    service.go                   создание и завершение поездки
    repository.go                SQL-запросы поездок и истории через Squirrel
    idempotency_repository.go     захват ключа и сохранённый ответ создания
    errors.go                    ошибки операций поездок
api/                             сгенерированные типы и chi-сервер
contracts/                       контракты курса
migrations/                      SQL-миграции goose
Dockerfile                       сборка докер
```

## Решения

- **Простая структура.** `cmd/trip-service` собирает зависимости и управляет
  жизненным циклом; `internal/config` читает env; `internal/database` управляет
  пулом и транзакциями; `internal/httpapi` отвечает за HTTP;
  `internal/trip` содержит операции поездок и их репозиторий.
  Сгенерированные типы используются напрямую. Разделение на доменные модели
  и транспортные DTO предусмотрено рефакторингом следующей лабораторной.
- **Контракт первичен.** `contracts/` — неизменённая копия материалов курса.
  `api/oapi-codegen.yaml` выбирает только пять операций ЛР1. Nullable-типы
  сохраняют явные `null` для `finished_at` и `last_position_at`. Колонка
  `last_position_at` появляется по заданию в ЛР3. Входные запросы проверяются
  по встроенному OpenAPI; декодирование в generated-типы дополнительно
  проверяет точный `int64`, UUID и отсутствие второго JSON-значения.
- **Изоляция `READ COMMITTED`.** Для этих операций достаточно уникального
  индекса и блокировки конкретной поездки. Этот уровень не требует повторять
  транзакции после ошибок сериализации.
- **Менеджер `Do(ctx, fn)`.** Начинает транзакцию с явным уровнем изоляции,
  сохраняет её в контексте. Репозиторий выбирает транзакцию из контекста либо
  пул. Бизнес-код не импортирует `pgx`. Успешный callback ведёт к COMMIT;
  ошибка — к ROLLBACK, затем ошибка выходит наружу.
  Rollback получает отдельный ограниченный контекст без отмены исходного
  запроса. Вложенный `Do` использует ту же транзакцию, без savepoint;
  вызывающий код должен возвращать его ошибку наружу.
- **Атомарность.** Создание поездки и запись `NULL → active` выполняются в
  одном `Do`. Завершение и история `active → completed` также объединены в
  одну транзакцию. Ошибка записи истории откатывает изменение поездки.
- **Один активный заказ.** Частичный уникальный индекс
  `trips_active_driver_uniq` на `driver_id WHERE status = 'active'`
  защищает от конкурентных созданий. `23505` именно этого индекса
  преобразуется в `driver_busy`.
- **Конкурентное завершение.** `SELECT ... FOR UPDATE` держит блокировку до
  фиксации статуса и истории. Второй запрос читает уже завершённую поездку
  и получает `409`; `finished_at` не перезаписывается. Время задаёт сервис
  в UTC с точностью PostgreSQL до микросекунд.
- **Остановка.** SIGINT/SIGTERM прекращает приём новых соединений, HTTP
  дожидается текущих запросов, затем закрывается пул. Контексты запросов
  отменяются после ожидания. Если общий `SHUTDOWN_TIMEOUT` исчерпан,
  соединения закрываются принудительно, причина логируется, процесс
  завершается с кодом 1.
- **Бонусы.** Идемпотентность хранится в PostgreSQL, чтобы переживать перезапуск
  процесса и работать между экземплярами. Scratch выбран потому, что сервис
  собирается статически и не требует shell или динамических системных библиотек.
