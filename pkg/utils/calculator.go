package utils

import (
	"runtime"

	"github.com/tb0hdan/idun/pkg/types"

	sigar "github.com/cloudfoundry/gosigar"
)

const (
	MaxPerCore = 32
	MaxPerGig  = 8
)

type Calculator struct {
	OvercommitRatio int64
}

func (c *Calculator) CalculateMaxWorkers() (int64, error) {
	maxAllowed := int64(1)
	mem := sigar.Mem{}
	err := mem.Get()
	if err != nil {
		return 0, err
	}

	cpus := runtime.NumCPU()
	cpuMax := int64(cpus * MaxPerCore)
	gigs := mem.ActualFree / types.OneGig
	memMax := int64(gigs * MaxPerGig)

	if cpuMax > memMax || cpuMax == memMax {
		maxAllowed = memMax
	}

	if memMax > cpuMax {
		maxAllowed = cpuMax
	}

	if c.OvercommitRatio > 1 {
		maxAllowed = maxAllowed * c.OvercommitRatio
	}

	if maxAllowed > types.MaxDomainsInMap {
		maxAllowed = types.MaxDomainsInMap
	}

	return maxAllowed, nil
}
