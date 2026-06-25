package router

import (
	"fmt"
	"sort"
	"strings"
	"time"

	datautils "github.com/soumitsalman/data-utils"
	"github.com/soumitsalman/espressoapi/cupboard"
)

// ErrorResponse is the standard JSON error envelope for 4xx and 5xx responses.
type ErrorResponse struct {
	Error string `json:"error" example:"invalid id: not-a-uuid"`
}

// Event is the flattened JSON shape returned by GET /events and GET /related for event-kind sips.
// The handler merges persisted sip metadata (`id`, `created`, `site_name`) into the digest map before responding.
// The fields below are the stable, commonly present keys; individual records may include additional
// pipeline-specific keys not listed here.
type Event struct {
	ID                 string   `json:"id" swaggertype:"string" format:"uuid" example:"339366bc-464d-582f-8132-6875ccc814d2"`
	Reported           string   `json:"reported" example:"2026-05-19T06:00:00-04:00"`
	SiteName           string   `json:"site_name,omitempty" example:"Example News"`
	Briefing           string   `json:"briefing" example:"Discussion on recent Supreme Court rulings affecting redistricting and implications for Black political representation in Mississippi."`
	EventType          string   `json:"event_type" example:"political_analysis"`
	ImpactLevel        string   `json:"impact_level" example:"high" enums:"low,medium,high"`
	FutureOutlook      string   `json:"future_outlook" example:"Concerns about erosion of Black political influence amid ongoing gerrymandering debates; potential shifts in power dynamics."`
	Actions            []string `json:"actions" example:"Senator Bernie Sanders introduced a 50% ownership tax on major AI firms"`
	CrossDomainImpacts []string `json:"cross_domain_impacts" example:"Legislative actions influencing redistricting,Judicial changes altering court mandates on fair representation"`
	People             []string `json:"people" example:"michael_watts,rep_bennie_thompson"`
	Regions            []string `json:"regions" example:"mississippi,deep_south"`
	Tags               []string `json:"tags" example:"voter_suppression,gerrymandering,racial_politics,supreme_court,political_representation"`
}

// Signal is the flattened JSON shape returned by GET /signals and GET /related for signal-kind sips.
// The handler merges persisted sip metadata (`id`, `created`, `site_name`) into the digest map before responding.
// The fields below are the stable, commonly present keys; individual records may include additional
// pipeline-specific keys not listed here.
type Signal struct {
	ID              string   `json:"id" swaggertype:"string" format:"uuid" example:"e7d7571a-13f0-56f0-8563-50863b79c781"`
	Reported        string   `json:"reported" example:"2026-06-02T14:02:00-04:00"`
	SiteName        string   `json:"site_name,omitempty" example:"Example News"`
	Briefing        string   `json:"briefing" example:"On 2026-06-02, U.S. lawmakers and the Trump administration debated AI sovereign-wealth and compute-tax proposals amid soaring inflation..."`
	ImpactLevel     string   `json:"impact_level" example:"high" enums:"low,medium,high"`
	Forecast        string   `json:"forecast" example:"Short-term: Market volatility will persist, AI regulatory scrutiny will intensify, and consumer confidence remains low."`
	Events          []string `json:"events" example:"2026-06-01: Senator Bernie Sanders introduced a 50% ownership tax on major AI firms"`
	Impacts         []string `json:"impacts" example:"9.3% market sell-off across tech and financial sectors.,Decline in consumer confidence and increased credit-card delinquency."`
	Drivers         []string `json:"drivers" example:"High inflation and rising consumer costs driven by supply-chain bottlenecks and geopolitical tensions."`
	ImpactedDomains []string `json:"impacted_domains" example:"finance,technology,cybersecurity,labor,healthcare,energy,policy"`
	Tags            []string `json:"tags" example:"ai_sovereign_wealth_fund,ai_taxation,compute_tax,inflation,market_volatility"`
}

