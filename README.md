# idun
[DomainsProject.org](https://domainsproject.org) HTTP worker


WORK IN PROGRESS


## Docker compose way (recommended)

1. Create `.env` file with contents like this: `FREYA=123` where `123` is your API key.
2. Run `./start.sh` (will invoke docker-compose and start `1` containers)
3. Run `docker ps` to get container id
4. Run `docker logs -f container_id` to confirm proper functioning


## Docker run way (debugging)

1. `docker pull tb0hdan/idun`
2. `docker run --env FREYA=123 --rm tb0hdan/idun`
