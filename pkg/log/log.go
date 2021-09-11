package log

import (
	"fmt"

	"github.com/docker/go-units"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *zap.Logger

type Config struct {
	Level       string
	Filename    string
	MaxSize     string
	MaxBackups  int
	MaxAgeDays  int
	Development bool
}

func Init(config Config) (*zap.Logger, error) {
	maxSize, err := units.FromHumanSize(config.MaxSize)
	if err != nil {
		return nil, err
	}

	level := zapcore.InfoLevel
	err = level.UnmarshalText([]byte(config.Level))
	if err != nil {
		return nil, fmt.Errorf("Failed to parse log level: %w", err)
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    "function",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if config.Development {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	writer := zapcore.AddSync(&lumberjack.Logger{
		Filename:   config.Filename,
		MaxSize:    int(maxSize),
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAgeDays,
	})

	core := zapcore.NewCore(encoder, writer, level)

	logger := zap.New(core, zap.AddStacktrace(zap.ErrorLevel))
	zap.ReplaceGlobals(logger)

	return logger, nil
}

func Sync() error {
	return logger.Sync()
}
