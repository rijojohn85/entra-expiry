package graph

import (
	"fmt"
	"time"
)

type APIError struct {
	Status     int
	URL        string // endpoint that failed
	Err        error
	RetryAfter time.Duration
}

func NewAPIError(status int, url string, err error, retryAfter time.Duration) APIError {
	return APIError{
		Status:     status,
		URL:        url,
		Err:        err,
		RetryAfter: retryAfter,
	}
}

func (e APIError) Error() string {
	return fmt.Sprintf("status: %d, url: %s, error: %v", e.Status, e.URL, e.Err)
}
func (e APIError) Unwrap() error { return e.Err }

type ApplicationsPage struct {
	Value    []GraphApplication `json:"value"`
	NextLink string             `json:"@odata.nextLink"` // paging; empty string = last page
}

type GraphApplication struct {
	AppID               string          `json:"appId"`
	DisplayName         string          `json:"displayName"`
	PasswordCredentials []GraphPassword `json:"passwordCredentials"` // can be null → nil slice, fine
	KeyCredentials      []GraphKey      `json:"keyCredentials"`
}

type GraphPassword struct {
	KeyID       string    `json:"keyId"`
	DisplayName string    `json:"displayName"`
	EndDateTime time.Time `json:"endDateTime"`
}

type GraphKey struct {
	KeyID       string    `json:"keyId"`
	DisplayName string    `json:"displayName"`
	EndDateTime time.Time `json:"endDateTime"`
}
