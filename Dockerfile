FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -o /gg ./cmd/main.go

FROM alpine:3.19

RUN apk add --no-cache git openssh-client ca-certificates

RUN for id in $(seq 1000 2000); do adduser -D -u "$id" -h / "u$id" ; done
ENV SSH_AUTH_SOCK=/.SSH_AUTH_SOCK

COPY --from=builder /gg /usr/local/bin/gg

WORKDIR /data
ENTRYPOINT ["gg"]
