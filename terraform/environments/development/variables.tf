variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project."
}

variable "project_region" {
  type        = string
  description = "The GCP region where resources will be deployed."
  default     = "us-central1"
}

variable "container_image" {
  type        = string
  description = "The container image to be used for the Cloud Run service."
}

variable "service_account_name" {
  type        = string
  description = "The name of the service account to be used by the Cloud Run service."
}

variable "memory_limit" {
  type        = string
  description = "The memory limit for the Cloud Run service."
  default     = "512Mi"
}

variable "cpu_limit" {
  type        = string
  description = "The CPU limit for the Cloud Run service."
  default     = "1"
}

variable "min_instance_count" {
  type        = number
  description = "The minimum number of instances for the Cloud Run service."
  default     = 0
}

variable "max_instance_count" {
  type        = number
  description = "The maximum number of instances for the Cloud Run service."
  default     = 3
}

variable "app_env" {
  type        = string
  description = "The application environment (e.g., development, staging, production)."
  default     = "development"
}

variable "max_outstanding_messages" {
  type        = string
  description = "The maximum number of outstanding Pub/Sub messages for processing."
  default     = "10"
}

variable "otel_exporter_otlp_endpoint" {
  type        = string
  description = "The endpoint URL for the OpenTelemetry OTLP exporter."
}

variable "otel_exporter_otlp_headers" {
  type        = string
  description = "Headers to include in OpenTelemetry OTLP exporter requests."
}

variable "firestore_database_id" {
  type        = string
  description = "The ID of the Firestore database for storing transcoding job metadata."
}
