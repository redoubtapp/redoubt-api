package livekit

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/webhook"
)

// PresencePublisher defines the interface for publishing presence events.
// This will be implemented by the presence hub in Phase 2.
type PresencePublisher interface {
	PublishVoiceJoin(ctx context.Context, channelID, userID string)
	PublishVoiceLeave(ctx context.Context, channelID, userID string)
	PublishVoiceMute(ctx context.Context, channelID, userID string, muted bool)
}

// WebhookHandler handles LiveKit webhook events.
type WebhookHandler struct {
	keyProvider auth.KeyProvider
	presence    PresencePublisher
	logger      *slog.Logger
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(apiKey, apiSecret string, presence PresencePublisher, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		keyProvider: auth.NewSimpleKeyProvider(apiKey, apiSecret),
		presence:    presence,
		logger:      logger,
	}
}

// ServeHTTP handles incoming webhook requests from LiveKit.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	event, err := webhook.ReceiveWebhookEvent(r, h.keyProvider)
	if err != nil {
		h.logger.Error("invalid webhook", slog.String("error", err.Error()))
		http.Error(w, "invalid webhook", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()

	// Extract room name (which encodes channel info)
	roomName := ""
	if event.GetRoom() != nil {
		roomName = event.GetRoom().GetName()
	}

	switch event.GetEvent() {
	case webhook.EventParticipantJoined:
		p := event.GetParticipant()
		h.logger.Info("participant joined",
			slog.String("room", roomName),
			slog.String("identity", p.GetIdentity()),
			slog.String("name", p.GetName()),
		)
		if h.presence != nil {
			h.presence.PublishVoiceJoin(ctx, roomName, p.GetIdentity())
		}

	case webhook.EventParticipantLeft:
		p := event.GetParticipant()
		h.logger.Info("participant left",
			slog.String("room", roomName),
			slog.String("identity", p.GetIdentity()),
		)
		if h.presence != nil {
			h.presence.PublishVoiceLeave(ctx, roomName, p.GetIdentity())
		}

	case webhook.EventTrackPublished:
		p := event.GetParticipant()
		track := event.GetTrack()
		h.logger.Debug("track published",
			slog.String("room", roomName),
			slog.String("identity", p.GetIdentity()),
			slog.String("track_type", track.GetType().String()),
		)

	case webhook.EventTrackUnpublished:
		p := event.GetParticipant()
		track := event.GetTrack()
		h.logger.Debug("track unpublished",
			slog.String("room", roomName),
			slog.String("identity", p.GetIdentity()),
			slog.String("track_type", track.GetType().String()),
		)

	case webhook.EventRoomStarted:
		h.logger.Info("room started", slog.String("room", roomName))

	case webhook.EventRoomFinished:
		h.logger.Info("room finished", slog.String("room", roomName))

	default:
		h.logger.Debug("unhandled webhook event",
			slog.String("event", event.GetEvent()),
		)
	}

	w.WriteHeader(http.StatusOK)
}
