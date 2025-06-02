# Build the manager binary
FROM --platform=${BUILDPLATFORM:-linux/amd64} docker.io/golang:1.24.3 AS builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

WORKDIR /code

COPY pkg pkg
COPY cmd cmd
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o app ./cmd/app/main.go

FROM --platform=${BUILDPLATFORM:-linux/amd64} gcr.io/distroless/static:nonroot
LABEL org.opencontainers.image.source="https://github.com/jmcgrath207/k8s-ephemeral-storage-metrics"
WORKDIR /
COPY --from=builder /code/app .
USER 65532:65532

ENTRYPOINT ["/app"]
