// Command ingest is the Function URL Lambda that the iPhone Shortcut calls.
// It wires up dependencies once per cold start; request handling lives in internal/ingest.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kjj1998/kinji-v2/lambdas/internal/db"
	"github.com/kjj1998/kinji-v2/lambdas/internal/ingest"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Lambda's init phase is limited to about 10 s, so cold-start setup gets a deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	h, err := setup(ctx, logger)
	cancel()
	if err != nil {
		logger.Error("init failed", "err", err)
		os.Exit(1)
	}

	// Never returns. Warm invocations reuse h and its pool.
	lambda.Start(h.Handle)
}

// setup builds the handler and its dependencies.
func setup(ctx context.Context, logger *slog.Logger) (*ingest.Handler, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	queueURL := os.Getenv("ENRICH_QUEUE_URL")
	if queueURL == "" {
		return nil, errors.New("ENRICH_QUEUE_URL is not set")
	}

	enrichQueue := &sqsQueue{client: sqs.NewFromConfig(cfg), url: queueURL}

	dbURL, err := loadDatabaseURL(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("load database url: %w", err)
	}

	pool, err := newPool(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return &ingest.Handler{Store: db.New(pool), Queue: enrichQueue, Log: logger}, nil
}

// loadDatabaseURL reads the ingest connection string from SSM Parameter Store.
// The value is a secret: never log it or include it in an error.
func loadDatabaseURL(ctx context.Context, cfg aws.Config) (string, error) {
	paramName := os.Getenv("DATABASE_URL_PARAM")
	if paramName == "" {
		return "", errors.New("DATABASE_URL_PARAM is not set")
	}
	client := ssm.NewFromConfig(cfg)

	input := &ssm.GetParameterInput{
		Name:           aws.String(paramName),
		WithDecryption: aws.Bool(true),
	}

	result, err := client.GetParameter(ctx, input)
	if err != nil {
		return "", fmt.Errorf("get parameter %s: %w", paramName, err)
	}

	if result.Parameter == nil || result.Parameter.Value == nil {
		return "", fmt.Errorf("parameter %s has no value", paramName)
	}

	return *result.Parameter.Value, nil
}

// newPool creates a small pgx pool. Each Lambda instance serves one request at a time,
// so two connections are enough and keep total Neon connections low.
func newPool(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.MaxConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	return pool, nil
}
