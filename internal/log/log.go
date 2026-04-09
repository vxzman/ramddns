package log

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	// DebugLevel logs detailed debugging information
	DebugLevel LogLevel = iota
	// InfoLevel logs general operational information
	InfoLevel
	// WarningLevel logs potentially harmful situations
	WarningLevel
	// ErrorLevel logs error conditions
	ErrorLevel
	// FatalLevel logs critical errors that cause termination
	FatalLevel
	// SuccessLevel logs successful operations (ipflow specific)
	SuccessLevel
)

// String returns the string representation of the log level
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
	// currentLogLevel is the minimum level to log
	currentLogLevel = InfoLevel
	isLogTerminal   bool
	colorReset      = "\033[0m"
	colorRed        = "\033[31m"
	colorBlue       = "\033[34m"
	colorCyan       = "\033[36m"
	colorGreen      = "\033[32m"
	colorYellow     = "\033[33m"
	colorMagenta    = "\033[35m"
	colorGray       = "\033[90m"

	// mu protects concurrent access to logger settings
	mu sync.RWMutex
)

// 敏感信息脱敏正则
var (
	// 匹配 API Token (至少 20 位字母数字)
	tokenRegex = regexp.MustCompile(`(?i)(?:token|api[_-]?key|secret)[\s:=]+['"]?([a-zA-Z0-9_-]{20,})['"]?`)
	// 匹配 AccessKey (如阿里云 LTAI 开头)
	accessKeyRegex = regexp.MustCompile(`(?i)(?:access[_-]?key[_-]?id)[\s:=]+['"]?([a-zA-Z0-9]{12,})['"]?`)
)

// Init initializes the logger with the given output path
func Init(logOutput string) error {
	mu.Lock()
	defer mu.Unlock()

	// 根据 logOutput 参数决定日志输出方式
	if logOutput != "" && logOutput != "shell" {
		// 尝试打开日志文件
		file, err := os.OpenFile(logOutput, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		log.SetOutput(file)
		isLogTerminal = false
	} else {
		// 输出到终端
		log.SetOutput(os.Stdout)
		isLogTerminal = isStdoutTerminal()
	}

	// 设置日志格式
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return nil
}

// SetLevel sets the minimum log level to display
func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLogLevel = level
}

// GetLevel returns the current log level
func GetLevel() LogLevel {
	mu.RLock()
	defer mu.RUnlock()
	return currentLogLevel
}

// SetupDefaultLogger sets up the default logger configuration
func SetupDefaultLogger() {
	mu.Lock()
	defer mu.Unlock()

	// 判断标准输出是否为终端
	isLogTerminal = isStdoutTerminal()

	// Set log format with timestamp but no extra prefixes
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

// sanitizeMessage 脱敏敏感信息
func sanitizeMessage(msg string) string {
	// 脱敏 API Token
	msg = tokenRegex.ReplaceAllStringFunc(msg, func(s string) string {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			parts = strings.SplitN(s, ":", 2)
		}
		if len(parts) == 2 {
			return parts[0] + "=***REDACTED***"
		}
		return "***REDACTED***"
	})

	// 脱敏 AccessKey
	msg = accessKeyRegex.ReplaceAllStringFunc(msg, func(s string) string {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			parts = strings.SplitN(s, ":", 2)
		}
		if len(parts) == 2 {
			return parts[0] + "=***REDACTED***"
		}
		return "***REDACTED***"
	})

	return msg
}

func isTerminal() bool {
	// 只在类 Unix 下简单判断
	return runtime.GOOS != "windows" && os.Getenv("TERM") != "" && os.Getenv("TERM") != "dumb"
}

func isStdoutTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func colorWrap(s, color string) string {
	if isLogTerminal {
		return color + s + colorReset
	}
	return s
}

// shouldLog checks if a message with the given level should be logged
func shouldLog(level LogLevel) bool {
	mu.RLock()
	defer mu.RUnlock()
	return level >= currentLogLevel
}

// Debug logs debug-level messages (only shown when log level is DEBUG)
func Debug(format string, args ...interface{}) {
	if !shouldLog(DebugLevel) {
		return
	}
	msg := sanitizeMessage(fmt.Sprintf(format, args...))
	log.Printf("%s %s", colorWrap("[DEBUG]", colorGray), msg)
}

// Info logs informational messages
func Info(format string, args ...interface{}) {
	if !shouldLog(InfoLevel) {
		return
	}
	msg := sanitizeMessage(fmt.Sprintf(format, args...))
	log.Printf("%s %s", colorWrap("[INFO]", colorCyan), msg)
}

// Warning logs warning messages
func Warning(format string, args ...interface{}) {
	if !shouldLog(WarningLevel) {
		return
	}
	msg := sanitizeMessage(fmt.Sprintf(format, args...))
	log.Printf("%s %s", colorWrap("[WARNING]", colorYellow), msg)
}

// Error logs error messages
func Error(format string, args ...interface{}) {
	if !shouldLog(ErrorLevel) {
		return
	}
	msg := sanitizeMessage(fmt.Sprintf(format, args...))
	log.Printf("%s %s", colorWrap("[ERROR]", colorRed), msg)
}

// Success logs success messages
func Success(format string, args ...interface{}) {
	if !shouldLog(SuccessLevel) {
		return
	}
	msg := sanitizeMessage(fmt.Sprintf(format, args...))
	log.Printf("%s %s", colorWrap("[SUCCESS]", colorGreen), msg)
}

// Fatal logs fatal messages and exits
func Fatal(format string, args ...interface{}) {
	msg := sanitizeMessage(fmt.Sprintf(format, args...))
	log.Printf("%s %s", colorWrap("[FATAL]", colorRed), msg)
	os.Exit(1)
}

// ParseLogLevel parses a string into a LogLevel
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
