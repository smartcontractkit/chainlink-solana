package fees

// returns new fee based on number of times bumped
func CalculateFee(base, max, min uint64, count uint) uint64 {
	amount := base

	for i := uint(0); i < count; i++ {
		if base == 0 && i == 0 {
			amount = 1
		} else {
			next := amount + amount
			if next <= amount {
				// overflowed
				amount = max
				break
			}
			amount = next
		}
	}

	// respect bounds
	if amount < min {
		return min
	}
	if amount > max {
		return max
	}
	return amount
}
