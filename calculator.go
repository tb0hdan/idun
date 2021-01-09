package main

import (
	"runtime"

	sigar "github.com/cloudfoundry/gosigar"
)

const (
	MaxPerCore = 16
	MaxPerGig  = 4
)

func CalculateMaxWorkers() (int64, error) {
	maxAllowed := int64(1)
	mem := sigar.Mem{}
	err := mem.Get()
	if err != nil {
		return 0, err
	}

	cpus := runtime.NumCPU()
	cpuMax := int64(cpus * MaxPerCore)
	gigs := mem.ActualFree / OneGig
	memMax := int64(gigs * MaxPerGig)

	if cpuMax > memMax || cpuMax == memMax {
		maxAllowed = memMax
	}

	if memMax > cpuMax {
		maxAllowed = cpuMax
	}

	if maxAllowed > MaxDomainsInMap {
		maxAllowed = MaxDomainsInMap
	}

	return maxAllowed, nil
}
