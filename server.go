package main

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/memcache"
)

type S struct {
	cache     *memcache.CacheType
	userAgent string
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
	log.Println("Domains in memcache: ", s.cache.LenSafe())
}

func (s *S) UA(w http.ResponseWriter, r *http.Request) {
	message := &JSONResponse{}
	message.Code = http.StatusOK
	message.Message = s.userAgent
	data, err := json.Marshal(message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}
	w.Header().Add("Content-type", "application/json")
	w.Write(data)
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
	log.Println("Popped", item)
	return item
}
