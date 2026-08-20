package finance

import "math"

// Remaining returns how much of a budget is left after committed spend.
// Negative values mean the budget is overspent.
func Remaining(budgetAmount, committed float64) float64 {
	return roundMoney(budgetAmount - committed)
}

// WithinBudget reports whether adding amount would stay within the budget.
func WithinBudget(budgetAmount, committed, amount float64) bool {
	return Remaining(budgetAmount, committed+amount) >= 0
}

func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}
