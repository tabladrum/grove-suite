package ingestion

import (
	"strconv"
	"strings"

	"github.com/provasign/provasign/internal/grove"
)

type GranularityScorer struct {
	groveClient *grove.Client
	threshold   float64
}

func NewGranularityScorer(gc *grove.Client, threshold float64) *GranularityScorer {
	return &GranularityScorer{groveClient: gc, threshold: threshold}
}

type GSResult struct {
	Score      float64
	ICRSymbols int
	Feedback   string
}

func (gs *GranularityScorer) Score(description string) GSResult {
	hScore := heuristicScore(description)

	if hScore < 0.60 {
		return GSResult{
			Score:    hScore,
			Feedback: heuristicFeedback(description, hScore),
		}
	}

	icrScore, symbolCount := gs.icrScore(description)
	final := 0.4*hScore + 0.6*icrScore

	return GSResult{
		Score:      final,
		ICRSymbols: symbolCount,
		Feedback:   buildFeedback(final, symbolCount, gs.threshold),
	}
}

func heuristicScore(desc string) float64 {
	words := strings.Fields(desc)
	score := 0.65

	if len(words) < 8 || len(words) > 150 {
		score = 0.50
	}

	lower := strings.ToLower(desc)
	vague := []string{"refactor all", "update everything", "improve", "clean up", "cleanup"}
	for _, v := range vague {
		if strings.Contains(lower, v) {
			if score > 0.50 {
				score = 0.50
			}
			break
		}
	}

	specific := []string{"/", "endpoint", "function", "method", "class", "interface",
		"struct", "handler", "service", "model", "route", "api", "config"}
	for _, s := range specific {
		if strings.Contains(lower, s) {
			score += 0.15
			break
		}
	}

	domains := []string{"auth", "payment", "notification", "storage", "cache", "queue",
		"search", "report", "admin", "billing", "analytics", "email", "sms"}
	domainCount := 0
	for _, d := range domains {
		if containsWord(lower, d) {
			domainCount++
		}
	}
	if domainCount >= 3 {
		score = 0.40
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

func (gs *GranularityScorer) icrScore(description string) (float64, int) {
	if gs.groveClient == nil {
		return 0.65, 0
	}
	resp, err := gs.groveClient.ICR(description)
	if err != nil {
		return 0.65, 0
	}
	symbols := resp.SymbolCount
	var score float64
	switch {
	case symbols <= 50:
		score = 0.70 + 0.25*float64(50-symbols)/50
	case symbols <= 150:
		score = 0.40 + 0.30*float64(150-symbols)/100
	default:
		score = 0.40 * float64(300) / float64(symbols+300)
	}
	return score, symbols
}

func heuristicFeedback(desc string, score float64) string {
	words := strings.Fields(desc)
	if len(words) < 8 {
		return "Intent description is too short. Add more detail: what exactly should change, in which component, and why."
	}
	if len(words) > 150 {
		return "Intent description is too long. Focus on one specific change. Break into multiple intents if needed."
	}
	return "Intent is too vague. Name a specific endpoint, function, or component to change."
}

func buildFeedback(score float64, symbolCount int, threshold float64) string {
	if score >= threshold {
		return ""
	}
	msg := "Intent scope is too broad"
	if symbolCount > 0 {
		msg += " — " + pluralSymbols(symbolCount) + " affected"
	}
	msg += ". Narrow the scope to a specific component, endpoint, or function."
	return msg
}

func containsWord(s, word string) bool {
	wl := len(word)
	for i := 0; i <= len(s)-wl; i++ {
		if s[i:i+wl] != word {
			continue
		}
		before := i == 0 || !isAlphaNum(s[i-1])
		after := i+wl >= len(s) || !isAlphaNum(s[i+wl])
		if before && after {
			return true
		}
	}
	return false
}

func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

func pluralSymbols(n int) string {
	if n == 1 {
		return "1 symbol"
	}
	return strconv.Itoa(n) + " symbols"
}
