package types

import (
	"net/http"

	log "github.com/sirupsen/logrus"
)

type APIClientInterface interface {
	GetUA(uaURL string) (string, error)
	GetDomains() ([]string, error)
	FilterDomains(incoming []string) (outgoing []string, err error)
	Fatal(args ...interface{})
	Debugf(format string, args ...interface{})
	GetLogger() *log.Logger
}

type WorkerCalculator interface {
	CalculateMaxWorkers() (int64, error)
}

type APIServerInterface interface {
	UploadDomains(w http.ResponseWriter, r *http.Request)
	UA(w http.ResponseWriter, r *http.Request)
	Pop() string
	GetUA() string
}
