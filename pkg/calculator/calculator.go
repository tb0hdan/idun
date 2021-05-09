package calculator

import (
	"runtime"

	sigar "github.com/cloudfoundry/gosigar"
	"github.com/tb0hdan/idun/pkg/varstruct"
)

const (
	MaxPerCore = 32
	MaxPerGig  = 8
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
	gigs := mem.ActualFree / varstruct.OneGig
	memMax := int64(gigs * MaxPerGig)

	if cpuMax > memMax || cpuMax == memMax {
		maxAllowed = memMax
	}

	if memMax > cpuMax {
		maxAllowed = cpuMax
	}

	if maxAllowed > varstruct.MaxDomainsInMap {
		maxAllowed = varstruct.MaxDomainsInMap
	}

	return maxAllowed, nil
}
