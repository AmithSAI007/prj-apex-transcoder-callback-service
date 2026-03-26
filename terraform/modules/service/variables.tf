variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project for resource organization and billing."
  validation {
    condition     = length(var.project_id) > 0
    error_message = "The project_id must not be empty."
  }
}

variable "project_region" {
  type        = string
  description = "The GCP region where the resources will be deployed, impacting latency and compliance."
  validation {
    condition     = length(var.project_region) > 0
    error_message = "The project_region must be specified."
  }
}

variable "service_name" {
  description = "The name of the Cloud Run service."
  type        = string
  default     = "prj-apex-transcoder-callback-service"
}

variable "container_image" {
  description = "The container image to be used for the Cloud Run service."
  type        = string
}

variable "min_instance_count" {
  description = "The minimum number of instances for the Cloud Run service."
  type        = number
}

variable "max_instance_count" {
  description = "The maximum number of instances for the Cloud Run service."
  type        = number
}

variable "memory_limit" {
  description = "The memory limit for the Cloud Run service."
  type        = string
}

variable "cpu_limit" {
  description = "The CPU limit for the Cloud Run service."
  type        = string
}

variable "app_env" {
  description = "The application environment (e.g., development, staging, production)."
  type        = string
}

variable "http_port" {
  description = "The HTTP port for the Cloud Run service."
  type        = string
  default     = ":8080"
}

variable "gcs_bucket" {
  description = "The GCS bucket where uploaded objects are stored."
  type        = string
}

variable "otel_service_name" {
  description = "The logical service name reported to the OpenTelemetry collector."
  type        = string
  default     = "prj-apex-transcode-submitter"
}

variable "otel_exporter_otlp_endpoint" {
  description = "The endpoint URL for the OpenTelemetry Protocol (OTLP) exporter to send telemetry data."
  type        = string
}

variable "firestore_collection" {
  description = "The name of the Firestore collection where transcoding job metadata will be stored."
  type        = string
  default     = "videos"
}

variable "firestore_database_id" {
  description = "The ID of the Firestore database to use for storing transcoding job metadata."
  type        = string
}

variable "service_account_name" {
  description = "The name of the service account to be used by the Cloud Run service."
  type        = string
}

variable "otel_exporter_otlp_headers" {
  description = "A comma-separated list of key=value pairs to be included as headers in OpenTelemetry OTLP exporter requests."
  type        = string
}

variable "pubsub_subscription_id" {
  description = "The ID of the Pub/Sub subscription that the Cloud Run service will pull messages from for transcoding tasks."
  type        = string
}

variable "max_outstanding_messages" {
  description = "The maximum number of outstanding messages that the Cloud Run service will pull from the Pub/Sub subscription for processing."
  type        = string
}
