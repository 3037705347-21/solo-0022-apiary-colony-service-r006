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

// Inspection 是一次现场巡检的固化结果。原始量测与创建时派生的关注项
// （Concerns）在保存后不再改写；事后补充通过 Amendments 追加，当前生效
// 结论体现在 Effective 上。
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
	Effective      Conclusion   `json:"effective"`
	Amendments     []Amendment  `json:"amendments"`
}

// Conclusion 是一份结论视图：量测、备注与按固定阈值派生的关注项。
// 创建时它与原始记录一致，追加随访或更正后它表示当前生效的结论。
type Conclusion struct {
	ColonyStrength int          `json:"colonyStrength"`
	BroodPattern   BroodPattern `json:"broodPattern"`
	Temper         Temper       `json:"temper"`
	MiteCount      int          `json:"miteCount"`
	Note           string       `json:"note"`
	Concerns       []string     `json:"concerns"`
}

// Apply 把一条追加作用到生效结论上：被设置的字段覆盖当前值，
// 关注项由调用方按最新量测重新派生。
func (c Conclusion) Apply(amendment Amendment) Conclusion {
	if amendment.ColonyStrength != nil {
		c.ColonyStrength = *amendment.ColonyStrength
	}
	if amendment.BroodPattern != nil {
		c.BroodPattern = *amendment.BroodPattern
	}
	if amendment.Temper != nil {
		c.Temper = *amendment.Temper
	}
	if amendment.MiteCount != nil {
		c.MiteCount = *amendment.MiteCount
	}
	if amendment.Note != nil {
		c.Note = *amendment.Note
	}
	c.Concerns = []string{}
	return c
}

type AmendmentKind string

const (
	AmendmentFollowup   AmendmentKind = "followup"
	AmendmentCorrection AmendmentKind = "correction"
)

// Amendment 是对一次巡检的追加（随访结论或笔误更正）。追加只增不改：
// 未设置的字段保持生效结论中的原值，被设置的字段覆盖生效结论。
type Amendment struct {
	ID             string        `json:"id"`
	Kind           AmendmentKind `json:"kind"`
	RecordedAt     time.Time     `json:"recordedAt"`
	ColonyStrength *int          `json:"colonyStrength,omitempty"`
	BroodPattern   *BroodPattern `json:"broodPattern,omitempty"`
	Temper         *Temper       `json:"temper,omitempty"`
	MiteCount      *int          `json:"miteCount,omitempty"`
	Note           *string       `json:"note,omitempty"`
}

// SameContent 比较两条追加的业务内容是否一致，用于幂等重放判定；
// 记录时间由服务写入，不参与比较。
func (a Amendment) SameContent(other Amendment) bool {
	return a.Kind == other.Kind &&
		equalOptional(a.ColonyStrength, other.ColonyStrength) &&
		equalOptional(a.BroodPattern, other.BroodPattern) &&
		equalOptional(a.Temper, other.Temper) &&
		equalOptional(a.MiteCount, other.MiteCount) &&
		equalOptional(a.Note, other.Note)
}

func equalOptional[T comparable](left, right *T) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// AmendmentDraft 是建立追加记录前的未校验输入。
type AmendmentDraft struct {
	ID             string
	Kind           AmendmentKind
	ColonyStrength *int
	BroodPattern   *BroodPattern
	Temper         *Temper
	MiteCount      *int
	Note           *string
	RecordedAt     time.Time
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
	trimmedNote := TrimText(note)
	return Inspection{
		HiveID:         TrimText(hiveID),
		ObservedAt:     observedAt.UTC(),
		ColonyStrength: strength,
		BroodPattern:   brood,
		Temper:         temper,
		MiteCount:      miteCount,
		Note:           trimmedNote,
		Concerns:       []string{},
		Effective: Conclusion{
			ColonyStrength: strength,
			BroodPattern:   brood,
			Temper:         temper,
			MiteCount:      miteCount,
			Note:           trimmedNote,
			Concerns:       []string{},
		},
		Amendments: []Amendment{},
	}, nil
}

// NewAmendment 校验并建立一条追加记录。追加至少要携带一项内容
// （任一量测更正或备注），否则对生效结论没有影响。
func NewAmendment(draft AmendmentDraft) (Amendment, error) {
	if err := ValidateIdentifier("amendmentId", draft.ID, 3, 40); err != nil {
		return Amendment{}, err
	}
	if !ValidAmendmentKind(draft.Kind) {
		return Amendment{}, Invalid("kind", "kind 必须是 %s 或 %s", AmendmentFollowup, AmendmentCorrection)
	}
	if draft.ColonyStrength != nil {
		if err := ValidateIntRange("colonyStrength", *draft.ColonyStrength, MinColonyStrength, MaxColonyStrength); err != nil {
			return Amendment{}, err
		}
	}
	if draft.BroodPattern != nil && !ValidBroodPattern(*draft.BroodPattern) {
		return Amendment{}, Invalid("broodPattern", "broodPattern 必须是 %s、%s 或 %s", BroodSolid, BroodPatchy, BroodNone)
	}
	if draft.Temper != nil && !ValidTemper(*draft.Temper) {
		return Amendment{}, Invalid("temper", "temper 必须是 %s、%s 或 %s", TemperCalm, TemperNervous, TemperAggressive)
	}
	if draft.MiteCount != nil {
		if err := ValidateIntRange("miteCount", *draft.MiteCount, MinMiteCount, MaxMiteCount); err != nil {
			return Amendment{}, err
		}
	}
	var note *string
	if draft.Note != nil {
		if err := ValidateText("note", *draft.Note, 0, MaxNoteRunes); err != nil {
			return Amendment{}, err
		}
		trimmed := TrimText(*draft.Note)
		note = &trimmed
	}
	if draft.ColonyStrength == nil && draft.BroodPattern == nil && draft.Temper == nil &&
		draft.MiteCount == nil && note == nil {
		return Amendment{}, Invalid("amendment", "追加至少需要携带一项量测更正或备注")
	}
	recordedAt := draft.RecordedAt
	if recordedAt.IsZero() {
		return Amendment{}, Invalid("recordedAt", "recordedAt 不能为空")
	}
	return Amendment{
		ID:             TrimText(draft.ID),
		Kind:           draft.Kind,
		RecordedAt:     recordedAt.UTC(),
		ColonyStrength: draft.ColonyStrength,
		BroodPattern:   draft.BroodPattern,
		Temper:         draft.Temper,
		MiteCount:      draft.MiteCount,
		Note:           note,
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

func ValidAmendmentKind(kind AmendmentKind) bool {
	switch kind {
	case AmendmentFollowup, AmendmentCorrection:
		return true
	default:
		return false
	}
}
