package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/joho/godotenv"
)

const folder = "./config"

type AppConfig struct {
	GroupId               int64
	TimeToGatherBooks     int `json:"time_to_gather_books"`    // seconds
	NotifyBeforeGathering int `json:"notify_before_gathering"` // seconds
	TimeForTelegramPoll   int `json:"time_for_telegram_poll"`  // seconds
	NotifyBeforePoll      int `json:"notify_before_poll"`      //seconds
	LongPollingTimeout    int `json:"long_polling_timeout"`    // seconds
	TKey                  string
	DBPath                string `json:"db_path"`
	LogFileName           string `json:"log_file_name"`
	DebugMode             bool   `json:"debug_mode"`
}

func LoadConfig() (*AppConfig, error) {
	godotenv.Load()
	env := determineEnv()
	cfg, err := readConfigFile(env)
	if err != nil {
		return nil, err
	}

	tKey := os.Getenv("telegrammApiKey")
	if tKey == "" {
		return nil, fmt.Errorf("cannot find telegrammApiKey env varaible")
	}

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
	fileName := fmt.Sprintf("%s/config_%s.json", folder, env)
	f, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("Cannot open %s", fileName)
	}
	defer f.Close()
	return parsreAppConfig(f)
}

func parsreAppConfig(r io.Reader) (*AppConfig, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("Cannot read from reader during parsing App config")
	}

	var res AppConfig
	err = json.Unmarshal(data, &res)

	if err != nil {
		return nil, fmt.Errorf("Cannot unmarshal data to AppConfig during parsing App config")
	}

	return &res, nil
}
