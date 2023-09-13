# Build the manager binary
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.19.2 as builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

RUN apt-get update && apt-get install -y ca-certificates openssl \ 
    && openssl s_client -showcerts -connect github.com:443 </dev/null 2>/dev/null|openssl x509 -outform PEM > /usr/local/share/ca-certificates/github.crt \
    && openssl s_client -showcerts -connect proxy.golang.org:443 </dev/null 2>/dev/null|openssl x509 -outform PEM >  /usr/local/share/ca-certificates/proxy.golang.crt \
    && update-ca-certificates

WORKDIR /code

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o app main.go

FROM --platform=${BUILDPLATFORM:-linux/amd64} gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /code/app .
USER 65532:65532

ENTRYPOINT ["/app"]