func enrichSipDigest(sip *cupboard.Sip) {
	sip.Digest["id"] = sip.ID
	sip.Digest["reported"] = sip.Created
	if sip.SiteName != nil && *sip.SiteName != "" {
		sip.Digest["site_name"] = *sip.SiteName
	}
}

func sipsToDigest(sips []cupboard.Sip) []map[string]any {
	for i := range sips {
		enrichSipDigest(&sips[i])
	}
	return datautils.Transform(sips, func(sip *cupboard.Sip) map[string]any {
		return sip.Digest
	})
}

var _entityKeys = map[string]struct{}{
	"regions":       {},
	"people":        {},
	"products":      {},
	"companies":     {},
	"stock_tickers": {},
}

var _priorityDigestKeys = []string{"briefing", "actions"}

var _outlookKeys = []string{"forecast", "future_outlook"}

var _skipDigestKeys = map[string]struct{}{
	"id":        {},
	"created":   {},
	"site_name": {},
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch typed := v.(type) {
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case []int:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func valueToStr(v any) string {
	switch typed := v.(type) {
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, valueToStr(item))
		}
		return strings.Join(parts, "|")
	case []string:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, valueToStr(item))
		}
		return strings.Join(parts, "|")
	case []int:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, valueToStr(item))
		}
		return strings.Join(parts, "|")
	case map[string]any:
		parts := make([]string, 0, len(typed))
		for k, item := range typed {
			if isEmpty(item) {
				continue
			}
			parts = append(parts, k+":"+valueToStr(item))
		}
		return strings.Join(parts, "|")
	case time.Time:
		return typed.Format(time.DateOnly)
	default:
		return fmt.Sprint(typed)
	}
}

func entityTags(digest map[string]any) []string {
	seen := make(map[string]struct{}, 10)
	for key := range _entityKeys {
		v, ok := digest[key]
		if !ok || isEmpty(v) {
			continue
		}
		switch typed := v.(type) {
		case []any:
			for _, tag := range typed {
				if !isEmpty(tag) {
					seen[fmt.Sprint(tag)] = struct{}{}
				}
			}
		case []string:
			for _, tag := range typed {
				if tag != "" {
					seen[tag] = struct{}{}
				}
			}
		default:
			seen[fmt.Sprint(typed)] = struct{}{}
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func isExcludedMiddleKey(key string) bool {
	if _, ok := _entityKeys[key]; ok {
		return true
	}
	if _, ok := _skipDigestKeys[key]; ok {
		return true
	}
	for _, k := range _priorityDigestKeys {
		if k == key {
			return true
		}
	}
	for _, k := range _outlookKeys {
		if k == key {
			return true
		}
	}
	return false
}

func SipToText(sip *cupboard.Sip) string {
	var lines []string

	if !sip.Created.IsZero() {
		lines = append(lines, "reported:"+valueToStr(sip.Created))
	}

	if tags := entityTags(sip.Digest); len(tags) > 0 {
		lines = append(lines, "related:"+strings.Join(tags, "|"))
	}

	for _, key := range _priorityDigestKeys {
		if v, ok := sip.Digest[key]; ok && !isEmpty(v) {
			lines = append(lines, key+":"+valueToStr(v))
		}
	}

	for key, v := range sip.Digest {
		if isEmpty(v) || isExcludedMiddleKey(key) {
			continue
		}
		lines = append(lines, key+":"+valueToStr(v))
	}

	for _, key := range _outlookKeys {
		if v, ok := sip.Digest[key]; ok && !isEmpty(v) {
			lines = append(lines, key+":"+valueToStr(v))
		}
	}

	return strings.Join(lines, "\n")
}

func SipsToText(sips []cupboard.Sip) string {
	blocks := make([]string, len(sips))
	for i := range sips {
		blocks[i] = SipToText(&sips[i])
	}
	return strings.Join(blocks, "\n\n")
}
