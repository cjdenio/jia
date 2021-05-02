package jia

import (
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"
)

type Event struct {
	Name      string `yaml:"name"`
	StartTime time.Time
	EndTime   time.Time

	StartString string `yaml:"start"`
	EndString   string `yaml:"end"`
}

type Config struct {
	BotToken          string
	ChannelID         string
	Port              int
	RedisURL          string
	VerificationToken string

	Events []Event
}

func (c Config) GetRunningEvents() []Event {
	var running_events []Event

	for _, event := range c.Events {
		if time.Since(event.StartTime) > 0 && time.Until(event.EndTime) > 0 {
			running_events = append(running_events, event)
		}
	}

	return running_events
}

func NewConfig() *Config {
	events_file, err := os.ReadFile("events.yaml")
	if err != nil {
		panic(err)
	}

	var events []Event

	err = yaml.Unmarshal(events_file, &events)
	if err != nil {
		panic(err)
	}

	for i, event := range events {
		events[i].StartTime, _ = time.Parse("15:04 Jan 2, 2006 MST", event.StartString)
		events[i].EndTime, _ = time.Parse("15:04 Jan 2, 2006 MST", event.EndString)
	}

	return &Config{
		BotToken:          getEnv("SLACK_BOT_TOKEN", ""),
		ChannelID:         getEnv("SLACK_CHANNEL_ID", ""),
		Port:              getEnvAsInt("PORT", 3000),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379/0"),
		VerificationToken: getEnv("SLACK_VERIFICATION_TOKEN", ""),
		Events:            events,
	}
}

func getEnv(key string, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if exists {
		return value
	}

	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}

	return defaultValue
}
