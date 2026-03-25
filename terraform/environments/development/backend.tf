terraform {
  backend "gcs" {
    bucket = "prj-apex-infra-terraform-state"
    prefix = "terraform/apex-transcoder-callback-service/state"
  }
}
