# Build the manager binary
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.19.2 as builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

WORKDIR /code

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o app main.go

FROM --platform=${BUILDPLATFORM:-linux/amd64} gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /code/app .
USER 65532:65532

ENTRYPOINT ["/app"]
