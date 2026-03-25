output "processed_videos_bucket_name" {
  description = "The name of the GCS bucket where processed videos will be stored."
  value       = data.google_storage_bucket.apex_dev_processed_videos.name
}
