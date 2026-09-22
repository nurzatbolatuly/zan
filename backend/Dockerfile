# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src

# Кэшируем слой зависимостей отдельно от исходников — go.mod/go.sum меняются
# сильно реже, чем код.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# Финальный образ — distroless: без шелла и пакетного менеджера, меньше
# площадь атаки, чем alpine с оставленным busybox.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
USER 65532:65532
EXPOSE 8080
# distroless без shell/curl — проверка реализована в самом бинаре
# (cmd/api/healthcheck.go), не CMD-SHELL.
HEALTHCHECK --interval=5s --timeout=3s --retries=5 CMD ["/api", "healthcheck"]
ENTRYPOINT ["/api"]
