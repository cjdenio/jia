package jia

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

func HandleInnerEvent(slackClient *slack.Client, innerEvent *slackevents.EventsAPIInnerEvent) {
	switch e := innerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		onMessage(slackClient, e)
	}
}

func onMessage(slackClient *slack.Client, event *slackevents.MessageEvent) {
	// Ignore messages that aren't in the target channel, or are non-user messages.
	if event.Channel != jiaConfig.ChannelID || event.User == "USLACKBOT" || event.User == "" {
		return
	}

	// Ignore threaded messages.
	if event.ThreadTimeStamp != "" {
		return
	}

	// Attempt to extract a positive number at the start of a string.
	countPattern := regexp.MustCompile(`^\d+`)
	matchedNumber, err := strconv.Atoi(countPattern.FindString(event.Text))

	// Ignore messages that don't have numbers.
	if err != nil {
		return
	}

	log.Printf("Got number: %d", matchedNumber)

	// Reject if sender also sent the previous number.
	lastSenderID, err := redisClient.Get("last_sender_id").Result()
	if err != nil {
		log.Println("Failed to retrieve the last sender")
		return
	}
	if event.User == lastSenderID {
		slackClient.AddReaction("bangbang", slack.ItemRef{
			Channel:   event.Channel,
			Timestamp: event.TimeStamp,
		})
		slackClient.PostEphemeral(event.Channel, event.User, slack.MsgOptionText(
			"You counted consecutively! That’s not allowed.", false))
		return
	}

	// Retrieve stored info about the last valid number and its sender.
	lastValidNumberStr, err := redisClient.Get("last_valid_number").Result()
	if err != nil {
		log.Println("Failed to retrieve the last valid number")
		return
	}
	lastValidNumber, err := strconv.Atoi(lastValidNumberStr)
	if err != nil {
		log.Println("Failed to convert the last valid number to an integer")
		return
	}

	// Ignore numbers that aren't in order.
	if matchedNumber != lastValidNumber+1 {
		slackClient.AddReaction("bangbang", slack.ItemRef{
			Channel:   event.Channel,
			Timestamp: event.TimeStamp,
		})
		slackClient.PostEphemeral(event.Channel, event.User, slack.MsgOptionText(
			fmt.Sprintf("You counted incorrectly! The next valid number is supposed to be *%d*.", lastValidNumber+1), false))
		return
	}

	// Finally!
	redisClient.Set("last_valid_number", matchedNumber, 0)
	redisClient.Set("last_valid_ts", event.TimeStamp, 0)
	redisClient.Set("last_sender_id", event.User, 0)

	// Get the current month/year in UTC
	now := time.Now().UTC()
	year := now.Year()
	month := now.Month()

	// Increment the person's monthly count
	redisClient.Incr(fmt.Sprintf("leaderboard:%d-%d:%s", year, month, event.User))

	// Increment the person's count for any running events
	for _, counting_event := range jiaConfig.GetRunningEvents() {
		redisClient.Incr(fmt.Sprintf("event:%s:%s", counting_event.Name, event.User))
	}
}

func HandleLeaderboardSlashCommand(w http.ResponseWriter, r *http.Request) {
	// Get the current month/year in UTC
	now := time.Now().UTC()
	year := now.Year()
	month := now.Month()

	scan := redisClient.Scan(0, fmt.Sprintf("leaderboard:%d-%d:*", year, month), 10)
	if scan.Err() != nil {
		w.Write([]byte("Something went wrong while loading the leaderboard :cry: Please try again later!"))
		return
	}

	scanIterator := scan.Iterator()

	type Entry struct {
		Number int
		User   string
	}

	entries := []Entry{}

	for scanIterator.Next() {
		entry := redisClient.Get(scanIterator.Val())
		entryInt, err := entry.Int()
		if err != nil {
			return
		}

		if user, ok := parseLeaderboardEntry(scanIterator.Val()); ok {
			entries = append(entries, Entry{
				Number: entryInt,
				User:   user,
			})
		}
	}

	// Sort entries
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Number > entries[j].Number
	})

	if len(entries) > 10 {
		entries = entries[:10]
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", fmt.Sprintf(":chart_with_upwards_trend: Counting stats for *%s %d*:", month.String(), year), false, false),
			nil,
			nil,
		),
	}

	for i, v := range entries {
		emoji := ""
		if i == 0 {
			emoji = ":first_place_medal:"
		} else if i == 1 {
			emoji = ":second_place_medal:"
		} else if i == 2 {
			emoji = ":third_place_medal:"
		}

		blocks = append(blocks, slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("%s <@%s> has counted *%d* this month", emoji, v.User, v.Number), false, false), nil, nil))
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"blocks":        blocks,
		"response_type": "ephemeral",
	})

	w.Header().Add("Content-Type", "application/json")
	w.Write(resp)
}

func HandleEventsSlashCommand(w http.ResponseWriter, r *http.Request) {
	events := jiaConfig.GetRunningEvents()

	var blocks []slack.Block

	if len(events) == 0 {
		blocks = append(blocks, slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", "There aren't any counting events running right now.", false, false), nil, nil))
	} else if len(events) == 1 {
		blocks = append(blocks, slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", fmt.Sprintf(":calendar: Counting stats for event *%s*:", events[0].Name), false, false), nil, nil))

		scan := redisClient.Scan(0, fmt.Sprintf("event:%s:*", events[0].Name), 10)
		scanIterator := scan.Iterator()

		type Entry struct {
			Number int
			User   string
		}

		entries := []Entry{}

		for scanIterator.Next() {
			entry := redisClient.Get(scanIterator.Val())
			entryInt, err := entry.Int()
			if err != nil {
				return
			}

			if _, user, ok := parseEventEntry(scanIterator.Val()); ok {
				entries = append(entries, Entry{
					Number: entryInt,
					User:   user,
				})
			}
		}

		// Sort entries
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Number > entries[j].Number
		})

		for i, v := range entries {
			emoji := ""
			if i == 0 {
				emoji = ":first_place_medal:"
			} else if i == 1 {
				emoji = ":second_place_medal:"
			} else if i == 2 {
				emoji = ":third_place_medal:"
			}

			blocks = append(blocks, slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("%s <@%s> has counted *%d* so far", emoji, v.User, v.Number), false, false), nil, nil))
		}

		if len(entries) > 10 {
			entries = entries[:10]
		}
		blocks = append(blocks, slack.NewContextBlock("", slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("Event will end at *<!date^%d^{time} on {date}|some date>*, your time", events[0].EndTime.Unix()), false, false)))
	} else {
		blocks = append(blocks, slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", "Something went wrong fetching the events leaderboard :cry:", false, false), nil, nil))
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"blocks":        blocks,
		"response_type": "ephemeral",
	})

	w.Header().Add("Content-Type", "application/json")
	w.Write(resp)
}

func parseLeaderboardEntry(key string) (string, bool) {
	re := regexp.MustCompile(`leaderboard:\d+-\d+:(\w+)`)

	match := re.FindStringSubmatch(key)
	if match == nil {
		return "", false
	}
	return match[1], true
}

func parseEventEntry(key string) (event_name string, user_id string, ok bool) {
	re := regexp.MustCompile(`event:(.+):(\w+)`)

	match := re.FindStringSubmatch(key)
	if match == nil {
		ok = false
		return
	}

	ok = true
	event_name = match[1]
	user_id = match[2]

	return
}
