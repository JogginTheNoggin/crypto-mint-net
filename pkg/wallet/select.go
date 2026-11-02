package wallet

import (
	"sort"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// Spendable is a portion the wallet can spend now.
type Spendable struct {
	Serial  uint64  `json:"serial"`
	Portion Portion `json:"portion"`
	Owner   Owner   `json:"owner"`
}

// plan is one leg of a payment before it is built: the portions of one
// mint it spends and what it pays the receiver.
type plan struct {
	serial uint64
	inputs []Spendable
	pay    uint64
	total  uint64
}

// pickIn chooses portions of one mint to cover amount: the smallest single
// portion that covers it if there is one, else the largest first. ok is
// false when the mint holds too little.
func pickIn(ps []Spendable, amount uint64) (in []Spendable, total uint64, ok bool) {
	best := -1
	for i, p := range ps {
		if p.Portion.Amount >= amount && (best < 0 || p.Portion.Amount < ps[best].Portion.Amount) {
			best = i
		}
	}
	if best >= 0 {
		return []Spendable{ps[best]}, ps[best].Portion.Amount, true
	}
	sorted := append([]Spendable(nil), ps...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Portion.Amount > sorted[j].Portion.Amount })
	for _, p := range sorted {
		in = append(in, p)
		total += p.Portion.Amount
		if total >= amount {
			return in, total, true
		}
	}
	return nil, total, false
}

// choose plans a payment of amount from the spendable portions (decision
// D65). One mint is best: a merge and pay in one transaction. Else the
// fewest mints of one pool, since legs in one pool settle on one ledger.
// Else mints across pools, largest first, which the receiver must accept
// leg by leg.
func choose(s *series.Series, sp []Spendable, amount uint64) ([]plan, bool) {
	bySerial := map[uint64][]Spendable{}
	totals := map[uint64]uint64{}
	for _, x := range sp {
		bySerial[x.Serial] = append(bySerial[x.Serial], x)
		totals[x.Serial] += x.Portion.Amount
	}
	serials := make([]uint64, 0, len(bySerial))
	for n := range bySerial {
		serials = append(serials, n)
	}
	sort.Slice(serials, func(i, j int) bool { return serials[i] < serials[j] })

	// One mint: fewest inputs, then least change.
	var one *plan
	for _, n := range serials {
		in, total, ok := pickIn(bySerial[n], amount)
		if !ok {
			continue
		}
		p := plan{serial: n, inputs: in, pay: amount, total: total}
		if one == nil || len(p.inputs) < len(one.inputs) ||
			(len(p.inputs) == len(one.inputs) && p.total-p.pay < one.total-one.pay) {
			one = &p
		}
	}
	if one != nil {
		return []plan{*one}, true
	}

	// Several mints, largest first, from the given serials.
	greedy := func(ns []uint64) ([]plan, bool) {
		ns = append([]uint64(nil), ns...)
		sort.SliceStable(ns, func(i, j int) bool { return totals[ns[i]] > totals[ns[j]] })
		var out []plan
		left := amount
		for _, n := range ns {
			if left == 0 {
				break
			}
			if totals[n] <= left {
				out = append(out, plan{serial: n, inputs: bySerial[n], pay: totals[n], total: totals[n]})
				left -= totals[n]
				continue
			}
			in, total, _ := pickIn(bySerial[n], left)
			out = append(out, plan{serial: n, inputs: in, pay: left, total: total})
			left = 0
		}
		return out, left == 0
	}

	// One pool: the pool needing the fewest mints.
	pools := map[uint64][]uint64{}
	for _, n := range serials {
		pools[s.PoolOf(n)] = append(pools[s.PoolOf(n)], n)
	}
	var best []plan
	for _, ns := range pools {
		if out, ok := greedy(ns); ok && (best == nil || len(out) < len(best) ||
			(len(out) == len(best) && out[0].serial < best[0].serial)) {
			best = out
		}
	}
	if best != nil {
		return best, true
	}
	return greedy(serials)
}
