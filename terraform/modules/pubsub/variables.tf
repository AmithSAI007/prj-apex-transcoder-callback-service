variable "apex_transcoder_subscription_name" {
  description = "The name of the Pub/Sub subscription to monitor for new video upload notifications."
  type        = string
  default     = "apex.video-processing.transcoder-api.job-completed.callback-service"
}
