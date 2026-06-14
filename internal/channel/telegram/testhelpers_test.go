// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// stubBotClient is a do-nothing botClient implementation used by
// tests that need an Adapter wired up but don't exercise the Send
// path. Send-style tests construct richer fakes (see send_test.go).
type stubBotClient struct{}

func (stubBotClient) SendMessage(context.Context, *bot.SendMessageParams) (*models.Message, error) {
	return &models.Message{}, nil
}
func (stubBotClient) SendPhoto(context.Context, *bot.SendPhotoParams) (*models.Message, error) {
	return &models.Message{}, nil
}
func (stubBotClient) SendDocument(context.Context, *bot.SendDocumentParams) (*models.Message, error) {
	return &models.Message{}, nil
}
func (stubBotClient) SendVoice(context.Context, *bot.SendVoiceParams) (*models.Message, error) {
	return &models.Message{}, nil
}
func (stubBotClient) SendAudio(context.Context, *bot.SendAudioParams) (*models.Message, error) {
	return &models.Message{}, nil
}
func (stubBotClient) SendVideo(context.Context, *bot.SendVideoParams) (*models.Message, error) {
	return &models.Message{}, nil
}
func (stubBotClient) SendLocation(context.Context, *bot.SendLocationParams) (*models.Message, error) {
	return &models.Message{}, nil
}
func (stubBotClient) AnswerCallbackQuery(context.Context, *bot.AnswerCallbackQueryParams) (bool, error) {
	return true, nil
}
func (stubBotClient) EditMessageText(context.Context, *bot.EditMessageTextParams) (*models.Message, error) {
	return &models.Message{}, nil
}
func (stubBotClient) EditMessageCaption(context.Context, *bot.EditMessageCaptionParams) (*models.Message, error) {
	return &models.Message{}, nil
}
func (stubBotClient) DeleteMessage(context.Context, *bot.DeleteMessageParams) (bool, error) {
	return true, nil
}
func (stubBotClient) SetMessageReaction(context.Context, *bot.SetMessageReactionParams) (bool, error) {
	return true, nil
}
func (stubBotClient) SetWebhook(context.Context, *bot.SetWebhookParams) (bool, error) {
	return true, nil
}
func (stubBotClient) DeleteWebhook(context.Context, *bot.DeleteWebhookParams) (bool, error) {
	return true, nil
}
