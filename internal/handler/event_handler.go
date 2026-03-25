package handler

import (
	"context"
	"fmt"

	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/config"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/model"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/platform"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	handlerTracerName = "handler"
)

type HeaderKey string

const (
	CeTypeHeaderKey       HeaderKey = "eventType"
	CeSourceHeaderKey     HeaderKey = "bucketId"
	CeIDHeaderKey         HeaderKey = "objectId"
	CeSubjectHeaderKey    HeaderKey = "objectGeneration"
	CeTimeHeaderKey       HeaderKey = "eventTime"
	GoogClientTraceparent HeaderKey = "googclient_traceparent"
)

type EventHandler struct {
	logger    *zap.Logger
	i         *service.CallbackService
	projectID string
}

func NewEventHandler(logger *zap.Logger, i *service.CallbackService, projectID string) *EventHandler {
	return &EventHandler{
		logger:    logger,
		i:         i,
		projectID: projectID,
	}
}

func (h *EventHandler) Handle(ctx context.Context, messageId string, data []byte, attrs map[string]string) platform.Result {
	tracer := otel.Tracer(handlerTracerName)

	ctx, span := tracer.Start(ctx, "handler.Handle",
		trace.WithAttributes(
			attribute.String("messaging.message.id", messageId),
		),
	)
	defer span.End()

	// Create a trace-correlated logger for this request.
	// Every log entry emitted with this logger will carry the GCP Cloud Logging
	// trace/spanId fields, enabling correlation in BigQuery audit queries.
	log := config.LoggerWithTrace(ctx, h.logger, h.projectID)

	meta, err := parseCloudEventMeta(attrs)
	if err != nil {
		span.SetStatus(codes.Error, "missing cloud event attributes")
		span.RecordError(err)

		log.Error("Failed to parse CloudEvent metadata — message will be acked and dropped",
			zap.String("component", "handler"),
			zap.String("action", "parse_cloud_event_meta"),
			zap.String("messageId", messageId),
			zap.Error(err))

		return platform.ResultAck
	}

	span.SetAttributes(
		attribute.String("cloudevent.type", meta.EventType),
		attribute.String("cloudevent.source", meta.Source),
		attribute.String("cloudevent.id", meta.ID),
		attribute.String("cloudevent.subject", meta.Subject),
		attribute.String("cloudevent.time", meta.EventTime),
	)
	span.AddEvent("cloudevent.metadata.parsed")

	log.Info("Processing callback message",
		zap.String("component", "handler"),
		zap.String("action", "process_callback"),
		zap.String("messageId", messageId),
		zap.String("eventType", meta.EventType),
		zap.String("eventId", meta.ID),
		zap.String("source", meta.Source),
		zap.String("subject", meta.Subject))

	if err := h.i.ProcessCallback(ctx, meta.ID, data, attrs); err != nil {
		return h.respondWithError(ctx, err, meta, messageId)
	}

	span.SetStatus(codes.Ok, "callback processed successfully")

	// Audit log: successful processing — this is the happy path entry
	// that confirms a message was fully handled and will be acked.
	log.Info("Callback processed successfully",
		zap.String("component", "handler"),
		zap.String("action", "callback_processed"),
		zap.String("outcome", "success"),
		zap.String("messageId", messageId),
		zap.String("eventId", meta.ID))

	return platform.ResultAck
}

func parseCloudEventMeta(attrs map[string]string) (*model.MetaData, error) {
	ceType, ok := attrs[string(CeTypeHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-type attribute")
	}

	ceSource, ok := attrs[string(CeSourceHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-source attribute")
	}

	ceID, ok := attrs[string(CeIDHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-id attribute")
	}

	ceSubject, ok := attrs[string(CeSubjectHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-subject attribute")
	}

	ceTime, ok := attrs[string(CeTimeHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-time attribute")
	}

	return &model.MetaData{
		EventType: ceType,
		Source:    ceSource,
		ID:        ceID,
		Subject:   ceSubject,
		EventTime: ceTime,
	}, nil
}

func (h *EventHandler) respondWithError(ctx context.Context, err error, meta *model.MetaData, messageId string) platform.Result {
	span := trace.SpanFromContext(ctx)
	log := config.LoggerWithTrace(ctx, h.logger, h.projectID)

	switch e := err.(type) {
	case *service.PermanentError:
		span.SetStatus(codes.Error, "permanent error")
		span.RecordError(e, trace.WithAttributes(
			attribute.String("error.type", "permanent"),
		))
		span.AddEvent("message.ack.permanent_error")

		// Audit log: permanent error — message will be acked and not retried.
		// This is important for tracking poison messages in BigQuery.
		log.Error("Permanent error processing callback — message acked (no retry)",
			zap.String("component", "handler"),
			zap.String("action", "permanent_error"),
			zap.String("outcome", "ack"),
			zap.String("messageId", messageId),
			zap.String("eventId", meta.ID),
			zap.Error(e))

		return platform.ResultAck

	case *service.TransientError:
		span.SetStatus(codes.Error, "transient error")
		span.RecordError(e, trace.WithAttributes(
			attribute.String("error.type", "transient"),
		))
		span.AddEvent("message.nack.transient_error")

		// Audit log: transient error — message will be nacked for retry.
		log.Warn("Transient error processing callback — message nacked (will retry)",
			zap.String("component", "handler"),
			zap.String("action", "transient_error"),
			zap.String("outcome", "nack"),
			zap.String("messageId", messageId),
			zap.String("eventId", meta.ID),
			zap.Error(e))

		return platform.ResultNack

	default:
		span.SetStatus(codes.Error, "unknown error")
		span.RecordError(e, trace.WithAttributes(
			attribute.String("error.type", "unknown"),
		))
		span.AddEvent("message.nack.unknown_error")

		// Audit log: unclassified error — fail-open, nack for retry.
		log.Error("Unknown error type processing callback — treating as transient, nacking",
			zap.String("component", "handler"),
			zap.String("action", "unknown_error"),
			zap.String("outcome", "nack"),
			zap.String("messageId", messageId),
			zap.String("eventId", meta.ID),
			zap.Error(e))

		return platform.ResultNack
	}
}
