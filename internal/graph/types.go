package graph

import "time"

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
