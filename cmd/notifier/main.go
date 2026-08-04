package main

import (
	"log/slog"
	"os"

	notifier "my-mailer/internal"
	"my-mailer/internal/config"
)

func main () {
	conf, err := config.LoadConfig()
	if err != nil {
		slog.Error("ошибка настроек:", "err", err)
		os.Exit(1)
	}

	notifier.Run(conf)
}