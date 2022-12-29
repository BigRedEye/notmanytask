package main

import (
	"go.uber.org/zap"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api *tgbotapi.BotAPI
	log *zap.Logger
}

func NewBot(token string, log *zap.Logger) (*Bot, error) {
	if token == "" {
		return nil, nil
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{bot, log}, nil
}

func (b *Bot) Notify(chat int64, text string) error {
	if b == nil {
		return nil
	}

	mode := tgbotapi.ModeMarkdownV2
	text = tgbotapi.EscapeText(mode, text)

	b.log.Info("Sending telegram message", zap.Int64("chat", chat), zap.String("text", text))
	msg := tgbotapi.NewMessage(chat, text)
	msg.ParseMode = mode
	_, err := b.api.Send(msg)
	if err != nil {
		b.log.Error("Failed to send telegram message", zap.Error(err))
	}
	return err
}
