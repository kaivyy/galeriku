// Package messaging publishes domain events to RabbitMQ for the asynchronous
// Phase 4 AI worker pipeline.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// PhotoIngestEvent is published once per uploaded object so the AI worker can fetch it,
// extract faces and BIB numbers, and index the result.
type PhotoIngestEvent struct {
	EventType        string    `json:"event_type"`
	ObjectKey        string    `json:"object_key"`
	UploadBatchID    string    `json:"upload_batch_id"`
	UploadItemID     string    `json:"upload_item_id"`
	StorageAccountID string    `json:"storage_account_id"`
	ActualBytes      int64     `json:"actual_bytes"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// Publisher delivers photo ingest events to the AI queue.
type Publisher interface {
	PublishPhotoIngest(ctx context.Context, event PhotoIngestEvent) error
	Close() error
}

// RabbitPublisher publishes events onto one durable queue.
type RabbitPublisher struct {
	queueName string
	mu        sync.Mutex
	conn      *amqp.Connection
	channel   *amqp.Channel
	url       string
}

// NewRabbitPublisher connects to RabbitMQ and declares the target queue as durable.
func NewRabbitPublisher(url, queueName string) (*RabbitPublisher, error) {
	if url == "" || queueName == "" {
		return nil, fmt.Errorf("rabbit publisher requires both a url and a queue name")
	}
	p := &RabbitPublisher{url: url, queueName: queueName}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *RabbitPublisher) connect() error {
	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to open rabbitmq channel: %w", err)
	}
	// The queue is durable so a broker restart does not drop pending ingest work.
	if _, err := ch.QueueDeclare(p.queueName, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("failed to declare queue %s: %w", p.queueName, err)
	}
	p.conn = conn
	p.channel = ch
	return nil
}

// PublishPhotoIngest sends one event, reconnecting once if the channel has gone away.
func (p *RabbitPublisher) PublishPhotoIngest(ctx context.Context, event PhotoIngestEvent) error {
	if event.EventType == "" {
		event.EventType = "photo.uploaded"
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to encode photo ingest event: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.channel == nil || p.channel.IsClosed() {
		if err := p.connect(); err != nil {
			return err
		}
	}

	err = p.channel.PublishWithContext(ctx, "", p.queueName, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
	if err != nil {
		// One reconnect attempt keeps a transient channel failure from losing the event.
		if reconnErr := p.connect(); reconnErr != nil {
			return fmt.Errorf("failed to publish photo ingest event: %w", err)
		}
		err = p.channel.PublishWithContext(ctx, "", p.queueName, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now().UTC(),
			Body:         body,
		})
		if err != nil {
			return fmt.Errorf("failed to publish photo ingest event after reconnect: %w", err)
		}
	}
	return nil
}

// Close releases the channel and connection.
func (p *RabbitPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// NoopPublisher accepts events without delivering them, for deployments where the AI
// pipeline is not running yet.
type NoopPublisher struct{}

// PublishPhotoIngest discards the event.
func (NoopPublisher) PublishPhotoIngest(context.Context, PhotoIngestEvent) error { return nil }

// Close is a no-op.
func (NoopPublisher) Close() error { return nil }
