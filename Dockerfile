FROM golang:1.26.1-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -o /gitgit ./cmd/main.go

FROM alpine:3.19

RUN apk add --no-cache git openssh-client ca-certificates

RUN for id in $(seq 1000 2000); do adduser -D -u "$id" -h / "u$id" ; done
ENV SSH_AUTH_SOCK=/.SSH_AUTH_SOCK

COPY --from=builder /gitgit /usr/local/bin/gitgit

WORKDIR /data
ENTRYPOINT ["gitgit"]
