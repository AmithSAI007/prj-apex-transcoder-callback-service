package repository

import (
	"context"
	"errors"

	"github.com/AmithSAI007/prj-apex-transcoder-callback-service/internal/model"
)

var (
	ErrDocumentNotFound  = errors.New("document not found")
	ErrInvalidTransition = errors.New("invalid state transition")
)

type CompletedResult struct {
	UserID             string
	OutputManifestPath string
	AlreadyProcessed   bool
}

type FailedResult struct {
	UserID           string
	AlreadyProcessed bool
}

type FirestoreRepository interface {
	TransitionToCompleted(ctx context.Context, videoID string, processedBucket string) (*CompletedResult, error)
	TransitionToFailed(ctx context.Context, videoID string, errorCode int, errorMessage string) (*FailedResult, error)
}

type PubSubPublisher interface {
	PublishPipelineEvent(ctx context.Context, event *model.PipelineEvent) error
}
