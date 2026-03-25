resource "google_cloud_run_v2_service" "apex_ingestion_service" {
  name     = var.service_name
  location = var.project_region

  scaling {
    min_instance_count = var.min_instance_count
    max_instance_count = var.max_instance_count
  }



  template {

    service_account = var.service_account_name
    containers {
      image = var.container_image

      resources {
        limits = {
          memory = var.memory_limit
          cpu    = var.cpu_limit
        }
      }

      env {
        name  = "APP_ENV"
        value = var.app_env
      }
      env {
        name  = "GCP_PROJECT_REGION"
        value = var.project_region
      }
      env {
        name  = "HTTP_PORT"
        value = var.http_port
      }
      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "GCS_BUCKET"
        value = var.gcs_bucket
      }
      env {
        name  = "OTEL_SERVICE_NAME"
        value = var.otel_service_name
      }
      env {
        name  = "OTEL_EXPORTER_OTLP_ENDPOINT"
        value = var.otel_exporter_otlp_endpoint
      }
      env {
        name  = "OTEL_EXPORTER_OTLP_HEADERS"
        value = var.otel_exporter_otlp_headers
      }
      env {
        name  = "PUBSUB_SUBSCRIPTION_ID"
        value = var.pubsub_subscription_id
      }
      env {
        name  = "MAX_OUTSTANDING_MESSAGES"
        value = var.max_outstanding_messages
      }
      env {
        name  = "FIRESTORE_COLLECTION_NAME"
        value = var.firestore_collection
      }
      env {
        name  = "FIRESTORE_DATABASE_ID"
        value = var.firestore_database_id
      }
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}
