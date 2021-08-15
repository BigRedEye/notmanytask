package log

import (
	"fmt"
	"os"

	"go.uber.org/zap"
)

var logger *zap.Logger

func InitProd() *zap.Logger {
	return initLogger(zap.NewProductionConfig())
}

func InitDev() *zap.Logger {
	return initLogger(zap.NewDevelopmentConfig())
}

func initLogger(config zap.Config) *zap.Logger {
	var err error
	logger, err = config.Build(zap.AddStacktrace(zap.WarnLevel))
	if err != nil {
		fmt.Printf("Failed to init zap logger: %v", err)
		os.Exit(1)
	}
	zap.ReplaceGlobals(logger)
	return logger
}

func Sync() {
	logger.Sync()
}
