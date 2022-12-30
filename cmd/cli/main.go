package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.Logger

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func unwrap[T any](value T, err error) T {
	check(err)
	return value
}

var (
	rootCmd = &cobra.Command{
		Use:   "nmt",
		Short: "Notmanytask client",
	}

	dumpCmd = &cobra.Command{
		Use:   "dump",
		Short: "Dump various info",
	}
)

func initLogging() {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.ConsoleSeparator = " "
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.StampMilli)
	log = unwrap(config.Build())
}

func initCommands() {
    dumpCmd.AddCommand(makeDumpStandingsCommand())
    dumpCmd.AddCommand(makeDumpSuccessfulSubmits())
	rootCmd.AddCommand(makeOverrideCommand())
	rootCmd.AddCommand(dumpCmd)
}

func init() {
	initLogging()
	initCommands()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Command failed: %s", err.Error())
		os.Exit(1)
	}
}
