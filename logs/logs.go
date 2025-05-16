package logs

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Global logging state
var (
	logger  *zap.SugaredLogger
	Options = struct {
		Verbose     bool
		AppName     string
		Version     string
		Environment string
	}{
		AppName: "sidehub", // default
	}

	initOnce sync.Once
)

// Logger returns the global zap.SugaredLogger instance.
// If it's nil, InitLogger is called automatically.
func Logger() *zap.SugaredLogger {
	if logger == nil {
		InitLogger(os.Getenv("ENV"))
		if logger == nil {
			// Fallback to a simple logger if InitLogger failed
			panic("Logger not initialized")
		}
	}
	return logger
}

// InitLogger configures the global logger based on environment & LOG_FMT overrides.
// If environment is 'development' or 'dev', default to text console logs unless overridden.
// If environment is 'production' or ”, default to JSON unless overridden.
// LOG_FMT can be 'json', 'formatted', or 'text'.
func InitLogger(env string) {
	initOnce.Do(func() {
		if env == "" || strings.EqualFold(env, "production") {
			env = "production"
		} else if strings.EqualFold(env, "dev") || strings.EqualFold(env, "development") {
			env = "development"
		}
		Options.Environment = env

		format := os.Getenv("LOG_FMT") // user override
		if format == "" {
			if env == "development" {
				format = "text"
			} else {
				format = "json"
			}
		}

		var cfg zap.Config
		if format == "text" {
			cfg = zap.NewDevelopmentConfig()
			cfg.Encoding = "console"
		} else {
			// 'json' or 'formatted' => base is ProductionConfig
			cfg = zap.NewProductionConfig()
			cfg.Encoding = "json"
			if format == "formatted" {
				// Example of a more pretty JSON
				cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
				cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
				cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
			}
		}

		// Common settings
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stderr"}

		if Options.Verbose {
			// Make logs more verbose. For JSON, might do debug-level.
			// For console, we already have stacktraces on error, etc.
			cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		}

		// Add app/version/env fields in each log line
		log, err := cfg.Build(zap.Fields(
			zap.String("app", Options.AppName),
			zap.String("version", Options.Version),
			zap.String("env", Options.Environment),
		))
		if err != nil {
			// Create a fallback development logger to report the error
			fallback := zap.NewExample().Sugar()
			fallback.Errorf("Failed to initialize logger: %v", err)
			logger = fallback // Use the fallback logger going forward
			return
		}

		logger = log.Sugar()
	})
}

// Debug uses fmt.Sprint to construct and log a message.
// Only logs if Options.Verbose is true.
func Debug(args ...any) {
	if args == nil {
		return
	}
	if Options.Verbose {
		Logger().Debug(args...)
	}
}

// Info uses fmt.Sprint to construct and log a message.
func Info(args ...any) {
	Logger().Info(args...)
}

// Warn uses fmt.Sprint to construct and log a message.
func Warn(args ...any) {
	if args == nil {
		return
	}
	Logger().Warn(args...)
}

// Error uses fmt.Sprint to construct and log a message.
func Error(args ...any) {
	if args == nil {
		return
	}
	fmt.Println("------------Error", args)
	Logger().Error(args...)
}

// DPanic uses fmt.Sprint to construct and log a message. In development, the logger then panics.
func DPanic(args ...any) {
	Logger().DPanic(args...)
}

// Panic uses fmt.Sprint to construct and log a message, then panics.
func Panic(args ...any) {
	Logger().Panic(args...)
}

// Fatal uses fmt.Sprint to construct and log a message, then calls os.Exit(1).
func Fatal(args ...any) {
	Logger().Fatal(args...)
}

// Debugf uses fmt.Sprintf to construct and log a message.
// Only logs if Options.Verbose is true.
func Debugf(format string, args ...any) {
	if Options.Verbose {
		Logger().Debugf(format, args...)
	}
}

// Infof uses fmt.Sprintf to construct and log a message.
func Infof(format string, args ...any) {
	Logger().Infof(format, args...)
}

// Warnf uses fmt.Sprintf to construct and log a message.
func Warnf(format string, args ...any) {
	Logger().Warnf(format, args...)
}

// Errorf uses fmt.Sprintf to construct and log a message.
func Errorf(format string, args ...any) {
	Logger().Errorf(format, args...)
}

// DPanicf uses fmt.Sprintf to construct and log a message. In development, the logger then panics.
func DPanicf(format string, args ...any) {
	Logger().DPanicf(format, args...)
}

// Panicf uses fmt.Sprintf to construct and log a message, then panics.
func Panicf(format string, args ...any) {
	Logger().Panicf(format, args...)
}

// Fatalf uses fmt.Sprintf to construct and log a message, then calls os.Exit(1).
func Fatalf(format string, args ...any) {
	Logger().Fatalf(format, args...)
}
