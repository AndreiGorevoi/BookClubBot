package main

import (
	"BookClubBot/bot"
	"BookClubBot/config"
	"BookClubBot/message"
	"BookClubBot/repository"
	"log"

	_ "modernc.org/sqlite"
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

	db, err := repository.InitDB("./db/book_club_bot.db")
	if err != nil {
		log.Fatal(err)
	}

	subRepository := repository.NewSubscriberRepository(db)
	metaRepository := repository.NewMetadataRepository(db)

	b := bot.NewBot(cfg, msg, subRepository, metaRepository)
	b.Run()
}
