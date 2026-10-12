package service

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultSuggestionLimit = 50
	minSuggestionMatches   = 3
	minPatternLen          = 3
	maxSuggestionExamples  = 5
)

func (s *catRuleSvc) Suggest(ctx context.Context, userID uuid.UUID, limit int) (*pb.SuggestRulesResponse, error) {
	uncategorized, err := s.queries.ListUncategorizedDescriptions(ctx, userID)
	if err != nil {
		return nil, wrapErr("RuleService.Suggest.Uncategorized", err)
	}

	categorized, err := s.queries.ListCategorizedDescriptions(ctx, userID)
	if err != nil {
		return nil, wrapErr("RuleService.Suggest.Categorized", err)
	}

	var total int64
	for _, row := range uncategorized {
		total += row.TotalCount
	}

	return &pb.SuggestRulesResponse{
		Suggestions:        suggestRules(uncategorized, categorized, limit),
		UncategorizedCount: total,
	}, nil
}

type categorizedStat struct {
	lower      string
	categoryID int64
	count      int64
}

type descStat struct {
	desc  string
	lower string
	count int64
	last  time.Time
}

// suggestRules greedily picks "description starts with" patterns that cover the
// most uncategorized transactions, so the first rules made clear the most rows
func suggestRules(uncategorized []sqlc.ListUncategorizedDescriptionsRow, categorized []sqlc.ListCategorizedDescriptionsRow, limit int) []*pb.RuleSuggestion {
	if limit <= 0 {
		limit = defaultSuggestionLimit
	}

	descs := make([]descStat, 0, len(uncategorized))
	for _, row := range uncategorized {
		if row.EligibleCount == 0 || strings.TrimSpace(row.TxDesc) == "" {
			continue
		}
		descs = append(descs, descStat{
			desc:  row.TxDesc,
			lower: strings.ToLower(row.TxDesc),
			count: row.EligibleCount,
			last:  row.LastSeen,
		})
	}
	cats := make([]categorizedStat, len(categorized))
	for i, row := range categorized {
		cats[i] = categorizedStat{lower: strings.ToLower(row.TxDesc), categoryID: row.CategoryID, count: row.TxCount}
	}

	// most common first, so a pattern's display casing and examples come from its biggest rows
	sort.SliceStable(descs, func(i, j int) bool {
		if descs[i].count != descs[j].count {
			return descs[i].count > descs[j].count
		}
		return descs[i].desc < descs[j].desc
	})

	// every description proposes its own leading words as a pattern
	type candidate struct {
		display string
		lower   string
		matches []int
	}
	keys := make([]string, len(descs))
	keyCount := map[string]int64{}
	byLower := map[string]*candidate{}
	var candidates []*candidate
	for i, d := range descs {
		p := descriptionPattern(d.desc)
		if len(p) < minPatternLen {
			continue
		}
		lp := strings.ToLower(p)
		keys[i] = lp
		keyCount[lp] += d.count
		if _, ok := byLower[lp]; ok {
			continue
		}
		c := &candidate{display: p, lower: lp}
		byLower[lp] = c
		candidates = append(candidates, c)
	}
	// a broader pattern only absorbs descriptions too rare to be suggested on
	// their own, so "PAYPAL" does not swallow "PAYPAL *STARBUCKS" and "PAYPAL *PROTON"
	for _, c := range candidates {
		for i := range descs {
			k := keys[i]
			if !strings.HasPrefix(descs[i].lower, c.lower) {
				continue
			}
			if k == c.lower || keyCount[k] < minSuggestionMatches {
				c.matches = append(c.matches, i)
			}
		}
	}

	covered := make([]bool, len(descs))
	remaining := func(c *candidate) int64 {
		var n int64
		for _, i := range c.matches {
			if !covered[i] {
				n += descs[i].count
			}
		}
		return n
	}

	var out []*pb.RuleSuggestion
	for len(out) < limit {
		var best *candidate
		var bestN int64
		for _, c := range candidates {
			n := remaining(c)
			// on a tie the longer pattern is the more specific rule
			if n > bestN || (n == bestN && best != nil && len(c.lower) > len(best.lower)) {
				best, bestN = c, n
			}
		}
		if best == nil || bestN < minSuggestionMatches {
			break
		}

		suggestion := &pb.RuleSuggestion{
			Pattern:          best.display,
			TransactionCount: bestN,
		}
		var last time.Time
		for _, i := range best.matches {
			if covered[i] {
				continue
			}
			covered[i] = true
			if len(suggestion.Examples) < maxSuggestionExamples {
				suggestion.Examples = append(suggestion.Examples, descs[i].desc)
			}
			if descs[i].last.After(last) {
				last = descs[i].last
			}
		}
		if !last.IsZero() {
			suggestion.LastSeen = timestamppb.New(last)
		}
		suggestion.CategoryId = majorityCategory(best.lower, cats)

		out = append(out, suggestion)
	}

	return out
}

// descriptionPattern keeps the leading words of a description up to the first
// one with a digit, which is usually a store number, card mask, or reference
func descriptionPattern(desc string) string {
	fields := strings.Fields(desc)
	if len(fields) == 0 {
		return ""
	}

	end := len(fields)
	for i := 1; i < len(fields); i++ {
		if strings.IndexFunc(fields[i], unicode.IsDigit) >= 0 {
			end = i
			break
		}
	}

	// cut the original text so the pattern stays a substring of it
	stop := 0
	for _, f := range fields[:end] {
		stop += strings.Index(desc[stop:], f) + len(f)
	}

	return strings.TrimFunc(desc[:stop], func(r rune) bool {
		// keep a processor's star, "AMAZON*" is narrower than "AMAZON"
		return r != '*' && unicode.IsPunct(r) || unicode.IsSpace(r)
	})
}

// majorityCategory returns the category most categorized matches of the
// pattern already have, when more than half of them agree
func majorityCategory(lowerPattern string, categorized []categorizedStat) *int64 {
	counts := map[int64]int64{}
	var total int64
	for _, c := range categorized {
		if !strings.HasPrefix(c.lower, lowerPattern) {
			continue
		}
		counts[c.categoryID] += c.count
		total += c.count
	}

	var bestID, bestN int64
	for id, n := range counts {
		if n > bestN || (n == bestN && id < bestID) {
			bestID, bestN = id, n
		}
	}
	if bestN == 0 || bestN*2 <= total {
		return nil
	}
	return &bestID
}
