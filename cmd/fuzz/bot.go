package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api *tgbotapi.BotAPI
}

func NewBot(token string) (*Bot, error) {
	if token == "" {
		return nil, nil
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{bot}, nil
}

func (b *Bot) Notify(chat int64, text string) error {
	if b == nil {
		return nil
	}

	msg := tgbotapi.NewMessage(chat, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	_, err := b.api.Send(msg)
	return err
}
