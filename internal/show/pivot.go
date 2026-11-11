package show

// PlanPivot reports whether ev is a plan change that turns the run (the permit, the mortgage, a
// refusal, a cash floor, …) rather than reshuffling the day's shopping: the same test that makes a
// changed plan a moment.
func PlanPivot(ev Event) bool {
	return ev.Kind == KindPlan && ev.Plan != nil && ev.Plan.Change && substantive(ev.Plan.Changes)
}
