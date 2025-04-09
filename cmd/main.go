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

	logFile, err := os.OpenFile(cfg.LogFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

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
