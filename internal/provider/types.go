package provider

import "time"

type CredentialType int

func (ct CredentialType) String() string {
	switch ct {
	case Secret:
		return "Secret"
	case Cert:
		return "Cert"
	default:
		return "Unknown"
	}
}

const (
	Secret CredentialType = iota
	Cert
)

type Credential struct {
	Type    CredentialType // Secret | Cert — your enum, computed from which array it came from
	KeyID   string
	Name    string
	Expires time.Time
}

type Application struct {
	ID          string // appId
	DisplayName string
	Credentials []Credential // merged: passwords + certs, ready for report
}
