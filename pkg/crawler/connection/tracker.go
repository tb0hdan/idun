package connection

import (
	"fmt"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/memcache"
)

const (
	MaxConnectionsPerFiveMinutes = 32
	FiveMinutesInSeconds         = 300
)

type Tracker struct {
	cache  *memcache.CacheType
	logger *log.Logger
}

func (t *Tracker) Check(domainName string) bool {
	addrs, err := net.LookupIP(domainName)
	if err != nil {
		return false
	}
	if len(addrs) == 0 {
		return false
	}
	// use cache
	for _, addr := range addrs {
		ipAddr := addr.String()
		value, ok := t.cache.GetEx(fmt.Sprintf("conntrack_%s", ipAddr))
		connectionCount := int64(1)
		expirationDiff := int64(FiveMinutesInSeconds)
		if ok {
			currentConnectionCount := value.Value.(int64)
			if currentConnectionCount > MaxConnectionsPerFiveMinutes {
				return false
			}
			connectionCount = currentConnectionCount + 1
			expirationDiff = value.Expires - time.Now().Unix()
		}
		t.cache.SetEx(fmt.Sprintf("conntrack_%s", ipAddr), connectionCount, expirationDiff)
	}
	//
	return true
}

func New(cache *memcache.CacheType, logger *log.Logger) *Tracker {
	return &Tracker{cache: cache, logger: logger}
}
