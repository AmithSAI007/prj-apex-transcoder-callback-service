package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/config"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/model"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/repository"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	serviceTracerName = "service.callback"
)

var (
	uuidRegex = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
)

type CallbackService struct {
	cfg           *config.Config
	firestoreRepo repository.FirestoreRepository
	pubsubRepo    repository.PubSubPublisher
	logger        *zap.Logger
}

func NewCallbackService(cfg *config.Config, firestoreRepo repository.FirestoreRepository, pubsubRepo repository.PubSubPublisher, logger *zap.Logger) *CallbackService {
	return &CallbackService{
		cfg:           cfg,
		firestoreRepo: firestoreRepo,
		pubsubRepo:    pubsubRepo,
		logger:        logger,
	}
}

func (s *CallbackService) ProcessCallback(ctx context.Context, messageID string, data []byte, attributes map[string]string) error {
	tracer := otel.Tracer(serviceTracerName)

	ctx, span := tracer.Start(ctx, "service.ProcessCallback",
		trace.WithAttributes(
			attribute.String("messaging.message.id", messageID),
		),
	)
	defer span.End()

	log := config.LoggerWithTrace(ctx, s.logger, s.cfg.GCPProjectID)

	// --- Parse job result ---
	jobResult, err := s.parseJobResult(data)
	if err != nil {
		span.SetStatus(codes.Error, "failed to parse job result")
		span.RecordError(err)

		log.Error("Failed to parse job result from message payload",
			zap.String("component", "service"),
			zap.String("action", "parse_job_result"),
			zap.String("messageId", messageID),
			zap.Error(err))

		return &PermanentError{Err: fmt.Errorf("parse job result: %w", err)}
	}

	span.AddEvent("job_result.parsed", trace.WithAttributes(
		attribute.String("job.name", jobResult.Job.Name),
		attribute.String("job.state", jobResult.Job.State),
	))

	// --- Extract video ID ---
	videoID, err := s.extractVideoID(jobResult.Job.Name)
	if err != nil {
		span.SetStatus(codes.Error, "failed to extract video ID")
		span.RecordError(err)

		log.Error("Failed to extract video ID from job name",
			zap.String("component", "service"),
			zap.String("action", "extract_video_id"),
			zap.String("jobName", jobResult.Job.Name),
			zap.String("messageId", messageID),
			zap.Error(err))

		return &PermanentError{Err: fmt.Errorf("extract video ID: %w", err)}
	}

	span.SetAttributes(
		attribute.String("video.id", videoID),
		attribute.String("job.state", jobResult.Job.State),
		attribute.String("job.name", jobResult.Job.Name),
	)

	log.Info("Callback parsed successfully",
		zap.String("component", "service"),
		zap.String("action", "callback_parsed"),
		zap.String("videoId", videoID),
		zap.String("jobState", jobResult.Job.State),
		zap.String("jobName", jobResult.Job.Name),
		zap.String("messageId", messageID))

	// --- Route by job state ---
	switch jobResult.Job.State {
	case "SUCCEEDED":
		span.AddEvent("job.state.routed", trace.WithAttributes(
			attribute.String("job.state", "SUCCEEDED"),
		))
		return s.handleSucceeded(ctx, videoID, messageID)

	case "FAILED":
		var jobError *model.JobError
		if jobResult.Job.Error != nil {
			jobError = &model.JobError{
				Code:    jobResult.Job.Error.Code,
				Message: jobResult.Job.Error.Message,
			}
		} else {
			jobError = &model.JobError{
				Code:    -1,
				Message: "unknown error",
			}
		}

		span.AddEvent("job.state.routed", trace.WithAttributes(
			attribute.String("job.state", "FAILED"),
			attribute.Int("job.error.code", jobError.Code),
			attribute.String("job.error.message", jobError.Message),
		))

		return s.handleFailed(ctx, videoID, jobError, messageID)

	default:
		span.SetStatus(codes.Error, "unrecognized job state")
		span.AddEvent("job.state.unrecognized", trace.WithAttributes(
			attribute.String("job.state", jobResult.Job.State),
		))

		log.Warn("Received callback with unrecognized job state",
			zap.String("component", "service"),
			zap.String("action", "unrecognized_job_state"),
			zap.String("jobState", jobResult.Job.State),
			zap.String("videoId", videoID),
			zap.String("messageId", messageID))

		return &TransientError{Err: fmt.Errorf("unrecognized job state: %s", jobResult.Job.State)}
	}
}

func (s *CallbackService) parseJobResult(data []byte) (*model.JobResult, error) {

	var jobResult model.JobResult
	err := json.Unmarshal(data, &jobResult)
	if err != nil {
		return nil, &PermanentError{Err: err}
	}

	if jobResult.Job.Name == "" || jobResult.Job.State == "" {
		return nil, &PermanentError{Err: fmt.Errorf("job name or state is empty")}
	}

	return &jobResult, nil
}

