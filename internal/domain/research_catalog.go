package domain

// research_catalog.go is the canonical, code-defined catalog of research projects (Phase 3).
// Completing a project unlocks recipes (and, later, hullmods) for that player. Projects cost
// credits to start and take real time to complete; some require prerequisite projects.

type ResearchProject struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Cost        int64    `json:"cost"`         // credits charged when research starts
	TimeSeconds float32  `json:"time_seconds"` // research duration
	Prereqs     []string `json:"prereqs"`      // project IDs that must be completed first
	Unlocks     []string `json:"unlocks"`      // recipe IDs unlocked on completion (display/info)
}

// StockResearch is the canonical research catalog.
var StockResearch = []ResearchProject{
	{
		ID: "adv_weapons", Name: "Продвинутое оружие", Cost: 500, TimeSeconds: 20,
		Prereqs: nil, Unlocks: []string{"assemble_heavy_blaster", "assemble_heavy_mauler"},
	},
	{
		ID: "capital_weapons", Name: "Орудия капитальных кораблей", Cost: 1500, TimeSeconds: 35,
		Prereqs: []string{"adv_weapons"}, Unlocks: []string{"assemble_hellbore"},
	},
}

var researchByID = map[string]*ResearchProject{}

func init() {
	for i := range StockResearch {
		researchByID[StockResearch[i].ID] = &StockResearch[i]
	}
}

// ResearchByID returns a copy of the catalog project, or nil.
func ResearchByID(id string) *ResearchProject {
	if r, ok := researchByID[id]; ok {
		c := *r
		return &c
	}
	return nil
}
