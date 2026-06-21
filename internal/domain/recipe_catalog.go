package domain

// recipe_catalog.go is the CANONICAL, code-defined catalog of crafting recipes (Phase 3).
// It generalizes the hard-coded refinery conversion chain into data: each Recipe declares its
// inputs, outputs and craft time. The player-facing craft queue (ProductionSystem) and the
// visualizer's craft picker both read from this single source of truth.
//
// Input/output keys are ResourceType strings exactly as stored in Cargo (e.g. "Iron",
// "IronPlates", "Microchips", "Laser Cannon"), so applying a recipe is a direct cargo edit.

type Recipe struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Tier        int32            `json:"tier"` // 1 refine, 2 components, 3 assembly (display grouping)
	Inputs      map[string]int32 `json:"inputs"`
	Outputs     map[string]int32 `json:"outputs"`
	TimeSeconds float32          `json:"time_seconds"`
}

// StockRecipes mirrors the legacy refinery chain (refinery.go) as data. Numbers and conversions
// match the old hard-coded behavior so balance is unchanged; only the delivery path is new.
var StockRecipes = []Recipe{
	// Tier 1 — refining (ores -> materials)
	{ID: "refine_iron", Name: "Iron Plates", Tier: 1, Inputs: map[string]int32{"Iron": 2}, Outputs: map[string]int32{"IronPlates": 1}, TimeSeconds: 3},
	{ID: "refine_titanium", Name: "Titanium Plates", Tier: 1, Inputs: map[string]int32{"Titanium": 2}, Outputs: map[string]int32{"TitaniumPlates": 1}, TimeSeconds: 3},
	{ID: "refine_crystal", Name: "Silicon Wafers", Tier: 1, Inputs: map[string]int32{"Crystal": 2}, Outputs: map[string]int32{"SiliconWafers": 1}, TimeSeconds: 3},
	{ID: "refine_gas", Name: "Fuel Cells", Tier: 1, Inputs: map[string]int32{"RareGas": 2}, Outputs: map[string]int32{"FuelCells": 1}, TimeSeconds: 3},
	// Tier 2 — components (materials -> components)
	{ID: "comp_microchips", Name: "Microchips", Tier: 2, Inputs: map[string]int32{"SiliconWafers": 1, "TitaniumPlates": 1}, Outputs: map[string]int32{"Microchips": 1}, TimeSeconds: 5},
	{ID: "comp_energycoils", Name: "Energy Coils", Tier: 2, Inputs: map[string]int32{"FuelCells": 1, "IronPlates": 1}, Outputs: map[string]int32{"EnergyCoils": 1}, TimeSeconds: 5},
	// Tier 3 — assembly (components -> modules)
	{ID: "assemble_laser", Name: "Laser Cannon", Tier: 3, Inputs: map[string]int32{"Microchips": 2, "EnergyCoils": 2}, Outputs: map[string]int32{"Laser Cannon": 1}, TimeSeconds: 8},
	{ID: "assemble_mininglaser", Name: "Mining Laser", Tier: 3, Inputs: map[string]int32{"Microchips": 2, "IronPlates": 2}, Outputs: map[string]int32{"Mining Laser": 1}, TimeSeconds: 8},
}

var recipeByID = map[string]*Recipe{}

func init() {
	for i := range StockRecipes {
		recipeByID[StockRecipes[i].ID] = &StockRecipes[i]
	}
}

// RecipeByID returns a copy of the catalog recipe, or nil.
func RecipeByID(id string) *Recipe {
	if r, ok := recipeByID[id]; ok {
		c := *r
		return &c
	}
	return nil
}