func (s *CallbackService) extractVideoID(jobName string) (string, error) {
	// Assuming the job name is in the format "projects/{project}/locations/{location}/jobs/{jobId}"
	parts := strings.Split(jobName, "/")
	if len(parts) < 6 {
		return "", &PermanentError{Err: fmt.Errorf("invalid job name format: %s", jobName)}
	}

	jobNamePart := parts[len(parts)-1]
	// if job name does not start with "transcode-", return error
	if !strings.HasPrefix(jobNamePart, "transcode-") {
		return "", &PermanentError{Err: fmt.Errorf("invalid job name format: %s", jobName)}
	}

	videoID := strings.TrimPrefix(jobNamePart, "transcode-")
	if videoID == "" {
		return "", &PermanentError{Err: fmt.Errorf("video ID is empty in job name: %s", jobName)}
	}

	if !ValidateUUID(videoID) {
		return "", &PermanentError{Err: fmt.Errorf("invalid video ID format in job name: %s", jobName)}
	}

	return videoID, nil
}

func (s *CallbackService) handleSucceeded(ctx context.Context, videoID string, messageID string) error {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "service.handleSucceeded",
		trace.WithAttributes(
			attribute.String("video.id", videoID),
			attribute.String("messaging.message.id", messageID),
		),
	)
	defer span.End()

	log := config.LoggerWithTrace(ctx, s.logger, s.cfg.GCPProjectID)

	// --- Firestore state transition ---
	result, err := s.firestoreRepo.TransitionToCompleted(ctx, videoID, s.cfg.GCSBucket)
	if err != nil {
		span.SetStatus(codes.Error, "firestore transition failed")
		span.RecordError(err)

		log.Error("Firestore transition to COMPLETED failed",
			zap.String("component", "service"),
			zap.String("action", "transition_to_completed"),
			zap.String("videoId", videoID),
			zap.String("messageId", messageID),
			zap.Error(err))

		return s.classifyRepoError(err)
	}

	if result != nil && result.AlreadyProcessed {
		span.AddEvent("state_transition.idempotent_skip", trace.WithAttributes(
			attribute.String("video.id", videoID),
			attribute.String("reason", "already_in_terminal_state"),
		))

		// Audit log: idempotency guard triggered — important for tracking
		// duplicate deliveries in BigQuery.
		log.Info("Callback already processed (idempotent skip)",
			zap.String("component", "service"),
			zap.String("action", "idempotent_skip"),
			zap.String("videoId", videoID),
			zap.String("targetStatus", "COMPLETED"),
			zap.String("messageId", messageID))

		return nil
	}

	span.AddEvent("state_transition.completed", trace.WithAttributes(
		attribute.String("video.id", videoID),
		attribute.String("new_status", "COMPLETED"),
	))

	// Audit log: state transition — this is the primary audit event for
	// a successful transcode. This row in BigQuery represents the moment
	// the video's state moved to COMPLETED.
	log.Info("Video state transitioned to COMPLETED",
		zap.String("component", "service"),
		zap.String("action", "state_transition"),
		zap.String("videoId", videoID),
		zap.String("previousStatus", "TRANSCODING"),
		zap.String("newStatus", "COMPLETED"),
		zap.String("messageId", messageID))

	if result != nil {
		event := &model.PipelineEvent{
			VideoID:           videoID,
			UserID:            result.UserID,
			Status:            model.STATUS_COMPLETED,
			OuputManifestPath: result.OutputManifestPath,
		}

		span.AddEvent("pipeline_event.publishing", trace.WithAttributes(
			attribute.String("video.id", videoID),
			attribute.String("event.status", model.STATUS_COMPLETED),
		))

		if err := s.pubsubRepo.PublishPipelineEvent(ctx, event); err != nil {
			span.SetStatus(codes.Error, "failed to publish pipeline event")
			span.RecordError(err)

			log.Error("Failed to publish pipeline event for completed video",
				zap.String("component", "service"),
				zap.String("action", "publish_pipeline_event"),
				zap.String("videoId", videoID),
				zap.String("status", model.STATUS_COMPLETED),
				zap.String("messageId", messageID),
				zap.Error(err))

			return &TransientError{Err: fmt.Errorf("publish pipeline event for video %s: %w", videoID, err)}
		}

		span.AddEvent("pipeline_event.published", trace.WithAttributes(
			attribute.String("video.id", videoID),
			attribute.String("event.status", model.STATUS_COMPLETED),
		))

		log.Info("Pipeline event published",
			zap.String("component", "service"),
			zap.String("action", "pipeline_event_published"),
			zap.String("videoId", videoID),
			zap.String("userId", result.UserID),
			zap.String("status", model.STATUS_COMPLETED),
			zap.String("messageId", messageID))
	}

	return nil
}

