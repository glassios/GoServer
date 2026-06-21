package domain

// progression.go is the Phase 3 skills/XP spine. A player accrues XP in a handful of skills from
// the activities they do (mining, crafting, combat); each level grants a small, concrete bonus that
// other systems read (mining yield, craft speed, weapon damage). Skills start at level 1.

const (
	SkillMining      = "mining"
	SkillEngineering = "engineering"
	SkillCombat      = "combat"
)

// SkillKeys is the canonical, ordered list of skills (used for UI + persistence iteration).
var SkillKeys = []string{SkillMining, SkillEngineering, SkillCombat}

type SkillState struct {
	Level int32
	XP    int32 // progress toward the next level
}

// PlayerProgress is the per-player skill set.
type PlayerProgress struct {
	Skills map[string]*SkillState
}

// NewPlayerProgress returns a fresh progress with every known skill at level 1.
func NewPlayerProgress() *PlayerProgress {
	p := &PlayerProgress{Skills: make(map[string]*SkillState, len(SkillKeys))}
	for _, k := range SkillKeys {
		p.Skills[k] = &SkillState{Level: 1}
	}
	return p
}

func (p *PlayerProgress) ensure(skill string) *SkillState {
	if p.Skills == nil {
		p.Skills = map[string]*SkillState{}
	}
	st, ok := p.Skills[skill]
	if !ok {
		st = &SkillState{Level: 1}
		p.Skills[skill] = st
	}
	if st.Level < 1 {
		st.Level = 1
	}
	return st
}

// Level returns the (>=1) level of a skill, tolerating a nil receiver.
func (p *PlayerProgress) Level(skill string) int32 {
	if p == nil || p.Skills == nil {
		return 1
	}
	if st, ok := p.Skills[skill]; ok && st.Level >= 1 {
		return st.Level
	}
	return 1
}

// XPForNextLevel is the XP required to advance FROM the given level to the next.
func XPForNextLevel(level int32) int32 {
	if level < 1 {
		level = 1
	}
	return 100 * level
}

// AddXP adds XP to a skill and applies any resulting level-ups. Returns the number of levels gained.
func (p *PlayerProgress) AddXP(skill string, amount int32) int32 {
	if amount <= 0 {
		return 0
	}
	st := p.ensure(skill)
	st.XP += amount
	gained := int32(0)
	for st.XP >= XPForNextLevel(st.Level) {
		st.XP -= XPForNextLevel(st.Level)
		st.Level++
		gained++
	}
	return gained
}

// --- Skill bonuses (read by the relevant systems) ---

// MiningYieldMult scales mined amount: +10% per mining level above 1.
func (p *PlayerProgress) MiningYieldMult() float32 {
	return 1 + 0.10*float32(p.Level(SkillMining)-1)
}

// CraftTimeMult scales craft duration: -5% per engineering level above 1, floored at 20%.
func (p *PlayerProgress) CraftTimeMult() float32 {
	m := 1 - 0.05*float32(p.Level(SkillEngineering)-1)
	if m < 0.2 {
		m = 0.2
	}
	return m
}

// WeaponDamageMult scales weapon damage: +5% per combat level above 1.
func (p *PlayerProgress) WeaponDamageMult() float32 {
	return 1 + 0.05*float32(p.Level(SkillCombat)-1)
}
