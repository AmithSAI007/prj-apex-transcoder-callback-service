package repository

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/config"
	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	repoTracerName = "repository.firestore"

	StatusField             = "status"
	UpdatedAtField          = "updatedAt"
	OutputManifestPathField = "outputManifestPath"
	ErrorCodeField          = "errorCode"
	ErrorMessageField       = "errorMessage"
)

type FirestoreRepo struct {
	client     *firestore.Client
	collection string
	logger     *zap.Logger
	projectID  string
}

func NewFirestoreRepo(client *firestore.Client, collection string, logger *zap.Logger, projectID string) *FirestoreRepo {
	return &FirestoreRepo{
		client:     client,
		collection: collection,
		logger:     logger,
		projectID:  projectID,
	}
}

func (r *FirestoreRepo) TransitionToCompleted(ctx context.Context, videoID string, processedBucket string) (*CompletedResult, error) {
	tracer := otel.Tracer(repoTracerName)

	ctx, span := tracer.Start(ctx, "repository.TransitionToCompleted",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "firestore"),
			attribute.String("db.operation", "transaction"),
			attribute.String("db.collection", r.collection),
			attribute.String("db.document.id", videoID),
		),
	)
	defer span.End()

	log := config.LoggerWithTrace(ctx, r.logger, r.projectID)

	var result *CompletedResult

	docRef := r.client.Collection(r.collection).Doc(videoID)

	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		span.AddEvent("firestore.transaction.started")

		docSnap, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == grpccodes.NotFound {
				span.AddEvent("firestore.document.not_found", trace.WithAttributes(
					attribute.String("db.document.id", videoID),
				))
				return ErrDocumentNotFound
			}
			return fmt.Errorf("firestore get document %s: %w", videoID, err)
		}

		span.AddEvent("firestore.document.read", trace.WithAttributes(
			attribute.String("db.document.id", videoID),
		))

		// get VideoDocument from Firestore document snapshot
		var videoDoc *model.VideoDocument
		if err := docSnap.DataTo(&videoDoc); err != nil {
			return fmt.Errorf("firestore data to struct for document %s: %w", videoID, err)
		}

		// Check if the document is already in a terminal state
		if videoDoc.Status == model.STATUS_COMPLETED || videoDoc.Status == model.STATUS_FAILED {
			span.AddEvent("firestore.idempotency_guard.triggered", trace.WithAttributes(
				attribute.String("db.document.id", videoID),
				attribute.String("current_status", videoDoc.Status),
			))

			result = &CompletedResult{
				UserID:             videoDoc.UserID,
				OutputManifestPath: videoDoc.OutputManifestPath,
				AlreadyProcessed:   true,
			}
			return nil
		}

		if videoDoc.Status != model.STATUS_TRANSCODING {
			span.AddEvent("firestore.invalid_transition", trace.WithAttributes(
				attribute.String("db.document.id", videoID),
				attribute.String("current_status", videoDoc.Status),
				attribute.String("target_status", model.STATUS_COMPLETED),
			))
			return ErrInvalidTransition
		}

		outputManifestPath := fmt.Sprintf("gs://%s/%s/%s/manifest.m3u8", processedBucket, videoDoc.UserID, videoID)

		firestoreUpdates := []firestore.Update{
			{Path: StatusField, Value: model.STATUS_COMPLETED},
			{Path: UpdatedAtField, Value: firestore.ServerTimestamp},
			{Path: OutputManifestPathField, Value: outputManifestPath},
		}

		if err := tx.Update(docRef, firestoreUpdates); err != nil {
			return fmt.Errorf("firestore update document %s: %w", videoID, err)
		}

		span.AddEvent("firestore.document.updated", trace.WithAttributes(
			attribute.String("db.document.id", videoID),
			attribute.String("new_status", model.STATUS_COMPLETED),
			attribute.String("output_manifest_path", outputManifestPath),
		))

		result = &CompletedResult{
			UserID:             videoDoc.UserID,
			OutputManifestPath: outputManifestPath,
			AlreadyProcessed:   false,
		}

		return nil
	})

	if err != nil {
		span.SetStatus(codes.Error, "firestore transaction failed")
		span.RecordError(err)

		log.Error("Firestore transaction failed for TransitionToCompleted",
			zap.String("component", "repository.firestore"),
			zap.String("action", "transition_to_completed"),
			zap.String("videoId", videoID),
			zap.Error(err))

		return nil, err
	}

	span.SetStatus(codes.Ok, "firestore transaction committed")
	span.AddEvent("firestore.transaction.committed")

	log.Info("Firestore transaction committed for TransitionToCompleted",
		zap.String("component", "repository.firestore"),
		zap.String("action", "transition_to_completed"),
		zap.String("videoId", videoID),
		zap.Bool("alreadyProcessed", result != nil && result.AlreadyProcessed))

	return result, nil
}

