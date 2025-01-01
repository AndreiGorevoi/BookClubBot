package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

const confFolder = "./config"

type AppConfig struct {
	GroupId               int64
	TimeForGatheringBooks int `json:"time_for_gathering_books"`
	TimeForTelegramPoll   int `json:"time_for_telegram_poll"`
	TKey                  string
}

func LoadConfig() (*AppConfig, error) {
	godotenv.Load()
	env := determineEnv()
	cfg, err := readConfigFile(env)
	if err != nil {
		return nil, err
	}

	tKey := os.Getenv("telegrammApiKey")
	groupIdStr := os.Getenv("groupId")

	groupId, err := strconv.ParseInt(groupIdStr, 10, 64)

	if err != nil {
		return nil, err
	}
	cfg.GroupId = groupId
	cfg.TKey = tKey
	return cfg, nil
}

func determineEnv() string {
	env := os.Getenv("APP_ENV")
	if env == "" {
		return "dev"
	}
	return env
}

func readConfigFile(env string) (*AppConfig, error) {
	fileName := fmt.Sprintf("%s/config_%s.json", confFolder, env)
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, err := parsreAppConfig(f)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func parsreAppConfig(r io.Reader) (*AppConfig, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var res AppConfig
	err = json.Unmarshal(data, &res)

	if err != nil {
		return nil, err
	}

	fmt.Println("APP CONFIG:", res)
	return &res, nil
}
