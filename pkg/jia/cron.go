package jia

import (
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron"
	"github.com/slack-go/slack"
)

func InitCron(config *Config) {
	c := cron.New()

	c.AddFunc("0 * * * * *", func() {
		slackClient := slack.New(config.BotToken)

		now := time.Now()
		entries, _ := getLeaderboardEntries(now.Month(), now.Year())

		_, _, err := slackClient.PostMessage(
			config.ChannelID,
			slack.MsgOptionText(
				fmt.Sprintf("It is the start of a new minute! %v", entries[0].User),
				false,
			),
		)

		if err != nil {
			log.Println(err)
		}
	})

	c.Start()
}
