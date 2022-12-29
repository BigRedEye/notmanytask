package main

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api *tg.BotAPI
	log *zap.Logger
}

func NewBot(token string, log *zap.Logger) (*Bot, error) {
	if token == "" {
		return nil, nil
	}

	bot, err := tg.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{bot, log}, nil
}

type messageBuilder struct {
	bot  *Bot
	chat int64
	mode string
	text strings.Builder
}

func (b *Bot) NewMessage(chat int64) *messageBuilder {
	if b == nil {
		return nil
	}
	return &messageBuilder{b, chat, tg.ModeMarkdownV2, strings.Builder{}}
}

func (b *messageBuilder) Escaped(format string, args ...any) *messageBuilder {
	if b == nil {
		return nil
	}
	return b.Raw(tg.EscapeText(b.mode, fmt.Sprintf(format, args...)))
}

func (b *messageBuilder) Raw(format string, args ...any) *messageBuilder {
	if b == nil {
		return nil
	}
	b.text.WriteString(fmt.Sprintf(format, args...))
	return b
}

func (b *messageBuilder) Send() error {
	if b == nil {
		return nil
	}
	return b.bot.send(b)
}

func (b *Bot) send(m *messageBuilder) error {
	if b == nil {
		return nil
	}

	b.log.Info("Sending telegram message", zap.Int64("chat", m.chat), zap.Stringer("text", &m.text))
	msg := tg.NewMessage(m.chat, m.text.String())
	msg.ParseMode = m.mode
	_, err := b.api.Send(msg)
	if err != nil {
		b.log.Error("Failed to send telegram message", zap.Error(err))
	}
	return err
}
