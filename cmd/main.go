package main

import (
	"BookClubBot/bot"
	"BookClubBot/config"
	"BookClubBot/internal/repository"
	"BookClubBot/message"
	"context"
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

	// Session persistence layer. The bot does not consume it yet (wiring lands
	// in a follow-up), but the indexes — notably the unique "one active session"
	// index — must exist before sessions are written, so create them at startup.
	sessionRepository, err := repository.NewSessionRepository(db)
	if err != nil {
		log.Fatal(err)
	}
	if err := sessionRepository.EnsureIndexes(context.Background()); err != nil {
		log.Fatalf("error ensuring session indexes: '%v'", err)
	}

	b := bot.NewBot(cfg, msg, subRepository, settingsRepository)
	b.Run()
}
