FROM golang:alpine AS build-env
WORKDIR /
ADD . /
RUN apk update
RUN apk add gcc git make musl-dev
RUN apk add --no-cache ca-certificates apache2-utils
RUN make idun-docker


# final stage
FROM alpine
WORKDIR /
COPY --from=build-env /etc/ssl /etc/ssl
COPY --from=build-env /idun-docker /
CMD /idun-docker
EXPOSE 80
