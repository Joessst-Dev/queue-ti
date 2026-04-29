FROM golang:1.25-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG BUILD_VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${BUILD_VERSION:-dev}" -o /queue-ti ./cmd/server
RUN printf 'package main\nimport("net/http";"os")\nfunc main(){r,e:=http.Get("http://localhost:8080/healthz");if e!=nil||r.StatusCode!=200{os.Exit(1)}}' > /tmp/hc.go && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /healthcheck /tmp/hc.go

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /queue-ti /queue-ti
COPY --from=builder /healthcheck /healthcheck
COPY config.yaml /config.yaml

EXPOSE 50051

ENTRYPOINT ["/queue-ti"]

