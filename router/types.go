package router

import (
	"fmt"
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
// The handler merges persisted sip metadata (`id`, `created`) into the digest map before responding.
// The fields below are the stable, commonly present keys; individual records may include additional
// pipeline-specific keys not listed here.
type Event struct {
	ID                 string   `json:"id" swaggertype:"string" format:"uuid" example:"339366bc-464d-582f-8132-6875ccc814d2"`
	Created            string   `json:"created" example:"2026-05-19T06:00:00-04:00"`
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
// The handler merges persisted sip metadata (`id`, `created`) into the digest map before responding.
// The fields below are the stable, commonly present keys; individual records may include additional
// pipeline-specific keys not listed here.
type Signal struct {
	ID              string   `json:"id" swaggertype:"string" format:"uuid" example:"e7d7571a-13f0-56f0-8563-50863b79c781"`
	Created         string   `json:"created" example:"2026-06-02T14:02:00-04:00"`
	Briefing        string   `json:"briefing" example:"On 2026-06-02, U.S. lawmakers and the Trump administration debated AI sovereign-wealth and compute-tax proposals amid soaring inflation..."`
	ImpactLevel     string   `json:"impact_level" example:"high" enums:"low,medium,high"`
	Forecast        string   `json:"forecast" example:"Short-term: Market volatility will persist, AI regulatory scrutiny will intensify, and consumer confidence remains low."`
	Events          []string `json:"events" example:"2026-06-01: Senator Bernie Sanders introduced a 50% ownership tax on major AI firms"`
	Impacts         []string `json:"impacts" example:"9.3% market sell-off across tech and financial sectors.,Decline in consumer confidence and increased credit-card delinquency."`
	Drivers         []string `json:"drivers" example:"High inflation and rising consumer costs driven by supply-chain bottlenecks and geopolitical tensions."`
	ImpactedDomains []string `json:"impacted_domains" example:"finance,technology,cybersecurity,labor,healthcare,energy,policy"`
	Tags            []string `json:"tags" example:"ai_sovereign_wealth_fund,ai_taxation,compute_tax,inflation,market_volatility"`
}

func sipsToDigest(sips []cupboard.Sip) []map[string]any {
	for i := range sips {
		sips[i].Digest["id"] = sips[i].ID
		sips[i].Digest["created"] = sips[i].Created
	}
	return datautils.Transform(sips, func(sip *cupboard.Sip) map[string]any {
		return sip.Digest
	})
}

var _TAG_FIELDS = map[string]struct{}{
	"regions":       {},
	"people":        {},
	"products":      {},
	"companies":     {},
	"organization":  {},
	"stock_tickers": {},
	"tags":          {},
}

func sipsToText(sips []cupboard.Sip) string {
	text := strings.Builder{}
	for _, sip := range sips {
		text.WriteString("date: ")
		text.WriteString(sip.Created.Format(time.DateOnly))
		text.WriteByte('\n')

		tags := make(map[string]struct{}, 10)
		for key, value := range sip.Digest {
			if _, ok := _TAG_FIELDS[key]; ok {
				appendTags(tags, value)
			} else {
				text.WriteString(key)
				text.WriteString(": ")
				appendValue(&text, value)
				text.WriteByte('\n')
			}
		}
		if len(tags) > 0 {
			tag_items, _ := datautils.MapToArray(tags)
			text.WriteString("related_to: ")
			text.WriteString(strings.Join(tag_items, ", "))
			text.WriteByte('\n')
		}
		text.WriteByte('\n')
	}
	return text.String()
}

func appendValue(text *strings.Builder, value any) {
	switch typed := value.(type) {
	case []string:
		text.WriteString(strings.Join(typed, ", "))
	case []any:
		for i, item := range typed {
			if i > 0 {
				text.WriteString(", ")
			}
			text.WriteString(fmt.Sprint(item))
		}
	case []int:
		for i, item := range typed {
			if i > 0 {
				text.WriteString(", ")
			}
			text.WriteString(fmt.Sprint(item))
		}
	default:
		text.WriteString(fmt.Sprint(typed))
	}
}

func appendTags(seen map[string]struct{}, value any) {
	switch typed := value.(type) {
	case []string:
		for _, tag := range typed {
			seen[tag] = struct{}{}
		}
	case []any:
		for _, tag := range typed {
			seen[fmt.Sprint(tag)] = struct{}{}
		}
	default:
		seen[fmt.Sprint(typed)] = struct{}{}
	}
}
