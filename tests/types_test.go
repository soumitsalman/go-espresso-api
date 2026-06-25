package espressoapi_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/soumitsalman/espressoapi/cupboard"
	"github.com/soumitsalman/espressoapi/router"
	"github.com/stretchr/testify/assert"
)

func TestSipToText(t *testing.T) {
	sip := cupboard.Sip{
		Created: time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC),
		Digest: map[string]any{
			"id":             uuid.New(),
			"created":        time.Now(),
			"site_name":      "Example News",
			"briefing":       "Summary text",
			"actions":        []any{"act1", "act2"},
			"people":         []any{"bob", "alice"},
			"tags":           []any{"t1", "t2"},
			"impact_level":   "high",
			"forecast":       "Outlook text",
			"future_outlook": "",
		},
	}

	text := router.SipToText(&sip)
	lines := strings.Split(text, "\n")

	assert.Equal(t, "reported:2026-05-19", lines[0])
	assert.Equal(t, "related:alice|bob", lines[1])
	assert.Equal(t, "briefing:Summary text", lines[2])
	assert.Equal(t, "actions:act1|act2", lines[3])
	assert.Contains(t, text, "tags:t1|t2")
	assert.Contains(t, text, "impact_level:high")
	assert.NotContains(t, text, "id:")
	assert.NotContains(t, text, "site_name:")
	assert.NotContains(t, text, "future_outlook:")
	assert.Equal(t, "forecast:Outlook text", lines[len(lines)-1])
}

func TestSipsToText(t *testing.T) {
	sips := []cupboard.Sip{
		{
			Created: time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC),
			Digest:  map[string]any{"briefing": "first"},
		},
		{
			Created: time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC),
			Digest:  map[string]any{"briefing": "second"},
		},
	}

	text := router.SipsToText(sips)
	assert.Contains(t, text, "reported:2026-05-19\nbriefing:first")
	assert.Contains(t, text, "reported:2026-05-20\nbriefing:second")
	assert.Equal(t, 1, strings.Count(text, "\n\n"))
}
