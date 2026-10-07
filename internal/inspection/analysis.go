package inspection

import "example.com/apiary-colony-service/internal/domain"

const (
	ConcernMiteLoad    = "mite_load"
	ConcernBroodGap    = "brood_gap"
	ConcernTemperRisk  = "temper_risk"
	ConcernLowStrength = "low_strength"
)

const (
	MiteLoadThreshold  = 9
	LowStrengthCeiling = 2
)

// DeriveConcerns 按固定阈值从一份结论量测派生出关注项，顺序稳定。
// 原始记录与每次追加后的生效结论使用同一套阈值。
func DeriveConcerns(conclusion domain.Conclusion) []string {
	concerns := make([]string, 0, 4)
	if conclusion.MiteCount >= MiteLoadThreshold {
		concerns = append(concerns, ConcernMiteLoad)
	}
	if conclusion.BroodPattern == domain.BroodNone {
		concerns = append(concerns, ConcernBroodGap)
	}
	if conclusion.Temper == domain.TemperAggressive {
		concerns = append(concerns, ConcernTemperRisk)
	}
	if conclusion.ColonyStrength <= LowStrengthCeiling {
		concerns = append(concerns, ConcernLowStrength)
	}
	return concerns
}
