package apiserver

import (
	"encoding/json"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/tb0hdan/idun/pkg/types"
	"github.com/tb0hdan/memcache"
)

type apiServer struct {
	Cache     *memcache.CacheType
	UserAgent string
	Expires   int64
}

func (s *apiServer) GetUA() string {
	return s.UserAgent
}

func (s *apiServer) UploadDomains(w http.ResponseWriter, r *http.Request) {
	var domainsResponse types.DomainsResponse

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
		s.Cache.SetEx(domain, "1", s.Expires)
	}

	log.Println("Domains in memcache: ", s.Cache.LenSafe())
}

func (s *apiServer) UA(w http.ResponseWriter, r *http.Request) {
	message := &types.JSONResponse{}
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

func (s *apiServer) Pop() string {
	var item string

	if s.Cache.LenSafe() == 0 {
		return ""
	}

	for k := range s.Cache.Cache() {
		if strings.HasPrefix(k, "conntrack_") {
			continue
		}
		item = k

		break
	}

	s.Cache.Delete(item)
	log.Println("Popped", item)

	return item
}

func NewAPIServer(cache *memcache.CacheType, ua string, expires int64) *apiServer {
	return &apiServer{
		Cache:     cache,
		UserAgent: ua,
		Expires:   expires,
	}
}
