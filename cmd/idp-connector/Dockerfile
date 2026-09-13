FROM golang:1.27.1-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG COMMIT_SHA=unknown
RUN CGO_ENABLED=0 go build -ldflags "-X loginid-takehome/internal/app.Commit=${COMMIT_SHA}" -o /out/idp-connector ./cmd/idp-connector

# distroless base (not -static): idp-connector needs CA certs for outbound
# TLS to vendors, per ../../04-infra-devops/containerization-design.md §1.
FROM gcr.io/distroless/base-debian12:nonroot AS final
COPY --from=builder /out/idp-connector /idp-connector
USER nonroot:nonroot
ENTRYPOINT ["/idp-connector"]
