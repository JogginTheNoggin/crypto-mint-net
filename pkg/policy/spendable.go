package policy

import (
	"errors"
	"fmt"
	"sort"
)

// ErrUnspendable: a witness for the policy might not fit the series'
// witness ceiling, so funds paid to its address could not be spent by
// every set of signers it allows, or by any (RED-077, RED-079, D169,
// D170). It is a check where an address is made or paid, never a rule of
// validity.
var ErrUnspendable = errors.New("policy: a witness for this policy might not fit the series")

// CheckSpendable reports whether every witness p allows fits rules'
// witness ceiling (Red's condition 5, D170): the worst case, so that any
// set of signers the policy accepts can spend. It adds up the largest
// witness data p can be satisfied with: for a threshold, K signatures of
// the largest sizes among its keys; for k-of-policies, the K dearest
// parts; for a hashlock, a preimage of MaxPreimageSize bytes. A type it
// does not know counts as no data. With no ceiling every policy fits.
//
// There is no network-wide size limit (D170), so the series' ceiling is
// the only one checked.
func (r *Registry) CheckSpendable(rules Rules, p Policy) error {
	most, err := r.witnessData(p, 1, true)
	if err != nil {
		return err
	}
	max := rules.MaxWitnessSize()
	if max <= 0 {
		return nil
	}
	frame := 12 + len(p.Params) + 4
	if frame+most <= max {
		return nil
	}
	least, err := r.witnessData(p, 1, false)
	if err != nil {
		return err
	}
	if frame+least > max {
		return fmt.Errorf("%w: its smallest witness is %d bytes, and the series allows at most %d (max_witness)%s", ErrUnspendable, frame+least, max, r.fitHint(p, max-frame))
	}
	return fmt.Errorf("%w: its largest witness is %d bytes, and the series allows at most %d (max_witness), so some sets of signers could not spend%s", ErrUnspendable, frame+most, max, r.fitHint(p, max-frame))
}

// witnessData returns the largest (most) or the least witness data that
// satisfies p.
func (r *Registry) witnessData(p Policy, depth int, most bool) (int, error) {
	pick := func(sizes []int, k int) int {
		if most {
			sort.Sort(sort.Reverse(sort.IntSlice(sizes)))
		} else {
			sort.Ints(sizes)
		}
		total := 4
		for _, s := range sizes[:k] {
			total += s
		}
		return total
	}
	switch p.Type {
	case TypeThreshold:
		tp, err := DecodeThresholdParams(r, p.Params)
		if err != nil {
			return 0, err
		}
		sizes := make([]int, len(tp.Keys))
		for i, k := range tp.Keys {
			a, _ := r.Alg(k.Alg)
			sizes[i] = 2 + 4 + a.SignatureSize()
		}
		return pick(sizes, int(tp.K)), nil
	case TypeHashlock:
		if most {
			return 4 + MaxPreimageSize, nil
		}
		return 4, nil
	case TypeKOfPolicies:
		if depth > MaxPolicyDepth {
			return 0, fmt.Errorf("%w: nested deeper than %d", ErrBadParams, MaxPolicyDepth)
		}
		kp, err := DecodeKOfPoliciesParams(p.Params)
		if err != nil {
			return 0, err
		}
		sizes := make([]int, len(kp.Policies))
		for i, q := range kp.Policies {
			m, err := r.witnessData(q, depth+1, most)
			if err != nil {
				return 0, err
			}
			sizes[i] = 2 + 4 + m
		}
		return pick(sizes, int(kp.K)), nil
	}
	return 0, nil
}

// fitHint says, for a threshold policy, how many of its keys' signatures
// fit in budget bytes of witness data, the largest first.
func (r *Registry) fitHint(p Policy, budget int) string {
	if p.Type != TypeThreshold {
		return ""
	}
	tp, err := DecodeThresholdParams(r, p.Params)
	if err != nil {
		return ""
	}
	sizes := make([]int, len(tp.Keys))
	for i, k := range tp.Keys {
		a, _ := r.Alg(k.Alg)
		sizes[i] = 2 + 4 + a.SignatureSize()
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sizes)))
	fit, sum := 0, 4
	for i, s := range sizes {
		if sum += s; sum <= budget {
			fit = i + 1
		}
	}
	return fmt.Sprintf(": it needs %d signatures, and at most %d of these keys' signatures surely fit; ask for fewer, use keys with smaller signatures, or use a series with a larger ceiling", tp.K, fit)
}

// MaxK returns the largest k for which a k-of-k threshold policy of
// algorithm a fits a witness ceiling of max bytes (0: none), counting the
// policy's params as well as its signatures. It is at most
// MaxThresholdKeys. poo crypto shows it (Red's condition 5, D170).
func MaxK(a KeyAlg, max int) int {
	if max <= 0 {
		return MaxThresholdKeys
	}
	k := 0
	for n := 1; n <= MaxThresholdKeys; n++ {
		params := 2 + 4 + n*(4+4+a.PublicKeySize())
		data := 4 + n*(2+4+a.SignatureSize())
		if 12+params+4+data > max {
			break
		}
		k = n
	}
	return k
}
