FROM golang:1.26.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X dispatch/internal/buildinfo.Version=${VERSION}" -o /out/dispatch-api ./cmd/api \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X dispatch/internal/buildinfo.Version=${VERSION}" -o /out/dispatch-worker ./cmd/worker

FROM alpine:3.23 AS runtime
RUN addgroup -S dispatch && adduser -S -G dispatch dispatch \
    && apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/dispatch-api /usr/local/bin/dispatch-api
COPY --from=build /out/dispatch-worker /usr/local/bin/dispatch-worker
USER dispatch
EXPOSE 8080
