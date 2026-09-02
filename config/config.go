package config

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const folder = "./config"

// adminIDsEnv is the environment variable that overrides the JSON admin_ids
// list, so a deployment (Railway, Docker) can set admins without editing the
// shipped config. Comma-separated Telegram numeric user IDs: "123,456".
//
// The name follows the platform convention of the other environment variables
// (APP_ENV, MONGO_URL); telegrammApiKey predates it and is kept only because
// renaming it would break the running deployments.
const adminIDsEnv = "ADMIN_IDS"

type AppConfig struct {
	GroupId             int64
	TimeToGatherBooks   int `json:"time_to_gather_books"`   // seconds
	TimeForTelegramPoll int `json:"time_for_telegram_poll"` // seconds
	NotifyBeforePoll    int `json:"notify_before_poll"`     //seconds
	LongPollingTimeout  int `json:"long_polling_timeout"`   // seconds

	// GatheringReminderInterval is the spacing of the gathering reminders, which
	// are anchored backwards from the gathering deadline: one fires at
	// deadline-k*interval for every k >= 1 that still falls inside the gathering
	// window. The k == 1 reminder is the last call and is the only one that also
	// DMs the members who have not submitted. 0 disables gathering reminders
	// entirely. See bot/reminders.go.
	GatheringReminderInterval int `json:"gathering_reminder_interval"` // seconds

	// QuietHoursStart and QuietHoursEnd bound a nightly window, as hours [0,24)
	// in Timezone, during which reminders are held rather than sent: a reminder
	// that comes due inside it goes out when the window ends. The window wraps
	// midnight when start > end. Equal values disable quiet hours.
	QuietHoursStart int    `json:"quiet_hours_start"`
	QuietHoursEnd   int    `json:"quiet_hours_end"`
	Timezone        string `json:"timezone"`

	// Location is Timezone resolved at startup. A nil Location disables quiet
	// hours, so reminders are never held.
	Location *time.Location

	// AdminIDs are the Telegram numeric user IDs allowed to run admin commands
	// such as /start_vote. The ADMIN_IDS env var, when set, replaces the JSON
	// list. An empty list denies the admin commands to everyone (fail-closed).
	AdminIDs []int64 `json:"admin_ids"`

	TKey      string
	MongoURI  string `json:"mongo_uri"`
	DBName    string `json:"db_name"`
	DebugMode bool   `json:"debug_mode"`
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

	// Railway's MongoDB plugin injects MONGO_URL; prefer it over the JSON value.
	if mongoURL := os.Getenv("MONGO_URL"); mongoURL != "" {
		cfg.MongoURI = mongoURL
	}

	if err := cfg.resolveQuietHours(); err != nil {
		return nil, err
	}

	if err := cfg.applyAdminIDsEnv(os.Getenv(adminIDsEnv)); err != nil {
		return nil, err
	}
	if len(cfg.AdminIDs) == 0 {
		log.Printf("WARNING: no admin IDs configured (admin_ids in config or %s env); /start_vote is disabled for everyone", adminIDsEnv)
	}

	return cfg, nil
}

// applyAdminIDsEnv replaces AdminIDs with the IDs parsed from raw, the value of
// the ADMIN_IDS env var. An unset or blank value leaves the JSON list as is; a
// value that does not parse fails loudly rather than silently locking the
// admins out.
func (c *AppConfig) applyAdminIDsEnv(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	ids, err := parseAdminIDs(raw)
	if err != nil {
		return fmt.Errorf("cannot parse %s env variable: %w", adminIDsEnv, err)
	}
	c.AdminIDs = ids
	return nil
}

// parseAdminIDs parses a comma-separated list of Telegram user IDs. Whitespace
// around entries and empty entries are ignored.
func parseAdminIDs(raw string) ([]int64, error) {
	var ids []int64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid Telegram user id %q", part)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// resolveQuietHours validates the quiet-hours window and loads its timezone.
// It fails loudly rather than falling back to UTC: a silently wrong timezone
// would silence reminders during the wrong nine hours of the day.
func (c *AppConfig) resolveQuietHours() error {
	if c.Timezone == "" {
		return nil // quiet hours disabled
	}
	if !validHour(c.QuietHoursStart) || !validHour(c.QuietHoursEnd) {
		return fmt.Errorf("quiet hours must be within [0,24), got start=%d end=%d", c.QuietHoursStart, c.QuietHoursEnd)
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return fmt.Errorf("cannot load timezone %q: %w", c.Timezone, err)
	}
	c.Location = loc
	return nil
}

func validHour(h int) bool {
	return h >= 0 && h < 24
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
