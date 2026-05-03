package policy

type Decision string

const (
	Accept  Decision = "ACCEPT"
	Reject  Decision = "REJECT"
	Pending Decision = "PENDING"
)

type AcceptanceResult struct {
	Decision Decision
	Score    float64
	Reasons  []string
}
