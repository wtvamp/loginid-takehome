# Migration runner image: goose CLI + this repo's migration files.
# Not one of the two application images — built and pushed separately,
# used only by the migration Job (deploy/manifests.yaml), never by
# api-service/idp-connector.
FROM golang:1.27.1-alpine AS builder
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.22.1

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY migrations /migrations
ENTRYPOINT ["/usr/local/bin/goose"]
