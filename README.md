<div align="center">
<h1>logger</h1>
<p>Structured Go logger built on <code>log/slog</code> with colored console output, component-aware prefixes, and rotating JSON log files.</p>

<p>
    <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://goreportcard.com/badge/github.com/ra1phdd/logger" alt="Go report">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
</p>

**English** | [Russian](README.ru.md)
</div>

## Features

- Small logger API on top of `slog`.
- Colored console logs with component and source location prefixes.
- JSON logs to a rotating file or any custom `io.Writer`.
- Built-in levels: `trace`, `debug`, `info`, `warn`, `error`, `fatal`.
- Component scoping with `Named(...)` and `WithComponent(...)`.
- Context helpers for request-scoped logging.
- Package-level default logger for quick integration.
- Helpers for common `slog.Attr` values and errors.

## Install

```bash
go get github.com/ra1phdd/logger
```

## Quick Start

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

## Output

- Console output is human-readable and colorized through `tint`.
- Each console message gets a prefix like `<component> <file:line> >`.
- If no component is set explicitly, it is derived from the caller path.
- JSON output includes normalized `level`, `source`, and `component` fields.

By default:

- console logs go to `os.Stdout`
- JSON logs go to `logs/<timestamp>.log`
- file output is disabled automatically under `go test`

## File Rotation

Managed log files use `lumberjack` with these defaults:

- max size: `64` MB
- max backups: `32`
- max age: `30` days
- compression: enabled
- local time: enabled

## Levels

Supported level strings:

- `trace`
- `debug`
- `info`
- `warn` or `warning`
- `error`
- `fatal`

You can change the level at construction time or later with:

- `WithLevel(level)`
- `WithLevelString("debug")`
- `(*Logger).SetLevel(level)`
- `(*Logger).SetLogLevel("debug")`
- `logger.SetLogLevel("debug")`

`Fatal` writes the record and then calls the configured exit function. The default is `os.Exit(1)`.

## Options

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

## Default Logger

Use `Init(...)` to configure the package-level logger:

```go
logger.Init(
	logger.WithLevelString("info"),
	logger.WithComponent("app"),
)

logger.Info("ready")
logger.Warn("slow request", logger.Int("ms", 250))
```

Package-level `Trace`, `Debug`, `Info`, `Warn`, `Error`, and `Fatal` functions all use the default logger.

## Context Helpers

- `NewContext(ctx, l)` stores a logger in a context.
- `FromContext(ctx)` returns the stored logger or the default logger.
- `TraceContext`, `DebugContext`, `InfoContext`, `WarnContext`, `ErrorContext`, and `FatalContext` use the logger from the context.

## Attr Helpers

The package re-exports common `slog` helpers:

- `Any`
- `String`
- `Int`
- `Int64`
- `Float64`
- `Bool`
- `Duration`
- `Time`

Use `Err(err)` to attach an error as `error=<message>`.
