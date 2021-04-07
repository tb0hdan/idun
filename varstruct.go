package idun

import (
	"os"
	"time"
)

const (
	OneK            = 1 << 10
	OneMeg          = 1 << 20
	HalfGig         = 512 * OneMeg
	OneGig          = 1 << 30
	TwoGigs         = OneGig * 2
	MaxDomainsInMap = 32
	TickEvery       = 10 * time.Second
	Parallelism     = 2
	RandomDelay     = 15 * time.Second
	APIRetryMax     = 3
	//
	ReadTimeout  = 30 * time.Second
	WriteTimeout = 30 * time.Second
	IdleTimeout  = 60 * time.Second
	//
	GetDomainsRetry = 60 * time.Second
	// process control.
	CrawlerExtra = 10 * time.Second
	KillSleep    = 3 * time.Second
)

var (
	FreyaKey = os.Getenv("FREYA")                      // nolint:gochecknoglobals
	APIBase  = "https://api.domainsproject.org/api/vo" // nolint:gochecknoglobals
	// Version Build info.
	Version   = "unset" // nolint:gochecknoglobals
	GoVersion = "unset" // nolint:gochecknoglobals
	Build     = "unset" // nolint:gochecknoglobals
	BuildDate = "unset" // nolint:gochecknoglobals
)
