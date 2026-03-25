data "google_storage_bucket" "apex_dev_processed_videos" {
  name = var.processed_videos_bucket_name
}
