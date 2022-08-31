package tgbot

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
)

type Bot struct {
	bot *tgbotapi.BotAPI
	log *zap.Logger
	db  *database.DataBase
}

func NewBot(conf *config.Config, log *zap.Logger, db *database.DataBase) (*Bot, error) {
	bot, err := tgbotapi.NewBotAPI(conf.Telegram.BotToken)
	if err != nil {
		return nil, err
	}
	return &Bot{bot, log, db}, nil
}

func (b *Bot) Run(ctx context.Context) {
	b.log.Info("Authorized on account %s", zap.String("username", b.bot.Self.UserName))

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.bot.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			if err := b.handleUpdate(update); err != nil {
				b.log.Error("Failed to handle update", zap.Error(err), zap.Int("update_id", update.UpdateID))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (b *Bot) handleUpdate(update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}
	b.log.Info("Got message",
		zap.String("user", update.Message.From.UserName),
		zap.String("text", update.Message.Text),
	)

	author := update.Message.ForwardFrom
	if author == nil {
		return nil
	}

	text := ""
	user, err := b.db.FindUserByTelegramID(author.ID)
	if err != nil {
		b.log.Error("Failed to find user by telegram ID", zap.Int64("telegram_id", author.ID), zap.Error(err))
		text = "Failed to find user by telegram ID, try again later"
	} else if user == nil {
		text = "The message is from unknown user"
	} else {
		text = fmt.Sprintf("The message is from %s %s (telegram id: %v)", user.FirstName, user.LastName, *user.TelegramID)
	}

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
	msg.ReplyToMessageID = update.Message.MessageID

	_, err = b.bot.Send(msg)
	return err
}
