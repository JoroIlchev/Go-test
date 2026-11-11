package campaign

import "math/rand/v2"

// maxSequence caps how long an expanded cycle may get; beyond it the
// "every Nth" pattern is only approximately kept across the cycle's seam.
const maxSequence = 240

// Sequence is the order the playlist plays in, as indexes into s.Items, for
// one cycle that the player then repeats.
//
// Items with Every >= 2 are "focus" items shown as every Nth slot (a product
// on sale as every 3rd picture); the others play in their order, or in an
// order shuffled with seed when s.Order is "shuffle". The cycle runs over
// enough rounds of the other items that the pattern continues seamlessly
// when it repeats.
func (s Spec) Sequence(seed uint64) []int {
	var regular, focus []int
	for i, it := range s.Items {
		if it.Every >= 2 {
			focus = append(focus, i)
		} else {
			regular = append(regular, i)
		}
	}
	rng := rand.New(rand.NewPCG(seed, 0x6d61727175656521))
	round := func() []int {
		r := append([]int(nil), regular...)
		if s.Order == "shuffle" {
			rng.Shuffle(len(r), func(i, j int) { r[i], r[j] = r[j], r[i] })
		}
		return r
	}
	if len(focus) == 0 {
		return round()
	}
	if len(regular) == 0 {
		return focus
	}

	// Rounds needed for the pattern to line up with the end of the cycle.
	rounds := 1
	for _, f := range focus {
		gap := s.Items[f].Every - 1
		rounds = lcm(rounds, gap/gcd(len(regular), gap))
	}
	if rounds*len(regular)*2 > maxSequence {
		rounds = max(1, maxSequence/2/len(regular))
	}
	var regs []int
	for range rounds {
		regs = append(regs, round()...)
	}

	// Slot by slot: a focus item whose turn has come goes first (the most
	// overdue one when several are), otherwise the next regular item.
	last := make([]int, len(focus))
	due := func(slot int) int {
		best, late := -1, -1
		for j, f := range focus {
			if l := slot - last[j] - s.Items[f].Every; l >= 0 && l > late {
				best, late = j, l
			}
		}
		return best
	}
	var out []int
	for slot, next := 1, 0; ; slot++ {
		if len(out) >= 4*maxSequence { // focus items leaving no room (ValidateDraft rejects that)
			return out
		}
		if j := due(slot); j >= 0 {
			out = append(out, focus[j])
			last[j] = slot
			continue
		}
		if next == len(regs) {
			return out
		}
		out = append(out, regs[next])
		next++
	}
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func lcm(a, b int) int { return a / gcd(a, b) * b }
