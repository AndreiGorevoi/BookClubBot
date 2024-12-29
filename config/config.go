package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	GroupId int64
	TKey    string
}

func LoadConfig() (*AppConfig, error) {
	godotenv.Load()
	tKey := os.Getenv("telegrammApiKey")
	groupIdStr := os.Getenv("groupId")

	groupId, err := strconv.ParseInt(groupIdStr, 10, 64)

	if err != nil {
		return nil, err
	}

	return &AppConfig{
		GroupId: groupId,
		TKey:    tKey,
	}, nil
}
