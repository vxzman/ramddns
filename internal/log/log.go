package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message.
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarningLevel
	ErrorLevel
	FatalLevel
	SuccessLevel
)

func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarningLevel:
		return "WARNING"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	case SuccessLevel:
		return "SUCCESS"
	default:
		return "UNKNOWN"
	}
}

const (
	levelSuccess slog.Level = 1
	levelFatal   slog.Level = 12
)

var (
	mu       sync.Mutex
	handler  slog.Handler
	levelVar = new(slog.LevelVar)

	// Keep the key name and redact only the secret value.
	sensitiveRegex = regexp.MustCompile(`(?i)(token|api[_-]?key|secret|access[_-]?key|password)(\s*[:=]\s*['"]?)([A-Za-z0-9_-]{16,})`)
)

func init() {
	levelVar.Set(slog.LevelInfo)
	setHandler(os.Stderr)
}

// Init points the process logger at stdout. The probe child must not call it:
// probe diagnostics stay on stderr so stdout remains a single JSON line.
func Init() error {
	mu.Lock()
	defer mu.Unlock()
	setHandler(os.Stdout)
	return nil
}

func setHandler(w io.Writer) {
	base := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level:       levelVar,
		ReplaceAttr: replaceLevel,
	})
	handler = redactHandler{next: base}
	slog.SetDefault(slog.New(handler))
}

func replaceLevel(_ []string, a slog.Attr) slog.Attr {
	if a.Key != slog.LevelKey {
		return a
	}
	level, ok := a.Value.Any().(slog.Level)
	if !ok {
		return a
	}
	switch level {
	case levelSuccess:
		a.Value = slog.StringValue("SUCCESS")
	case levelFatal:
		a.Value = slog.StringValue("FATAL")
	}
	return a
}

func emit(level slog.Level, msg string, args ...any) {
	mu.Lock()
	h := handler
	mu.Unlock()
	if h == nil || !h.Enabled(context.Background(), level) {
		return
	}
	record := slog.NewRecord(time.Now(), level, msg, 0)
	record.Add(args...)
	_ = h.Handle(context.Background(), record)
	if level >= levelFatal {
		os.Exit(1)
	}
}

func Debug(msg string, args ...any)   { emit(slog.LevelDebug, msg, args...) }
func Info(msg string, args ...any)    { emit(slog.LevelInfo, msg, args...) }
func Warning(msg string, args ...any) { emit(slog.LevelWarn, msg, args...) }
func Error(msg string, args ...any)   { emit(slog.LevelError, msg, args...) }
func Success(msg string, args ...any) { emit(levelSuccess, msg, args...) }
func Fatal(msg string, args ...any)   { emit(levelFatal, msg, args...) }

func sanitizeMessage(msg string) string {
	return sensitiveRegex.ReplaceAllString(msg, "${1}${2}***REDACTED***")
}

func sensitiveKey(key string) bool {
	k := strings.ToLower(key)
	k = strings.ReplaceAll(k, "-", "_")
	return strings.Contains(k, "token") ||
		strings.Contains(k, "secret") ||
		strings.Contains(k, "password") ||
		strings.Contains(k, "access_key") ||
		strings.Contains(k, "api_key")
}

func redactAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		for i := range group {
			group[i] = redactAttr(group[i])
		}
		a.Value = slog.GroupValue(group...)
		return a
	}
	if sensitiveKey(a.Key) {
		return slog.String(a.Key, "***REDACTED***")
	}
	switch a.Value.Kind() {
	case slog.KindString:
		a.Value = slog.StringValue(sanitizeMessage(a.Value.String()))
	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok && err != nil {
			a.Value = slog.StringValue(sanitizeMessage(err.Error()))
		}
	}
	return a
}

type redactHandler struct {
	next slog.Handler
}

func (h redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Message = sanitizeMessage(r.Message)
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, redactAttr(a))
		return true
	})
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	out.AddAttrs(attrs...)
	return h.next.Handle(ctx, out)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cleaned := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		cleaned[i] = redactAttr(a)
	}
	return redactHandler{next: h.next.WithAttrs(cleaned)}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{next: h.next.WithGroup(name)}
}

func SetLevel(level LogLevel) {
	levelVar.Set(slogLevel(level))
}

func GetLevel() LogLevel {
	switch levelVar.Level() {
	case slog.LevelDebug:
		return DebugLevel
	case slog.LevelWarn:
		return WarningLevel
	case slog.LevelError:
		return ErrorLevel
	case levelFatal:
		return FatalLevel
	case levelSuccess:
		return SuccessLevel
	default:
		return InfoLevel
	}
}

func slogLevel(level LogLevel) slog.Level {
	switch level {
	case DebugLevel:
		return slog.LevelDebug
	case WarningLevel:
		return slog.LevelWarn
	case ErrorLevel:
		return slog.LevelError
	case FatalLevel:
		return levelFatal
	case SuccessLevel:
		return levelSuccess
	default:
		return slog.LevelInfo
	}
}

func ParseLogLevel(s string) (LogLevel, error) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return DebugLevel, nil
	case "INFO":
		return InfoLevel, nil
	case "WARNING", "WARN":
		return WarningLevel, nil
	case "ERROR":
		return ErrorLevel, nil
	case "FATAL":
		return FatalLevel, nil
	case "SUCCESS":
		return SuccessLevel, nil
	default:
		return InfoLevel, fmt.Errorf("unknown log level: %s", s)
	}
}
