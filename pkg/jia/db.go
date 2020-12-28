package jia

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

type Entry struct {
	Number int
	User   string
}

func getLeaderboardEntries(month time.Month, year int) ([]Entry, error) {
	scan := redisClient.Scan(0, fmt.Sprintf("leaderboard:%d-%d:*", year, month), 10)
	if scan.Err() != nil {
		return nil, errors.New("Something went wrong while loading the leaderboard :cry: Please try again later!")
	}

	scanIterator := scan.Iterator()

	entries := []Entry{}

	for scanIterator.Next() {
		entry := redisClient.Get(scanIterator.Val())
		entryInt, err := entry.Int()
		if err != nil {
			return nil, err
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

	return entries, nil
}
