package service

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	pb "nagomi-core/internal/gen/nagomi/v1"
)

// Transfer matching pairs an outgoing transaction with an incoming one on
// another of the user's accounts. It knows nothing about particular banks:
// importers that know two rows belong together say so with transfer_ref, and
// everything else is paired from amount, date and what the descriptions share.

const (
	transferMaxDays = 5
	// a transfer may lose a fee on the way: up to 1% plus a flat $10
	transferFeeFlatCents = 1000
	transferFeePercent   = 1
	// a number seen in more transactions than this is an account number or
	// similar, not a reference shared by two sides of one transfer
	transferTokenMaxUses = 3
)

type transferTx struct {
	ID        int64
	AccountID int64
	Date      time.Time
	Cents     int64
	Currency  string
	Direction pb.TransactionDirection
	Text      string
	Ref       *string
}

type transferAccount struct {
	ID         int64
	Bank       string
	Names      []string
	CreditCard bool
}

type transferPair struct {
	Out, In int64
	Method  pb.TransferMethod
}

type transferPlan struct {
	Links       []transferPair
	Suggestions []transferPair
}

// transferScore orders candidate pairs; a higher score is a more convincing pair.
type transferScore struct {
	signals int
	exact   bool
	days    int
}

func (a transferScore) compare(b transferScore) int {
	if a.signals != b.signals {
		return b.signals - a.signals
	}
	if a.exact != b.exact {
		if a.exact {
			return -1
		}
		return 1
	}
	return a.days - b.days
}

type transferEdge struct {
	out, in *transferTx
	score   transferScore
	// confident enough to link without asking
	auto bool
}

// planTransfers decides which transactions to link and which pairs to suggest.
// linked holds transactions already in a linked transfer; rejected holds
// out/in pairs the user said are not transfers.
func planTransfers(
	txs []transferTx,
	accounts map[int64]transferAccount,
	linked map[int64]bool,
	rejected map[[2]int64]bool,
) transferPlan {
	var plan transferPlan
	used := make(map[int64]bool, len(linked))
	for id := range linked {
		used[id] = true
	}

	free := make([]*transferTx, 0, len(txs))
	for i := range txs {
		t := &txs[i]
		if used[t.ID] || accounts[t.AccountID].ID == 0 {
			continue
		}
		free = append(free, t)
	}

	// pairs the importer already knows about
	byRef := map[string][]*transferTx{}
	for _, t := range free {
		if t.Ref != nil {
			byRef[*t.Ref] = append(byRef[*t.Ref], t)
		}
	}
	for _, group := range byRef {
		if len(group) != 2 {
			continue
		}
		out, in := group[0], group[1]
		if out.Direction == pb.TransactionDirection_DIRECTION_INCOMING {
			out, in = in, out
		}
		if out.Direction != pb.TransactionDirection_DIRECTION_OUTGOING ||
			in.Direction != pb.TransactionDirection_DIRECTION_INCOMING ||
			out.AccountID == in.AccountID || rejected[[2]int64{out.ID, in.ID}] {
			continue
		}
		plan.Links = append(plan.Links, transferPair{Out: out.ID, In: in.ID, Method: pb.TransferMethod_TRANSFER_METHOD_REFERENCE})
		used[out.ID], used[in.ID] = true, true
	}
	slices.SortFunc(plan.Links, func(a, b transferPair) int { return int(a.Out - b.Out) })

	edges := transferEdges(free, accounts, used, rejected)

	// walk from the most convincing pairs down. a transaction with two equally
	// good counterparts is ambiguous and only suggested, unless the counterparts
	// are interchangeable, in which case which one is picked changes nothing.
	contested := map[int64]bool{}
	for start := 0; start < len(edges); {
		end := start + 1
		for end < len(edges) && edges[end].score.compare(edges[start].score) == 0 {
			end++
		}
		group := edges[start:end]
		start = end

		open := func(e transferEdge) bool {
			return !used[e.out.ID] && !used[e.in.ID] && !contested[e.out.ID] && !contested[e.in.ID]
		}
		var ambiguous []transferEdge
		for _, e := range group {
			if !open(e) {
				continue
			}
			clear := true
			for _, other := range group {
				if other == e || !open(other) {
					continue
				}
				if other.out == e.out && !interchangeable(other.in, e.in) ||
					other.in == e.in && !interchangeable(other.out, e.out) {
					clear = false
					break
				}
			}
			if !clear {
				ambiguous = append(ambiguous, e)
				continue
			}
			pair := transferPair{Out: e.out.ID, In: e.in.ID, Method: pb.TransferMethod_TRANSFER_METHOD_MATCHED}
			if e.auto {
				plan.Links = append(plan.Links, pair)
				used[e.out.ID], used[e.in.ID] = true, true
			} else {
				plan.Suggestions = append(plan.Suggestions, pair)
				contested[e.out.ID], contested[e.in.ID] = true, true
			}
		}
		for _, e := range ambiguous {
			if used[e.out.ID] || used[e.in.ID] {
				continue
			}
			plan.Suggestions = append(plan.Suggestions, transferPair{Out: e.out.ID, In: e.in.ID, Method: pb.TransferMethod_TRANSFER_METHOD_MATCHED})
			contested[e.out.ID], contested[e.in.ID] = true, true
		}
	}

	return plan
}

