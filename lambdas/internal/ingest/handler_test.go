package ingest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kjj1998/kinji-v2/lambdas/internal/db"
	"github.com/kjj1998/kinji-v2/lambdas/internal/token"
)

// fakeStore is an in-memory Store. It records every write so tests can check what the
// handler did, and its error fields make a given call fail.
type fakeStore struct {
	tokens map[string]db.GetActiveTokenByHashRow // keyed by string(token hash)

	lookupErr error
	insertErr error
	touchErr  error
	duplicate bool // InsertCaptureEvent behaves like ON CONFLICT DO NOTHING skipping the row

	inserted []db.InsertCaptureEventParams
	touched  []uuid.UUID
}

type fakeQueue struct {
	sent    []uuid.UUID
	sendErr error
}

func (f *fakeStore) GetActiveTokenByHash(_ context.Context, hash []byte) (db.GetActiveTokenByHashRow, error) {
	if f.lookupErr != nil {
		return db.GetActiveTokenByHashRow{}, f.lookupErr
	}
	row, ok := f.tokens[string(hash)]
	if !ok {
		return db.GetActiveTokenByHashRow{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeStore) InsertCaptureEvent(_ context.Context, arg db.InsertCaptureEventParams) (uuid.UUID, error) {
	if f.insertErr != nil {
		return uuid.Nil, f.insertErr
	}
	f.inserted = append(f.inserted, arg)
	if f.duplicate {
		return uuid.Nil, pgx.ErrNoRows
	}
	return uuid.New(), nil
}

func (f *fakeStore) TouchAPIToken(_ context.Context, id uuid.UUID) error {
	f.touched = append(f.touched, id)
	return f.touchErr
}

func (q *fakeQueue) SendEvent(_ context.Context, id uuid.UUID) error {
	q.sent = append(q.sent, id)
	return q.sendErr
}

var (
	testToken  = token.Generate()
	testUserID = uuid.New()
	testTokID  = uuid.New()
)

const validBody = `{"source":"wallet_tap","sent_at":"2026-10-31T12:34:56+08:00",` +
	`"data":{"merchant":"Kopitiam","amount":"4.50","card":"1234"}}`

// newHandler returns a handler whose store knows testToken, plus the store for assertions.
func newHandler() (*Handler, *fakeStore, *fakeQueue) {
	store := &fakeStore{tokens: map[string]db.GetActiveTokenByHashRow{
		string(token.Hash(testToken)): {UserID: testUserID, ID: testTokID},
	}}
	queue := &fakeQueue{}
	h := &Handler{Store: store, Queue: queue, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return h, store, queue
}

// newRequest builds a Function URL request. Header names are lowercase, as Function URLs send them.
func newRequest(method, body, auth string) events.LambdaFunctionURLRequest {
	req := events.LambdaFunctionURLRequest{
		Body:    body,
		Headers: map[string]string{},
	}
	req.RequestContext.HTTP.Method = method
	if auth != "" {
		req.Headers["authorization"] = auth
	}
	return req
}

func TestHandle(t *testing.T) {
	bearer := "Bearer " + testToken

	tests := []struct {
		name       string
		req        events.LambdaFunctionURLRequest
		setup      func(*fakeStore) // optional: make the store fail or report a duplicate
		sendErr    error            // optional: make the queue send fail
		wantStatus int
		wantBody   map[string]string // checked key by key; "id" is only checked to be present
		wantInsert int
		wantTouch  int
		wantSend   int
	}{
		{
			name:       "wrong method",
			req:        newRequest(http.MethodGet, validBody, bearer),
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   map[string]string{"error": "method not allowed"},
		},
		{
			name: "invalid base64",
			req: func() events.LambdaFunctionURLRequest {
				r := newRequest(http.MethodPost, "!!!not-base64", bearer)
				r.IsBase64Encoded = true
				return r
			}(),
			wantStatus: http.StatusBadRequest,
			wantBody:   map[string]string{"error": "invalid body"},
		},
		{
			name:       "body at the limit is accepted",
			req:        newRequest(http.MethodPost, padTo(maxBodyBytes), bearer),
			wantStatus: http.StatusAccepted,
			wantBody:   map[string]string{"status": "accepted"},
			wantInsert: 1,
			wantTouch:  1,
			wantSend:   1,
		},
		{
			name:       "body over the limit",
			req:        newRequest(http.MethodPost, padTo(maxBodyBytes+1), bearer),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantBody:   map[string]string{"error": "body too large"},
		},
		{
			name:       "missing auth header",
			req:        newRequest(http.MethodPost, validBody, ""),
			wantStatus: http.StatusUnauthorized,
			wantBody:   map[string]string{"error": "unauthorized"},
		},
		{
			name:       "not a bearer token",
			req:        newRequest(http.MethodPost, validBody, "Basic "+testToken),
			wantStatus: http.StatusUnauthorized,
			wantBody:   map[string]string{"error": "unauthorized"},
		},
		{
			name:       "wrong prefix",
			req:        newRequest(http.MethodPost, validBody, "Bearer xyz_"+testToken[4:]),
			wantStatus: http.StatusUnauthorized,
			wantBody:   map[string]string{"error": "unauthorized"},
		},
		{
			name:       "wrong length",
			req:        newRequest(http.MethodPost, validBody, bearer+"x"),
			wantStatus: http.StatusUnauthorized,
			wantBody:   map[string]string{"error": "unauthorized"},
		},
		{
			name:       "unknown token",
			req:        newRequest(http.MethodPost, validBody, "Bearer "+token.Generate()),
			wantStatus: http.StatusUnauthorized,
			wantBody:   map[string]string{"error": "unauthorized"},
		},
		{
			name:       "token lookup fails",
			req:        newRequest(http.MethodPost, validBody, bearer),
			setup:      func(f *fakeStore) { f.lookupErr = errors.New("db down") },
			wantStatus: http.StatusInternalServerError,
			wantBody:   map[string]string{"error": "internal error"},
		},
		{
			name:       "invalid json",
			req:        newRequest(http.MethodPost, `{"source":`, bearer),
			wantStatus: http.StatusBadRequest,
			wantBody:   map[string]string{"error": "invalid body"},
		},
		{
			name:       "missing source",
			req:        newRequest(http.MethodPost, `{"data":{}}`, bearer),
			wantStatus: http.StatusBadRequest,
			wantBody:   map[string]string{"error": "invalid source"},
		},
		{
			name:       "unknown source",
			req:        newRequest(http.MethodPost, `{"source":"email"}`, bearer),
			wantStatus: http.StatusBadRequest,
			wantBody:   map[string]string{"error": "invalid source"},
		},
		{
			name: "otp payload is dropped",
			req: newRequest(http.MethodPost,
				`{"source":"bank_notification","data":{"body":"Your OTP is 123456. Do not share."}}`, bearer),
			wantStatus: http.StatusAccepted,
			wantBody:   map[string]string{"status": "dropped"},
		},
		{
			name:       "new event",
			req:        newRequest(http.MethodPost, validBody, bearer),
			wantStatus: http.StatusAccepted,
			wantBody:   map[string]string{"status": "accepted"},
			wantInsert: 1,
			wantTouch:  1,
			wantSend:   1,
		},
		{
			// A real purchase SMS has a 4-digit card number but no OTP keyword, so it must not be dropped.
			name: "bank sms purchase is accepted",
			req: newRequest(http.MethodPost,
				`{"source":"bank_sms","sender":"UOB","received_at":"2026-11-12T12:34:56+08:00",`+
					`"text":"A transaction of SGD 93.46 was made with your UOB Card ending 1234 on 11/12/25 `+
					`at SHOPEE SG MP. If unauthorised, call 24/7 Fraud Hotline now"}`, bearer),
			wantStatus: http.StatusAccepted,
			wantBody:   map[string]string{"status": "accepted"},
			wantInsert: 1,
			wantTouch:  1,
			wantSend:   1,
		},
		{
			name: "new event, base64 body",
			req: func() events.LambdaFunctionURLRequest {
				r := newRequest(http.MethodPost, base64.StdEncoding.EncodeToString([]byte(validBody)), bearer)
				r.IsBase64Encoded = true
				return r
			}(),
			wantStatus: http.StatusAccepted,
			wantBody:   map[string]string{"status": "accepted"},
			wantInsert: 1,
			wantTouch:  1,
			wantSend:   1,
		},
		{
			name:       "duplicate event",
			req:        newRequest(http.MethodPost, validBody, bearer),
			setup:      func(f *fakeStore) { f.duplicate = true },
			wantStatus: http.StatusAccepted,
			wantBody:   map[string]string{"status": "duplicate"},
			wantInsert: 1,
			wantTouch:  1,
		},
		{
			name:       "insert fails",
			req:        newRequest(http.MethodPost, validBody, bearer),
			setup:      func(f *fakeStore) { f.insertErr = errors.New("db down") },
			wantStatus: http.StatusInternalServerError,
			wantBody:   map[string]string{"error": "internal error"},
		},
		{
			name:       "touch fails but event is accepted",
			req:        newRequest(http.MethodPost, validBody, bearer),
			setup:      func(f *fakeStore) { f.touchErr = errors.New("db down") },
			wantStatus: http.StatusAccepted,
			wantBody:   map[string]string{"status": "accepted"},
			wantInsert: 1,
			wantTouch:  1,
			wantSend:   1,
		},
		{
			// The event is already in Postgres; the sweeper re-queues it, so the phone still gets a 202.
			name:       "send fails but event is accepted",
			req:        newRequest(http.MethodPost, validBody, bearer),
			sendErr:    errors.New("sqs down"),
			wantStatus: http.StatusAccepted,
			wantBody:   map[string]string{"status": "accepted"},
			wantInsert: 1,
			wantTouch:  1,
			wantSend:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, store, queue := newHandler()
			if tt.setup != nil {
				tt.setup(store)
			}
			queue.sendErr = tt.sendErr

			resp, err := h.Handle(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("Handle returned error %v; it must always return nil (a non-nil error is a 502)", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", resp.StatusCode, tt.wantStatus, resp.Body)
			}
			if ct := resp.Headers["Content-Type"]; ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var got map[string]string
			if err := json.Unmarshal([]byte(resp.Body), &got); err != nil {
				t.Fatalf("response body %q is not a JSON object of strings: %v", resp.Body, err)
			}
			for k, want := range tt.wantBody {
				if got[k] != want {
					t.Errorf("body[%q] = %q, want %q", k, got[k], want)
				}
			}
			if tt.wantBody["status"] == "accepted" {
				if _, err := uuid.Parse(got["id"]); err != nil {
					t.Errorf("body[\"id\"] = %q, want a UUID", got["id"])
				}
			}

			if len(store.inserted) != tt.wantInsert {
				t.Errorf("inserts = %d, want %d", len(store.inserted), tt.wantInsert)
			}
			if len(store.touched) != tt.wantTouch {
				t.Errorf("touches = %d, want %d", len(store.touched), tt.wantTouch)
			}
			if len(queue.sent) != tt.wantSend {
				t.Errorf("sends = %d, want %d", len(queue.sent), tt.wantSend)
			}
		})
	}
}

func TestHandleMethodNotAllowedSetsAllow(t *testing.T) {
	h, _, _ := newHandler()
	resp, _ := h.Handle(context.Background(), newRequest(http.MethodGet, validBody, ""))
	if got := resp.Headers["Allow"]; got != "POST" {
		t.Errorf("Allow = %q, want POST", got)
	}
}

// TestHandleWritesRawEvent checks what reaches the store, not just how many calls were made.
func TestHandleWritesRawEvent(t *testing.T) {
	h, store, queue := newHandler()
	resp, err := h.Handle(context.Background(), newRequest(http.MethodPost, validBody, "Bearer "+testToken))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("inserts = %d, want 1", len(store.inserted))
	}

	got := store.inserted[0]
	if got.UserID != testUserID {
		t.Errorf("UserID = %s, want the token's user %s", got.UserID, testUserID)
	}
	if got.Source != "wallet_tap" {
		t.Errorf("Source = %q, want wallet_tap", got.Source)
	}
	if !bytes.Equal(got.Payload, []byte(validBody)) {
		t.Errorf("Payload was changed; raw-first means it is stored exactly as received")
	}
	if want := dedupeKey(testUserID, "wallet_tap", []byte(validBody)); !bytes.Equal(got.DedupeKey, want) {
		t.Errorf("DedupeKey = %x, want %x", got.DedupeKey, want)
	}

	if len(store.touched) != 1 || store.touched[0] != testTokID {
		t.Errorf("touched = %v, want [%s] (the token ID, not the event ID)", store.touched, testTokID)
	}

	var body map[string]string
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("response body %q is not JSON: %v", resp.Body, err)
	}
	if len(queue.sent) != 1 || queue.sent[0].String() != body["id"] {
		t.Errorf("sent = %v, want [%s] (the inserted event ID from the response)", queue.sent, body["id"])
	}
}

// padTo returns a valid wallet_tap envelope of exactly n bytes.
func padTo(n int) string {
	const head, tail = `{"source":"wallet_tap","pad":"`, `"}`
	return head + strings.Repeat("a", n-len(head)-len(tail)) + tail
}
