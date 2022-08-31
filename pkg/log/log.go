package log

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

const (
	kibibyte = 1024
	mebibyte = kibibyte * 1024
)

func Init(config Config) (*zap.Logger, error) {
	maxSizeBytes, err := units.FromHumanSize(config.MaxSize)
	if err != nil {
		return nil, err
	}

	level := zapcore.InfoLevel
	err = level.UnmarshalText([]byte(config.Level))
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level: %w", err)
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

	lj := &lumberjack.Logger{
		Filename:   config.Filename,
		MaxSize:    int(maxSizeBytes / mebibyte),
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAgeDays,
	}
	sighupChan := make(chan os.Signal, 1)
	signal.Notify(sighupChan, syscall.SIGHUP)
	go func() {
		for {
			<-sighupChan
			_ = lj.Rotate()
		}
	}()

	writer := zapcore.AddSync(lj)

	core := zapcore.NewCore(encoder, writer, level)

	logger := zap.New(core, zap.AddStacktrace(zap.ErrorLevel))
	zap.ReplaceGlobals(logger)

	return logger, nil
}

func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}
