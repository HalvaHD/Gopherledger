# Gopherledger

**Система лояльности для интернет-магазина** — бэкенд-сервис на Go, позволяющий пользователям регистрироваться, загружать номера заказов, автоматически получать бонусные баллы и тратить их при оформлении новых покупок.

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker)](https://www.docker.com/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

---

## 📖 О проекте

Gopherledger реализует полную бизнес-логику программы лояльности:

- 👤 **Регистрация и аутентификация** пользователей через токены
- 📦 **Загрузка номеров заказов** с валидацией по [алгоритму Луна](https://ru.wikipedia.org/wiki/Алгоритм_Луна)
- ⚙️ **Фоновый воркер** для асинхронной обработки заказов и начисления баллов
- 💰 **Управление балансом**: начисления за заказы и списания при оплате
- 📊 **Экспорт статистики** в файл для аналитики

Проект демонстрирует принципы **чистой архитектуры**, работу с **конкурентностью** (goroutines, mutexes, errgroup), **graceful shutdown** и **multi-stage Docker build**.

---

## ✨ Ключевые возможности

| Возможность | Реализация |
|-------------|------------|
| 🔐 Авторизация | Токены на основе `crypto/rand` с потокобезопасным хранилищем |
| 🛡 Валидация | Алгоритм Луна + кастомные sentinel-ошибки |
| ⚡ Конкурентность | `sync.RWMutex` для защиты in-memory хранилища |
| 🔄 Воркер | Асинхронная обработка с `errgroup` и ограничением concurrency |
| 📝 Логирование | Middleware с настраиваемым уровнем (`debug` / `info`) |
| 🚨 Recovery | Middleware для перехвата паник с логированием stacktrace |
| 🛑 Graceful shutdown | Корректное завершение по `SIGINT` / `SIGTERM` |

---

## 🛠 Технологический стек

| Компонент | Технология | Обоснование выбора |
|-----------|-----------|-------------------|
| Язык | **Go 1.25** | Высокая производительность, встроенная конкурентность |
| HTTP-сервер | **net/http** | Стандартная библиотека, минимум зависимостей |
| Роутинг | **http.ServeMux** | Встроен в stdlib с Go 1.22+, поддерживает path parameters |
| Сериализация | **encoding/json** | Стандартный JSON-пакет |
| Хеширование | **crypto/sha256** | Безопасное хранение паролей |
| Токены | **crypto/rand** | Криптографически стойкая генерация |
| Конфигурация | **gopkg.in/yaml.v3** | Удобная работа с YAML (единственная внешняя зависимость) |
| Контейнеризация | **Docker** (multi-stage) | Минимальный финальный образ (~15 MB) на Alpine |

> 💡 Проект придерживается философии **"stdlib first"** — максимальное использование стандартной библиотеки.

---

## 🏗 Архитектура

Проект следует принципам **чистой архитектуры** с чётким разделением на слои:

```
┌─────────────────────────────────────────────────────────────┐
│                        cmd/server                           │  ← Точка входа, graceful shutdown
├─────────────────────────────────────────────────────────────┤
│                      internal/router                        │  ← Маршрутизация
├─────────────────────────────────────────────────────────────┤
│  internal/handler    │    internal/middleware               │  ← HTTP-слой + Auth/Logging/Recover
├─────────────────────────────────────────────────────────────┤
│                      internal/service                       │  ← Бизнес-логика + Воркер
├─────────────────────────────────────────────────────────────┤
│  internal/store      │  internal/auth   │  internal/config  │  ← Инфраструктура
├─────────────────────────────────────────────────────────────┤
│                       pkg/domain                            │  ← Общие модели и ошибки
└─────────────────────────────────────────────────────────────┘
```

### Жизненный цикл заказа

```
   ┌────────────────────────────────────────────┐
   │                                            │
   ▼                                            │
  NEW ──▶ PROCESSING ──▶ PROCESSED  (баллы начислены)
              │
              └─────────▶ INVALID    (10% заказов)
```

Фоновый воркер запускается при старте сервера и каждые N секунд обрабатывает заказы в статусах `NEW` и `PROCESSING` с ограничением concurrency через `errgroup`.

---

## 🚀 Быстрый старт

### Требования

- Go 1.25+
- Docker (опционально)

### Локальный запуск

```bash
# 1. Клонировать репозиторий
git clone https://github.com/yourusername/gopherledger.git
cd gopherledger

# 2. Установить зависимости
go mod download

# 3. Запустить сервер
go run ./cmd/server/
```

Сервер стартует на `http://localhost:8080`.

### 🐳 Запуск через Docker

```bash
# Собрать образ
docker build -t gopherledger .

# Запустить контейнер
docker run -d \
  --name gopherledger \
  -p 8080:8080 \
  gopherledger

# Посмотреть логи
docker logs -f gopherledger

# Остановить
docker stop gopherledger
```

---

## ⚙️ Конфигурация

Параметры задаются в `config.yaml`:

```yaml
server_host: "0.0.0.0"          # Слушать все интерфейсы
server_port: 8080
log_level: "info"               # info | debug
accrual_interval_seconds: 3     # Интервал работы воркера
worker_concurrency: 5           # Максимум параллельных обработок
```

Если файл отсутствует — используются значения по умолчанию выше.

---

## 📡 API

Базовый URL: `http://localhost:8080`

### Публичные endpoints

| Метод | Путь | Описание |
|-------|------|----------|
| `POST` | `/api/user/register` | Регистрация нового пользователя |
| `POST` | `/api/user/login` | Аутентификация |

### Защищённые endpoints

Все endpoints ниже требуют заголовок `Authorization: <token>`, полученный при регистрации или входе.

| Метод | Путь | Описание |
|-------|------|----------|
| `POST` | `/api/user/orders` | Загрузить номер заказа |
| `GET` | `/api/user/orders` | Список заказов пользователя |
| `GET` | `/api/user/balance` | Текущий баланс |
| `POST` | `/api/user/balance/withdraw` | Списать баллы |
| `GET` | `/api/user/withdrawals` | История списаний |
| `POST` | `/api/stats/export` | Экспорт статистики в `stats.txt` |

### Примеры запросов

<details>
<summary><b>Регистрация</b></summary>

```bash
curl -i -X POST http://localhost:8080/api/user/register \
  -H "Content-Type: application/json" \
  -d '{"login": "alice", "password": "secret"}'
```

Ответ: `200 OK` с заголовком `Authorization: <token>`
</details>

<details>
<summary><b>Вход</b></summary>

```bash
curl -i -X POST http://localhost:8080/api/user/login \
  -H "Content-Type: application/json" \
  -d '{"login": "alice", "password": "secret"}'
```

Ответ: `200 OK` с заголовком `Authorization: <token>`
</details>

<details>
<summary><b>Загрузка заказа</b></summary>

```bash
curl -i -X POST http://localhost:8080/api/user/orders \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: text/plain" \
  --data-binary "79927398713"
```

Возможные ответы:
- `202` — заказ принят в обработку
- `200` — заказ уже загружен этим пользователем
- `409` — заказ принадлежит другому пользователю
- `422` — номер не прошёл проверку Луна
</details>

<details>
<summary><b>Список заказов</b></summary>

```bash
curl -i http://localhost:8080/api/user/orders \
  -H "Authorization: $TOKEN"
```

Ответ:
```json
[
  {
    "number": "79927398713",
    "status": "PROCESSED",
    "accrual": 150.5,
    "uploaded_at": "2026-06-22T19:57:22Z"
  }
]
```

Поле `accrual` присутствует только у заказов в статусе `PROCESSED`.  
При пустом списке возвращается `204 No Content`.
</details>

<details>
<summary><b>Получение баланса</b></summary>

```bash
curl -i http://localhost:8080/api/user/balance \
  -H "Authorization: $TOKEN"
```

Ответ:
```json
{"current": 300.5, "withdrawn": 100.0}
```
</details>

<details>
<summary><b>Списание баллов</b></summary>

```bash
curl -i -X POST http://localhost:8080/api/user/balance/withdraw \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"order": "79927398713", "sum": 100.0}'
```

Возможные ответы:
- `200` — успешно
- `402` — недостаточно баллов
- `422` — неверный номер заказа
</details>

<details>
<summary><b>История списаний</b></summary>

```bash
curl -i http://localhost:8080/api/user/withdrawals \
  -H "Authorization: $TOKEN"
```

Ответ:
```json
[
  {
    "order": "79927398713",
    "sum": 100.0,
    "processed_at": "2026-06-22T20:00:00Z"
  }
]
```

При пустой истории возвращается `204 No Content`.
</details>

<details>
<summary><b>Экспорт статистики</b></summary>

```bash
curl -i -X POST http://localhost:8080/api/stats/export \
  -H "Authorization: $TOKEN"
```

Создаёт файл `stats.txt` в корне проекта.
</details>

### Формат ошибок

Все ошибочные ответы возвращаются в едином формате:

```json
{
  "code": "INSUFFICIENT_FUNDS",
  "message": "недостаточно баллов"
}
```

---

## 📁 Структура проекта

```
gopherledger/
├── cmd/
│   └── server/              # Точка входа, graceful shutdown
│       └── main.go
├── internal/
│   ├── auth/                # Генерация и валидация токенов
│   ├── config/              # Загрузка YAML-конфигурации
│   ├── handler/             # HTTP-обработчики
│   ├── middleware/          # Auth, Logging, Recover
│   ├── router/              # Сборка маршрутов
│   ├── service/             # Бизнес-логика + фоновый воркер
│   └── store/               # In-memory хранилище
├── pkg/
│   └── domain/              # Общие модели и sentinel-ошибки
├── config.yaml              # Конфигурация приложения
├── Dockerfile               # Multi-stage build
├── go.mod
└── README.md
```

---

## 🧪 Тестирование

```bash
# Запустить все тесты
go test ./...

# С покрытием
go test -cover ./...

go test -race ./...

# Линтинг
go vet ./...
```
---

## 📊 Мониторинг и observability

- **Логирование** с уровнями (`info` / `debug`) и структурированными полями
- **Recovery middleware** — паники логируются с полным stacktrace, клиент получает `500`

---

## 📄 Лицензия

Этот проект распространяется под лицензией **MIT**. См. файл [LICENSE](LICENSE) для подробностей.
