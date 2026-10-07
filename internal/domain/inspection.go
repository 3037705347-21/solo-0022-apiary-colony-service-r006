package domain

import "time"

type BroodPattern string

const (
	BroodSolid  BroodPattern = "solid"
	BroodPatchy BroodPattern = "patchy"
	BroodNone   BroodPattern = "none"
)

type Temper string

const (
	TemperCalm       Temper = "calm"
	TemperNervous    Temper = "nervous"
	TemperAggressive Temper = "aggressive"
)

// AmendmentKind 是对已固化巡检追加内容的类型。
type AmendmentKind string

const (
	// AmendmentFollowUp 是现场几天后补充的随访结论，可关闭当时派生的关注项。
	AmendmentFollowUp AmendmentKind = "follow_up"
	// AmendmentCorrection 是对备注笔误的更正，不触碰任何量测。
	AmendmentCorrection AmendmentKind = "correction"
)

// 关注项名称集合，与 inspection 分析层的派生结果保持一致，
// 随访只能关闭这些由原始量测派生出的关注项。
const (
	ConcernMiteLoad    = "mite_load"
	ConcernBroodGap    = "brood_gap"
	ConcernTemperRisk  = "temper_risk"
	ConcernLowStrength = "low_strength"
)

// Inspection 是一次现场巡检的固化结果，关注项由量测派生。
type Inspection struct {
	ID             string       `json:"id"`
	HiveID         string       `json:"hiveId"`
	ObservedAt     time.Time    `json:"observedAt"`
	ColonyStrength int          `json:"colonyStrength"`
	BroodPattern   BroodPattern `json:"broodPattern"`
	Temper         Temper       `json:"temper"`
	MiteCount      int          `json:"miteCount"`
	Note           string       `json:"note"`
	Concerns       []string     `json:"concerns"`
}

const (
	MinColonyStrength = 1
	MaxColonyStrength = 5
	MinMiteCount      = 0
	MaxMiteCount      = 60
	MaxNoteRunes      = 200
)

func NewInspection(
	hiveID string,
	observedAt time.Time,
	strength int,
	brood BroodPattern,
	temper Temper,
	miteCount int,
	note string,
) (Inspection, error) {
	if err := ValidateIdentifier("hiveId", hiveID, 3, 40); err != nil {
		return Inspection{}, err
	}
	if observedAt.IsZero() {
		return Inspection{}, Invalid("observedAt", "observedAt 不能为空")
	}
	if err := ValidateIntRange("colonyStrength", strength, MinColonyStrength, MaxColonyStrength); err != nil {
		return Inspection{}, err
	}
	if !ValidBroodPattern(brood) {
		return Inspection{}, Invalid("broodPattern", "broodPattern 必须是 %s、%s 或 %s", BroodSolid, BroodPatchy, BroodNone)
	}
	if !ValidTemper(temper) {
		return Inspection{}, Invalid("temper", "temper 必须是 %s、%s 或 %s", TemperCalm, TemperNervous, TemperAggressive)
	}
	if err := ValidateIntRange("miteCount", miteCount, MinMiteCount, MaxMiteCount); err != nil {
		return Inspection{}, err
	}
	if err := ValidateText("note", note, 0, MaxNoteRunes); err != nil {
		return Inspection{}, err
	}
	return Inspection{
		HiveID:         TrimText(hiveID),
		ObservedAt:     observedAt.UTC(),
		ColonyStrength: strength,
		BroodPattern:   brood,
		Temper:         temper,
		MiteCount:      miteCount,
		Note:           TrimText(note),
		Concerns:       []string{},
	}, nil
}

func ValidBroodPattern(value BroodPattern) bool {
	switch value {
	case BroodSolid, BroodPatchy, BroodNone:
		return true
	default:
		return false
	}
}

func ValidTemper(value Temper) bool {
	switch value {
	case TemperCalm, TemperNervous, TemperAggressive:
		return true
	default:
		return false
	}
}
