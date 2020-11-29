package webserver

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type WebServer struct {
	// config
	address      string
	readTimeout  time.Duration
	writeTimeout time.Duration
	idleTimeout  time.Duration
	// build info
	version   string
	goVersion string
	build     string
	buildDate string
	//
	router *mux.Router
}

func (ws *WebServer) Health(w http.ResponseWriter, r *http.Request) {
	data := fmt.Sprintf("Build info: version: %s, go: %s, hash: %s, date: %s\n",
		ws.version,
		ws.goVersion, ws.build,
		ws.buildDate,
	)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(data))
}

func (ws *WebServer) SetBuildInfo(version, goVersion, build, buildDate string) {
	ws.version = version
	ws.goVersion = goVersion
	ws.build = build
	ws.buildDate = buildDate
}

func (ws *WebServer) Run() {
	srv := http.Server{
		Addr:         ws.address,
		Handler:      ws.router,
		ReadTimeout:  ws.readTimeout,
		WriteTimeout: ws.writeTimeout,
		IdleTimeout:  ws.idleTimeout,
	}

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func New(address string, readTimeout, writeTimeout, idleTimeout time.Duration) *WebServer {
	ws := &WebServer{
		address:      address,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		idleTimeout:  idleTimeout,
	}
	r := mux.NewRouter()
	r.HandleFunc("/", ws.Health)
	r.HandleFunc("/health", ws.Health)
	ws.router = r

	return ws
}
