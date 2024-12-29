package main

import (
	"BookClubBot/bot"
	"BookClubBot/config"
	"log"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	b := bot.NewBot(cfg)
	b.Run()
}
