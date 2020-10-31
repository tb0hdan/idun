package main

import (
	"encoding/json"
	"net/http"

	"github.com/tb0hdan/memcache"
)

type S struct {
	cache *memcache.CacheType
}

func (s *S) UploadDomains(w http.ResponseWriter, r *http.Request) {
	var domainsResponse DomainsResponse

	err := json.NewDecoder(r.Body).Decode(&domainsResponse)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	if len(domainsResponse.Domains) == 0 {
		http.Error(w, "empty domain list", http.StatusInternalServerError)

		return
	}

	for _, domain := range domainsResponse.Domains {
		s.cache.Set(domain, "1")
	}
}

func (s *S) Pop() string {
	var item string

	if s.cache.LenSafe() == 0 {
		return ""
	}

	for k, _ := range s.cache.Cache() {
		item = k

		break
	}

	s.cache.Delete(item)

	return item
}
