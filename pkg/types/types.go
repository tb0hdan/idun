package types

import (
	"os"
	"time"
)

const (
	OneK            = 1 << 10
	OneMeg          = 1 << 20
	TwoGigs         = OneGig * 2
	QuarterGig      = 256 * OneMeg
	HalfGig         = QuarterGig * 2
	OneGig          = HalfGig * 2
	MaxDomainsInMap = 256
	TickEvery       = 10 * time.Second
	Parallelism     = 2
	RandomDelay     = 60 * time.Second
	APIRetryMax     = 3
	//
	ReadTimeout  = 30 * time.Second
	WriteTimeout = 30 * time.Second
	IdleTimeout  = 60 * time.Second
	//
	GetDomainsRetry = 60 * time.Second
	// process control.
	CrawlerExtra     = 10 * time.Second
	KillSleep        = 3 * time.Second
	CrawlFilterRetry = 60 * time.Second
	HeadCheckTimeout = 10 * time.Second
	// process limits.
	CrawlerMaxRunTime = 600 * time.Second
)

var (
	FreyaKey = os.Getenv("FREYA")                      // nolint:gochecknoglobals
	APIBase  = "https://api.domainsproject.org/api/vo" // nolint:gochecknoglobals
)

type DomainsResponse struct {
	Domains []string `json:"domains"`
}

type JSONResponse struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}
