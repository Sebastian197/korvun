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
// path. Send-style tests use capturingBotClient instead.
type stubBotClient struct{}

// capturingBotClient records the params it last received per method
// and returns either a non-nil result or, if errOn matches the kind,
// the configured forced error. Used by Send tests to assert dispatch
// + error propagation without an HTTPS call to Telegram.
type capturingBotClient struct {
	lastMessage        *bot.SendMessageParams
	lastPhoto          *bot.SendPhotoParams
	lastDocument       *bot.SendDocumentParams
	lastVoice          *bot.SendVoiceParams
	lastAudio          *bot.SendAudioParams
	lastVideo          *bot.SendVideoParams
	lastLocation       *bot.SendLocationParams
	lastAnswerCallback *bot.AnswerCallbackQueryParams
	lastEditText       *bot.EditMessageTextParams
	lastEditCaption    *bot.EditMessageCaptionParams
	lastDelete         *bot.DeleteMessageParams
	lastSetReaction    *bot.SetMessageReactionParams
	lastSetWebhook     *bot.SetWebhookParams
	lastDeleteWebhook  *bot.DeleteWebhookParams

	// errOn, when non-zero, causes the matching method to return
	// forcedErr instead of a happy-path response. Lets tests check
	// the error-wrap path on Send without staging twelve fakes.
	errOn     OutboundKind
	forcedErr error
}

func (c *capturingBotClient) maybeErr(k OutboundKind) error {
	if c.errOn == k {
		return c.forcedErr
	}
	return nil
}

func (c *capturingBotClient) SendMessage(_ context.Context, p *bot.SendMessageParams) (*models.Message, error) {
	c.lastMessage = p
	return &models.Message{}, c.maybeErr(OutboundKindMessage)
}
func (c *capturingBotClient) SendPhoto(_ context.Context, p *bot.SendPhotoParams) (*models.Message, error) {
	c.lastPhoto = p
	return &models.Message{}, c.maybeErr(OutboundKindPhoto)
}
func (c *capturingBotClient) SendDocument(_ context.Context, p *bot.SendDocumentParams) (*models.Message, error) {
	c.lastDocument = p
	return &models.Message{}, c.maybeErr(OutboundKindDocument)
}
func (c *capturingBotClient) SendVoice(_ context.Context, p *bot.SendVoiceParams) (*models.Message, error) {
	c.lastVoice = p
	return &models.Message{}, c.maybeErr(OutboundKindVoice)
}
func (c *capturingBotClient) SendAudio(_ context.Context, p *bot.SendAudioParams) (*models.Message, error) {
	c.lastAudio = p
	return &models.Message{}, c.maybeErr(OutboundKindAudio)
}
func (c *capturingBotClient) SendVideo(_ context.Context, p *bot.SendVideoParams) (*models.Message, error) {
	c.lastVideo = p
	return &models.Message{}, c.maybeErr(OutboundKindVideo)
}
func (c *capturingBotClient) SendLocation(_ context.Context, p *bot.SendLocationParams) (*models.Message, error) {
	c.lastLocation = p
	return &models.Message{}, c.maybeErr(OutboundKindLocation)
}
func (c *capturingBotClient) AnswerCallbackQuery(_ context.Context, p *bot.AnswerCallbackQueryParams) (bool, error) {
	c.lastAnswerCallback = p
	return true, c.maybeErr(OutboundKindAnswerCallback)
}
func (c *capturingBotClient) EditMessageText(_ context.Context, p *bot.EditMessageTextParams) (*models.Message, error) {
	c.lastEditText = p
	return &models.Message{}, c.maybeErr(OutboundKindEditText)
}
func (c *capturingBotClient) EditMessageCaption(_ context.Context, p *bot.EditMessageCaptionParams) (*models.Message, error) {
	c.lastEditCaption = p
	return &models.Message{}, c.maybeErr(OutboundKindEditCaption)
}
func (c *capturingBotClient) DeleteMessage(_ context.Context, p *bot.DeleteMessageParams) (bool, error) {
	c.lastDelete = p
	return true, c.maybeErr(OutboundKindDelete)
}
func (c *capturingBotClient) SetMessageReaction(_ context.Context, p *bot.SetMessageReactionParams) (bool, error) {
	c.lastSetReaction = p
	return true, c.maybeErr(OutboundKindSetReaction)
}
func (c *capturingBotClient) SetWebhook(_ context.Context, p *bot.SetWebhookParams) (bool, error) {
	c.lastSetWebhook = p
	return true, nil
}
func (c *capturingBotClient) DeleteWebhook(_ context.Context, p *bot.DeleteWebhookParams) (bool, error) {
	c.lastDeleteWebhook = p
	return true, nil
}

// runnableBotClient embeds capturingBotClient and adds a blocking
// Start so the adapter's polling lifecycle (Phase 2E.8 sub-E) can
// be exercised end to end. Start blocks until ctx is cancelled,
// which is the same shape *bot.Bot.Start has.
type runnableBotClient struct {
	capturingBotClient
	started chan struct{}
}

func newRunnableBotClient() *runnableBotClient {
	return &runnableBotClient{started: make(chan struct{}, 1)}
}

func (r *runnableBotClient) Start(ctx context.Context) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
}

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
