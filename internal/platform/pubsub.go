package platform

import (
	"context"

	"cloud.google.com/go/pubsub/v2"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	pubsubTracerName = "platform.pubsub"
)

type Result int

const (
	ResultAck Result = iota
	ResultNack
)

type MessageHandler func(ctx context.Context, messageId string, data []byte, attrs map[string]string) Result

type Subscriber struct {
	logger       *zap.Logger
	client       *pubsub.Client
	subscription *pubsub.Subscriber
	projectID    string
}

func NewSubscriber(ctx context.Context, logger *zap.Logger, projectID, subscriptionID string, maxOutstanding int) (*Subscriber, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}

	sub := client.Subscriber(subscriptionID)
	sub.ReceiveSettings = pubsub.ReceiveSettings{
		MaxOutstandingMessages: maxOutstanding,
		NumGoroutines:          10,
	}

	return &Subscriber{
		client:       client,
		subscription: sub,
		logger:       logger,
		projectID:    projectID,
	}, nil
}

func (s *Subscriber) Start(ctx context.Context, handler MessageHandler) error {
	s.logger.Info("Starting Pub/Sub subscriber",
		zap.String("component", "platform.pubsub"),
		zap.String("action", "start_subscriber"),
		zap.String("subscription", s.subscription.ID()))

	return s.subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		tracer := otel.Tracer(pubsubTracerName)

		// Extract trace context propagated via Pub/Sub message attributes.
		// This links the span to the upstream producer's trace so the
		// entire pipeline appears as a single distributed trace in Cloud Trace.
		propagator := otel.GetTextMapPropagator()
		carrier := mapCarrier(msg.Attributes)
		ctx = propagator.Extract(ctx, carrier)

		ctx, span := tracer.Start(ctx, "pubsub.receive",
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(
				attribute.String("messaging.system", "gcp_pubsub"),
				attribute.String("messaging.operation", "receive"),
				attribute.String("messaging.message.id", msg.ID),
				attribute.String("messaging.destination.name", s.subscription.ID()),
				attribute.Int("messaging.message.body.size", len(msg.Data)),
			),
		)
		defer span.End()

		// Create a trace-correlated logger for this message's processing lifecycle.
		msgLogger := config.LoggerWithTrace(ctx, s.logger, s.projectID)

		msgLogger.Info("Message received",
			zap.String("component", "platform.pubsub"),
			zap.String("action", "receive_message"),
			zap.String("messageId", msg.ID),
			zap.Int("dataSize", len(msg.Data)),
			zap.Time("publishTime", msg.PublishTime))

		span.AddEvent("message.processing.start", trace.WithAttributes(
			attribute.String("messaging.message.id", msg.ID),
		))

		result := handler(ctx, msg.ID, msg.Data, msg.Attributes)

		switch result {
		case ResultAck:
			msg.Ack()
			span.SetStatus(codes.Ok, "message acknowledged")
			span.AddEvent("message.ack")

			msgLogger.Info("Message acknowledged",
				zap.String("component", "platform.pubsub"),
				zap.String("action", "ack_message"),
				zap.String("outcome", "ack"),
				zap.String("messageId", msg.ID))

		case ResultNack:
			msg.Nack()
			span.SetStatus(codes.Error, "message negatively acknowledged")
			span.AddEvent("message.nack")

			msgLogger.Warn("Message negatively acknowledged",
				zap.String("component", "platform.pubsub"),
				zap.String("action", "nack_message"),
				zap.String("outcome", "nack"),
				zap.String("messageId", msg.ID))
		}
	})
}

func (s *Subscriber) Close() error {
	s.logger.Info("Closing Pub/Sub subscriber",
		zap.String("component", "platform.pubsub"),
		zap.String("action", "close_subscriber"),
		zap.String("subscription", s.subscription.ID()))
	return s.client.Close()
}

// mapCarrier adapts a map[string]string (Pub/Sub message attributes)
// to the propagation.TextMapCarrier interface for trace context extraction.
type mapCarrier map[string]string

func (c mapCarrier) Get(key string) string {
	return c[key]
}

func (c mapCarrier) Set(key, value string) {
	c[key] = value
}

func (c mapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
