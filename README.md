# idun
[DomainsProject.org](https://domainsproject.org) HTTP worker - [Docker image](https://hub.docker.com/r/tb0hdan/idun)


## Docker compose way (recommended)

1. Create `.env` file with contents like this: `FREYA=123` where `123` is your API key.
2. Run `./start.sh` (will invoke docker-compose and start `1` containers)
3. Run `docker ps` to get container id
4. Run `docker logs -f container_id` to confirm proper functioning

### Consul

Consul is available at http://host_ip:8500/

### Prometheus

Prometheus is available at http://host_ip:9090/

### Grafana

Grafana is available at http://host_ip:3000/dashboards -> Idun workers

Default credentials: `admin:admin`


## Docker run way (debugging)

1. `docker pull tb0hdan/idun`
2. `docker run --env FREYA=123 --rm tb0hdan/idun`


### Using with your own domains server


Server has to implement following methods:

```go
r.HandleFunc("/api/vo/ua", server.UserAgent).Methods(http.MethodGet)
r.HandleFunc("/api/vo/domains", server.GetDomains).Methods(http.MethodGet)
r.HandleFunc("/api/vo/filter", server.Filter).Methods(http.MethodPost)
```

server.UserAgent handler:

```go
type JSONResponse struct {
    Code    int64  `json:"code"`
    Message string `json:"message"`
}
```

with message containing User Agent. Code has to be http.StatusOK (i.e. 200)

server.GetDomains handler should marshal following structure:

```go
type DomainsJSON struct {
    Domains []string `json:"domains"`
}
```



server.Filter handler should accept DomainsJSON structure and return filtered out domains in DomainsJSON structure.




Running idun using custom server URL:

```
./idun -apiBase http://192.168.1.2:1234/api/vo
```
