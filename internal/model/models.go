package model

const (
	STATUS_PENDING_UPLOAD = "PENDING_UPLOAD"
	STATUS_QUEUED         = "QUEUED"
	STATUS_TRANSCODING    = "TRANSCODING"
	STATUS_COMPLETED      = "COMPLETED"
	STATUS_FAILED         = "FAILED"
)

type JobResult struct {
	Job JobInfo `json:"job"`
}

type JobInfo struct {
	Name  string    `json:"name"`
	State string    `json:"state"`
	Error *JobError `json:"error,omitempty"`
}

type JobError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type PipelineEvent struct {
	VideoID           string `json:"videoId"`
	UserID            string `json:"userId"`
	Status            string `json:"status"`
	OuputManifestPath string `json:"outputManifestPath,omitempty"`
	ErrorCode         int    `json:"errorCode,omitempty"`
	ErrorMessage      string `json:"errorMessage,omitempty"`
	CompletedAt       string `json:"completedAt,omitempty"`
}

type VideoDocument struct {
	VideoID            string `json:"videoId" firestore:"videoId"`
	UserID             string `json:"userId" firestore:"userId"`
	Status             string `json:"status" firestore:"status"`
	RawFilePath        string `json:"rawFilePath" firestore:"rawFilePath"`
	FileSize           int64  `json:"fileSize" firestore:"fileSize"`
	ContentType        string `json:"contentType" firestore:"contentType"`
	FileHash           string `json:"fileHash" firestore:"fileHash"`
	TranscoderJobName  string `json:"transcoderJobName" firestore:"transcoderJobName,omitempty"`
	OutputManifestPath string `json:"outputManifestPath" firestore:"outputManifestPath,omitempty"`
	ErrorCode          int    `json:"errorCode,omitempty" firestore:"errorCode,omitempty"`
	ErrorMessage       string `json:"errorMessage,omitempty" firestore:"errorMessage,omitempty"`
	CreatedAt          string `json:"createdAt" firestore:"createdAt"`
	UpdatedAt          string `json:"updatedAt" firestore:"updatedAt"`
}

type MetaData struct {
	EventType string `json:"event_type"`
	Source    string `json:"source"`
	ID        string `json:"id"`
	Subject   string `json:"subject"`
	EventTime string `json:"event_time"`
	TraceID   string `json:"trace_id"`
}
