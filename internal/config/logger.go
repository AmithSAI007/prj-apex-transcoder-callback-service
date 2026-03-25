package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// GCP Cloud Logging special JSON fields.
// When these keys appear in a structured log entry, Cloud Logging
// recognises them and populates the corresponding LogEntry fields.
// This is critical for:
//   - Log-trace correlation in Cloud Trace
//   - Querying audit logs in BigQuery via the log router/sink
const (
	gcpTraceKey   = "logging.googleapis.com/trace"
	gcpSpanIDKey  = "logging.googleapis.com/spanId"
	gcpSampledKey = "logging.googleapis.com/trace_sampled"
	gcpLabelsKey  = "logging.googleapis.com/labels"
)

// NewLogger creates a configured zap.Logger based on the APP_ENV environment
// variable. In non-local modes it outputs structured JSON to stdout with
// GCP Cloud Logging compatible field names (severity, trace, spanId).
// In "local" mode it outputs colorized, human-readable console logs.
//
// The serviceName parameter is added as a base field to every log entry,
// making it possible to distinguish logs from different services when they
// are routed to a shared audit sink (e.g., BigQuery via log router).
func NewLogger(appEnv string, serviceName string) (*zap.Logger, error) {
	var logger *zap.Logger
	var err error

	if appEnv != "local" {
		// Production/staging: structured JSON format to stdout only.
		// Cloud Run captures stdout; writing to ephemeral files is avoided.
		cfg := zap.NewProductionConfig()
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stderr"}

		// Map Zap's level key to GCP's "severity" so Cloud Logging parses it correctly.
		cfg.EncoderConfig.LevelKey = "severity"
		cfg.EncoderConfig.EncodeLevel = func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
			severity := strings.ToUpper(l.CapitalString())
			if severity == "WARN" {
				severity = "WARNING"
			}
			enc.AppendString(severity)
		}

		// Use "message" as the message key for consistency with GCP structured logging.
		cfg.EncoderConfig.MessageKey = "message"
		cfg.EncoderConfig.TimeKey = "timestamp"
		cfg.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder

		logger, err = cfg.Build(
			zap.Fields(zap.String("service", serviceName)),
		)
	} else {
		// Development: colorized console output with RFC3339 timestamps for readability.
		config := zap.NewDevelopmentEncoderConfig()
		config.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)
		config.EncodeLevel = zapcore.CapitalColorLevelEncoder

		logger = zap.New(zapcore.NewCore(
			zapcore.NewConsoleEncoder(config),
			zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)),
			zap.InfoLevel,
		),
			zap.Fields(zap.String("service", serviceName)),
		)
	}

	if err != nil {
		return nil, err
	}

	return logger, nil
}

// LoggerWithTrace returns a child logger enriched with GCP Cloud Logging
// trace correlation fields extracted from the given context. When these
// logs are ingested by Cloud Logging and routed to BigQuery, the trace
// and span IDs allow joining log rows with Cloud Trace spans for full
// audit trail reconstruction.
//
// Fields added (only when a valid span context exists):
//   - logging.googleapis.com/trace       — full trace resource name
//   - logging.googleapis.com/spanId      — hex-encoded span ID
//   - logging.googleapis.com/trace_sampled — whether the trace was sampled
func LoggerWithTrace(ctx context.Context, logger *zap.Logger, projectID string) *zap.Logger {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return logger
	}

	traceID := sc.TraceID().String()
	spanID := sc.SpanID().String()

	return logger.With(
		zap.String(gcpTraceKey, fmt.Sprintf("projects/%s/traces/%s", projectID, traceID)),
		zap.String(gcpSpanIDKey, spanID),
		zap.Bool(gcpSampledKey, sc.IsSampled()),
	)
}
