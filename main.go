package main

import (
	"BookClubBot/bot"
	"BookClubBot/config"
	"BookClubBot/message"
	"log"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	msg, err := message.LoadMessaged()
	if err != nil {
		log.Fatal(err)
	}

	b := bot.NewBot(cfg, msg)
	b.Run()
}
