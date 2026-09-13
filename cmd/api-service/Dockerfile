# Serves both api-service (verifier, default) and api-service-issuer
# (APP_MODE=issuer) — one image, two Deployments. See
# ../../04-infra-devops/containerization-design.md.

FROM golang:1.27.1-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG COMMIT_SHA=unknown
RUN CGO_ENABLED=0 go build -ldflags "-X loginid-takehome/internal/app.Commit=${COMMIT_SHA}" -o /out/api-service ./cmd/api-service

FROM gcr.io/distroless/static-debian12:nonroot AS final
COPY --from=builder /out/api-service /api-service
USER nonroot:nonroot
ENTRYPOINT ["/api-service"]
