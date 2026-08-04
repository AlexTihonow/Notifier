package main

import (
	"log"

	notifier "my-mailer/internal"
	"my-mailer/internal/configs"
)

func main () {
	conf, err := configs.LoadConfig()
	if err != nil {
		log.Fatalf("ошибка настроек: %v", err)
	}

	notifier.Run(conf)
}