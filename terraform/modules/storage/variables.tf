variable "processed_videos_bucket_name" {
  description = "The name of the GCS bucket where processed videos will be stored."
  type        = string
  default     = "apex-dev-gcs-processed-videos"
}
