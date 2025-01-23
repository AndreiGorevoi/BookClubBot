package main

import (
	"BookClubBot/bot"
	"BookClubBot/config"
	"BookClubBot/message"
	"BookClubBot/repository"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	logFile, err := os.OpenFile("application.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

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
