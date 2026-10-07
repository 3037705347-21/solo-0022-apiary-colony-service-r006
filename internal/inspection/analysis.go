package inspection

import "example.com/apiary-colony-service/internal/domain"

const (
	MiteLoadThreshold  = 9
	LowStrengthCeiling = 2
)

// DeriveConcerns 按固定阈值从一次巡检派生出关注项，顺序稳定。
// 关注项名称常量定义在 domain 中，随访关闭关注项时复用同一套名称。
func DeriveConcerns(record domain.Inspection) []string {
	concerns := make([]string, 0, 4)
	if record.MiteCount >= MiteLoadThreshold {
		concerns = append(concerns, domain.ConcernMiteLoad)
	}
	if record.BroodPattern == domain.BroodNone {
		concerns = append(concerns, domain.ConcernBroodGap)
	}
	if record.Temper == domain.TemperAggressive {
		concerns = append(concerns, domain.ConcernTemperRisk)
	}
	if record.ColonyStrength <= LowStrengthCeiling {
		concerns = append(concerns, domain.ConcernLowStrength)
	}
	return concerns
}