func (r *FirestoreRepo) TransitionToFailed(ctx context.Context, videoID string, errorCode int, errorMessage string) (*FailedResult, error) {
	tracer := otel.Tracer(repoTracerName)

	ctx, span := tracer.Start(ctx, "repository.TransitionToFailed",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "firestore"),
			attribute.String("db.operation", "transaction"),
			attribute.String("db.collection", r.collection),
			attribute.String("db.document.id", videoID),
			attribute.Int("job.error.code", errorCode),
		),
	)
	defer span.End()

	log := config.LoggerWithTrace(ctx, r.logger, r.projectID)

	var result *FailedResult

	docRef := r.client.Collection(r.collection).Doc(videoID)

	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		span.AddEvent("firestore.transaction.started")

		docSnap, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == grpccodes.NotFound {
				span.AddEvent("firestore.document.not_found", trace.WithAttributes(
					attribute.String("db.document.id", videoID),
				))
				return ErrDocumentNotFound
			}
			return fmt.Errorf("firestore get document %s: %w", videoID, err)
		}

		span.AddEvent("firestore.document.read", trace.WithAttributes(
			attribute.String("db.document.id", videoID),
		))

		var videoDoc *model.VideoDocument
		if err := docSnap.DataTo(&videoDoc); err != nil {
			return fmt.Errorf("firestore data to struct for document %s: %w", videoID, err)
		}

		if videoDoc.Status == model.STATUS_COMPLETED || videoDoc.Status == model.STATUS_FAILED {
			span.AddEvent("firestore.idempotency_guard.triggered", trace.WithAttributes(
				attribute.String("db.document.id", videoID),
				attribute.String("current_status", videoDoc.Status),
			))

			result = &FailedResult{
				UserID:           videoDoc.UserID,
				AlreadyProcessed: true,
			}
			return nil
		}

		if videoDoc.Status != model.STATUS_TRANSCODING {
			span.AddEvent("firestore.invalid_transition", trace.WithAttributes(
				attribute.String("db.document.id", videoID),
				attribute.String("current_status", videoDoc.Status),
				attribute.String("target_status", model.STATUS_FAILED),
			))
			return ErrInvalidTransition
		}

		firestoreUpdates := []firestore.Update{
			{Path: StatusField, Value: model.STATUS_FAILED},
			{Path: UpdatedAtField, Value: firestore.ServerTimestamp},
			{Path: ErrorCodeField, Value: errorCode},
			{Path: ErrorMessageField, Value: errorMessage},
		}

		if err := tx.Update(docRef, firestoreUpdates); err != nil {
			return fmt.Errorf("firestore update document %s: %w", videoID, err)
		}

		span.AddEvent("firestore.document.updated", trace.WithAttributes(
			attribute.String("db.document.id", videoID),
			attribute.String("new_status", model.STATUS_FAILED),
			attribute.Int("job.error.code", errorCode),
		))

		result = &FailedResult{
			UserID:           videoDoc.UserID,
			AlreadyProcessed: false,
		}

		return nil
	})

	if err != nil {
		span.SetStatus(codes.Error, "firestore transaction failed")
		span.RecordError(err)

		log.Error("Firestore transaction failed for TransitionToFailed",
			zap.String("component", "repository.firestore"),
			zap.String("action", "transition_to_failed"),
			zap.String("videoId", videoID),
			zap.Int("errorCode", errorCode),
			zap.String("errorMessage", errorMessage),
			zap.Error(err))

		return nil, err
	}

	span.SetStatus(codes.Ok, "firestore transaction committed")
	span.AddEvent("firestore.transaction.committed")

	log.Info("Firestore transaction committed for TransitionToFailed",
		zap.String("component", "repository.firestore"),
		zap.String("action", "transition_to_failed"),
		zap.String("videoId", videoID),
		zap.Bool("alreadyProcessed", result != nil && result.AlreadyProcessed))

	return result, nil
}

var _ FirestoreRepository = (*FirestoreRepo)(nil)
