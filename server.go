package idun

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
	"github.com/tb0hdan/memcache"
)

type S struct {
	Cache     *memcache.CacheType
	UserAgent string
}

func (s *S) UploadDomains(w http.ResponseWriter, r *http.Request) {
	var domainsResponse DomainsResponse

	err := json.NewDecoder(r.Body).Decode(&domainsResponse)
	if err != nil {
		log.Error("Upload error: ", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	if len(domainsResponse.Domains) == 0 {
		log.Error("Upload error: empty domain list")
		http.Error(w, "empty domain list", http.StatusInternalServerError)

		return
	}

	for _, domain := range domainsResponse.Domains {
		s.Cache.Set(domain, "1")
	}

	log.Println("Domains in memcache: ", s.Cache.LenSafe())
}

func (s *S) UA(w http.ResponseWriter, r *http.Request) {
	message := &JSONResponse{}
	message.Code = http.StatusOK
	message.Message = s.UserAgent
	data, err := json.Marshal(message)
	//
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.Header().Add("Content-type", "application/json")
	_, _ = w.Write(data)
}

func (s *S) Pop() string {
	var item string

	if s.Cache.LenSafe() == 0 {
		return ""
	}

	for k := range s.Cache.Cache() {
		item = k

		break
	}

	s.Cache.Delete(item)
	log.Println("Popped", item)

	return item
}
