package main

import (
	"net/http"
	"strings"
	"time"
)

// runHealthcheck — режим `api healthcheck`: используется docker
// HEALTHCHECK/compose healthcheck для backend-контейнера. Финальный образ —
// distroless (Dockerfile), в нём нет ни shell, ни curl/wget, поэтому
// проверка "жив ли процесс" реализована прямо в бинаре, а не как отдельная
// CMD-SHELL команда.
func runHealthcheck(httpAddr string) int {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthcheckURL(httpAddr))
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }() // тело не читаем и не используем дальше — ошибка закрытия здесь не влияет на итог проверки

	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// healthcheckURL превращает HTTP_ADDR (например ":8080" — обычная форма
// для net/http.Server.Addr, слушающего на всех интерфейсах) в URL,
// пригодный для запроса к самому себе изнутри контейнера. Вынесена отдельно
// от runHealthcheck, чтобы протестировать построение URL без реального
// сетевого вызова.
func healthcheckURL(httpAddr string) string {
	host := httpAddr
	if strings.HasPrefix(httpAddr, ":") {
		host = "localhost" + httpAddr
	}
	return "http://" + host + "/healthz"
}
