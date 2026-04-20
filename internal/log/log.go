package log

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
)

// LogLevel represents the severity of a log message
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

var (
	currentLevel = InfoLevel
	isTerminal   bool
	mu           sync.Mutex

	// 简化：单一正则匹配常见敏感词
	sensitiveRegex = regexp.MustCompile(`(?i)(?:token|api[_-]?key|secret|access[_-]?key|password)\s*[:=]\s*['"]?([a-zA-Z0-9_-]{16,})['"]?`)
)

// 颜色映射
var colors = map[LogLevel]string{
	DebugLevel:   "\033[90m",
	InfoLevel:    "\033[36m",
	WarningLevel: "\033[33m",
	ErrorLevel:   "\033[31m",
	SuccessLevel: "\033[32m",
	FatalLevel:   "\033[31m",
}

func Init(logOutput string) error {
	mu.Lock()
	defer mu.Unlock()

	if logOutput != "" && logOutput != "shell" {
		file, err := os.OpenFile(logOutput, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		log.SetOutput(file)
		isTerminal = false
	} else {
		log.SetOutput(os.Stdout)
		isTerminal = checkTerminal()
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return nil
}

func logf(level LogLevel, format string, args ...interface{}) {
	if level < currentLevel {
		return
	}

	msg := sanitizeMessage(fmt.Sprintf(format, args...))
	color := colors[level]
	name := level.String()

	if isTerminal {
		log.Printf("%s[%s]%s %s", color, name, "\033[0m", msg)
	} else {
		log.Printf("[%s] %s", name, msg)
	}

	if level == FatalLevel {
		os.Exit(1)
	}
}

func Debug(format string, args ...interface{})  { logf(DebugLevel, format, args...) }
func Info(format string, args ...interface{})   { logf(InfoLevel, format, args...) }
func Warning(format string, args ...interface{}) { logf(WarningLevel, format, args...) }
func Error(format string, args ...interface{})  { logf(ErrorLevel, format, args...) }
func Success(format string, args ...interface{}) { logf(SuccessLevel, format, args...) }
func Fatal(format string, args ...interface{})  { logf(FatalLevel, format, args...) }

func sanitizeMessage(msg string) string {
	return sensitiveRegex.ReplaceAllString(msg, "$1=***REDACTED***")
}

func checkTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
}

func GetLevel() LogLevel {
	mu.Lock()
	defer mu.Unlock()
	return currentLevel
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