// transferEdges lists every plausible out/in pair, best first.
func transferEdges(
	free []*transferTx,
	accounts map[int64]transferAccount,
	used map[int64]bool,
	rejected map[[2]int64]bool,
) []transferEdge {
	tokens := make(map[int64][]string, len(free))
	uses := map[string]int{}
	for _, t := range free {
		tokens[t.ID] = numberTokens(t.Text)
		for _, tok := range tokens[t.ID] {
			uses[tok]++
		}
	}

	var outs, ins []*transferTx
	for _, t := range free {
		if used[t.ID] {
			continue
		}
		switch t.Direction {
		case pb.TransactionDirection_DIRECTION_OUTGOING:
			outs = append(outs, t)
		case pb.TransactionDirection_DIRECTION_INCOMING:
			ins = append(ins, t)
		}
	}

	var edges []transferEdge
	for _, o := range outs {
		for _, i := range ins {
			if o.AccountID == i.AccountID || o.Currency != i.Currency || rejected[[2]int64{o.ID, i.ID}] {
				continue
			}
			days := int(math.Round(math.Abs(o.Date.Sub(i.Date).Hours()) / 24))
			if days > transferMaxDays {
				continue
			}
			exact := o.Cents == i.Cents
			fee := o.Cents > i.Cents && o.Cents-i.Cents <= transferFeeFlatCents+i.Cents*transferFeePercent/100
			if !exact && !fee {
				continue
			}

			signals := 0
			if sharesReference(tokens[o.ID], tokens[i.ID], uses, o.Cents, i.Cents) {
				signals++
			}
			if names(o.Text, accounts[o.AccountID], accounts[i.AccountID]) ||
				names(i.Text, accounts[i.AccountID], accounts[o.AccountID]) {
				signals++
			}
			// a different amount needs something else tying the two together
			if !exact && signals == 0 {
				continue
			}
			edges = append(edges, transferEdge{
				out:   o,
				in:    i,
				score: transferScore{signals: signals, exact: exact, days: days},
				// money arriving on a credit card from another own account is
				// a payment; anything else with no shared detail is a guess
				auto: signals > 0 || accounts[i.AccountID].CreditCard,
			})
		}
	}

	slices.SortStableFunc(edges, func(a, b transferEdge) int {
		if c := a.score.compare(b.score); c != 0 {
			return c
		}
		if a.out.ID != b.out.ID {
			return int(a.out.ID - b.out.ID)
		}
		return int(a.in.ID - b.in.ID)
	})
	return edges
}

func interchangeable(a, b *transferTx) bool {
	return a.AccountID == b.AccountID && a.Cents == b.Cents && a.Currency == b.Currency && a.Direction == b.Direction
}

// numberTokens returns the distinct runs of three or more digits in s.
func numberTokens(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsDigit(r) }) {
		if len(f) >= 3 && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// sharesReference reports whether both descriptions carry the same rare
// number that isn't just one of the amounts.
func sharesReference(a, b []string, uses map[string]int, amounts ...int64) bool {
	for _, tok := range a {
		if uses[tok] > transferTokenMaxUses || !slices.Contains(b, tok) {
			continue
		}
		isAmount := false
		for _, cents := range amounts {
			if tok == strconv.FormatInt(cents/100, 10) || tok == strconv.FormatInt(cents, 10) {
				isAmount = true
			}
		}
		if !isAmount {
			return true
		}
	}
	return false
}

// names reports whether text, written on the own account, mentions the other
// account by its bank, name or one of its aliases. a bank both accounts share
// says nothing about which one is meant.
func names(text string, own, other transferAccount) bool {
	words := wordSet(text)
	if other.Bank != "" && !strings.EqualFold(other.Bank, own.Bank) && containsPhrase(words, other.Bank) {
		return true
	}
	for _, n := range other.Names {
		if containsPhrase(words, n) {
			return true
		}
	}
	return false
}

func wordSet(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// containsPhrase reports whether phrase appears in words as whole words.
func containsPhrase(words []string, phrase string) bool {
	p := wordSet(phrase)
	if len(p) == 0 {
		return false
	}
	if len(p) == 1 && len(p[0]) < 3 {
		return false
	}
	for i := 0; i+len(p) <= len(words); i++ {
		if slices.Equal(words[i:i+len(p)], p) {
			return true
		}
	}
	return false
}
