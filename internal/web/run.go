package web

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/platform"
	"github.com/bigredeye/notmanytask/internal/scorer"
	"github.com/bigredeye/notmanytask/internal/tgbot"
	zlog "github.com/bigredeye/notmanytask/pkg/log"
	"github.com/pkg/errors"
)

func Run() error {
	flag.Parse()
	config, err := config.ParseConfig()
	if err != nil {
		return err
	}
	log.Printf("Parsed config: %+v", config)

	logger, err := zlog.Init(config.Log)
	if err != nil {
		return errors.Wrap(err, "Failed to init logger")
	}
	defer func() {
		err = zlog.Sync()
	}()

	wg := sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.OpenDataBase(logger.Named("database"), fmt.Sprintf("postgresql://%s:%s@%s:%d/%s",
		config.DataBase.User,
		config.DataBase.Pass,
		config.DataBase.Host,
		config.DataBase.Port,
		config.DataBase.Name,
	))

	db_proxy := database.DataBaseProxy{
		DataBase: db,
		Conf:     config,
	}
	if err != nil {
		return errors.Wrap(err, "Failed to open database")
	}

	bot, err := tgbot.NewBot(config, logger.Named("tgbot"), db)
	if err != nil {
		return errors.Wrap(err, "failed to create telegram bot")
	}

	deadlines, err := deadlines.NewFetcher(config, logger.Named("deadlines.fetcher"))
	if err != nil {
		return errors.Wrap(err, "Failed to create deadlines fetcher")
	}

	git, err := platform.NewClient(config, logger.Named(config.Platform.Mode))
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to create %s client", config.Platform.Mode))
	}

	projects, err := platform.NewProjectsMaker(config, git, &db_proxy)
	if err != nil {
		return errors.Wrap(err, "Failed to create projects maker")
	}

	pipelines, err := platform.NewPipelinesFetcher(config, git, db)
	if err != nil {
		return errors.Wrap(err, "Failed to create pipeline fetcher")
	}

	scorer := scorer.NewScorer(config, &db_proxy, deadlines, git)

	wg.Add(5)
	go func() {
		defer wg.Done()
		deadlines.Run(ctx)
	}()
	go func() {
		defer wg.Done()
		projects.Run(ctx)
	}()
	go func() {
		defer wg.Done()
		pipelines.Run(ctx)
	}()
	go func() {
		defer wg.Done()
		pipelines.RunFresh(ctx)
	}()
	go func() {
		defer wg.Done()
		bot.Run(ctx)
	}()

	s := newServer(config, logger.Named("server"), &db_proxy, deadlines, projects, pipelines, scorer, git)

	return errors.Wrap(s.run(), "Server failed")
}
