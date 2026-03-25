module "storage" {
  source = "../../modules/storage"
}

module "pubsub" {
  source = "../../modules/pubsub"
}

module "service" {
  source                      = "../../modules/service"
  project_id                  = var.project_id
  project_region              = var.project_region
  container_image             = var.container_image
  service_account_name        = var.service_account_name
  memory_limit                = var.memory_limit
  cpu_limit                   = var.cpu_limit
  gcs_bucket                  = module.storage.processed_videos_bucket_name
  pubsub_subscription_id      = module.pubsub.apex_transcoder_subscription_id
  min_instance_count          = var.min_instance_count
  max_instance_count          = var.max_instance_count
  app_env                     = var.app_env
  max_outstanding_messages    = var.max_outstanding_messages
  otel_exporter_otlp_endpoint = var.otel_exporter_otlp_endpoint
  otel_exporter_otlp_headers  = var.otel_exporter_otlp_headers
  firestore_database_id       = var.firestore_database_id
}
