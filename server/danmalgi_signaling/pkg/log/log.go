package log

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/sytallax/prettylog"
)

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (m *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range m.handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var handlers []slog.Handler
	for _, handler := range m.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	var handlers []slog.Handler
	for _, handler := range m.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return &multiHandler{handlers: handlers}
}

type levelRangeHandler struct {
	minLevel slog.Level
	maxLevel slog.Level
	handler  slog.Handler
}

func (h *levelRangeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.minLevel && level <= h.maxLevel
}
func (h *levelRangeHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.handler.Handle(ctx, record)
}
func (h *levelRangeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelRangeHandler{minLevel: h.minLevel, maxLevel: h.maxLevel, handler: h.handler.WithAttrs(attrs)}
}
func (h *levelRangeHandler) WithGroup(name string) slog.Handler {
	return &levelRangeHandler{minLevel: h.minLevel, maxLevel: h.maxLevel, handler: h.handler.WithGroup(name)}
}

func NewLog(basePath string) *slog.Logger {
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		os.MkdirAll(basePath, 0755)
	}

	infoWriter := newDaliyFileWriter(basePath, "_info")
	errorWriter := newDaliyFileWriter(basePath, "_error")

	var handlers []slog.Handler
	handlers = append(handlers, &levelRangeHandler{
		minLevel: slog.LevelInfo,
		maxLevel: slog.LevelWarn,
		handler:  prettylog.New(nil, prettylog.WithDestinationWriter(io.MultiWriter(infoWriter, os.Stdout))),
	})
	handlers = append(handlers, &levelRangeHandler{
		minLevel: slog.LevelError,
		maxLevel: 12, // Emergency level
		handler:  prettylog.New(nil, prettylog.WithDestinationWriter(io.MultiWriter(errorWriter, os.Stdout))),
	})

	return slog.New(&multiHandler{handlers: handlers})
}
