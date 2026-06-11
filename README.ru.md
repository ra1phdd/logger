<div align="center">
<h1>logger</h1>
<p>Структурированный Go-логгер на базе <code>log/slog</code> с цветным консольным выводом, префиксами компонентов и ротацией JSON-логов.</p>

<p>
    <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://goreportcard.com/badge/github.com/ra1phdd/logger" alt="Go report">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
</p>

[English](README.md) | **Russian**
</div>

## Возможности

- Небольшой API поверх `slog`.
- Цветные консольные логи с префиксом компонента и `file:line`.
- JSON-логи в ротируемый файл или в любой `io.Writer`.
- Встроенные уровни: `trace`, `debug`, `info`, `warn`, `error`, `fatal`.
- Выделение компонентов через `Named(...)` и `WithComponent(...)`.
- Контекстные хелперы для request-scoped логирования.
- Глобальный логгер пакета для быстрой интеграции.
- Хелперы для `slog.Attr` и ошибок.

## Установка

```bash
go get github.com/ra1phdd/logger
```

## Быстрый старт

```go
package main

import (
	"context"

	"github.com/ra1phdd/logger"
)

func main() {
	appLogger := logger.New(
		logger.WithLevelString("debug"),
		logger.WithComponent("api"),
		logger.WithFile("logs/app.log"),
	)

	appLogger.Info("service started", logger.String("addr", ":8080"))

	requestLogger := appLogger.Named("worker").With(
		logger.String("request_id", "42"),
	)
	ctx := logger.NewContext(context.Background(), requestLogger)

	logger.InfoContext(ctx, "job accepted", logger.Int("attempt", 1))
}
```

## Вывод

- Консольный вывод человекочитаемый и цветной благодаря `tint`.
- Каждое сообщение в консоли получает префикс вида `<component> <file:line> >`.
- Если компонент явно не задан, он вычисляется из пути вызывающего кода.
- В JSON-выводе нормализуются поля `level`, `source` и `component`.

По умолчанию:

- консольные логи пишутся в `os.Stdout`
- JSON-логи пишутся в `logs/<timestamp>.log`
- файловый вывод автоматически отключается под `go test`

## Ротация файлов

Для управляемых лог-файлов используется `lumberjack` со следующими значениями по умолчанию:

- максимальный размер: `64` MB
- количество бэкапов: `32`
- срок хранения: `30` дней
- сжатие: включено
- локальное время: включено

## Уровни

Поддерживаются такие строковые уровни:

- `trace`
- `debug`
- `info`
- `warn` или `warning`
- `error`
- `fatal`

Уровень можно задать при создании логгера или поменять позже через:

- `WithLevel(level)`
- `WithLevelString("debug")`
- `(*Logger).SetLevel(level)`
- `(*Logger).SetLogLevel("debug")`
- `logger.SetLogLevel("debug")`

`Fatal` сначала пишет запись в лог, а затем вызывает настроенную функцию завершения процесса. По умолчанию это `os.Exit(1)`.

## Опции

- `WithLevel(level)`
- `WithLevelString(level)`
- `WithOutput(writer)`
- `WithJSONOutput(writer)`
- `WithFile(path)`
- `WithoutFile()`
- `WithComponent(name)`
- `WithTimeFormat(layout)`
- `WithNoColor(true)`
- `WithExit(func(int))`

## Глобальный логгер

Через `Init(...)` можно настроить логгер уровня пакета:

```go
logger.Init(
	logger.WithLevelString("info"),
	logger.WithComponent("app"),
)

logger.Info("ready")
logger.Warn("slow request", logger.Int("ms", 250))
```

Функции уровня пакета `Trace`, `Debug`, `Info`, `Warn`, `Error` и `Fatal` используют именно этот логгер.

## Работа с context

- `NewContext(ctx, l)` сохраняет логгер в контексте.
- `FromContext(ctx)` возвращает логгер из контекста или глобальный логгер.
- `TraceContext`, `DebugContext`, `InfoContext`, `WarnContext`, `ErrorContext` и `FatalContext` берут логгер из контекста.

## Хелперы атрибутов

Пакет переэкспортирует стандартные хелперы `slog`:

- `Any`
- `String`
- `Int`
- `Int64`
- `Float64`
- `Bool`
- `Duration`
- `Time`

Функция `Err(err)` добавляет ошибку как `error=<message>`.
