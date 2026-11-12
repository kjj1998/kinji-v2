package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
	"github.com/kjj1998/kinji-v2/lambdas/internal/ingest"
	"github.com/kjj1998/kinji-v2/lambdas/internal/queue"
)

type sqsQueue struct {
	client *sqs.Client
	url    string
}

// Compile-time check that sqsQueue satisfies ingest.Queue.
var _ ingest.Queue = (*sqsQueue)(nil)

func (q *sqsQueue) SendEvent(ctx context.Context, id uuid.UUID) error {
	body, err := json.Marshal(queue.EventMessage{EventID: id})
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err = q.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(q.url),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}