func (s *CallbackService) handleFailed(ctx context.Context, videoID string, jobError *model.JobError, messageID string) error {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "service.handleFailed",
		trace.WithAttributes(
			attribute.String("video.id", videoID),
			attribute.String("messaging.message.id", messageID),
			attribute.Int("job.error.code", jobError.Code),
			attribute.String("job.error.message", jobError.Message),
		),
	)
	defer span.End()

	log := config.LoggerWithTrace(ctx, s.logger, s.cfg.GCPProjectID)

	// --- Firestore state transition ---
	result, err := s.firestoreRepo.TransitionToFailed(ctx, videoID, jobError.Code, jobError.Message)
	if err != nil {
		span.SetStatus(codes.Error, "firestore transition failed")
		span.RecordError(err)

		log.Error("Firestore transition to FAILED failed",
			zap.String("component", "service"),
			zap.String("action", "transition_to_failed"),
			zap.String("videoId", videoID),
			zap.Int("errorCode", jobError.Code),
			zap.String("errorMessage", jobError.Message),
			zap.String("messageId", messageID),
			zap.Error(err))

		return s.classifyRepoError(err)
	}

	if result != nil && result.AlreadyProcessed {
		span.AddEvent("state_transition.idempotent_skip", trace.WithAttributes(
			attribute.String("video.id", videoID),
			attribute.String("reason", "already_in_terminal_state"),
		))

		log.Info("Callback already processed (idempotent skip)",
			zap.String("component", "service"),
			zap.String("action", "idempotent_skip"),
			zap.String("videoId", videoID),
			zap.String("targetStatus", "FAILED"),
			zap.String("messageId", messageID))

		return nil
	}

	span.AddEvent("state_transition.failed", trace.WithAttributes(
		attribute.String("video.id", videoID),
		attribute.String("new_status", "FAILED"),
		attribute.Int("job.error.code", jobError.Code),
		attribute.String("job.error.message", jobError.Message),
	))

	// Audit log: state transition to FAILED. This is a critical audit event —
	// it captures why a transcode failed with the error details.
	log.Warn("Video state transitioned to FAILED",
		zap.String("component", "service"),
		zap.String("action", "state_transition"),
		zap.String("videoId", videoID),
		zap.String("previousStatus", "TRANSCODING"),
		zap.String("newStatus", "FAILED"),
		zap.Int("jobErrorCode", jobError.Code),
		zap.String("jobErrorMessage", jobError.Message),
		zap.String("messageId", messageID))

	if result != nil {
		event := &model.PipelineEvent{
			VideoID:      videoID,
			UserID:       result.UserID,
			Status:       model.STATUS_FAILED,
			ErrorCode:    jobError.Code,
			ErrorMessage: jobError.Message,
		}

		span.AddEvent("pipeline_event.publishing", trace.WithAttributes(
			attribute.String("video.id", videoID),
			attribute.String("event.status", model.STATUS_FAILED),
		))

		if err := s.pubsubRepo.PublishPipelineEvent(ctx, event); err != nil {
			span.SetStatus(codes.Error, "failed to publish pipeline event")
			span.RecordError(err)

			log.Error("Failed to publish pipeline event for failed video",
				zap.String("component", "service"),
				zap.String("action", "publish_pipeline_event"),
				zap.String("videoId", videoID),
				zap.String("status", model.STATUS_FAILED),
				zap.String("messageId", messageID),
				zap.Error(err))

			return &TransientError{Err: fmt.Errorf("publish pipeline event for video %s: %w", videoID, err)}
		}

		span.AddEvent("pipeline_event.published", trace.WithAttributes(
			attribute.String("video.id", videoID),
			attribute.String("event.status", model.STATUS_FAILED),
		))

		log.Info("Pipeline event published",
			zap.String("component", "service"),
			zap.String("action", "pipeline_event_published"),
			zap.String("videoId", videoID),
			zap.String("userId", result.UserID),
			zap.String("status", model.STATUS_FAILED),
			zap.String("messageId", messageID))
	}

	return nil
}

func (s *CallbackService) classifyRepoError(err error) error {
	if errors.Is(err, repository.ErrDocumentNotFound) {
		return &PermanentError{Err: err}
	}
	if errors.Is(err, repository.ErrInvalidTransition) {
		return &PermanentError{Err: err}
	}

	code := status.Code(err)
	switch code {
	case grpccodes.Unavailable, grpccodes.DeadlineExceeded, grpccodes.Aborted, grpccodes.Canceled:
		return &TransientError{Err: err}
	case grpccodes.PermissionDenied, grpccodes.Unauthenticated:
		return &PermanentError{Err: err}
	default:
		return &TransientError{Err: err}
	}
}

func ValidateUUID(value string) bool {
	return uuidRegex.MatchString(value)
}
