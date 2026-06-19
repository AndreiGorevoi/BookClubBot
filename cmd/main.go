package main

import (
	"BookClubBot/bot"
	"BookClubBot/config"
	"BookClubBot/internal/repository"
	"BookClubBot/message"
	"log"
	"os"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Log to stdout so the platform (Railway, Docker, etc.) captures logs.
	log.SetOutput(os.Stdout)

	msg, err := message.LoadMessaged()
	if err != nil {
		log.Fatal(err)
	}

	db, err := repository.InitMongoDB(cfg.MongoURI, cfg.DBName)
	if err != nil {
		log.Fatalf("error during initialisation of mongodb : '%v'", err)
	}

	subRepository, err := repository.NewSubscriberRepository(db)
	if err != nil {
		log.Fatal(err)
	}

	settingsRepository, err := repository.NewSettingsRepository(db)

	if err != nil {
		log.Fatal(err)
	}

	b := bot.NewBot(cfg, msg, subRepository, settingsRepository)
	b.Run()
}
