FROM golang:1.10.2 AS builder

RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/v0.4.1/dep-linux-amd64 && chmod +x /usr/local/bin/dep

RUN mkdir -p /go/src/app
WORKDIR /go/src/app
ADD . . 

RUN dep ensure -vendor-only
# install the dependencies without checking for go code

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .
# Static build required so that we can safely copy the binary over.

FROM alpine:latest as alpine
RUN apk --no-cache add tzdata zip ca-certificates
WORKDIR /usr/share/zoneinfo
# -0 means no compression.  Needed because go's
# tz loader doesn't handle compressed data.
RUN zip -r -0 /zoneinfo.zip .




FROM scratch

COPY --from=builder /go/src/app/main /

# the timezone data:
COPY --from=alpine /zoneinfo.zip /
# the tls certificates:
COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
  
ENV ZONEINFO=/zoneinfo.zip \
  HTTPPORT="80" \
  PUBSUB_VERIFICATION_TOKEN="ABCDEFGHIJKLMN123456789" \
  REDISHOST="10.0.0.3" \
  REDISPORT="6379"

EXPOSE 80/tcp

CMD ["/main"]