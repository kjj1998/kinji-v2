package ingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kjj1998/kinji-v2/lambdas/internal/db"
	"github.com/kjj1998/kinji-v2/lambdas/internal/token"
)

type Store interface {
	GetActiveTokenByHash(ctx context.Context, hash []byte) (db.GetActiveTokenByHashRow, error)
	InsertCaptureEvent(ctx context.Context, arg db.InsertCaptureEventParams) (uuid.UUID, error)
	TouchAPIToken(ctx context.Context, id uuid.UUID) error
}

type Handler struct {
	Store Store
	Log   *slog.Logger
}

type envelope struct {
	Source string `json:"source"`
}

var allowedSources = map[string]bool{
	"wallet_tap":          true,
	"wallet_notification": true,
	"bank_notification":   true,
}

var errUnauthorized = errors.New("unauthorized")

func (h *Handler) Handle(ctx context.Context, req events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	if req.RequestContext.HTTP.Method != http.MethodPost {
		resp := respond(http.StatusMethodNotAllowed, errBody("method not allowed"))
		resp.Headers["Allow"] = "POST"
		return resp, nil
	}

	body, err := decodeBody(req)
	if err != nil {
		return respond(http.StatusBadRequest, errBody("invalid body")), nil
	}
	if len(body) > maxBodyBytes {
		return respond(http.StatusRequestEntityTooLarge, errBody("body too large")), nil
	}

	authToken, ok := strings.CutPrefix(req.Headers["authorization"], "Bearer ")
	if !ok || !strings.HasPrefix(authToken, token.Prefix) || len(authToken) != 47 {
		return respond(http.StatusUnauthorized, errBody("unauthorized")), nil
	}

	tok, err := h.activeToken(ctx, authToken)
	switch {
	case errors.Is(err, errUnauthorized):
		return respond(http.StatusUnauthorized, errBody("unauthorized")), nil
	case err != nil:
		h.Log.ErrorContext(ctx, "token lookup failed", "err", err)
		return respond(http.StatusInternalServerError, errBody("internal error")), nil
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return respond(http.StatusBadRequest, errBody("invalid body")), nil
	}
	if !allowedSources[env.Source] {
		return respond(http.StatusBadRequest, errBody("invalid source")), nil
	}

	if looksLikeOTP(body) {
		h.Log.InfoContext(ctx, "dropped otp-like payload", "source", env.Source)
		return respond(http.StatusAccepted, map[string]string{"status": "dropped"}), nil
	}

	id, err := h.Store.InsertCaptureEvent(ctx, db.InsertCaptureEventParams{
		UserID:    tok.UserID,
		Source:    env.Source,
		Payload:   body,
		DedupeKey: dedupeKey(tok.UserID, env.Source, body),
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// ON CONFLICT DO NOTHING returned no row: this is a duplicate, which is still a 202.
	case err != nil:
		h.Log.ErrorContext(ctx, "insert capture event failed", "err", err)
		return respond(http.StatusInternalServerError, errBody("internal error")), nil
	}

	if err := h.touchToken(ctx, tok.ID); err != nil {
		h.Log.ErrorContext(ctx, "touch api token failed", "err", err)
	}

	if id == uuid.Nil {
		return respond(http.StatusAccepted, map[string]string{"status": "duplicate"}), nil
	}
	return respond(http.StatusAccepted, map[string]string{"status": "accepted", "id": id.String()}), nil
}

func respond(status int, body any) events.LambdaFunctionURLResponse {
	b, _ := json.Marshal(body)
	return events.LambdaFunctionURLResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}
}

func errBody(msg string) map[string]string {
	return map[string]string{"error": msg}
}

const maxBodyBytes = 16 << 10 // 16 KiB

var errBadEncoding = errors.New("body is not valid base64")

func decodeBody(req events.LambdaFunctionURLRequest) ([]byte, error) {
	if !req.IsBase64Encoded {
		return []byte(req.Body), nil
	}
	b, err := base64.StdEncoding.DecodeString(req.Body)
	if err != nil {
		return nil, errBadEncoding
	}
	return b, nil
}

func (h *Handler) activeToken(ctx context.Context, authToken string) (db.GetActiveTokenByHashRow, error) {
	activeToken, err := h.Store.GetActiveTokenByHash(ctx, token.Hash(authToken))

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return db.GetActiveTokenByHashRow{}, errUnauthorized
	case err != nil:
		return db.GetActiveTokenByHashRow{}, fmt.Errorf("look up token: %w", err)
	}

	return activeToken, nil
}

func (h *Handler) touchToken(ctx context.Context, id uuid.UUID) error {
	if err := h.Store.TouchAPIToken(ctx, id); err != nil {
		return fmt.Errorf("touch api token: %w", err)
	}

	return nil
}
