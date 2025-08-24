package models

type TraceRequest struct {
	Resource     string `json:"resource"`
	Misconfig    string `json:"misconfig"`
	Account      string `json:"account"`
	Organization string `json:"organization"`
}

type GitHubIWebhook struct {
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}
