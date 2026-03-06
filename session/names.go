package session

import (
	"fmt"
	"math/rand"
)

var branchNames = []string{
	"Zi",    // 子 Rat
	"Chou",  // 丑 Ox
	"Yin",   // 寅 Tiger
	"Mao",   // 卯 Hare
	"Chen",  // 辰 Dragon
	"Si",    // 巳 Serpent
	"Wu",    // 午 Steed
	"Wei",   // 未 Sheep
	"Shen",  // 申 Monkey
	"You",   // 酉 Rooster
	"Xu",    // 戌 Dog
	"Hai",   // 亥 Pig
}

// PickBranchName selects a random unused branch name.
// If all 12 are taken, appends a numeric suffix to a random one.
func PickBranchName() string {
	sessions, err := ListSessions()
	if err != nil {
		sessions = nil
	}

	used := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		if s.Status != StatusDone {
			used[s.Name] = true
		}
	}

	var available []string
	for _, name := range branchNames {
		if !used[name] {
			available = append(available, name)
		}
	}

	if len(available) > 0 {
		return available[rand.Intn(len(available))]
	}

	// All 12 taken — pick a random branch and find the lowest free suffix
	base := branchNames[rand.Intn(len(branchNames))]
	for i := 2; ; i++ {
		candidate := base + fmt.Sprintf(" %d", i)
		if !used[candidate] {
			return candidate
		}
	}
}
