package policy

import (
	"errors"
	"fmt"
	"sort"
)

// ErrUnspendable: no witness for the policy can decode or fit the series'
// witness ceiling, so funds paid to its address could never be spent
// (RED-077, RED-079, D169). It is a check where an address is made or
// paid, never a rule of validity: every witness that decodes and fits
// already passes it, because the sizes it adds up are the least any
// witness of the policy can have.
var ErrUnspendable = errors.New("policy: no witness for this policy can be spent")

// impossible stands for a size no witness can have.
const impossible = int(^uint(0) >> 1)

// CheckSpendable reports whether some witness for p could decode at every
// node and fit rules' witness ceiling. It adds up the least witness data
// p can be satisfied with: for a threshold, K signatures of the smallest
// sizes among its keys; for k-of-policies, the K cheapest parts; for a
// hashlock, an empty preimage. A type it does not know counts as no data.
// The decode limits (MaxDataSize, MaxParamsSize) are consensus and are
// not changed here.
func (r *Registry) CheckSpendable(rules Rules, p Policy) error {
	if len(p.Params) > MaxParamsSize {
		return fmt.Errorf("%w: its params are %d bytes, and a node reads at most %d", ErrUnspendable, len(p.Params), MaxParamsSize)
	}
	min, hint, err := r.minData(p, 1)
	if err != nil {
		return err
	}
	if min > MaxDataSize {
		size := "more than the limit"
		if min != impossible {
			size = fmt.Sprintf("at least %d bytes", min)
		}
		return fmt.Errorf("%w: its witness data is %s, and a node reads at most %d%s", ErrUnspendable, size, MaxDataSize, hint)
	}
	if max := rules.MaxWitnessSize(); max > 0 {
		if w := 12 + len(p.Params) + 4 + min; w > max {
			return fmt.Errorf("%w: its witness is at least %d bytes, and the series allows at most %d (max_witness)", ErrUnspendable, w, max)
		}
	}
	return nil
}

// minData returns the least witness data that satisfies p, impossible when
// none fits MaxDataSize, and a hint that says how many signatures fit.
func (r *Registry) minData(p Policy, depth int) (int, string, error) {
	switch p.Type {
	case TypeThreshold:
		tp, err := DecodeThresholdParams(r, p.Params)
		if err != nil {
			return 0, "", err
		}
		sizes := make([]int, len(tp.Keys))
		for i, k := range tp.Keys {
			a, _ := r.Alg(k.Alg)
			sizes[i] = 2 + 4 + a.SignatureSize()
		}
		sort.Ints(sizes)
		total, fit, sum := 4, 0, 4
		for i, s := range sizes {
			if sum += s; sum <= MaxDataSize {
				fit = i + 1
			}
			if i < int(tp.K) {
				total += s
			}
		}
		hint := ""
		if total > MaxDataSize {
			hint = fmt.Sprintf(": it needs %d signatures, and at most %d of these keys' signatures fit; ask for fewer, or use keys with smaller signatures", tp.K, fit)
		}
		return total, hint, nil
	case TypeHashlock:
		return 4, "", nil
	case TypeKOfPolicies:
		if depth > MaxPolicyDepth {
			return 0, "", fmt.Errorf("%w: nested deeper than %d", ErrBadParams, MaxPolicyDepth)
		}
		kp, err := DecodeKOfPoliciesParams(p.Params)
		if err != nil {
			return 0, "", err
		}
		var parts []int
		hint := ""
		for _, q := range kp.Policies {
			m, h, err := r.minData(q, depth+1)
			if err != nil {
				return 0, "", err
			}
			if m > MaxDataSize {
				if hint == "" {
					hint = h
				}
				continue
			}
			parts = append(parts, 2+4+m)
		}
		if len(parts) < int(kp.K) {
			return impossible, fmt.Sprintf(": %d of its policies are needed and only %d can be satisfied%s", kp.K, len(parts), hint), nil
		}
		sort.Ints(parts)
		total := 4
		for _, s := range parts[:kp.K] {
			total += s
		}
		return total, "", nil
	}
	return 0, "", nil
}
