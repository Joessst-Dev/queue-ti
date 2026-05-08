FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.work go.work.sum ./
COPY backend/go.mod backend/go.sum ./backend/
COPY clients/go-client/go.mod clients/go-client/go.sum ./clients/go-client/
COPY tools/seeder/go.mod tools/seeder/go.sum ./tools/seeder/

RUN go work sync

COPY backend/ ./backend/
COPY clients/go-client/ ./clients/go-client/
COPY tools/seeder/ ./tools/seeder/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /seeder ./tools/seeder

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /seeder /seeder
ENTRYPOINT ["/seeder"]
